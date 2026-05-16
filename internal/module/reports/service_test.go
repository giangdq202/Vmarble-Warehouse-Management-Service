package reports

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/xuri/excelize/v2"
)

// readSheet opens the generated workbook and returns rows from the named sheet.
// The caller asserts on cell content rather than picking apart the binary blob.
func readSheet(t *testing.T, raw []byte, sheet string) [][]string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("open generated xlsx: %v", err)
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("get rows from %s: %v", sheet, err)
	}
	return rows
}

// ── mocks ────────────────────────────────────────────────────────────────────

type mockCostingReader struct {
	rows []CostingRow
	err  error
	last Period
}

func (m *mockCostingReader) ListCostingsInPeriod(_ context.Context, p Period) ([]CostingRow, error) {
	m.last = p
	return m.rows, m.err
}

type mockPOReader struct {
	rows []PORow
	err  error
}

func (m *mockPOReader) ListPOsInPeriod(_ context.Context, _ Period) ([]PORow, error) {
	return m.rows, m.err
}

type mockSKUReader struct {
	rows []SKURow
	err  error
}

func (m *mockSKUReader) ListAllSKUs(_ context.Context) ([]SKURow, error) {
	return m.rows, m.err
}

type mockWOReader struct {
	rows []WORow
	err  error
}

func (m *mockWOReader) ListWorkOrdersInPeriod(_ context.Context, _ Period) ([]WORow, error) {
	return m.rows, m.err
}

type mockWasteReader struct {
	rows []WasteRow
	err  error
}

func (m *mockWasteReader) ListWasteInPeriod(_ context.Context, _ Period) ([]WasteRow, error) {
	return m.rows, m.err
}

// ── ExportCostings ───────────────────────────────────────────────────────────

func TestExportCostings_HappyPath(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	cr := &mockCostingReader{rows: []CostingRow{{
		WorkOrderID: "wo-1", SKUCode: "SKU-001", SKUName: "Bàn ăn",
		CostingType: "ACTUAL",
		MaterialCost: 1_500_000, AuxiliaryCost: 200_000, LaborCost: 300_000,
		TotalCost: 2_000_000, Finalized: true, CreatedAt: now,
	}}}
	svc := NewService(cr, nil, nil, nil, nil)

	file, err := svc.ExportCostings(context.Background(), Period{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(file.Filename, ".xlsx") {
		t.Errorf("Filename = %q, want .xlsx suffix", file.Filename)
	}
	if len(file.Bytes) < 100 {
		t.Fatalf("file too small (%d bytes), generation likely failed", len(file.Bytes))
	}

	rows := readSheet(t, file.Bytes, "Costings")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (header + 1 data)", len(rows))
	}
	wantHeader := []string{"Mã WO", "Mã SKU", "Tên SKU", "Loại"}
	for i, want := range wantHeader {
		if rows[0][i] != want {
			t.Errorf("header[%d] = %q, want %q", i, rows[0][i], want)
		}
	}
	if rows[1][0] != "wo-1" || rows[1][1] != "SKU-001" {
		t.Errorf("data row mismatch: %v", rows[1])
	}
	if rows[1][9] != "14/05/2026" {
		t.Errorf("created_at = %q, want 14/05/2026", rows[1][9])
	}
}

func TestExportCostings_NoReader_ReturnsInvalidInput(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil)
	_, err := svc.ExportCostings(context.Background(), Period{})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestExportCostings_FromAfterTo_ReturnsInvalidInput(t *testing.T) {
	cr := &mockCostingReader{}
	svc := NewService(cr, nil, nil, nil, nil)
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.ExportCostings(context.Background(), Period{From: &from, To: &to})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestExportCostings_PeriodIsForwardedToReader(t *testing.T) {
	cr := &mockCostingReader{}
	svc := NewService(cr, nil, nil, nil, nil)
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	if _, err := svc.ExportCostings(context.Background(), Period{From: &from, To: &to}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cr.last.From == nil || !cr.last.From.Equal(from) {
		t.Errorf("From not forwarded: %+v", cr.last.From)
	}
	if cr.last.To == nil || !cr.last.To.Equal(to) {
		t.Errorf("To not forwarded: %+v", cr.last.To)
	}
}

func TestExportCostings_ReaderError_Propagates(t *testing.T) {
	want := errors.New("db down")
	cr := &mockCostingReader{err: want}
	svc := NewService(cr, nil, nil, nil, nil)
	_, err := svc.ExportCostings(context.Background(), Period{})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

// ── ExportPurchaseOrders ─────────────────────────────────────────────────────

func TestExportPurchaseOrders_HappyPath(t *testing.T) {
	now := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	pr := &mockPOReader{rows: []PORow{{
		Code: "PO-100", Supplier: "ACME", Status: "ORDERED",
		MaterialName: "MDF 18mm", OrderedAt: &now, Items: "5×1000×600 @150000",
		TotalCost: 750_000, CreatedAt: now,
	}}}
	svc := NewService(nil, pr, nil, nil, nil)

	file, err := svc.ExportPurchaseOrders(context.Background(), Period{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := readSheet(t, file.Bytes, "PurchaseOrders")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[1][0] != "PO-100" || rows[1][1] != "ACME" {
		t.Errorf("data row mismatch: %v", rows[1])
	}
}

// ── ExportSKUs ───────────────────────────────────────────────────────────────

func TestExportSKUs_HappyPath(t *testing.T) {
	sr := &mockSKUReader{rows: []SKURow{{
		Code: "SKU-001", Name: "Bàn ăn", LengthMM: 1200, WidthMM: 600,
		RequiresMetal: false, IsActive: true, BOMSummary: "MDF 18mm × 0.5",
	}}}
	svc := NewService(nil, nil, sr, nil, nil)
	file, err := svc.ExportSKUs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := readSheet(t, file.Bytes, "SKUs")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[1][0] != "SKU-001" {
		t.Errorf("code = %q, want SKU-001", rows[1][0])
	}
	if rows[1][4] != "Không" { // RequiresMetal=false → "Không"
		t.Errorf("requires_metal cell = %q, want Không", rows[1][4])
	}
}

// ── ExportWorkOrders ─────────────────────────────────────────────────────────

func TestExportWorkOrders_HappyPath_WithAndWithoutCost(t *testing.T) {
	now := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	cost := int64(2_500_000)
	wr := &mockWOReader{rows: []WORow{
		{ID: "wo-A", SKUCode: "SKU-001", SKUName: "Bàn", Quantity: 5,
			Status: "COMPLETED", CreatedAt: now, TotalCost: &cost},
		{ID: "wo-B", SKUCode: "SKU-002", SKUName: "Ghế", Quantity: 10,
			Status: "PLANNED", CreatedAt: now, TotalCost: nil},
	}}
	svc := NewService(nil, nil, nil, wr, nil)
	file, err := svc.ExportWorkOrders(context.Background(), Period{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := readSheet(t, file.Bytes, "WorkOrders")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	// wo-B has no cost → cell may be absent or empty
	if len(rows[2]) > 7 && rows[2][7] != "" {
		t.Errorf("wo-B cost cell = %q, want empty", rows[2][7])
	}
}

// ── ExportWaste ──────────────────────────────────────────────────────────────

func TestExportWaste_HappyPath(t *testing.T) {
	wr := &mockWasteReader{rows: []WasteRow{{
		MaterialName: "MDF 18mm", SheetsConsumed: 12,
		WasteAreaMM2: 4_500_000, AvgSheetCost: 150_000, TotalWasteCost: 200_000,
	}}}
	svc := NewService(nil, nil, nil, nil, wr)
	file, err := svc.ExportWaste(context.Background(), Period{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := readSheet(t, file.Bytes, "Waste")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[1][0] != "MDF 18mm" {
		t.Errorf("material = %q, want MDF 18mm", rows[1][0])
	}
}

// ── filename ─────────────────────────────────────────────────────────────────

func TestFilename_PrefixesAndStampsPeriod(t *testing.T) {
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	got := filename("costings", Period{From: &from, To: &to})
	want := "costings-20260501-20260515.xlsx"
	if got != want {
		t.Errorf("filename = %q, want %q", got, want)
	}
}

func TestFilename_NoPeriod_FallsBackToToday(t *testing.T) {
	got := filename("skus", Period{})
	if !strings.HasPrefix(got, "skus-") || !strings.HasSuffix(got, ".xlsx") {
		t.Errorf("filename = %q, want skus-YYYYMMDD.xlsx", got)
	}
}
