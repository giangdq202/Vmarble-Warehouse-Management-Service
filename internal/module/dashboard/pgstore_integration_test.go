//go:build integration

package dashboard

import (
	"context"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/testhelper"
)

var (
	sharedPool *pgxpool.Pool
	setupOnce  sync.Once
)

func getPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	setupOnce.Do(func() {
		sharedPool = testhelper.StartTestDB(t)
	})
	return sharedPool
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func truncateDashboard(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	testhelper.TruncateAll(t, pool)
}

type overviewFixture struct {
	expectedDate        string
	latestCutID         uuid.UUID
	latestCompletedWOID uuid.UUID
	latestCostedWOID    uuid.UUID
}

func seedOverviewFixture(t *testing.T, pool *pgxpool.Pool) overviewFixture {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	day := now.Add(-24 * time.Hour)

	poID := uuid.New()
	planID := uuid.New()

	skuA := uuid.New()
	skuB := uuid.New()

	woPlanned := uuid.New()
	woCutting := uuid.New()
	woProcessing := uuid.New()
	woCompleted := uuid.New()
	woCostedA := uuid.New()
	woCostedB := uuid.New()

	matPlywood := uuid.New()
	matMetal := uuid.New()
	matAccessory := uuid.New()

	lotID := uuid.New()
	sheet1 := uuid.New()
	sheet2 := uuid.New()

	cutOld := uuid.New()
	cutNew := uuid.New()

	remAvailable := uuid.New()
	remAllocated := uuid.New()
	remConsumed := uuid.New()
	remWaste := uuid.New()

	costedNewer := uuid.New()
	costedOlder := uuid.New()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO materials (id, type, name, unit, created_at)
		 VALUES ($1, $2, $3, $4, $5),
		        ($6, $7, $8, $9, $10),
		        ($11, $12, $13, $14, $15)`,
		matPlywood, "PLYWOOD", "Plywood", "sheet", now,
		matMetal, "METAL", "Metal", "kg", now,
		matAccessory, "OTHER", "Glue", "kg", now,
	); err != nil {
		t.Fatalf("seed materials: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO skus (id, code, name, length_mm, width_mm, requires_metal, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7),
		        ($8, $9, $10, $11, $12, $13, $14)`,
		skuA, "SKU-A", "SKU A", 1000, 500, false, now,
		skuB, "SKU-B", "SKU B", 1200, 600, false, now,
	); err != nil {
		t.Fatalf("seed skus: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO purchase_orders (id, code, expected_delivery, created_at)
		 VALUES ($1, $2, $3, $4)`,
		poID, "PO-DASH-001", now.AddDate(0, 0, 7), now,
	); err != nil {
		t.Fatalf("seed purchase order: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO production_plans (id, po_id, status, deadline, created_at, code)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		planID, poID, domain.PlanApproved, now.AddDate(0, 0, 10), now, "KH-2026-901",
	); err != nil {
		t.Fatalf("seed production plan: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO work_orders (id, plan_id, sku_id, quantity, status, created_at)
		 VALUES
		   ($1, $2, $3, $4, $5, $6),
		   ($7, $8, $9, $10, $11, $12),
		   ($13, $14, $15, $16, $17, $18),
		   ($19, $20, $21, $22, $23, $24),
		   ($25, $26, $27, $28, $29, $30),
		   ($31, $32, $33, $34, $35, $36)`,
		woPlanned, planID, skuA, 1, domain.WOPlanned, now.Add(-2*time.Hour),
		woCutting, planID, skuA, 1, domain.WOInCutting, now.Add(-110*time.Minute),
		woProcessing, planID, skuB, 1, domain.WOInProcessing, now.Add(-100*time.Minute),
		woCompleted, planID, skuA, 1, domain.WOCompleted, now.Add(-5*time.Minute),
		woCostedA, planID, skuA, 1, domain.WOCosted, now.Add(-15*time.Minute),
		woCostedB, planID, skuB, 1, domain.WOCosted, now.Add(-25*time.Minute),
	); err != nil {
		t.Fatalf("seed work orders: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO inventory_lots (id, material_id, quantity, cost_per_sheet_amount, cost_per_sheet_currency, supplier_ref, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		lotID, matPlywood, 2, 100000, "VND", "SUP-DASH", now,
	); err != nil {
		t.Fatalf("seed lot: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO board_sheets (id, lot_id, length_mm, width_mm, cost_amount, cost_currency, status)
		 VALUES
		   ($1, $2, $3, $4, $5, $6, $7),
		   ($8, $9, $10, $11, $12, $13, $14)`,
		sheet1, lotID, 100, 100, 100000, "VND", "AVAILABLE",
		sheet2, lotID, 100, 50, 50000, "VND", "AVAILABLE",
	); err != nil {
		t.Fatalf("seed sheets: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO cutting_records (id, sheet_id, remnant_source_id, work_order_id, sku_id, used_length_mm, used_width_mm, created_at)
		 VALUES
		   ($1, $2, NULL, $3, $4, $5, $6, $7),
		   ($8, $9, NULL, $10, $11, $12, $13, $14)`,
		cutOld, sheet1, woPlanned, skuA, 50, 30, now.Add(-20*time.Minute),
		cutNew, sheet2, woCutting, skuB, 50, 50, now.Add(-10*time.Minute),
	); err != nil {
		t.Fatalf("seed cutting records: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO remnants (id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, allocated_to_wo_id, created_at)
		 VALUES
		   ($1, $2, NULL, $3, $4, $5, NULL, $6),
		   ($7, $8, NULL, $9, $10, $11, $12, $13),
		   ($14, $15, NULL, $16, $17, $18, NULL, $19),
		   ($20, $21, NULL, $22, $23, $24, NULL, $25)`,
		remAvailable, sheet1, 30, 20, domain.RemnantAvailable, day,
		remAllocated, sheet1, 30, 20, domain.RemnantAllocated, woPlanned, day,
		remConsumed, sheet1, 30, 20, domain.RemnantConsumed, day,
		remWaste, sheet1, 30, 20, domain.RemnantWaste, day,
	); err != nil {
		t.Fatalf("seed remnants: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO consumption_records (id, work_order_id, material_id, material_type, quantity, unit, created_at)
		 VALUES
		   ($1, $2, $3, $4, $5, $6, $7),
		   ($8, $9, $10, $11, $12, $13, $14),
		   ($15, $16, $17, $18, $19, $20, $21)`,
		uuid.New(), woCompleted, matPlywood, "PLYWOOD", 5.0, "sheet", day,
		uuid.New(), woCompleted, matMetal, "METAL", 2.0, "kg", day,
		uuid.New(), woCompleted, matAccessory, "GLUE", 1.0, "kg", day,
	); err != nil {
		t.Fatalf("seed consumption records: %v", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO costing_records (
			id, work_order_id, sku_id,
			material_cost_amount, material_cost_currency,
			auxiliary_cost_amount, auxiliary_cost_currency,
			total_cost_amount, total_cost_currency,
			finalized, created_at
		)
		 VALUES
		   ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11),
		   ($12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
		costedNewer, woCostedA, skuA, 6000, "VND", 1000, "VND", 7000, "VND", true, now.Add(-12*time.Minute),
		costedOlder, woCostedB, skuB, 2500, "VND", 500, "VND", 3000, "VND", true, now.Add(-22*time.Minute),
	); err != nil {
		t.Fatalf("seed costing records: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit fixture tx: %v", err)
	}

	return overviewFixture{
		expectedDate:        day.Format("2006-01-02"),
		latestCutID:         cutNew,
		latestCompletedWOID: woCompleted,
		latestCostedWOID:    woCostedA,
	}
}

func TestIntegration_GetOverview_EmptyDB(t *testing.T) {
	pool := getPool(t)
	truncateDashboard(t, pool)

	svc := NewService(NewPGStore(pool))
	out, err := svc.GetOverview(context.Background())
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}

	if out.KPI.Remnants.Total != 0 || out.KPI.Remnants.Available != 0 || out.KPI.Remnants.Allocated != 0 || out.KPI.Remnants.Consumed != 0 || out.KPI.Remnants.Waste != 0 {
		t.Errorf("empty remnants KPI = %+v, want all zeros", out.KPI.Remnants)
	}
	if out.KPI.UtilizationPct != 0 {
		t.Errorf("utilization_pct = %v, want 0", out.KPI.UtilizationPct)
	}
	if out.KPI.ActiveWorkOrders != 0 {
		t.Errorf("active_work_orders = %d, want 0", out.KPI.ActiveWorkOrders)
	}
	if out.KPI.PendingCosting != 0 {
		t.Errorf("pending_costing = %d, want 0", out.KPI.PendingCosting)
	}
	if len(out.Charts.RemnantTrend7D) != 7 {
		t.Errorf("remnant_trend_7d len = %d, want 7", len(out.Charts.RemnantTrend7D))
	}
	if len(out.Charts.MaterialUsage) != 7 {
		t.Errorf("material_usage len = %d, want 7", len(out.Charts.MaterialUsage))
	}
	if len(out.Charts.CostAllocation) != 0 {
		t.Errorf("cost_allocation len = %d, want 0", len(out.Charts.CostAllocation))
	}
	if len(out.RecentActivity.RecentCuts) != 0 {
		t.Errorf("recent_cuts len = %d, want 0", len(out.RecentActivity.RecentCuts))
	}
	if len(out.RecentActivity.CompletedWorkOrders) != 0 {
		t.Errorf("completed_work_orders len = %d, want 0", len(out.RecentActivity.CompletedWorkOrders))
	}
	if len(out.RecentActivity.CostingFinalizations) != 0 {
		t.Errorf("costing_finalizations len = %d, want 0", len(out.RecentActivity.CostingFinalizations))
	}
}

func TestIntegration_GetOverview_AggregatesData(t *testing.T) {
	pool := getPool(t)
	truncateDashboard(t, pool)
	fixture := seedOverviewFixture(t, pool)

	svc := NewService(NewPGStore(pool))
	out, err := svc.GetOverview(context.Background())
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}

	if out.KPI.Remnants.Total != 4 {
		t.Errorf("remnants.total = %d, want 4", out.KPI.Remnants.Total)
	}
	if out.KPI.Remnants.Available != 1 {
		t.Errorf("remnants.available = %d, want 1", out.KPI.Remnants.Available)
	}
	if out.KPI.Remnants.Allocated != 1 {
		t.Errorf("remnants.allocated = %d, want 1", out.KPI.Remnants.Allocated)
	}
	if out.KPI.Remnants.Consumed != 1 {
		t.Errorf("remnants.consumed = %d, want 1", out.KPI.Remnants.Consumed)
	}
	if out.KPI.Remnants.Waste != 1 {
		t.Errorf("remnants.waste = %d, want 1", out.KPI.Remnants.Waste)
	}
	if math.Abs(out.KPI.UtilizationPct-26.67) > 0.001 {
		t.Errorf("utilization_pct = %v, want 26.67", out.KPI.UtilizationPct)
	}
	if out.KPI.ActiveWorkOrders != 3 {
		t.Errorf("active_work_orders = %d, want 3", out.KPI.ActiveWorkOrders)
	}
	if out.KPI.PendingCosting != 1 {
		t.Errorf("pending_costing = %d, want 1", out.KPI.PendingCosting)
	}

	if len(out.Charts.RemnantTrend7D) != 7 {
		t.Fatalf("remnant_trend_7d len = %d, want 7", len(out.Charts.RemnantTrend7D))
	}
	var trendFound bool
	for _, p := range out.Charts.RemnantTrend7D {
		if p.Date == fixture.expectedDate {
			trendFound = true
			if p.Available != 1 || p.Allocated != 1 || p.Waste != 1 {
				t.Errorf("trend point %+v, want available=1 allocated=1 waste=1", p)
			}
		}
	}
	if !trendFound {
		t.Errorf("expected remnant trend date %s not found", fixture.expectedDate)
	}

	if len(out.Charts.CostAllocation) != 2 {
		t.Fatalf("cost_allocation len = %d, want 2", len(out.Charts.CostAllocation))
	}
	if out.Charts.CostAllocation[0].SKUCode != "SKU-A" || out.Charts.CostAllocation[0].Cost != 7000 {
		t.Errorf("cost_allocation[0] = %+v, want SKU-A/7000", out.Charts.CostAllocation[0])
	}
	if out.Charts.CostAllocation[1].SKUCode != "SKU-B" || out.Charts.CostAllocation[1].Cost != 3000 {
		t.Errorf("cost_allocation[1] = %+v, want SKU-B/3000", out.Charts.CostAllocation[1])
	}

	if len(out.Charts.MaterialUsage) != 7 {
		t.Fatalf("material_usage len = %d, want 7", len(out.Charts.MaterialUsage))
	}
	var usageFound bool
	for _, p := range out.Charts.MaterialUsage {
		if p.Date == fixture.expectedDate {
			usageFound = true
			if math.Abs(p.Plywood-5) > 0.001 || math.Abs(p.Metal-2) > 0.001 || math.Abs(p.Accessory-1) > 0.001 {
				t.Errorf("material usage point %+v, want plywood=5 metal=2 accessory=1", p)
			}
		}
	}
	if !usageFound {
		t.Errorf("expected material usage date %s not found", fixture.expectedDate)
	}

	if len(out.RecentActivity.RecentCuts) < 1 {
		t.Fatalf("recent_cuts len = %d, want >=1", len(out.RecentActivity.RecentCuts))
	}
	if out.RecentActivity.RecentCuts[0].ID != fixture.latestCutID {
		t.Errorf("recent_cuts[0].id = %v, want %v", out.RecentActivity.RecentCuts[0].ID, fixture.latestCutID)
	}

	if len(out.RecentActivity.CompletedWorkOrders) < 1 {
		t.Fatalf("completed_work_orders len = %d, want >=1", len(out.RecentActivity.CompletedWorkOrders))
	}
	if out.RecentActivity.CompletedWorkOrders[0].ID != fixture.latestCompletedWOID {
		t.Errorf("completed_work_orders[0].id = %v, want %v", out.RecentActivity.CompletedWorkOrders[0].ID, fixture.latestCompletedWOID)
	}

	if len(out.RecentActivity.CostingFinalizations) != 2 {
		t.Fatalf("costing_finalizations len = %d, want 2", len(out.RecentActivity.CostingFinalizations))
	}
	if out.RecentActivity.CostingFinalizations[0].WorkOrderID != fixture.latestCostedWOID {
		t.Errorf("costing_finalizations[0].work_order_id = %v, want %v", out.RecentActivity.CostingFinalizations[0].WorkOrderID, fixture.latestCostedWOID)
	}
	if out.RecentActivity.CostingFinalizations[0].TotalCost != 7000 {
		t.Errorf("costing_finalizations[0].total_cost = %d, want 7000", out.RecentActivity.CostingFinalizations[0].TotalCost)
	}
}
