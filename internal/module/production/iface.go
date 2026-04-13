package production

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type CreateWOInput struct {
	PlanID   uuid.UUID `json:"plan_id"`
	SKUID    uuid.UUID `json:"sku_id"`
	Quantity int       `json:"quantity"`
}

type WorkOrder struct {
	ID         uuid.UUID              `json:"id"`
	PlanID     uuid.UUID              `json:"plan_id"`
	SKUID      uuid.UUID              `json:"sku_id"`
	SKUCode    string                 `json:"sku_code"`
	SKUName    string                 `json:"sku_name"`
	Quantity   int                    `json:"quantity"`
	Status     domain.WorkOrderStatus `json:"status"`
	AssignedTo *uuid.UUID             `json:"assigned_to,omitempty"`
	AssignedAt *time.Time             `json:"assigned_at,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

type RecordConsumptionInput struct {
	WorkOrderID  uuid.UUID `json:"work_order_id"`
	MaterialID   uuid.UUID `json:"material_id"`
	MaterialType string    `json:"material_type"`
	Quantity     float64   `json:"quantity"`
	Unit         string    `json:"unit"`
}

type ConsumptionRecord struct {
	ID           uuid.UUID `json:"id"`
	WorkOrderID  uuid.UUID `json:"work_order_id"`
	MaterialID   uuid.UUID `json:"material_id"`
	MaterialType string    `json:"material_type"`
	Quantity     float64   `json:"quantity"`
	Unit         string    `json:"unit"`
	CreatedAt    time.Time `json:"created_at"`
}

type AssignWorkOrderInput struct {
	WorkOrderID uuid.UUID
	UserID      uuid.UUID
}

// AdvanceStatusInput is the request to advance a work order's status.
// SheetID is optional — when provided and the target status is IN_CUTTING,
// the sheet will be pre-assigned to the work order before the transition.
type AdvanceStatusInput struct {
	To      domain.WorkOrderStatus `json:"status"`
	SheetID *uuid.UUID             `json:"sheet_id,omitempty"`
}

type SuggestAssignmentResult struct {
	UserID         uuid.UUID `json:"user_id"`
	InCuttingCount int       `json:"in_cutting_count"`
}

type Service interface {
	CreateWorkOrder(ctx context.Context, in CreateWOInput) (WorkOrder, error)
	GetWorkOrder(ctx context.Context, woID uuid.UUID) (WorkOrder, error)
	ListWorkOrders(ctx context.Context, p httpkit.PageParams, status string) (httpkit.PagedResult[WorkOrder], error)
	ListWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error)
	ListWorkOrdersByAssignee(ctx context.Context, userID uuid.UUID) ([]WorkOrder, error)
	AdvanceStatus(ctx context.Context, woID uuid.UUID, in AdvanceStatusInput) error
	RecordConsumption(ctx context.Context, in RecordConsumptionInput) (ConsumptionRecord, error)
	ListConsumptions(ctx context.Context, woID uuid.UUID) ([]ConsumptionRecord, error)
	AssignWorkOrder(ctx context.Context, in AssignWorkOrderInput) (WorkOrder, error)
	SuggestAssignment(ctx context.Context, woID uuid.UUID) (SuggestAssignmentResult, error)
}
