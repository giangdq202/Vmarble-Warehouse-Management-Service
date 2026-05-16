package inventory

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

// WorkOrderAdvancer advances a work order to the next status.
// Implementation lives in the production module; wired in main.go.
// After a successful cut record, the inventory service calls this to
// automatically move the work order from IN_CUTTING to IN_PROCESSING.
// If the work order is already past IN_CUTTING (e.g. a second cut was
// submitted), the transition error is silently logged and ignored.
type WorkOrderAdvancer interface {
	AdvanceStatus(ctx context.Context, woID uuid.UUID, in AdvanceWOInput) error
}

// BarcodeGenerator creates barcode records for cut outputs.
// Implementation lives in barcode module; wired in main.go.
type BarcodeGenerator interface {
	GenerateForCut(ctx context.Context, in BarcodeForCutInput) (BarcodeForCutOutput, error)
}

type BarcodeForCutInput struct {
	WorkOrderID      uuid.UUID
	UsedDimension    domain.Dimension
	RemnantDimension *domain.Dimension
	ProducedDate     time.Time
}

type BarcodeForCutOutput struct {
	WIPBarcodeID     *uuid.UUID
	RemnantBarcodeID *uuid.UUID
}

// AdvanceWOInput is the inventory module's view of a work order advance request.
// It intentionally mirrors only the fields needed by the auto-advance call.
type AdvanceWOInput struct {
	To domain.WorkOrderStatus
}

// CutNotifier fires a CUTTING_RECORDED SSE event after RecordCut commits so
// the planner queue and accountant cost panel refresh without polling.
// Implementation lives in internal/platform/events; wired in main.go. Calls
// are best-effort: a non-nil error is logged and the request still succeeds.
type CutNotifier interface {
	NotifyCuttingRecorded(ctx context.Context, woID, cuttingRecordID string) error
}
