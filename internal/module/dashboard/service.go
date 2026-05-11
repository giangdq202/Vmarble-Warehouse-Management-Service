package dashboard

import (
	"context"
	"time"

	"github.com/vmarble/warehouse-management-service/internal/domain"
)

const (
	topCostAllocationLimit = 10
	recentItemsLimit       = 10
	// wipAtRiskWindow defines when a work order is flagged "at risk":
	// production plan deadline within the next 2 days and status not yet
	// COMPLETED. Per issue #226 DoD.
	wipAtRiskWindow = 48 * time.Hour
)

// canonicalWIPStages lists every work-order status in state-machine order so
// the response always carries one row per stage, even those with zero work
// orders. This keeps the dashboard rendering stable.
var canonicalWIPStages = []domain.WorkOrderStatus{
	domain.WOPlanned,
	domain.WOInCutting,
	domain.WOInProcessing,
	domain.WOCompleted,
	domain.WOCosted,
}

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

func (s *service) GetBoardStockSummary(ctx context.Context) ([]BoardStockSummaryItem, error) {
	items, err := s.s.selectBoardStockSummary(ctx)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *service) GetWIPPipeline(ctx context.Context) (WIPPipelineOutput, error) {
	rows, err := s.s.selectWIPPipeline(ctx, wipAtRiskWindow)
	if err != nil {
		return WIPPipelineOutput{}, err
	}

	// Build a status → row lookup, then materialise the response in canonical
	// state-machine order so the dashboard column layout is stable across calls.
	byStatus := make(map[string]WIPStageRow, len(rows))
	for _, r := range rows {
		byStatus[r.Status] = r
	}
	out := WIPPipelineOutput{Stages: make([]WIPStageRow, 0, len(canonicalWIPStages))}
	for _, st := range canonicalWIPStages {
		key := string(st)
		if row, ok := byStatus[key]; ok {
			row.Status = key
			out.Stages = append(out.Stages, row)
			continue
		}
		out.Stages = append(out.Stages, WIPStageRow{Status: key})
	}
	return out, nil
}
