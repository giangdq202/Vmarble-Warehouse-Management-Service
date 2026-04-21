package costing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type CostingRecord struct {
	ID            uuid.UUID  `json:"id"`
	WorkOrderID   uuid.UUID  `json:"work_order_id"`
	SKUID         uuid.UUID  `json:"sku_id"`
	MaterialCost  domain.Money `json:"material_cost"`
	AuxiliaryCost domain.Money `json:"auxiliary_cost"`
	LaborCost     domain.Money `json:"labor_cost"`
	TotalCost     domain.Money `json:"total_cost"`
	Finalized     bool         `json:"finalized"`
	FinalizedAt   *time.Time `json:"finalized_at,omitempty"`
	FinalizedBy   *uuid.UUID `json:"finalized_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
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

type CreateAdjustmentInput struct {
	WorkOrderID      uuid.UUID    `json:"-"`
	Reason           string       `json:"reason"`
	DeltaMaterial    domain.Money `json:"delta_material"`
	DeltaAuxiliary   domain.Money `json:"delta_auxiliary"`
	DeltaLabor       domain.Money `json:"delta_labor"`
	CreatedBy        uuid.UUID    `json:"-"`
}

type Service interface {
	ComputeCost(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error)
	FinalizeCost(ctx context.Context, workOrderID uuid.UUID, actorID uuid.UUID) error
	GetCostingRecord(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error)
	ListCostingRecords(ctx context.Context, p httpkit.PageParams, finalized *bool) (httpkit.PagedResult[CostingRecord], error)
	CreateAdjustment(ctx context.Context, in CreateAdjustmentInput) (CostingAdjustment, error)
	ListAdjustments(ctx context.Context, workOrderID uuid.UUID) ([]CostingAdjustment, error)
}
