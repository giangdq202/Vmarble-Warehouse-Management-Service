package barcode

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertBarcode(ctx context.Context, b Barcode) error
	selectBarcodeByID(ctx context.Context, id uuid.UUID) (Barcode, error)
	selectBarcodesByWorkOrder(ctx context.Context, workOrderID uuid.UUID) ([]Barcode, error)
	selectBarcodesByIDsOrdered(ctx context.Context, ids []uuid.UUID) ([]Barcode, error)
	insertScanEvent(ctx context.Context, e ScanEvent) error
	// selectScanEventsByBarcodeKeyset returns scan_events for barcodeID
	// strictly after the (scanned_at, id) cursor, ordered chronologically.
	// Callers pass limit+1 so the service layer can detect has_more.
	selectScanEventsByBarcodeKeyset(ctx context.Context, barcodeID uuid.UUID, cur httpkit.Cursor, limit int) ([]ScanEvent, error)
	selectLastScanEventByBarcode(ctx context.Context, barcodeID uuid.UUID) (ScanEvent, error)
}
