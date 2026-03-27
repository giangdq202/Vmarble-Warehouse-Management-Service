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
func newSvc(pool *pgxpool.Pool) Service {
	return NewService(NewPGStore(pool))
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

	sheets, err := svc.ListAvailableSheets(context.Background())
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

	lots, err := svc.ListLots(context.Background())
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
	sheets, _ := svc.ListAvailableSheets(context.Background())
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
	sheets, _ := svc.ListAvailableSheets(context.Background())
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
	sheets, _ := svc.ListAvailableSheets(context.Background())
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
	sheets, _ := svc.ListAvailableSheets(context.Background())
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
	sheets, _ := svc.ListAvailableSheets(context.Background())
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
	sheets, _ := svc.ListAvailableSheets(context.Background())
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
	sheets, _ := svc.ListAvailableSheets(context.Background())
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
	sheets, _ := svc.ListAvailableSheets(context.Background())
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
	sheets, _ := svc.ListAvailableSheets(context.Background())
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
