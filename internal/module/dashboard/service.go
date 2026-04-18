package dashboard

import "context"

const (
	topCostAllocationLimit = 10
	recentItemsLimit       = 10
)

type service struct {
	s store
}

func NewService(s store) Service {
	return &service{s: s}
}

func (svc *service) GetOverview(ctx context.Context) (OverviewOutput, error) {
	remnants, err := svc.s.selectRemnantKPIs(ctx)
	if err != nil {
		return OverviewOutput{}, err
	}
	utilizationPct, err := svc.s.selectUtilizationPct(ctx)
	if err != nil {
		return OverviewOutput{}, err
	}
	activeWorkOrders, err := svc.s.selectActiveWorkOrders(ctx)
	if err != nil {
		return OverviewOutput{}, err
	}
	pendingCosting, err := svc.s.selectPendingCosting(ctx)
	if err != nil {
		return OverviewOutput{}, err
	}
	remnantTrend7D, err := svc.s.selectRemnantTrend7D(ctx)
	if err != nil {
		return OverviewOutput{}, err
	}
	costAllocation, err := svc.s.selectCostAllocation(ctx, topCostAllocationLimit)
	if err != nil {
		return OverviewOutput{}, err
	}
	materialUsage, err := svc.s.selectMaterialUsage7D(ctx)
	if err != nil {
		return OverviewOutput{}, err
	}
	recentCuts, err := svc.s.selectRecentCuts(ctx, recentItemsLimit)
	if err != nil {
		return OverviewOutput{}, err
	}
	completedWOs, err := svc.s.selectCompletedWorkOrders(ctx, recentItemsLimit)
	if err != nil {
		return OverviewOutput{}, err
	}
	costingFinalizations, err := svc.s.selectCostingFinalizations(ctx, recentItemsLimit)
	if err != nil {
		return OverviewOutput{}, err
	}

	return OverviewOutput{
		KPI: KPIOutput{
			Remnants:         remnants,
			UtilizationPct:   utilizationPct,
			ActiveWorkOrders: activeWorkOrders,
			PendingCosting:   pendingCosting,
		},
		Charts: ChartsOutput{
			RemnantTrend7D: remnantTrend7D,
			CostAllocation: costAllocation,
			MaterialUsage:  materialUsage,
		},
		RecentActivity: RecentActivityOutput{
			RecentCuts:           recentCuts,
			CompletedWorkOrders:  completedWOs,
			CostingFinalizations: costingFinalizations,
		},
	}, nil
}
