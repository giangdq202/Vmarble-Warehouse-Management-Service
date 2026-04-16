package barcode

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

const qrSize = 256 // pixels

// qrPayload is the JSON content encoded into the QR image.
type qrPayload struct {
	ID         string `json:"id"`
	SKUCode    string `json:"sku_code"`
	Dimensions string `json:"dimensions"`
	POID       string `json:"po_id"`
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

	png, err := qrcode.Encode(string(raw), qrcode.Medium, qrSize)
	if err != nil {
		return nil, fmt.Errorf("encode qr code: %w", err)
	}
	return png, nil
}
