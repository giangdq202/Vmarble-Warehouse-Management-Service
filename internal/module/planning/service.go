package planning

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s          store
	woCanceler WorkOrderCanceller
}

func NewService(s store) Service {
	return &service{s: s}
}

// NewServiceWithDeps wires the optional WorkOrderCanceller used by the
// APPROVED → CANCELED cascade (#249). Pass nil and the cascade refuses any
// APPROVED cancel — useful for unit tests that exercise DRAFT-only flows.
func NewServiceWithDeps(s store, wo WorkOrderCanceller) Service {
	return &service{s: s, woCanceler: wo}
}

func (svc *service) CreatePlan(ctx context.Context, in CreatePlanInput) (Plan, error) {
	// Exactly one of POID/SOID must be set — DB enforces via chk_plan_root,
	// but we reject early so the planner gets a clear ErrInvalidInput rather
	// than a 23514 from Postgres.
	if (in.POID == nil) == (in.SOID == nil) {
		return Plan{}, domain.NewBizError(domain.ErrInvalidInput, "exactly one of po_id or sales_order_id is required")
	}
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
		SOID:      in.SOID,
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

func (svc *service) ListPlans(ctx context.Context, p httpkit.PageParams, status string, from, to *time.Time) (httpkit.PagedResult[Plan], error) {
	if from != nil && to != nil && from.After(*to) {
		return httpkit.PagedResult[Plan]{}, domain.NewBizError(domain.ErrInvalidInput, "from must not be after to")
	}
	plans, total, err := svc.s.selectPlansPaged(ctx, p, status, from, to)
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

// CancelPlan implements the two-mode cancel introduced in #249.
//
//   - DRAFT  → flip status, no audit metadata required (no business activity yet).
//   - APPROVED → reason mandatory; every linked work order must still be PLANNED.
//     The cascade flips PLANNED WOs to CANCELED via the production adapter, then
//     persists status/reason/actor/timestamp atomically on the plan row.
//
// Any other source status is rejected with ErrInvalidTransition.
func (svc *service) CancelPlan(ctx context.Context, in CancelPlanInput) error {
	plan, err := svc.s.selectPlanByID(ctx, in.PlanID)
	if err != nil {
		return err
	}

	switch plan.Status {
	case domain.PlanDraft:
		return svc.s.updatePlanStatus(ctx, in.PlanID, string(domain.PlanCanceled))

	case domain.PlanApproved:
		if in.Reason == "" {
			return domain.NewBizError(domain.ErrInvalidInput, "cancel reason is required for approved plans")
		}
		if svc.woCanceler == nil {
			return domain.NewBizError(domain.ErrPreconditionFailed, "work order canceler is not configured")
		}
		statuses, err := svc.woCanceler.ListStatusesByPlan(ctx, in.PlanID)
		if err != nil {
			return err
		}
		for _, st := range statuses {
			if st != domain.WOPlanned && st != domain.WOCanceled {
				return domain.NewBizError(domain.ErrInvalidTransition,
					"cannot cancel plan: a work order has progressed past PLANNED")
			}
		}
		if _, err := svc.woCanceler.CancelPlannedByPlan(ctx, in.PlanID); err != nil {
			return err
		}
		return svc.s.cancelPlanWithMetadata(ctx, in.PlanID, in.Reason, in.ActorID, time.Now())

	default:
		return domain.NewBizError(domain.ErrInvalidTransition, "only draft or approved plans can be canceled")
	}
}

const maxLookupLimit = 50

func (svc *service) LookupPlans(ctx context.Context, in LookupPlansInput) (httpkit.PagedResult[PlanLookupItem], error) {
	if in.DeadlineFrom != nil && in.DeadlineTo != nil && in.DeadlineFrom.After(*in.DeadlineTo) {
		return httpkit.PagedResult[PlanLookupItem]{}, domain.NewBizError(domain.ErrInvalidInput, "deadline_from must not be after deadline_to")
	}

	limit := in.Limit
	if limit <= 0 || limit > maxLookupLimit {
		limit = maxLookupLimit
	}
	page := in.Page
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	items, total, err := svc.s.selectPlansLookup(ctx, in.Search, in.Status, in.DeadlineFrom, in.DeadlineTo, limit, offset)
	if err != nil {
		return httpkit.PagedResult[PlanLookupItem]{}, err
	}

	p := httpkit.PageParams{Page: page, Limit: limit}
	return httpkit.NewPagedResult(items, total, p), nil
}
