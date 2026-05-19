package reports

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/xuri/excelize/v2"
)

// service streams Excel workbooks from the reader-projected rows. It owns no
// state — every export is fresh.
type service struct {
	costings CostingReader
	pos      POReader
	skus     SKUReader
	wos      WOReader
	waste    WasteReader
}

// NewService wires the readers. Any reader may be nil; the corresponding
// export will return ErrInvalidInput so misconfiguration surfaces as a 400
// rather than a panic.
func NewService(costings CostingReader, pos POReader, skus SKUReader, wos WOReader, waste WasteReader) Service {
	return &service{costings: costings, pos: pos, skus: skus, wos: wos, waste: waste}
}

const (
	moneyFormat = `#,##0" đ"`
	intFormat   = "#,##0"
)

// streamCtx bundles the per-export workbook state used by streamRows below.
// The struct is unexported and lives only on the stack of one Export* call;
// concurrent exports each get their own.
type streamCtx struct {
	file        *excelize.File
	sw          *excelize.StreamWriter
	sheet       string
	headerStyle int
	colStyles   map[int]int // 0-based column index → excelize style ID
	row         int         // 1-based current row; header lives on row 1
}

// startWorkbook creates an empty workbook with one sheet, registers the
// header + per-column number-format styles, and writes the header row using
// the StreamWriter so subsequent calls can append data rows in order.
func startWorkbook(sheet string, headers []string, colFormats map[int]string) (*streamCtx, error) {
	f := excelize.NewFile()
	// Rename the default sheet so the StreamWriter operates on it directly.
	if sheet != "Sheet1" {
		if err := f.SetSheetName("Sheet1", sheet); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("rename sheet: %w", err)
		}
	}
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("new stream writer: %w", err)
	}
	headerStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("header style: %w", err)
	}
	colStyles := map[int]int{}
	for col, fmtStr := range colFormats {
		s, err := f.NewStyle(&excelize.Style{NumFmt: 0, CustomNumFmt: &fmtStr})
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("col style %d: %w", col, err)
		}
		colStyles[col] = s
	}
	headerCells := make([]any, len(headers))
	for i, h := range headers {
		headerCells[i] = excelize.Cell{StyleID: headerStyle, Value: h}
	}
	if err := sw.SetRow("A1", headerCells); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write header: %w", err)
	}
	return &streamCtx{
		file:        f,
		sw:          sw,
		sheet:       sheet,
		headerStyle: headerStyle,
		colStyles:   colStyles,
		row:         1,
	}, nil
}

// writeRow appends one data row to the StreamWriter, applying any registered
// per-column number-format styles. Cells without a style fall through to the
// excelize default (number / string auto-detection).
func (s *streamCtx) writeRow(values []any) error {
	s.row++
	cells := make([]any, len(values))
	for i, v := range values {
		if styleID, ok := s.colStyles[i]; ok {
			cells[i] = excelize.Cell{StyleID: styleID, Value: v}
		} else {
			cells[i] = v
		}
	}
	axis := fmt.Sprintf("A%d", s.row)
	if err := s.sw.SetRow(axis, cells); err != nil {
		return fmt.Errorf("write row %d: %w", s.row, err)
	}
	return nil
}

// finish flushes the StreamWriter, freezes the header pane, then writes the
// finalized workbook to w. The receiver's File is closed regardless of
// outcome so a partial export does not leak the temp file excelize allocates
// for the stream.
func (s *streamCtx) finish(w io.Writer) error {
	defer func() { _ = s.file.Close() }()
	if err := s.sw.Flush(); err != nil {
		return fmt.Errorf("stream flush: %w", err)
	}
	if err := s.file.SetPanes(s.sheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return fmt.Errorf("freeze pane: %w", err)
	}
	if err := s.file.Write(w); err != nil {
		return fmt.Errorf("write workbook: %w", err)
	}
	return nil
}

// abort releases excelize's temp file when an error occurred mid-stream and
// nothing was sent to the client. No-op if finish already ran.
func (s *streamCtx) abort() {
	_ = s.file.Close()
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

// validate enforces the [from, to) ordering and the MaxPeriodDays span. The
// span check only fires when BOTH bounds are present — an open-ended export
// is permitted if the FE has not chosen a stop date, but the streaming
// pipeline keeps RAM bounded either way.
func (p Period) validate() error {
	if p.From == nil || p.To == nil {
		return nil
	}
	if p.From.After(*p.To) {
		return domain.NewBizError(domain.ErrInvalidInput, "from must be before to")
	}
	span := p.To.Sub(*p.From)
	if span > time.Duration(MaxPeriodDays)*24*time.Hour {
		return domain.NewBizError(domain.ErrInvalidInput,
			fmt.Sprintf("khoảng thời gian tối đa %d ngày, vui lòng thu hẹp lại", MaxPeriodDays))
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

func (s *service) ExportCostings(ctx context.Context, w io.Writer, p Period) (string, error) {
	if s.costings == nil {
		return "", domain.NewBizError(domain.ErrInvalidInput, "costings reader not configured")
	}
	if err := p.validate(); err != nil {
		return "", err
	}
	headers := []string{
		"Mã WO", "Mã SKU", "Tên SKU", "Loại",
		"CP vật tư", "CP phụ trợ", "CP nhân công", "Tổng",
		"Đã chốt", "Ngày tạo",
	}
	sc, err := startWorkbook("Costings", headers, map[int]string{
		4: moneyFormat, 5: moneyFormat, 6: moneyFormat, 7: moneyFormat,
	})
	if err != nil {
		return "", err
	}
	err = s.costings.IterateCostingsInPeriod(ctx, p, func(r CostingRow) error {
		return sc.writeRow([]any{
			r.WorkOrderID, r.SKUCode, r.SKUName, r.CostingType,
			r.MaterialCost, r.AuxiliaryCost, r.LaborCost, r.TotalCost,
			boolStr(r.Finalized), formatDate(r.CreatedAt),
		})
	})
	if err != nil {
		sc.abort()
		return "", err
	}
	if err := sc.finish(w); err != nil {
		return "", err
	}
	return filename("costings", p), nil
}

// ── ExportPurchaseOrders ─────────────────────────────────────────────────────

func (s *service) ExportPurchaseOrders(ctx context.Context, w io.Writer, p Period) (string, error) {
	if s.pos == nil {
		return "", domain.NewBizError(domain.ErrInvalidInput, "purchase orders reader not configured")
	}
	if err := p.validate(); err != nil {
		return "", err
	}
	headers := []string{
		"Mã PO", "Nhà cung cấp", "Trạng thái", "Vật tư",
		"Ngày đặt", "Ngày nhận", "Chi tiết item", "Tổng tiền",
		"Ngày tạo",
	}
	sc, err := startWorkbook("PurchaseOrders", headers, map[int]string{
		7: moneyFormat,
	})
	if err != nil {
		return "", err
	}
	err = s.pos.IteratePOsInPeriod(ctx, p, func(r PORow) error {
		return sc.writeRow([]any{
			r.Code, r.Supplier, r.Status, r.MaterialName,
			formatDatePtr(r.OrderedAt), formatDatePtr(r.ReceivedAt),
			r.Items, r.TotalCost, formatDate(r.CreatedAt),
		})
	})
	if err != nil {
		sc.abort()
		return "", err
	}
	if err := sc.finish(w); err != nil {
		return "", err
	}
	return filename("purchase-orders", p), nil
}

// ── ExportSKUs ───────────────────────────────────────────────────────────────

func (s *service) ExportSKUs(ctx context.Context, w io.Writer) (string, error) {
	if s.skus == nil {
		return "", domain.NewBizError(domain.ErrInvalidInput, "skus reader not configured")
	}
	headers := []string{
		"Mã SKU", "Tên SKU", "Dài (mm)", "Rộng (mm)",
		"Cần kim loại", "Đang dùng", "BOM",
	}
	sc, err := startWorkbook("SKUs", headers, map[int]string{
		2: intFormat, 3: intFormat,
	})
	if err != nil {
		return "", err
	}
	err = s.skus.IterateSKUs(ctx, MaxSKURows, func(r SKURow) error {
		return sc.writeRow([]any{
			r.Code, r.Name, r.LengthMM, r.WidthMM,
			boolStr(r.RequiresMetal), boolStr(r.IsActive), r.BOMSummary,
		})
	})
	if err != nil {
		sc.abort()
		return "", err
	}
	if err := sc.finish(w); err != nil {
		return "", err
	}
	return filename("skus", Period{}), nil
}

// ── ExportWorkOrders ─────────────────────────────────────────────────────────

func (s *service) ExportWorkOrders(ctx context.Context, w io.Writer, p Period) (string, error) {
	if s.wos == nil {
		return "", domain.NewBizError(domain.ErrInvalidInput, "work orders reader not configured")
	}
	if err := p.validate(); err != nil {
		return "", err
	}
	headers := []string{
		"Mã WO", "Mã SKU", "Tên SKU", "Số lượng", "Trạng thái",
		"Ngày giao", "Ngày tạo", "Tổng giá thành",
	}
	sc, err := startWorkbook("WorkOrders", headers, map[int]string{
		7: moneyFormat,
	})
	if err != nil {
		return "", err
	}
	err = s.wos.IterateWorkOrdersInPeriod(ctx, p, func(r WORow) error {
		var totalCost any
		if r.TotalCost != nil {
			totalCost = *r.TotalCost
		} else {
			totalCost = ""
		}
		return sc.writeRow([]any{
			r.ID, r.SKUCode, r.SKUName, r.Quantity, r.Status,
			formatDatePtr(r.AssignedAt), formatDate(r.CreatedAt), totalCost,
		})
	})
	if err != nil {
		sc.abort()
		return "", err
	}
	if err := sc.finish(w); err != nil {
		return "", err
	}
	return filename("work-orders", p), nil
}

// ── ExportWaste ──────────────────────────────────────────────────────────────

func (s *service) ExportWaste(ctx context.Context, w io.Writer, p Period) (string, error) {
	if s.waste == nil {
		return "", domain.NewBizError(domain.ErrInvalidInput, "waste reader not configured")
	}
	if err := p.validate(); err != nil {
		return "", err
	}
	headers := []string{
		"Vật tư", "Số tấm tiêu thụ", "Diện tích hao (mm²)",
		"Giá TB / tấm", "Tổng chi phí hao",
	}
	sc, err := startWorkbook("Waste", headers, map[int]string{
		1: intFormat,
		2: intFormat,
		3: moneyFormat,
		4: moneyFormat,
	})
	if err != nil {
		return "", err
	}
	err = s.waste.IterateWasteInPeriod(ctx, p, func(r WasteRow) error {
		return sc.writeRow([]any{
			r.MaterialName, r.SheetsConsumed, r.WasteAreaMM2,
			r.AvgSheetCost, r.TotalWasteCost,
		})
	})
	if err != nil {
		sc.abort()
		return "", err
	}
	if err := sc.finish(w); err != nil {
		return "", err
	}
	return filename("waste", p), nil
}
