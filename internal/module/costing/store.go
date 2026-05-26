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
	// selectCostingRecordsKeyset returns up to `limit` rows strictly older than
	// the (created_at, id) cursor, ordered by created_at DESC, id DESC.
	// Filters narrow by finalized flag, sku_id, created_at range, and ILIKE
	// search on sku code/name. Callers pass limit+1 to detect has_more.
	selectCostingRecordsKeyset(ctx context.Context, filter CostingListFilter, cur httpkit.Cursor, limit int) ([]CostingRecord, error)
	finalizeCostingRecord(ctx context.Context, woID uuid.UUID, actorID uuid.UUID) error
	hasCostingRecord(ctx context.Context, woID uuid.UUID) (bool, error)
	insertCostingAdjustment(ctx context.Context, a CostingAdjustment) error
	selectAdjustmentsByRecord(ctx context.Context, costingRecordID uuid.UUID) ([]CostingAdjustment, error)
	selectWasteReport(ctx context.Context, filter WasteReportFilter) ([]WasteReportRow, error)
}
