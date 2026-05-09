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
}

// UserChecker fetches user identity from the authn module without importing it.
type UserChecker interface {
	GetUser(ctx context.Context, userID uuid.UUID) (UserInfo, error)
}

// UserInfo is the production module's view of a user — role is all we need.
type UserInfo struct {
	ID   uuid.UUID
	Role string
}

// WorkOrderNotifier fires a notification after a work order is assigned.
// Implementation lives in internal/platform/events; wired in main.go.
type WorkOrderNotifier interface {
	NotifyAssignment(ctx context.Context, userID, woID, sku string) error
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
}
