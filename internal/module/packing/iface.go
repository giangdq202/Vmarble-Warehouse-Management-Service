// Package packing owns the Finished-Goods Pool (fg_pool) and defect tracking
// (fg_defect). One physical FG = one barcode = one fg_pool row, generated
// when a WorkOrder advances to COMPLETED. Status machine:
//
//	AVAILABLE -> RESERVED (delivery.AddLine) -> LOADED (delivery.Seal)
//	AVAILABLE -> DEFECT (packing.ReportDefect) -> DISPOSED (resolve)
//	RESERVED  -> DEFECT  (line auto-removed first) -> DISPOSED / AVAILABLE
//
// v1 covers core CRUD + scan + defect lifecycle. v2 (loading-plan VERIFY) and
// v3 (defect routing -> supplemental WO) live as follow-up issues.
package packing

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// FG status values; mirror chk_fg_status in 00052.
const (
	FGStatusAvailable = "AVAILABLE"
	FGStatusReserved  = "RESERVED"
	FGStatusLoaded    = "LOADED"
	FGStatusDefect    = "DEFECT"
	FGStatusDisposed  = "DISPOSED"
)

// Defect reason / resolution values; mirror chk_fg_defect_reason and
// chk_fg_defect_resolution.
const (
	DefectReasonBroken           = "BROKEN"
	DefectReasonWrongSize        = "WRONG_SIZE"
	DefectReasonMissingAccessory = "MISSING_ACCESSORY"
	DefectReasonScratched        = "SCRATCHED"
	DefectReasonOther            = "OTHER"

	DefectResolutionDiscard   = "DISCARD"
	DefectResolutionRework    = "REWORK"
	DefectResolutionReturnNCC = "RETURN_NCC"
)

type FGPool struct {
	ID               uuid.UUID  `json:"id"`
	WorkOrderID      uuid.UUID  `json:"work_order_id"`
	SKUID            uuid.UUID  `json:"sku_id"`
	SKUCode          string     `json:"sku_code,omitempty"`
	SKUName          string     `json:"sku_name,omitempty"`
	BarcodeID        uuid.UUID  `json:"barcode_id"`
	SalesOrderLineID *uuid.UUID `json:"sales_order_line_id,omitempty"`
	Status           string     `json:"status"`
	ContainerLineID  *uuid.UUID `json:"container_line_id,omitempty"`
	QCPassedAt       time.Time  `json:"qc_passed_at"`
	QCPassedBy       uuid.UUID  `json:"qc_passed_by"`
	CreatedAt        time.Time  `json:"created_at"`
}

type FGDefect struct {
	ID         uuid.UUID  `json:"id"`
	FGPoolID   uuid.UUID  `json:"fg_pool_id"`
	Reason     string     `json:"reason"`
	Detail     string     `json:"detail,omitempty"`
	PhotoURLs  []string   `json:"photo_urls,omitempty"`
	DetectedBy uuid.UUID  `json:"detected_by"`
	DetectedAt time.Time  `json:"detected_at"`
	Resolution string     `json:"resolution,omitempty"`
	ResolvedBy *uuid.UUID `json:"resolved_by,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Note       string     `json:"note,omitempty"`
}

// CreateFromCompletedWOInput is the payload for the production -> packing
// hook. Qty is taken from the WO at advance time; the hook is idempotent —
// re-calling for the same WO returns the existing rows without inserting
// duplicates (production may retry on transient failure).
type CreateFromCompletedWOInput struct {
	WorkOrderID      uuid.UUID
	SKUID            uuid.UUID
	SKUCode          string
	SKUName          string
	Dimensions       string
	Quantity         int
	SalesOrderLineID *uuid.UUID
	ProductionPlanID uuid.UUID
	POID             uuid.UUID // legacy PO-rooted plans; zero for SO-rooted
	QCPassedBy       uuid.UUID
	ProducedDate     time.Time
}

// ScanResult is the kiosk's view of a scanned barcode. SuggestedContainers
// lists OPEN/LOADING containers carrying the same SO line, ordered by
// fill_pct ascending so the operator picks the most-empty container first.
type ScanResult struct {
	FG                  FGPool                `json:"fg"`
	WOStatus            string                `json:"wo_status"`
	SuggestedContainers []ContainerSuggestion `json:"suggested_containers"`
}

type ContainerSuggestion struct {
	ContainerID   uuid.UUID `json:"container_id"`
	Code          string    `json:"code"`
	Status        string    `json:"status"`
	FillPctCBM    float64   `json:"fill_pct_cbm"`
	FillPctMass   float64   `json:"fill_pct_mass"`
	ContainerType string    `json:"container_type"`
}

type ReportDefectInput struct {
	BarcodeID  uuid.UUID `json:"barcode_id"`
	Reason     string    `json:"reason"`
	Detail     string    `json:"detail,omitempty"`
	PhotoURLs  []string  `json:"photo_urls,omitempty"`
	DetectedBy uuid.UUID `json:"-"`
}

type ResolveDefectInput struct {
	DefectID   uuid.UUID `json:"-"`
	Resolution string    `json:"resolution"`
	Note       string    `json:"note,omitempty"`
	ResolvedBy uuid.UUID `json:"-"`
}

type FGListFilter struct {
	Status         string
	SKUID          *uuid.UUID
	SOLineID       *uuid.UUID
	WorkOrderID    *uuid.UUID
}

type Service interface {
	// CreateFromCompletedWO is the production -> packing hook. Idempotent:
	// repeated calls for the same WO return the previously generated rows.
	// Best-effort from production's perspective — the AdvanceStatus tx does
	// NOT roll back if this returns an error; the caller logs and moves on.
	CreateFromCompletedWO(ctx context.Context, in CreateFromCompletedWOInput) ([]FGPool, error)

	GetFG(ctx context.Context, id uuid.UUID) (FGPool, error)
	ListFG(ctx context.Context, p httpkit.PageParams, f FGListFilter) (httpkit.PagedResult[FGPool], error)

	// ScanBarcode resolves a barcode to its FG and returns suggested loadable
	// containers. BR-PK01: returns ErrPreconditionFailed if the underlying
	// WO has not reached COMPLETED.
	ScanBarcode(ctx context.Context, barcodeID, actorID uuid.UUID) (ScanResult, error)

	// ReportDefect flips the FG to DEFECT and inserts the defect row. If the
	// FG was RESERVED, it is first released from its container_line via the
	// ContainerLineRemover dep so the line stops counting toward the
	// container's qty. BR-PK02 / BR-PK03.
	ReportDefect(ctx context.Context, in ReportDefectInput) (FGDefect, error)

	// ResolveDefect records the resolution + audit columns and flips fg_pool
	// status: DISCARD/RETURN_NCC -> DISPOSED, REWORK -> AVAILABLE so the FG
	// can re-enter the pool after rework. v3 will create a supplemental WO
	// for REWORK; v1 just flips state.
	ResolveDefect(ctx context.Context, in ResolveDefectInput) (FGDefect, error)

	// ReserveOnContainerAdd is called by delivery.AddLine inside its tx. The
	// hook picks `qty` AVAILABLE rows matching (sku, soLineID) and flips
	// them to RESERVED, stamping container_line_id. Returns the number of
	// rows actually reserved (may be less than qty if the pool is short —
	// the caller decides whether that is fatal).
	ReserveOnContainerAdd(ctx context.Context, in ReserveInput) (int, error)

	// ReleaseOnContainerDelete is called by delivery.DeleteLine. Flips every
	// fg_pool row pointing at the line back to AVAILABLE.
	ReleaseOnContainerDelete(ctx context.Context, containerLineID uuid.UUID) error

	// MarkLoadedOnSeal is called by delivery.Seal. Flips every RESERVED
	// fg_pool row whose container_line_id is on the sealed container to
	// LOADED. Idempotent.
	MarkLoadedOnSeal(ctx context.Context, containerID uuid.UUID) error
}

type ReserveInput struct {
	SKUID           uuid.UUID
	SalesOrderLineID uuid.UUID
	Qty             int
	ContainerLineID uuid.UUID
}
