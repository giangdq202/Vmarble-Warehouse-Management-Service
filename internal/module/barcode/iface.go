package barcode

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ScanCheckpoint string

const (
	CheckpointCNCComplete    ScanCheckpoint = "CNC_COMPLETE"
	CheckpointFinishedGoods ScanCheckpoint = "FINISHED_GOODS"
	CheckpointShipped       ScanCheckpoint = "SHIPPED"
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
	BarcodeID  uuid.UUID     `json:"barcode_id"`
	Checkpoint ScanCheckpoint `json:"checkpoint"`
	ScannedBy  string        `json:"scanned_by"`
}

type ScanEvent struct {
	ID         uuid.UUID     `json:"id"`
	BarcodeID  uuid.UUID     `json:"barcode_id"`
	Checkpoint ScanCheckpoint `json:"checkpoint"`
	ScannedBy  string        `json:"scanned_by"`
	ScannedAt  time.Time     `json:"scanned_at"`
}

type Service interface {
	GenerateBarcode(ctx context.Context, in GenerateBarcodeInput) (Barcode, error)
	LookupBarcode(ctx context.Context, barcodeID uuid.UUID) (Barcode, error)
	ListBarcodesByWorkOrder(ctx context.Context, workOrderID uuid.UUID) ([]Barcode, error)
	RecordScan(ctx context.Context, in RecordScanInput) (ScanEvent, error)
	ListScans(ctx context.Context, barcodeID uuid.UUID) ([]ScanEvent, error)
}
