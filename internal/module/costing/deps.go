package costing

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type WorkOrderReader interface {
	GetWorkOrder(ctx context.Context, woID uuid.UUID) (WOInfo, error)
}

type WOInfo struct {
	ID     uuid.UUID
	SKUID  uuid.UUID
	Status domain.WorkOrderStatus
}

type CuttingDataReader interface {
	GetCuttingDataForWO(ctx context.Context, woID uuid.UUID) ([]CuttingData, error)
}

type CuttingData struct {
	SheetCost    domain.Money
	SheetAreaMM2 int64
	UsedAreaMM2  int64
}

type ConsumptionDataReader interface {
	GetConsumptionCostForWO(ctx context.Context, woID uuid.UUID) (domain.Money, error)
}

// LaborDataReader returns the aggregated labor cost recorded against a work
// order, computed as SUM(minutes * rate_per_hour / 60). Implementation lives
// in the production module; wired in main.go.
type LaborDataReader interface {
	GetLaborCostForWO(ctx context.Context, woID uuid.UUID) (domain.Money, error)
}

// CostingNotifier fires a COSTING_COMPUTED SSE event after ComputeCost
// persists a record so the accountant panel + admin dashboard refresh without
// polling. Implementation lives in internal/platform/events; wired in main.go.
// Best-effort: a non-nil error is logged and the request still succeeds.
type CostingNotifier interface {
	NotifyCostingComputed(ctx context.Context, woID, costingType string) error
}

// AuditLogger writes a COSTING_ADJUSTED row to inventory_audit_log after a
// CostingAdjustment is persisted (BR-C04 audit trail for #250). The
// implementation delegates to inventory.Service.RecordCostingAdjustmentAudit
// and is wired in main.go via costingAuditAdapter.
//
// Best-effort: a non-nil error is logged via slog.Warn and the parent
// CreateAdjustment call still returns success — the adjustment row is the
// canonical record, the audit row is supplementary.
type AuditLogger interface {
	LogCostingAdjustment(ctx context.Context, in AuditCostingAdjustmentInput) error
}

// AuditCostingAdjustmentInput carries the fields needed to write a single
// COSTING_ADJUSTED row to inventory_audit_log. Metadata receives a JSON
// payload with the per-axis deltas + the parent costing_record_id and
// work_order_id so accountants can reconstruct the change without joining.
type AuditCostingAdjustmentInput struct {
	AdjustmentID    uuid.UUID
	CostingRecordID uuid.UUID
	WorkOrderID     uuid.UUID
	ActorID         uuid.UUID
	Reason          string
	DeltaMaterial   domain.Money
	DeltaAuxiliary  domain.Money
	DeltaLabor      domain.Money
	DeltaTotal      domain.Money
}
