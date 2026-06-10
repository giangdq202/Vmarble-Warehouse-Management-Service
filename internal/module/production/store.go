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
	selectWorkOrdersKeyset(ctx context.Context, f WorkOrderListFilter, cur httpkit.Cursor, limit int) ([]WorkOrder, error)
	selectWorkOrderByID(ctx context.Context, id uuid.UUID) (WorkOrder, error)
	selectWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error)
	selectWorkOrdersByAssignee(ctx context.Context, userID uuid.UUID) ([]WorkOrder, error)
	updateWorkOrderStatus(ctx context.Context, id uuid.UUID, status string) error
	updateWorkOrderAssignment(ctx context.Context, woID uuid.UUID, userID uuid.UUID, assignedAt time.Time) error
	// reassignWorkOrderAtomically acquires a row-level lock on the WO, verifies
	// status is IN_CUTTING or IN_PROCESSING, updates assigned_to, and inserts a
	// wo_reassign_log row — all inside one transaction.
	reassignWorkOrderAtomically(ctx context.Context, op reassignOp) (WorkOrder, error)
	// claimWorkOrderAtomically acquires a row-level lock on the WO, verifies
	// status is PLANNED and assigned_to IS NULL, then sets assigned_to. Returns
	// 409 (ErrPreconditionFailed) if the WO is already claimed.
	claimWorkOrderAtomically(ctx context.Context, op claimOp) (WorkOrder, error)
	// updateWorkOrderQCStatus writes the denormalized qc_status column.
	updateWorkOrderQCStatus(ctx context.Context, woID uuid.UUID, status string) error
	// partialCompleteAtomically performs the entire #292 PartialComplete write
	// inside a single SELECT FOR UPDATE transaction so two concurrent callers
	// cannot both win. The store re-reads the WO under the lock, validates
	// status==IN_PROCESSING, flips status + actual_qty + shortfall_reason, and
	// optionally inserts a carry-over WO. Returns the post-update parent WO
	// plus the carry-over WO (zero-value when CarryOver=false).
	partialCompleteAtomically(ctx context.Context, op partialCompleteOp) (WorkOrder, WorkOrder, error)
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

	// Smart re-allocation (BE #2)
	// selectWOWithPlanDeadline returns the WO quantity, status, plan deadline,
	// and primary sheet material for feasibility scoring.
	selectWOWithPlanDeadline(ctx context.Context, woID uuid.UUID) (woFeasibilityData, error)
	// selectFeasibilitySuggestions returns up to limit PLANNED WOs that share
	// the given sheet material, ordered by score desc (1/days_to_due).
	selectFeasibilitySuggestions(ctx context.Context, materialID uuid.UUID, excludeWOID uuid.UUID, limit int) ([]woSuggestionRow, error)
	// setPriorityBoostAtomically sets priority_boost=true and inserts a
	// wo_boost_log row inside one transaction.
	setPriorityBoostAtomically(ctx context.Context, op setPriorityBoostOp) (uuid.UUID, time.Time, error)
	// selectPreemptCandidates returns PLANNED work orders sharing a sheet
	// material with woID, ordered by ascending plan deadline (most urgent last
	// → most slack first so preemption impact is minimised).
	selectPreemptCandidates(ctx context.Context, woID uuid.UUID) ([]preemptCandidateRow, error)
	// preemptAtomically acquires row locks on both WOs, verifies from_wo is
	// PLANNED or IN_CUTTING (rejects IN_PROCESSING+), reverts from_wo to
	// PLANNED, and inserts a wo_preemption_log row.
	preemptAtomically(ctx context.Context, op preemptOp) (uuid.UUID, time.Time, int, error)
}

// reassignOp carries the payload for reassignWorkOrderAtomically.
type reassignOp struct {
	WorkOrderID uuid.UUID
	NewUserID   uuid.UUID
	Reason      string
	ActorID     uuid.UUID
	LogID       uuid.UUID
	AssignedAt  time.Time
}

// claimOp carries the payload for claimWorkOrderAtomically.
type claimOp struct {
	WorkOrderID uuid.UUID
	UserID      uuid.UUID
	AssignedAt  time.Time
}

// assignSlotOp carries the pre-validated data for a single slot assignment.
type assignSlotOp struct {
	WorkOrderID    uuid.UUID
	SlotID         uuid.UUID
	EstimatedHours float64
}

// partialCompleteOp carries the pre-validated payload for the atomic
// PartialComplete write. The service does upstream validation; the store
// re-reads the WO under SELECT FOR UPDATE so the BR-P05 IN_PROCESSING gate
// is enforced under the lock (concurrency safety).
type partialCompleteOp struct {
	WorkOrderID     uuid.UUID
	ActualQty       int
	ShortfallReason string
	CarryOver       bool
	// CarryOverWO is fully populated by the service (id, qty, plan_id, sku_id,
	// sales_order_line_id, parent_wo_id, status=PLANNED, created_at). The store
	// just inserts it when CarryOver=true.
	CarryOverWO WorkOrder
}

// woFeasibilityData is the slim WO projection for feasibility check.
type woFeasibilityData struct {
	WOID       uuid.UUID
	Quantity   int
	Status     string
	Deadline   *time.Time
	MaterialID uuid.UUID // primary sheet material from BOM
	SOCode     string
}

// woSuggestionRow is one candidate returned by selectFeasibilitySuggestions.
type woSuggestionRow struct {
	WOID      uuid.UUID
	SKUCode   string
	Quantity  int
	Deadline  *time.Time
	FreedQty  int
}

// setPriorityBoostOp carries the payload for setPriorityBoostAtomically.
type setPriorityBoostOp struct {
	WOID    uuid.UUID
	Reason  string
	ActorID uuid.UUID
}

// preemptCandidateRow is one row from selectPreemptCandidates.
type preemptCandidateRow struct {
	WOID      uuid.UUID
	Status    string
	SOCode    string
	Deadline  *time.Time
	Quantity  int
}

// preemptOp carries the payload for preemptAtomically.
type preemptOp struct {
	FromWOID   uuid.UUID
	ToWOID     uuid.UUID
	MaterialID uuid.UUID
	Reason     string
	ActorID    uuid.UUID
}
