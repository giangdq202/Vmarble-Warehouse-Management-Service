package domain

import "fmt"

// ── WorkOrder status ────────────────────────────────────────

type WorkOrderStatus string

const (
	WOPlanned         WorkOrderStatus = "PLANNED"
	WOInCutting       WorkOrderStatus = "IN_CUTTING"
	WOInProcessing    WorkOrderStatus = "IN_PROCESSING"
	WOCompleted       WorkOrderStatus = "COMPLETED"
	WOPartialComplete WorkOrderStatus = "PARTIAL_COMPLETE"
	WOCosted          WorkOrderStatus = "COSTED"
	// WOCanceled marks a PLANNED work order that was cascade-cancelled when
	// its parent plan was cancelled (#249). It is a terminal state with no
	// forward transitions; AdvanceStatus refuses to leave or enter it. The
	// only way in is via planning.CancelPlan → production cascade SQL update.
	WOCanceled WorkOrderStatus = "CANCELED"
)

// woTransitions enumerates every valid forward transition. IN_PROCESSING has
// two outgoing edges (#292): COMPLETED for full production, PARTIAL_COMPLETE
// for the "8 of 10 made, 2 carry over to tomorrow" path. Both terminate at
// COSTED. The first entry of each slice is the canonical happy-path next
// state — Next() returns it for backward compatibility.
var woTransitions = map[WorkOrderStatus][]WorkOrderStatus{
	WOPlanned:         {WOInCutting},
	WOInCutting:       {WOInProcessing},
	WOInProcessing:    {WOCompleted, WOPartialComplete},
	WOCompleted:       {WOCosted},
	WOPartialComplete: {WOCosted},
}

// Next returns the canonical happy-path successor. When a state has multiple
// valid next states (IN_PROCESSING), Next returns the first listed in
// woTransitions (COMPLETED for IN_PROCESSING). Use CanTransitionTo for
// membership checks.
func (s WorkOrderStatus) Next() (WorkOrderStatus, bool) {
	nexts, ok := woTransitions[s]
	if !ok || len(nexts) == 0 {
		return "", false
	}
	return nexts[0], true
}

func (s WorkOrderStatus) CanTransitionTo(to WorkOrderStatus) error {
	nexts, ok := woTransitions[s]
	if !ok {
		return fmt.Errorf("invalid transition %s -> %s", s, to)
	}
	for _, n := range nexts {
		if n == to {
			return nil
		}
	}
	return fmt.Errorf("invalid transition %s -> %s", s, to)
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

// ── Labor cost stage ────────────────────────────────────────

// LaborStage identifies the production stage a labor cost entry applies to.
// The set is closed; new stages require a migration update to the
// labor_cost_entries CHECK constraint as well.
type LaborStage string

const (
	LaborStageCNC       LaborStage = "CNC"
	LaborStageGrinding  LaborStage = "GRINDING"
	LaborStageAssembly  LaborStage = "ASSEMBLY"
	LaborStagePolishing LaborStage = "POLISHING"
)

func (s LaborStage) Valid() bool {
	switch s {
	case LaborStageCNC, LaborStageGrinding, LaborStageAssembly, LaborStagePolishing:
		return true
	}
	return false
}
