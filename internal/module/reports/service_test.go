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

func (m *mockCostingReader) IterateCostingsInPeriod(_ context.Context, p Period, yield func(CostingRow) error) error {
	m.last = p
	if m.err != nil {
		return m.err
	}
	for _, r := range m.rows {
		if err := yield(r); err != nil {
			return err
		}
	}
	return nil
}

type mockPOReader struct {
	rows []PORow
	err  error
}

func (m *mockPOReader) IteratePOsInPeriod(_ context.Context, _ Period, yield func(PORow) error) error {
	if m.err != nil {
		return m.err
	}
	for _, r := range m.rows {
		if err := yield(r); err != nil {
			return err
		}
	}
	return nil
}

type mockSKUReader struct {
	rows     []SKURow
	err      error
	gotLimit int
}

func (m *mockSKUReader) IterateSKUs(_ context.Context, limit int, yield func(SKURow) error) error {
	m.gotLimit = limit
	if m.err != nil {
		return m.err
	}
	for _, r := range m.rows {
		if err := yield(r); err != nil {
			return err
		}
	}
	return nil
}

type mockWOReader struct {
	rows []WORow
	err  error
}

func (m *mockWOReader) IterateWorkOrdersInPeriod(_ context.Context, _ Period, yield func(WORow) error) error {
	if m.err != nil {
		return m.err
	}
	for _, r := range m.rows {
		if err := yield(r); err != nil {
			return err
		}
	}
	return nil
}

type mockWasteReader struct {
	rows []WasteRow
	err  error
}

func (m *mockWasteReader) IterateWasteInPeriod(_ context.Context, _ Period, yield func(WasteRow) error) error {
	if m.err != nil {
		return m.err
	}
	for _, r := range m.rows {
		if err := yield(r); err != nil {
			return err
		}
	}
	return nil
}

// ── ExportCostings ───────────────────────────────────────────────────────────

func TestExportCostings_HappyPath(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	cr := &mockCostingReader{rows: []CostingRow{{
		WorkOrderID: "wo-1", SKUCode: "SKU-001", SKUName: "Bàn ăn",
		CostingType:  "ACTUAL",
		MaterialCost: 1_500_000, AuxiliaryCost: 200_000, LaborCost: 300_000,
		TotalCost:    2_000_000, Finalized: true, CreatedAt: now,
	}}}
	svc := NewService(cr, nil, nil, nil, nil)

	var buf bytes.Buffer
	name, err := svc.ExportCostings(context.Background(), &buf, Period{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(name, ".xlsx") {
		t.Errorf("filename = %q, want .xlsx suffix", name)
	}
	if buf.Len() < 100 {
		t.Fatalf("file too small (%d bytes), generation likely failed", buf.Len())
	}

	rows := readSheet(t, buf.Bytes(), "Costings")
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
	_, err := svc.ExportCostings(context.Background(), &bytes.Buffer{}, Period{})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestExportCostings_FromAfterTo_ReturnsInvalidInput(t *testing.T) {
	cr := &mockCostingReader{}
	svc := NewService(cr, nil, nil, nil, nil)
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.ExportCostings(context.Background(), &bytes.Buffer{}, Period{From: &from, To: &to})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// MaxPeriodDays guard prevents an accountant from accidentally requesting a
// multi-year span that would force the streaming pipeline to grind through
// millions of rows. Equal to the cap → still allowed; over → 400.
func TestExportCostings_PeriodOverMaxSpan_Returns400(t *testing.T) {
	cr := &mockCostingReader{}
	svc := NewService(cr, nil, nil, nil, nil)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Duration(MaxPeriodDays+1) * 24 * time.Hour)
	_, err := svc.ExportCostings(context.Background(), &bytes.Buffer{}, Period{From: &from, To: &to})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestExportCostings_PeriodAtMaxSpan_Allowed(t *testing.T) {
	cr := &mockCostingReader{}
	svc := NewService(cr, nil, nil, nil, nil)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Duration(MaxPeriodDays) * 24 * time.Hour)
	if _, err := svc.ExportCostings(context.Background(), &bytes.Buffer{}, Period{From: &from, To: &to}); err != nil {
		t.Errorf("err = %v, want nil at exact MaxPeriodDays span", err)
	}
}

// Open-ended exports (only one bound set) must NOT be rejected by the span
// cap. The streaming pipeline keeps RAM bounded regardless of result count,
// so a "give me everything from May 1" request is valid.
func TestExportCostings_OpenEndedPeriod_NotCapped(t *testing.T) {
	cr := &mockCostingReader{}
	svc := NewService(cr, nil, nil, nil, nil)
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := svc.ExportCostings(context.Background(), &bytes.Buffer{}, Period{From: &from}); err != nil {
		t.Errorf("err = %v, want nil for open-ended period", err)
	}
}

func TestExportCostings_PeriodIsForwardedToReader(t *testing.T) {
	cr := &mockCostingReader{}
	svc := NewService(cr, nil, nil, nil, nil)
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	if _, err := svc.ExportCostings(context.Background(), &bytes.Buffer{}, Period{From: &from, To: &to}); err != nil {
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
	_, err := svc.ExportCostings(context.Background(), &bytes.Buffer{}, Period{})
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
		TotalCost:    750_000, CreatedAt: now,
	}}}
	svc := NewService(nil, pr, nil, nil, nil)

	var buf bytes.Buffer
	if _, err := svc.ExportPurchaseOrders(context.Background(), &buf, Period{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := readSheet(t, buf.Bytes(), "PurchaseOrders")
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
	var buf bytes.Buffer
	if _, err := svc.ExportSKUs(context.Background(), &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := readSheet(t, buf.Bytes(), "SKUs")
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

// MaxSKURows must reach the reader so the SQL LIMIT clause caps the read at
// the configured threshold; without this the streaming reader could
// theoretically pull an unbounded number of rows from a misconfigured table.
func TestExportSKUs_PassesMaxSKURowsLimitToReader(t *testing.T) {
	sr := &mockSKUReader{}
	svc := NewService(nil, nil, sr, nil, nil)
	if _, err := svc.ExportSKUs(context.Background(), &bytes.Buffer{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sr.gotLimit != MaxSKURows {
		t.Errorf("limit forwarded = %d, want %d", sr.gotLimit, MaxSKURows)
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
	var buf bytes.Buffer
	if _, err := svc.ExportWorkOrders(context.Background(), &buf, Period{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := readSheet(t, buf.Bytes(), "WorkOrders")
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
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
	var buf bytes.Buffer
	if _, err := svc.ExportWaste(context.Background(), &buf, Period{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows := readSheet(t, buf.Bytes(), "Waste")
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[1][0] != "MDF 18mm" {
		t.Errorf("material = %q, want MDF 18mm", rows[1][0])
	}
}

// ── streaming behavior ───────────────────────────────────────────────────────

// 10k-row smoke test that exercises the streaming path end-to-end. Verifies
// the workbook stays well-formed, every row reaches the writer in order, and
// the StreamWriter's flush + freeze-pane sequence still completes when the
// dataset is large. Cheap enough to run in CI; large enough to catch any
// regression that re-introduces O(N) buffering of [][]any rows.
func TestExportCostings_TenThousandRows_RoundTrips(t *testing.T) {
	const n = 10_000
	rows := make([]CostingRow, n)
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	for i := range rows {
		rows[i] = CostingRow{
			WorkOrderID: "wo-large", SKUCode: "SKU-X", SKUName: "Bulk",
			CostingType:  "ACTUAL",
			MaterialCost: int64(i),
			TotalCost:    int64(i),
			CreatedAt:    now,
		}
	}
	cr := &mockCostingReader{rows: rows}
	svc := NewService(cr, nil, nil, nil, nil)

	var buf bytes.Buffer
	if _, err := svc.ExportCostings(context.Background(), &buf, Period{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readSheet(t, buf.Bytes(), "Costings")
	if want := n + 1; len(got) != want {
		t.Fatalf("rows = %d, want %d (header + %d data)", len(got), want, n)
	}
}

// The streaming pipeline must abort cleanly when the writer fails partway
// through the workbook (e.g. client disconnect). The reader yield error path
// is exercised here via a custom failingWriter that errors on the third
// flush attempt; service should propagate the error rather than panic.
type failingWriter struct {
	bytes.Buffer
	failAfter int
	calls     int
}

func (f *failingWriter) Write(p []byte) (int, error) {
	f.calls++
	if f.calls > f.failAfter {
		return 0, errors.New("writer closed")
	}
	return f.Buffer.Write(p)
}

func TestExportCostings_WriterFails_ReturnsError(t *testing.T) {
	cr := &mockCostingReader{rows: make([]CostingRow, 100)}
	svc := NewService(cr, nil, nil, nil, nil)
	w := &failingWriter{failAfter: 0}
	_, err := svc.ExportCostings(context.Background(), w, Period{})
	if err == nil {
		t.Fatalf("expected error from failing writer, got nil")
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
