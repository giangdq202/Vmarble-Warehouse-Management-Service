//go:build integration

package inventory

// min_remnant_policy_integration_test.go — BR-K06/K07/K08 integration tests
// for the per-material min_remnant policy.
//
// These tests run against a real PostgreSQL 17 container (managed by
// testcontainers-go). They verify that:
//   - sub-threshold remnants are NOT inserted into the remnants table;
//   - the cut still produces a cutting_records row with a parent sheet, so
//     the waste-report SQL computes (source_area − used_area − 0) for that
//     cut — i.e. the dropped remnant area lands in WasteReport.waste_area_mm2.
//
// Run with:
//
//	make test-integration
//	# or directly:
//	go test -tags integration ./internal/module/inventory/... -v -count=1 \
//	    -run TestIntegration_BRK06

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// seedMaterialWithPolicy inserts a plywood material whose BR-K06/K07
// thresholds are set to the supplied values.
func seedMaterialWithPolicy(t *testing.T, pool *pgxpool.Pool, lengthMM, widthMM int) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO materials (id, type, name, unit, min_remnant_length_mm, min_remnant_width_mm, created_at)
		 VALUES ($1, 'PLYWOOD', 'Test Plywood (policy)', 'sheet', $2, $3, now())`,
		id, lengthMM, widthMM,
	)
	if err != nil {
		t.Fatalf("seed material: %v", err)
	}
	return id
}

// receiveAndPassQC creates one lot of `qty` sheets with the given dimension
// and immediately pulls them through QC so they land in AVAILABLE — required
// because ReceiveStock writes PENDING_QC by default (BR-INV01).
func receiveAndPassQC(t *testing.T, svc Service, matID uuid.UUID, dim domain.Dimension, qty int) (lotID uuid.UUID) {
	t.Helper()
	lot, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   dim,
		CostPerSheet: testCost,
		Quantity:     qty,
		SupplierRef:  "SUP-BRK06",
	})
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	if err := svc.QCPassLot(context.Background(), lot.ID, uuid.New()); err != nil {
		t.Fatalf("QCPassLot: %v", err)
	}
	return lot.ID
}

// computeWasteForSheet runs the same arithmetic as costing.selectWasteReport
// for a single board sheet: source − used − new_remnant_area, summed across
// every cutting_record for that sheet (and any remnants in its lineage).
//
// Implementing it in raw SQL inside the inventory package keeps the
// dependency direction clean (inventory does not import costing) while still
// asserting the exact same shape the WasteReport endpoint exposes.
func computeWasteForSheet(t *testing.T, pool *pgxpool.Pool, sheetID uuid.UUID) int64 {
	t.Helper()
	const query = `
WITH cuts_with_waste AS (
    SELECT
        CAST(cr.used_length_mm AS bigint) * CAST(cr.used_width_mm AS bigint) AS used_area_mm2,
        CASE
            WHEN cr.sheet_id IS NOT NULL THEN
                CAST(bs_direct.length_mm AS bigint) * CAST(bs_direct.width_mm AS bigint)
            ELSE
                CAST(r.length_mm AS bigint) * CAST(r.width_mm AS bigint)
        END AS source_area_mm2,
        COALESCE((
            SELECT CAST(nr.length_mm AS bigint) * CAST(nr.width_mm AS bigint)
            FROM remnants nr
            WHERE (
                (cr.sheet_id IS NOT NULL AND nr.parent_board_id = cr.sheet_id AND nr.parent_remnant_id IS NULL)
                OR
                (cr.remnant_source_id IS NOT NULL AND nr.parent_remnant_id = cr.remnant_source_id)
            )
            LIMIT 1
        ), 0) AS new_remnant_area_mm2
    FROM cutting_records cr
    LEFT JOIN board_sheets bs_direct ON bs_direct.id = cr.sheet_id
    LEFT JOIN remnants r ON r.id = cr.remnant_source_id
    WHERE COALESCE(cr.sheet_id, r.parent_board_id) = $1
)
SELECT COALESCE(SUM(GREATEST(source_area_mm2 - used_area_mm2 - new_remnant_area_mm2, 0)), 0)
FROM cuts_with_waste`
	var total int64
	if err := pool.QueryRow(context.Background(), query, sheetID).Scan(&total); err != nil {
		t.Fatalf("compute waste: %v", err)
	}
	return total
}

// countRemnantsForSheet returns the number of remnant rows in the lineage of
// the given board sheet. Used to assert "no remnant row was inserted" when
// BR-K06 drops a sub-threshold leftover.
func countRemnantsForSheet(t *testing.T, pool *pgxpool.Pool, sheetID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM remnants WHERE parent_board_id = $1`, sheetID,
	).Scan(&n); err != nil {
		t.Fatalf("count remnants: %v", err)
	}
	return n
}

// TestIntegration_BRK06_DroppedRemnantLandsInWasteReport is the end-to-end
// proof of DoD #4 for issue #297: a cut whose leftover falls below the
// material's min_remnant policy must (a) skip remnant insertion entirely and
// (b) make the dropped area show up in the waste-report ledger.
//
// The BR-K07 escape hatch (both thresholds = 0 → keep remnant) is covered by
// the BR-K06/K07 unit tests in service_test.go and is intentionally NOT
// repeated here. Splitting these into two top-level integration tests fails
// because testhelper.StartTestDB registers t.Cleanup(pool.Close) against the
// first caller's t — once that test ends the shared pool is closed.
func TestIntegration_BRK06_DroppedRemnantLandsInWasteReport(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterialWithPolicy(t, pool, 100, 100) // length AND width threshold = 100mm
	svc := newSvc(pool)

	// Source sheet: 2_000_000 mm² (2000×1000).
	receiveAndPassQC(t, svc, matID, testDim2000x1000, 1)

	sheetsResult, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, nil)
	if err != nil {
		t.Fatalf("ListAvailableSheets: %v", err)
	}
	if len(sheetsResult.Items) != 1 {
		t.Fatalf("available sheets = %d, want 1", len(sheetsResult.Items))
	}
	sheetID := sheetsResult.Items[0].ID

	// Cut consumes 1000×500 (500_000 mm²) and would normally produce a
	// 99×400 remnant (39_600 mm²) — but length 99 < threshold 100, so the
	// remnant is dropped and that area becomes waste.
	subThresholdRemnant := domain.Dimension{LengthMM: 99, WidthMM: 400}
	result, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim1000x500,
		RemnantDimension: &subThresholdRemnant,
	})
	if err != nil {
		t.Fatalf("RecordCut: %v", err)
	}

	// (1) The result must signal the drop to the caller.
	if result.DroppedRemnantCount != 1 {
		t.Errorf("DroppedRemnantCount = %d, want 1", result.DroppedRemnantCount)
	}
	if result.RemnantID != nil {
		t.Errorf("RemnantID = %v, want nil when remnant is dropped", *result.RemnantID)
	}

	// (2) No remnants row was inserted — this is what makes the dropped area
	//     show up as waste in the WasteReport SQL: new_remnant_area_mm2 = 0
	//     because there's no row to join.
	if got := countRemnantsForSheet(t, pool, sheetID); got != 0 {
		t.Errorf("remnant rows for sheet = %d, want 0 — drop should not persist", got)
	}

	// (3) The waste-report arithmetic for this sheet must equal
	//     source − used = 2_000_000 − 500_000 = 1_500_000 mm².
	//     With new_remnant_area = 0 (dropped), the entire leftover lands in waste.
	const wantWaste int64 = 2_000_000 - 500_000
	if got := computeWasteForSheet(t, pool, sheetID); got != wantWaste {
		t.Errorf("waste_area_mm2 = %d, want %d (sub-threshold remnant must land in waste)", got, wantWaste)
	}
}
