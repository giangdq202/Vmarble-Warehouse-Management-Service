package dashboard

import "context"

type store interface {
	selectRemnantKPIs(ctx context.Context) (RemnantKPIOutput, error)
	selectUtilizationPct(ctx context.Context) (float64, error)
	selectActiveWorkOrders(ctx context.Context) (int, error)
	selectPendingCosting(ctx context.Context) (int, error)
	selectRemnantTrend7D(ctx context.Context) ([]RemnantTrendPoint, error)
	selectCostAllocation(ctx context.Context, limit int) ([]CostAllocationItem, error)
	selectMaterialUsage7D(ctx context.Context) ([]MaterialUsagePoint, error)
	selectRecentCuts(ctx context.Context, limit int) ([]RecentCutItem, error)
	selectCompletedWorkOrders(ctx context.Context, limit int) ([]RecentWorkOrderItem, error)
	selectCostingFinalizations(ctx context.Context, limit int) ([]RecentCostingFinalizationItem, error)
}
