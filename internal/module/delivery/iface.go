// Package delivery owns containers — the unit by which finished goods leave
// the warehouse. A container scoops finished-goods quantities from one or more
// sales_order_lines, gets sealed once loading is complete, and is shipped.
// Sealing is the moment qty_shipped on the underlying SO lines actually moves.
package delivery

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// Container status values; mirror chk_container_status in 00051.
const (
	ContainerStatusOpen      = "OPEN"
	ContainerStatusLoading   = "LOADING"
	ContainerStatusSealed    = "SEALED"
	ContainerStatusShipped   = "SHIPPED"
	ContainerStatusCancelled = "CANCELLED"
)

// Container types with default capacity envelopes; client may override
// max_cbm / max_payload_kg in CreateContainerInput when a non-standard
// container is used. The defaults map to ISO 20GP / 40GP / 40HC.
const (
	ContainerType20GP = "20GP"
	ContainerType40GP = "40GP"
	ContainerType40HC = "40HC"
)

type Container struct {
	ID            uuid.UUID  `json:"id"`
	Code          string     `json:"code"`
	ContainerType string     `json:"container_type"`
	MaxCBM        float64    `json:"max_cbm"`
	MaxPayloadKG  float64    `json:"max_payload_kg"`
	Status        string     `json:"status"`
	SealedAt      *time.Time `json:"sealed_at,omitempty"`
	SealedBy      *uuid.UUID `json:"sealed_by,omitempty"`
	Note          string     `json:"note,omitempty"`
	CreatedBy     uuid.UUID  `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`

	// Computed projections — populated by GetContainer; List does not hydrate
	// these to keep the page query a single round-trip.
	Lines       []ContainerLine `json:"lines,omitempty"`
	UsedCBM     float64         `json:"used_cbm,omitempty"`
	UsedWeight  float64         `json:"used_weight_kg,omitempty"`
	FillPctCBM  float64         `json:"fill_pct_cbm,omitempty"`
	FillPctMass float64         `json:"fill_pct_mass,omitempty"`
}

type ContainerLine struct {
	ID               uuid.UUID `json:"id"`
	ContainerID      uuid.UUID `json:"container_id"`
	SKUID            uuid.UUID `json:"sku_id"`
	SKUCode          string    `json:"sku_code,omitempty"`
	SKUName          string    `json:"sku_name,omitempty"`
	Qty              int       `json:"qty"`
	SalesOrderLineID uuid.UUID `json:"sales_order_line_id"`
	CBMTotal         float64   `json:"cbm_total"`
	WeightKGTotal    float64   `json:"weight_kg_total"`
	AddedBy          uuid.UUID `json:"added_by"`
	AddedAt          time.Time `json:"added_at"`
}

type ContainerStatusLogEntry struct {
	ID          uuid.UUID `json:"id"`
	ContainerID uuid.UUID `json:"container_id"`
	FromStatus  string    `json:"from_status,omitempty"`
	ToStatus    string    `json:"to_status"`
	ActorID     uuid.UUID `json:"actor_id"`
	Note        string    `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateContainerInput accepts max_cbm / max_payload_kg explicitly so the
// caller can override the defaults for a non-standard 20GP / 40GP / 40HC
// container. When zero, the service substitutes the default for the type.
type CreateContainerInput struct {
	ContainerType string    `json:"container_type"`
	MaxCBM        float64   `json:"max_cbm,omitempty"`
	MaxPayloadKG  float64   `json:"max_payload_kg,omitempty"`
	Note          string    `json:"note,omitempty"`
	CreatedBy     uuid.UUID `json:"-"`
}

// AddLineInput carries one finished-goods allocation onto a container.
//
// CBMTotal and WeightKGTotal are snapshotted at add time. Until issue #294
// adds height_mm / weight_kg / cbm to the catalog SKU, the client supplies
// these values from its own SKU registry. Once #294 lands, FE will compute
// `sku.cbm * qty` and `sku.weight_kg * qty` and pass them through unchanged
// — no service-side change required.
type AddLineInput struct {
	ContainerID      uuid.UUID `json:"-"`
	SKUID            uuid.UUID `json:"sku_id"`
	Qty              int       `json:"qty"`
	SalesOrderLineID uuid.UUID `json:"sales_order_line_id"`
	CBMTotal         float64   `json:"cbm_total"`
	WeightKGTotal    float64   `json:"weight_kg_total"`
	AddedBy          uuid.UUID `json:"-"`
}

// TransferLineInput moves part or all of a line from the source container
// (path :id) to the target. When Qty is zero the line is moved in full; when
// 0 < Qty < line.qty the source line is decremented and a new line is
// inserted at the target. Qty > line.qty returns ErrInvalidInput.
//
// CBMTotal / WeightKGTotal must be supplied for any partial transfer because
// the service cannot derive them: the original snapshot was for `line.qty`,
// not the new `Qty` slice. For a full transfer (Qty == 0) they are ignored
// and the source line's snapshot is reused unchanged.
type TransferLineInput struct {
	ContainerID       uuid.UUID `json:"-"`
	LineID            uuid.UUID `json:"line_id"`
	TargetContainerID uuid.UUID `json:"target_container_id"`
	Qty               int       `json:"qty,omitempty"`
	CBMTotal          float64   `json:"cbm_total,omitempty"`
	WeightKGTotal     float64   `json:"weight_kg_total,omitempty"`
	ActorID           uuid.UUID `json:"-"`
}

type TransferLineResult struct {
	SourceLine *ContainerLine `json:"source_line,omitempty"` // nil when the source line was fully consumed
	TargetLine ContainerLine  `json:"target_line"`
}

// SealInput carries the actor for the audit row. BR-D05: sealing flips the
// container to SEALED and atomically bumps qty_shipped on every underlying
// sales_order_line in a single transaction.
type SealInput struct {
	ContainerID uuid.UUID `json:"-"`
	ActorID     uuid.UUID `json:"-"`
	Note        string    `json:"note,omitempty"`
}

// ReopenInput requires Reason — BR-D06 mandates an audit trail when an
// already-sealed container is reopened (admin only).
type ReopenInput struct {
	ContainerID uuid.UUID `json:"-"`
	ActorID     uuid.UUID `json:"-"`
	Reason      string    `json:"reason"`
}

type ShipInput struct {
	ContainerID uuid.UUID `json:"-"`
	ActorID     uuid.UUID `json:"-"`
	Note        string    `json:"note,omitempty"`
}

type CancelInput struct {
	ContainerID uuid.UUID `json:"-"`
	ActorID     uuid.UUID `json:"-"`
	Reason      string    `json:"reason,omitempty"`
}

type ContainerListFilter struct {
	Status        string
	ContainerType string
}

// ── Loading plans (#301) ────────────────────────────────────────────────────
//
// loading_plans is the Excel-driven "intent" layer above container_lines. One
// file = one container; the parser produces a PARSED plan that admin approves
// to lock the version. The kiosk's VERIFY-mode scan (#291) joins
// loading_plan_lines to actual scans.

const (
	LoadingPlanStatusParsed     = "PARSED"
	LoadingPlanStatusValidated  = "VALIDATED"
	LoadingPlanStatusApproved   = "APPROVED"
	LoadingPlanStatusSuperseded = "SUPERSEDED"
)

type LoadingPlan struct {
	ID           uuid.UUID         `json:"id"`
	ContainerID  uuid.UUID         `json:"container_id"`
	ExcelFileURL string            `json:"excel_file_url"`
	ExcelHash    string            `json:"excel_hash"`
	ParsedAt     time.Time         `json:"parsed_at"`
	UploadedBy   uuid.UUID         `json:"uploaded_by"`
	Status       string            `json:"status"`
	Version      int               `json:"version"`
	Notes        string            `json:"notes,omitempty"`
	ApprovedAt   *time.Time        `json:"approved_at,omitempty"`
	ApprovedBy   *uuid.UUID        `json:"approved_by,omitempty"`
	SupersededAt *time.Time        `json:"superseded_at,omitempty"`
	SupersededBy *uuid.UUID        `json:"superseded_by,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	Lines        []LoadingPlanLine `json:"lines,omitempty"`
}

type LoadingPlanLine struct {
	ID               uuid.UUID `json:"id"`
	LoadingPlanID    uuid.UUID `json:"loading_plan_id"`
	SKUID            uuid.UUID `json:"sku_id"`
	QtyPlannedPieces int       `json:"qty_planned_pieces"`
	UnitInExcel      string    `json:"unit_in_excel"`
	QtyInExcel       float64   `json:"qty_in_excel"`
	CustomerSKUCode  string    `json:"customer_sku_code"`
	RawExcelRow      []byte    `json:"raw_excel_row,omitempty"`
	ExcelRowNum      int       `json:"excel_row_num"`
	CreatedAt        time.Time `json:"created_at"`
}

// UploadLoadingPlanInput drives POST /containers/:id/loading-plan. The Excel
// reader is fully consumed during parse — caller hands an io.Reader so the
// service can compute the hash inline. CustomerID disambiguates which
// customer_sku_mappings row is authoritative for each row in the file.
type UploadLoadingPlanInput struct {
	ContainerID  uuid.UUID
	CustomerID   uuid.UUID
	ExcelFileURL string
	UploadedBy   uuid.UUID
	Notes        string
	File         io.Reader
}

// LoadingPlanRowError pinpoints the offending Excel row + column. Code values
// mirror the customer-sku bulk-import convention so the FE can reuse toasts.
type LoadingPlanRowError struct {
	Row     int    `json:"row"`
	Col     string `json:"col,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// LoadingPlanUploadResult is what UploadLoadingPlan returns. When Errors is
// non-empty no rows are persisted (fail-all mirrors BR-D08) and Plan/Lines
// are zero-valued.
type LoadingPlanUploadResult struct {
	Plan     LoadingPlan           `json:"plan,omitempty"`
	Lines    []LoadingPlanLine     `json:"lines,omitempty"`
	Warnings []LoadingPlanRowError `json:"warnings,omitempty"`
	Errors   []LoadingPlanRowError `json:"errors,omitempty"`
}

type ApproveLoadingPlanInput struct {
	PlanID  uuid.UUID
	ActorID uuid.UUID
	Notes   string
	// ConfirmSupersede must be true when the container already has scanned
	// container_lines that would be wiped by this approve (BR-D11). When the
	// flag is false and lines exist, ApproveLoadingPlan returns
	// ErrPreconditionFailed with the affected count so the FE can show the
	// confirm dialog.
	ConfirmSupersede bool
}

// ContainerLineHistoryEntry is one row of container_lines_history (#302). Used
// by GET /containers/:id/lines-history?plan_id= to surface the audit timeline
// to packers when their kiosk forces a reload.
type ContainerLineHistoryEntry struct {
	ID               uuid.UUID  `json:"id"`
	OriginalLineID   uuid.UUID  `json:"original_line_id"`
	ContainerID      uuid.UUID  `json:"container_id"`
	SKUID            uuid.UUID  `json:"sku_id"`
	BarcodeID        *uuid.UUID `json:"barcode_id,omitempty"`
	SupersededAt     time.Time  `json:"superseded_at"`
	SupersededByPlan uuid.UUID  `json:"superseded_by_plan"`
	SupersededByUser uuid.UUID  `json:"superseded_by_user"`
	Reason           string     `json:"reason"`
	RawSnapshot      []byte     `json:"raw_snapshot,omitempty"`
}

// LoadingPlanDiff contrasts a new plan against a prior one for the FE to
// render when v2 supersedes v1.
type LoadingPlanDiff struct {
	Against uuid.UUID             `json:"against"`
	NewPlan uuid.UUID             `json:"new_plan"`
	Added   []LoadingPlanLine     `json:"added"`
	Removed []LoadingPlanLine     `json:"removed"`
	Changed []LoadingPlanLineDiff `json:"changed"`
}

type LoadingPlanLineDiff struct {
	SKUID           uuid.UUID `json:"sku_id"`
	CustomerSKUCode string    `json:"customer_sku_code"`
	OldQty          int       `json:"old_qty"`
	NewQty          int       `json:"new_qty"`
}

type Service interface {
	CreateContainer(ctx context.Context, in CreateContainerInput) (Container, error)
	GetContainer(ctx context.Context, id uuid.UUID) (Container, error)
	ListContainers(ctx context.Context, p httpkit.PageParams, f ContainerListFilter) (httpkit.PagedResult[Container], error)

	AddLine(ctx context.Context, in AddLineInput) (ContainerLine, error)
	DeleteLine(ctx context.Context, containerID, lineID uuid.UUID, actorID uuid.UUID) error
	TransferLine(ctx context.Context, in TransferLineInput) (TransferLineResult, error)

	Seal(ctx context.Context, in SealInput) (Container, error)
	Reopen(ctx context.Context, in ReopenInput) (Container, error)
	Ship(ctx context.Context, in ShipInput) (Container, error)
	Cancel(ctx context.Context, in CancelInput) (Container, error)

	ListStatusLog(ctx context.Context, containerID uuid.UUID) ([]ContainerStatusLogEntry, error)

	// ── Loading plans (#301) ────────────────────────────────────────────────

	// UploadLoadingPlan parses the Excel file, validates every row (every
	// customer_sku_code maps, qty>0, unit non-empty), rejects duplicates of
	// the active plan's hash (BR-D10), and on success persists a PARSED plan
	// with version = previous_active.version + 1 (or 1). When validation
	// fails the result carries Errors populated and zero rows are written
	// (fail-all, BR-D08).
	UploadLoadingPlan(ctx context.Context, in UploadLoadingPlanInput) (LoadingPlanUploadResult, error)

	// GetActiveLoadingPlan returns the latest non-SUPERSEDED plan for a
	// container with its lines hydrated. ErrNotFound when none exists.
	GetActiveLoadingPlan(ctx context.Context, containerID uuid.UUID) (LoadingPlan, error)

	// GetLoadingPlan returns one plan + lines by id (any status). Used by
	// the diff endpoint and audit drill-downs.
	GetLoadingPlan(ctx context.Context, planID uuid.UUID) (LoadingPlan, error)

	// DiffLoadingPlans contrasts `planID` against `againstID` (typically the
	// previously-approved plan) so the FE can show "added / removed / qty
	// changed" before approve.
	DiffLoadingPlans(ctx context.Context, planID, againstID uuid.UUID) (LoadingPlanDiff, error)

	// ApproveLoadingPlan flips PARSED|VALIDATED → APPROVED on the named plan
	// and SUPERSEDED on the previously-active plan for the same container,
	// in a single tx. Idempotent on a plan already APPROVED — returns the
	// row unchanged.
	//
	// BR-D11: when the container has live container_lines (workers already
	// scanned units against v1) the call snapshots every row to
	// container_lines_history, then DELETEs them so packers restart from
	// zero. The caller MUST set ConfirmSupersede = true; otherwise the call
	// returns ErrPreconditionFailed with the affected line count so the FE
	// can render the confirm dialog. Containers with no live lines accept
	// approve unconditionally.
	//
	// BR-D12: refuses approve when the container is SEALED or SHIPPED. The
	// caller must force-unseal first (admin-only path).
	ApproveLoadingPlan(ctx context.Context, in ApproveLoadingPlanInput) (LoadingPlan, error)

	// ListContainerLinesHistory returns the audit trail for one container,
	// optionally filtered by the plan that triggered the supersede. Newest
	// supersede event first.
	ListContainerLinesHistory(ctx context.Context, containerID uuid.UUID, planID *uuid.UUID) ([]ContainerLineHistoryEntry, error)
}

// DefaultCapacityForType returns the ISO defaults for a container type. When
// the caller omits max_cbm / max_payload_kg in CreateContainerInput, the
// service substitutes these. Returning (0,0,false) for an unknown type lets
// the service emit a clean ErrInvalidInput without an additional lookup.
func DefaultCapacityForType(t string) (cbm, payloadKG float64, ok bool) {
	switch t {
	case ContainerType20GP:
		return 33.2, 28000, true
	case ContainerType40GP:
		return 67.7, 26500, true
	case ContainerType40HC:
		return 76.4, 26500, true
	}
	return 0, 0, false
}
