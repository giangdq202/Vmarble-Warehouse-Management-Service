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
package reports

import (
	"context"
	"time"
)

// Period narrows an export to [From, To). Both bounds are inclusive on the
// "from" side and exclusive on the "to" side so a daily report run with
// from=to does not double-count records on the boundary.
type Period struct {
	From *time.Time
	To   *time.Time
}

// Service generates the Excel files. Each method returns the raw .xlsx bytes
// plus a suggested filename so the handler can set Content-Disposition.
type Service interface {
	ExportCostings(ctx context.Context, p Period) (ExportFile, error)
	ExportPurchaseOrders(ctx context.Context, p Period) (ExportFile, error)
	ExportSKUs(ctx context.Context) (ExportFile, error)
	ExportWorkOrders(ctx context.Context, p Period) (ExportFile, error)
	ExportWaste(ctx context.Context, p Period) (ExportFile, error)
}

// ExportFile is the in-memory result of a generated workbook.
type ExportFile struct {
	Filename string
	Bytes    []byte
}
