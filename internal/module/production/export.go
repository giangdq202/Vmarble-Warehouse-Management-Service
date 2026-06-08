package production

import (
	"context"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

const exportMaxLimit = 5000

func (svc *service) ExportWorkOrders(ctx context.Context, p httpkit.PageParams, f WorkOrderListFilter, w io.Writer) error {
	if p.Limit <= 0 || p.Limit > exportMaxLimit {
		p.Limit = exportMaxLimit
	}
	result, err := svc.ListWorkOrders(ctx, p, f)
	if err != nil {
		return err
	}
	return buildWorkOrdersXLSX(result.Items, w)
}

func buildWorkOrdersXLSX(wos []WorkOrder, w io.Writer) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheet := "Work Orders"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return fmt.Errorf("bold style: %w", err)
	}

	headers := []string{"No.", "SKU Code", "SKU Name", "Length (mm)", "Width (mm)", "Quantity", "Actual Qty", "Status", "Priority Boost", "Created At"}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = f.SetCellStyle(sheet, cell, cell, bold)
		_ = f.SetCellValue(sheet, cell, h)
	}

	for i, wo := range wos {
		row := i + 2
		actualQty := ""
		if wo.ActualQty != nil {
			actualQty = fmt.Sprintf("%d", *wo.ActualQty)
		}
		values := []any{
			i + 1, wo.SKUCode, wo.SKUName,
			wo.SKUDimensions.LengthMM, wo.SKUDimensions.WidthMM,
			wo.Quantity, actualQty,
			string(wo.Status), boolToYN(wo.PriorityBoost),
			wo.CreatedAt.Format("2006-01-02"),
		}
		for col, v := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	_ = f.SetColWidth(sheet, "A", "A", 6)
	_ = f.SetColWidth(sheet, "B", "B", 14)
	_ = f.SetColWidth(sheet, "C", "C", 30)
	_ = f.SetColWidth(sheet, "D", "J", 14)

	return f.Write(w)
}

func boolToYN(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}
