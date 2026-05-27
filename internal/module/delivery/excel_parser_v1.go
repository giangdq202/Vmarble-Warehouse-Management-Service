package delivery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// excelTemplateV1 nails the column layout the planner promised in the issue.
// When the customer's actual template lands, change these constants in one
// place and the rest of the parser keeps working.
//
//	col A  customer_sku_code  (string)
//	col B  qty                (numeric, > 0)
//	col C  unit               (string, non-empty)
//	col D  notes              (string, optional)
//
// Row 1 is the header; data starts from row 2. The first sheet of the workbook
// is read regardless of name — operators routinely rename it.
const (
	excelHeaderRow             = 1
	excelDataStartRow          = 2
	excelColCustomerSKU        = 0 // column A
	excelColQty                = 1 // column B
	excelColUnit               = 2 // column C
	excelColNotes              = 3 // column D
	excelMinExpectedColumns    = 3 // notes is optional
	excelMaxRowsPerLoadingPlan = 10000
)

// parsedExcelRow is the parser's pre-validation snapshot of one Excel row.
// It is not the LoadingPlanLine yet — that DTO requires sku_id which only
// appears after CustomerSKUResolver.ResolveCustomerSKU at the service layer.
type parsedExcelRow struct {
	RowNum          int
	CustomerSKUCode string
	QtyInExcel      float64
	UnitInExcel     string
	Notes           string
	Raw             map[string]any
}

// excelParseOutcome carries the parsed rows, structural row errors, and the
// content hash. Hash is populated even when row errors are present so the
// service can short-circuit BR-D10 (duplicate hash) on a re-upload of the
// same broken file.
type excelParseOutcome struct {
	Rows      []parsedExcelRow
	RowErrors []LoadingPlanRowError
	Hash      string
}

// parseExcelV1 reads the upload Excel and returns one slice of clean rows
// + structural errors. Hash is sha256 of the raw bytes (BR-D10).
//
// On a fundamental error (cannot open file, no sheets, header missing) the
// function returns an error and the outcome holds whatever it could compute
// (Hash always populated when bytes were readable). Per-row errors land in
// outcome.RowErrors so the caller decides whether to bail or accumulate; the
// service layer always bails on first non-empty errors slice (fail-all,
// BR-D08).
func parseExcelV1(r io.Reader) (excelParseOutcome, error) {
	if r == nil {
		return excelParseOutcome{}, errors.New("nil reader")
	}

	// Pull the entire body so we can hash + parse from one buffer. Excel files
	// are typically <500 KB so fully buffering is cheaper than two-pass IO.
	buf, err := io.ReadAll(r)
	if err != nil {
		return excelParseOutcome{}, fmt.Errorf("read excel: %w", err)
	}
	sum := sha256.Sum256(buf)
	hash := hex.EncodeToString(sum[:])

	f, err := excelize.OpenReader(strings.NewReader(string(buf)))
	if err != nil {
		return excelParseOutcome{Hash: hash}, fmt.Errorf("open excel: %w", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return excelParseOutcome{Hash: hash}, errors.New("workbook has no sheets")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return excelParseOutcome{Hash: hash}, fmt.Errorf("read rows: %w", err)
	}
	if len(rows) < excelDataStartRow {
		return excelParseOutcome{Hash: hash}, errors.New("workbook has no data rows; row 1 must be header, row 2+ is data")
	}

	header := rows[excelHeaderRow-1]
	if len(header) < excelMinExpectedColumns {
		return excelParseOutcome{Hash: hash}, fmt.Errorf("header row needs at least %d columns (customer_sku_code, qty, unit), found %d", excelMinExpectedColumns, len(header))
	}

	out := excelParseOutcome{Hash: hash}
	dataRows := rows[excelDataStartRow-1:]
	if len(dataRows) > excelMaxRowsPerLoadingPlan {
		return out, fmt.Errorf("workbook exceeds the %d data row cap", excelMaxRowsPerLoadingPlan)
	}

	for i, row := range dataRows {
		rowNum := excelDataStartRow + i

		// excelize trims trailing empty cells, so a row with only the SKU code
		// returns a 1-element slice. Pad to the expected column count so cell
		// access is safe and a missing required cell is still caught below.
		if len(row) < excelMinExpectedColumns {
			padded := make([]string, excelMinExpectedColumns)
			copy(padded, row)
			row = padded
		}

		if isBlankRow(row) {
			// Skip silent empty rows so trailing whitespace at the bottom of
			// the sheet does not look like an error to the operator.
			continue
		}

		code := strings.TrimSpace(row[excelColCustomerSKU])
		if code == "" {
			out.RowErrors = append(out.RowErrors, LoadingPlanRowError{
				Row:     rowNum,
				Col:     "A",
				Code:    "MISSING_CODE",
				Message: "customer_sku_code is empty",
			})
			continue
		}

		qtyRaw := strings.TrimSpace(row[excelColQty])
		qty, qtyErr := strconv.ParseFloat(qtyRaw, 64)
		if qtyErr != nil || qty <= 0 {
			out.RowErrors = append(out.RowErrors, LoadingPlanRowError{
				Row:     rowNum,
				Col:     "B",
				Code:    "INVALID_QTY",
				Message: "qty must be a positive number, got " + strconv.Quote(qtyRaw),
			})
			continue
		}

		unit := strings.TrimSpace(row[excelColUnit])
		if unit == "" {
			out.RowErrors = append(out.RowErrors, LoadingPlanRowError{
				Row:     rowNum,
				Col:     "C",
				Code:    "MISSING_UNIT",
				Message: "unit is empty",
			})
			continue
		}

		notes := ""
		if len(row) > excelColNotes {
			notes = strings.TrimSpace(row[excelColNotes])
		}

		raw := map[string]any{
			"customer_sku_code": code,
			"qty":               qty,
			"unit":              unit,
		}
		if notes != "" {
			raw["notes"] = notes
		}

		out.Rows = append(out.Rows, parsedExcelRow{
			RowNum:          rowNum,
			CustomerSKUCode: code,
			QtyInExcel:      qty,
			UnitInExcel:     unit,
			Notes:           notes,
			Raw:             raw,
		})
	}

	return out, nil
}

func isBlankRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// rawRowJSON serializes a parsedExcelRow.Raw into the JSONB bytes the
// loading_plan_lines.raw_excel_row column expects. Returns "{}" on any
// marshal failure so the row never blocks insert on a tooling glitch.
func rawRowJSON(raw map[string]any) []byte {
	if raw == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return []byte("{}")
	}
	return b
}
