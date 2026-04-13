package inventory

import (
	"context"

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

// AdvanceWOInput is the inventory module's view of a work order advance request.
// It intentionally mirrors only the fields needed by the auto-advance call.
type AdvanceWOInput struct {
	To domain.WorkOrderStatus
}
