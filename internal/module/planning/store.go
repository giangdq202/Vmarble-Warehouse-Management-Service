package planning

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertPlan(ctx context.Context, p Plan) error
	selectPlansPaged(ctx context.Context, p httpkit.PageParams, status string) ([]Plan, int, error)
	selectPlanByID(ctx context.Context, id uuid.UUID) (Plan, error)
	updatePlanStatus(ctx context.Context, id uuid.UUID, status string) error
	insertPlanItems(ctx context.Context, items []PlanItem) error
	selectPlanItemsByPlanID(ctx context.Context, planID uuid.UUID) ([]PlanItem, error)
}
