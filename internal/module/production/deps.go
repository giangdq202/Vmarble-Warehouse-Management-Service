package production

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type PlanChecker interface {
	GetPlan(ctx context.Context, planID uuid.UUID) (PlanInfo, error)
}

type PlanInfo struct {
	ID     uuid.UUID
	Status domain.PlanStatus
	SKUIDs []uuid.UUID // SKU IDs from plan items — used to validate CreateWorkOrder
}

type SKUChecker interface {
	GetSKU(ctx context.Context, skuID uuid.UUID) (SKUInfo, error)
}

type SKUInfo struct {
	ID            uuid.UUID
	RequiresMetal bool
	Dimensions    domain.Dimension
}

// UserChecker fetches user identity from the authn module without importing it.
type UserChecker interface {
	GetUser(ctx context.Context, userID uuid.UUID) (UserInfo, error)
}

// UserInfo is the production module's view of a user.
// IsActive lets the labor-entry flow reject worker references to deactivated
// accounts; assignment logic only relies on Role.
type UserInfo struct {
	ID       uuid.UUID
	Role     string
	IsActive bool
}

// WorkOrderNotifier fires SSE notifications for work-order events.
// Implementation lives in internal/platform/events; wired in main.go.
//
// Both methods are best-effort: callers must continue past a non-nil error so a
// transient broker failure never aborts the business write.
type WorkOrderNotifier interface {
	// NotifyAssignment is fired after a planner assigns a CNC operator. Routed
	// to the assignee plus the manager-side audience.
	NotifyAssignment(ctx context.Context, userID, woID, sku string) error
	// NotifyWOStatusChanged is fired after AdvanceStatus persists a transition.
	// Routed to planner + manager + accountant + admin so dashboards can refresh
	// without polling.
	NotifyWOStatusChanged(ctx context.Context, woID, status string) error
}

// SheetAssigner pre-assigns a board sheet to a work order when the work order
// transitions to IN_CUTTING. The sheet must be AVAILABLE; if it is not,
// ErrPreconditionFailed is returned and the advance is aborted.
// BypassOverflow=true allows an admin to issue a new sheet even when the
// remnant overflow status is RED — the bypass is recorded in inventory_audit_log.
// Implementation lives in the inventory module; wired in main.go.
type SheetAssigner interface {
	PreAssignSheet(ctx context.Context, in PreAssignSheetRequest) error
}

// PreAssignSheetRequest is the production module's view of the cross-module
// PreAssignSheet call. Mirrors inventory.PreAssignSheetInput but kept local
// so production does not import the inventory package.
type PreAssignSheetRequest struct {
	SheetID        uuid.UUID
	WorkOrderID    uuid.UUID
	BypassOverflow bool
	ActorID        uuid.UUID
	Reason         string
}

// CostingChecker verifies whether a costing record exists for a work order.
// Implementation lives in the costing module; wired in main.go.
type CostingChecker interface {
	HasCostingRecord(ctx context.Context, workOrderID uuid.UUID) (bool, error)
	// IsCostingFinalized reports whether the costing record for the work order
	// is finalized (immutable per BR-C04). Returns false (no error) when no
	// costing record exists yet.
	IsCostingFinalized(ctx context.Context, workOrderID uuid.UUID) (bool, error)
}

// RemnantAdvisor is the production module's view of inventory's remnant
// suggestion + bypass logging API. Used at CreateWorkOrder time to enforce
// BR-K05: when fitting remnants exist but the planner did not allocate any,
// log a REMNANT_BYPASSED row to inventory_audit_log.
type RemnantAdvisor interface {
	SuggestRemnants(ctx context.Context, requiredDim domain.Dimension) ([]RemnantSuggestionRef, error)
	LogRemnantBypass(ctx context.Context, in LogRemnantBypassRequest) error
}

// RemnantSuggestionRef is the production-side projection of an inventory
// remnant suggestion. Only the remnant ID is needed because the bypass log
// records a list of skipped IDs.
type RemnantSuggestionRef struct {
	RemnantID uuid.UUID
}

// LogRemnantBypassRequest mirrors inventory.LogRemnantBypassInput so the
// production module does not need to import the inventory package.
type LogRemnantBypassRequest struct {
	WorkOrderID         uuid.UUID
	ActorID             uuid.UUID
	SuggestedRemnantIDs []uuid.UUID
	Reason              string
}

// StockChecker reports how many AVAILABLE board sheets back a given material.
// Used at CreateWorkOrder time to enforce BR-K01 (aggregate stock check) so a
// planner cannot create a work order for which the warehouse cannot supply at
// least one sheet per unit. Implementation lives in the inventory module.
type StockChecker interface {
	CountAvailableSheetsByMaterial(ctx context.Context, materialID uuid.UUID) (int, error)
}

// BOMReader returns the SHEET-type material requirements for a SKU. Production
// uses it to project a work order's quantity onto materials so StockChecker can
// be queried per material. The "default" BOM (legacy or DEFAULT variant) is
// what CreateWorkOrder consults — variant-specific BOMs are out of scope here.
type BOMReader interface {
	GetSheetMaterials(ctx context.Context, skuID uuid.UUID) ([]SheetRequirement, error)
}

// SheetRequirement is one row of the projected stock requirement: a SHEET-type
// material referenced by the SKU's BOM. The unit is implicit (sheets); the
// production aggregate check floors the requirement at one sheet per unit
// regardless of QuantityPerUnit because partial sheets cannot be issued.
type SheetRequirement struct {
	MaterialID uuid.UUID
}

// FinishedGoodsHook is called after a work order successfully advances to
// COMPLETED. The packing module's CreateFromCompletedWO is the canonical
// implementation: it generates a barcode + fg_pool row per produced unit so
// the warehouse can scan finished goods at the packing kiosk. Best-effort
// from production's perspective — a non-nil error is logged and the
// AdvanceStatus call still returns success because the WO transition itself
// already persisted.
type FinishedGoodsHook interface {
	OnWOCompleted(ctx context.Context, in WOCompletedEvent) error
}

// WOCompletedEvent is the slim projection production hands to the FG hook.
// Mirrors the fields packing.CreateFromCompletedWOInput needs without
// pulling the packing types into production's import graph.
type WOCompletedEvent struct {
	WorkOrderID      uuid.UUID
	SKUID            uuid.UUID
	SKUCode          string
	SKUName          string
	Dimensions       string
	Quantity         int
	SalesOrderLineID *uuid.UUID
	ProductionPlanID uuid.UUID
	QCPassedBy       uuid.UUID
}
