package planning

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s store
}

func NewService(s store) Service {
	return &service{s: s}
}

func (svc *service) CreatePlan(ctx context.Context, in CreatePlanInput) (Plan, error) {
	if len(in.Items) == 0 {
		return Plan{}, domain.NewBizError(domain.ErrInvalidInput, "at least one item is required")
	}
	for _, item := range in.Items {
		if item.Quantity <= 0 {
			return Plan{}, domain.NewBizError(domain.ErrInvalidInput, "item quantity must be greater than 0")
		}
	}

	now := time.Now()
	code, err := svc.s.nextPlanCode(ctx, now.Year())
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{
		ID:        uuid.New(),
		Code:      code,
		POID:      in.POID,
		Status:    domain.PlanDraft,
		Deadline:  in.Deadline,
		CreatedAt: now,
	}

	if err := svc.s.insertPlan(ctx, plan); err != nil {
		return Plan{}, err
	}

	items := make([]PlanItem, len(in.Items))
	for i, inItem := range in.Items {
		items[i] = PlanItem{
			ID:       uuid.New(),
			PlanID:   plan.ID,
			SKUID:    inItem.SKUID,
			Quantity: inItem.Quantity,
		}
	}
	if err := svc.s.insertPlanItems(ctx, items); err != nil {
		return Plan{}, err
	}

	plan.Items = items
	return plan, nil
}

func (svc *service) GetPlan(ctx context.Context, planID uuid.UUID) (Plan, error) {
	plan, err := svc.s.selectPlanByID(ctx, planID)
	if err != nil {
		return Plan{}, err
	}
	items, err := svc.s.selectPlanItemsByPlanID(ctx, planID)
	if err != nil {
		return Plan{}, err
	}
	plan.Items = items
	return plan, nil
}

func (svc *service) ListPlans(ctx context.Context, p httpkit.PageParams, status string) (httpkit.PagedResult[Plan], error) {
	plans, total, err := svc.s.selectPlansPaged(ctx, p, status)
	if err != nil {
		return httpkit.PagedResult[Plan]{}, err
	}
	// Hydrate each plan's items inline.
	for i := range plans {
		items, err := svc.s.selectPlanItemsByPlanID(ctx, plans[i].ID)
		if err != nil {
			return httpkit.PagedResult[Plan]{}, err
		}
		plans[i].Items = items
	}
	return httpkit.NewPagedResult(plans, total, p), nil
}

func (svc *service) ApprovePlan(ctx context.Context, planID uuid.UUID) error {
	plan, err := svc.s.selectPlanByID(ctx, planID)
	if err != nil {
		return err
	}
	if plan.Status != domain.PlanDraft {
		return domain.NewBizError(domain.ErrInvalidTransition, "only draft plans can be approved")
	}
	return svc.s.updatePlanStatus(ctx, planID, string(domain.PlanApproved))
}

func (svc *service) CancelPlan(ctx context.Context, planID uuid.UUID) error {
	plan, err := svc.s.selectPlanByID(ctx, planID)
	if err != nil {
		return err
	}
	if plan.Status != domain.PlanDraft {
		return domain.NewBizError(domain.ErrInvalidTransition, "only draft plans can be canceled")
	}
	return svc.s.updatePlanStatus(ctx, planID, string(domain.PlanCanceled))
}
