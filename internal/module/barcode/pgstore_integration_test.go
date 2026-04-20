//go:build integration

package barcode

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

func truncateBarcode(t *testing.T) {
	t.Helper()
	testhelper.TruncateAll(t, sharedPool)
}

func seedBarcodeFixture(t *testing.T, pool *pgxpool.Pool) Barcode {
	t.Helper()
	ctx := context.Background()

	skuID := uuid.New()
	poID := uuid.New()
	planID := uuid.New()
	woID := uuid.New()

	if _, err := pool.Exec(ctx,
		`INSERT INTO skus (id, code, name, length_mm, width_mm, requires_metal, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())`,
		skuID, "SKU-BARCODE-INT", "Barcode Integration SKU", 1200, 600, false,
	); err != nil {
		t.Fatalf("seed sku: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO purchase_orders (id, code, expected_delivery, created_at)
		 VALUES ($1, $2, now()::date, now())`,
		poID, "PO-BARCODE-INT-001",
	); err != nil {
		t.Fatalf("seed purchase order: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO production_plans (id, po_id, status, deadline, created_at)
		 VALUES ($1, $2, 'APPROVED', now()::date, now())`,
		planID, poID,
	); err != nil {
		t.Fatalf("seed production plan: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO work_orders (id, plan_id, sku_id, quantity, status, created_at)
		 VALUES ($1, $2, $3, $4, 'IN_CUTTING', now())`,
		woID, planID, skuID, 10,
	); err != nil {
		t.Fatalf("seed work order: %v", err)
	}

	return Barcode{
		ID:               uuid.New(),
		WorkOrderID:      woID,
		SKUID:            skuID,
		POID:             poID,
		ProductionPlanID: planID,
		SKUCode:          "SKU-BARCODE-INT",
		SKUName:          "Barcode Integration SKU",
		Dimensions:       "1200x600",
		ProducedDate:     time.Now().UTC(),
		CreatedAt:        time.Now().UTC(),
	}
}

func TestIntegration_PGStore_InsertAndListScanMetadata(t *testing.T) {
	pool := getPool(t)
	truncateBarcode(t)

	ctx := context.Background()
	st := NewPGStore(pool)
	bc := seedBarcodeFixture(t, pool)
	if err := st.insertBarcode(ctx, bc); err != nil {
		t.Fatalf("insertBarcode: %v", err)
	}

	want := ScanEvent{
		ID:         uuid.New(),
		BarcodeID:  bc.ID,
		Checkpoint: CheckpointCNCComplete,
		ScannedBy:  uuid.New(),
		Location:   "Cutting Line A",
		Note:       "Operator confirmed",
		DeviceID:   "CNC-01",
		DeviceName: "Kiosk A",
		Shift:      "Morning",
		ScannedAt:  time.Now().UTC(),
	}
	if err := st.insertScanEvent(ctx, want); err != nil {
		t.Fatalf("insertScanEvent: %v", err)
	}

	events, err := st.selectScanEventsByBarcode(ctx, bc.ID)
	if err != nil {
		t.Fatalf("selectScanEventsByBarcode: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	got := events[0]
	if got.DeviceID != want.DeviceID {
		t.Errorf("DeviceID = %q, want %q", got.DeviceID, want.DeviceID)
	}
	if got.DeviceName != want.DeviceName {
		t.Errorf("DeviceName = %q, want %q", got.DeviceName, want.DeviceName)
	}
	if got.Shift != want.Shift {
		t.Errorf("Shift = %q, want %q", got.Shift, want.Shift)
	}
}

func TestIntegration_PGStore_SelectLastScanEvent_WithNullMetadata(t *testing.T) {
	pool := getPool(t)
	truncateBarcode(t)

	ctx := context.Background()
	st := NewPGStore(pool)
	bc := seedBarcodeFixture(t, pool)
	if err := st.insertBarcode(ctx, bc); err != nil {
		t.Fatalf("insertBarcode: %v", err)
	}

	first := ScanEvent{
		ID:         uuid.New(),
		BarcodeID:  bc.ID,
		Checkpoint: CheckpointCNCComplete,
		ScannedBy:  uuid.New(),
		DeviceID:   "",
		DeviceName: "",
		Shift:      "",
		ScannedAt:  time.Now().UTC().Add(-time.Minute),
	}
	if err := st.insertScanEvent(ctx, first); err != nil {
		t.Fatalf("insert first scan: %v", err)
	}

	second := ScanEvent{
		ID:         uuid.New(),
		BarcodeID:  bc.ID,
		Checkpoint: CheckpointFinishedGoods,
		ScannedBy:  uuid.New(),
		DeviceID:   "CNC-02",
		DeviceName: "Kiosk B",
		Shift:      "Night",
		ScannedAt:  time.Now().UTC(),
	}
	if err := st.insertScanEvent(ctx, second); err != nil {
		t.Fatalf("insert second scan: %v", err)
	}

	last, err := st.selectLastScanEventByBarcode(ctx, bc.ID)
	if err != nil {
		t.Fatalf("selectLastScanEventByBarcode: %v", err)
	}
	if last.ID != second.ID {
		t.Fatalf("last.ID = %v, want %v", last.ID, second.ID)
	}
	if last.DeviceID != second.DeviceID || last.DeviceName != second.DeviceName || last.Shift != second.Shift {
		t.Fatalf("last metadata mismatch: got (%q, %q, %q)", last.DeviceID, last.DeviceName, last.Shift)
	}

	var (
		dID   *string
		dName *string
		shift *string
	)
	if err := pool.QueryRow(ctx,
		`SELECT device_id, device_name, shift FROM scan_events WHERE id = $1`,
		first.ID,
	).Scan(&dID, &dName, &shift); err != nil {
		t.Fatalf("query first scan metadata: %v", err)
	}
	if dID != nil || dName != nil || shift != nil {
		t.Fatalf("expected NULL metadata, got device_id=%v device_name=%v shift=%v", dID, dName, shift)
	}
}
