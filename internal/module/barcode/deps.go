package barcode

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

// WorkOrderGateway provides work-order state read + transition for scan workflow.
type WorkOrderGateway interface {
	GetWorkOrder(ctx context.Context, woID uuid.UUID) (WorkOrderRef, error)
	AdvanceStatus(ctx context.Context, woID uuid.UUID, to domain.WorkOrderStatus) error
	// UpdateQCStatus writes the denormalized qc_status column on work_orders so
	// dashboards can filter by QC result without joining qc_events.
	UpdateQCStatus(ctx context.Context, woID uuid.UUID, status string) error
}

// WorkOrderRef is the minimal work-order shape needed by barcode workflow.
type WorkOrderRef struct {
	ID      uuid.UUID
	Status  domain.WorkOrderStatus
	SKUCode string
	SKUName string
}

// UserLookup resolves user display information for scan response payload.
type UserLookup interface {
	GetUser(ctx context.Context, userID uuid.UUID) (UserRef, error)
}

type UserRef struct {
	ID       uuid.UUID
	Username string
}

// ScanNotifier fires a SCAN_CHECKPOINT SSE event after RecordScan persists so
// manager dashboards and the accountant panel react to checkpoint transitions
// without polling. Implementation lives in internal/platform/events; wired in
// main.go. The call is best-effort — a non-nil error is logged and the request
// still succeeds.
type ScanNotifier interface {
	NotifyScanCheckpoint(ctx context.Context, woID, checkpoint string) error
}

// PackingSuggester is called when a QC_FAILED scan is recorded to surface
// packing reassignment suggestions for the resulting shortfall. Optional dep
// wired in main.go after both services are constructed.
type PackingSuggester interface {
	SuggestReassignFG(ctx context.Context, workOrderID uuid.UUID) error
}
