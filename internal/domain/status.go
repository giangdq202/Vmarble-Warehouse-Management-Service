package domain

import "fmt"

// ── WorkOrder status ────────────────────────────────────────

type WorkOrderStatus string

const (
	WOPlanned      WorkOrderStatus = "PLANNED"
	WOInCutting    WorkOrderStatus = "IN_CUTTING"
	WOInProcessing WorkOrderStatus = "IN_PROCESSING"
	WOCompleted    WorkOrderStatus = "COMPLETED"
	WOCosted       WorkOrderStatus = "COSTED"
)

var woTransitions = map[WorkOrderStatus]WorkOrderStatus{
	WOPlanned:      WOInCutting,
	WOInCutting:    WOInProcessing,
	WOInProcessing: WOCompleted,
	WOCompleted:    WOCosted,
}

func (s WorkOrderStatus) Next() (WorkOrderStatus, bool) {
	next, ok := woTransitions[s]
	return next, ok
}

func (s WorkOrderStatus) CanTransitionTo(to WorkOrderStatus) error {
	next, ok := woTransitions[s]
	if !ok || next != to {
		return fmt.Errorf("invalid transition %s -> %s", s, to)
	}
	return nil
}

// ── Remnant status ──────────────────────────────────────────

type RemnantStatus string

const (
	RemnantAvailable RemnantStatus = "AVAILABLE"
	RemnantAllocated RemnantStatus = "ALLOCATED"
	RemnantConsumed  RemnantStatus = "CONSUMED"
	RemnantWaste     RemnantStatus = "WASTE"
)

// ── Production Plan status ──────────────────────────────────

type PlanStatus string

const (
	PlanDraft    PlanStatus = "DRAFT"
	PlanApproved PlanStatus = "APPROVED"
	PlanCanceled PlanStatus = "CANCELED"
)
