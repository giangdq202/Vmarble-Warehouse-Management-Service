package barcode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

const (
	qrSize      = 256  // pixels for /qr endpoint
	labelQRSize = 1024 // pixels for print-ready labels

	maxScanDeviceIDLen   = 64
	maxScanDeviceNameLen = 120
	maxScanShiftLen      = 40
)

// qrPayload is the JSON content encoded into the QR image.
type qrPayload struct {
	ID         string `json:"id"`
	SKUCode    string `json:"sku_code"`
	Dimensions string `json:"dimensions"`
	POID       string `json:"po_id"`
}

type labelLayout struct {
	pageWidthMM  float64
	pageHeightMM float64
	qrXMM        float64
	qrYMM        float64
	qrSizeMM     float64
	textXMM      float64
	textYMM      float64
	textWidthMM  float64
	lineHeightMM float64
	titleFontPt  float64
	bodyFontPt   float64
	skuMaxRunes  int
}

type service struct {
	st       store
	wo       WorkOrderGateway
	usr      UserLookup
	notifier ScanNotifier
}

func NewService(st store, deps ...any) Service {
	svc := &service{st: st}
	for _, dep := range deps {
		switch d := dep.(type) {
		case WorkOrderGateway:
			svc.wo = d
		case UserLookup:
			svc.usr = d
		case ScanNotifier:
			svc.notifier = d
		}
	}
	return svc
}

func validCheckpoint(c ScanCheckpoint) bool {
	switch c {
	case CheckpointCNCComplete, CheckpointFinishedGoods, CheckpointShipped:
		return true
	default:
		return false
	}
}

func checkpointOrder(c ScanCheckpoint) int {
	switch c {
	case CheckpointCNCComplete:
		return 1
	case CheckpointFinishedGoods:
		return 2
	case CheckpointShipped:
		return 3
	default:
		return 0
	}
}

func nextCheckpoint(c ScanCheckpoint) *ScanCheckpoint {
	switch c {
	case CheckpointCNCComplete:
		next := CheckpointFinishedGoods
		return &next
	case CheckpointFinishedGoods:
		next := CheckpointShipped
		return &next
	default:
		return nil
	}
}

func normalizeScanMeta(v string) string {
	return strings.TrimSpace(v)
}

func validateScanMeta(in RecordScanInput) error {
	if utf8.RuneCountInString(in.DeviceID) > maxScanDeviceIDLen {
		return domain.NewBizError(domain.ErrInvalidInput, "device_id exceeds max length 64")
	}
	if utf8.RuneCountInString(in.DeviceName) > maxScanDeviceNameLen {
		return domain.NewBizError(domain.ErrInvalidInput, "device_name exceeds max length 120")
	}
	if utf8.RuneCountInString(in.Shift) > maxScanShiftLen {
		return domain.NewBizError(domain.ErrInvalidInput, "shift exceeds max length 40")
	}
	return nil
}

func resolveLabelLayout(size LabelSize) (labelLayout, error) {
	switch size {
	case LabelSize50x30:
		return labelLayout{
			pageWidthMM:  50,
			pageHeightMM: 30,
			qrXMM:        2,
			qrYMM:        4,
			qrSizeMM:     20,
			textXMM:      24,
			textYMM:      5,
			textWidthMM:  24,
			lineHeightMM: 4,
			titleFontPt:  7,
			bodyFontPt:   6,
			skuMaxRunes:  12,
		}, nil
	case LabelSize100x70:
		return labelLayout{
			pageWidthMM:  100,
			pageHeightMM: 70,
			qrXMM:        5,
			qrYMM:        8,
			qrSizeMM:     34,
			textXMM:      42,
			textYMM:      10,
			textWidthMM:  54,
			lineHeightMM: 8,
			titleFontPt:  12,
			bodyFontPt:   10,
			skuMaxRunes:  24,
		}, nil
	default:
		return labelLayout{}, domain.NewBizError(domain.ErrInvalidInput, "size must be one of: 50x30, 100x70")
	}
}

func qrPayloadBytes(bc Barcode) ([]byte, error) {
	payload := qrPayload{
		ID:         bc.ID.String(),
		SKUCode:    bc.SKUCode,
		Dimensions: bc.Dimensions,
		POID:       bc.POID.String(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal qr payload: %w", err)
	}
	return raw, nil
}

func encodeQRCode(raw []byte, size int) ([]byte, error) {
	png, err := qrcode.Encode(string(raw), qrcode.High, size)
	if err != nil {
		return nil, fmt.Errorf("encode qr code: %w", err)
	}
	return png, nil
}

func clampRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

func shortUUID(id uuid.UUID) string {
	s := id.String()
	return s[:8]
}

func (s *service) GenerateBarcode(ctx context.Context, in GenerateBarcodeInput) (Barcode, error) {
	if in.SKUCode == "" {
		return Barcode{}, domain.NewBizError(domain.ErrInvalidInput, "SKU code is required")
	}
	if in.WorkOrderID == uuid.Nil {
		return Barcode{}, domain.NewBizError(domain.ErrInvalidInput, "work order ID is required")
	}

	b := Barcode{
		ID:               uuid.New(),
		WorkOrderID:      in.WorkOrderID,
		SKUID:            in.SKUID,
		POID:             in.POID,
		ProductionPlanID: in.ProductionPlanID,
		SKUCode:          in.SKUCode,
		SKUName:          in.SKUName,
		Dimensions:       in.Dimensions,
		ProducedDate:     in.ProducedDate,
		CreatedAt:        time.Now().UTC(),
	}
	if err := s.st.insertBarcode(ctx, b); err != nil {
		return Barcode{}, err
	}
	return b, nil
}

func (s *service) LookupBarcode(ctx context.Context, barcodeID uuid.UUID) (Barcode, error) {
	return s.st.selectBarcodeByID(ctx, barcodeID)
}

func (s *service) ListBarcodesByWorkOrder(ctx context.Context, workOrderID uuid.UUID) ([]Barcode, error) {
	return s.st.selectBarcodesByWorkOrder(ctx, workOrderID)
}

func (s *service) RecordScan(ctx context.Context, in RecordScanInput) (ScanResult, error) {
	if in.ScannedBy == uuid.Nil {
		return ScanResult{}, domain.NewBizError(domain.ErrInvalidInput, "scanned_by is required")
	}
	if !validCheckpoint(in.Checkpoint) {
		return ScanResult{}, domain.NewBizError(domain.ErrInvalidInput, "invalid checkpoint")
	}

	in.DeviceID = normalizeScanMeta(in.DeviceID)
	in.DeviceName = normalizeScanMeta(in.DeviceName)
	in.Shift = normalizeScanMeta(in.Shift)
	if err := validateScanMeta(in); err != nil {
		return ScanResult{}, err
	}

	bc, err := s.st.selectBarcodeByID(ctx, in.BarcodeID)
	if err != nil {
		return ScanResult{}, err
	}

	woStatus, hasWOStatus, err := s.resolveWorkOrderStatus(ctx, bc.WorkOrderID)
	if err != nil {
		return ScanResult{}, err
	}
	if hasWOStatus {
		expected, _ := expectedWOStatusForCheckpoint(in.Checkpoint)
		if woStatus != expected {
			return ScanResult{}, domain.NewBizError(domain.ErrPreconditionFailed,
				"work order status does not allow this checkpoint")
		}
	}

	var last *ScanCheckpoint
	lastEvent, err := s.st.selectLastScanEventByBarcode(ctx, in.BarcodeID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return ScanResult{}, err
		}
	} else {
		last = &lastEvent.Checkpoint
	}
	if !isCheckpointAllowed(last, in.Checkpoint) {
		return ScanResult{}, domain.NewBizError(domain.ErrInvalidTransition, "checkpoint scanned out of order")
	}

	e := ScanEvent{
		ID:         uuid.New(),
		BarcodeID:  in.BarcodeID,
		Checkpoint: in.Checkpoint,
		ScannedBy:  in.ScannedBy,
		Location:   in.Location,
		Note:       in.Note,
		DeviceID:   in.DeviceID,
		DeviceName: in.DeviceName,
		Shift:      in.Shift,
		ScannedAt:  time.Now().UTC(),
	}
	if err := s.st.insertScanEvent(ctx, e); err != nil {
		return ScanResult{}, err
	}

	newStatus := woStatus
	if in.Checkpoint == CheckpointCNCComplete {
		if s.wo == nil {
			return ScanResult{}, domain.NewBizError(domain.ErrPreconditionFailed, "work order transition dependency is not configured")
		}
		if err := s.wo.AdvanceStatus(ctx, bc.WorkOrderID, domain.WOInProcessing); err != nil {
			return ScanResult{}, err
		}
		newStatus = domain.WOInProcessing
	}

	scannedByName := in.ScannedBy.String()
	if s.usr != nil {
		u, err := s.usr.GetUser(ctx, in.ScannedBy)
		if err == nil && u.Username != "" {
			scannedByName = u.Username
		}
	}

	// Best-effort SSE notification — log + continue if the broker is down so a
	// transient failure never rolls back the persisted scan event.
	if s.notifier != nil {
		if err := s.notifier.NotifyScanCheckpoint(ctx, bc.WorkOrderID.String(), string(in.Checkpoint)); err != nil {
			slog.Warn("barcode: notify scan checkpoint failed",
				"work_order_id", bc.WorkOrderID, "checkpoint", in.Checkpoint, "err", err)
		}
	}

	return ScanResult{
		ScanID:         e.ID,
		BarcodeID:      e.BarcodeID,
		Checkpoint:     e.Checkpoint,
		ScannedBy:      in.ScannedBy,
		ScannedByName:  scannedByName,
		DeviceID:       e.DeviceID,
		DeviceName:     e.DeviceName,
		Shift:          e.Shift,
		ScannedAt:      e.ScannedAt,
		NextCheckpoint: nextCheckpoint(e.Checkpoint),
		WorkOrder: WorkOrderScan{
			ID:        bc.WorkOrderID,
			NewStatus: newStatus,
			SKUCode:   bc.SKUCode,
			SKUName:   bc.SKUName,
		},
	}, nil
}

func (s *service) resolveWorkOrderStatus(ctx context.Context, workOrderID uuid.UUID) (domain.WorkOrderStatus, bool, error) {
	if s.wo == nil {
		return "", false, nil
	}
	w, err := s.wo.GetWorkOrder(ctx, workOrderID)
	if err != nil {
		return "", false, err
	}
	return w.Status, true, nil
}

func (s *service) ListScans(ctx context.Context, barcodeID uuid.UUID, params httpkit.CursorParams) (httpkit.CursorResult[ScanEvent], error) {
	cur, err := params.Decoded()
	if err != nil {
		return httpkit.CursorResult[ScanEvent]{}, err
	}
	// Over-fetch by 1 row so NewCursorResult can detect has_more without an
	// extra round-trip.
	rows, err := s.st.selectScanEventsByBarcodeKeyset(ctx, barcodeID, cur, params.Limit+1)
	if err != nil {
		return httpkit.CursorResult[ScanEvent]{}, err
	}
	return httpkit.NewCursorResult(rows, params.Limit, func(e ScanEvent) httpkit.Cursor {
		return httpkit.Cursor{Ts: e.ScannedAt, ID: e.ID}
	}), nil
}

// GenerateQRCode returns a PNG image containing the barcode's key metadata
// encoded as JSON. Scanning the QR with a mobile app yields:
//
//	{"id":"<uuid>","sku_code":"...","dimensions":"...","po_id":"<uuid>"}
func (s *service) GenerateQRCode(ctx context.Context, barcodeID uuid.UUID) ([]byte, error) {
	bc, err := s.st.selectBarcodeByID(ctx, barcodeID)
	if err != nil {
		return nil, err
	}

	raw, err := qrPayloadBytes(bc)
	if err != nil {
		return nil, err
	}
	return encodeQRCode(raw, qrSize)
}

func renderLabelPage(pdf *gofpdf.Fpdf, layout labelLayout, bc Barcode, imageName string, qrPNG []byte) {
	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: layout.pageWidthMM, Ht: layout.pageHeightMM})
	imgOpts := gofpdf.ImageOptions{ImageType: "PNG"}
	pdf.RegisterImageOptionsReader(imageName, imgOpts, bytes.NewReader(qrPNG))
	pdf.ImageOptions(imageName, layout.qrXMM, layout.qrYMM, layout.qrSizeMM, layout.qrSizeMM, false, imgOpts, 0, "")

	pdf.SetFont("Arial", "B", layout.titleFontPt)
	pdf.SetXY(layout.textXMM, layout.textYMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, clampRunes(bc.SKUCode, layout.skuMaxRunes), "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", layout.bodyFontPt)
	pdf.SetX(layout.textXMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, "WO: "+shortUUID(bc.WorkOrderID), "", 1, "L", false, 0, "")
	pdf.SetX(layout.textXMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, "PO: "+shortUUID(bc.POID), "", 1, "L", false, 0, "")
	pdf.SetX(layout.textXMM)
	pdf.CellFormat(layout.textWidthMM, layout.lineHeightMM, clampRunes("Dim: "+bc.Dimensions, layout.skuMaxRunes+8), "", 1, "L", false, 0, "")
}

func (s *service) GenerateLabelPDF(ctx context.Context, barcodeID uuid.UUID, size LabelSize) ([]byte, error) {
	layout, err := resolveLabelLayout(size)
	if err != nil {
		return nil, err
	}

	bc, err := s.st.selectBarcodeByID(ctx, barcodeID)
	if err != nil {
		return nil, err
	}

	raw, err := qrPayloadBytes(bc)
	if err != nil {
		return nil, err
	}
	qrPNG, err := encodeQRCode(raw, labelQRSize)
	if err != nil {
		return nil, err
	}

	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: layout.pageWidthMM, Ht: layout.pageHeightMM},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	renderLabelPage(pdf, layout, bc, "qr", qrPNG)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, fmt.Errorf("render label pdf: %w", err)
	}
	return out.Bytes(), nil
}

func (s *service) GenerateBatchLabelPDF(ctx context.Context, in BatchPrintInput) ([]byte, error) {
	if len(in.BarcodeIDs) == 0 {
		return nil, domain.NewBizError(domain.ErrInvalidInput, "barcode_ids is required")
	}
	size := in.Size
	if size == "" {
		size = LabelSize50x30
	}
	layout, err := resolveLabelLayout(size)
	if err != nil {
		return nil, err
	}

	barcodes, err := s.st.selectBarcodesByIDsOrdered(ctx, in.BarcodeIDs)
	if err != nil {
		return nil, err
	}

	pdf := gofpdf.NewCustom(&gofpdf.InitType{
		UnitStr: "mm",
		Size:    gofpdf.SizeType{Wd: layout.pageWidthMM, Ht: layout.pageHeightMM},
	})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)

	for i, bc := range barcodes {
		raw, err := qrPayloadBytes(bc)
		if err != nil {
			return nil, err
		}
		qrPNG, err := encodeQRCode(raw, labelQRSize)
		if err != nil {
			return nil, err
		}
		renderLabelPage(pdf, layout, bc, fmt.Sprintf("qr-%d", i), qrPNG)
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, fmt.Errorf("render batch label pdf: %w", err)
	}
	return out.Bytes(), nil
}
