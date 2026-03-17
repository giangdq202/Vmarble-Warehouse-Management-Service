package production

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type CreateWOInput struct {
	PlanID   uuid.UUID `json:"plan_id"`
	SKUID    uuid.UUID `json:"sku_id"`
	Quantity int       `json:"quantity"`
}

type WorkOrder struct {
	ID        uuid.UUID            `json:"id"`
	PlanID    uuid.UUID            `json:"plan_id"`
	SKUID     uuid.UUID            `json:"sku_id"`
	Quantity  int                   `json:"quantity"`
	Status    domain.WorkOrderStatus `json:"status"`
	CreatedAt time.Time            `json:"created_at"`
}

type RecordConsumptionInput struct {
	WorkOrderID  uuid.UUID `json:"work_order_id"`
	MaterialID   uuid.UUID `json:"material_id"`
	MaterialType string    `json:"material_type"`
	Quantity     float64   `json:"quantity"`
	Unit         string    `json:"unit"`
}

type ConsumptionRecord struct {
	ID            uuid.UUID `json:"id"`
	WorkOrderID   uuid.UUID `json:"work_order_id"`
	MaterialID    uuid.UUID `json:"material_id"`
	MaterialType  string    `json:"material_type"`
	Quantity      float64   `json:"quantity"`
	Unit          string    `json:"unit"`
	CreatedAt     time.Time `json:"created_at"`
}

type Service interface {
	CreateWorkOrder(ctx context.Context, in CreateWOInput) (WorkOrder, error)
	GetWorkOrder(ctx context.Context, woID uuid.UUID) (WorkOrder, error)
	ListWorkOrders(ctx context.Context) ([]WorkOrder, error)
	ListWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error)
	AdvanceStatus(ctx context.Context, woID uuid.UUID, to domain.WorkOrderStatus) error
	RecordConsumption(ctx context.Context, in RecordConsumptionInput) (ConsumptionRecord, error)
	ListConsumptions(ctx context.Context, woID uuid.UUID) ([]ConsumptionRecord, error)
}
