package costing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type CostingRecord struct {
	ID            uuid.UUID    `json:"id"`
	WorkOrderID   uuid.UUID    `json:"work_order_id"`
	SKUID         uuid.UUID    `json:"sku_id"`
	MaterialCost  domain.Money `json:"material_cost"`
	AuxiliaryCost domain.Money `json:"auxiliary_cost"`
	TotalCost     domain.Money `json:"total_cost"`
	Finalized     bool         `json:"finalized"`
	CreatedAt     time.Time    `json:"created_at"`
}

type Service interface {
	ComputeCost(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error)
	FinalizeCost(ctx context.Context, workOrderID uuid.UUID) error
	GetCostingRecord(ctx context.Context, workOrderID uuid.UUID) (CostingRecord, error)
	ListCostingRecords(ctx context.Context) ([]CostingRecord, error)
}
