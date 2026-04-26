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
	CreatedAt      time.Time              `json:"created_at"`
}

type WorkOrderListFilter struct {
	Status      string     `json:"status,omitempty"`
	PlanID      *uuid.UUID `json:"plan_id,omitempty"`
	CreatedFrom *time.Time `json:"created_from,omitempty"`
	CreatedTo   *time.Time `json:"created_to,omitempty"` // exclusive upper bound
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
type AdvanceStatusInput struct {
	To         domain.WorkOrderStatus `json:"status"`
	SheetID    *uuid.UUID             `json:"sheet_id,omitempty"`
	CallerID   *uuid.UUID             `json:"-"` // populated by handler from JWT claims, not from request body
	CallerRole auth.Role              `json:"-"` // populated by handler from JWT claims, not from request body
}

type SuggestAssignmentResult struct {
	UserID         uuid.UUID `json:"user_id"`
	InCuttingCount int       `json:"in_cutting_count"`
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
}
