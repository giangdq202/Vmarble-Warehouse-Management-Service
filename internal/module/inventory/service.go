package inventory

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	st  store
	woa WorkOrderAdvancer
	bcg BarcodeGenerator
}

func NewService(st store, woa WorkOrderAdvancer, bcg ...BarcodeGenerator) Service {
	var generator BarcodeGenerator
	if len(bcg) > 0 {
		generator = bcg[0]
	}
	return &service{st: st, woa: woa, bcg: generator}
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
		IsActive:     true,
		ReceivedAt:   time.Now().UTC(),
	}
	if err := s.st.insertLot(ctx, lot); err != nil {
		return InventoryLot{}, err
	}

	lotBatch := &lot.SupplierRef
	sheets := make([]BoardSheet, in.Quantity)
	for i := 0; i < in.Quantity; i++ {
		sheets[i] = BoardSheet{
			ID:           uuid.New(),
			LotID:        lot.ID,
			Dimensions:   in.Dimensions,
			CostPerSheet: in.CostPerSheet,
			Status:       "AVAILABLE",
			LotBatch:     lotBatch,
		}
	}
	if err := s.st.insertSheets(ctx, sheets); err != nil {
		return InventoryLot{}, err
	}

	return lot, nil
}

func (s *service) ListLots(ctx context.Context, p httpkit.PageParams) (httpkit.PagedResult[InventoryLot], error) {
	items, total, err := s.st.selectLotsPaged(ctx, p)
	if err != nil {
		return httpkit.PagedResult[InventoryLot]{}, err
	}
	return httpkit.NewPagedResult(items, total, p), nil
}

func (s *service) DeactivateLot(ctx context.Context, lotID uuid.UUID) error {
	return s.st.deactivateLot(ctx, lotID)
}

func (s *service) GetSheet(ctx context.Context, sheetID uuid.UUID) (BoardSheet, error) {
	return s.st.selectSheetByID(ctx, sheetID)
}

func (s *service) PreAssignSheet(ctx context.Context, sheetID uuid.UUID, workOrderID uuid.UUID) error {
	return s.st.preAssignSheet(ctx, sheetID, workOrderID)
}

func (s *service) ListAvailableSheets(ctx context.Context, p httpkit.PageParams, materialID *uuid.UUID) (httpkit.PagedResult[BoardSheet], error) {
	items, total, err := s.st.selectAvailableSheetsPaged(ctx, p, materialID)
	if err != nil {
		return httpkit.PagedResult[BoardSheet]{}, err
	}
	return httpkit.NewPagedResult(items, total, p), nil
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

	// Material attributes inherited by any new remnant produced by this cut.
	var inheritSupplierCode *string
	var inheritLotBatch *string
	var inheritGrainPattern *string
	var inheritQualityGrade *string

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
		inheritSupplierCode = sheet.SupplierCode
		inheritLotBatch = sheet.LotBatch
		inheritGrainPattern = sheet.GrainPattern
		inheritQualityGrade = sheet.QualityGrade
	} else {
		remnant, err := s.st.selectRemnantByID(ctx, *in.RemnantID)
		if err != nil {
			return CutResult{}, err
		}
		if remnant.Status != domain.RemnantAvailable && remnant.Status != domain.RemnantAllocated {
			return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "remnant is not available for cutting")
		}
		sourceDim = remnant.Dimensions
		parentBoardID = remnant.ParentBoardID
		parentRemnantID = &remnant.ID
		inheritSupplierCode = remnant.SupplierCode
		inheritLotBatch = remnant.LotBatch
		inheritGrainPattern = remnant.GrainPattern
		inheritQualityGrade = remnant.QualityGrade
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
		// Validate bounding_box does not exceed the actual remnant dimension.
		// Both axes must be provided together; partial specification is rejected.
		if (in.BoundingBoxLengthMM == nil) != (in.BoundingBoxWidthMM == nil) {
			return CutResult{}, domain.NewBizError(domain.ErrInvalidInput,
				"bounding_box_length_mm and bounding_box_width_mm must be provided together")
		}
		if in.BoundingBoxLengthMM != nil && in.BoundingBoxWidthMM != nil {
			if *in.BoundingBoxLengthMM > in.RemnantDimension.LengthMM ||
				*in.BoundingBoxWidthMM > in.RemnantDimension.WidthMM {
				return CutResult{}, domain.NewBizError(domain.ErrInvalidInput,
					"usable dimension cannot exceed actual dimension")
			}
		}

		// Default bounding_box to the actual remnant dimension when not provided.
		// This guarantees that FindAvailableRemnants can always filter on bounding_box
		// without needing to fall back to length_mm / width_mm at query time.
		bbLen := in.BoundingBoxLengthMM
		bbWid := in.BoundingBoxWidthMM
		if bbLen == nil {
			v := in.RemnantDimension.LengthMM
			bbLen = &v
		}
		if bbWid == nil {
			v := in.RemnantDimension.WidthMM
			bbWid = &v
		}

		newRemnant := Remnant{
			ID:                  uuid.New(),
			ParentBoardID:       parentBoardID,
			ParentRemnantID:     parentRemnantID,
			Dimensions:          *in.RemnantDimension,
			Status:              domain.RemnantAvailable,
			SupplierCode:        inheritSupplierCode,
			LotBatch:            inheritLotBatch,
			GrainPattern:        inheritGrainPattern,
			QualityGrade:        inheritQualityGrade,
			BoundingBoxLengthMM: bbLen,
			BoundingBoxWidthMM:  bbWid,
			CreatedAt:           time.Now().UTC(),
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

	if s.bcg != nil {
		bcOut, err := s.bcg.GenerateForCut(ctx, BarcodeForCutInput{
			WorkOrderID:      in.WorkOrderID,
			UsedDimension:    in.UsedDimension,
			RemnantDimension: in.RemnantDimension,
			ProducedDate:     cr.CreatedAt,
		})
		if bcOut.WIPBarcodeID != nil {
			result.BarcodeIDs = append(result.BarcodeIDs, *bcOut.WIPBarcodeID)
		}
		if bcOut.RemnantBarcodeID != nil {
			result.BarcodeIDs = append(result.BarcodeIDs, *bcOut.RemnantBarcodeID)
		}
		if err != nil {
			slog.Warn("inventory: RecordCut barcode generation failed",
				"work_order_id", in.WorkOrderID, "err", err)
		}
	}

	// After a successful cut, auto-advance the work order from IN_CUTTING to
	// IN_PROCESSING. If the transition is not valid (e.g. the work order has
	// already been advanced by another path), silently log and continue.
	if s.woa != nil {
		if err := s.woa.AdvanceStatus(ctx, in.WorkOrderID, AdvanceWOInput{To: domain.WOInProcessing}); err != nil {
			if !errors.Is(err, domain.ErrInvalidTransition) {
				slog.Warn("inventory: RecordCut auto-advance failed",
					"work_order_id", in.WorkOrderID, "err", err)
			}
		}
	}

	return result, nil
}

func (s *service) FindAvailableRemnants(ctx context.Context, minDim domain.Dimension) ([]Remnant, error) {
	return s.st.selectAvailableRemnantsByMinDimension(ctx, minDim)
}

const (
	defaultSuggestionLimit = 3
	maxSuggestionLimit     = 10
)

func (s *service) SuggestRemnants(ctx context.Context, in SuggestRemnantsInput) ([]RemnantSuggestion, error) {
	if !in.RequiredDimension.Valid() {
		return nil, domain.NewBizError(domain.ErrInvalidInput, "required dimension must have positive length and width")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultSuggestionLimit
	}
	if limit > maxSuggestionLimit {
		limit = maxSuggestionLimit
	}
	return s.st.selectTopRemnantSuggestions(ctx, in.RequiredDimension, limit)
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

func (s *service) ListRemnants(ctx context.Context, f RemnantFilter, p httpkit.PageParams) (httpkit.PagedResult[Remnant], error) {
	if f.Status == "" {
		f.Status = domain.RemnantAvailable
	}
	items, total, err := s.st.selectRemnantsByFilter(ctx, f, p)
	if err != nil {
		return httpkit.PagedResult[Remnant]{}, err
	}
	return httpkit.NewPagedResult(items, total, p), nil
}

func (s *service) GetRemnantLineageByRemnant(ctx context.Context, remnantID uuid.UUID) ([]Remnant, error) {
	remnant, err := s.st.selectRemnantByID(ctx, remnantID)
	if err != nil {
		return nil, err
	}
	return s.st.selectRemnantsByBoardSheet(ctx, remnant.ParentBoardID)
}

func (s *service) ListStorageLocations(ctx context.Context) ([]StorageLocation, error) {
	return s.st.selectActiveStorageLocations(ctx)
}

func (s *service) GetRemnant(ctx context.Context, remnantID uuid.UUID) (Remnant, error) {
	return s.st.selectRemnantByID(ctx, remnantID)
}

func (s *service) StockRemnant(ctx context.Context, remnantID uuid.UUID, locationBarcode string) error {
	loc, err := s.st.selectStorageLocationByBarcode(ctx, locationBarcode)
	if err != nil {
		return err
	}
	return s.st.updateRemnantBinLocation(ctx, remnantID, loc.ID)
}

func (s *service) ReleaseExpiredAllocations(ctx context.Context, before time.Time) (int, error) {
	n, err := s.st.releaseExpiredAllocations(ctx, before)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
