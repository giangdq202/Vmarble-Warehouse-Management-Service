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
