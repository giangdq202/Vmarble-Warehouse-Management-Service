package production

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s        store
	pc       PlanChecker
	sc       SKUChecker
	uc       UserChecker
	sa       SheetAssigner
	notifier WorkOrderNotifier
}

func NewService(s store, pc PlanChecker, sc SKUChecker, uc UserChecker, sa SheetAssigner, notifier WorkOrderNotifier) Service {
	return &service{s: s, pc: pc, sc: sc, uc: uc, sa: sa, notifier: notifier}
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

func (svc *service) ListWorkOrders(ctx context.Context, p httpkit.PageParams, status string) (httpkit.PagedResult[WorkOrder], error) {
	wos, total, err := svc.s.selectWorkOrdersPaged(ctx, p, status)
	if err != nil {
		return httpkit.PagedResult[WorkOrder]{}, err
	}
	return httpkit.NewPagedResult(wos, total, p), nil
}

func (svc *service) ListWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error) {
	return svc.s.selectWorkOrdersByPlan(ctx, planID)
}

func (svc *service) ListWorkOrdersByAssignee(ctx context.Context, userID uuid.UUID) ([]WorkOrder, error) {
	return svc.s.selectWorkOrdersByAssignee(ctx, userID)
}

func (svc *service) AdvanceStatus(ctx context.Context, woID uuid.UUID, in AdvanceStatusInput) error {
	wo, err := svc.s.selectWorkOrderByID(ctx, woID)
	if err != nil {
		return err
	}

	if err := wo.Status.CanTransitionTo(in.To); err != nil {
		return domain.NewBizError(domain.ErrInvalidTransition, err.Error())
	}

	if in.To == domain.WOCompleted {
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

	// Pre-assign the board sheet to the work order when advancing to IN_CUTTING.
	// The SheetAssigner validates the sheet is AVAILABLE and stamps
	// issued_to_work_order_id on the board_sheets row before the status changes.
	if in.To == domain.WOInCutting && in.SheetID != nil && svc.sa != nil {
		if err := svc.sa.PreAssignSheet(ctx, *in.SheetID, woID); err != nil {
			return err
		}
	}

	return svc.s.updateWorkOrderStatus(ctx, woID, string(in.To))
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
		ID:           uuid.New(),
		WorkOrderID:  in.WorkOrderID,
		MaterialID:   in.MaterialID,
		MaterialType: in.MaterialType,
		Unit:         in.Unit,
		Quantity:     in.Quantity,
		CreatedAt:    now,
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

// AssignWorkOrder assigns a PLANNED WorkOrder to a CNC operator.
// Only users with role "cnc" can be assigned.
// Only WorkOrders in status PLANNED can be assigned.
func (svc *service) AssignWorkOrder(ctx context.Context, in AssignWorkOrderInput) (WorkOrder, error) {
	wo, err := svc.s.selectWorkOrderByID(ctx, in.WorkOrderID)
	if err != nil {
		return WorkOrder{}, err
	}

	if wo.Status != domain.WOPlanned {
		return WorkOrder{}, domain.NewBizError(domain.ErrPreconditionFailed, "only PLANNED work orders can be assigned")
	}

	u, err := svc.uc.GetUser(ctx, in.UserID)
	if err != nil {
		return WorkOrder{}, err
	}

	if u.Role != string(auth.RoleCNC) {
		return WorkOrder{}, domain.NewBizError(domain.ErrInvalidInput, "assigned user must have role 'cnc'")
	}

	assignedAt := time.Now()
	if err := svc.s.updateWorkOrderAssignment(ctx, in.WorkOrderID, in.UserID, assignedAt); err != nil {
		return WorkOrder{}, err
	}

	wo.AssignedTo = &in.UserID
	wo.AssignedAt = &assignedAt

	if svc.notifier != nil {
		if err := svc.notifier.NotifyAssignment(ctx, in.UserID.String(), wo.ID.String(), wo.SKUCode); err != nil {
			slog.Warn("production: AssignWorkOrder notification failed", "wo_id", wo.ID, "err", err)
		}
	}

	return wo, nil
}

// SuggestAssignment returns the CNC user with the fewest WorkOrders currently
// in status IN_CUTTING (Least Busy algorithm). Ties are broken by UUID ordering
// for determinism.
func (svc *service) SuggestAssignment(ctx context.Context, woID uuid.UUID) (SuggestAssignmentResult, error) {
	if _, err := svc.s.selectWorkOrderByID(ctx, woID); err != nil {
		return SuggestAssignmentResult{}, err
	}

	cncUsers, err := svc.s.selectCNCUserIDs(ctx)
	if err != nil {
		return SuggestAssignmentResult{}, err
	}
	if len(cncUsers) == 0 {
		return SuggestAssignmentResult{}, domain.NewBizError(domain.ErrNotFound, "no CNC operators available for assignment")
	}

	load, err := svc.s.selectInCuttingCountByUser(ctx)
	if err != nil {
		return SuggestAssignmentResult{}, err
	}

	// Pick the user with the lowest current IN_CUTTING count.
	// cncUsers is ordered by id (UUID) so ties are broken deterministically.
	bestUser := cncUsers[0]
	bestCount := load[bestUser] // 0 if not in map
	for _, uid := range cncUsers[1:] {
		if c := load[uid]; c < bestCount {
			bestUser = uid
			bestCount = c
		}
	}

	return SuggestAssignmentResult{UserID: bestUser, InCuttingCount: bestCount}, nil
}
