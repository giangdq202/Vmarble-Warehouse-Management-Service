package barcode

import (
	"context"

	"github.com/google/uuid"
)

type store interface {
	insertBarcode(ctx context.Context, b Barcode) error
	selectBarcodeByID(ctx context.Context, id uuid.UUID) (Barcode, error)
	selectBarcodesByWorkOrder(ctx context.Context, workOrderID uuid.UUID) ([]Barcode, error)
	insertScanEvent(ctx context.Context, e ScanEvent) error
	selectScanEventsByBarcode(ctx context.Context, barcodeID uuid.UUID) ([]ScanEvent, error)
}
