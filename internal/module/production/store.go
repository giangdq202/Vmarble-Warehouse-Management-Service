package production

import (
	"context"

	"github.com/google/uuid"
)

type store interface {
	insertWorkOrder(ctx context.Context, wo WorkOrder) error
	selectWorkOrders(ctx context.Context) ([]WorkOrder, error)
	selectWorkOrderByID(ctx context.Context, id uuid.UUID) (WorkOrder, error)
	selectWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error)
	updateWorkOrderStatus(ctx context.Context, id uuid.UUID, status string) error
	insertConsumption(ctx context.Context, cr ConsumptionRecord) error
	selectConsumptionsByWO(ctx context.Context, woID uuid.UUID) ([]ConsumptionRecord, error)
	hasMetalConsumption(ctx context.Context, woID uuid.UUID) (bool, error)
}
