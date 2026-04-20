package barcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/png"
	"math"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liyue201/goqr"
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

	// selectBarcodesByIDsOrdered
	selectBarcodesByIDsOrderedResult []Barcode
	selectBarcodesByIDsOrderedErr    error
	selectBarcodesByIDsOrderedArg    []uuid.UUID

	// selectScanEventsByBarcode
	selectScanEventsResult []ScanEvent
	selectScanEventsErr    error

	// selectLastScanEventByBarcode
	selectLastScanEventResult ScanEvent
	selectLastScanEventErr    error
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

func (m *mockStore) selectBarcodesByIDsOrdered(_ context.Context, ids []uuid.UUID) ([]Barcode, error) {
	m.selectBarcodesByIDsOrderedArg = append([]uuid.UUID(nil), ids...)
	return m.selectBarcodesByIDsOrderedResult, m.selectBarcodesByIDsOrderedErr
}

func (m *mockStore) insertScanEvent(_ context.Context, e ScanEvent) error {
	m.insertScanEventCalled = true
	m.insertScanEventResult = e
	return m.insertScanEventErr
}

func (m *mockStore) selectScanEventsByBarcode(_ context.Context, _ uuid.UUID) ([]ScanEvent, error) {
	return m.selectScanEventsResult, m.selectScanEventsErr
}

func (m *mockStore) selectLastScanEventByBarcode(_ context.Context, _ uuid.UUID) (ScanEvent, error) {
	return m.selectLastScanEventResult, m.selectLastScanEventErr
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

func parsePageSizeMMFromPDF(t *testing.T, pdf []byte) (float64, float64) {
	t.Helper()
	re := regexp.MustCompile(`/MediaBox\s*\[\s*0\s+0\s+([0-9]+(?:\.[0-9]+)?)\s+([0-9]+(?:\.[0-9]+)?)\s*\]`)
	m := re.FindSubmatch(pdf)
	if len(m) != 3 {
		t.Fatalf("MediaBox not found in PDF")
	}
	wPt, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		t.Fatalf("parse MediaBox width: %v", err)
	}
	hPt, err := strconv.ParseFloat(string(m[2]), 64)
	if err != nil {
		t.Fatalf("parse MediaBox height: %v", err)
	}
	const ptToMM = 25.4 / 72.0
	return wPt * ptToMM, hPt * ptToMM
}

func assertCloseMM(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.2 {
		t.Fatalf("size mismatch: got %.3fmm, want %.3fmm", got, want)
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
	barcodeID := uuid.New()
	scannedBy := uuid.New()
	st := &mockStore{
		selectBarcodeByIDResult: storedBarcode(barcodeID),
		selectLastScanEventErr:  domain.NewBizError(domain.ErrNotFound, "scan event not found"),
	}
	wo := &mockWOGateway{status: domain.WOInCutting}
	svc := NewService(st, wo)

	in1 := RecordScanInput{
		BarcodeID:  barcodeID,
		Checkpoint: CheckpointCNCComplete,
		ScannedBy:  scannedBy,
		DeviceID:   "  CNC-01  ",
		DeviceName: "  Kiosk A  ",
		Shift:      "  Morning  ",
		Location:   "  Zone 1  ",
		Note:       "  first  ",
	}
	res1, err := svc.RecordScan(context.Background(), in1)
	if err != nil {
		t.Fatalf("checkpoint %s: unexpected error: %v", CheckpointCNCComplete, err)
	}
	if res1.ScanID == uuid.Nil {
		t.Error("ScanID must be set")
	}
	if res1.NextCheckpoint == nil || *res1.NextCheckpoint != CheckpointFinishedGoods {
		t.Errorf("next checkpoint = %v, want %v", res1.NextCheckpoint, CheckpointFinishedGoods)
	}
	if res1.ScannedBy != scannedBy {
		t.Errorf("ScannedBy = %v, want %v", res1.ScannedBy, scannedBy)
	}
	if res1.DeviceID != "CNC-01" {
		t.Errorf("DeviceID = %q, want %q", res1.DeviceID, "CNC-01")
	}
	if res1.DeviceName != "Kiosk A" {
		t.Errorf("DeviceName = %q, want %q", res1.DeviceName, "Kiosk A")
	}
	if res1.Shift != "Morning" {
		t.Errorf("Shift = %q, want %q", res1.Shift, "Morning")
	}
	if st.insertScanEventResult.DeviceID != "CNC-01" || st.insertScanEventResult.DeviceName != "Kiosk A" || st.insertScanEventResult.Shift != "Morning" {
		t.Errorf("inserted metadata mismatch: %+v", st.insertScanEventResult)
	}

	st.selectLastScanEventResult = ScanEvent{BarcodeID: barcodeID, Checkpoint: CheckpointCNCComplete}
	st.selectLastScanEventErr = nil
	wo.status = domain.WOInProcessing
	in2 := RecordScanInput{BarcodeID: barcodeID, Checkpoint: CheckpointFinishedGoods, ScannedBy: scannedBy}
	res2, err := svc.RecordScan(context.Background(), in2)
	if err != nil {
		t.Fatalf("checkpoint %s: unexpected error: %v", CheckpointFinishedGoods, err)
	}
	if res2.NextCheckpoint == nil || *res2.NextCheckpoint != CheckpointShipped {
		t.Errorf("next checkpoint = %v, want %v", res2.NextCheckpoint, CheckpointShipped)
	}

	st.selectLastScanEventResult = ScanEvent{BarcodeID: barcodeID, Checkpoint: CheckpointFinishedGoods}
	wo.status = domain.WOCompleted
	in3 := RecordScanInput{BarcodeID: barcodeID, Checkpoint: CheckpointShipped, ScannedBy: scannedBy}
	res3, err := svc.RecordScan(context.Background(), in3)
	if err != nil {
		t.Fatalf("checkpoint %s: unexpected error: %v", CheckpointShipped, err)
	}
	if res3.NextCheckpoint != nil {
		t.Errorf("next checkpoint = %v, want nil", res3.NextCheckpoint)
	}
	if !st.insertScanEventCalled {
		t.Error("insertScanEvent must be called")
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
				ScannedBy:  uuid.New(),
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
		ScannedBy:  uuid.New(),
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
		selectBarcodeByIDResult:   storedBarcode(barcodeID),
		selectLastScanEventResult: ScanEvent{BarcodeID: barcodeID, Checkpoint: CheckpointFinishedGoods},
		insertScanEventErr:        dbErr,
	}
	wo := &mockWOGateway{status: domain.WOCompleted}
	svc := NewService(st, wo)
	_, err := svc.RecordScan(context.Background(), RecordScanInput{
		BarcodeID:  barcodeID,
		Checkpoint: CheckpointShipped,
		ScannedBy:  uuid.New(),
	})

	if !errors.Is(err, dbErr) {
		t.Errorf("expected insertScanEvent error to propagate, got %v", err)
	}
}

// RecordScan must check checkpoint validity BEFORE querying the barcode store,
// so an invalid checkpoint must never reach selectBarcodeByID.
func TestRecordScan_InvalidCheckpoint_DoesNotQueryStore(t *testing.T) {
	st := &mockStore{}

	svc := NewService(st)
	_, err := svc.RecordScan(context.Background(), RecordScanInput{
		BarcodeID:  uuid.New(),
		Checkpoint: "BOGUS",
		ScannedBy:  uuid.New(),
	})

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput before store call, got %v", err)
	}
}

func TestRecordScan_MetadataValidation(t *testing.T) {
	barcodeID := uuid.New()
	st := &mockStore{selectBarcodeByIDResult: storedBarcode(barcodeID)}
	svc := NewService(st)

	t.Run("device_id_too_long", func(t *testing.T) {
		_, err := svc.RecordScan(context.Background(), RecordScanInput{
			BarcodeID:  barcodeID,
			Checkpoint: CheckpointCNCComplete,
			ScannedBy:  uuid.New(),
			DeviceID:   string(bytes.Repeat([]byte{'a'}, 65)),
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
		if st.insertScanEventCalled {
			t.Fatal("insertScanEvent must not be called when validation fails")
		}
	})

	t.Run("device_name_too_long", func(t *testing.T) {
		st.insertScanEventCalled = false
		_, err := svc.RecordScan(context.Background(), RecordScanInput{
			BarcodeID:  barcodeID,
			Checkpoint: CheckpointCNCComplete,
			ScannedBy:  uuid.New(),
			DeviceName: string(bytes.Repeat([]byte{'b'}, 121)),
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
		if st.insertScanEventCalled {
			t.Fatal("insertScanEvent must not be called when validation fails")
		}
	})

	t.Run("shift_too_long", func(t *testing.T) {
		st.insertScanEventCalled = false
		_, err := svc.RecordScan(context.Background(), RecordScanInput{
			BarcodeID:  barcodeID,
			Checkpoint: CheckpointCNCComplete,
			ScannedBy:  uuid.New(),
			Shift:      string(bytes.Repeat([]byte{'c'}, 41)),
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
		if st.insertScanEventCalled {
			t.Fatal("insertScanEvent must not be called when validation fails")
		}
	})
}

type mockWOGateway struct {
	status        domain.WorkOrderStatus
	getErr        error
	advanceCalled bool
	advanceWOID   uuid.UUID
	advanceTo     domain.WorkOrderStatus
	advanceErr    error
}

func (m *mockWOGateway) GetWorkOrder(_ context.Context, woID uuid.UUID) (WorkOrderRef, error) {
	if m.getErr != nil {
		return WorkOrderRef{}, m.getErr
	}
	return WorkOrderRef{ID: woID, Status: m.status}, nil
}

func (m *mockWOGateway) AdvanceStatus(_ context.Context, woID uuid.UUID, to domain.WorkOrderStatus) error {
	m.advanceCalled = true
	m.advanceWOID = woID
	m.advanceTo = to
	return m.advanceErr
}

func TestRecordScan_OutOfOrder_ReturnsErrInvalidTransition(t *testing.T) {
	barcodeID := uuid.New()
	st := &mockStore{
		selectBarcodeByIDResult:   storedBarcode(barcodeID),
		selectLastScanEventResult: ScanEvent{BarcodeID: barcodeID, Checkpoint: CheckpointCNCComplete},
	}
	wo := &mockWOGateway{status: domain.WOCompleted}
	svc := NewService(st, wo)

	_, err := svc.RecordScan(context.Background(), RecordScanInput{
		BarcodeID:  barcodeID,
		Checkpoint: CheckpointShipped,
		ScannedBy:  uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	if st.insertScanEventCalled {
		t.Fatal("insertScanEvent must not be called for out-of-order scan")
	}
}

func TestRecordScan_CNCComplete_AutoAdvanceWorkOrder(t *testing.T) {
	barcodeID := uuid.New()
	bc := storedBarcode(barcodeID)
	st := &mockStore{
		selectBarcodeByIDResult: bc,
		selectLastScanEventErr:  domain.NewBizError(domain.ErrNotFound, "scan event not found"),
	}
	wo := &mockWOGateway{status: domain.WOInCutting}
	svc := NewService(st, wo)

	res, err := svc.RecordScan(context.Background(), RecordScanInput{
		BarcodeID:  barcodeID,
		Checkpoint: CheckpointCNCComplete,
		ScannedBy:  uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wo.advanceCalled {
		t.Fatal("expected work order to be advanced")
	}
	if wo.advanceWOID != bc.WorkOrderID {
		t.Fatalf("advance wo id = %v, want %v", wo.advanceWOID, bc.WorkOrderID)
	}
	if wo.advanceTo != domain.WOInProcessing {
		t.Fatalf("advance target = %s, want %s", wo.advanceTo, domain.WOInProcessing)
	}
	if res.WorkOrder.NewStatus != domain.WOInProcessing {
		t.Fatalf("new status = %s, want %s", res.WorkOrder.NewStatus, domain.WOInProcessing)
	}
}

func TestRecordScan_RejectsWhenWorkOrderStatusMismatched(t *testing.T) {
	barcodeID := uuid.New()
	st := &mockStore{
		selectBarcodeByIDResult:   storedBarcode(barcodeID),
		selectLastScanEventResult: ScanEvent{BarcodeID: barcodeID, Checkpoint: CheckpointCNCComplete},
	}
	wo := &mockWOGateway{status: domain.WOCompleted}
	svc := NewService(st, wo)

	_, err := svc.RecordScan(context.Background(), RecordScanInput{
		BarcodeID:  barcodeID,
		Checkpoint: CheckpointFinishedGoods,
		ScannedBy:  uuid.New(),
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
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
	if len(png) < 4 || png[0] != 0x89 || png[1] != 0x50 || png[2] != 0x4E || png[3] != 0x47 {
		t.Errorf("response is not a valid PNG (first bytes: %v)", png[:4])
	}

	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	symbols, err := goqr.Recognize(img)
	if err != nil {
		t.Fatalf("recognize qr from png: %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("no QR code recognized from generated PNG")
	}

	var payload qrPayload
	if err := json.Unmarshal(symbols[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal qr payload: %v", err)
	}
	if payload.ID != bc.ID.String() {
		t.Errorf("payload.id = %q, want %q", payload.ID, bc.ID.String())
	}
	if payload.SKUCode != bc.SKUCode {
		t.Errorf("payload.sku_code = %q, want %q", payload.SKUCode, bc.SKUCode)
	}
	if payload.Dimensions != bc.Dimensions {
		t.Errorf("payload.dimensions = %q, want %q", payload.Dimensions, bc.Dimensions)
	}
	if payload.POID != bc.POID.String() {
		t.Errorf("payload.po_id = %q, want %q", payload.POID, bc.POID.String())
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

	widthMM, heightMM := parsePageSizeMMFromPDF(t, pdf)
	assertCloseMM(t, widthMM, 50)
	assertCloseMM(t, heightMM, 30)
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

	widthMM, heightMM := parsePageSizeMMFromPDF(t, pdf)
	assertCloseMM(t, widthMM, 100)
	assertCloseMM(t, heightMM, 70)
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

func TestGenerateBatchLabelPDF_EmptyIDs_ReturnsErrInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.GenerateBatchLabelPDF(context.Background(), BatchPrintInput{})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestGenerateBatchLabelPDF_HappyPath_ReturnsMultiPagePDF(t *testing.T) {
	ids := make([]uuid.UUID, 10)
	barcodes := make([]Barcode, 0, 10)
	for i := 0; i < 10; i++ {
		id := uuid.New()
		ids[i] = id
		bc := storedBarcode(id)
		bc.POID = uuid.New()
		bc.WorkOrderID = uuid.New()
		bc.Dimensions = "1200x600"
		bc.SKUCode = "SKU-" + strconv.Itoa(i+1)
		barcodes = append(barcodes, bc)
	}
	st := &mockStore{selectBarcodesByIDsOrderedResult: barcodes}
	svc := NewService(st)

	pdf, err := svc.GenerateBatchLabelPDF(context.Background(), BatchPrintInput{BarcodeIDs: ids, Size: LabelSize50x30})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pdf) < 5 || string(pdf[:5]) != "%PDF-" {
		t.Fatalf("response is not a valid PDF header")
	}
	if count := bytes.Count(pdf, []byte("/Type /Page")); count < 10 {
		t.Fatalf("page count too low: got %d, want at least 10", count)
	}
	if len(st.selectBarcodesByIDsOrderedArg) != len(ids) {
		t.Fatalf("store received %d ids, want %d", len(st.selectBarcodesByIDsOrderedArg), len(ids))
	}
	for i := range ids {
		if st.selectBarcodesByIDsOrderedArg[i] != ids[i] {
			t.Fatalf("id order mismatch at %d: got %v, want %v", i, st.selectBarcodesByIDsOrderedArg[i], ids[i])
		}
	}
}
