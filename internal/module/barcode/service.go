package barcode

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

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
