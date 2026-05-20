package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

const defaultOverflowThresholdPct = 15.0

type service struct {
	st                   store
	woa                  WorkOrderAdvancer
	bcg                  BarcodeGenerator
	notifier             CutNotifier
	overflowThresholdPct float64
}

func NewService(st store, woa WorkOrderAdvancer, bcg ...BarcodeGenerator) Service {
	var generator BarcodeGenerator
	if len(bcg) > 0 {
		generator = bcg[0]
	}
	return &service{st: st, woa: woa, bcg: generator, overflowThresholdPct: defaultOverflowThresholdPct}
}

func NewServiceWithOverflowThreshold(st store, woa WorkOrderAdvancer, thresholdPct float64, bcg ...BarcodeGenerator) Service {
	var generator BarcodeGenerator
	if len(bcg) > 0 {
		generator = bcg[0]
	}
	if thresholdPct <= 0 || thresholdPct > 100 {
		thresholdPct = defaultOverflowThresholdPct
	}
	return &service{st: st, woa: woa, bcg: generator, overflowThresholdPct: thresholdPct}
}

// NewServiceFull wires the optional cut notifier in addition to the existing
// dependencies. Kept as a separate constructor so existing callers (and tests)
// that don't need notifications continue to compile unchanged.
func NewServiceFull(st store, woa WorkOrderAdvancer, bcg BarcodeGenerator, notifier CutNotifier, thresholdPct float64) Service {
	if thresholdPct <= 0 || thresholdPct > 100 {
		thresholdPct = defaultOverflowThresholdPct
	}
	return &service{st: st, woa: woa, bcg: bcg, notifier: notifier, overflowThresholdPct: thresholdPct}
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

func (s *service) PreAssignSheet(ctx context.Context, in PreAssignSheetInput) error {
	overflow, err := s.GetOverflowStatus(ctx)
	if err != nil {
		return err
	}
	if overflow.BlockNewSheetIssue {
		if !in.BypassOverflow {
			return domain.NewBizError(domain.ErrPreconditionFailed,
				"remnant overflow: must consume remnants before issuing new sheets")
		}
		if in.ActorID == uuid.Nil || in.Reason == "" {
			return domain.NewBizError(domain.ErrInvalidInput,
				"overflow bypass requires actor and reason")
		}
		reason := in.Reason
		if err := s.st.insertAuditLog(ctx, AuditLogEntry{
			ID:         uuid.New(),
			EntityType: entityTypeBoardSheet,
			EntityID:   in.SheetID,
			Action:     auditActionOverflowBypassed,
			ActorID:    in.ActorID,
			Reason:     &reason,
			CreatedAt:  time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("write overflow bypass audit log: %w", err)
		}
	}
	return s.st.preAssignSheet(ctx, in.SheetID, in.WorkOrderID)
}

func (s *service) ListAvailableSheets(ctx context.Context, p httpkit.PageParams, materialID *uuid.UUID) (httpkit.PagedResult[BoardSheet], error) {
	items, total, err := s.st.selectAvailableSheetsPaged(ctx, p, materialID)
	if err != nil {
		return httpkit.PagedResult[BoardSheet]{}, err
	}
	return httpkit.NewPagedResult(items, total, p), nil
}

// CountAvailableSheetsByMaterial returns how many AVAILABLE board sheets back
// the given material. Used by the production module to enforce BR-K01.
func (s *service) CountAvailableSheetsByMaterial(ctx context.Context, materialID uuid.UUID) (int, error) {
	return s.st.countAvailableSheetsByMaterial(ctx, materialID)
}

func (s *service) GetOverflowStatus(ctx context.Context) (OverflowStatus, error) {
	totalRemnantAreaMM2, totalSheetAreaMM2, err := s.st.selectOverflowAreas(ctx)
	if err != nil {
		return OverflowStatus{}, err
	}

	overflowPct := 0.0
	if totalSheetAreaMM2 > 0 {
		overflowPct = float64(totalRemnantAreaMM2) * 100 / float64(totalSheetAreaMM2)
	} else if totalRemnantAreaMM2 > 0 {
		overflowPct = 100
	}

	status := OverflowGreen
	if overflowPct > s.overflowThresholdPct {
		status = OverflowRed
	}

	return OverflowStatus{
		Status:              status,
		OverflowPct:         overflowPct,
		ThresholdPct:        s.overflowThresholdPct,
		BlockNewSheetIssue:  status == OverflowRed,
		TotalRemnantAreaMM2: totalRemnantAreaMM2,
		TotalSheetAreaMM2:   totalSheetAreaMM2,
	}, nil
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
	// BR-K02: every cut must declare its leftover outcome explicitly — either
	// the remnant dimensions are supplied OR is_waste=true. Empty + false is
	// ambiguous (looks like a missing field) and is rejected so implicit waste
	// cannot slip through. Both being set together is also rejected because
	// the caller's intent is contradictory.
	hasRemnant := in.RemnantDimension != nil
	switch {
	case hasRemnant && in.IsWaste:
		return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "remnant_dimension and is_waste cannot both be set")
	case !hasRemnant && !in.IsWaste:
		return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "must provide remnant_dimension or set is_waste=true")
	}
	if in.ShapeType == "" {
		in.ShapeType = "rectangle"
	}
	if in.ShapeType != "rectangle" && in.ShapeType != "irregular" {
		return CutResult{}, domain.NewBizError(domain.ErrInvalidInput, "shape_type must be 'rectangle' or 'irregular'")
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
		overflow, err := s.GetOverflowStatus(ctx)
		if err != nil {
			return CutResult{}, err
		}
		if overflow.BlockNewSheetIssue {
			return CutResult{}, domain.NewBizError(domain.ErrPreconditionFailed,
				"cannot issue new sheet while overflow status is RED")
		}

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
			ShapeType:           in.ShapeType,
			SupplierCode:        inheritSupplierCode,
			LotBatch:            inheritLotBatch,
			GrainPattern:        inheritGrainPattern,
			QualityGrade:        inheritQualityGrade,
			BoundingBoxLengthMM: bbLen,
			BoundingBoxWidthMM:  bbWid,
			CreatedAt:           time.Now().UTC(),
		}
		op.NewRemnant = &newRemnant
		// Link the cutting record to the produced remnant so the kiosk can
		// later regenerate combined WIP + remnant labels without time-proximity
		// joins.
		op.Record.ProducedRemnantID = &newRemnant.ID
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

	// Best-effort SSE notification — log + continue if the broker is down so a
	// transient failure never rolls back the persisted cut.
	if s.notifier != nil {
		if err := s.notifier.NotifyCuttingRecorded(ctx, in.WorkOrderID.String(), cr.ID.String()); err != nil {
			slog.Warn("inventory: notify cutting recorded failed",
				"work_order_id", in.WorkOrderID, "cutting_record_id", cr.ID, "err", err)
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

func (s *service) ListCuttingRecords(ctx context.Context, f CuttingRecordFilter, p httpkit.PageParams) (httpkit.PagedResult[CuttingRecordReport], error) {
	if !f.From.IsZero() && !f.To.IsZero() && f.To.Before(f.From) {
		return httpkit.PagedResult[CuttingRecordReport]{}, domain.NewBizError(domain.ErrInvalidInput, "to must be greater than or equal to from")
	}
	items, total, err := s.st.selectCuttingRecordsReport(ctx, f, p)
	if err != nil {
		return httpkit.PagedResult[CuttingRecordReport]{}, err
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

const (
	entityTypeRemnant    = "REMNANT"
	entityTypeBoardSheet = "BOARD_SHEET"
	entityTypeWorkOrder  = "WORK_ORDER"
	auditActionTransfer  = "TRANSFER"
	auditActionAdjust    = "ADJUSTMENT"
	// auditActionOverflowBypassed is written when an authorised caller pre-assigns
	// a board sheet to a work order while remnant overflow is RED.
	auditActionOverflowBypassed = "OVERFLOW_BYPASSED"
	// auditActionRemnantBypassed is written when CreateWorkOrder fires with at
	// least one matching remnant suggestion but the planner did not allocate
	// any remnant to the work order (BR-K05).
	auditActionRemnantBypassed = "REMNANT_BYPASSED"
)

func (s *service) Transfer(ctx context.Context, in TransferInput) (TransferResult, error) {
	if in.EntityType != entityTypeRemnant && in.EntityType != entityTypeBoardSheet {
		return TransferResult{}, domain.NewBizError(domain.ErrInvalidInput,
			"entity_type must be REMNANT or BOARD_SHEET")
	}

	targetLoc, err := s.st.selectStorageLocationByBarcode(ctx, in.TargetBarcode)
	if err != nil {
		return TransferResult{}, err
	}
	if !targetLoc.IsActive {
		return TransferResult{}, domain.NewBizError(domain.ErrInvalidInput, "target location is not active")
	}

	var fromLocationID *uuid.UUID
	switch in.EntityType {
	case entityTypeRemnant:
		remnant, err := s.st.selectRemnantByID(ctx, in.EntityID)
		if err != nil {
			return TransferResult{}, err
		}
		if remnant.Status != domain.RemnantAvailable && remnant.Status != domain.RemnantAllocated {
			return TransferResult{}, domain.NewBizError(domain.ErrInvalidInput,
				"remnant must be AVAILABLE or ALLOCATED to transfer")
		}
		fromLocationID = remnant.BinLocationID
		if err := s.st.updateRemnantBinLocation(ctx, in.EntityID, targetLoc.ID); err != nil {
			return TransferResult{}, err
		}
	case entityTypeBoardSheet:
		sheet, err := s.st.selectSheetByID(ctx, in.EntityID)
		if err != nil {
			return TransferResult{}, err
		}
		if sheet.Status != "AVAILABLE" {
			return TransferResult{}, domain.NewBizError(domain.ErrInvalidInput,
				"board sheet must be AVAILABLE to transfer")
		}
		fromLocationID = sheet.BinLocationID
		if err := s.st.updateSheetBinLocation(ctx, in.EntityID, targetLoc.ID); err != nil {
			return TransferResult{}, err
		}
	}

	entry := AuditLogEntry{
		ID:           uuid.New(),
		EntityType:   in.EntityType,
		EntityID:     in.EntityID,
		Action:       auditActionTransfer,
		ActorID:      in.ActorID,
		FromLocation: fromLocationID,
		ToLocation:   &targetLoc.ID,
		CreatedAt:    now(),
	}
	if err := s.st.insertAuditLog(ctx, entry); err != nil {
		slog.Warn("inventory: Transfer audit log failed", "entity_id", in.EntityID, "err", err)
	}

	return TransferResult{
		EntityType:   in.EntityType,
		EntityID:     in.EntityID,
		FromLocation: fromLocationID,
		ToLocation:   targetLoc.ID,
		AuditLogID:   entry.ID,
	}, nil
}

func (s *service) ListAuditLog(ctx context.Context, entityID uuid.UUID, entityType string, params httpkit.CursorParams) (httpkit.CursorResult[AuditLogEntry], error) {
	if entityType != entityTypeRemnant && entityType != entityTypeBoardSheet && entityType != entityTypeWorkOrder {
		return httpkit.CursorResult[AuditLogEntry]{}, domain.NewBizError(domain.ErrInvalidInput,
			"entity_type must be REMNANT, BOARD_SHEET, or WORK_ORDER")
	}
	cur, err := params.Decoded()
	if err != nil {
		return httpkit.CursorResult[AuditLogEntry]{}, err
	}
	rows, err := s.st.selectAuditLogByEntityKeyset(ctx, entityID, entityType, cur, params.Limit+1)
	if err != nil {
		return httpkit.CursorResult[AuditLogEntry]{}, err
	}
	return httpkit.NewCursorResult(rows, params.Limit, func(e AuditLogEntry) httpkit.Cursor {
		return httpkit.Cursor{Ts: e.CreatedAt, ID: e.ID}
	}), nil
}

func (s *service) ListAuditLogByAction(ctx context.Context, action string, params httpkit.CursorParams) (httpkit.CursorResult[AuditLogEntry], error) {
	if action == "" {
		return httpkit.CursorResult[AuditLogEntry]{}, domain.NewBizError(domain.ErrInvalidInput, "action is required")
	}
	cur, err := params.Decoded()
	if err != nil {
		return httpkit.CursorResult[AuditLogEntry]{}, err
	}
	rows, err := s.st.selectAuditLogByActionKeyset(ctx, action, cur, params.Limit+1)
	if err != nil {
		return httpkit.CursorResult[AuditLogEntry]{}, err
	}
	return httpkit.NewCursorResult(rows, params.Limit, func(e AuditLogEntry) httpkit.Cursor {
		return httpkit.Cursor{Ts: e.CreatedAt, ID: e.ID}
	}), nil
}

func (s *service) LogRemnantBypass(ctx context.Context, in LogRemnantBypassInput) error {
	if in.WorkOrderID == uuid.Nil {
		return domain.NewBizError(domain.ErrInvalidInput, "work_order_id is required")
	}
	if in.ActorID == uuid.Nil {
		return domain.NewBizError(domain.ErrInvalidInput, "actor_id is required")
	}
	if len(in.SuggestedRemnantIDs) == 0 {
		return domain.NewBizError(domain.ErrInvalidInput, "suggested_remnant_ids must contain at least one id")
	}
	metaBytes, err := json.Marshal(map[string]any{
		"suggested_remnant_ids": in.SuggestedRemnantIDs,
	})
	if err != nil {
		return fmt.Errorf("encode remnant bypass metadata: %w", err)
	}
	entry := AuditLogEntry{
		ID:         uuid.New(),
		EntityType: entityTypeWorkOrder,
		EntityID:   in.WorkOrderID,
		Action:     auditActionRemnantBypassed,
		ActorID:    in.ActorID,
		Metadata:   metaBytes,
		CreatedAt:  time.Now().UTC(),
	}
	if in.Reason != "" {
		reason := in.Reason
		entry.Reason = &reason
	}
	if err := s.st.insertAuditLog(ctx, entry); err != nil {
		return fmt.Errorf("write remnant bypass audit log: %w", err)
	}
	return nil
}

func (s *service) CreateCycleCountSession(ctx context.Context, in CreateCycleCountInput) (CycleCountSession, error) {
	sess := CycleCountSession{
		ID:        uuid.New(),
		Zone:      in.Zone,
		Status:    "OPEN",
		CreatedBy: in.ActorID,
		CreatedAt: now(),
	}
	if err := s.st.insertCycleCountSession(ctx, sess); err != nil {
		return CycleCountSession{}, err
	}
	return sess, nil
}

func (s *service) GetCycleCountSession(ctx context.Context, sessionID uuid.UUID) (CycleCountSession, error) {
	return s.st.selectCycleCountSessionByID(ctx, sessionID)
}

func (s *service) AddCycleCountLine(ctx context.Context, in AddCountLineInput) (CycleCountLine, error) {
	sess, err := s.st.selectCycleCountSessionByID(ctx, in.SessionID)
	if err != nil {
		return CycleCountLine{}, err
	}
	if sess.Status != "OPEN" {
		return CycleCountLine{}, domain.NewBizError(domain.ErrInvalidTransition,
			"cycle count session is not OPEN")
	}

	if in.EntityType != entityTypeRemnant && in.EntityType != entityTypeBoardSheet {
		return CycleCountLine{}, domain.NewBizError(domain.ErrInvalidInput,
			"entity_type must be REMNANT or BOARD_SHEET")
	}
	if in.Reason == "" {
		return CycleCountLine{}, domain.NewBizError(domain.ErrInvalidInput, "reason is required")
	}
	if len([]rune(in.Reason)) > 255 {
		return CycleCountLine{}, domain.NewBizError(domain.ErrInvalidInput, "reason exceeds max length 255")
	}

	switch in.EntityType {
	case entityTypeRemnant:
		validRemnantStatuses := map[string]bool{
			string(domain.RemnantAvailable): true,
			string(domain.RemnantAllocated): true,
			string(domain.RemnantConsumed):  true,
			string(domain.RemnantWaste):     true,
		}
		if !validRemnantStatuses[in.CountedStatus] {
			return CycleCountLine{}, domain.NewBizError(domain.ErrInvalidInput,
				"counted_status is not a valid remnant status")
		}
		if _, err := s.st.selectRemnantByID(ctx, in.EntityID); err != nil {
			return CycleCountLine{}, err
		}
	case entityTypeBoardSheet:
		if in.CountedStatus != "AVAILABLE" && in.CountedStatus != "ISSUED" {
			return CycleCountLine{}, domain.NewBizError(domain.ErrInvalidInput,
				"counted_status for BOARD_SHEET must be AVAILABLE or ISSUED")
		}
		if _, err := s.st.selectSheetByID(ctx, in.EntityID); err != nil {
			return CycleCountLine{}, err
		}
	}

	line := CycleCountLine{
		ID:                uuid.New(),
		SessionID:         in.SessionID,
		EntityType:        in.EntityType,
		EntityID:          in.EntityID,
		CountedStatus:     in.CountedStatus,
		CountedLocationID: in.CountedLocationID,
		Reason:            in.Reason,
		CreatedAt:         now(),
	}
	if err := s.st.insertCycleCountLine(ctx, line); err != nil {
		return CycleCountLine{}, err
	}
	return line, nil
}

func (s *service) ListCycleCountLines(ctx context.Context, sessionID uuid.UUID) ([]CycleCountLine, error) {
	return s.st.selectCycleCountLinesBySession(ctx, sessionID)
}

func (s *service) PostCycleCount(ctx context.Context, in PostCycleCountInput) error {
	sess, err := s.st.selectCycleCountSessionByID(ctx, in.SessionID)
	if err != nil {
		return err
	}
	if sess.Status != "OPEN" {
		return domain.NewBizError(domain.ErrInvalidTransition, "cycle count session is not OPEN")
	}

	lines, err := s.st.selectCycleCountLinesBySession(ctx, in.SessionID)
	if err != nil {
		return err
	}

	var adjustments []cycleCountAdjustment
	for _, line := range lines {
		var currentStatus string
		var currentLocID *uuid.UUID

		if line.EntityType == entityTypeRemnant {
			remnant, err := s.st.selectRemnantByID(ctx, line.EntityID)
			if err != nil {
				return fmt.Errorf("load remnant %s: %w", line.EntityID, err)
			}
			currentStatus = string(remnant.Status)
			currentLocID = remnant.BinLocationID
		} else {
			sheet, err := s.st.selectSheetByID(ctx, line.EntityID)
			if err != nil {
				return fmt.Errorf("load sheet %s: %w", line.EntityID, err)
			}
			currentStatus = sheet.Status
			currentLocID = sheet.BinLocationID
		}

		statusChanged := currentStatus != line.CountedStatus
		locChanged := !uuidPtrEqual(currentLocID, line.CountedLocationID)

		if !statusChanged && !locChanged {
			continue
		}

		reason := line.Reason
		entry := AuditLogEntry{
			ID:           uuid.New(),
			EntityType:   line.EntityType,
			EntityID:     line.EntityID,
			Action:       auditActionAdjust,
			ActorID:      in.ActorID,
			FromLocation: currentLocID,
			ToLocation:   line.CountedLocationID,
			FromStatus:   &currentStatus,
			ToStatus:     &line.CountedStatus,
			Reason:       &reason,
			SessionID:    &in.SessionID,
			CreatedAt:    now(),
		}

		adjustments = append(adjustments, cycleCountAdjustment{
			EntityType:    line.EntityType,
			EntityID:      line.EntityID,
			OldLocationID: currentLocID,
			NewLocationID: line.CountedLocationID,
			OldStatus:     currentStatus,
			NewStatus:     line.CountedStatus,
			AuditEntry:    entry,
		})
	}

	return s.st.postCycleCountAtomically(ctx, cycleCountPostOp{
		SessionID:   in.SessionID,
		PostedBy:    in.ActorID,
		Adjustments: adjustments,
	})
}

func (s *service) CancelCycleCountSession(ctx context.Context, sessionID uuid.UUID, actorID uuid.UUID) error {
	sess, err := s.st.selectCycleCountSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.Status != "OPEN" {
		return domain.NewBizError(domain.ErrInvalidTransition, "cycle count session is not OPEN")
	}
	return s.st.updateCycleCountSessionStatus(ctx, sessionID, "CANCELLED", nil)
}

func uuidPtrEqual(a, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func now() time.Time {
	return time.Now().UTC()
}

// ── Remnant label PDF ─────────────────────────────────────────────────────────

// remnantLabelLayout mirrors barcode.labelLayout but is private to this module.
type remnantLabelLayout struct {
	pageWidthMM  float64
	pageHeightMM float64
	qrXMM        float64
	qrYMM        float64
	qrSizeMM     float64
	textXMM      float64
	textYMM      float64
	textWidthMM  float64
	lineHeightMM float64
	titleFontPt  float64
	bodyFontPt   float64
}

func resolveRemnantLabelLayout(size RemnantLabelSize) (remnantLabelLayout, error) {
	switch size {
	case RemnantLabelSize50x30:
		return remnantLabelLayout{
			pageWidthMM:  50,
			pageHeightMM: 30,
			qrXMM:        2,
			qrYMM:        4,
			qrSizeMM:     20,
			textXMM:      24,
			textYMM:      5,
			textWidthMM:  24,
			lineHeightMM: 4,
			titleFontPt:  7,
			bodyFontPt:   6,
		}, nil
	case RemnantLabelSize100x70:
		return remnantLabelLayout{
			pageWidthMM:  100,
			pageHeightMM: 70,
			qrXMM:        5,
			qrYMM:        8,
			qrSizeMM:     34,
			textXMM:      42,
			textYMM:      10,
			textWidthMM:  54,
			lineHeightMM: 8,
			titleFontPt:  12,
			bodyFontPt:   10,
		}, nil
	default:
		return remnantLabelLayout{}, domain.NewBizError(domain.ErrInvalidInput, "size must be one of: 50x30, 100x70")
	}
}

// remnantQRPayload is the JSON content encoded into the remnant label QR code.
type remnantQRPayload struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	ParentBoardID string `json:"parent_board_id"`
}

func buildRemnantQRBytes(r Remnant) ([]byte, error) {
	payload := remnantQRPayload{
		Type:          "remnant",
		ID:            r.ID.String(),
		ParentBoardID: r.ParentBoardID.String(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal remnant qr payload: %w", err)
	}
	return raw, nil
}

func shortID(id uuid.UUID) string {
	s := id.String()
	return s[:8]
}

func (s *service) GenerateRemnantLabelPDF(ctx context.Context, in RemnantLabelInput) ([]byte, error) {
	layout, err := resolveRemnantLabelLayout(in.Size)
	if err != nil {
		return nil, err
	}

	r, err := s.st.selectRemnantByID(ctx, in.RemnantID)
	if err != nil {
		return nil, err
	}

	pdf := newLabelPDF(layout)
	if err := drawRemnantLabelPage(pdf, layout, r); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, fmt.Errorf("render remnant label pdf: %w", err)
	}
	return out.Bytes(), nil
}

// newLabelPDF constructs a gofpdf instance configured with zero margins and
// the layout's page size, ready for AddPageFormat calls. Used by both
// GenerateRemnantLabelPDF and GenerateCutLabelsPDF so they share rendering
// settings.
func newLabelPDF(layout remnantLabelLayout) *gofpdf.Fpdf {
	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: layout.pageWidthMM, Ht: layout.pageHeightMM},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	return pdf
}

// drawRemnantLabelPage adds a remnant label page to pdf — same layout used
// by GenerateRemnantLabelPDF.
func drawRemnantLabelPage(pdf *gofpdf.Fpdf, layout remnantLabelLayout, r Remnant) error {
	raw, err := buildRemnantQRBytes(r)
	if err != nil {
		return err
	}
	qrPNG, err := qrcode.Encode(string(raw), qrcode.High, 1024)
	if err != nil {
		return fmt.Errorf("encode remnant qr: %w", err)
	}

	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: layout.pageWidthMM, Ht: layout.pageHeightMM})

	imgName := "qr-rem-" + r.ID.String()
	imgOpts := gofpdf.ImageOptions{ImageType: "PNG"}
	pdf.RegisterImageOptionsReader(imgName, imgOpts, bytes.NewReader(qrPNG))
	pdf.ImageOptions(imgName, layout.qrXMM, layout.qrYMM, layout.qrSizeMM, layout.qrSizeMM, false, imgOpts, 0, "")

	dimText := fmt.Sprintf("%dx%d mm", r.Dimensions.LengthMM, r.Dimensions.WidthMM)

	pdf.SetFont("Arial", "B", layout.titleFontPt)
	pdf.SetXY(layout.textXMM, layout.textYMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, "REM: "+shortID(r.ID), "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", layout.bodyFontPt)
	pdf.SetX(layout.textXMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, "Board: "+shortID(r.ParentBoardID), "", 1, "L", false, 0, "")
	pdf.SetX(layout.textXMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, dimText, "", 1, "L", false, 0, "")
	return nil
}

// ── Cut labels PDF (WIP + optional remnant, single document) ─────────────────

// wipQRPayload is encoded into the WIP label QR code. Scanning the QR
// returns the originating cutting_record_id so kiosk operators can re-print
// or look up the cut later.
type wipQRPayload struct {
	Type            string `json:"type"`
	CuttingRecordID string `json:"cutting_record_id"`
	WorkOrderID     string `json:"work_order_id"`
	SKUID           string `json:"sku_id"`
}

func buildWIPQRBytes(d CuttingRecordDetails) ([]byte, error) {
	payload := wipQRPayload{
		Type:            "wip",
		CuttingRecordID: d.Record.ID.String(),
		WorkOrderID:     d.Record.WorkOrderID.String(),
		SKUID:           d.Record.SKUID.String(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal wip qr payload: %w", err)
	}
	return raw, nil
}

// drawWIPLabelPage adds a WIP label page to pdf. The label shows SKU code,
// used dimensions, work-order short id, and a QR encoding the cutting_record_id.
func drawWIPLabelPage(pdf *gofpdf.Fpdf, layout remnantLabelLayout, d CuttingRecordDetails) error {
	raw, err := buildWIPQRBytes(d)
	if err != nil {
		return err
	}
	qrPNG, err := qrcode.Encode(string(raw), qrcode.High, 1024)
	if err != nil {
		return fmt.Errorf("encode wip qr: %w", err)
	}

	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: layout.pageWidthMM, Ht: layout.pageHeightMM})

	imgName := "qr-wip-" + d.Record.ID.String()
	imgOpts := gofpdf.ImageOptions{ImageType: "PNG"}
	pdf.RegisterImageOptionsReader(imgName, imgOpts, bytes.NewReader(qrPNG))
	pdf.ImageOptions(imgName, layout.qrXMM, layout.qrYMM, layout.qrSizeMM, layout.qrSizeMM, false, imgOpts, 0, "")

	skuLabel := d.SKUCode
	if skuLabel == "" {
		skuLabel = "SKU " + shortID(d.Record.SKUID)
	}
	dimText := fmt.Sprintf("%dx%d mm", d.Record.UsedLengthMM, d.Record.UsedWidthMM)

	pdf.SetFont("Arial", "B", layout.titleFontPt)
	pdf.SetXY(layout.textXMM, layout.textYMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, "WIP: "+skuLabel, "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", layout.bodyFontPt)
	pdf.SetX(layout.textXMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, "WO: "+shortID(d.Record.WorkOrderID), "", 1, "L", false, 0, "")
	pdf.SetX(layout.textXMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, dimText, "", 1, "L", false, 0, "")
	return nil
}

func (s *service) GenerateCutLabelsPDF(ctx context.Context, in CutLabelsInput) ([]byte, error) {
	layout, err := resolveRemnantLabelLayout(in.Size)
	if err != nil {
		return nil, err
	}

	d, err := s.st.selectCuttingRecordDetails(ctx, in.CuttingRecordID)
	if err != nil {
		return nil, err
	}

	pdf := newLabelPDF(layout)
	if err := drawWIPLabelPage(pdf, layout, d); err != nil {
		return nil, err
	}
	if d.ProducedRemnant != nil {
		if err := drawRemnantLabelPage(pdf, layout, *d.ProducedRemnant); err != nil {
			return nil, err
		}
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, fmt.Errorf("render cut labels pdf: %w", err)
	}
	return out.Bytes(), nil
}

// ── Pick-slip PDF ─────────────────────────────────────────────────────────────

// pickSlipPageWidthMM / pickSlipPageHeightMM are standard A4 dimensions.
const (
	pickSlipPageWidthMM  = 210.0
	pickSlipPageHeightMM = 297.0
	pickSlipMarginMM     = 15.0
)

func (s *service) GeneratePickSlipPDF(ctx context.Context, workOrderID uuid.UUID) ([]byte, error) {
	lines, err := s.st.selectAllocatedRemnantsByWO(ctx, workOrderID)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, domain.NewBizError(domain.ErrNotFound, "no allocated remnants found for this work order")
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pickSlipMarginMM, pickSlipMarginMM, pickSlipMarginMM)
	pdf.SetAutoPageBreak(true, pickSlipMarginMM)
	pdf.AddPage()

	contentW := pickSlipPageWidthMM - 2*pickSlipMarginMM

	// Header
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(contentW, 8, "PICK SLIP", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.CellFormat(contentW, 5, "Work Order: "+workOrderID.String(), "", 1, "C", false, 0, "")
	pdf.Ln(4)

	// Column widths: Zone | Label | Remnant ID | Dimensions | Bin Barcode
	colZone := 20.0
	colLabel := 30.0
	colID := 55.0
	colDim := 35.0
	colBarcode := contentW - colZone - colLabel - colID - colDim

	// Table header
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(220, 220, 220)
	pdf.CellFormat(colZone, 7, "Zone", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colLabel, 7, "Bin Label", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colID, 7, "Remnant ID", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colDim, 7, "Dimensions", "1", 0, "C", true, 0, "")
	pdf.CellFormat(colBarcode, 7, "Bin Barcode", "1", 1, "C", true, 0, "")

	// Rows, grouped by zone (already sorted by zone/rack/shelf from the query)
	pdf.SetFont("Arial", "", 9)
	pdf.SetFillColor(255, 255, 255)
	currentZone := ""
	fill := false
	for _, line := range lines {
		zone := line.Zone
		if zone == "" {
			zone = "—"
		}
		label := line.Label
		if label == "" {
			label = "—"
		}
		barcode := line.BinBarcode
		if barcode == "" {
			barcode = "—"
		}
		dimText := fmt.Sprintf("%d × %d mm", line.Dimensions.LengthMM, line.Dimensions.WidthMM)

		// Shade alternate zones for readability.
		if zone != currentZone {
			currentZone = zone
			fill = !fill
		}
		if fill {
			pdf.SetFillColor(245, 245, 245)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}

		pdf.CellFormat(colZone, 6, zone, "1", 0, "C", fill, 0, "")
		pdf.CellFormat(colLabel, 6, label, "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colID, 6, line.RemnantID.String(), "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colDim, 6, dimText, "1", 0, "C", fill, 0, "")
		pdf.CellFormat(colBarcode, 6, barcode, "1", 1, "L", fill, 0, "")
	}

	// Footer: total count
	pdf.Ln(3)
	pdf.SetFont("Arial", "I", 8)
	pdf.CellFormat(contentW, 5, fmt.Sprintf("Total: %d remnant(s)", len(lines)), "", 1, "R", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render pick slip pdf: %w", err)
	}
	return buf.Bytes(), nil
}
