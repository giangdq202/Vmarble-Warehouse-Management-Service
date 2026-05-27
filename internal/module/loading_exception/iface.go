// Package loading_exception owns loading_exceptions (#303). One row per
// variance / damage / customer-change event raised against a container
// during loading. Pending = approved_by IS NULL; the SEAL flow refuses to
// close a container with any pending row (BR-D14).
//
// Auto-creation hooks (called from delivery / packing modules):
//   - SHORT_SHIPPED at seal time when actual loaded < planned (BR-D15)
//   - OVER_LOADED   on YELLOW scan outcome (BR-D16, ties to #291)
//
// Resolution drives downstream side effects:
//   - BACKORDER            -> creates a carry-over sales_order_lines row (BR-D17)
//   - SUBSTITUTE_ACCEPTED  -> requires substitute_sku_id + photo evidence (BR-D18)
package loading_exception

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

const (
	TypeShortShipped     = "SHORT_SHIPPED"
	TypeOverLoaded       = "OVER_LOADED"
	TypeWrongSKU         = "WRONG_SKU"
	TypeSubstitution     = "SUBSTITUTION"
	TypeDamagedAtLoading = "DAMAGED_AT_LOADING"
	TypeUnplannedUnit    = "UNPLANNED_UNIT"
	TypeCustomerChange   = "CUSTOMER_CHANGE"

	ResolutionBackorder          = "BACKORDER"
	ResolutionCancelFromSO       = "CANCEL_FROM_SO"
	ResolutionSubstituteAccepted = "SUBSTITUTE_ACCEPTED"
	ResolutionWriteOff           = "WRITE_OFF"
	ResolutionDeferToNext        = "DEFER_TO_NEXT"
)

// LoadingException is the public projection of one loading_exceptions row.
// approved_by + approved_at are the source of truth for the pending vs
// closed split — resolution can be NULL on a "rejected" row (admin closed
// it without picking a downstream action).
type LoadingException struct {
	ID                uuid.UUID  `json:"id"`
	ContainerID       uuid.UUID  `json:"container_id"`
	LoadingPlanID     *uuid.UUID `json:"loading_plan_id,omitempty"`
	ExceptionType     string     `json:"exception_type"`
	SKUID             *uuid.UUID `json:"sku_id,omitempty"`
	Qty               *int       `json:"qty,omitempty"`
	Reason            string     `json:"reason"`
	PhotoURLs         []string   `json:"photo_urls"`
	ApprovedBy        *uuid.UUID `json:"approved_by,omitempty"`
	ApprovedAt        *time.Time `json:"approved_at,omitempty"`
	Resolution        *string    `json:"resolution,omitempty"`
	ResolutionNotes   string     `json:"resolution_notes,omitempty"`
	CarryOverSOLineID *uuid.UUID `json:"carry_over_so_line_id,omitempty"`
	SubstituteSKUID   *uuid.UUID `json:"substitute_sku_id,omitempty"`
	CreatedBy         uuid.UUID  `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
}

// CreateInput drives manual creation via POST /containers/:id/exceptions.
// SOLineID is captured so BACKORDER resolution knows which parent line to
// carry-over from. The handler resolves the loading_plan_id from the
// container's currently active plan (best-effort — nil if no active plan).
type CreateInput struct {
	ContainerID   uuid.UUID
	LoadingPlanID *uuid.UUID
	ExceptionType string
	SKUID         *uuid.UUID
	SOLineID      *uuid.UUID
	Qty           *int
	Reason        string
	PhotoURLs     []string
	CreatedBy     uuid.UUID
}

// AutoCreateInput is the cross-module hook payload. ContainerID and
// ExceptionType are pre-baked by the caller (delivery fills SHORT_SHIPPED at
// seal, packing fills OVER_LOADED at YELLOW scan). CreatedBy is the actor
// that triggered the automation — packer or seal-caller.
type AutoCreateInput struct {
	ContainerID   uuid.UUID
	LoadingPlanID *uuid.UUID
	ExceptionType string
	SKUID         *uuid.UUID
	Qty           *int
	Reason        string
	PhotoURLs     []string
	CreatedBy     uuid.UUID
}

// ApproveInput captures the admin/sales decision. SubstituteSKUID is required
// when Resolution = SUBSTITUTE_ACCEPTED (BR-D18). ParentSOLineID is required
// when Resolution = BACKORDER (BR-D17) so the service knows which sales
// order line to carry-over from.
type ApproveInput struct {
	ID              uuid.UUID
	Resolution      string
	ResolutionNotes string
	SubstituteSKUID *uuid.UUID
	ParentSOLineID  *uuid.UUID
	ApprovedBy      uuid.UUID
}

// RejectInput closes a pending exception without picking a resolution. The
// reason is stored into resolution_notes so the audit trail has the admin's
// rationale even though resolution column stays NULL.
type RejectInput struct {
	ID         uuid.UUID
	Reason     string
	ApprovedBy uuid.UUID
}

// ListFilter narrows GET /containers/:id/exceptions to pending / approved
// / all. Default returns every exception for the container.
type ListFilter struct {
	Status string // "pending" | "approved" | "all"
}

// PendingSummary is what delivery.Seal needs: just the count + the ids
// that are still open so the 412 error body can list them. Cheap query
// against idx_le_pending.
type PendingSummary struct {
	Count int         `json:"count"`
	IDs   []uuid.UUID `json:"ids"`
}

type Service interface {
	Create(ctx context.Context, in CreateInput) (LoadingException, error)

	// AutoCreate is the hook used by delivery (SHORT_SHIPPED) and packing
	// (OVER_LOADED). Same shape as Create but ExceptionType is pre-set.
	AutoCreate(ctx context.Context, in AutoCreateInput) (LoadingException, error)

	// Approve flips approved_by + sets resolution. Rejects re-approval of
	// already-approved rows with ErrInvalidTransition. Side effects per
	// resolution (carry-over line creation) run inside the same tx via the
	// CarryOverCreator dep.
	Approve(ctx context.Context, in ApproveInput) (LoadingException, error)

	// Reject flips approved_by but leaves resolution = NULL.
	Reject(ctx context.Context, in RejectInput) (LoadingException, error)

	Get(ctx context.Context, id uuid.UUID) (LoadingException, error)

	List(ctx context.Context, containerID uuid.UUID, f ListFilter, p httpkit.CursorParams) (httpkit.CursorResult[LoadingException], error)

	// PendingForContainer is the SEAL pre-check (BR-D14). Count == 0 means
	// "ok to seal".
	PendingForContainer(ctx context.Context, containerID uuid.UUID) (PendingSummary, error)
}
