package sales

import (
	"context"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

const exportMaxLimit = 5000

func (svc *service) ExportSOs(ctx context.Context, p httpkit.PageParams, f SOListFilter, w io.Writer) error {
	if p.Limit <= 0 || p.Limit > exportMaxLimit {
		p.Limit = exportMaxLimit
	}
	result, err := svc.ListSOs(ctx, p, f)
	if err != nil {
		return err
	}
	return buildSOsXLSX(result.Items, w)
}

func buildSOsXLSX(orders []SalesOrder, w io.Writer) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheet := "Sales Orders"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return fmt.Errorf("bold style: %w", err)
	}

	headers := []string{"No.", "Code", "Customer", "Country", "Currency", "Status", "Incoterm", "Port of Loading", "Port of Discharge", "Expected Ship Date", "Created At"}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = f.SetCellStyle(sheet, cell, cell, bold)
		_ = f.SetCellValue(sheet, cell, h)
	}

	for i, so := range orders {
		row := i + 2
		shipDate := ""
		if so.ExpectedShipDate != nil {
			shipDate = so.ExpectedShipDate.Format("2006-01-02")
		}
		customerName := so.CustomerName
		if customerName == "" {
			customerName = so.CustomerCode
		}
		values := []any{
			i + 1, so.Code, customerName, so.CustomerCountry,
			so.Currency, so.Status, so.Incoterm,
			so.PortOfLoading, so.PortOfDischarge,
			shipDate, so.CreatedAt.Format("2006-01-02"),
		}
		for col, v := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	_ = f.SetColWidth(sheet, "A", "A", 6)
	_ = f.SetColWidth(sheet, "B", "B", 14)
	_ = f.SetColWidth(sheet, "C", "C", 28)
	_ = f.SetColWidth(sheet, "D", "K", 16)

	return f.Write(w)
}
