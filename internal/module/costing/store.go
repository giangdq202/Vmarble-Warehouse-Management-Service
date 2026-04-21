package costing

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertCostingRecord(ctx context.Context, r CostingRecord) error
	updateCostingRecord(ctx context.Context, r CostingRecord) error
	selectCostingRecordByWO(ctx context.Context, woID uuid.UUID) (CostingRecord, error)
	selectCostingRecordsPaged(ctx context.Context, p httpkit.PageParams, finalized *bool) ([]CostingRecord, int, error)
	finalizeCostingRecord(ctx context.Context, woID uuid.UUID, actorID uuid.UUID) error
	insertCostingAdjustment(ctx context.Context, a CostingAdjustment) error
	selectAdjustmentsByRecord(ctx context.Context, costingRecordID uuid.UUID) ([]CostingAdjustment, error)
}
