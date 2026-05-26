package costing

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
	st       store
	wor      WorkOrderReader
	cdr      CuttingDataReader
	conr     ConsumptionDataReader
	lbr      LaborDataReader
	notifier CostingNotifier
	audit    AuditLogger
}

func NewService(st store, wor WorkOrderReader, cdr CuttingDataReader, conr ConsumptionDataReader, lbr LaborDataReader) Service {
	return &service{st: st, wor: wor, cdr: cdr, conr: conr, lbr: lbr}
}

// NewServiceWithNotifier wires an optional SSE notifier so ComputeCost can fan
// out COSTING_COMPUTED to accountant + admin dashboards. Notifier failures are
// best-effort (logged, not returned) so the persisted record is never rolled
// back by a transient broker outage.
func NewServiceWithNotifier(st store, wor WorkOrderReader, cdr CuttingDataReader, conr ConsumptionDataReader, lbr LaborDataReader, notifier CostingNotifier) Service {
	return &service{st: st, wor: wor, cdr: cdr, conr: conr, lbr: lbr, notifier: notifier}
}

// NewServiceFull wires the full set of optional cross-module dependencies
// (notifier + audit). Tests construct without these via NewService; production
// uses this constructor in cmd/server/main.go. Each optional dep is guarded
// individually inside the service so a missing audit logger never breaks
// notify, and vice-versa.
func NewServiceFull(st store, wor WorkOrderReader, cdr CuttingDataReader, conr ConsumptionDataReader, lbr LaborDataReader, notifier CostingNotifier, audit AuditLogger) Service {
	return &service{st: st, wor: wor, cdr: cdr, conr: conr, lbr: lbr, notifier: notifier, audit: audit}
}

func (s *service) ComputeCost(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error) {
	wo, err := s.wor.GetWorkOrder(ctx, workOrderID)
	if err != nil {
		return CostingRecord{}, err
	}
	if wo.Status != domain.WOPlanned && wo.Status != domain.WOCompleted && wo.Status != domain.WOPartialComplete {
		return CostingRecord{}, domain.NewBizError(domain.ErrPreconditionFailed, "costing can only be computed for PLANNED (estimated) or COMPLETED/PARTIAL_COMPLETE (actual) work orders")
	}

	costingType := CostingTypeActual
	if wo.Status == domain.WOPlanned {
		costingType = CostingTypeEstimated
	}

	cuttingData, err := s.cdr.GetCuttingDataForWO(ctx, workOrderID)
	if err != nil {
		return CostingRecord{}, err
	}

	var materialCost domain.Money
	for _, cd := range cuttingData {
		if cd.SheetAreaMM2 <= 0 {
			continue
		}
		allocated := cd.SheetCost.Scale(cd.UsedAreaMM2, cd.SheetAreaMM2)
		materialCost = materialCost.Add(allocated)
	}

	auxiliaryCost, err := s.conr.GetConsumptionCostForWO(ctx, workOrderID)
	if err != nil {
		return CostingRecord{}, err
	}

	var laborCost domain.Money
	if s.lbr != nil {
		laborCost, err = s.lbr.GetLaborCostForWO(ctx, workOrderID)
		if err != nil {
			return CostingRecord{}, err
		}
	}
	if laborCost.Currency == "" {
		laborCost = domain.VND(0)
	}
	totalCost := materialCost.Add(auxiliaryCost).Add(laborCost)

	record := CostingRecord{
		WorkOrderID:   workOrderID,
		SKUID:         wo.SKUID,
		CostingType:   costingType,
		MaterialCost:  materialCost,
		AuxiliaryCost: auxiliaryCost,
		LaborCost:     laborCost,
		TotalCost:     totalCost,
		Finalized:     false,
		CreatedAt:     time.Now().UTC(),
	}

	existing, err := s.st.selectCostingRecordByWO(ctx, workOrderID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return CostingRecord{}, err
	}
	if err == nil && existing.Finalized {
		return CostingRecord{}, domain.NewBizError(domain.ErrAlreadyFinalized, "costing record already finalized, create an adjustment instead")
	}

	// Sprint 6 guard: ACTUAL costing for a COMPLETED work order requires at
	// least one non-zero cost component (material, auxiliary, or labor). A
	// zero-total ACTUAL record is almost always a data entry gap (missing
	// consumption or labor). Estimated runs on PLANNED WOs are exempt because
	// upstream data may still be empty by design. Evaluated AFTER the finalized
	// check so that BR-C04 continues to take precedence.
	if costingType == CostingTypeActual && totalCost.Amount == 0 {
		return CostingRecord{}, domain.NewBizError(domain.ErrPreconditionFailed, "WO chưa có chi phí vật tư/nhân công, không thể tính giá thành")
	}

	if err == nil {
		record.ID = existing.ID
		record.CreatedAt = existing.CreatedAt
		if err := s.st.updateCostingRecord(ctx, record); err != nil {
			return CostingRecord{}, err
		}
		s.notifyComputed(ctx, record)
		return record, nil
	}

	record.ID = uuid.New()
	if err := s.st.insertCostingRecord(ctx, record); err != nil {
		return CostingRecord{}, err
	}
	s.notifyComputed(ctx, record)
	return record, nil
}

// notifyComputed broadcasts COSTING_COMPUTED to subscribed accountants/admins.
// Best-effort: a non-nil notifier error is logged and the request still
// succeeds so a transient broker outage never rolls back the persisted record.
func (s *service) notifyComputed(ctx context.Context, record CostingRecord) {
	if s.notifier == nil {
		return
	}
	if err := s.notifier.NotifyCostingComputed(ctx, record.WorkOrderID.String(), string(record.CostingType)); err != nil {
		slog.Warn("costing: notify costing computed failed",
			"work_order_id", record.WorkOrderID, "costing_type", record.CostingType, "err", err)
	}
}

func (s *service) FinalizeCost(ctx context.Context, workOrderID uuid.UUID, actorID uuid.UUID) error {
	return s.st.finalizeCostingRecord(ctx, workOrderID, actorID)
}

func (s *service) CreateAdjustment(ctx context.Context, in CreateAdjustmentInput) (CostingAdjustment, error) {
	if in.Reason == "" {
		return CostingAdjustment{}, domain.NewBizError(domain.ErrInvalidInput, "reason is required for costing adjustment")
	}

	record, err := s.st.selectCostingRecordByWO(ctx, in.WorkOrderID)
	if err != nil {
		return CostingAdjustment{}, err
	}
	if !record.Finalized {
		return CostingAdjustment{}, domain.NewBizError(domain.ErrPreconditionFailed, "costing record must be finalized before creating an adjustment")
	}

	deltaTotal := in.DeltaMaterial.Add(in.DeltaAuxiliary).Add(in.DeltaLabor)
	if deltaTotal.Amount == 0 && in.DeltaMaterial.Amount == 0 && in.DeltaAuxiliary.Amount == 0 && in.DeltaLabor.Amount == 0 {
		return CostingAdjustment{}, domain.NewBizError(domain.ErrInvalidInput, "at least one non-zero delta is required")
	}

	adj := CostingAdjustment{
		ID:              uuid.New(),
		CostingRecordID: record.ID,
		Reason:          in.Reason,
		DeltaMaterial:   in.DeltaMaterial,
		DeltaAuxiliary:  in.DeltaAuxiliary,
		DeltaLabor:      in.DeltaLabor,
		DeltaTotal:      deltaTotal,
		CreatedBy:       in.CreatedBy,
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.st.insertCostingAdjustment(ctx, adj); err != nil {
		return CostingAdjustment{}, err
	}

	// Best-effort audit row (BR-C04 audit trail, #250). Logged on failure but
	// never propagated — the adjustment row is the canonical record; the audit
	// row is supplementary review data for accountants.
	if s.audit != nil {
		if err := s.audit.LogCostingAdjustment(ctx, AuditCostingAdjustmentInput{
			AdjustmentID:    adj.ID,
			CostingRecordID: record.ID,
			WorkOrderID:     record.WorkOrderID,
			ActorID:         in.CreatedBy,
			Reason:          in.Reason,
			DeltaMaterial:   in.DeltaMaterial,
			DeltaAuxiliary:  in.DeltaAuxiliary,
			DeltaLabor:      in.DeltaLabor,
			DeltaTotal:      deltaTotal,
		}); err != nil {
			slog.Warn("audit log for costing adjustment failed",
				"adjustment_id", adj.ID,
				"costing_record_id", record.ID,
				"work_order_id", record.WorkOrderID,
				"err", err)
		}
	}

	return adj, nil
}

func (s *service) ListAdjustments(ctx context.Context, workOrderID uuid.UUID) ([]CostingAdjustment, error) {
	record, err := s.st.selectCostingRecordByWO(ctx, workOrderID)
	if err != nil {
		return nil, err
	}
	adjs, err := s.st.selectAdjustmentsByRecord(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	if adjs == nil {
		adjs = []CostingAdjustment{}
	}
	return adjs, nil
}

func (s *service) GetCostingRecord(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error) {
	return s.st.selectCostingRecordByWO(ctx, workOrderID)
}

// GetCostingRecordDetail loads the record plus its adjustments and folds the
// deltas into running "effective" totals. Effective values are simply the
// per-bucket sums (record + Σ adjustment deltas), so the FE adjustment
// dialog can render them without re-implementing money arithmetic.
//
// The original record numbers stay immutable per BR-C04 — adjustments are
// the only mechanism that moves the effective totals.
func (s *service) GetCostingRecordDetail(ctx context.Context, workOrderID uuid.UUID) (CostingRecordDetail, error) {
	record, err := s.st.selectCostingRecordByWO(ctx, workOrderID)
	if err != nil {
		return CostingRecordDetail{}, err
	}
	adjs, err := s.st.selectAdjustmentsByRecord(ctx, record.ID)
	if err != nil {
		return CostingRecordDetail{}, err
	}
	if adjs == nil {
		adjs = []CostingAdjustment{}
	}

	effMaterial := record.MaterialCost
	effAuxiliary := record.AuxiliaryCost
	effLabor := record.LaborCost
	effTotal := record.TotalCost
	for _, a := range adjs {
		effMaterial = effMaterial.Add(a.DeltaMaterial)
		effAuxiliary = effAuxiliary.Add(a.DeltaAuxiliary)
		effLabor = effLabor.Add(a.DeltaLabor)
		effTotal = effTotal.Add(a.DeltaTotal)
	}

	return CostingRecordDetail{
		Record:             record,
		Adjustments:        adjs,
		EffectiveMaterial:  effMaterial,
		EffectiveAuxiliary: effAuxiliary,
		EffectiveLabor:     effLabor,
		EffectiveTotal:     effTotal,
	}, nil
}

func (s *service) HasCostingRecord(ctx context.Context, workOrderID uuid.UUID) (bool, error) {
	return s.st.hasCostingRecord(ctx, workOrderID)
}

// IsCostingFinalized returns true when a costing record exists for the work
// order and its Finalized flag is set. Missing records are not an error —
// callers (production module) treat that as "not finalized, write allowed".
func (s *service) IsCostingFinalized(ctx context.Context, workOrderID uuid.UUID) (bool, error) {
	record, err := s.st.selectCostingRecordByWO(ctx, workOrderID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return record.Finalized, nil
}

func (s *service) ListCostingRecords(ctx context.Context, params httpkit.CursorParams, finalized *bool) (httpkit.CursorResult[CostingRecord], error) {
	cur, err := params.Decoded()
	if err != nil {
		return httpkit.CursorResult[CostingRecord]{}, err
	}
	items, err := s.st.selectCostingRecordsKeyset(ctx, finalized, cur, params.Limit+1)
	if err != nil {
		return httpkit.CursorResult[CostingRecord]{}, err
	}
	return httpkit.NewCursorResult(items, params.Limit, func(r CostingRecord) httpkit.Cursor {
		return httpkit.Cursor{Ts: r.CreatedAt, ID: r.ID}
	}), nil
}

func (s *service) ListWasteReport(ctx context.Context, filter WasteReportFilter) ([]WasteReportRow, error) {
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return nil, domain.NewBizError(domain.ErrInvalidInput, "from must be before to")
	}
	rows, err := s.st.selectWasteReport(ctx, filter)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []WasteReportRow{}
	}
	return rows, nil
}
