package planning

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	// nextPlanCode generates the next unique human-readable code (e.g. KH-2026-001).
	// It draws from the production_plan_code_seq sequence so concurrent inserts are safe.
	nextPlanCode(ctx context.Context, year int) (string, error)
	insertPlan(ctx context.Context, p Plan) error
	selectPlansPaged(ctx context.Context, p httpkit.PageParams, status string) ([]Plan, int, error) // search uses p.Search against plan code and PO code
	selectPlansLookup(ctx context.Context, search, status string, deadlineFrom, deadlineTo *time.Time, limit, offset int) ([]PlanLookupItem, int, error)
	selectPlanByID(ctx context.Context, id uuid.UUID) (Plan, error)
	updatePlanStatus(ctx context.Context, id uuid.UUID, status string) error
	insertPlanItems(ctx context.Context, items []PlanItem) error
	selectPlanItemsByPlanID(ctx context.Context, planID uuid.UUID) ([]PlanItem, error)
}
