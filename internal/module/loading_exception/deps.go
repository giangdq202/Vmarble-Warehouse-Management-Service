package loading_exception

import (
	"context"

	"github.com/google/uuid"
)

// SKUChecker validates a SKU exists. Loading exceptions only need existence
// (no extra metadata), so the dep stays slim.
type SKUChecker interface {
	GetSKU(ctx context.Context, skuID uuid.UUID) (SKUInfo, error)
}

type SKUInfo struct {
	ID   uuid.UUID
	Code string
	Name string
}

// SOLineChecker validates a sales_order_line exists and reports the
// (sku_id, qty_planned, qty_shipped) tuple. Used by Approve when the admin
// picks BACKORDER so the carry-over line inherits the same SKU + missing qty.
type SOLineChecker interface {
	GetSOLine(ctx context.Context, soLineID uuid.UUID) (SOLineInfo, error)
}

type SOLineInfo struct {
	ID         uuid.UUID
	SOID       uuid.UUID
	SKUID      uuid.UUID
	QtyOrdered int
	QtyPlanned int
	QtyShipped int
}

// CarryOverCreator is the cross-module hook into sales (#289) used when
// Approve picks BACKORDER (BR-D17). The implementation in main.go delegates
// to sales.CreateCarryOverSOLine which inserts a fresh sales_order_lines
// row pointing at the same SKU/customer/SO with parent_sales_order_line_id
// referencing the original.
//
// Returns the new line id so loading_exceptions.carry_over_so_line_id can
// be stamped before the row is committed.
type CarryOverCreator interface {
	CreateCarryOver(ctx context.Context, in CarryOverInput) (uuid.UUID, error)
}

type CarryOverInput struct {
	ParentSOLineID uuid.UUID
	Qty            int
	Reason         string
	CreatedBy      uuid.UUID
}

// AuditLogger writes a row to inventory_audit_log when an exception is
// created / approved / rejected. Best-effort: a non-nil error is logged via
// slog.Warn but never aborts the business write.
type AuditLogger interface {
	LogException(ctx context.Context, in AuditInput) error
}

type AuditAction string

const (
	AuditActionCreated  AuditAction = "LE_CREATED"
	AuditActionApproved AuditAction = "LE_APPROVED"
	AuditActionRejected AuditAction = "LE_REJECTED"
)

type AuditInput struct {
	Action        AuditAction
	ExceptionID   uuid.UUID
	ContainerID   uuid.UUID
	ExceptionType string
	Resolution    string
	ActorID       uuid.UUID
	Notes         string
}
