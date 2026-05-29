//go:build integration

package inventory

// pgstore_integration_test.go — integration tests for the inventory pgstore.
//
// These tests run against a real PostgreSQL 17 container (managed by
// testcontainers-go). They verify that SQL queries, transactions,
// row-level locks, and business constraints work correctly end-to-end.
//
// Run with:
//
//	make test-integration
//	# or directly:
//	go test -tags integration ./internal/module/inventory/... -v -count=1

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
	"github.com/vmarble/warehouse-management-service/internal/testhelper"
)

// ── shared pool ──────────────────────────────────────────────────────────────
//
// All tests in this package share one container to avoid the overhead of
// starting a new container per test. State is reset between tests by
// truncateInventory().

var (
	sharedPool *pgxpool.Pool
	setupOnce  sync.Once
	setupErr   error
)

// getPool returns the shared pool, initialising it lazily on first use.
// This avoids the need for a custom testing.TB wrapper in TestMain.
func getPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	setupOnce.Do(func() {
		sharedPool = testhelper.StartTestDB(t)
	})
	if setupErr != nil {
		t.Fatalf("shared DB setup failed: %v", setupErr)
	}
	return sharedPool
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// newSvcWithThreshold returns inventory service with explicit overflow threshold.
func newSvcWithThreshold(pool *pgxpool.Pool, threshold float64) Service {
	return NewServiceWithOverflowThreshold(NewPGStore(pool), nil, threshold)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// truncateInventory deletes inventory-module rows in FK-safe order.
func truncateInventory(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range []string{
		"cutting_records",
		"remnants",
		"board_sheets",
		"inventory_lots",
	} {
		if _, err := sharedPool.Exec(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
}

// seedMaterial inserts a minimal material row so that inventory_lot → material FK is satisfied.
func seedMaterial(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO materials (id, type, name, unit, created_at)
		 VALUES ($1, 'PLYWOOD', 'Test Plywood', 'sheet', now())`,
		id,
	)
	if err != nil {
		t.Fatalf("seed material: %v", err)
	}
	return id
}

// newSvc returns an Service backed by the shared real DB.
// It uses a lenient overflow threshold to keep non-overflow integration tests stable.
func newSvc(pool *pgxpool.Pool) Service {
	return NewServiceWithOverflowThreshold(NewPGStore(pool), nil, 100)
}

func ptrUUID(v uuid.UUID) *uuid.UUID { return &v }

// ── fixtures ─────────────────────────────────────────────────────────────────

var (
	testDim2000x1000 = domain.Dimension{LengthMM: 2000, WidthMM: 1000}
	testDim1000x500  = domain.Dimension{LengthMM: 1000, WidthMM: 500}
	testDim800x400   = domain.Dimension{LengthMM: 800, WidthMM: 400}
	testDim100x100   = domain.Dimension{LengthMM: 100, WidthMM: 100}
	testCost         = domain.Money{Amount: 100_000, Currency: "VND"}
)

// ── ReceiveStock ──────────────────────────────────────────────────────────────

func TestIntegration_ReceiveStock_CreatesLotAndSheets(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	lot, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     3,
		SupplierRef:  "SUP-INTEGRATION-001",
	})
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	if lot.ID == uuid.Nil {
		t.Error("lot.ID must be non-nil")
	}
	if lot.Quantity != 3 {
		t.Errorf("lot.Quantity = %d, want 3", lot.Quantity)
	}

	sheetsResult, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	if err != nil {
		t.Fatalf("ListAvailableSheets: %v", err)
	}
	if len(sheets) != 3 {
		t.Errorf("available sheets = %d, want 3", len(sheets))
	}
	for _, sh := range sheets {
		if sh.LotID != lot.ID {
			t.Errorf("sheet.LotID = %v, want lot.ID %v", sh.LotID, lot.ID)
		}
		if sh.Status != "AVAILABLE" {
			t.Errorf("sheet.Status = %q, want AVAILABLE", sh.Status)
		}
		if sh.Dimensions != testDim2000x1000 {
			t.Errorf("sheet.Dimensions = %v, want %v", sh.Dimensions, testDim2000x1000)
		}
	}
}

func TestIntegration_ReceiveStock_ListLots(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	for i := range 3 {
		_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
			MaterialID:   matID,
			Dimensions:   testDim2000x1000,
			CostPerSheet: domain.Money{Amount: int64(10_000 * (i + 1)), Currency: "VND"},
			Quantity:     1,
			SupplierRef:  fmt.Sprintf("SUP-%02d", i),
		})
		if err != nil {
			t.Fatalf("ReceiveStock[%d]: %v", i, err)
		}
	}

	lotsResult, err := svc.ListLots(context.Background(), httpkit.PageParams{Page: 1, Limit: 100})
	lots := lotsResult.Items
	if err != nil {
		t.Fatalf("ListLots: %v", err)
	}
	if len(lots) != 3 {
		t.Errorf("lots count = %d, want 3", len(lots))
	}
}

// ── RecordCut from sheet ──────────────────────────────────────────────────────

func TestIntegration_RecordCut_FromSheet_NoRemnant(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	sheetID := sheets[0].ID
	woID := uuid.New()

	result, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptrUUID(sheetID),
		WorkOrderID:   woID,
		SKUID:         uuid.New(),
		UsedDimension: testDim1000x500,
	})
	if err != nil {
		t.Fatalf("RecordCut: %v", err)
	}
	if result.CuttingRecordID == uuid.Nil {
		t.Error("CuttingRecordID must be set")
	}
	if result.RemnantID != nil {
		t.Error("RemnantID must be nil when no remnant dimension given")
	}

	// Sheet must be ISSUED now.
	sh, err := svc.GetSheet(context.Background(), sheetID)
	if err != nil {
		t.Fatalf("GetSheet: %v", err)
	}
	if sh.Status != "ISSUED" {
		t.Errorf("sheet.Status = %q after cut, want ISSUED", sh.Status)
	}
	if sh.IssuedToWorkOrderID == nil || *sh.IssuedToWorkOrderID != woID {
		t.Errorf("sheet.IssuedToWorkOrderID = %v, want %v", sh.IssuedToWorkOrderID, woID)
	}
}

func TestIntegration_RecordCut_FromSheet_WithRemnant(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	sheetID := sheets[0].ID
	remnantDim := testDim800x400

	result, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim1000x500,
		RemnantDimension: &remnantDim,
	})
	if err != nil {
		t.Fatalf("RecordCut: %v", err)
	}
	if result.RemnantID == nil {
		t.Fatal("RemnantID must be set when remnant dimension is provided")
	}

	remnants, err := svc.FindAvailableRemnants(context.Background(), testDim100x100)
	if err != nil {
		t.Fatalf("FindAvailableRemnants: %v", err)
	}
	if len(remnants) != 1 {
		t.Fatalf("available remnants = %d, want 1", len(remnants))
	}
	r := remnants[0]
	if r.ID != *result.RemnantID {
		t.Errorf("remnant.ID = %v, want %v", r.ID, *result.RemnantID)
	}
	if r.ParentBoardID != sheetID {
		t.Errorf("remnant.ParentBoardID = %v, want sheetID %v", r.ParentBoardID, sheetID)
	}
	if r.Status != domain.RemnantAvailable {
		t.Errorf("remnant.Status = %v, want AVAILABLE", r.Status)
	}
	if r.Dimensions != remnantDim {
		t.Errorf("remnant.Dimensions = %v, want %v", r.Dimensions, remnantDim)
	}
}

// ── RecordCut from remnant (nested cutting BR-K04) ───────────────────────────

func TestIntegration_RecordCut_FromRemnant_NestedLineage(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	sheetID := sheets[0].ID

	// First cut: sheet → remnant 1000×500.
	remnantDim := testDim1000x500
	r1, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim1000x500,
		RemnantDimension: &remnantDim,
	})
	if err != nil {
		t.Fatalf("first RecordCut: %v", err)
	}

	// Second cut: remnant → nested remnant 100×100.
	nestedDim := testDim100x100
	r2, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:        r1.RemnantID,
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim100x100,
		RemnantDimension: &nestedDim,
	})
	if err != nil {
		t.Fatalf("second RecordCut (from remnant): %v", err)
	}
	if r2.RemnantID == nil {
		t.Fatal("nested remnant ID must be set")
	}

	// GetRemnantLineage must return both remnants linked to the original board.
	lineage, err := svc.GetRemnantLineage(context.Background(), sheetID)
	if err != nil {
		t.Fatalf("GetRemnantLineage: %v", err)
	}
	for _, rem := range lineage {
		if rem.ParentBoardID != sheetID {
			t.Errorf("remnant.ParentBoardID = %v, want %v", rem.ParentBoardID, sheetID)
		}
	}
	found := false
	for _, rem := range lineage {
		if rem.ID == *r2.RemnantID {
			found = true
			if rem.ParentRemnantID == nil || *rem.ParentRemnantID != *r1.RemnantID {
				t.Errorf("nested remnant.ParentRemnantID = %v, want %v",
					rem.ParentRemnantID, *r1.RemnantID)
			}
		}
	}
	if !found {
		t.Error("nested remnant not found in lineage")
	}
}

// ── BR-K03 area conservation ──────────────────────────────────────────────────

func TestIntegration_RecordCut_AreaConservation_Rejected(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	sheetID := sheets[0].ID

	overDim := domain.Dimension{LengthMM: 2001, WidthMM: 1000}
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptrUUID(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: overDim,
	})
	if !errors.Is(err, domain.ErrAreaConservation) {
		t.Errorf("expected ErrAreaConservation, got %v", err)
	}

	// Sheet must still be AVAILABLE — no DB write happened.
	sh, _ := svc.GetSheet(context.Background(), sheetID)
	if sh.Status != "AVAILABLE" {
		t.Errorf("sheet.Status = %q after rejected cut, want AVAILABLE", sh.Status)
	}
}

// ── AllocateRemnant ───────────────────────────────────────────────────────────

func TestIntegration_AllocateRemnant_HappyPath(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	rd := testDim1000x500
	r, _ := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheets[0].ID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim1000x500,
		RemnantDimension: &rd,
	})

	woID := uuid.New()
	if err := svc.AllocateRemnant(context.Background(), *r.RemnantID, woID); err != nil {
		t.Fatalf("AllocateRemnant: %v", err)
	}

	available, _ := svc.FindAvailableRemnants(context.Background(), testDim100x100)
	for _, rem := range available {
		if rem.ID == *r.RemnantID {
			t.Error("allocated remnant must not appear in available list")
		}
	}
}

func TestIntegration_AllocateRemnant_AlreadyAllocated_Rejected(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	rd := testDim1000x500
	r, _ := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheets[0].ID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim1000x500,
		RemnantDimension: &rd,
	})

	svc.AllocateRemnant(context.Background(), *r.RemnantID, uuid.New())
	err := svc.AllocateRemnant(context.Background(), *r.RemnantID, uuid.New())
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for double allocation, got %v", err)
	}
}

// ── MarkRemnantWaste ──────────────────────────────────────────────────────────

func TestIntegration_MarkRemnantWaste_FromAvailable(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	rd := testDim1000x500
	r, _ := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheets[0].ID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim1000x500,
		RemnantDimension: &rd,
	})

	if err := svc.MarkRemnantWaste(context.Background(), *r.RemnantID); err != nil {
		t.Fatalf("MarkRemnantWaste: %v", err)
	}

	available, _ := svc.FindAvailableRemnants(context.Background(), testDim100x100)
	for _, rem := range available {
		if rem.ID == *r.RemnantID {
			t.Error("wasted remnant must not appear in available list")
		}
	}
}

// ── Row-level locking: concurrent RecordCut on same sheet ─────────────────────

// TestIntegration_ConcurrentRecordCut_SameSheet is the key integration test for
// the row-level locking story. It starts 10 goroutines that all simultaneously
// try to cut the same board sheet against a real PostgreSQL database. The
// SELECT … FOR UPDATE inside recordCutAtomically must serialise them so that
// exactly one succeeds and the rest receive ErrPreconditionFailed.
func TestIntegration_ConcurrentRecordCut_SameSheet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent lock test in short mode")
	}

	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	sheetID := sheets[0].ID

	const workers = 10
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		failures  int
	)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.RecordCut(context.Background(), RecordCutInput{
				SheetID:       ptrUUID(sheetID),
				WorkOrderID:   uuid.New(),
				SKUID:         uuid.New(),
				UsedDimension: testDim1000x500,
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				if !errors.Is(err, domain.ErrPreconditionFailed) && !errors.Is(err, domain.ErrInvalidInput) {
					t.Errorf("unexpected error from concurrent worker: %v", err)
				}
				failures++
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("FOR UPDATE: expected exactly 1 successful cut, got %d (failures: %d)",
			successes, failures)
	}

	sh, _ := svc.GetSheet(context.Background(), sheetID)
	if sh.Status != "ISSUED" {
		t.Errorf("sheet.Status = %q after concurrent cuts, want ISSUED", sh.Status)
	}
}

// TestIntegration_ConcurrentAllocateRemnant verifies that FOR UPDATE serialises
// concurrent allocation of the same remnant on a real PostgreSQL database.
func TestIntegration_ConcurrentAllocateRemnant_SameRemnant(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent lock test in short mode")
	}

	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheets := sheetsResult.Items
	rd := testDim1000x500
	r, _ := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheets[0].ID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim1000x500,
		RemnantDimension: &rd,
	})
	remnantID := *r.RemnantID

	const workers = 10
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
	)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.AllocateRemnant(context.Background(), remnantID, uuid.New())
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else if !errors.Is(err, domain.ErrPreconditionFailed) && !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("FOR UPDATE: expected exactly 1 successful allocation, got %d", successes)
	}
}

// ── ListLots pagination & search ──────────────────────────────────────────────

func TestIntegration_ListLots_Pagination_CorrectMetadata(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	svc := newSvc(pool)

	mat := seedMaterial(t, pool)

	// Seed 5 lots with distinct supplier refs.
	for i := range 5 {
		_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
			MaterialID:   mat,
			Dimensions:   domain.Dimension{LengthMM: 2000, WidthMM: 1000},
			CostPerSheet: domain.Money{Amount: 100_000, Currency: "VND"},
			Quantity:     1,
			SupplierRef:  fmt.Sprintf("PAG-SUP-%02d", i),
		})
		if err != nil {
			t.Fatalf("ReceiveStock[%d]: %v", i, err)
		}
	}

	// Page 1, limit 2 → 3 pages total.
	p1, err := svc.ListLots(context.Background(), httpkit.PageParams{Page: 1, Limit: 2})
	if err != nil {
		t.Fatalf("ListLots page 1: %v", err)
	}
	if p1.TotalItems != 5 {
		t.Errorf("total_items = %d, want 5", p1.TotalItems)
	}
	if p1.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", p1.TotalPages)
	}
	if p1.CurrentPage != 1 {
		t.Errorf("current_page = %d, want 1", p1.CurrentPage)
	}
	if len(p1.Items) != 2 {
		t.Errorf("page-1 items = %d, want 2", len(p1.Items))
	}

	// Last page (3) has 1 item.
	p3, err := svc.ListLots(context.Background(), httpkit.PageParams{Page: 3, Limit: 2})
	if err != nil {
		t.Fatalf("ListLots page 3: %v", err)
	}
	if len(p3.Items) != 1 {
		t.Errorf("last-page items = %d, want 1", len(p3.Items))
	}
}

func TestIntegration_ListLots_Search_MatchesSupplierRef(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	svc := newSvc(pool)

	mat := seedMaterial(t, pool)
	dim := domain.Dimension{LengthMM: 2000, WidthMM: 1000}
	cost := domain.Money{Amount: 100_000, Currency: "VND"}

	svc.ReceiveStock(context.Background(), ReceiveStockInput{MaterialID: mat, Dimensions: dim, CostPerSheet: cost, Quantity: 1, SupplierRef: "ACME-2024"})
	svc.ReceiveStock(context.Background(), ReceiveStockInput{MaterialID: mat, Dimensions: dim, CostPerSheet: cost, Quantity: 1, SupplierRef: "BETA-2024"})
	svc.ReceiveStock(context.Background(), ReceiveStockInput{MaterialID: mat, Dimensions: dim, CostPerSheet: cost, Quantity: 1, SupplierRef: "GAMMA-2024"})

	result, err := svc.ListLots(context.Background(), httpkit.PageParams{Page: 1, Limit: 10, Search: "acme"})
	if err != nil {
		t.Fatalf("ListLots search: %v", err)
	}
	if result.TotalItems != 1 {
		t.Errorf("total_items = %d, want 1 (ILIKE 'acme')", result.TotalItems)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(result.Items))
	}
	if result.Items[0].SupplierRef != "ACME-2024" {
		t.Errorf("supplier_ref = %q, want 'ACME-2024'", result.Items[0].SupplierRef)
	}
}

func TestIntegration_ListLots_Search_NoResults(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	svc := newSvc(pool)

	mat := seedMaterial(t, pool)
	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   mat,
		Dimensions:   domain.Dimension{LengthMM: 2000, WidthMM: 1000},
		CostPerSheet: domain.Money{Amount: 100_000, Currency: "VND"},
		Quantity:     1,
		SupplierRef:  "ACME-2024",
	})

	result, err := svc.ListLots(context.Background(), httpkit.PageParams{Page: 1, Limit: 10, Search: "ZZZZZ-NO-MATCH"})
	if err != nil {
		t.Fatalf("ListLots no-match search: %v", err)
	}
	if result.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", result.TotalItems)
	}
	if len(result.Items) != 0 {
		t.Errorf("items = %d, want 0", len(result.Items))
	}
	if result.TotalPages < 1 {
		t.Errorf("total_pages = %d, want at least 1", result.TotalPages)
	}
}

// ── ListAvailableSheets pagination ────────────────────────────────────────────

func TestIntegration_ListAvailableSheets_Pagination_CorrectMetadata(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	svc := newSvc(pool)

	mat := seedMaterial(t, pool)
	// Seed 1 lot with 6 sheets.
	_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   mat,
		Dimensions:   domain.Dimension{LengthMM: 2000, WidthMM: 1000},
		CostPerSheet: domain.Money{Amount: 100_000, Currency: "VND"},
		Quantity:     6,
		SupplierRef:  "SHEET-PAG",
	})
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	// Page 1, limit 4 → 2 pages.
	p1, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 4}, nil)
	if err != nil {
		t.Fatalf("ListAvailableSheets page 1: %v", err)
	}
	if p1.TotalItems != 6 {
		t.Errorf("total_items = %d, want 6", p1.TotalItems)
	}
	if p1.TotalPages != 2 {
		t.Errorf("total_pages = %d, want 2", p1.TotalPages)
	}
	if len(p1.Items) != 4 {
		t.Errorf("page-1 items = %d, want 4", len(p1.Items))
	}

	// Last page (2) has 2 items.
	p2, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 2, Limit: 4}, nil)
	if err != nil {
		t.Fatalf("ListAvailableSheets page 2: %v", err)
	}
	if len(p2.Items) != 2 {
		t.Errorf("last-page items = %d, want 2", len(p2.Items))
	}
}

func TestIntegration_ListAvailableSheets_Empty_WhenNoneAvailable(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	svc := newSvc(pool)

	result, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, nil)
	if err != nil {
		t.Fatalf("ListAvailableSheets: %v", err)
	}
	if result.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", result.TotalItems)
	}
	if len(result.Items) != 0 {
		t.Errorf("items = %d, want 0", len(result.Items))
	}
	if result.TotalPages < 1 {
		t.Errorf("total_pages = %d, want at least 1", result.TotalPages)
	}
}

// BR-K01 — aggregate stock check: count must reflect AVAILABLE sheets per
// material, ignoring sheets in any other status (issued, cut, etc.).
func TestIntegration_CountAvailableSheetsByMaterial(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matA := seedMaterial(t, pool)
	matB := seedMaterial(t, pool)
	svc := newSvc(pool)

	if _, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID: matA, Dimensions: testDim2000x1000, CostPerSheet: testCost, Quantity: 3, SupplierRef: "SUP-A",
	}); err != nil {
		t.Fatalf("ReceiveStock matA: %v", err)
	}
	if _, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID: matB, Dimensions: testDim2000x1000, CostPerSheet: testCost, Quantity: 5, SupplierRef: "SUP-B",
	}); err != nil {
		t.Fatalf("ReceiveStock matB: %v", err)
	}

	gotA, err := svc.CountAvailableSheetsByMaterial(context.Background(), matA)
	if err != nil {
		t.Fatalf("CountAvailableSheetsByMaterial matA: %v", err)
	}
	if gotA != 3 {
		t.Errorf("count matA = %d, want 3", gotA)
	}
	gotB, err := svc.CountAvailableSheetsByMaterial(context.Background(), matB)
	if err != nil {
		t.Fatalf("CountAvailableSheetsByMaterial matB: %v", err)
	}
	if gotB != 5 {
		t.Errorf("count matB = %d, want 5", gotB)
	}

	// Reserve one matA sheet to a work order (status → ISSUED) — count drops by 1.
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, &matA)
	if len(sheetsResult.Items) == 0 {
		t.Fatal("expected at least one matA sheet to issue")
	}
	woID := uuid.New()
	if err := svc.PreAssignSheet(context.Background(), PreAssignSheetInput{
		SheetID: sheetsResult.Items[0].ID, WorkOrderID: woID,
	}); err != nil {
		t.Fatalf("PreAssignSheet: %v", err)
	}
	gotA2, err := svc.CountAvailableSheetsByMaterial(context.Background(), matA)
	if err != nil {
		t.Fatalf("CountAvailableSheetsByMaterial after pre-assign: %v", err)
	}
	if gotA2 != 2 {
		t.Errorf("count matA after pre-assign = %d, want 2", gotA2)
	}

	// Unknown material → 0, not error.
	gotUnknown, err := svc.CountAvailableSheetsByMaterial(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("CountAvailableSheetsByMaterial unknown: %v", err)
	}
	if gotUnknown != 0 {
		t.Errorf("count unknown material = %d, want 0", gotUnknown)
	}
}

// ── Sprint-2 tests (2.7 DoD) ─────────────────────────────────────────────────

// TestPGStore_InheritanceRoundtrip verifies that material attributes set on a
// board_sheet are persisted to the DB and then propagated to the new remnant
// row when RecordCut is called — end-to-end through the real pgstore layer.
func TestPGStore_InheritanceRoundtrip(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	// 1. Receive stock: insert a lot + one sheet.  The sheet has no material
	//    attrs yet; we will update them directly to simulate a sheet that was
	//    enriched after receipt (e.g. via a separate admin endpoint).
	lot, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
		SupplierRef:  "INHERIT-SUP-001",
	})
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	_ = lot

	// 2. Fetch the sheet ID.
	sheetsResult, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	if err != nil {
		t.Fatalf("ListAvailableSheets: %v", err)
	}
	if len(sheetsResult.Items) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(sheetsResult.Items))
	}
	sheetID := sheetsResult.Items[0].ID

	// 3. Patch the sheet row in the DB with supplier_code so RecordCut can
	//    inherit it.  This simulates having a richer insert path or admin update.
	wantSupplierCode := "SUP-VN-ROUNDTRIP"
	wantLotBatch := "LOT-2026-RT"
	wantGrainPattern := "VERTICAL"
	wantQualityGrade := "A"
	_, err = pool.Exec(context.Background(),
		`UPDATE board_sheets
		 SET supplier_code = $1, lot_batch = $2, grain_pattern = $3, quality_grade = $4
		 WHERE id = $5`,
		wantSupplierCode, wantLotBatch, wantGrainPattern, wantQualityGrade, sheetID,
	)
	if err != nil {
		t.Fatalf("patch board_sheet attrs: %v", err)
	}

	// 4. RecordCut: consume the sheet and produce a remnant.
	remnantDim := testDim1000x500
	cutResult, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim1000x500,
		RemnantDimension: &remnantDim,
	})
	if err != nil {
		t.Fatalf("RecordCut: %v", err)
	}
	if cutResult.RemnantID == nil {
		t.Fatal("RemnantID must be set")
	}

	// 5. Read the remnant back via the public API (FindAvailableRemnants queries
	//    the real DB and returns all material attributes) and verify inheritance.
	available, err := svc.FindAvailableRemnants(context.Background(), testDim100x100)
	if err != nil {
		t.Fatalf("FindAvailableRemnants after cut: %v", err)
	}
	var rem *Remnant
	for i := range available {
		if available[i].ID == *cutResult.RemnantID {
			rem = &available[i]
			break
		}
	}
	if rem == nil {
		t.Fatal("newly created remnant not found in available list")
	}

	if rem.SupplierCode == nil || *rem.SupplierCode != wantSupplierCode {
		t.Errorf("remnant.SupplierCode = %v, want %q", rem.SupplierCode, wantSupplierCode)
	}
	if rem.LotBatch == nil || *rem.LotBatch != wantLotBatch {
		t.Errorf("remnant.LotBatch = %v, want %q", rem.LotBatch, wantLotBatch)
	}
	if rem.GrainPattern == nil || *rem.GrainPattern != wantGrainPattern {
		t.Errorf("remnant.GrainPattern = %v, want %q", rem.GrainPattern, wantGrainPattern)
	}
	if rem.QualityGrade == nil || *rem.QualityGrade != wantQualityGrade {
		t.Errorf("remnant.QualityGrade = %v, want %q", rem.QualityGrade, wantQualityGrade)
	}
	if rem.ParentBoardID != sheetID {
		t.Errorf("remnant.ParentBoardID = %v, want sheetID %v", rem.ParentBoardID, sheetID)
	}
}

// TestPGStore_NestedRemnantLineage cuts 3 levels deep (sheet → L1 → L2 → L3),
// then calls GetRemnantLineage and verifies the complete parent chain stored in
// the DB: ParentBoardID is the original sheet at every level; ParentRemnantID
// tracks the direct parent at each intermediate level.
func TestPGStore_NestedRemnantLineage(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	// Seed 1 sheet 2000×1000.
	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheetID := sheetsResult.Items[0].ID

	// Level 1: sheet → remnant L1 (1000×500).
	// used 900×400 + remnant 1000×500 = 360_000 + 500_000 = 860_000 ≤ 2_000_000 ✓
	dimL1 := domain.Dimension{LengthMM: 1000, WidthMM: 500}
	r1, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    domain.Dimension{LengthMM: 900, WidthMM: 400},
		RemnantDimension: &dimL1,
	})
	if err != nil {
		t.Fatalf("L1 cut: %v", err)
	}
	if r1.RemnantID == nil {
		t.Fatal("L1 RemnantID must be set")
	}

	// Level 2: L1 (1000×500) → remnant L2 (500×250).
	// used 400×200 = 80_000 + remnant 500×250 = 125_000 → total 205_000 ≤ 500_000 ✓
	dimL2 := domain.Dimension{LengthMM: 500, WidthMM: 250}
	r2, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:        r1.RemnantID,
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    domain.Dimension{LengthMM: 400, WidthMM: 200},
		RemnantDimension: &dimL2,
	})
	if err != nil {
		t.Fatalf("L2 cut: %v", err)
	}
	if r2.RemnantID == nil {
		t.Fatal("L2 RemnantID must be set")
	}

	// Level 3: L2 (500×250) → remnant L3 (200×200).
	// used 200×150 = 30_000 + remnant 200×200 = 40_000 → total 70_000 ≤ 125_000 ✓
	dimL3 := domain.Dimension{LengthMM: 200, WidthMM: 200}
	r3, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:        r2.RemnantID,
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    domain.Dimension{LengthMM: 200, WidthMM: 150},
		RemnantDimension: &dimL3,
	})
	if err != nil {
		t.Fatalf("L3 cut: %v", err)
	}
	if r3.RemnantID == nil {
		t.Fatal("L3 RemnantID must be set")
	}

	// Query the full lineage from the original board sheet.
	lineage, err := svc.GetRemnantLineage(context.Background(), sheetID)
	if err != nil {
		t.Fatalf("GetRemnantLineage: %v", err)
	}

	// There should be exactly 3 remnants, one per level.
	if len(lineage) != 3 {
		t.Fatalf("lineage length = %d, want 3 (L1+L2+L3)", len(lineage))
	}

	// Build an id-keyed map for order-independent assertions.
	byID := make(map[uuid.UUID]Remnant, 3)
	for _, r := range lineage {
		byID[r.ID] = r
	}

	// ── L1 ────────────────────────────────────────────────────────────────────
	l1, ok := byID[*r1.RemnantID]
	if !ok {
		t.Fatal("L1 remnant not found in lineage")
	}
	if l1.ParentBoardID != sheetID {
		t.Errorf("L1.ParentBoardID = %v, want sheetID %v", l1.ParentBoardID, sheetID)
	}
	if l1.ParentRemnantID != nil {
		t.Errorf("L1.ParentRemnantID = %v, want nil (direct child of board)", l1.ParentRemnantID)
	}

	// ── L2 ────────────────────────────────────────────────────────────────────
	l2, ok := byID[*r2.RemnantID]
	if !ok {
		t.Fatal("L2 remnant not found in lineage")
	}
	if l2.ParentBoardID != sheetID {
		t.Errorf("L2.ParentBoardID = %v, want sheetID %v", l2.ParentBoardID, sheetID)
	}
	if l2.ParentRemnantID == nil || *l2.ParentRemnantID != *r1.RemnantID {
		t.Errorf("L2.ParentRemnantID = %v, want L1 %v", l2.ParentRemnantID, *r1.RemnantID)
	}

	// ── L3 ────────────────────────────────────────────────────────────────────
	l3, ok := byID[*r3.RemnantID]
	if !ok {
		t.Fatal("L3 remnant not found in lineage")
	}
	if l3.ParentBoardID != sheetID {
		t.Errorf("L3.ParentBoardID = %v, want sheetID %v", l3.ParentBoardID, sheetID)
	}
	if l3.ParentRemnantID == nil || *l3.ParentRemnantID != *r2.RemnantID {
		t.Errorf("L3.ParentRemnantID = %v, want L2 %v", l3.ParentRemnantID, *r2.RemnantID)
	}
}

// TestPGStore_BoundingBoxSearch verifies the COALESCE(bounding_box_*, *_mm)
// filter in selectAvailableRemnantsByMinDimension and selectRemnantsByFilter:
//
//   - A remnant whose bounding_box is smaller than the requested min dimension
//     must NOT appear in the results.
//   - A remnant whose bounding_box meets or exceeds the min dimension MUST appear.
//   - A remnant created without an explicit bounding_box (defaults to actual dim)
//     is filtered correctly against the actual dimension.
func TestPGStore_BoundingBoxSearch(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvc(pool)

	// Seed 1 sheet 2000×1000.
	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	sheetID := sheetsResult.Items[0].ID

	// Cut 1: produce remnant A (800×400) with bounding_box explicitly set to
	// 600×300 — simulating a chipped corner that reduces the usable area.
	// used 1000×500 + remnant 800×400 = 500_000 + 320_000 = 820_000 ≤ 2_000_000 ✓
	dimA := domain.Dimension{LengthMM: 800, WidthMM: 400}
	bbALen := 600
	bbAWid := 300
	rA, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:             ptrUUID(sheetID),
		WorkOrderID:         uuid.New(),
		SKUID:               uuid.New(),
		UsedDimension:       testDim1000x500,
		RemnantDimension:    &dimA,
		BoundingBoxLengthMM: &bbALen,
		BoundingBoxWidthMM:  &bbAWid,
	})
	if err != nil {
		t.Fatalf("cut remnant A: %v", err)
	}
	if rA.RemnantID == nil {
		t.Fatal("remnant A ID must be set")
	}

	// We need a second source to cut remnant B.  Seed a second sheet.
	svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   testDim2000x1000,
		CostPerSheet: testCost,
		Quantity:     1,
	})
	sheetsResult2, _ := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 1000}, nil)
	// Pick the sheet that hasn't been issued yet.
	var sheetID2 uuid.UUID
	for _, sh := range sheetsResult2.Items {
		if sh.Status == "AVAILABLE" {
			sheetID2 = sh.ID
			break
		}
	}
	if sheetID2 == uuid.Nil {
		t.Fatal("could not find second available sheet")
	}

	// Cut 2: produce remnant B (800×400) with NO explicit bounding_box → defaults
	// to actual dimension 800×400.
	dimB := domain.Dimension{LengthMM: 800, WidthMM: 400}
	rB, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheetID2),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    testDim1000x500,
		RemnantDimension: &dimB,
		// No bounding_box → defaults to 800×400.
	})
	if err != nil {
		t.Fatalf("cut remnant B: %v", err)
	}
	if rB.RemnantID == nil {
		t.Fatal("remnant B ID must be set")
	}

	// ── Test 1: FindAvailableRemnants with minDim = 700×350 ──────────────────
	// Remnant A bounding_box = 600×300 < 700×350 → must be excluded.
	// Remnant B bounding_box = 800×400 ≥ 700×350 → must be included.
	minDim := domain.Dimension{LengthMM: 700, WidthMM: 350}
	found, err := svc.FindAvailableRemnants(context.Background(), minDim)
	if err != nil {
		t.Fatalf("FindAvailableRemnants(700×350): %v", err)
	}
	for _, r := range found {
		if r.ID == *rA.RemnantID {
			t.Error("remnant A (bounding_box 600×300) must NOT appear for minDim 700×350")
		}
	}
	foundB := false
	for _, r := range found {
		if r.ID == *rB.RemnantID {
			foundB = true
		}
	}
	if !foundB {
		t.Error("remnant B (bounding_box 800×400) must appear for minDim 700×350")
	}

	// ── Test 2: FindAvailableRemnants with minDim = 500×250 ──────────────────
	// Both remnants satisfy 500×250 → both must appear.
	minDimSmall := domain.Dimension{LengthMM: 500, WidthMM: 250}
	foundSmall, err := svc.FindAvailableRemnants(context.Background(), minDimSmall)
	if err != nil {
		t.Fatalf("FindAvailableRemnants(500×250): %v", err)
	}
	foundASmall, foundBSmall := false, false
	for _, r := range foundSmall {
		if r.ID == *rA.RemnantID {
			foundASmall = true
		}
		if r.ID == *rB.RemnantID {
			foundBSmall = true
		}
	}
	if !foundASmall {
		t.Error("remnant A (bounding_box 600×300) must appear for minDim 500×250")
	}
	if !foundBSmall {
		t.Error("remnant B (bounding_box 800×400) must appear for minDim 500×250")
	}

	// ── Test 3: ListRemnants with MinLengthMM/MinWidthMM filter ──────────────
	// Using the paginated API with same min-dim as Test 1.
	pageResult, err := svc.ListRemnants(context.Background(),
		RemnantFilter{MinLengthMM: 700, MinWidthMM: 350},
		httpkit.PageParams{Page: 1, Limit: 100},
	)
	if err != nil {
		t.Fatalf("ListRemnants(min 700×350): %v", err)
	}
	for _, r := range pageResult.Items {
		if r.ID == *rA.RemnantID {
			t.Error("remnant A must NOT appear in ListRemnants for min 700×350")
		}
	}
	foundBPaged := false
	for _, r := range pageResult.Items {
		if r.ID == *rB.RemnantID {
			foundBPaged = true
		}
	}
	if !foundBPaged {
		t.Error("remnant B must appear in ListRemnants for min 700×350")
	}
}

func TestIntegration_GetOverflowStatus_AggregatesByStatus(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvcWithThreshold(pool, 15)

	_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   domain.Dimension{LengthMM: 100, WidthMM: 100},
		CostPerSheet: testCost,
		Quantity:     3,
		SupplierRef:  "OVERFLOW-AGG",
	})
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	sheets, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 100}, nil)
	if err != nil {
		t.Fatalf("ListAvailableSheets: %v", err)
	}
	if len(sheets.Items) != 3 {
		t.Fatalf("want 3 sheets, got %d", len(sheets.Items))
	}

	remA := domain.Dimension{LengthMM: 50, WidthMM: 20} // 1000
	cutA, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheets.Items[0].ID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    domain.Dimension{LengthMM: 10, WidthMM: 10},
		RemnantDimension: &remA,
	})
	if err != nil {
		t.Fatalf("RecordCut A: %v", err)
	}

	remB := domain.Dimension{LengthMM: 10, WidthMM: 10} // 100
	cutB, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheets.Items[1].ID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    domain.Dimension{LengthMM: 10, WidthMM: 10},
		RemnantDimension: &remB,
	})
	if err != nil {
		t.Fatalf("RecordCut B: %v", err)
	}

	if cutB.RemnantID == nil {
		t.Fatal("expected remnant ID from cut B")
	}
	if err := svc.AllocateRemnant(context.Background(), *cutB.RemnantID, uuid.New()); err != nil {
		t.Fatalf("AllocateRemnant B: %v", err)
	}

	if cutA.RemnantID == nil {
		t.Fatal("expected remnant ID from cut A")
	}
	_, err = svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:     cutA.RemnantID,
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: domain.Dimension{LengthMM: 10, WidthMM: 10},
	})
	if err != nil {
		t.Fatalf("consume remnant A: %v", err)
	}

	status, err := svc.GetOverflowStatus(context.Background())
	if err != nil {
		t.Fatalf("GetOverflowStatus: %v", err)
	}
	if status.TotalRemnantAreaMM2 != 100 {
		t.Errorf("TotalRemnantAreaMM2 = %d, want 100", status.TotalRemnantAreaMM2)
	}
	if status.TotalSheetAreaMM2 != 10000 {
		t.Errorf("TotalSheetAreaMM2 = %d, want 10000", status.TotalSheetAreaMM2)
	}
	if status.Status != OverflowGreen {
		t.Errorf("Status = %s, want GREEN", status.Status)
	}
}

func TestIntegration_OverflowRed_BlocksSheetIssueAndAllowsRemnantCut(t *testing.T) {
	pool := getPool(t)
	truncateInventory(t)
	matID := seedMaterial(t, pool)
	svc := newSvcWithThreshold(pool, 15)

	_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   matID,
		Dimensions:   domain.Dimension{LengthMM: 100, WidthMM: 100},
		CostPerSheet: testCost,
		Quantity:     2,
		SupplierRef:  "OVERFLOW-RED",
	})
	if err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}

	sheets, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 100}, nil)
	if err != nil {
		t.Fatalf("ListAvailableSheets: %v", err)
	}
	if len(sheets.Items) != 2 {
		t.Fatalf("want 2 sheets, got %d", len(sheets.Items))
	}

	remDim := domain.Dimension{LengthMM: 50, WidthMM: 40} // 2000 => 20%% over one 100x100 available sheet
	cut, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptrUUID(sheets.Items[0].ID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    domain.Dimension{LengthMM: 10, WidthMM: 10},
		RemnantDimension: &remDim,
	})
	if err != nil {
		t.Fatalf("seed RecordCut: %v", err)
	}
	if cut.RemnantID == nil {
		t.Fatal("expected remnant ID from seed cut")
	}

	status, err := svc.GetOverflowStatus(context.Background())
	if err != nil {
		t.Fatalf("GetOverflowStatus: %v", err)
	}
	if status.Status != OverflowRed {
		t.Fatalf("Status = %s, want RED", status.Status)
	}

	remaining, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 100}, nil)
	if err != nil {
		t.Fatalf("ListAvailableSheets(remaining): %v", err)
	}
	if len(remaining.Items) != 1 {
		t.Fatalf("want 1 available remaining sheet, got %d", len(remaining.Items))
	}
	blockedSheetID := remaining.Items[0].ID

	if err := svc.PreAssignSheet(context.Background(), PreAssignSheetInput{
		SheetID:     blockedSheetID,
		WorkOrderID: uuid.New(),
	}); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("PreAssignSheet: want ErrPreconditionFailed, got %v", err)
	}

	_, err = svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptrUUID(blockedSheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: domain.Dimension{LengthMM: 10, WidthMM: 10},
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("RecordCut from sheet: want ErrPreconditionFailed, got %v", err)
	}

	_, err = svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:     cut.RemnantID,
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: domain.Dimension{LengthMM: 10, WidthMM: 10},
	})
	if err != nil {
		t.Fatalf("RecordCut from remnant should be allowed in RED, got %v", err)
	}
}
