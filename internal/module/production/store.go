package production

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertWorkOrder(ctx context.Context, wo WorkOrder) error
	selectWorkOrdersPaged(ctx context.Context, p httpkit.PageParams, f WorkOrderListFilter) ([]WorkOrder, int, error)
	selectWorkOrderByID(ctx context.Context, id uuid.UUID) (WorkOrder, error)
	selectWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error)
	selectWorkOrdersByAssignee(ctx context.Context, userID uuid.UUID) ([]WorkOrder, error)
	updateWorkOrderStatus(ctx context.Context, id uuid.UUID, status string) error
	updateWorkOrderAssignment(ctx context.Context, woID uuid.UUID, userID uuid.UUID, assignedAt time.Time) error
	insertConsumption(ctx context.Context, cr ConsumptionRecord) error
	selectConsumptionsByWO(ctx context.Context, woID uuid.UUID) ([]ConsumptionRecord, error)
	hasMetalConsumption(ctx context.Context, woID uuid.UUID) (bool, error)
	selectInCuttingCountByUser(ctx context.Context) (map[uuid.UUID]int, error)
	selectCNCUserIDs(ctx context.Context) ([]uuid.UUID, error)

	// Machine CRUD
	insertMachine(ctx context.Context, m Machine) error
	selectMachines(ctx context.Context) ([]Machine, error)
	selectMachineByID(ctx context.Context, id uuid.UUID) (Machine, error)
	deactivateMachine(ctx context.Context, id uuid.UUID) error

	// Slot CRUD (assigned_hours computed via JOIN at query time)
	insertSlot(ctx context.Context, s MachineShiftSlot) error
	selectSlotByID(ctx context.Context, id uuid.UUID) (MachineShiftSlot, error)
	selectSlotsByMachine(ctx context.Context, machineID uuid.UUID, from, to time.Time) ([]MachineShiftSlot, error)
	// selectFutureSlotsWithCapacity returns OPEN slots (shift_date >= today) whose
	// available capacity (capacity_hours - assigned_hours) >= minAvailableHours,
	// sorted by shift_date ASC then available_hours DESC.
	selectFutureSlotsWithCapacity(ctx context.Context, minAvailableHours float64) ([]MachineShiftSlot, error)
	deleteSlot(ctx context.Context, id uuid.UUID) error

	// Work order scheduling
	updateEstimatedHours(ctx context.Context, woID uuid.UUID, hours float64) error
	unassignWOFromSlot(ctx context.Context, woID uuid.UUID) error
	// assignSlotAtomically acquires a row-level lock on the slot, re-validates
	// remaining capacity under the lock, then sets machine_slot_id on the WO.
	assignSlotAtomically(ctx context.Context, op assignSlotOp) error

	// Labor cost entries
	insertLaborEntry(ctx context.Context, e LaborEntry) error
	selectLaborEntriesByWO(ctx context.Context, woID uuid.UUID) ([]LaborEntry, error)
	// sumLaborMinuteRateByWO returns SUM(minutes * rate_per_hour) for the work
	// order in dong·minutes; callers divide by 60 to get dong. Returns 0 when
	// no entries exist.
	sumLaborMinuteRateByWO(ctx context.Context, woID uuid.UUID) (int64, error)

	// Plan cascade cancel (#249)
	// listStatusesByPlan returns the current status of every work order tied to
	// the plan. Used by planning.CancelPlan to verify no WO has progressed past
	// PLANNED before cascading.
	listStatusesByPlan(ctx context.Context, planID uuid.UUID) ([]string, error)
	// cancelPlannedByPlan flips every PLANNED work order under the plan to
	// CANCELED in a single UPDATE. Returns the affected row count. Bypasses
	// the AdvanceStatus state machine deliberately — the cascade is only
	// invoked from planning.CancelPlan after upstream validation.
	cancelPlannedByPlan(ctx context.Context, planID uuid.UUID) (int64, error)
}

// assignSlotOp carries the pre-validated data for a single slot assignment.
type assignSlotOp struct {
	WorkOrderID    uuid.UUID
	SlotID         uuid.UUID
	EstimatedHours float64
}
