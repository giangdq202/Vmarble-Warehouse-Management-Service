package barcode

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

// ── mockStore ─────────────────────────────────────────────────────────────────

type mockStore struct {
	// insertBarcode
	insertBarcodeErr error

	// selectBarcodeByID
	selectBarcodeByIDResult Barcode
	selectBarcodeByIDErr    error

	// insertScanEvent — captures the event written for assertion
	insertScanEventCalled bool
	insertScanEventResult ScanEvent
	insertScanEventErr    error

	// selectBarcodesByWorkOrder
	selectBarcodesByWorkOrderResult []Barcode
	selectBarcodesByWorkOrderErr    error

	// selectScanEventsByBarcode
	selectScanEventsResult []ScanEvent
	selectScanEventsErr    error
}

func (m *mockStore) insertBarcode(_ context.Context, _ Barcode) error {
	return m.insertBarcodeErr
}

func (m *mockStore) selectBarcodeByID(_ context.Context, _ uuid.UUID) (Barcode, error) {
	return m.selectBarcodeByIDResult, m.selectBarcodeByIDErr
}

func (m *mockStore) selectBarcodesByWorkOrder(_ context.Context, _ uuid.UUID) ([]Barcode, error) {
	return m.selectBarcodesByWorkOrderResult, m.selectBarcodesByWorkOrderErr
}

func (m *mockStore) insertScanEvent(_ context.Context, e ScanEvent) error {
	m.insertScanEventCalled = true
	m.insertScanEventResult = e
	return m.insertScanEventErr
}

func (m *mockStore) selectScanEventsByBarcode(_ context.Context, _ uuid.UUID) ([]ScanEvent, error) {
	return m.selectScanEventsResult, m.selectScanEventsErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func validGenerateInput() GenerateBarcodeInput {
	return GenerateBarcodeInput{
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		POID:             uuid.New(),
		ProductionPlanID: uuid.New(),
		SKUCode:          "SKU-TABLE-001",
		SKUName:          "Bàn gỗ sồi",
		Dimensions:       "1200x800x750",
		ProducedDate:     time.Now().UTC(),
	}
}

func storedBarcode(id uuid.UUID) Barcode {
	return Barcode{
		ID:          id,
		WorkOrderID: uuid.New(),
		SKUCode:     "SKU-CHAIR-002",
		CreatedAt:   time.Now().UTC(),
	}
}

// ── TestGenerateBarcode ───────────────────────────────────────────────────────

func TestGenerateBarcode_HappyPath_ReturnsWithID(t *testing.T) {
	in := validGenerateInput()
	svc := NewService(&mockStore{})

	b, err := svc.GenerateBarcode(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if b.ID == uuid.Nil {
		t.Error("Barcode ID must be set")
	}
	if b.WorkOrderID != in.WorkOrderID {
		t.Errorf("WorkOrderID = %v, want %v", b.WorkOrderID, in.WorkOrderID)
	}
	if b.SKUCode != in.SKUCode {
		t.Errorf("SKUCode = %q, want %q", b.SKUCode, in.SKUCode)
	}
	if b.SKUName != in.SKUName {
		t.Errorf("SKUName = %q, want %q", b.SKUName, in.SKUName)
	}
	if b.Dimensions != in.Dimensions {
		t.Errorf("Dimensions = %q, want %q", b.Dimensions, in.Dimensions)
	}
	if b.CreatedAt.IsZero() {
		t.Error("CreatedAt must be set")
	}
}

func TestGenerateBarcode_EmptySKUCode_ReturnsErrInvalidInput(t *testing.T) {
	in := validGenerateInput()
	in.SKUCode = ""

	svc := NewService(&mockStore{})
	_, err := svc.GenerateBarcode(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty SKUCode, got %v", err)
	}
}

func TestGenerateBarcode_NilWorkOrderID_ReturnsErrInvalidInput(t *testing.T) {
	in := validGenerateInput()
	in.WorkOrderID = uuid.Nil

	svc := NewService(&mockStore{})
	_, err := svc.GenerateBarcode(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for nil WorkOrderID, got %v", err)
	}
}

func TestGenerateBarcode_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("insert barcode failed")
	st := &mockStore{insertBarcodeErr: dbErr}

	svc := NewService(st)
	_, err := svc.GenerateBarcode(context.Background(), validGenerateInput())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected insertBarcode error to propagate, got %v", err)
	}
}

// ── TestLookupBarcode ─────────────────────────────────────────────────────────

func TestLookupBarcode_HappyPath(t *testing.T) {
	barcodeID := uuid.New()
	want := storedBarcode(barcodeID)
	st := &mockStore{selectBarcodeByIDResult: want}

	svc := NewService(st)
	got, err := svc.LookupBarcode(context.Background(), barcodeID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != barcodeID {
		t.Errorf("ID = %v, want %v", got.ID, barcodeID)
	}
	if got.SKUCode != want.SKUCode {
		t.Errorf("SKUCode = %q, want %q", got.SKUCode, want.SKUCode)
	}
}

func TestLookupBarcode_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectBarcodeByIDErr: domain.NewBizError(domain.ErrNotFound, "barcode not found"),
	}

	svc := NewService(st)
	_, err := svc.LookupBarcode(context.Background(), uuid.New())

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

// ── TestRecordScan ────────────────────────────────────────────────────────────

func TestRecordScan_AllCheckpoints_Succeed(t *testing.T) {
	checkpoints := []ScanCheckpoint{
		CheckpointCNCComplete,
		CheckpointFinishedGoods,
		CheckpointShipped,
	}

	for _, cp := range checkpoints {
		cp := cp
		t.Run(string(cp), func(t *testing.T) {
			barcodeID := uuid.New()
			st := &mockStore{
				selectBarcodeByIDResult: storedBarcode(barcodeID),
			}
			svc := NewService(st)

			in := RecordScanInput{
				BarcodeID:  barcodeID,
				Checkpoint: cp,
				ScannedBy:  "worker-01",
			}
			event, err := svc.RecordScan(context.Background(), in)
			if err != nil {
				t.Fatalf("checkpoint %s: unexpected error: %v", cp, err)
			}

			if event.ID == uuid.Nil {
				t.Error("ScanEvent ID must be set")
			}
			if event.BarcodeID != barcodeID {
				t.Errorf("BarcodeID = %v, want %v", event.BarcodeID, barcodeID)
			}
			if event.Checkpoint != cp {
				t.Errorf("Checkpoint = %q, want %q", event.Checkpoint, cp)
			}
			if event.ScannedBy != "worker-01" {
				t.Errorf("ScannedBy = %q, want %q", event.ScannedBy, "worker-01")
			}
			if event.ScannedAt.IsZero() {
				t.Error("ScannedAt must be set")
			}
			if !st.insertScanEventCalled {
				t.Error("insertScanEvent must be called")
			}
		})
	}
}

func TestRecordScan_InvalidCheckpoint_ReturnsErrInvalidInput(t *testing.T) {
	invalidCheckpoints := []ScanCheckpoint{
		"",
		"UNKNOWN",
		"cnc_complete",  // wrong case
		"CNC-COMPLETE",  // wrong separator
		"FINISHED_GOOD", // typo
	}

	for _, cp := range invalidCheckpoints {
		cp := cp
		t.Run(string(cp)+"_is_invalid", func(t *testing.T) {
			svc := NewService(&mockStore{})
			_, err := svc.RecordScan(context.Background(), RecordScanInput{
				BarcodeID:  uuid.New(),
				Checkpoint: cp,
				ScannedBy:  "worker-01",
			})

			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("checkpoint %q: expected ErrInvalidInput, got %v", cp, err)
			}
		})
	}
}

func TestRecordScan_BarcodeNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectBarcodeByIDErr: domain.NewBizError(domain.ErrNotFound, "barcode not found"),
	}

	svc := NewService(st)
	_, err := svc.RecordScan(context.Background(), RecordScanInput{
		BarcodeID:  uuid.New(),
		Checkpoint: CheckpointCNCComplete,
		ScannedBy:  "worker-01",
	})

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
	if st.insertScanEventCalled {
		t.Error("insertScanEvent must NOT be called when barcode is not found")
	}
}

func TestRecordScan_StoreInsertError_Propagates(t *testing.T) {
	dbErr := errors.New("insert scan event failed")
	barcodeID := uuid.New()
	st := &mockStore{
		selectBarcodeByIDResult: storedBarcode(barcodeID),
		insertScanEventErr:      dbErr,
	}

	svc := NewService(st)
	_, err := svc.RecordScan(context.Background(), RecordScanInput{
		BarcodeID:  barcodeID,
		Checkpoint: CheckpointShipped,
		ScannedBy:  "worker-02",
	})

	if !errors.Is(err, dbErr) {
		t.Errorf("expected insertScanEvent error to propagate, got %v", err)
	}
}

// RecordScan must check checkpoint validity BEFORE querying the barcode store,
// so an invalid checkpoint must never reach selectBarcodeByID.
func TestRecordScan_InvalidCheckpoint_DoesNotQueryStore(t *testing.T) {
	// If selectBarcodeByID were called, the mock would return its zero-value
	// Barcode which has no error — so if we still get ErrInvalidInput we know
	// the guard fired before the store call.
	st := &mockStore{}

	svc := NewService(st)
	_, err := svc.RecordScan(context.Background(), RecordScanInput{
		BarcodeID:  uuid.New(),
		Checkpoint: "BOGUS",
	})

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput before store call, got %v", err)
	}
}

// ── TestListScans ─────────────────────────────────────────────────────────────

func TestListScans_HappyPath_ReturnsScanEvents(t *testing.T) {
	barcodeID := uuid.New()
	events := []ScanEvent{
		{ID: uuid.New(), BarcodeID: barcodeID, Checkpoint: CheckpointCNCComplete, ScannedAt: time.Now().UTC()},
		{ID: uuid.New(), BarcodeID: barcodeID, Checkpoint: CheckpointFinishedGoods, ScannedAt: time.Now().UTC()},
	}
	st := &mockStore{selectScanEventsResult: events}

	svc := NewService(st)
	got, err := svc.ListScans(context.Background(), barcodeID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
	for i, e := range got {
		if e.BarcodeID != barcodeID {
			t.Errorf("events[%d].BarcodeID = %v, want %v", i, e.BarcodeID, barcodeID)
		}
	}
}

func TestListScans_Empty_ReturnsNil(t *testing.T) {
	st := &mockStore{selectScanEventsResult: nil}

	svc := NewService(st)
	got, err := svc.ListScans(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d events", len(got))
	}
}

func TestListScans_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("select scan events failed")
	st := &mockStore{selectScanEventsErr: dbErr}

	svc := NewService(st)
	_, err := svc.ListScans(context.Background(), uuid.New())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected selectScanEventsByBarcode error to propagate, got %v", err)
	}
}

// ── TestGenerateQRCode ────────────────────────────────────────────────────────

func TestGenerateQRCode_HappyPath_ReturnsPNGBytes(t *testing.T) {
	id := uuid.New()
	bc := storedBarcode(id)
	bc.SKUCode = "SKU-001"
	bc.Dimensions = "1200x600"
	bc.POID = uuid.New()

	st := &mockStore{selectBarcodeByIDResult: bc}
	svc := NewService(st)

	png, err := svc.GenerateQRCode(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// PNG magic bytes: 0x89 0x50 0x4E 0x47
	if len(png) < 4 || png[0] != 0x89 || png[1] != 0x50 || png[2] != 0x4E || png[3] != 0x47 {
		t.Errorf("response is not a valid PNG (first bytes: %v)", png[:4])
	}
}

func TestGenerateQRCode_BarcodeNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectBarcodeByIDErr: domain.NewBizError(domain.ErrNotFound, "barcode not found")}
	svc := NewService(st)

	_, err := svc.GenerateQRCode(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ── TestGenerateLabelPDF ───────────────────────────────────────────────────────

func TestGenerateLabelPDF_50x30_HappyPath_ReturnsPDFBytes(t *testing.T) {
	id := uuid.New()
	bc := storedBarcode(id)
	bc.SKUCode = "SKU-001"
	bc.Dimensions = "1200x600"
	bc.POID = uuid.New()
	bc.WorkOrderID = uuid.New()

	st := &mockStore{selectBarcodeByIDResult: bc}
	svc := NewService(st)

	pdf, err := svc.GenerateLabelPDF(context.Background(), id, LabelSize50x30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pdf) < 5 || string(pdf[:5]) != "%PDF-" {
		t.Fatalf("response is not a valid PDF header, got: %q", string(pdf[:5]))
	}
}

func TestGenerateLabelPDF_100x70_HappyPath_ReturnsPDFBytes(t *testing.T) {
	id := uuid.New()
	bc := storedBarcode(id)
	bc.SKUCode = "SKU-002"
	bc.Dimensions = "2000x800"
	bc.POID = uuid.New()
	bc.WorkOrderID = uuid.New()

	st := &mockStore{selectBarcodeByIDResult: bc}
	svc := NewService(st)

	pdf, err := svc.GenerateLabelPDF(context.Background(), id, LabelSize100x70)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pdf) < 5 || string(pdf[:5]) != "%PDF-" {
		t.Fatalf("response is not a valid PDF header, got: %q", string(pdf[:5]))
	}
}

func TestGenerateLabelPDF_InvalidSize_ReturnsErrInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{})

	_, err := svc.GenerateLabelPDF(context.Background(), uuid.New(), LabelSize("A4"))
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestGenerateLabelPDF_BarcodeNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectBarcodeByIDErr: domain.NewBizError(domain.ErrNotFound, "barcode not found")}
	svc := NewService(st)

	_, err := svc.GenerateLabelPDF(context.Background(), uuid.New(), LabelSize50x30)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
