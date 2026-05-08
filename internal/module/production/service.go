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
	cc       CostingChecker
	notifier WorkOrderNotifier
}

func NewService(s store, pc PlanChecker, sc SKUChecker, uc UserChecker, sa SheetAssigner, cc CostingChecker, notifier WorkOrderNotifier) Service {
	return &service{s: s, pc: pc, sc: sc, uc: uc, sa: sa, cc: cc, notifier: notifier}
}

func (svc *service) CreateWorkOrder(ctx context.Context, in CreateWOInput) (WorkOrder, error) {
	plan, err := svc.pc.GetPlan(ctx, in.PlanID)
	if err != nil {
		return WorkOrder{}, err
	}
	if plan.Status != domain.PlanApproved {
		return WorkOrder{}, domain.NewBizError(domain.ErrPreconditionFailed, "production plan must be approved")
	}

	if !planContainsSKU(plan, in.SKUID) {
		return WorkOrder{}, domain.NewBizError(domain.ErrPreconditionFailed, "SKU does not belong to this production plan")
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

// planContainsSKU returns true if the given SKU ID is among the plan's items.
func planContainsSKU(plan PlanInfo, skuID uuid.UUID) bool {
	for _, id := range plan.SKUIDs {
		if id == skuID {
			return true
		}
	}
	return false
}

func (svc *service) GetWorkOrder(ctx context.Context, woID uuid.UUID) (WorkOrder, error) {
	return svc.s.selectWorkOrderByID(ctx, woID)
}

func (svc *service) ListWorkOrders(ctx context.Context, p httpkit.PageParams, f WorkOrderListFilter) (httpkit.PagedResult[WorkOrder], error) {
	wos, total, err := svc.s.selectWorkOrdersPaged(ctx, p, f)
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

	// When advancing to IN_CUTTING, enforce assignment invariant (Spec 5.1):
	// - WO must already be assigned to a CNC operator.
	// - If a CallerID is provided (from JWT), it must match the assigned operator.
	// - Admin is a super-user and may override these operator-specific guards.
	if in.To == domain.WOInCutting && in.CallerRole != auth.RoleAdmin {
		if wo.AssignedTo == nil {
			return domain.NewBizError(domain.ErrPreconditionFailed, "work order must be assigned to a CNC operator before cutting starts")
		}
		if in.CallerID != nil && *in.CallerID != *wo.AssignedTo {
			return domain.NewBizError(domain.ErrPreconditionFailed, "only the assigned CNC operator can start cutting this work order")
		}
	}

	// When advancing to IN_CUTTING, an estimated costing record must exist.
	if in.To == domain.WOInCutting && svc.cc != nil {
		hasCost, err := svc.cc.HasCostingRecord(ctx, woID)
		if err != nil {
			return err
		}
		if !hasCost {
			return domain.NewBizError(domain.ErrPreconditionFailed, "estimated cost must be computed before cutting starts")
		}
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

// --- Machine management ---

func (svc *service) CreateMachine(ctx context.Context, in CreateMachineInput) (Machine, error) {
	if in.Code == "" {
		return Machine{}, domain.NewBizError(domain.ErrInvalidInput, "machine code is required")
	}
	if in.Name == "" {
		return Machine{}, domain.NewBizError(domain.ErrInvalidInput, "machine name is required")
	}
	if in.CapacityHoursPerShift <= 0 {
		return Machine{}, domain.NewBizError(domain.ErrInvalidInput, "capacity_hours_per_shift must be greater than 0")
	}
	m := Machine{
		ID:                    uuid.New(),
		Code:                  in.Code,
		Name:                  in.Name,
		CapacityHoursPerShift: in.CapacityHoursPerShift,
		IsActive:              true,
		CreatedAt:             time.Now().UTC(),
	}
	if err := svc.s.insertMachine(ctx, m); err != nil {
		return Machine{}, err
	}
	return m, nil
}

func (svc *service) ListMachines(ctx context.Context) ([]Machine, error) {
	return svc.s.selectMachines(ctx)
}

func (svc *service) GetMachine(ctx context.Context, machineID uuid.UUID) (Machine, error) {
	return svc.s.selectMachineByID(ctx, machineID)
}

func (svc *service) DeactivateMachine(ctx context.Context, machineID uuid.UUID) error {
	return svc.s.deactivateMachine(ctx, machineID)
}

// --- Shift slot management ---

func (svc *service) CreateSlot(ctx context.Context, in CreateSlotInput) (MachineShiftSlot, error) {
	if in.ShiftName == "" {
		return MachineShiftSlot{}, domain.NewBizError(domain.ErrInvalidInput, "shift_name is required")
	}
	if in.ShiftDate.IsZero() {
		return MachineShiftSlot{}, domain.NewBizError(domain.ErrInvalidInput, "shift_date is required")
	}
	machine, err := svc.s.selectMachineByID(ctx, in.MachineID)
	if err != nil {
		return MachineShiftSlot{}, err
	}
	if !machine.IsActive {
		return MachineShiftSlot{}, domain.NewBizError(domain.ErrInvalidInput, "cannot create slot for inactive machine")
	}

	capacityHours := machine.CapacityHoursPerShift
	if in.CapacityHours != nil {
		if *in.CapacityHours <= 0 {
			return MachineShiftSlot{}, domain.NewBizError(domain.ErrInvalidInput, "capacity_hours must be greater than 0")
		}
		capacityHours = *in.CapacityHours
	}

	sl := MachineShiftSlot{
		ID:            uuid.New(),
		MachineID:     in.MachineID,
		MachineCode:   machine.Code,
		MachineName:   machine.Name,
		ShiftDate:     in.ShiftDate,
		ShiftName:     in.ShiftName,
		CapacityHours: capacityHours,
		CreatedAt:     time.Now().UTC(),
	}
	if err := svc.s.insertSlot(ctx, sl); err != nil {
		return MachineShiftSlot{}, err
	}
	return sl, nil
}

func (svc *service) ListSlots(ctx context.Context, machineID uuid.UUID, from, to time.Time) ([]MachineShiftSlot, error) {
	if from.After(to) {
		return nil, domain.NewBizError(domain.ErrInvalidInput, "from must be before or equal to to")
	}
	return svc.s.selectSlotsByMachine(ctx, machineID, from, to)
}

func (svc *service) GetSlot(ctx context.Context, slotID uuid.UUID) (MachineShiftSlot, error) {
	return svc.s.selectSlotByID(ctx, slotID)
}

func (svc *service) DeleteSlot(ctx context.Context, slotID uuid.UUID) error {
	return svc.s.deleteSlot(ctx, slotID)
}

// --- Work order scheduling ---

func (svc *service) SetEstimatedHours(ctx context.Context, in SetEstimatedHoursInput) (WorkOrder, error) {
	if in.EstimatedHours <= 0 {
		return WorkOrder{}, domain.NewBizError(domain.ErrInvalidInput, "estimated_hours must be greater than 0")
	}
	if err := svc.s.updateEstimatedHours(ctx, in.WorkOrderID, in.EstimatedHours); err != nil {
		return WorkOrder{}, err
	}
	return svc.s.selectWorkOrderByID(ctx, in.WorkOrderID)
}

func (svc *service) AssignSlot(ctx context.Context, in AssignSlotInput) (WorkOrder, error) {
	wo, err := svc.s.selectWorkOrderByID(ctx, in.WorkOrderID)
	if err != nil {
		return WorkOrder{}, err
	}
	if wo.Status != domain.WOPlanned && wo.Status != domain.WOInCutting {
		return WorkOrder{}, domain.NewBizError(domain.ErrPreconditionFailed, "slot can only be assigned to PLANNED or IN_CUTTING work orders")
	}
	if wo.EstimatedHours == nil {
		return WorkOrder{}, domain.NewBizError(domain.ErrPreconditionFailed, "set estimated_hours before assigning a slot")
	}

	op := assignSlotOp{
		WorkOrderID:    in.WorkOrderID,
		SlotID:         in.SlotID,
		EstimatedHours: *wo.EstimatedHours,
	}
	if err := svc.s.assignSlotAtomically(ctx, op); err != nil {
		return WorkOrder{}, err
	}
	return svc.s.selectWorkOrderByID(ctx, in.WorkOrderID)
}

func (svc *service) UnassignSlot(ctx context.Context, woID uuid.UUID) (WorkOrder, error) {
	if err := svc.s.unassignWOFromSlot(ctx, woID); err != nil {
		return WorkOrder{}, err
	}
	return svc.s.selectWorkOrderByID(ctx, woID)
}

// SuggestSchedule returns the top 5 available slots that can accommodate the
// work order's estimated hours, sorted by earliest date then largest remaining capacity.
func (svc *service) SuggestSchedule(ctx context.Context, woID uuid.UUID) ([]ScheduleSuggestion, error) {
	wo, err := svc.s.selectWorkOrderByID(ctx, woID)
	if err != nil {
		return nil, err
	}
	if wo.EstimatedHours == nil {
		return nil, domain.NewBizError(domain.ErrPreconditionFailed, "set estimated_hours before requesting schedule suggestions")
	}

	slots, err := svc.s.selectFutureSlotsWithCapacity(ctx, *wo.EstimatedHours)
	if err != nil {
		return nil, err
	}

	const maxSuggestions = 5
	if len(slots) > maxSuggestions {
		slots = slots[:maxSuggestions]
	}

	out := make([]ScheduleSuggestion, len(slots))
	for i, sl := range slots {
		out[i] = ScheduleSuggestion{
			Slot:           sl,
			AvailableHours: sl.CapacityHours - sl.AssignedHours,
			Rank:           i + 1,
		}
	}
	return out, nil
}
