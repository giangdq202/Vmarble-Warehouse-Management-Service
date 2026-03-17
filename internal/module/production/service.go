package production

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type service struct {
	s  store
	pc PlanChecker
	sc SKUChecker
}

func NewService(s store, pc PlanChecker, sc SKUChecker) Service {
	return &service{s: s, pc: pc, sc: sc}
}

func (svc *service) CreateWorkOrder(ctx context.Context, in CreateWOInput) (WorkOrder, error) {
	plan, err := svc.pc.GetPlan(ctx, in.PlanID)
	if err != nil {
		return WorkOrder{}, err
	}
	if plan.Status != domain.PlanApproved {
		return WorkOrder{}, domain.NewBizError(domain.ErrPreconditionFailed, "production plan must be approved")
	}

	if in.Quantity <= 0 {
		return WorkOrder{}, domain.NewBizError(domain.ErrInvalidInput, "quantity must be greater than 0")
	}

	now := time.Now()
	wo := WorkOrder{
		ID:        uuid.New(),
		PlanID:    in.PlanID,
		SKUID:     in.SKUID,
		Quantity:  in.Quantity,
		Status:    domain.WOPlanned,
		CreatedAt: now,
	}
	if err := svc.s.insertWorkOrder(ctx, wo); err != nil {
		return WorkOrder{}, err
	}
	return wo, nil
}

func (svc *service) GetWorkOrder(ctx context.Context, woID uuid.UUID) (WorkOrder, error) {
	return svc.s.selectWorkOrderByID(ctx, woID)
}

func (svc *service) ListWorkOrders(ctx context.Context) ([]WorkOrder, error) {
	return svc.s.selectWorkOrders(ctx)
}

func (svc *service) ListWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error) {
	return svc.s.selectWorkOrdersByPlan(ctx, planID)
}

func (svc *service) AdvanceStatus(ctx context.Context, woID uuid.UUID, to domain.WorkOrderStatus) error {
	wo, err := svc.s.selectWorkOrderByID(ctx, woID)
	if err != nil {
		return err
	}

	if err := wo.Status.CanTransitionTo(to); err != nil {
		return domain.NewBizError(domain.ErrInvalidTransition, err.Error())
	}

	if to == domain.WOCompleted {
		sku, err := svc.sc.GetSKU(ctx, wo.SKUID)
		if err != nil {
			return err
		}
		if sku.RequiresMetal {
			hasMetal, err := svc.s.hasMetalConsumption(ctx, woID)
			if err != nil {
				return err
			}
			if !hasMetal {
				return domain.NewBizError(domain.ErrPreconditionFailed, "metal consumption required before completing this work order")
			}
		}
	}

	return svc.s.updateWorkOrderStatus(ctx, woID, string(to))
}

func (svc *service) RecordConsumption(ctx context.Context, in RecordConsumptionInput) (ConsumptionRecord, error) {
	wo, err := svc.s.selectWorkOrderByID(ctx, in.WorkOrderID)
	if err != nil {
		return ConsumptionRecord{}, err
	}

	if wo.Status != domain.WOInProcessing && wo.Status != domain.WOCompleted {
		return ConsumptionRecord{}, domain.NewBizError(domain.ErrPreconditionFailed, "consumption can only be recorded when work order is IN_PROCESSING or COMPLETED")
	}

	if in.Quantity <= 0 {
		return ConsumptionRecord{}, domain.NewBizError(domain.ErrInvalidInput, "quantity must be greater than 0")
	}

	now := time.Now()
	cr := ConsumptionRecord{
		ID:            uuid.New(),
		WorkOrderID:   in.WorkOrderID,
		MaterialID:    in.MaterialID,
		MaterialType:  in.MaterialType,
		Unit:          in.Unit,
		Quantity:      in.Quantity,
		CreatedAt:     now,
	}
	if err := svc.s.insertConsumption(ctx, cr); err != nil {
		return ConsumptionRecord{}, err
	}
	return cr, nil
}

func (svc *service) ListConsumptions(ctx context.Context, woID uuid.UUID) ([]ConsumptionRecord, error) {
	_, err := svc.s.selectWorkOrderByID(ctx, woID)
	if err != nil {
		return nil, err
	}
	return svc.s.selectConsumptionsByWO(ctx, woID)
}
