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
	ID        uuid.UUID         `json:"id"`
	Code      string            `json:"code"`
	POID      uuid.UUID         `json:"po_id"`
	POCode    string            `json:"po_code,omitempty"`
	Status    domain.PlanStatus `json:"status"`
	Deadline  *time.Time        `json:"deadline,omitempty"`
	Items     []PlanItem        `json:"items"`
	CreatedAt time.Time         `json:"created_at"`
}

type PlanItem struct {
	ID       uuid.UUID `json:"id"`
	PlanID   uuid.UUID `json:"plan_id"`
	SKUID    uuid.UUID `json:"sku_id"`
	Quantity int       `json:"quantity"`
}

// PlanLookupItem is a lightweight projection used by the async combobox lookup.
// It omits PlanItems to avoid N+1 hydration over large datasets.
type PlanLookupItem struct {
	ID       uuid.UUID         `json:"id"`
	Code     string            `json:"code"`
	POCode   string            `json:"po_code,omitempty"`
	Status   domain.PlanStatus `json:"status"`
	Deadline *time.Time        `json:"deadline,omitempty"`
}

// LookupPlansInput carries all filter parameters for the plan lookup endpoint.
type LookupPlansInput struct {
	Search       string     // ILIKE against plan code and PO code
	Status       string     // exact match on plan status; empty = all
	DeadlineFrom *time.Time // inclusive lower bound on deadline
	DeadlineTo   *time.Time // inclusive upper bound on deadline
	Page         int
	Limit        int // capped at maxLookupLimit in service
}

type Service interface {
	CreatePlan(ctx context.Context, in CreatePlanInput) (Plan, error)
	GetPlan(ctx context.Context, planID uuid.UUID) (Plan, error)
	ListPlans(ctx context.Context, p httpkit.PageParams, status string) (httpkit.PagedResult[Plan], error)
	LookupPlans(ctx context.Context, in LookupPlansInput) (httpkit.PagedResult[PlanLookupItem], error)
	ApprovePlan(ctx context.Context, planID uuid.UUID) error
	CancelPlan(ctx context.Context, planID uuid.UUID) error
}
