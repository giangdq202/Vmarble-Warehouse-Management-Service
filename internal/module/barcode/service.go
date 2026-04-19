package barcode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jung-kurt/gofpdf"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

const (
	qrSize      = 256  // pixels for /qr endpoint
	labelQRSize = 1024 // pixels for print-ready labels
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
	st store
}

func NewService(st store) Service {
	return &service{st: st}
}

func validCheckpoint(c ScanCheckpoint) bool {
	switch c {
	case CheckpointCNCComplete, CheckpointFinishedGoods, CheckpointShipped:
		return true
	default:
		return false
	}
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

func (s *service) RecordScan(ctx context.Context, in RecordScanInput) (ScanEvent, error) {
	if !validCheckpoint(in.Checkpoint) {
		return ScanEvent{}, domain.NewBizError(domain.ErrInvalidInput, "invalid checkpoint")
	}

	if _, err := s.st.selectBarcodeByID(ctx, in.BarcodeID); err != nil {
		return ScanEvent{}, err
	}

	e := ScanEvent{
		ID:         uuid.New(),
		BarcodeID:  in.BarcodeID,
		Checkpoint: in.Checkpoint,
		ScannedBy:  in.ScannedBy,
		ScannedAt:  time.Now().UTC(),
	}
	if err := s.st.insertScanEvent(ctx, e); err != nil {
		return ScanEvent{}, err
	}
	return e, nil
}

func (s *service) ListScans(ctx context.Context, barcodeID uuid.UUID) ([]ScanEvent, error) {
	return s.st.selectScanEventsByBarcode(ctx, barcodeID)
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
	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: layout.pageWidthMM, Ht: layout.pageHeightMM})

	imgOpts := gofpdf.ImageOptions{ImageType: "PNG"}
	const imageName = "qr"
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

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, fmt.Errorf("render label pdf: %w", err)
	}
	return out.Bytes(), nil
}
