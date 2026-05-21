package costing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// CostingType distinguishes pre-cut estimates from post-completion actuals.
type CostingType string

const (
	CostingTypeEstimated CostingType = "ESTIMATED"
	CostingTypeActual    CostingType = "ACTUAL"
)

type CostingRecord struct {
	ID            uuid.UUID    `json:"id"`
	WorkOrderID   uuid.UUID    `json:"work_order_id"`
	SKUID         uuid.UUID    `json:"sku_id"`
	CostingType   CostingType  `json:"costing_type"`
	MaterialCost  domain.Money `json:"material_cost"`
	AuxiliaryCost domain.Money `json:"auxiliary_cost"`
	LaborCost     domain.Money `json:"labor_cost"`
	TotalCost     domain.Money `json:"total_cost"`
	Finalized     bool         `json:"finalized"`
	FinalizedAt   *time.Time   `json:"finalized_at,omitempty"`
	FinalizedBy   *uuid.UUID   `json:"finalized_by,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
}

type CostingAdjustment struct {
	ID               uuid.UUID    `json:"id"`
	CostingRecordID  uuid.UUID    `json:"costing_record_id"`
	Reason           string       `json:"reason"`
	DeltaMaterial    domain.Money `json:"delta_material"`
	DeltaAuxiliary   domain.Money `json:"delta_auxiliary"`
	DeltaLabor       domain.Money `json:"delta_labor"`
	DeltaTotal       domain.Money `json:"delta_total"`
	CreatedBy        uuid.UUID    `json:"created_by"`
	CreatedAt        time.Time    `json:"created_at"`
}

// CostingRecordDetail bundles a finalized record with all of its adjustments
// and the running effective total. Effective values fold the original record
// numbers with the cumulative adjustment deltas so the FE never has to do
// money arithmetic itself.
type CostingRecordDetail struct {
	Record           CostingRecord       `json:"record"`
	Adjustments      []CostingAdjustment `json:"adjustments"`
	EffectiveMaterial  domain.Money      `json:"effective_material"`
	EffectiveAuxiliary domain.Money      `json:"effective_auxiliary"`
	EffectiveLabor     domain.Money      `json:"effective_labor"`
	EffectiveTotal     domain.Money      `json:"effective_total"`
}

type CreateAdjustmentInput struct {
	WorkOrderID      uuid.UUID    `json:"-"`
	Reason           string       `json:"reason"`
	DeltaMaterial    domain.Money `json:"delta_material"`
	DeltaAuxiliary   domain.Money `json:"delta_auxiliary"`
	DeltaLabor       domain.Money `json:"delta_labor"`
	CreatedBy        uuid.UUID    `json:"-"`
}

// WasteReportFilter narrows the waste-cost ledger by created_at range
// (cutting_records.created_at) and an optional material.
type WasteReportFilter struct {
	From       *time.Time
	To         *time.Time
	MaterialID *uuid.UUID
}

// WasteReportRow is one line in the per-material waste-cost ledger.
//
// BR-C03: waste area is excluded from per-SKU allocation; this report
// posts the corresponding cost to the "tài khoản hao hụt" (waste account).
type WasteReportRow struct {
	MaterialID     uuid.UUID    `json:"material_id"`
	MaterialName   string       `json:"material_name"`
	SheetsConsumed int          `json:"sheets_consumed"`
	WasteAreaMM2   int64        `json:"waste_area_mm2"`
	AvgSheetCost   domain.Money `json:"avg_sheet_cost"`
	TotalWasteCost domain.Money `json:"total_waste_cost"`
}

type Service interface {
	ComputeCost(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error)
	FinalizeCost(ctx context.Context, workOrderID uuid.UUID, actorID uuid.UUID) error
	GetCostingRecord(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error)
	// GetCostingRecordDetail returns the costing record together with all of
	// its adjustments and the running effective totals (record + Σ deltas).
	// Used by the accountant adjustment dialog so the FE never has to do
	// money arithmetic itself.
	GetCostingRecordDetail(ctx context.Context, workOrderID uuid.UUID) (CostingRecordDetail, error)
	ListCostingRecords(ctx context.Context, params httpkit.CursorParams, finalized *bool) (httpkit.CursorResult[CostingRecord], error)
	HasCostingRecord(ctx context.Context, workOrderID uuid.UUID) (bool, error)
	// IsCostingFinalized reports whether the costing record for the work order
	// is finalized (BR-C04). Returns (false, nil) when no record exists yet.
	IsCostingFinalized(ctx context.Context, workOrderID uuid.UUID) (bool, error)
	CreateAdjustment(ctx context.Context, in CreateAdjustmentInput) (CostingAdjustment, error)
	ListAdjustments(ctx context.Context, workOrderID uuid.UUID) ([]CostingAdjustment, error)
	ListWasteReport(ctx context.Context, filter WasteReportFilter) ([]WasteReportRow, error)
}
