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
