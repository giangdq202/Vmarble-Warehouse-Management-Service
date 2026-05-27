package sales

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SKUChecker validates that a SKU referenced by a sales order line exists.
// Implementation lives in the catalog module; wired in cmd/server/main.go.
// We only need existence here — pricing comes from the line itself, and
// requires_metal is a production-side concern resolved at WO time.
type SKUChecker interface {
	GetSKU(ctx context.Context, skuID uuid.UUID) (SKUInfo, error)
}

// SKUInfo is the sales-side projection of a catalog SKU. Slim on purpose —
// adding fields here forces the cross-module adapter to grow, which we want
// to keep visible.
type SKUInfo struct {
	ID   uuid.UUID
	Code string
	Name string
}

// ProductionSplitter creates a production plan plus the work orders that
// realize a SplitToPlan call. The adapter in cmd/server/main.go composes
// planning.CreatePlan + production.CreateWorkOrder per allocation in a
// single planning-side transaction so plan + WO insertion is atomic.
//
// Sales performs its qty_planned mutation in a separate transaction. If
// production succeeds but the sales tx fails, the WOs exist without
// matching qty_planned bumps — a recovery cron is out of scope for Phase A;
// the failure window is logged and surfaced as a 500 to the planner so the
// reconcile happens manually. Phase B will move both writes behind a single
// pool-level transaction once the planning module exposes a Tx-aware API.
//
// Each item carries the sales_order_line_id it serves so the lineage
// SO → Plan → WO is preserved end-to-end (consumed by #292 carry-over and
// #315 cross-SO FG reassignment).
type ProductionSplitter interface {
	CreatePlanWithWOs(ctx context.Context, in CreatePlanWithWOsRequest) (CreatePlanWithWOsResult, error)
}

type CreatePlanWithWOsRequest struct {
	SalesOrderID uuid.UUID
	Deadline     *time.Time
	ActorID      uuid.UUID
	Items        []CreatePlanWOItem
}

type CreatePlanWOItem struct {
	SOLineID uuid.UUID
	SKUID    uuid.UUID
	Quantity int
}

type CreatePlanWithWOsResult struct {
	PlanID       uuid.UUID
	PlanCode     string
	WorkOrderIDs []uuid.UUID
}

// CustomerSKUMappingAuditLogger writes a row to the shared inventory_audit_log
// after every customer_sku_mappings mutation. Implementation lives in
// cmd/server/main.go as salesMappingAuditAdapter to keep sales free of any
// inventory module dependency — the audit table is a cross-module ledger
// keyed by entity_type='CUSTOMER_SKU_MAPPING'. Calls are best-effort: a
// non-nil error is logged via slog.Warn but never propagates so a transient
// audit-write failure does not roll back the mapping mutation itself.
type CustomerSKUMappingAuditLogger interface {
	LogCustomerSKUMapping(ctx context.Context, in AuditCustomerSKUMappingInput) error
}

// AuditCustomerSKUMappingAction enumerates the mutations worth recording.
type AuditCustomerSKUMappingAction string

const (
	AuditCSMActionCreated      AuditCustomerSKUMappingAction = "CSM_CREATED"
	AuditCSMActionUpdated      AuditCustomerSKUMappingAction = "CSM_UPDATED"
	AuditCSMActionDeleted      AuditCustomerSKUMappingAction = "CSM_DELETED"
	AuditCSMActionBulkImported AuditCustomerSKUMappingAction = "CSM_BULK_IMPORTED"
)

// AuditCustomerSKUMappingInput is the payload the adapter persists. Metadata
// fields (PreviousSKUID, NewSKUID, RowsImported) are optional and rendered as
// JSON keys when non-zero so accountants can reconstruct the change without
// joining other tables.
type AuditCustomerSKUMappingInput struct {
	Action          AuditCustomerSKUMappingAction
	CustomerID      uuid.UUID
	CustomerSKUCode string
	SKUID           uuid.UUID  // current value after the mutation; zero on delete-only
	PreviousSKUID   *uuid.UUID // only set when an UPDATE actually moved sku_id
	RowsImported    int        // only set for BULK_IMPORTED
	ActorID         uuid.UUID
	Notes           string
}
