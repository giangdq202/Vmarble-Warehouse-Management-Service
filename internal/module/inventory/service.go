package inventory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type service struct {
	st store
}

func NewService(st store) Service {
	return &service{st: st}
}

func (s *service) ReceiveStock(ctx context.Context, in ReceiveStockInput) (InventoryLot, error) {
	if in.Quantity <= 0 {
		return InventoryLot{}, domain.NewBizError(domain.ErrInvalidInput, "quantity must be positive")
	}
	if !in.Dimensions.Valid() {
		return InventoryLot{}, domain.NewBizError(domain.ErrInvalidInput, "invalid dimensions")
	}

	lot := InventoryLot{
		ID:           uuid.New(),
		MaterialID:   in.MaterialID,
		Quantity:     in.Quantity,
		CostPerSheet: in.CostPerSheet,
		SupplierRef:  in.SupplierRef,
		ReceivedAt:   time.Now().UTC(),
	}
	if err := s.st.insertLot(ctx, lot); err != nil {
		return InventoryLot{}, err
	}

	sheets := make([]BoardSheet, in.Quantity)
	for i := 0; i < in.Quantity; i++ {
		sheets[i] = BoardSheet{
			ID:           uuid.New(),
			LotID:        lot.ID,
			Dimensions:   in.Dimensions,
			CostPerSheet: in.CostPerSheet,
			Status:       "AVAILABLE",
		}
	}
	if err := s.st.insertSheets(ctx, sheets); err != nil {
		return InventoryLot{}, err
	}

	return lot, nil
}

func (s *service) ListLots(ctx context.Context) ([]InventoryLot, error) {
	return s.st.selectLots(ctx)
}

func (s *service) GetSheet(ctx context.Context, sheetID uuid.UUID) (BoardSheet, error) {
	return s.st.selectSheetByID(ctx, sheetID)
}

func (s *service) ListAvailableSheets(ctx context.Context) ([]BoardSheet, error) {
	return s.st.selectAvailableSheets(ctx)
}

func (s *service) RecordCut(ctx context.Context, in RecordCutInput) (CutResult, error) {
	if (in.SheetID == nil && in.RemnantID == nil) || (in.SheetID != nil && in.RemnantID != nil) {
		return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "exactly one of sheet_id or remnant_id must be set")
	}
	if !in.UsedDimension.Valid() {
		return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "invalid used dimension")
	}
	if in.RemnantDimension != nil && !in.RemnantDimension.Valid() {
		return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "invalid remnant dimension")
	}

	var sourceDim domain.Dimension
	var parentBoardID uuid.UUID
	var parentRemnantID *uuid.UUID

	if in.SheetID != nil {
		sheet, err := s.st.selectSheetByID(ctx, *in.SheetID)
		if err != nil {
			return CutResult{}, err
		}
		if sheet.Status != "AVAILABLE" {
			return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "sheet is not available")
		}
		sourceDim = sheet.Dimensions
		parentBoardID = sheet.ID
		parentRemnantID = nil
	} else {
		remnant, err := s.st.selectRemnantByID(ctx, *in.RemnantID)
		if err != nil {
			return CutResult{}, err
		}
		if remnant.Status != domain.RemnantAvailable {
			return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "remnant is not available")
		}
		sourceDim = remnant.Dimensions
		parentBoardID = remnant.ParentBoardID
		parentRemnantID = &remnant.ID
	}

	usedArea := in.UsedDimension.AreaSqMM()
	remnantArea := int64(0)
	if in.RemnantDimension != nil {
		remnantArea = in.RemnantDimension.AreaSqMM()
		if !in.RemnantDimension.FitsInside(sourceDim) {
			return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "remnant dimension does not fit inside source")
		}
	}
	sourceArea := sourceDim.AreaSqMM()
	if usedArea+remnantArea > sourceArea {
		return CutResult{}, domain.NewBizError(domain.ErrAreaConservation, "used + remnant area exceeds source area")
	}

	cr := CuttingRecord{
		ID:           uuid.New(),
		WorkOrderID:  in.WorkOrderID,
		SKUID:        in.SKUID,
		UsedLengthMM: in.UsedDimension.LengthMM,
		UsedWidthMM:  in.UsedDimension.WidthMM,
		CreatedAt:    time.Now().UTC(),
	}

	op := cutWriteOp{Record: cr}

	if in.SheetID != nil {
		cr.SheetID = in.SheetID
		op.Record = cr
		op.SheetUpdate = &sheetStatusUpdate{
			ID:         *in.SheetID,
			Status:     "ISSUED",
			IssuedToWO: &in.WorkOrderID,
		}
	} else {
		cr.RemnantSourceID = in.RemnantID
		op.Record = cr
		op.RemnantUpdate = &remnantStatusUpdate{
			ID:     *in.RemnantID,
			Status: domain.RemnantConsumed,
		}
	}

	if in.RemnantDimension != nil {
		newRemnant := Remnant{
			ID:              uuid.New(),
			ParentBoardID:   parentBoardID,
			ParentRemnantID: parentRemnantID,
			Dimensions:      *in.RemnantDimension,
			Status:          domain.RemnantAvailable,
			CreatedAt:       time.Now().UTC(),
		}
		op.NewRemnant = &newRemnant
	}

	if err := s.st.recordCutAtomically(ctx, op); err != nil {
		return CutResult{}, err
	}

	result := CutResult{CuttingRecordID: cr.ID}
	if op.NewRemnant != nil {
		result.RemnantID = &op.NewRemnant.ID
	}
	return result, nil
}

func (s *service) FindAvailableRemnants(ctx context.Context, minDim domain.Dimension) ([]Remnant, error) {
	return s.st.selectAvailableRemnantsByMinDimension(ctx, minDim)
}

func (s *service) AllocateRemnant(ctx context.Context, remnantID uuid.UUID, workOrderID uuid.UUID) error {
	// Optimistic pre-check: give an early ErrInvalidInput when the remnant is
	// clearly not AVAILABLE before we even open a transaction. The authoritative
	// check happens inside allocateRemnantAtomically under a row-level lock
	// (SELECT … FOR UPDATE), so concurrent callers that pass this guard can
	// still be rejected by ErrPreconditionFailed from the store layer.
	remnant, err := s.st.selectRemnantByID(ctx, remnantID)
	if err != nil {
		return err
	}
	if remnant.Status != domain.RemnantAvailable {
		return domain.NewBizError(domain.ErrInvalidInput, "remnant is not available for allocation")
	}
	return s.st.allocateRemnantAtomically(ctx, remnantID, workOrderID)
}

func (s *service) MarkRemnantWaste(ctx context.Context, remnantID uuid.UUID) error {
	// Optimistic pre-check: give an early ErrInvalidInput for clearly invalid
	// states. The authoritative check under lock is in markRemnantWasteAtomically.
	remnant, err := s.st.selectRemnantByID(ctx, remnantID)
	if err != nil {
		return err
	}
	if remnant.Status != domain.RemnantAvailable && remnant.Status != domain.RemnantAllocated {
		return domain.NewBizError(domain.ErrInvalidInput, "remnant cannot be marked waste")
	}
	return s.st.markRemnantWasteAtomically(ctx, remnantID)
}

func (s *service) GetRemnantLineage(ctx context.Context, boardSheetID uuid.UUID) ([]Remnant, error) {
	return s.st.selectRemnantsByBoardSheet(ctx, boardSheetID)
}
