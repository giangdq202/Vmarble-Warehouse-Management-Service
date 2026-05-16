package reports

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/xuri/excelize/v2"
)

// service generates Excel workbooks from the reader-projected rows. It does
// not own any state — every export is fresh.
type service struct {
	costings CostingReader
	pos      POReader
	skus     SKUReader
	wos      WOReader
	waste    WasteReader
}

// NewService wires the readers. Any reader may be nil; the corresponding
// export will return ErrInvalidInput so misconfiguration is surfaced as a
// 400 rather than a panic.
func NewService(costings CostingReader, pos POReader, skus SKUReader, wos WOReader, waste WasteReader) Service {
	return &service{costings: costings, pos: pos, skus: skus, wos: wos, waste: waste}
}

const (
	dateFormat   = "dd/mm/yyyy"
	moneyFormat  = `#,##0" đ"`
	intFormat    = "#,##0"
)

// build constructs a single-sheet workbook with the given header row and
// data rows. dataFormatters defines per-column number formats by column
// index (0-based); columns absent from the map fall back to plain text.
//
// excelize is friendly enough that a single helper covers all five reports —
// the variation between them lives in the row mappers, not the workbook
// scaffolding.
func build(sheet string, headers []string, rows [][]any, dataFormatters map[int]string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	idx, err := f.NewSheet(sheet)
	if err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	f.SetActiveSheet(idx)
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, fmt.Errorf("delete default sheet: %w", err)
	}

	// Header row — bold, with a frozen pane below so accountants can scroll
	// the data while keeping column titles in view.
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return nil, fmt.Errorf("header style: %w", err)
	}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return nil, fmt.Errorf("set header %s: %w", cell, err)
		}
	}
	if len(headers) > 0 {
		first, _ := excelize.CoordinatesToCellName(1, 1)
		last, _ := excelize.CoordinatesToCellName(len(headers), 1)
		if err := f.SetCellStyle(sheet, first, last, headerStyle); err != nil {
			return nil, fmt.Errorf("apply header style: %w", err)
		}
	}
	if err := f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, fmt.Errorf("freeze pane: %w", err)
	}

	// Pre-build the per-column styles so we don't re-create one per cell.
	colStyles := map[int]int{}
	for col, fmtStr := range dataFormatters {
		s, err := f.NewStyle(&excelize.Style{NumFmt: 0, CustomNumFmt: &fmtStr})
		if err != nil {
			return nil, fmt.Errorf("col style %d: %w", col, err)
		}
		colStyles[col] = s
	}

	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2) // +2 because header is row 1
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return nil, fmt.Errorf("set %s: %w", cell, err)
			}
			if styleID, ok := colStyles[c]; ok {
				if err := f.SetCellStyle(sheet, cell, cell, styleID); err != nil {
					return nil, fmt.Errorf("style %s: %w", cell, err)
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write workbook: %w", err)
	}
	return buf.Bytes(), nil
}

func filename(prefix string, p Period) string {
	stamp := time.Now().UTC().Format("20060102")
	if p.From != nil {
		stamp = p.From.Format("20060102")
		if p.To != nil {
			stamp += "-" + p.To.Format("20060102")
		}
	}
	return prefix + "-" + stamp + ".xlsx"
}

func (p Period) validate() error {
	if p.From != nil && p.To != nil && p.From.After(*p.To) {
		return domain.NewBizError(domain.ErrInvalidInput, "from must be before to")
	}
	return nil
}

func formatDate(t time.Time) string {
	return t.Format("02/01/2006")
}

func formatDatePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatDate(*t)
}

func boolStr(b bool) string {
	if b {
		return "Có"
	}
	return "Không"
}

// ── ExportCostings ───────────────────────────────────────────────────────────

func (s *service) ExportCostings(ctx context.Context, p Period) (ExportFile, error) {
	if s.costings == nil {
		return ExportFile{}, domain.NewBizError(domain.ErrInvalidInput, "costings reader not configured")
	}
	if err := p.validate(); err != nil {
		return ExportFile{}, err
	}
	rows, err := s.costings.ListCostingsInPeriod(ctx, p)
	if err != nil {
		return ExportFile{}, err
	}
	headers := []string{
		"Mã WO", "Mã SKU", "Tên SKU", "Loại",
		"CP vật tư", "CP phụ trợ", "CP nhân công", "Tổng",
		"Đã chốt", "Ngày tạo",
	}
	data := make([][]any, 0, len(rows))
	for _, r := range rows {
		data = append(data, []any{
			r.WorkOrderID, r.SKUCode, r.SKUName, r.CostingType,
			r.MaterialCost, r.AuxiliaryCost, r.LaborCost, r.TotalCost,
			boolStr(r.Finalized), formatDate(r.CreatedAt),
		})
	}
	bytes, err := build("Costings", headers, data, map[int]string{
		4: moneyFormat, 5: moneyFormat, 6: moneyFormat, 7: moneyFormat,
	})
	if err != nil {
		return ExportFile{}, err
	}
	return ExportFile{Filename: filename("costings", p), Bytes: bytes}, nil
}

// ── ExportPurchaseOrders ─────────────────────────────────────────────────────

func (s *service) ExportPurchaseOrders(ctx context.Context, p Period) (ExportFile, error) {
	if s.pos == nil {
		return ExportFile{}, domain.NewBizError(domain.ErrInvalidInput, "purchase orders reader not configured")
	}
	if err := p.validate(); err != nil {
		return ExportFile{}, err
	}
	rows, err := s.pos.ListPOsInPeriod(ctx, p)
	if err != nil {
		return ExportFile{}, err
	}
	headers := []string{
		"Mã PO", "Nhà cung cấp", "Trạng thái", "Vật tư",
		"Ngày đặt", "Ngày nhận", "Chi tiết item", "Tổng tiền",
		"Ngày tạo",
	}
	data := make([][]any, 0, len(rows))
	for _, r := range rows {
		data = append(data, []any{
			r.Code, r.Supplier, r.Status, r.MaterialName,
			formatDatePtr(r.OrderedAt), formatDatePtr(r.ReceivedAt),
			r.Items, r.TotalCost, formatDate(r.CreatedAt),
		})
	}
	bytes, err := build("PurchaseOrders", headers, data, map[int]string{
		7: moneyFormat,
	})
	if err != nil {
		return ExportFile{}, err
	}
	return ExportFile{Filename: filename("purchase-orders", p), Bytes: bytes}, nil
}

// ── ExportSKUs ───────────────────────────────────────────────────────────────

func (s *service) ExportSKUs(ctx context.Context) (ExportFile, error) {
	if s.skus == nil {
		return ExportFile{}, domain.NewBizError(domain.ErrInvalidInput, "skus reader not configured")
	}
	rows, err := s.skus.ListAllSKUs(ctx)
	if err != nil {
		return ExportFile{}, err
	}
	headers := []string{
		"Mã SKU", "Tên SKU", "Dài (mm)", "Rộng (mm)",
		"Cần kim loại", "Đang dùng", "BOM",
	}
	data := make([][]any, 0, len(rows))
	for _, r := range rows {
		data = append(data, []any{
			r.Code, r.Name, r.LengthMM, r.WidthMM,
			boolStr(r.RequiresMetal), boolStr(r.IsActive), r.BOMSummary,
		})
	}
	bytes, err := build("SKUs", headers, data, map[int]string{
		2: intFormat, 3: intFormat,
	})
	if err != nil {
		return ExportFile{}, err
	}
	return ExportFile{Filename: filename("skus", Period{}), Bytes: bytes}, nil
}

// ── ExportWorkOrders ─────────────────────────────────────────────────────────

func (s *service) ExportWorkOrders(ctx context.Context, p Period) (ExportFile, error) {
	if s.wos == nil {
		return ExportFile{}, domain.NewBizError(domain.ErrInvalidInput, "work orders reader not configured")
	}
	if err := p.validate(); err != nil {
		return ExportFile{}, err
	}
	rows, err := s.wos.ListWorkOrdersInPeriod(ctx, p)
	if err != nil {
		return ExportFile{}, err
	}
	headers := []string{
		"Mã WO", "Mã SKU", "Tên SKU", "Số lượng", "Trạng thái",
		"Ngày giao", "Ngày tạo", "Tổng giá thành",
	}
	data := make([][]any, 0, len(rows))
	for _, r := range rows {
		var totalCost any
		if r.TotalCost != nil {
			totalCost = *r.TotalCost
		} else {
			totalCost = ""
		}
		data = append(data, []any{
			r.ID, r.SKUCode, r.SKUName, r.Quantity, r.Status,
			formatDatePtr(r.AssignedAt), formatDate(r.CreatedAt), totalCost,
		})
	}
	bytes, err := build("WorkOrders", headers, data, map[int]string{
		7: moneyFormat,
	})
	if err != nil {
		return ExportFile{}, err
	}
	return ExportFile{Filename: filename("work-orders", p), Bytes: bytes}, nil
}

// ── ExportWaste ──────────────────────────────────────────────────────────────

func (s *service) ExportWaste(ctx context.Context, p Period) (ExportFile, error) {
	if s.waste == nil {
		return ExportFile{}, domain.NewBizError(domain.ErrInvalidInput, "waste reader not configured")
	}
	if err := p.validate(); err != nil {
		return ExportFile{}, err
	}
	rows, err := s.waste.ListWasteInPeriod(ctx, p)
	if err != nil {
		return ExportFile{}, err
	}
	headers := []string{
		"Vật tư", "Số tấm tiêu thụ", "Diện tích hao (mm²)",
		"Giá TB / tấm", "Tổng chi phí hao",
	}
	data := make([][]any, 0, len(rows))
	for _, r := range rows {
		data = append(data, []any{
			r.MaterialName, r.SheetsConsumed, r.WasteAreaMM2,
			r.AvgSheetCost, r.TotalWasteCost,
		})
	}
	bytes, err := build("Waste", headers, data, map[int]string{
		1: intFormat,
		2: intFormat,
		3: moneyFormat,
		4: moneyFormat,
	})
	if err != nil {
		return ExportFile{}, err
	}
	return ExportFile{Filename: filename("waste", p), Bytes: bytes}, nil
}
