package production

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertWorkOrder(ctx context.Context, wo WorkOrder) error
	selectWorkOrdersPaged(ctx context.Context, p httpkit.PageParams, status string) ([]WorkOrder, int, error)
	selectWorkOrderByID(ctx context.Context, id uuid.UUID) (WorkOrder, error)
	selectWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error)
	selectWorkOrdersByAssignee(ctx context.Context, userID uuid.UUID) ([]WorkOrder, error)
	updateWorkOrderStatus(ctx context.Context, id uuid.UUID, status string) error
	updateWorkOrderAssignment(ctx context.Context, woID uuid.UUID, userID uuid.UUID, assignedAt time.Time) error
	insertConsumption(ctx context.Context, cr ConsumptionRecord) error
	selectConsumptionsByWO(ctx context.Context, woID uuid.UUID) ([]ConsumptionRecord, error)
	hasMetalConsumption(ctx context.Context, woID uuid.UUID) (bool, error)
	selectInCuttingCountByUser(ctx context.Context) (map[uuid.UUID]int, error)
	selectCNCUserIDs(ctx context.Context) ([]uuid.UUID, error)
}
