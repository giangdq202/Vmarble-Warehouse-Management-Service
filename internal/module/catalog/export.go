package catalog

import (
	"context"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

const exportMaxLimit = 5000

func (svc *service) ExportSKUs(ctx context.Context, p httpkit.PageParams, w io.Writer) error {
	if p.Limit <= 0 || p.Limit > exportMaxLimit {
		p.Limit = exportMaxLimit
	}
	result, err := svc.ListSKUs(ctx, p)
	if err != nil {
		return err
	}
	return buildSKUsXLSX(result.Items, w)
}

func buildSKUsXLSX(skus []SKU, w io.Writer) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	sheet := "SKUs"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return fmt.Errorf("bold style: %w", err)
	}

	headers := []string{"No.", "Code", "Name", "Length (mm)", "Width (mm)", "Height (mm)", "Weight (kg)", "HS Code", "CBM/Unit", "Requires Metal", "Active"}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = f.SetCellStyle(sheet, cell, cell, bold)
		_ = f.SetCellValue(sheet, cell, h)
	}

	for i, s := range skus {
		row := i + 2
		heightMM := ""
		if s.HeightMM != nil {
			heightMM = fmt.Sprintf("%d", *s.HeightMM)
		}
		weightKg := ""
		if s.WeightKg != nil {
			weightKg = fmt.Sprintf("%.3f", *s.WeightKg)
		}
		hsCode := ""
		if s.HSCode != nil {
			hsCode = *s.HSCode
		}
		cbm := ""
		if s.CbmPerUnit != nil {
			cbm = fmt.Sprintf("%.4f", *s.CbmPerUnit)
		}
		values := []any{
			i + 1, s.Code, s.Name,
			s.Dimensions.LengthMM, s.Dimensions.WidthMM,
			heightMM, weightKg, hsCode, cbm,
			boolToYN(s.RequiresMetal), boolToYN(s.IsActive),
		}
		for col, v := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	_ = f.SetColWidth(sheet, "A", "A", 6)
	_ = f.SetColWidth(sheet, "B", "B", 14)
	_ = f.SetColWidth(sheet, "C", "C", 30)
	_ = f.SetColWidth(sheet, "D", "K", 14)

	return f.Write(w)
}

func boolToYN(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}
