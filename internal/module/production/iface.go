package production

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type CreateWOInput struct {
	PlanID   uuid.UUID `json:"plan_id"`
	SKUID    uuid.UUID `json:"sku_id"`
	Quantity int       `json:"quantity"`
	// BypassReason is an optional planner note recorded on the REMNANT_BYPASSED
	// audit row when the work order is created without allocating any of the
	// fitting remnant suggestions (BR-K05).
	BypassReason string `json:"bypass_reason,omitempty"`
	// CallerID is the planner's user id, populated by the handler from JWT
	// claims (not accepted from the request body). Used as the actor for the
	// REMNANT_BYPASSED audit row.
	CallerID *uuid.UUID `json:"-"`
	// SalesOrderLineID, when set, links the WO back to a sales_order_lines row
	// for traceability. Populated by the sales module's split-to-plan adapter;
	// nil for legacy/PO-rooted plans. Carry-over WOs (#292) inherit this from
	// the parent.
	SalesOrderLineID *uuid.UUID `json:"-"`
}

type WorkOrder struct {
	ID             uuid.UUID              `json:"id"`
	PlanID         uuid.UUID              `json:"plan_id"`
	SKUID          uuid.UUID              `json:"sku_id"`
	SKUCode        string                 `json:"sku_code"`
	SKUName        string                 `json:"sku_name"`
	SKUDimensions  domain.Dimension       `json:"sku_dimensions"`
	Quantity       int                    `json:"quantity"`
	Status         domain.WorkOrderStatus `json:"status"`
	AssignedTo     *uuid.UUID             `json:"assigned_to,omitempty"`
	AssignedAt     *time.Time             `json:"assigned_at,omitempty"`
	EstimatedHours *float64               `json:"estimated_hours,omitempty"`
	MachineSlotID  *uuid.UUID             `json:"machine_slot_id,omitempty"`
	// SalesOrderLineID, when set, links the WO back to a sales_order_lines row
	// (Phase A pivot). Nullable so legacy/PO-rooted WOs read fine without it.
	SalesOrderLineID *uuid.UUID `json:"sales_order_line_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type WorkOrderListFilter struct {
	Status      string     `json:"status,omitempty"`
	PlanID      *uuid.UUID `json:"plan_id,omitempty"`
	CreatedFrom *time.Time `json:"created_from,omitempty"`
	CreatedTo   *time.Time `json:"created_to,omitempty"` // exclusive upper bound

	// AssignedNull, when true, filters to work orders with assigned_to IS NULL
	// (i.e. unassigned). Mutually exclusive with AssignedTo.
	AssignedNull bool
	// AssignedTo, when set, filters to work orders assigned to a specific user.
	// Mutually exclusive with AssignedNull.
	AssignedTo *uuid.UUID

	// DashboardPreset enables the operational queue: PLANNED-today first, then
	// PLANNED-yesterday, then active (IN_CUTTING/IN_PROCESSING), then older PLANNED.
	// COMPLETED and COSTED records are excluded. Mutually exclusive with Status /
	// CreatedFrom / CreatedTo filters.
	// TodayStart and TodayEnd must span [start-of-today, start-of-tomorrow) in
	// the caller's chosen timezone (Asia/Ho_Chi_Minh for production handlers).
	DashboardPreset bool
	TodayStart      time.Time
	TodayEnd        time.Time
}

// Machine represents a CNC machine that can be scheduled for work orders.
type Machine struct {
	ID                    uuid.UUID `json:"id"`
	Code                  string    `json:"code"`
	Name                  string    `json:"name"`
	CapacityHoursPerShift float64   `json:"capacity_hours_per_shift"`
	IsActive              bool      `json:"is_active"`
	CreatedAt             time.Time `json:"created_at"`
}

// CreateMachineInput carries the fields required to register a new machine.
type CreateMachineInput struct {
	Code                  string  `json:"code"`
	Name                  string  `json:"name"`
	CapacityHoursPerShift float64 `json:"capacity_hours_per_shift"`
}

// MachineShiftSlot is a concrete scheduled block: one machine on one date in one named shift.
// AssignedHours is computed at query time (SUM of estimated_hours of assigned WOs).
type MachineShiftSlot struct {
	ID            uuid.UUID `json:"id"`
	MachineID     uuid.UUID `json:"machine_id"`
	MachineCode   string    `json:"machine_code"`
	MachineName   string    `json:"machine_name"`
	ShiftDate     time.Time `json:"shift_date"`
	ShiftName     string    `json:"shift_name"`
	CapacityHours float64   `json:"capacity_hours"`
	AssignedHours float64   `json:"assigned_hours"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreateSlotInput carries parameters to create a machine shift slot.
// CapacityHours defaults to the machine's CapacityHoursPerShift when omitted.
type CreateSlotInput struct {
	MachineID     uuid.UUID `json:"-"`
	ShiftDate     time.Time `json:"shift_date"`
	ShiftName     string    `json:"shift_name"`
	CapacityHours *float64  `json:"capacity_hours,omitempty"`
}

// AssignSlotInput carries the slot to assign to a work order.
type AssignSlotInput struct {
	WorkOrderID uuid.UUID `json:"-"`
	SlotID      uuid.UUID `json:"slot_id"`
}

// SetEstimatedHoursInput updates the estimated machine-hours for a work order.
type SetEstimatedHoursInput struct {
	WorkOrderID    uuid.UUID `json:"-"`
	EstimatedHours float64   `json:"estimated_hours"`
}

// ScheduleSuggestion pairs an available slot with its rank and available capacity.
type ScheduleSuggestion struct {
	Slot           MachineShiftSlot `json:"slot"`
	AvailableHours float64          `json:"available_hours"`
	Rank           int              `json:"rank"`
}

type RecordConsumptionInput struct {
	WorkOrderID  uuid.UUID `json:"work_order_id"`
	MaterialID   uuid.UUID `json:"material_id"`
	MaterialType string    `json:"material_type"`
	Quantity     float64   `json:"quantity"`
	Unit         string    `json:"unit"`
}

type ConsumptionRecord struct {
	ID           uuid.UUID `json:"id"`
	WorkOrderID  uuid.UUID `json:"work_order_id"`
	MaterialID   uuid.UUID `json:"material_id"`
	MaterialType string    `json:"material_type"`
	Quantity     float64   `json:"quantity"`
	Unit         string    `json:"unit"`
	CreatedAt    time.Time `json:"created_at"`
}

type AssignWorkOrderInput struct {
	WorkOrderID uuid.UUID
	UserID      uuid.UUID
}

// AdvanceStatusInput is the request to advance a work order's status.
// SheetID is optional — when provided and the target status is IN_CUTTING,
// the sheet will be pre-assigned to the work order before the transition.
// CallerID is optional — when provided and the target status is IN_CUTTING,
// the caller must be the assigned CNC operator for this work order unless the
// caller role is admin (super-admin override).
// BypassOverflow=true asks the inventory module to skip the remnant-overflow
// guard when issuing a new sheet. It is honoured only for callers with the
// admin role and requires a non-empty BypassReason; the bypass is audit-logged.
type AdvanceStatusInput struct {
	To             domain.WorkOrderStatus `json:"status"`
	SheetID        *uuid.UUID             `json:"sheet_id,omitempty"`
	BypassOverflow bool                   `json:"bypass_overflow,omitempty"`
	BypassReason   string                 `json:"bypass_reason,omitempty"`
	CallerID       *uuid.UUID             `json:"-"` // populated by handler from JWT claims, not from request body
	CallerRole     auth.Role              `json:"-"` // populated by handler from JWT claims, not from request body
}

type SuggestAssignmentResult struct {
	UserID         uuid.UUID `json:"user_id"`
	InCuttingCount int       `json:"in_cutting_count"`
}

// RecordLaborEntryInput carries one labor cost line for a work order.
// RatePerHour is in the smallest currency unit (e.g. VND dong per hour).
//
// ActorID is the recorder (foreman / cnc_manager who logged the entry via the
// UI). It is populated by the handler from JWT claims and is never read from
// the request body.
//
// WorkerID is optional and identifies the person whose time is being recorded
// when that differs from the recorder (typical case: one foreman logging
// end-of-shift on behalf of a whole crew). When omitted, the worker is
// assumed to be the caller — the entry is attributed to ActorID.
type RecordLaborEntryInput struct {
	WorkOrderID uuid.UUID         `json:"-"`
	Stage       domain.LaborStage `json:"stage"`
	Minutes     int               `json:"minutes"`
	RatePerHour int64             `json:"rate_per_hour"`
	WorkerID    *uuid.UUID        `json:"worker_id,omitempty"`
	ActorID     uuid.UUID         `json:"-"`
}

// LaborEntry is a single recorded labor cost line for a work order.
// Cost contribution = minutes * rate_per_hour / 60 (computed by the costing module).
//
// WorkerID identifies the person who actually performed the work; ActorID
// identifies the user who recorded the entry. Frontend can render "X recorded
// by Y" by comparing the two fields. For legacy rows recorded before the
// worker_id field existed, WorkerID echoes ActorID so the API contract stays
// uniform.
type LaborEntry struct {
	ID          uuid.UUID         `json:"id"`
	WorkOrderID uuid.UUID         `json:"work_order_id"`
	Stage       domain.LaborStage `json:"stage"`
	Minutes     int               `json:"minutes"`
	RatePerHour int64             `json:"rate_per_hour"`
	WorkerID    uuid.UUID         `json:"worker_id"`
	ActorID     uuid.UUID         `json:"actor_id"`
	CreatedAt   time.Time         `json:"created_at"`
}

type Service interface {
	CreateWorkOrder(ctx context.Context, in CreateWOInput) (WorkOrder, error)
	GetWorkOrder(ctx context.Context, woID uuid.UUID) (WorkOrder, error)
	ListWorkOrders(ctx context.Context, p httpkit.PageParams, f WorkOrderListFilter) (httpkit.PagedResult[WorkOrder], error)
	ListWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error)
	ListWorkOrdersByAssignee(ctx context.Context, userID uuid.UUID) ([]WorkOrder, error)
	AdvanceStatus(ctx context.Context, woID uuid.UUID, in AdvanceStatusInput) error
	RecordConsumption(ctx context.Context, in RecordConsumptionInput) (ConsumptionRecord, error)
	ListConsumptions(ctx context.Context, woID uuid.UUID) ([]ConsumptionRecord, error)
	AssignWorkOrder(ctx context.Context, in AssignWorkOrderInput) (WorkOrder, error)
	SuggestAssignment(ctx context.Context, woID uuid.UUID) (SuggestAssignmentResult, error)

	// Machine management
	CreateMachine(ctx context.Context, in CreateMachineInput) (Machine, error)
	ListMachines(ctx context.Context) ([]Machine, error)
	GetMachine(ctx context.Context, machineID uuid.UUID) (Machine, error)
	DeactivateMachine(ctx context.Context, machineID uuid.UUID) error

	// Shift slot management
	CreateSlot(ctx context.Context, in CreateSlotInput) (MachineShiftSlot, error)
	ListSlots(ctx context.Context, machineID uuid.UUID, from, to time.Time) ([]MachineShiftSlot, error)
	GetSlot(ctx context.Context, slotID uuid.UUID) (MachineShiftSlot, error)
	DeleteSlot(ctx context.Context, slotID uuid.UUID) error

	// Work order scheduling
	SetEstimatedHours(ctx context.Context, in SetEstimatedHoursInput) (WorkOrder, error)
	AssignSlot(ctx context.Context, in AssignSlotInput) (WorkOrder, error)
	UnassignSlot(ctx context.Context, woID uuid.UUID) (WorkOrder, error)
	SuggestSchedule(ctx context.Context, woID uuid.UUID) ([]ScheduleSuggestion, error)

	// Labor cost entries (Pillar C — Actual Costing)
	// RecordLaborEntry adds one per-stage labor cost line for a work order.
	// Rejected with ErrAlreadyFinalized when the WO's costing record is locked.
	RecordLaborEntry(ctx context.Context, in RecordLaborEntryInput) (LaborEntry, error)
	// ListLaborEntries returns all labor entries for a work order, ordered by created_at ASC.
	ListLaborEntries(ctx context.Context, woID uuid.UUID) ([]LaborEntry, error)
	// SumLaborCost returns the total labor cost for a work order, computed as
	// SUM(minutes * rate_per_hour / 60). Used by the costing module via deps.
	SumLaborCost(ctx context.Context, woID uuid.UUID) (domain.Money, error)

	// Plan cascade cancel (#249)
	// ListStatusesByPlan returns every work order's status for the given plan.
	// Used by planning.CancelPlan to verify no WO has progressed past PLANNED.
	ListStatusesByPlan(ctx context.Context, planID uuid.UUID) ([]domain.WorkOrderStatus, error)
	// CancelPlannedByPlan flips every PLANNED work order under the plan to
	// CANCELED in a single SQL UPDATE and returns the affected row count.
	// Intended only for the planning cascade-cancel flow.
	CancelPlannedByPlan(ctx context.Context, planID uuid.UUID) (int64, error)
}
