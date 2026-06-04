package planning

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

// WorkOrderCanceller bridges planning → production for cascade cancel.
// Implementation lives in the production module; wired in main.go.
//
// CancelPlan first asks ListStatusesByPlan to verify no work order has
// progressed past PLANNED, then calls CancelPlannedByPlan to set every
// PLANNED row to CANCELED in a single SQL update.
type WorkOrderCanceller interface {
	// ListStatusesByPlan returns the current status of every work order tied
	// to the plan. Used as a precondition check before cancel.
	ListStatusesByPlan(ctx context.Context, planID uuid.UUID) ([]domain.WorkOrderStatus, error)
	// CancelPlannedByPlan flips every PLANNED work order under the plan to
	// CANCELED in one statement and returns the affected row count. Rows in
	// any other status are left untouched (they were validated upstream).
	CancelPlannedByPlan(ctx context.Context, planID uuid.UUID) (int64, error)
}

// WorkOrderAdvisor bridges planning → production for smart re-allocation (BE #2).
// Implementation lives in the production module; wired in main.go.
type WorkOrderAdvisor interface {
	// CheckFeasibility checks whether woID has sufficient AVAILABLE sheets.
	// Returns feasible=true when stock covers the WO quantity; otherwise
	// feasible=false with up to 5 scored alternative suggestions (BR-PL01/02/03).
	CheckFeasibility(ctx context.Context, woID uuid.UUID) (FeasibilityResult, error)

	// BoostPriority sets priority_boost=true on the work order and appends
	// an audit row. Requires planner role (enforced at handler). (BR-PL05)
	BoostPriority(ctx context.Context, in BoostPriorityInput) (BoostPriorityResult, error)

	// ListPreemptCandidates returns PLANNED work orders that could free
	// materials for woID, ordered by preemption safety score. (BR-PL06/07)
	ListPreemptCandidates(ctx context.Context, woID uuid.UUID) ([]PreemptCandidate, error)

	// PreemptWorkOrder atomically reverts from_wo to PLANNED and logs the
	// preemption. Refuses if from_wo has progressed past IN_CUTTING (BR-PL07).
	PreemptWorkOrder(ctx context.Context, in PreemptInput) (PreemptResult, error)
}

// FeasibilityResult is returned by CheckFeasibility.
type FeasibilityResult struct {
	Feasible    bool                  `json:"feasible"`
	Reason      string                `json:"reason,omitempty"`
	Suggestions []FeasibilitySuggestion `json:"suggestions,omitempty"`
}

// FeasibilitySuggestion is one scored alternative WO (BR-PL02/03).
type FeasibilitySuggestion struct {
	WOID       uuid.UUID `json:"wo_id"`
	SKUCode    string    `json:"sku_code"`
	Score      float64   `json:"score"`
	DaysToDue  int       `json:"days_to_due"`
	FreedQty   int       `json:"freed_qty"`
}

// BoostPriorityInput carries the parameters for BoostPriority.
type BoostPriorityInput struct {
	WOID    uuid.UUID `json:"-"`
	Reason  string    `json:"reason"`
	ActorID uuid.UUID `json:"-"`
}

// BoostPriorityResult is returned by BoostPriority.
type BoostPriorityResult struct {
	BoostedAt time.Time `json:"boosted_at"`
	AuditID   uuid.UUID `json:"audit_id"`
}

// PreemptCandidate is one work order that can be preempted to free materials.
type PreemptCandidate struct {
	WOID          uuid.UUID `json:"wo_id"`
	Status        string    `json:"status"`
	CurrentSOCode string    `json:"current_so_code,omitempty"`
	SlackDays     int       `json:"slack_days"`
	FreedQty      int       `json:"freed_qty"`
}

// PreemptInput carries the parameters for PreemptWorkOrder.
type PreemptInput struct {
	ToWOID   uuid.UUID `json:"-"`
	FromWOID uuid.UUID `json:"from_wo_id"`
	Reason   string    `json:"reason"`
	ActorID  uuid.UUID `json:"-"`
}

// PreemptResult is returned by PreemptWorkOrder.
type PreemptResult struct {
	PreemptedAt time.Time `json:"preempted_at"`
	AuditID     uuid.UUID `json:"audit_id"`
	FreedQty    int       `json:"freed_qty"`
}
