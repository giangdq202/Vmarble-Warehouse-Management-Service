package delivery

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SKUChecker validates that a SKU referenced by a container line exists.
// Implementation lives in catalog; wired in cmd/server/main.go.
type SKUChecker interface {
	GetSKU(ctx context.Context, skuID uuid.UUID) (SKUInfo, error)
}

type SKUInfo struct {
	ID   uuid.UUID
	Code string
	Name string
}

// SOLineChecker validates that a sales_order_line_id referenced by AddLine
// exists, belongs to a sales order in a status that allows shipment, and
// reports both qty_planned and qty_shipped so the delivery service can keep
// container_lines.qty + sum(other lines' qty for same SO line) ≤ qty_planned.
//
// The implementation in main.go wraps sales.Service.GetSOLine.
type SOLineChecker interface {
	GetSOLine(ctx context.Context, soLineID uuid.UUID) (SOLineInfo, error)
}

type SOLineInfo struct {
	ID         uuid.UUID
	SOID       uuid.UUID
	SOStatus   string
	SKUID      uuid.UUID
	QtyPlanned int
	QtyShipped int
}

// ShipmentRecorder is the cross-module hook into sales used by Seal.
//
// RecordShipmentTx accepts the delivery transaction so the qty_shipped bump
// runs in the SAME transaction as the container.status flip. This is the
// deliberate exception to the "modules own their pool" convention because
// seal is the moment that "stuff actually left the warehouse" — splitting it
// across two transactions risks an over-counted ship if the sales tx fails
// after the container is already marked SEALED.
//
// Items aggregate by sales_order_line_id; the recorder is allowed to assume
// each SOLineID appears at most once. Returns ErrInvalidInput when any
// proposed bump would push qty_shipped past qty_planned (the DB CHECK
// chk_qty_shipped_le_planned is the authoritative backstop).
type ShipmentRecorder interface {
	RecordShipmentTx(ctx context.Context, tx pgx.Tx, items []ShipmentItem) error
}

type ShipmentItem struct {
	SOLineID uuid.UUID
	Qty      int
}

// FGTracker mirrors finished-goods state (#291) onto the packing module.
// All methods are BEST-EFFORT from delivery's perspective: a non-nil error
// is logged but never aborts the AddLine/DeleteLine/Seal flow. The packing
// pool is a soft allocation — if reserve cannot find enough AVAILABLE rows,
// the warehouse can reconcile manually via the FG list.
type FGTracker interface {
	// ReserveOnAdd flips qty AVAILABLE FG rows matching (sku, soLineID) to
	// RESERVED, stamping container_line_id. Returns the count actually
	// reserved (may be < qty when the pool is short).
	ReserveOnAdd(ctx context.Context, in FGReserveRequest) (int, error)
	// ReleaseOnDelete flips RESERVED FG rows on the deleted line back to AVAILABLE.
	ReleaseOnDelete(ctx context.Context, containerLineID uuid.UUID) error
	// MarkLoadedOnSeal flips RESERVED FG rows on the sealed container to LOADED.
	MarkLoadedOnSeal(ctx context.Context, containerID uuid.UUID) error
}

type FGReserveRequest struct {
	SKUID            uuid.UUID
	SalesOrderLineID uuid.UUID
	Qty              int
	ContainerLineID  uuid.UUID
}

// CustomerSKUResolver translates a customer-facing SKU code (as it appears in
// the customer's packing-list Excel) into the internal catalog SKU id. Used
// by the loading-plan Excel parser (#301). Implementation lives in main.go
// as a thin adapter over sales.GetCustomerSKUMapping so delivery does not
// import the sales module directly.
//
// Returns ErrNotFound when the customer has not mapped that code yet — the
// parser surfaces UNMAPPED_SKU on row N so the operator can fix the mapping
// before re-uploading.
type CustomerSKUResolver interface {
	ResolveCustomerSKU(ctx context.Context, customerID uuid.UUID, code string) (uuid.UUID, error)
}

// LoadingPlanAuditLogger records loading-plan upload + approve events. Same
// best-effort contract as the costing/customer-sku-mapping audits: a non-nil
// error is logged via slog.Warn but never aborts the business write.
type LoadingPlanAuditLogger interface {
	LogLoadingPlan(ctx context.Context, in AuditLoadingPlanInput) error
}

type AuditLoadingPlanAction string

const (
	AuditLPActionUploaded   AuditLoadingPlanAction = "LP_UPLOADED"
	AuditLPActionApproved   AuditLoadingPlanAction = "LP_APPROVED"
	AuditLPActionSuperseded AuditLoadingPlanAction = "LP_SUPERSEDED"
)

type AuditLoadingPlanInput struct {
	Action      AuditLoadingPlanAction
	PlanID      uuid.UUID
	ContainerID uuid.UUID
	Version     int
	ExcelHash   string
	ActorID     uuid.UUID
	Notes       string
}

// PlanReloadNotifier fires the SSE PLAN_RELOAD event after a v2 supersede
// wipes container_lines (BR-D13). Best-effort — a non-nil error is logged but
// never aborts the supersede write because the user-facing notice is a
// nice-to-have, not the source of truth (the kiosk's next scan will refresh
// state regardless).
//
// Implementation lives in cmd/server/main.go as a thin adapter over
// events.Publisher so delivery does not depend on the events package.
type PlanReloadNotifier interface {
	NotifyPlanReload(ctx context.Context, in PlanReloadNotice) error
}

type PlanReloadNotice struct {
	ContainerID     uuid.UUID
	NewPlanID       uuid.UUID
	NewVersion      int
	SupersededLines int
	ActorID         uuid.UUID
}

// PendingExceptionsChecker is the BR-D14 SEAL guard hook (#303). Seal asks
// the loading_exception module whether any rows on this container still
// have approved_by IS NULL; if Count > 0 the seal is refused with 412 and
// the response body lists the blocking ids. Optional — when nil the guard
// is bypassed (legacy containers / migration window).
type PendingExceptionsChecker interface {
	PendingForContainer(ctx context.Context, containerID uuid.UUID) (PendingExceptionsSummary, error)
}

type PendingExceptionsSummary struct {
	Count int         `json:"count"`
	IDs   []uuid.UUID `json:"ids"`
}

// ShortShippedAutoCreator is the BR-D15 hook (#303). Seal calls this once it
// knows actual loaded qty per (container, sku) is short of the active loading
// plan; the implementation raises a SHORT_SHIPPED loading_exception so the
// admin can choose how to resolve. Optional — nil disables the auto-create.
type ShortShippedAutoCreator interface {
	AutoCreateShortShipped(ctx context.Context, in ShortShippedAutoInput) error
}

type ShortShippedAutoInput struct {
	ContainerID   uuid.UUID
	LoadingPlanID *uuid.UUID
	SKUID         uuid.UUID
	MissingQty    int
	ActorID       uuid.UUID
}

// ShortageReport is what the delivery store returns when joining the active
// loading_plan_lines against actual container_lines. Used internally by Seal
// to decide which SHORT_SHIPPED exceptions to auto-raise (BR-D15).
type ShortageReport struct {
	LoadingPlanID *uuid.UUID
	Items         []ShortageItem
}

type ShortageItem struct {
	SKUID      uuid.UUID
	Planned    int
	Actual     int
	MissingQty int
}
