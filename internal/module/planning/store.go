package planning

import (
	"context"

	"github.com/google/uuid"
)

type store interface {
	insertPlan(ctx context.Context, p Plan) error
	selectPlans(ctx context.Context) ([]Plan, error)
	selectPlanByID(ctx context.Context, id uuid.UUID) (Plan, error)
	updatePlanStatus(ctx context.Context, id uuid.UUID, status string) error
	insertPlanItems(ctx context.Context, items []PlanItem) error
	selectPlanItemsByPlanID(ctx context.Context, planID uuid.UUID) ([]PlanItem, error)
}
