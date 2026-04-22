package production

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertWorkOrder(ctx context.Context, wo WorkOrder) error
	selectWorkOrdersPaged(ctx context.Context, p httpkit.PageParams, status string, planID *uuid.UUID) ([]WorkOrder, int, error)
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
}

// assignSlotOp carries the pre-validated data for a single slot assignment.
type assignSlotOp struct {
	WorkOrderID    uuid.UUID
	SlotID         uuid.UUID
	EstimatedHours float64
}

