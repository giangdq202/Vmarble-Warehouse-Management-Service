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

	wipPipeline               []WIPStageRow
	wipPipelineErr            error
	wipPipelineWindowSeen     time.Duration

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

func (m *mockStore) selectWIPPipeline(_ context.Context, window time.Duration) ([]WIPStageRow, error) {
	m.wipPipelineWindowSeen = window
	return m.wipPipeline, m.wipPipelineErr
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

func TestGetWIPPipeline_FillsMissingStagesInCanonicalOrder(t *testing.T) {
	oldest := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	store := &mockStore{
		wipPipeline: []WIPStageRow{
			{Status: "IN_CUTTING", Count: 3, OldestStartedAt: &oldest, AtRiskCount: 1},
			{Status: "COSTED", Count: 2, AtRiskCount: 0},
		},
	}
	svc := NewService(store)

	out, err := svc.GetWIPPipeline(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantOrder := []string{"PLANNED", "IN_CUTTING", "IN_PROCESSING", "COMPLETED", "COSTED"}
	if len(out.Stages) != len(wantOrder) {
		t.Fatalf("len(stages) = %d, want %d", len(out.Stages), len(wantOrder))
	}
	for i, want := range wantOrder {
		if out.Stages[i].Status != want {
			t.Errorf("Stages[%d].Status = %q, want %q", i, out.Stages[i].Status, want)
		}
	}

	if out.Stages[0].Count != 0 || out.Stages[0].AtRiskCount != 0 || out.Stages[0].OldestStartedAt != nil {
		t.Errorf("PLANNED stage should be zeroed, got %+v", out.Stages[0])
	}
	if out.Stages[1].Count != 3 || out.Stages[1].AtRiskCount != 1 {
		t.Errorf("IN_CUTTING stage mismatch: %+v", out.Stages[1])
	}
	if out.Stages[1].OldestStartedAt == nil || !out.Stages[1].OldestStartedAt.Equal(oldest) {
		t.Errorf("IN_CUTTING OldestStartedAt = %v, want %v", out.Stages[1].OldestStartedAt, oldest)
	}
	if out.Stages[4].Count != 2 {
		t.Errorf("COSTED stage Count = %d, want 2", out.Stages[4].Count)
	}
}

func TestGetWIPPipeline_PassesAtRiskWindow(t *testing.T) {
	store := &mockStore{}
	svc := NewService(store)

	if _, err := svc.GetWIPPipeline(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.wipPipelineWindowSeen != wipAtRiskWindow {
		t.Errorf("window = %v, want %v", store.wipPipelineWindowSeen, wipAtRiskWindow)
	}
}

func TestGetWIPPipeline_StorePropagatesError(t *testing.T) {
	storeErr := errors.New("boom")
	svc := NewService(&mockStore{wipPipelineErr: storeErr})

	_, err := svc.GetWIPPipeline(context.Background())
	if !errors.Is(err, storeErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

func TestGetWIPPipeline_UnknownStatusInStoreOutput_IsDropped(t *testing.T) {
	store := &mockStore{
		wipPipeline: []WIPStageRow{
			{Status: "PLANNED", Count: 2},
			{Status: "MYSTERY", Count: 99},
		},
	}
	svc := NewService(store)

	out, err := svc.GetWIPPipeline(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Stages) != 5 {
		t.Fatalf("len = %d, want 5 canonical stages", len(out.Stages))
	}
	for _, s := range out.Stages {
		if s.Status == "MYSTERY" {
			t.Errorf("unknown status leaked into response: %+v", s)
		}
	}
	if out.Stages[0].Count != 2 {
		t.Errorf("PLANNED count = %d, want 2", out.Stages[0].Count)
	}
}
