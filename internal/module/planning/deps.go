package planning

import (
	"context"

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
