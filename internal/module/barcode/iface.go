package barcode

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type ScanCheckpoint string

type LabelSize string

const (
	CheckpointCNCComplete   ScanCheckpoint = "CNC_COMPLETE"
	CheckpointFinishedGoods ScanCheckpoint = "FINISHED_GOODS"
	CheckpointShipped       ScanCheckpoint = "SHIPPED"

	LabelSize50x30  LabelSize = "50x30"
	LabelSize100x70 LabelSize = "100x70"
)

type GenerateBarcodeInput struct {
	WorkOrderID      uuid.UUID `json:"work_order_id"`
	SKUID            uuid.UUID `json:"sku_id"`
	POID             uuid.UUID `json:"po_id"`
	ProductionPlanID uuid.UUID `json:"production_plan_id"`
	SKUCode          string    `json:"sku_code"`
	SKUName          string    `json:"sku_name"`
	Dimensions       string    `json:"dimensions"`
	ProducedDate     time.Time `json:"produced_date"`
}

type Barcode struct {
	ID               uuid.UUID `json:"id"`
	WorkOrderID      uuid.UUID `json:"work_order_id"`
	SKUID            uuid.UUID `json:"sku_id"`
	POID             uuid.UUID `json:"po_id"`
	ProductionPlanID uuid.UUID `json:"production_plan_id"`
	SKUCode          string    `json:"sku_code"`
	SKUName          string    `json:"sku_name"`
	Dimensions       string    `json:"dimensions"`
	ProducedDate     time.Time `json:"produced_date"`
	CreatedAt        time.Time `json:"created_at"`
}

type RecordScanInput struct {
	BarcodeID  uuid.UUID      `json:"-"`
	Checkpoint ScanCheckpoint `json:"checkpoint"`
	Location   string         `json:"location,omitempty"`
	Note       string         `json:"note,omitempty"`
	DeviceID   string         `json:"device_id,omitempty"`
	DeviceName string         `json:"device_name,omitempty"`
	Shift      string         `json:"shift,omitempty"`
	ScannedBy  uuid.UUID      `json:"-"`
}

type ScanEvent struct {
	ID         uuid.UUID      `json:"id"`
	BarcodeID  uuid.UUID      `json:"barcode_id"`
	Checkpoint ScanCheckpoint `json:"checkpoint"`
	ScannedBy  uuid.UUID      `json:"scanned_by"`
	Location   string         `json:"location,omitempty"`
	Note       string         `json:"note,omitempty"`
	DeviceID   string         `json:"device_id,omitempty"`
	DeviceName string         `json:"device_name,omitempty"`
	Shift      string         `json:"shift,omitempty"`
	ScannedAt  time.Time      `json:"scanned_at"`
}

// ScanResult is the enriched response returned by RecordScan.
type ScanResult struct {
	ScanID         uuid.UUID       `json:"scan_id"`
	BarcodeID      uuid.UUID       `json:"barcode_id"`
	Checkpoint     ScanCheckpoint  `json:"checkpoint"`
	ScannedBy      uuid.UUID       `json:"scanned_by"`
	ScannedByName  string          `json:"scanned_by_name"`
	DeviceID       string          `json:"device_id,omitempty"`
	DeviceName     string          `json:"device_name,omitempty"`
	Shift          string          `json:"shift,omitempty"`
	ScannedAt      time.Time       `json:"scanned_at"`
	NextCheckpoint *ScanCheckpoint `json:"next_checkpoint,omitempty"`
	WorkOrder      WorkOrderScan   `json:"work_order"`
}

type WorkOrderScan struct {
	ID        uuid.UUID              `json:"id"`
	NewStatus domain.WorkOrderStatus `json:"new_status"`
	SKUCode   string                 `json:"sku_code"`
	SKUName   string                 `json:"sku_name"`
}

type BatchPrintInput struct {
	BarcodeIDs []uuid.UUID `json:"barcode_ids"`
	Size       LabelSize   `json:"size,omitempty"`
}

type Service interface {
	GenerateBarcode(ctx context.Context, in GenerateBarcodeInput) (Barcode, error)
	LookupBarcode(ctx context.Context, barcodeID uuid.UUID) (Barcode, error)
	ListBarcodesByWorkOrder(ctx context.Context, workOrderID uuid.UUID) ([]Barcode, error)
	RecordScan(ctx context.Context, in RecordScanInput) (ScanResult, error)
	ListScans(ctx context.Context, barcodeID uuid.UUID) ([]ScanEvent, error)
	GenerateQRCode(ctx context.Context, barcodeID uuid.UUID) ([]byte, error)
	GenerateLabelPDF(ctx context.Context, barcodeID uuid.UUID, size LabelSize) ([]byte, error)
	GenerateBatchLabelPDF(ctx context.Context, in BatchPrintInput) ([]byte, error)
}
