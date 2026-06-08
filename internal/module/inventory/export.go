package inventory

import (
	"context"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

const exportMaxLimit = 5000

func (svc *service) ExportLots(ctx context.Context, p httpkit.PageParams, w io.Writer) error {
	if p.Limit <= 0 || p.Limit > exportMaxLimit {
		p.Limit = exportMaxLimit
	}
	result, err := svc.ListLots(ctx, p)
	if err != nil {
		return err
	}
	return buildLotsXLSX(result.Items, w)
}

func buildLotsXLSX(lots []InventoryLot, w io.Writer) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheet := "Inventory Lots"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return fmt.Errorf("bold style: %w", err)
	}

	headers := []string{"No.", "Supplier Ref", "Quantity", "Cost/Sheet Amount", "Cost/Sheet Currency", "Active", "Received At"}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = f.SetCellStyle(sheet, cell, cell, bold)
		_ = f.SetCellValue(sheet, cell, h)
	}

	for i, lot := range lots {
		row := i + 2
		active := "Y"
		if !lot.IsActive {
			active = "N"
		}
		values := []any{
			i + 1, lot.SupplierRef, lot.Quantity,
			lot.CostPerSheet.Amount, lot.CostPerSheet.Currency,
			active, lot.ReceivedAt.Format("2006-01-02"),
		}
		for col, v := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	_ = f.SetColWidth(sheet, "A", "A", 6)
	_ = f.SetColWidth(sheet, "B", "B", 20)
	_ = f.SetColWidth(sheet, "C", "G", 16)

	return f.Write(w)
}
