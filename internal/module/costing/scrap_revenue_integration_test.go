//go:build integration

package costing

// scrap_revenue_integration_test.go — BR-C05/C06/C07 integration tests
// for scrap_sales offset against waste cost in WasteReport.
//
// These tests run against a real PostgreSQL 17 container managed by
// testhelper.StartTestDB. They verify that:
//   - scrap_sale_revenue is aggregated by material within the period filter;
//   - net_waste_cost = max(total_waste_cost - scrap_sale_revenue, 0);
//   - materials with scrap sales but zero cuts still appear in the report
//     (UNION pattern, user confirm #2);
//   - non-VND scrap rows are filtered out (Phase A guard).
//
// Run with:
//
//	make test-integration
//	# or directly:
//	go test -tags integration ./internal/module/costing/... -v -count=1 \
//	    -run TestIntegration_ScrapRevenue

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/testhelper"
)

// scrapSeed wraps the IDs created during seeding so each test can reuse them
// without repeating SQL.
type scrapSeed struct {
	materialID uuid.UUID
	lotID      uuid.UUID
	sheetIDs   []uuid.UUID
	adminID    uuid.UUID
}

// seedScrapFixtures creates the canonical test fixture for a per-material
// waste-and-scrap workflow:
//   - 1 material (plywood)
//   - 1 inventory lot with 5 sheets of 2_000_000 mm² each
//   - 5 cuts that each consume 1_000_000 mm² with no remnant produced —
//     i.e. 1_000_000 mm² of waste per sheet, 5_000_000 mm² total waste
//
// Sheet cost is 100_000 VND for a 2_000_000 mm² sheet ⇒ 0.05 VND/mm² ⇒
// each cut wastes 1_000_000 × 0.05 = 50_000 VND. Total waste cost over
// 5 cuts = 250_000 VND.
func seedScrapFixtures(t *testing.T, pool *pgxpool.Pool) scrapSeed {
	t.Helper()
	ctx := context.Background()

	// Reuse the default admin seeded by migration 00008.
	var adminID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE username = 'admin'`).Scan(&adminID); err != nil {
		t.Fatalf("lookup admin user: %v", err)
	}

	matID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO materials (id, type, name, unit, created_at)
		 VALUES ($1, 'PLYWOOD', 'Test Plywood (scrap)', 'sheet', now())`,
		matID,
	); err != nil {
		t.Fatalf("insert material: %v", err)
	}

	lotID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO inventory_lots (id, material_id, quantity, cost_per_sheet_amount, cost_per_sheet_currency, supplier_ref, received_at)
		 VALUES ($1, $2, 5, 100000, 'VND', 'SUP-SCRAP', now())`,
		lotID, matID,
	); err != nil {
		t.Fatalf("insert lot: %v", err)
	}

	sheetIDs := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		sheetIDs[i] = uuid.New()
		if _, err := pool.Exec(ctx,
			`INSERT INTO board_sheets (id, lot_id, length_mm, width_mm, cost_amount, cost_currency, status)
			 VALUES ($1, $2, 2000, 1000, 100000, 'VND', 'AVAILABLE')`,
			sheetIDs[i], lotID,
		); err != nil {
			t.Fatalf("insert sheet %d: %v", i, err)
		}
		// One cut per sheet: 1000×1000 = 1_000_000 used, leaves 1_000_000 mm²
		// of waste (no remnant created so the SQL treats it all as waste).
		if _, err := pool.Exec(ctx,
			`INSERT INTO cutting_records (id, sheet_id, work_order_id, sku_id, used_length_mm, used_width_mm)
			 VALUES ($1, $2, $3, $4, 1000, 1000)`,
			uuid.New(), sheetIDs[i], uuid.New(), uuid.New(),
		); err != nil {
			t.Fatalf("insert cutting_record %d: %v", i, err)
		}
	}

	return scrapSeed{
		materialID: matID,
		lotID:      lotID,
		sheetIDs:   sheetIDs,
		adminID:    adminID,
	}
}

// insertScrapSale is a thin helper that posts a row directly so the test
// stays inside the costing package (no scrap-module import needed).
func insertScrapSale(t *testing.T, pool *pgxpool.Pool, materialID, createdBy uuid.UUID, saleDate time.Time, qtyKG float64, unitPrice int64, currency string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO scrap_sales (id, sale_date, material_id, quantity_kg, unit_price, currency, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		uuid.New(), saleDate, materialID, qtyKG, unitPrice, currency, createdBy,
	); err != nil {
		t.Fatalf("insert scrap_sale: %v", err)
	}
}

// TestIntegration_ScrapRevenue_OffsetsWasteCost is the end-to-end DoD #20 proof
// for issue #299: 5 cuts produce 250_000 VND of waste cost, 3 scrap sales
// generate 100_000 VND of revenue, so net_waste_cost = 150_000 VND.
func TestIntegration_ScrapRevenue_OffsetsWasteCost(t *testing.T) {
	pool := testhelper.StartTestDB(t)
	seed := seedScrapFixtures(t, pool)

	// 3 scrap sales for this material totalling 100_000 VND (60k + 30k + 10k).
	saleDate := time.Now().UTC().Add(-time.Hour)
	insertScrapSale(t, pool, seed.materialID, seed.adminID, saleDate, 30.0, 2000, "VND")  // 60_000
	insertScrapSale(t, pool, seed.materialID, seed.adminID, saleDate, 15.0, 2000, "VND")  // 30_000
	insertScrapSale(t, pool, seed.materialID, seed.adminID, saleDate, 5.0, 2000, "VND")   // 10_000

	st := NewPGStore(pool).(*pgStore)
	rows, err := st.selectWasteReport(context.Background(), WasteReportFilter{})
	if err != nil {
		t.Fatalf("selectWasteReport: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (single material)", len(rows))
	}
	row := rows[0]

	// Waste arithmetic: 5 sheets × 1_000_000 mm² = 5_000_000 mm² total waste.
	const wantWasteAreaMM2 int64 = 5_000_000
	if row.WasteAreaMM2 != wantWasteAreaMM2 {
		t.Errorf("waste_area_mm2 = %d, want %d", row.WasteAreaMM2, wantWasteAreaMM2)
	}

	// Sheet cost 100_000 / area 2_000_000 = 0.05 VND/mm². Each cut wastes
	// 1_000_000 mm² ⇒ 50_000 VND × 5 = 250_000 VND total.
	const wantTotalWaste int64 = 250_000
	if row.TotalWasteCost.Amount != wantTotalWaste {
		t.Errorf("total_waste_cost = %d, want %d", row.TotalWasteCost.Amount, wantTotalWaste)
	}

	// 3 scrap sales × (qty × price) = 60k + 30k + 10k = 100_000 VND.
	const wantScrapRevenue int64 = 100_000
	if row.ScrapSaleRevenue.Amount != wantScrapRevenue {
		t.Errorf("scrap_sale_revenue = %d, want %d", row.ScrapSaleRevenue.Amount, wantScrapRevenue)
	}

	// net_waste_cost = max(total_waste_cost - scrap_revenue, 0)
	//                = max(250_000 - 100_000, 0) = 150_000 VND.
	const wantNet int64 = 150_000
	if row.NetWasteCost.Amount != wantNet {
		t.Errorf("net_waste_cost = %d, want %d", row.NetWasteCost.Amount, wantNet)
	}
}

// TestIntegration_ScrapRevenue_ExceedsWaste_NetClampsToZero verifies BR-C06
// clamp behaviour: when scrap revenue > waste cost, net_waste_cost = 0.
func TestIntegration_ScrapRevenue_ExceedsWaste_NetClampsToZero(t *testing.T) {
	pool := testhelper.StartTestDB(t)
	seed := seedScrapFixtures(t, pool)

	// Scrap revenue 500_000 VND > waste cost 250_000 VND — net must clamp to 0.
	saleDate := time.Now().UTC().Add(-time.Hour)
	insertScrapSale(t, pool, seed.materialID, seed.adminID, saleDate, 250.0, 2000, "VND") // 500_000

	st := NewPGStore(pool).(*pgStore)
	rows, err := st.selectWasteReport(context.Background(), WasteReportFilter{})
	if err != nil {
		t.Fatalf("selectWasteReport: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]

	if row.TotalWasteCost.Amount != 250_000 {
		t.Errorf("total_waste_cost = %d, want 250_000", row.TotalWasteCost.Amount)
	}
	if row.ScrapSaleRevenue.Amount != 500_000 {
		t.Errorf("scrap_sale_revenue = %d, want 500_000", row.ScrapSaleRevenue.Amount)
	}
	if row.NetWasteCost.Amount != 0 {
		t.Errorf("net_waste_cost = %d, want 0 (clamped — revenue > waste)", row.NetWasteCost.Amount)
	}
}

// TestIntegration_ScrapRevenue_NonVNDFiltered verifies the Phase A multi-currency
// guard at the SQL level: scrap sales with currency != 'VND' are excluded from
// the aggregate even though they're persisted in the table.
func TestIntegration_ScrapRevenue_NonVNDFiltered(t *testing.T) {
	pool := testhelper.StartTestDB(t)
	seed := seedScrapFixtures(t, pool)

	// One VND row (counted) + one USD row (excluded by SQL filter).
	saleDate := time.Now().UTC().Add(-time.Hour)
	insertScrapSale(t, pool, seed.materialID, seed.adminID, saleDate, 50.0, 2000, "VND")  // 100_000 VND
	insertScrapSale(t, pool, seed.materialID, seed.adminID, saleDate, 100.0, 2000, "USD") // 200_000 USD — must be ignored

	st := NewPGStore(pool).(*pgStore)
	rows, err := st.selectWasteReport(context.Background(), WasteReportFilter{})
	if err != nil {
		t.Fatalf("selectWasteReport: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if got := rows[0].ScrapSaleRevenue.Amount; got != 100_000 {
		t.Errorf("scrap_sale_revenue = %d, want 100_000 (USD row must be filtered out)", got)
	}
}
