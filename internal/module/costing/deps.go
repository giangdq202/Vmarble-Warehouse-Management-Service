package costing

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type WorkOrderReader interface {
	GetWorkOrder(ctx context.Context, woID uuid.UUID) (WOInfo, error)
}

type WOInfo struct {
	ID     uuid.UUID
	SKUID  uuid.UUID
	Status domain.WorkOrderStatus
}

type CuttingDataReader interface {
	GetCuttingDataForWO(ctx context.Context, woID uuid.UUID) ([]CuttingData, error)
}

type CuttingData struct {
	SheetCost    domain.Money
	SheetAreaMM2 int64
	UsedAreaMM2  int64
}

type ConsumptionDataReader interface {
	GetConsumptionCostForWO(ctx context.Context, woID uuid.UUID) (domain.Money, error)
}
