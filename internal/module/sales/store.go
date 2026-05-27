package sales

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// store is the sales module's persistence boundary. The service depends on
// this interface so unit tests can swap in a fake without standing up
// Postgres. The pgstore implementation in pgstore.go owns the SQL.
type store interface {
	// Customer ----------------------------------------------------------------

	// nextCustomerCode draws the next value from customer_code_seq and formats
	// it as KH%03d. Used when CreateCustomerInput.Code is blank.
	nextCustomerCode(ctx context.Context) (string, error)
	insertCustomer(ctx context.Context, c Customer) error
	selectCustomerByID(ctx context.Context, id uuid.UUID) (Customer, error)
	selectCustomersPaged(ctx context.Context, p httpkit.PageParams, activeOnly bool) ([]Customer, int, error)
	updateCustomer(ctx context.Context, c Customer) error
	customerCodeExists(ctx context.Context, code string) (bool, error)

	// Sales order -------------------------------------------------------------

	// nextSOCode mutates the daily counter row atomically (INSERT … ON
	// CONFLICT DO UPDATE … RETURNING) and returns the formatted code
	// SO{YYYYMMDD}-{seq3}. Caller passes the wall clock so deterministic
	// tests can fix the date.
	nextSOCode(ctx context.Context, now time.Time) (string, error)
	insertSO(ctx context.Context, so SalesOrder) error
	insertSOLines(ctx context.Context, lines []SalesOrderLine) error
	deleteSOLinesBySO(ctx context.Context, soID uuid.UUID) error
	selectSOByID(ctx context.Context, id uuid.UUID) (SalesOrder, error)
	selectSOLinesBySOID(ctx context.Context, soID uuid.UUID) ([]SalesOrderLine, error)
	selectSOsPaged(ctx context.Context, p httpkit.PageParams, f SOListFilter) ([]SalesOrder, int, error)
	updateSO(ctx context.Context, so SalesOrder) error
	updateSOStatus(ctx context.Context, id uuid.UUID, status string) error

	// selectSOLineByID returns one line plus its parent SO in a single
	// round-trip. Used by GetSOLine to drive cross-module validation in
	// delivery.AddLine.
	selectSOLineByID(ctx context.Context, soLineID uuid.UUID) (SalesOrderLine, SalesOrder, error)

	// recordShipmentTx runs the qty_shipped bump + parent-SO status recompute
	// inside the caller's pgx.Tx — used by delivery.Seal so the entire seal
	// (container.status flip + qty_shipped move) lives in a single tx. Returns
	// ErrInvalidInput on CHECK violation (chk_qty_shipped_le_planned).
	recordShipmentTx(ctx context.Context, tx pgx.Tx, items []ShipmentItemInput) error

	// insertCarryOverSOLine inserts a new sales_order_line under the parent
	// SO, copying SKU + unit_price from the parent line and stamping
	// parent_sales_order_line_id (BR-D17). Returns the new line id.
	insertCarryOverSOLine(ctx context.Context, in CarryOverSOLineInput) (uuid.UUID, error)

	// SplitToPlan support -----------------------------------------------------

	// withTx runs fn inside a single transaction. The store handles
	// pgxpool.BeginTx/Commit/Rollback so the service stays SQL-free.
	withTx(ctx context.Context, fn func(tx txStore) error) error

	// Customer SKU mappings (#304) -------------------------------------------

	// insertCustomerSKUMapping returns ErrInvalidInput on PK collision (BR-CSM02)
	// or FK violation (unknown customer / SKU). The store maps the underlying
	// pgx errors to sentinel errors so the service stays driver-agnostic.
	insertCustomerSKUMapping(ctx context.Context, m CustomerSKUMapping) error

	// selectCustomerSKUMappingsPaged supports an optional customer_id filter.
	// Pass filter with nil CustomerID to list everything (admin only).
	selectCustomerSKUMappingsPaged(ctx context.Context, p httpkit.PageParams, f CustomerSKUMappingFilter) ([]CustomerSKUMapping, int, error)

	selectCustomerSKUMappingByPK(ctx context.Context, customerID uuid.UUID, code string) (CustomerSKUMapping, error)

	// updateCustomerSKUMapping refreshes sku_id and/or notes plus updated_at.
	// Returns ErrNotFound when the row does not exist; ErrInvalidInput on FK
	// violation (unknown SKU).
	updateCustomerSKUMapping(ctx context.Context, m CustomerSKUMapping) error

	deleteCustomerSKUMapping(ctx context.Context, customerID uuid.UUID, code string) error

	// bulkInsertCustomerSKUMappings inserts every row inside a single tx.
	// Any PK or FK violation rolls the whole batch back so the service can
	// surface fail-all semantics (BR-D08 mirror).
	bulkInsertCustomerSKUMappings(ctx context.Context, rows []CustomerSKUMapping) error
}

// txStore is the subset of store operations safe to call from inside a
// transactional split. All methods here run on a pgx.Tx; reads use
// SELECT ... FOR UPDATE so two concurrent splits cannot double-increment
// qty_planned for the same line (BR-S02).
type txStore interface {
	// lockSOForUpdate reads the SO row with FOR UPDATE so concurrent split or
	// confirm calls serialize on it. Returns ErrNotFound if the SO does not
	// exist.
	lockSOForUpdate(ctx context.Context, id uuid.UUID) (SalesOrder, error)

	// lockAndReadSOLines locks the listed lines FOR UPDATE and returns them
	// in the same order. Missing IDs return ErrNotFound.
	lockAndReadSOLines(ctx context.Context, lineIDs []uuid.UUID) ([]SalesOrderLine, error)

	// incrementQtyPlanned bumps qty_planned by delta on the line. The DB
	// CHECK chk_qty_planned_le_ordered guarantees we never exceed
	// qty_ordered (BR-S02); this method returns ErrInvalidInput if the
	// CHECK fires (a race with another split that snuck in between read
	// and write — the FOR UPDATE makes this rare but defensible).
	incrementQtyPlanned(ctx context.Context, lineID uuid.UUID, delta int) error

	// updateStatusIfCurrent flips status from one of `expected` to `target`.
	// Returns true when a row was updated. Used to bump CONFIRMED →
	// IN_PRODUCTION on the first split (BR-S07) without overwriting later
	// states like PARTIALLY_SHIPPED.
	updateStatusIfCurrent(ctx context.Context, id uuid.UUID, expected []string, target string) (bool, error)
}
