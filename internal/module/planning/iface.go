package planning

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type CreatePlanInput struct {
	POID     uuid.UUID       `json:"po_id"`
	Items    []PlanItemInput `json:"items"`
	Deadline *time.Time      `json:"deadline,omitempty"`
}

type PlanItemInput struct {
	SKUID    uuid.UUID `json:"sku_id"`
	Quantity int       `json:"quantity"`
}

type Plan struct {
	ID        uuid.UUID       `json:"id"`
	POID      uuid.UUID       `json:"po_id"`
	Status    domain.PlanStatus `json:"status"`
	Deadline  *time.Time      `json:"deadline,omitempty"`
	Items     []PlanItem      `json:"items"`
	CreatedAt time.Time       `json:"created_at"`
}

type PlanItem struct {
	ID     uuid.UUID `json:"id"`
	PlanID uuid.UUID `json:"plan_id"`
	SKUID  uuid.UUID `json:"sku_id"`
	Quantity int     `json:"quantity"`
}

type Service interface {
	CreatePlan(ctx context.Context, in CreatePlanInput) (Plan, error)
	GetPlan(ctx context.Context, planID uuid.UUID) (Plan, error)
	ListPlans(ctx context.Context, p httpkit.PageParams, status string) (httpkit.PagedResult[Plan], error)
	ApprovePlan(ctx context.Context, planID uuid.UUID) error
	CancelPlan(ctx context.Context, planID uuid.UUID) error
}
