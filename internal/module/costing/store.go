package costing

import (
	"context"

	"github.com/google/uuid"
)

type store interface {
	insertCostingRecord(ctx context.Context, r CostingRecord) error
	updateCostingRecord(ctx context.Context, r CostingRecord) error
	selectCostingRecordByWO(ctx context.Context, woID uuid.UUID) (CostingRecord, error)
	selectCostingRecords(ctx context.Context) ([]CostingRecord, error)
	finalizeCostingRecord(ctx context.Context, woID uuid.UUID) error
}
