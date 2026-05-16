package reports

import (
	"context"
	"time"
)

// CostingRow is the projected costing-record shape used in
// /reports/export/costings.xlsx. The reader maps from costing.CostingRecord
// + a SKU/WO lookup so the reports module never imports the costing package.
type CostingRow struct {
	WorkOrderID    string
	SKUCode        string
	SKUName        string
	CostingType    string // ESTIMATED / ACTUAL
	MaterialCost   int64
	AuxiliaryCost  int64
	LaborCost      int64
	TotalCost      int64
	Finalized      bool
	CreatedAt      time.Time
}

// PORow is one row in /reports/export/purchase-orders.xlsx. Items are
// concatenated into a single string so the workbook stays one row per PO —
// the accountant comparing against a vendor invoice doesn't want every line
// to spawn a new spreadsheet row.
type PORow struct {
	Code         string
	Supplier     string
	Status       string
	MaterialName string
	OrderedAt    *time.Time
	ReceivedAt   *time.Time
	Items        string // pre-formatted "5×1000×600 @150,000đ; 3×800×400 @120,000đ"
	TotalCost    int64
	CreatedAt    time.Time
}

// SKURow is the catalog-export row.
type SKURow struct {
	Code          string
	Name          string
	LengthMM      int
	WidthMM       int
	RequiresMetal bool
	IsActive      bool
	BOMSummary    string // pre-formatted "MDF 18mm × 0.5; metal × 0.1"
}

// WORow is the work-order summary row, including the latest costing snapshot
// when present.
type WORow struct {
	ID         string
	SKUCode    string
	SKUName    string
	Quantity   int
	Status     string
	AssignedAt *time.Time
	CreatedAt  time.Time
	TotalCost  *int64 // nil when no costing record exists
}

// WasteRow mirrors costing.WasteReportRow projected to primitives.
type WasteRow struct {
	MaterialName   string
	SheetsConsumed int
	WasteAreaMM2   int64
	AvgSheetCost   int64
	TotalWasteCost int64
}

// CostingReader returns all costing records overlapping the given period.
// "Overlap" is defined as created_at ∈ [From, To) when both bounds are set.
type CostingReader interface {
	ListCostingsInPeriod(ctx context.Context, p Period) ([]CostingRow, error)
}

type POReader interface {
	ListPOsInPeriod(ctx context.Context, p Period) ([]PORow, error)
}

type SKUReader interface {
	ListAllSKUs(ctx context.Context) ([]SKURow, error)
}

type WOReader interface {
	ListWorkOrdersInPeriod(ctx context.Context, p Period) ([]WORow, error)
}

type WasteReader interface {
	ListWasteInPeriod(ctx context.Context, p Period) ([]WasteRow, error)
}
