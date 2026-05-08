package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type mockStore struct {
	remnantKPIs               RemnantKPIOutput
	remnantKPIsErr            error
	utilizationPct            float64
	utilizationPctErr         error
	activeWorkOrders          int
	activeWorkOrdersErr       error
	pendingCosting            int
	pendingCostingErr         error
	remnantTrend7D            []RemnantTrendPoint
	remnantTrend7DErr         error
	costAllocation            []CostAllocationItem
	costAllocationErr         error
	materialUsage             []MaterialUsagePoint
	materialUsageErr          error
	recentCuts                []RecentCutItem
	recentCutsErr             error
	completedWorkOrders       []RecentWorkOrderItem
	completedWorkOrdersErr    error
	costingFinalizations      []RecentCostingFinalizationItem
	costingFinalizationsErr   error
	boardStockSummary         []BoardStockSummaryItem
	boardStockSummaryErr      error

	costAllocationLimitSeen   int
	recentCutsLimitSeen       int
	completedWOLimitSeen      int
	costingFinalLimitSeen     int
}

func (m *mockStore) selectRemnantKPIs(_ context.Context) (RemnantKPIOutput, error) {
	return m.remnantKPIs, m.remnantKPIsErr
}

func (m *mockStore) selectUtilizationPct(_ context.Context) (float64, error) {
	return m.utilizationPct, m.utilizationPctErr
}

func (m *mockStore) selectActiveWorkOrders(_ context.Context) (int, error) {
	return m.activeWorkOrders, m.activeWorkOrdersErr
}

func (m *mockStore) selectPendingCosting(_ context.Context) (int, error) {
	return m.pendingCosting, m.pendingCostingErr
}

func (m *mockStore) selectRemnantTrend7D(_ context.Context) ([]RemnantTrendPoint, error) {
	return m.remnantTrend7D, m.remnantTrend7DErr
}

func (m *mockStore) selectCostAllocation(_ context.Context, limit int) ([]CostAllocationItem, error) {
	m.costAllocationLimitSeen = limit
	return m.costAllocation, m.costAllocationErr
}

func (m *mockStore) selectMaterialUsage7D(_ context.Context) ([]MaterialUsagePoint, error) {
	return m.materialUsage, m.materialUsageErr
}

func (m *mockStore) selectRecentCuts(_ context.Context, limit int) ([]RecentCutItem, error) {
	m.recentCutsLimitSeen = limit
	return m.recentCuts, m.recentCutsErr
}

func (m *mockStore) selectCompletedWorkOrders(_ context.Context, limit int) ([]RecentWorkOrderItem, error) {
	m.completedWOLimitSeen = limit
	return m.completedWorkOrders, m.completedWorkOrdersErr
}

func (m *mockStore) selectCostingFinalizations(_ context.Context, limit int) ([]RecentCostingFinalizationItem, error) {
	m.costingFinalLimitSeen = limit
	return m.costingFinalizations, m.costingFinalizationsErr
}

func (m *mockStore) selectBoardStockSummary(_ context.Context) ([]BoardStockSummaryItem, error) {
	return m.boardStockSummary, m.boardStockSummaryErr
}

func TestGetOverview_HappyPath(t *testing.T) {
	now := time.Now().UTC()
	st := &mockStore{
		remnantKPIs: RemnantKPIOutput{Total: 120, Available: 80, Allocated: 25, Consumed: 10, Waste: 5},
		utilizationPct: 73.5,
		activeWorkOrders: 12,
		pendingCosting: 3,
		remnantTrend7D: []RemnantTrendPoint{{Date: "2026-03-29", Available: 70, Allocated: 20, Waste: 3}},
		costAllocation: []CostAllocationItem{{SKUCode: "SKU-001", Cost: 4500000}},
		materialUsage: []MaterialUsagePoint{{Date: "2026-03-29", Plywood: 5, Metal: 2, Accessory: 1}},
		recentCuts: []RecentCutItem{{ID: uuid.New(), WorkOrderID: uuid.New(), SKUID: uuid.New(), SKUCode: "SKU-001", CreatedAt: now}},
		completedWorkOrders: []RecentWorkOrderItem{{ID: uuid.New(), SKUCode: "SKU-001", Status: "COMPLETED", CreatedAt: now}},
		costingFinalizations: []RecentCostingFinalizationItem{{WorkOrderID: uuid.New(), SKUCode: "SKU-001", TotalCost: 4500000, CreatedAt: now}},
	}

	svc := NewService(st)
	out, err := svc.GetOverview(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.KPI.Remnants.Total != 120 {
		t.Errorf("remnants total = %d, want 120", out.KPI.Remnants.Total)
	}
	if out.KPI.UtilizationPct != 73.5 {
		t.Errorf("utilization_pct = %v, want 73.5", out.KPI.UtilizationPct)
	}
	if out.KPI.ActiveWorkOrders != 12 {
		t.Errorf("active_work_orders = %d, want 12", out.KPI.ActiveWorkOrders)
	}
	if out.KPI.PendingCosting != 3 {
		t.Errorf("pending_costing = %d, want 3", out.KPI.PendingCosting)
	}
	if len(out.Charts.RemnantTrend7D) != 1 {
		t.Errorf("remnant trend len = %d, want 1", len(out.Charts.RemnantTrend7D))
	}
	if len(out.RecentActivity.RecentCuts) != 1 {
		t.Errorf("recent cuts len = %d, want 1", len(out.RecentActivity.RecentCuts))
	}
	if st.costAllocationLimitSeen != topCostAllocationLimit {
		t.Errorf("cost allocation limit = %d, want %d", st.costAllocationLimitSeen, topCostAllocationLimit)
	}
	if st.recentCutsLimitSeen != recentItemsLimit {
		t.Errorf("recent cuts limit = %d, want %d", st.recentCutsLimitSeen, recentItemsLimit)
	}
	if st.completedWOLimitSeen != recentItemsLimit {
		t.Errorf("completed wo limit = %d, want %d", st.completedWOLimitSeen, recentItemsLimit)
	}
	if st.costingFinalLimitSeen != recentItemsLimit {
		t.Errorf("costing finalizations limit = %d, want %d", st.costingFinalLimitSeen, recentItemsLimit)
	}
}

func TestGetOverview_SelectUtilizationError_Propagates(t *testing.T) {
	expectedErr := errors.New("utilization query failed")
	st := &mockStore{
		remnantKPIs:       RemnantKPIOutput{},
		utilizationPctErr: expectedErr,
	}

	svc := NewService(st)
	_, err := svc.GetOverview(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected utilization error to propagate, got %v", err)
	}
}

func TestGetOverview_SelectRecentCutsError_Propagates(t *testing.T) {
	expectedErr := errors.New("recent cuts query failed")
	st := &mockStore{
		remnantKPIs:         RemnantKPIOutput{},
		utilizationPct:      0,
		activeWorkOrders:    0,
		pendingCosting:      0,
		remnantTrend7D:      []RemnantTrendPoint{},
		costAllocation:      []CostAllocationItem{},
		materialUsage:       []MaterialUsagePoint{},
		recentCutsErr:       expectedErr,
		completedWorkOrders: []RecentWorkOrderItem{},
		costingFinalizations: []RecentCostingFinalizationItem{},
	}

	svc := NewService(st)
	_, err := svc.GetOverview(context.Background())
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected recent cuts error to propagate, got %v", err)
	}
}

// ── GetBoardStockSummary ──────────────────────────────────────────────────────

func TestGetBoardStockSummary_HappyPath(t *testing.T) {
	matID := uuid.New()
	st := &mockStore{
		boardStockSummary: []BoardStockSummaryItem{
			{MaterialID: matID, MaterialName: "Đá marble trắng", Available: 12, Allocated: 3, AreaMM2: 86_400_000},
		},
	}
	svc := NewService(st)
	out, err := svc.GetBoardStockSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
	if out[0].Available != 12 {
		t.Errorf("Available = %d, want 12", out[0].Available)
	}
	if out[0].AreaMM2 != 86_400_000 {
		t.Errorf("AreaMM2 = %d, want 86400000", out[0].AreaMM2)
	}
}

func TestGetBoardStockSummary_Empty_ReturnsEmptySlice(t *testing.T) {
	st := &mockStore{boardStockSummary: []BoardStockSummaryItem{}}
	svc := NewService(st)
	out, err := svc.GetBoardStockSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(out) != 0 {
		t.Errorf("expected 0 items, got %d", len(out))
	}
}

func TestGetBoardStockSummary_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("db error")
	st := &mockStore{boardStockSummaryErr: storeErr}
	svc := NewService(st)
	_, err := svc.GetBoardStockSummary(context.Background())
	if !errors.Is(err, storeErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}
