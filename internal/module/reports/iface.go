// Package reports owns the Excel export endpoints used by accountants and
// the business owner. Five reports are exposed today:
//
//	GET /api/v1/reports/export/costings.xlsx
//	GET /api/v1/reports/export/purchase-orders.xlsx
//	GET /api/v1/reports/export/skus.xlsx
//	GET /api/v1/reports/export/work-orders.xlsx
//	GET /api/v1/reports/export/waste.xlsx
//
// Every endpoint accepts an optional ?from=YYYY-MM-DD&to=YYYY-MM-DD period
// filter and returns a single-sheet .xlsx file with VND amounts formatted
// with thousand separators and dates as dd/mm/yyyy. Auth is enforced at the
// route level — only `admin` and `accountant` reach the handler.
//
// The module owns no domain entities. It is a thin export adapter that pulls
// data from existing module services through reader interfaces declared in
// deps.go.
//
// # Streaming pipeline (#273)
//
// Workbooks are streamed end-to-end: pgx rows are fed through a callback
// iterator into excelize's StreamWriter, which flushes cells to a temp file
// before the final WriteTo. Heap usage is O(1) per row regardless of dataset
// size, so a 1M-row export does not balloon RAM on a low-spec VPS.
//
// Period bounds are capped at MaxPeriodDays (90) when both From and To are
// set; the SKU export — which has no time bound — caps at MaxSKURows
// (50_000). Both caps are enforced at the service entry, before any DB query
// runs, so a misconfigured FE never reaches the streaming path.
package reports

import (
	"context"
	"io"
	"time"
)

// MaxPeriodDays is the largest [from, to) span the period-bounded exports
// accept. Larger spans return ErrInvalidInput (400) so a single click cannot
// dump multiple years of data and OOM the process. The cap matches the
// average accounting cycle (1 quarter) plus a margin.
const MaxPeriodDays = 90

// MaxSKURows caps the SKU catalog export at 50_000 rows. The catalog is
// rarely larger than a few thousand SKUs, but the SQL still binds LIMIT to
// guarantee bounded RAM in the unlikely case of an explosion or migration
// glitch that briefly inflates the table.
const MaxSKURows = 50_000

// Period narrows an export to [From, To). Both bounds are inclusive on the
// "from" side and exclusive on the "to" side so a daily report run with
// from=to does not double-count records on the boundary.
type Period struct {
	From *time.Time
	To   *time.Time
}

// Service generates the Excel files. Each method writes the workbook bytes
// directly to the supplied writer (typically gin's c.Writer) and returns the
// suggested filename so the handler can set Content-Disposition before the
// stream begins.
//
// The writer is expected to be flushable and to honor ctx cancellation —
// when the client aborts, the underlying pgx rows are closed and the
// StreamWriter is discarded without committing.
type Service interface {
	ExportCostings(ctx context.Context, w io.Writer, p Period) (filename string, err error)
	ExportPurchaseOrders(ctx context.Context, w io.Writer, p Period) (filename string, err error)
	ExportSKUs(ctx context.Context, w io.Writer) (filename string, err error)
	ExportWorkOrders(ctx context.Context, w io.Writer, p Period) (filename string, err error)
	ExportWaste(ctx context.Context, w io.Writer, p Period) (filename string, err error)
}
