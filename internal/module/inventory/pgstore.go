package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type pgStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) store {
	return &pgStore{pool: pool}
}

func (s *pgStore) insertLot(ctx context.Context, lot InventoryLot) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO inventory_lots (id, material_id, quantity, cost_per_sheet_amount, cost_per_sheet_currency, supplier_ref, is_active, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		lot.ID, lot.MaterialID, lot.Quantity,
		lot.CostPerSheet.Amount, lot.CostPerSheet.Currency,
		lot.SupplierRef, lot.IsActive, lot.ReceivedAt,
	)
	return err
}

func (s *pgStore) selectLots(ctx context.Context) ([]InventoryLot, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, material_id, quantity, cost_per_sheet_amount, cost_per_sheet_currency, supplier_ref, is_active, received_at
		 FROM inventory_lots WHERE is_active = true ORDER BY received_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lots []InventoryLot
	for rows.Next() {
		var l InventoryLot
		if err := rows.Scan(&l.ID, &l.MaterialID, &l.Quantity,
			&l.CostPerSheet.Amount, &l.CostPerSheet.Currency,
			&l.SupplierRef, &l.IsActive, &l.ReceivedAt); err != nil {
			return nil, err
		}
		lots = append(lots, l)
	}
	return lots, rows.Err()
}

// selectLotsPaged returns a page of inventory lots optionally filtered by a
// case-insensitive keyword match on the supplier_ref column.
// It returns (items, totalMatchingItems, error).
func (s *pgStore) selectLotsPaged(ctx context.Context, p httpkit.PageParams) ([]InventoryLot, int, error) {
	search := "%" + p.Search + "%"

	sortCol := "received_at"
	if p.SortBy == "supplier_ref" {
		sortCol = "supplier_ref"
	}
	orderDir := "DESC"
	if p.Order == "asc" {
		orderDir = "ASC"
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM inventory_lots WHERE is_active = true AND supplier_ref ILIKE $1`,
		search,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count inventory_lots: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT id, material_id, quantity, cost_per_sheet_amount, cost_per_sheet_currency, supplier_ref, is_active, received_at
		 FROM inventory_lots
		 WHERE is_active = true AND supplier_ref ILIKE $1
		 ORDER BY %s %s
		 LIMIT $2 OFFSET $3`,
		sortCol, orderDir,
	)
	rows, err := s.pool.Query(ctx, query, search, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var lots []InventoryLot
	for rows.Next() {
		var l InventoryLot
		if err := rows.Scan(&l.ID, &l.MaterialID, &l.Quantity,
			&l.CostPerSheet.Amount, &l.CostPerSheet.Currency,
			&l.SupplierRef, &l.IsActive, &l.ReceivedAt); err != nil {
			return nil, 0, err
		}
		lots = append(lots, l)
	}
	return lots, total, rows.Err()
}

func (s *pgStore) deactivateLot(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE inventory_lots SET is_active = false WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *pgStore) insertSheets(ctx context.Context, sheets []BoardSheet) error {
	batch := &pgx.Batch{}
	for _, sh := range sheets {
		batch.Queue(
			`INSERT INTO board_sheets (id, lot_id, length_mm, width_mm, cost_amount, cost_currency, status,
			                           supplier_code, lot_batch, grain_pattern, quality_grade)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			sh.ID, sh.LotID, sh.Dimensions.LengthMM, sh.Dimensions.WidthMM,
			sh.CostPerSheet.Amount, sh.CostPerSheet.Currency, sh.Status,
			sh.SupplierCode, sh.LotBatch, sh.GrainPattern, sh.QualityGrade,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	for range sheets {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// sheetCols is the shared SELECT projection for all BoardSheet queries.
// Joins inventory_lots and materials to expose material_id + material_name directly on the DTO.
const sheetCols = `
	bs.id, bs.lot_id,
	COALESCE(il.material_id, '00000000-0000-0000-0000-000000000000'::uuid) AS material_id,
	COALESCE(m.name, '')                                                   AS material_name,
	bs.length_mm, bs.width_mm, bs.cost_amount, bs.cost_currency,
	bs.status, bs.issued_to_wo_id,
	bs.supplier_code, bs.lot_batch, bs.grain_pattern, bs.quality_grade,
	bs.bin_location_id
FROM board_sheets bs
LEFT JOIN inventory_lots il ON il.id = bs.lot_id
LEFT JOIN materials m ON m.id = il.material_id`

func scanSheet(row interface{ Scan(...any) error }) (BoardSheet, error) {
	var sh BoardSheet
	var binLocationID uuid.NullUUID
	err := row.Scan(
		&sh.ID, &sh.LotID, &sh.MaterialID, &sh.MaterialName,
		&sh.Dimensions.LengthMM, &sh.Dimensions.WidthMM,
		&sh.CostPerSheet.Amount, &sh.CostPerSheet.Currency,
		&sh.Status, &sh.IssuedToWorkOrderID,
		&sh.SupplierCode, &sh.LotBatch, &sh.GrainPattern, &sh.QualityGrade,
		&binLocationID,
	)
	if binLocationID.Valid {
		v := binLocationID.UUID
		sh.BinLocationID = &v
	}
	return sh, err
}

func (s *pgStore) selectSheetByID(ctx context.Context, id uuid.UUID) (BoardSheet, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+sheetCols+` WHERE bs.id = $1`, id)
	sh, err := scanSheet(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return BoardSheet{}, domain.NewBizError(domain.ErrNotFound, "board sheet not found")
	}
	return sh, err
}

func (s *pgStore) selectAvailableSheets(ctx context.Context) ([]BoardSheet, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+sheetCols+` WHERE bs.status = 'AVAILABLE' ORDER BY bs.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sheets []BoardSheet
	for rows.Next() {
		sh, err := scanSheet(rows)
		if err != nil {
			return nil, err
		}
		sheets = append(sheets, sh)
	}
	return sheets, rows.Err()
}

// selectAvailableSheetsPaged returns a page of AVAILABLE board sheets,
// with pagination support. Board sheets have no freeform text fields, so
// search filters on the lot_id (exact UUID prefix match is not useful in
// practice, but the field is provided for API consistency — an empty search
// returns all available sheets).
// It returns (items, totalMatchingItems, error).
func (s *pgStore) selectAvailableSheetsPaged(ctx context.Context, p httpkit.PageParams, materialID *uuid.UUID) ([]BoardSheet, int, error) {
	sortCol := "bs.id"
	if p.SortBy == "length_mm" || p.SortBy == "width_mm" {
		sortCol = "bs." + p.SortBy
	}
	orderDir := "ASC"
	if p.Order == "desc" {
		orderDir = "DESC"
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM board_sheets bs
		 LEFT JOIN inventory_lots il ON il.id = bs.lot_id
		 WHERE bs.status = 'AVAILABLE'
		   AND ($1::uuid IS NULL OR il.material_id = $1)`,
		materialID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count board_sheets: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT `+sheetCols+`
		 WHERE bs.status = 'AVAILABLE'
		   AND ($1::uuid IS NULL OR il.material_id = $1)
		 ORDER BY %s %s
		 LIMIT $2 OFFSET $3`,
		sortCol, orderDir,
	)
	rows, err := s.pool.Query(ctx, query, materialID, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sheets []BoardSheet
	for rows.Next() {
		sh, err := scanSheet(rows)
		if err != nil {
			return nil, 0, err
		}
		sheets = append(sheets, sh)
	}
	return sheets, total, rows.Err()
}

// countAvailableSheetsByMaterial returns the number of AVAILABLE board sheets
// whose owning lot's material_id matches the given id. Used by the production
// module's BR-K01 aggregate stock check.
func (s *pgStore) countAvailableSheetsByMaterial(ctx context.Context, materialID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM board_sheets bs
		 JOIN inventory_lots il ON il.id = bs.lot_id
		 WHERE bs.status = 'AVAILABLE'
		   AND il.material_id = $1`,
		materialID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count available board_sheets: %w", err)
	}
	return n, nil
}

func (s *pgStore) updateSheetStatus(ctx context.Context, id uuid.UUID, status string, issuedToWO *uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE board_sheets SET status = $1, issued_to_wo_id = $2 WHERE id = $3`,
		status, issuedToWO, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "board sheet not found")
	}
	return nil
}

func (s *pgStore) selectOverflowAreas(ctx context.Context) (int64, int64, error) {
	var totalRemnantAreaMM2 int64
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(CAST(length_mm AS bigint) * CAST(width_mm AS bigint)), 0)
		 FROM remnants
		 WHERE status IN ('AVAILABLE', 'ALLOCATED')`,
	).Scan(&totalRemnantAreaMM2)
	if err != nil {
		return 0, 0, err
	}

	var totalSheetAreaMM2 int64
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(CAST(length_mm AS bigint) * CAST(width_mm AS bigint)), 0)
		 FROM board_sheets
		 WHERE status = 'AVAILABLE'`,
	).Scan(&totalSheetAreaMM2)
	if err != nil {
		return 0, 0, err
	}

	return totalRemnantAreaMM2, totalSheetAreaMM2, nil
}

func (s *pgStore) preAssignSheet(ctx context.Context, sheetID uuid.UUID, workOrderID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var status string
	err = tx.QueryRow(ctx,
		`SELECT status FROM board_sheets WHERE id = $1 FOR UPDATE`,
		sheetID,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.NewBizError(domain.ErrNotFound, "board sheet not found")
		}
		return err
	}
	if status != "AVAILABLE" {
		return domain.NewBizError(domain.ErrPreconditionFailed,
			"sheet must be AVAILABLE to pre-assign, got "+status)
	}

	_, err = tx.Exec(ctx,
		`UPDATE board_sheets SET issued_to_wo_id = $1 WHERE id = $2`,
		workOrderID, sheetID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *pgStore) insertCuttingRecord(ctx context.Context, cr CuttingRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO cutting_records (id, sheet_id, remnant_source_id, work_order_id, sku_id, used_length_mm, used_width_mm, produced_remnant_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		cr.ID, cr.SheetID, cr.RemnantSourceID,
		cr.WorkOrderID, cr.SKUID,
		cr.UsedLengthMM, cr.UsedWidthMM, cr.ProducedRemnantID, cr.CreatedAt,
	)
	return err
}

// selectCuttingRecordDetails fetches a cutting record plus the SKU code/name
// (joined from skus table — same database, read-only access) and the
// produced remnant if any.
func (s *pgStore) selectCuttingRecordDetails(ctx context.Context, id uuid.UUID) (CuttingRecordDetails, error) {
	var d CuttingRecordDetails
	row := s.pool.QueryRow(ctx,
		`SELECT cr.id, cr.sheet_id, cr.remnant_source_id, cr.work_order_id,
		        cr.sku_id, cr.used_length_mm, cr.used_width_mm,
		        cr.produced_remnant_id, cr.created_at,
		        COALESCE(s.code, ''), COALESCE(s.name, '')
		   FROM cutting_records cr
		   LEFT JOIN skus s ON s.id = cr.sku_id
		  WHERE cr.id = $1`,
		id,
	)
	if err := row.Scan(
		&d.Record.ID, &d.Record.SheetID, &d.Record.RemnantSourceID, &d.Record.WorkOrderID,
		&d.Record.SKUID, &d.Record.UsedLengthMM, &d.Record.UsedWidthMM,
		&d.Record.ProducedRemnantID, &d.Record.CreatedAt,
		&d.SKUCode, &d.SKUName,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CuttingRecordDetails{}, domain.NewBizError(domain.ErrNotFound, "cutting record not found")
		}
		return CuttingRecordDetails{}, err
	}
	if d.Record.ProducedRemnantID != nil {
		r, err := s.selectRemnantByID(ctx, *d.Record.ProducedRemnantID)
		if err == nil {
			d.ProducedRemnant = &r
		} else if !errors.Is(err, domain.ErrNotFound) {
			return CuttingRecordDetails{}, err
		}
	}
	return d, nil
}

// selectCuttingRecordsReportKeyset returns cutting records strictly before
// the (created_at, id) cursor, enriched with SKU + assignee data, ordered by
// created_at DESC, id DESC. Filters are all optional. Callers pass limit+1
// so the service layer can detect has_more without a follow-up query.
func (s *pgStore) selectCuttingRecordsReportKeyset(ctx context.Context, f CuttingRecordFilter, cur httpkit.Cursor, limit int) ([]CuttingRecordReport, error) {
	var (
		userID   any = nil
		woID     any = nil
		fromTime any = nil
		toTime   any = nil
		curTs    any = nil
		curID    any = nil
	)
	if f.UserID != nil {
		userID = *f.UserID
	}
	if f.WorkOrderID != nil {
		woID = *f.WorkOrderID
	}
	if !f.From.IsZero() {
		fromTime = f.From
	}
	if !f.To.IsZero() {
		toTime = f.To
	}
	if !cur.IsZero() {
		curTs = cur.Ts
		curID = cur.ID
	}

	// Single SQL shape with NULL-guarded predicates so the planner can pick
	// idx_cutting_records_created_at_id whether or not a cursor is present.
	rows, err := s.pool.Query(ctx,
		`SELECT cr.id, cr.work_order_id, cr.sku_id,
		        COALESCE(sk.code, ''), COALESCE(sk.name, ''),
		        cr.sheet_id, cr.remnant_source_id,
		        cr.used_length_mm, cr.used_width_mm,
		        cr.produced_remnant_id,
		        wo.assigned_to,
		        u.username, u.full_name,
		        cr.created_at
		   FROM cutting_records cr
		   LEFT JOIN work_orders wo ON wo.id = cr.work_order_id
		   LEFT JOIN skus sk        ON sk.id = cr.sku_id
		   LEFT JOIN users u        ON u.id = wo.assigned_to
		  WHERE ($1::uuid IS NULL OR wo.assigned_to = $1)
		    AND ($2::uuid IS NULL OR cr.work_order_id = $2)
		    AND ($3::timestamptz IS NULL OR cr.created_at >= $3)
		    AND ($4::timestamptz IS NULL OR cr.created_at <= $4)
		    AND ($5::timestamptz IS NULL OR (cr.created_at, cr.id) < ($5, $6))
		  ORDER BY cr.created_at DESC, cr.id DESC
		  LIMIT $7`,
		userID, woID, fromTime, toTime, curTs, curID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]CuttingRecordReport, 0, limit)
	for rows.Next() {
		var (
			r           CuttingRecordReport
			sheetID     *uuid.UUID
			remnantSrc  *uuid.UUID
			username    sql.NullString
			fullName    sql.NullString
			usedLenMM   int
			usedWidthMM int
		)
		if err := rows.Scan(
			&r.ID, &r.WorkOrderID, &r.SKUID,
			&r.SKUCode, &r.SKUName,
			&sheetID, &remnantSrc,
			&usedLenMM, &usedWidthMM,
			&r.ProducedRemnantID,
			&r.AssignedTo,
			&username, &fullName,
			&r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.UsedDimension = domain.Dimension{LengthMM: usedLenMM, WidthMM: usedWidthMM}
		switch {
		case sheetID != nil:
			r.SourceType = "SHEET"
			r.SourceID = *sheetID
		case remnantSrc != nil:
			r.SourceType = "REMNANT"
			r.SourceID = *remnantSrc
		}
		if username.Valid {
			v := username.String
			r.AssignedUsername = &v
		}
		if fullName.Valid && fullName.String != "" {
			v := fullName.String
			r.AssignedFullName = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) insertRemnant(ctx context.Context, r Remnant) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO remnants (
			id, parent_board_id, parent_remnant_id, length_mm, width_mm,
			status, shape_type, allocated_to_wo_id, supplier_code, lot_batch, grain_pattern,
			quality_grade, bounding_box_length_mm, bounding_box_width_mm, bin_location_id, created_at
		)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		r.ID, r.ParentBoardID, r.ParentRemnantID,
		r.Dimensions.LengthMM, r.Dimensions.WidthMM,
		string(r.Status), r.ShapeType, r.AllocatedToWO,
		r.SupplierCode, r.LotBatch, r.GrainPattern, r.QualityGrade,
		r.BoundingBoxLengthMM, r.BoundingBoxWidthMM, r.BinLocationID, r.CreatedAt,
	)
	return err
}

func (s *pgStore) selectAvailableRemnantsByMinDimension(ctx context.Context, minDim domain.Dimension) ([]Remnant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, shape_type, allocated_to_wo_id, allocated_at,
		        supplier_code, lot_batch, grain_pattern, quality_grade,
		        bounding_box_length_mm, bounding_box_width_mm, bin_location_id, created_at
		 FROM remnants
		 WHERE status = 'AVAILABLE'
		   AND COALESCE(bounding_box_length_mm, length_mm) >= $1
		   AND COALESCE(bounding_box_width_mm, width_mm) >= $2
		 ORDER BY (COALESCE(bounding_box_length_mm, length_mm) * COALESCE(bounding_box_width_mm, width_mm)) ASC,
		          created_at ASC`,
		minDim.LengthMM, minDim.WidthMM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRemnants(rows)
}

// selectTopRemnantSuggestions returns up to `limit` AVAILABLE remnants that
// fit `minDim`, ranked by Best Fit (smallest bounding-box area) + FIFO
// (oldest created_at). Each row is LEFT JOINed with storage_locations so the
// caller gets the shelf position without a second round trip.
func (s *pgStore) selectTopRemnantSuggestions(ctx context.Context, minDim domain.Dimension, limit int) ([]RemnantSuggestion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT
			r.id, r.parent_board_id, r.parent_remnant_id,
			r.length_mm, r.width_mm, r.status, r.shape_type, r.allocated_to_wo_id, r.allocated_at,
			r.supplier_code, r.lot_batch, r.grain_pattern, r.quality_grade,
			r.bounding_box_length_mm, r.bounding_box_width_mm, r.bin_location_id, r.created_at,
			sl.id, sl.zone, sl.rack, sl.shelf, sl.label, sl.barcode, sl.is_active, sl.created_at
		 FROM remnants r
		 LEFT JOIN storage_locations sl ON sl.id = r.bin_location_id AND sl.is_active = TRUE
		 WHERE r.status = 'AVAILABLE'
		   AND COALESCE(r.bounding_box_length_mm, r.length_mm) >= $1
		   AND COALESCE(r.bounding_box_width_mm, r.width_mm) >= $2
		 ORDER BY
			(COALESCE(r.bounding_box_length_mm, r.length_mm) * COALESCE(r.bounding_box_width_mm, r.width_mm)) ASC,
			r.created_at ASC
		 LIMIT $3`,
		minDim.LengthMM, minDim.WidthMM, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RemnantSuggestion
	rank := 1
	for rows.Next() {
		var r Remnant
		var loc StorageLocation
		var locID uuid.NullUUID
		var locZone, locRack, locShelf, locLabel, locBarcode sql.NullString
		var locIsActive sql.NullBool
		var locCreatedAt sql.NullTime
		var allocatedAt sql.NullTime

		var supplierCode, lotBatch, grainPattern, qualityGrade sql.NullString
		var bbLengthMM, bbWidthMM sql.NullInt32
		var binLocationID uuid.NullUUID

		if err := rows.Scan(
			&r.ID, &r.ParentBoardID, &r.ParentRemnantID,
			&r.Dimensions.LengthMM, &r.Dimensions.WidthMM,
			&r.Status, &r.ShapeType, &r.AllocatedToWO, &allocatedAt,
			&supplierCode, &lotBatch, &grainPattern, &qualityGrade,
			&bbLengthMM, &bbWidthMM, &binLocationID, &r.CreatedAt,
			&locID, &locZone, &locRack, &locShelf, &locLabel, &locBarcode, &locIsActive, &locCreatedAt,
		); err != nil {
			return nil, err
		}

		// Map nullable remnant fields.
		r.SupplierCode = nullStringPtr(supplierCode)
		r.LotBatch = nullStringPtr(lotBatch)
		r.GrainPattern = nullStringPtr(grainPattern)
		r.QualityGrade = nullStringPtr(qualityGrade)
		r.BoundingBoxLengthMM = nullInt32Ptr(bbLengthMM)
		r.BoundingBoxWidthMM = nullInt32Ptr(bbWidthMM)
		if binLocationID.Valid {
			v := binLocationID.UUID
			r.BinLocationID = &v
		}
		if allocatedAt.Valid {
			t := allocatedAt.Time
			r.AllocatedAt = &t
		}

		sug := RemnantSuggestion{Remnant: r, Rank: rank}

		// Map nullable location fields (LEFT JOIN may produce NULLs).
		if locID.Valid {
			loc.ID = locID.UUID
			if locZone.Valid {
				loc.Zone = locZone.String
			}
			if locRack.Valid {
				loc.Rack = locRack.String
			}
			if locShelf.Valid {
				loc.Shelf = locShelf.String
			}
			if locLabel.Valid {
				loc.Label = locLabel.String
			}
			if locBarcode.Valid {
				loc.Barcode = locBarcode.String
			}
			if locIsActive.Valid {
				loc.IsActive = locIsActive.Bool
			}
			if locCreatedAt.Valid {
				loc.CreatedAt = locCreatedAt.Time
			}
			sug.Location = &loc
		}

		out = append(out, sug)
		rank++
	}
	return out, rows.Err()
}

// selectRemnantsByFilter returns a paginated slice of remnants that match f,
// plus the total count of matching rows (for pagination metadata).
// Status defaults to AVAILABLE when f.Status is empty.
// Dimension filters use COALESCE(bounding_box_*, *_mm) so that remnants
// created without an explicit bounding box are still matched correctly.
func (s *pgStore) selectRemnantsByFilter(ctx context.Context, f RemnantFilter, p httpkit.PageParams) ([]Remnant, int, error) {
	status := f.Status
	if status == "" {
		status = domain.RemnantAvailable
	}

	// Default sort: FIFO (oldest stock first, BR-K02). FE can override with ?order=desc.
	orderDir := "ASC"
	if p.Order == "desc" {
		orderDir = "DESC"
	}

	const baseWhere = `
	WHERE status = $1
	  AND ($2 = 0 OR COALESCE(bounding_box_length_mm, length_mm) >= $2)
	  AND ($3 = 0 OR COALESCE(bounding_box_width_mm,  width_mm)  >= $3)`

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM remnants`+baseWhere,
		string(status), f.MinLengthMM, f.MinWidthMM,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count remnants by filter: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, shape_type, allocated_to_wo_id, allocated_at,
		        supplier_code, lot_batch, grain_pattern, quality_grade,
		        bounding_box_length_mm, bounding_box_width_mm, bin_location_id, created_at
		 FROM remnants`+baseWhere+`
		 ORDER BY created_at %s, (COALESCE(bounding_box_length_mm, length_mm) * COALESCE(bounding_box_width_mm, width_mm)) ASC
		 LIMIT $4 OFFSET $5`,
		orderDir,
	)
	rows, err := s.pool.Query(ctx, query,
		string(status), f.MinLengthMM, f.MinWidthMM, p.Limit, p.Offset(),
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items, err := scanRemnants(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *pgStore) selectRemnantsByBoardSheet(ctx context.Context, boardSheetID uuid.UUID) ([]Remnant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, shape_type, allocated_to_wo_id, allocated_at,
		        supplier_code, lot_batch, grain_pattern, quality_grade,
		        bounding_box_length_mm, bounding_box_width_mm, bin_location_id, created_at
		 FROM remnants WHERE parent_board_id = $1 ORDER BY created_at`, boardSheetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRemnants(rows)
}

func (s *pgStore) selectRemnantByID(ctx context.Context, id uuid.UUID) (Remnant, error) {
	var r Remnant
	row := s.pool.QueryRow(ctx,
		`SELECT id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, shape_type, allocated_to_wo_id, allocated_at,
		        supplier_code, lot_batch, grain_pattern, quality_grade,
		        bounding_box_length_mm, bounding_box_width_mm, bin_location_id, created_at
		 FROM remnants WHERE id = $1`, id)
	err := scanRemnantRecord(row, &r)
	if errors.Is(err, pgx.ErrNoRows) {
		return Remnant{}, domain.NewBizError(domain.ErrNotFound, "remnant not found")
	}
	return r, err
}

func (s *pgStore) updateRemnantStatus(ctx context.Context, id uuid.UUID, status domain.RemnantStatus, allocatedToWO *uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE remnants SET status = $1, allocated_to_wo_id = $2 WHERE id = $3`,
		string(status), allocatedToWO, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "remnant not found")
	}
	return nil
}

func (s *pgStore) recordCutAtomically(ctx context.Context, op cutWriteOp) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// 1. Lock the source row and re-validate its status inside the transaction.
	//    SELECT … FOR UPDATE blocks any concurrent writer that holds or tries to
	//    acquire a lock on the same row, eliminating the TOCTOU window between
	//    the service-layer read and this write.
	if op.SheetUpdate != nil {
		var lockedStatus string
		lockErr := tx.QueryRow(ctx,
			`SELECT status FROM board_sheets WHERE id = $1 FOR UPDATE`,
			op.SheetUpdate.ID).Scan(&lockedStatus)
		if errors.Is(lockErr, pgx.ErrNoRows) {
			err = domain.NewBizError(domain.ErrNotFound, "board sheet not found")
			return err
		}
		if lockErr != nil {
			err = fmt.Errorf("lock board sheet: %w", lockErr)
			return err
		}
		if lockedStatus != "AVAILABLE" {
			err = domain.NewBizError(domain.ErrPreconditionFailed, "board sheet is no longer available")
			return err
		}

		tag, execErr := tx.Exec(ctx,
			`UPDATE board_sheets SET status = $1, issued_to_wo_id = $2 WHERE id = $3`,
			op.SheetUpdate.Status, op.SheetUpdate.IssuedToWO, op.SheetUpdate.ID)
		if execErr != nil {
			err = fmt.Errorf("update sheet status: %w", execErr)
			return err
		}
		if tag.RowsAffected() == 0 {
			err = domain.NewBizError(domain.ErrNotFound, "board sheet not found")
			return err
		}
	} else if op.RemnantUpdate != nil {
		var lockedStatus string
		lockErr := tx.QueryRow(ctx,
			`SELECT status FROM remnants WHERE id = $1 FOR UPDATE`,
			op.RemnantUpdate.ID).Scan(&lockedStatus)
		if errors.Is(lockErr, pgx.ErrNoRows) {
			err = domain.NewBizError(domain.ErrNotFound, "remnant not found")
			return err
		}
		if lockErr != nil {
			err = fmt.Errorf("lock remnant: %w", lockErr)
			return err
		}
		if domain.RemnantStatus(lockedStatus) != domain.RemnantAvailable &&
			domain.RemnantStatus(lockedStatus) != domain.RemnantAllocated {
			err = domain.NewBizError(domain.ErrPreconditionFailed, "remnant is no longer available")
			return err
		}

		tag, execErr := tx.Exec(ctx,
			`UPDATE remnants SET status = $1, allocated_to_wo_id = NULL WHERE id = $2`,
			string(op.RemnantUpdate.Status), op.RemnantUpdate.ID)
		if execErr != nil {
			err = fmt.Errorf("update remnant status: %w", execErr)
			return err
		}
		if tag.RowsAffected() == 0 {
			err = domain.NewBizError(domain.ErrNotFound, "remnant not found")
			return err
		}
	}

	// 2. Insert cutting record.
	cr := op.Record
	if _, execErr := tx.Exec(ctx,
		`INSERT INTO cutting_records (id, sheet_id, remnant_source_id, work_order_id, sku_id, used_length_mm, used_width_mm, produced_remnant_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		cr.ID, cr.SheetID, cr.RemnantSourceID,
		cr.WorkOrderID, cr.SKUID,
		cr.UsedLengthMM, cr.UsedWidthMM, cr.ProducedRemnantID, cr.CreatedAt,
	); execErr != nil {
		err = fmt.Errorf("insert cutting record: %w", execErr)
		return err
	}

	// 3. Insert new remnant if the cut produced leftover material.
	if op.NewRemnant != nil {
		r := op.NewRemnant
		if _, execErr := tx.Exec(ctx,
			`INSERT INTO remnants (
				id, parent_board_id, parent_remnant_id, length_mm, width_mm,
				status, shape_type, allocated_to_wo_id, supplier_code, lot_batch, grain_pattern,
				quality_grade, bounding_box_length_mm, bounding_box_width_mm, bin_location_id, created_at
			)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			r.ID, r.ParentBoardID, r.ParentRemnantID,
			r.Dimensions.LengthMM, r.Dimensions.WidthMM,
			string(r.Status), r.ShapeType, r.AllocatedToWO,
			r.SupplierCode, r.LotBatch, r.GrainPattern, r.QualityGrade,
			r.BoundingBoxLengthMM, r.BoundingBoxWidthMM, r.BinLocationID, r.CreatedAt,
		); execErr != nil {
			err = fmt.Errorf("insert remnant: %w", execErr)
			return err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// allocateRemnantAtomically locks the remnant row, confirms it is still
// AVAILABLE, then transitions it to ALLOCATED in a single transaction.
// Concurrent callers that lose the lock race receive ErrPreconditionFailed.
func (s *pgStore) allocateRemnantAtomically(ctx context.Context, remnantID uuid.UUID, workOrderID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var lockedStatus string
	lockErr := tx.QueryRow(ctx,
		`SELECT status FROM remnants WHERE id = $1 FOR UPDATE`,
		remnantID).Scan(&lockedStatus)
	if errors.Is(lockErr, pgx.ErrNoRows) {
		err = domain.NewBizError(domain.ErrNotFound, "remnant not found")
		return err
	}
	if lockErr != nil {
		err = fmt.Errorf("lock remnant: %w", lockErr)
		return err
	}
	if domain.RemnantStatus(lockedStatus) != domain.RemnantAvailable {
		err = domain.NewBizError(domain.ErrPreconditionFailed, "remnant is no longer available for allocation")
		return err
	}

	tag, execErr := tx.Exec(ctx,
		`UPDATE remnants SET status = $1, allocated_to_wo_id = $2, allocated_at = NOW() WHERE id = $3`,
		string(domain.RemnantAllocated), workOrderID, remnantID)
	if execErr != nil {
		err = fmt.Errorf("update remnant status: %w", execErr)
		return err
	}
	if tag.RowsAffected() == 0 {
		err = domain.NewBizError(domain.ErrNotFound, "remnant not found")
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// markRemnantWasteAtomically locks the remnant row, confirms it is in a
// wasteable state (AVAILABLE or ALLOCATED), then transitions it to WASTE in a
// single transaction. Concurrent callers that lose the lock race receive
// ErrPreconditionFailed.
func (s *pgStore) markRemnantWasteAtomically(ctx context.Context, remnantID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var lockedStatus string
	lockErr := tx.QueryRow(ctx,
		`SELECT status FROM remnants WHERE id = $1 FOR UPDATE`,
		remnantID).Scan(&lockedStatus)
	if errors.Is(lockErr, pgx.ErrNoRows) {
		err = domain.NewBizError(domain.ErrNotFound, "remnant not found")
		return err
	}
	if lockErr != nil {
		err = fmt.Errorf("lock remnant: %w", lockErr)
		return err
	}

	ls := domain.RemnantStatus(lockedStatus)
	if ls != domain.RemnantAvailable && ls != domain.RemnantAllocated {
		err = domain.NewBizError(domain.ErrPreconditionFailed, "remnant cannot be marked waste in its current state")
		return err
	}

	tag, execErr := tx.Exec(ctx,
		`UPDATE remnants SET status = $1, allocated_to_wo_id = NULL WHERE id = $2`,
		string(domain.RemnantWaste), remnantID)
	if execErr != nil {
		err = fmt.Errorf("update remnant status: %w", execErr)
		return err
	}
	if tag.RowsAffected() == 0 {
		err = domain.NewBizError(domain.ErrNotFound, "remnant not found")
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// releaseExpiredAllocations resets ALLOCATED remnants whose allocated_at is
// older than `before` back to AVAILABLE. This is the backing operation for the
// background auto-release task. A plain UPDATE (no transaction, no FOR UPDATE)
// is intentional: each row is updated atomically by PostgreSQL, and concurrent
// executions on multiple server instances are safe because only one UPDATE can
// match a given row's predicate at a time. allocated_to_wo_id and allocated_at
// are both cleared on release.
func (s *pgStore) releaseExpiredAllocations(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE remnants
		 SET status = $1, allocated_to_wo_id = NULL, allocated_at = NULL
		 WHERE status = $2 AND allocated_at IS NOT NULL AND allocated_at < $3`,
		string(domain.RemnantAvailable), string(domain.RemnantAllocated), before,
	)
	if err != nil {
		return 0, fmt.Errorf("release expired allocations: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *pgStore) selectActiveStorageLocations(ctx context.Context) ([]StorageLocation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, zone, rack, shelf, label, barcode, is_active, created_at
		 FROM storage_locations
		 WHERE is_active = TRUE
		 ORDER BY zone, rack, shelf`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locs []StorageLocation
	for rows.Next() {
		var l StorageLocation
		if err := rows.Scan(&l.ID, &l.Zone, &l.Rack, &l.Shelf,
			&l.Label, &l.Barcode, &l.IsActive, &l.CreatedAt); err != nil {
			return nil, err
		}
		locs = append(locs, l)
	}
	return locs, rows.Err()
}

func scanRemnants(rows pgx.Rows) ([]Remnant, error) {
	var remnants []Remnant
	for rows.Next() {
		var r Remnant
		if err := scanRemnantRecord(rows, &r); err != nil {
			return nil, err
		}
		remnants = append(remnants, r)
	}
	return remnants, rows.Err()
}

func (s *pgStore) updateRemnantBinLocation(ctx context.Context, remnantID uuid.UUID, locationID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE remnants SET bin_location_id = $1 WHERE id = $2`,
		locationID, remnantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "remnant not found")
	}
	return nil
}

func (s *pgStore) selectStorageLocationByBarcode(ctx context.Context, barcode string) (StorageLocation, error) {
	var l StorageLocation
	err := s.pool.QueryRow(ctx,
		`SELECT id, zone, rack, shelf, label, barcode, is_active, created_at
		 FROM storage_locations
		 WHERE barcode = $1 AND is_active = TRUE`, barcode).
		Scan(&l.ID, &l.Zone, &l.Rack, &l.Shelf, &l.Label, &l.Barcode, &l.IsActive, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StorageLocation{}, domain.NewBizError(domain.ErrNotFound, "storage location not found")
	}
	return l, err
}

type remnantScanner interface {
	Scan(dest ...any) error
}

func scanRemnantRecord(scanner remnantScanner, r *Remnant) error {
	var supplierCode sql.NullString
	var lotBatch sql.NullString
	var grainPattern sql.NullString
	var qualityGrade sql.NullString
	var boundingBoxLengthMM sql.NullInt32
	var boundingBoxWidthMM sql.NullInt32
	var binLocationID uuid.NullUUID
	var allocatedAt sql.NullTime
	if err := scanner.Scan(
		&r.ID, &r.ParentBoardID, &r.ParentRemnantID,
		&r.Dimensions.LengthMM, &r.Dimensions.WidthMM,
		&r.Status, &r.ShapeType, &r.AllocatedToWO, &allocatedAt,
		&supplierCode, &lotBatch, &grainPattern, &qualityGrade,
		&boundingBoxLengthMM, &boundingBoxWidthMM, &binLocationID,
		&r.CreatedAt,
	); err != nil {
		return err
	}
	r.SupplierCode = nullStringPtr(supplierCode)
	r.LotBatch = nullStringPtr(lotBatch)
	r.GrainPattern = nullStringPtr(grainPattern)
	r.QualityGrade = nullStringPtr(qualityGrade)
	r.BoundingBoxLengthMM = nullInt32Ptr(boundingBoxLengthMM)
	r.BoundingBoxWidthMM = nullInt32Ptr(boundingBoxWidthMM)
	if binLocationID.Valid {
		v := binLocationID.UUID
		r.BinLocationID = &v
	} else {
		r.BinLocationID = nil
	}
	if allocatedAt.Valid {
		t := allocatedAt.Time
		r.AllocatedAt = &t
	} else {
		r.AllocatedAt = nil
	}
	return nil
}

func nullStringPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func (s *pgStore) selectAllocatedRemnantsByWO(ctx context.Context, workOrderID uuid.UUID) ([]PickSlipLine, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT
			r.id,
			r.length_mm, r.width_mm,
			COALESCE(sl.zone,  '')    AS zone,
			COALESCE(sl.rack,  '')    AS rack,
			COALESCE(sl.shelf, '')    AS shelf,
			COALESCE(sl.label, '')    AS label,
			COALESCE(sl.barcode, '')  AS bin_barcode
		 FROM remnants r
		 LEFT JOIN storage_locations sl ON sl.id = r.bin_location_id AND sl.is_active = TRUE
		 WHERE r.allocated_to_wo_id = $1
		   AND r.status = 'ALLOCATED'
		 ORDER BY
			COALESCE(sl.zone,  '') ASC,
			COALESCE(sl.rack,  '') ASC,
			COALESCE(sl.shelf, '') ASC,
			r.created_at ASC`,
		workOrderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PickSlipLine
	for rows.Next() {
		var line PickSlipLine
		if err := rows.Scan(
			&line.RemnantID,
			&line.Dimensions.LengthMM, &line.Dimensions.WidthMM,
			&line.Zone, &line.Rack, &line.Shelf, &line.Label, &line.BinBarcode,
		); err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

func nullInt32Ptr(v sql.NullInt32) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}

func (s *pgStore) updateSheetBinLocation(ctx context.Context, sheetID uuid.UUID, locationID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE board_sheets SET bin_location_id = $1 WHERE id = $2`,
		locationID, sheetID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "board sheet not found")
	}
	return nil
}

func (s *pgStore) insertAuditLog(ctx context.Context, entry AuditLogEntry) error {
	var meta any
	if len(entry.Metadata) > 0 {
		meta = []byte(entry.Metadata)
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO inventory_audit_log
		    (id, entity_type, entity_id, action, actor_id, from_location, to_location,
		     from_status, to_status, reason, session_id, metadata, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		entry.ID, entry.EntityType, entry.EntityID, entry.Action, entry.ActorID,
		entry.FromLocation, entry.ToLocation,
		entry.FromStatus, entry.ToStatus,
		entry.Reason, entry.SessionID, meta, entry.CreatedAt,
	)
	return err
}

func (s *pgStore) selectAuditLogByEntityKeyset(ctx context.Context, entityID uuid.UUID, entityType string, cur httpkit.Cursor, limit int) ([]AuditLogEntry, error) {
	const cols = `id, entity_type, entity_id, action, actor_id,
	              from_location, to_location, from_status, to_status, reason, session_id, metadata, created_at`

	var (
		rows pgx.Rows
		err  error
	)
	if cur.IsZero() {
		rows, err = s.pool.Query(ctx,
			`SELECT `+cols+`
			   FROM inventory_audit_log
			  WHERE entity_id = $1 AND entity_type = $2
			  ORDER BY created_at DESC, id DESC
			  LIMIT $3`,
			entityID, entityType, limit,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT `+cols+`
			   FROM inventory_audit_log
			  WHERE entity_id = $1 AND entity_type = $2
			    AND (created_at, id) < ($3, $4)
			  ORDER BY created_at DESC, id DESC
			  LIMIT $5`,
			entityID, entityType, cur.Ts, cur.ID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuditLogEntry
	for rows.Next() {
		e, err := scanAuditLogEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *pgStore) selectAuditLogByActionKeyset(ctx context.Context, action string, cur httpkit.Cursor, limit int) ([]AuditLogEntry, error) {
	const cols = `id, entity_type, entity_id, action, actor_id,
	              from_location, to_location, from_status, to_status, reason, session_id, metadata, created_at`

	var (
		rows pgx.Rows
		err  error
	)
	if cur.IsZero() {
		rows, err = s.pool.Query(ctx,
			`SELECT `+cols+`
			   FROM inventory_audit_log
			  WHERE action = $1
			  ORDER BY created_at DESC, id DESC
			  LIMIT $2`,
			action, limit,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT `+cols+`
			   FROM inventory_audit_log
			  WHERE action = $1
			    AND (created_at, id) < ($2, $3)
			  ORDER BY created_at DESC, id DESC
			  LIMIT $4`,
			action, cur.Ts, cur.ID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuditLogEntry
	for rows.Next() {
		e, err := scanAuditLogEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanAuditLogEntry(row interface{ Scan(...any) error }) (AuditLogEntry, error) {
	var e AuditLogEntry
	var fromLoc, toLoc uuid.NullUUID
	var fromStatus, toStatus, reason sql.NullString
	var sessionID uuid.NullUUID
	var metadata []byte
	err := row.Scan(
		&e.ID, &e.EntityType, &e.EntityID, &e.Action, &e.ActorID,
		&fromLoc, &toLoc, &fromStatus, &toStatus, &reason, &sessionID, &metadata, &e.CreatedAt,
	)
	if err != nil {
		return AuditLogEntry{}, err
	}
	if len(metadata) > 0 {
		e.Metadata = metadata
	}
	if fromLoc.Valid {
		v := fromLoc.UUID
		e.FromLocation = &v
	}
	if toLoc.Valid {
		v := toLoc.UUID
		e.ToLocation = &v
	}
	if fromStatus.Valid {
		e.FromStatus = &fromStatus.String
	}
	if toStatus.Valid {
		e.ToStatus = &toStatus.String
	}
	if reason.Valid {
		e.Reason = &reason.String
	}
	if sessionID.Valid {
		v := sessionID.UUID
		e.SessionID = &v
	}
	return e, nil
}

func (s *pgStore) insertCycleCountSession(ctx context.Context, sess CycleCountSession) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO cycle_count_sessions (id, zone, status, created_by, created_at)
		 VALUES ($1, NULLIF($2,''), $3, $4, $5)`,
		sess.ID, sess.Zone, sess.Status, sess.CreatedBy, sess.CreatedAt,
	)
	return err
}

func (s *pgStore) selectCycleCountSessionByID(ctx context.Context, id uuid.UUID) (CycleCountSession, error) {
	var sess CycleCountSession
	var zone sql.NullString
	var postedBy uuid.NullUUID
	var postedAt sql.NullTime
	err := s.pool.QueryRow(ctx,
		`SELECT id, COALESCE(zone,''), status, created_by, posted_by, created_at, posted_at
		 FROM cycle_count_sessions WHERE id = $1`,
		id,
	).Scan(&sess.ID, &zone, &sess.Status, &sess.CreatedBy, &postedBy, &sess.CreatedAt, &postedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CycleCountSession{}, domain.NewBizError(domain.ErrNotFound, "cycle count session not found")
	}
	if err != nil {
		return CycleCountSession{}, err
	}
	if zone.Valid {
		sess.Zone = zone.String
	}
	if postedBy.Valid {
		v := postedBy.UUID
		sess.PostedBy = &v
	}
	if postedAt.Valid {
		t := postedAt.Time
		sess.PostedAt = &t
	}
	return sess, nil
}

func (s *pgStore) updateCycleCountSessionStatus(ctx context.Context, id uuid.UUID, status string, postedBy *uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE cycle_count_sessions
		 SET status = $1, posted_by = $2, posted_at = CASE WHEN $1 = 'POSTED' THEN NOW() ELSE NULL END
		 WHERE id = $3`,
		status, postedBy, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "cycle count session not found")
	}
	return nil
}

func (s *pgStore) insertCycleCountLine(ctx context.Context, l CycleCountLine) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO cycle_count_lines
		    (id, session_id, entity_type, entity_id, counted_status, counted_location_id, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		l.ID, l.SessionID, l.EntityType, l.EntityID,
		l.CountedStatus, l.CountedLocationID, l.Reason, l.CreatedAt,
	)
	if err != nil {
		if isPGUniqueViolation(err) {
			return domain.NewBizError(domain.ErrPreconditionFailed, "entity already counted in this session")
		}
		return err
	}
	return nil
}

func (s *pgStore) selectCycleCountLinesBySession(ctx context.Context, sessionID uuid.UUID) ([]CycleCountLine, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, session_id, entity_type, entity_id, counted_status, counted_location_id, reason, created_at
		 FROM cycle_count_lines WHERE session_id = $1 ORDER BY created_at`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CycleCountLine
	for rows.Next() {
		var l CycleCountLine
		var locID uuid.NullUUID
		if err := rows.Scan(&l.ID, &l.SessionID, &l.EntityType, &l.EntityID,
			&l.CountedStatus, &locID, &l.Reason, &l.CreatedAt); err != nil {
			return nil, err
		}
		if locID.Valid {
			v := locID.UUID
			l.CountedLocationID = &v
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *pgStore) postCycleCountAtomically(ctx context.Context, op cycleCountPostOp) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var lockedStatus string
	lockErr := tx.QueryRow(ctx,
		`SELECT status FROM cycle_count_sessions WHERE id = $1 FOR UPDATE`,
		op.SessionID).Scan(&lockedStatus)
	if errors.Is(lockErr, pgx.ErrNoRows) {
		err = domain.NewBizError(domain.ErrNotFound, "cycle count session not found")
		return err
	}
	if lockErr != nil {
		err = fmt.Errorf("lock cycle count session: %w", lockErr)
		return err
	}
	if lockedStatus != "OPEN" {
		err = domain.NewBizError(domain.ErrInvalidTransition,
			"cycle count session is not OPEN")
		return err
	}

	for i := range op.Adjustments {
		adj := &op.Adjustments[i]
		if adj.EntityType == "REMNANT" {
			if _, execErr := tx.Exec(ctx,
				`UPDATE remnants SET status = $1, bin_location_id = $2 WHERE id = $3`,
				adj.NewStatus, adj.NewLocationID, adj.EntityID,
			); execErr != nil {
				err = fmt.Errorf("adjust remnant %s: %w", adj.EntityID, execErr)
				return err
			}
		} else {
			if _, execErr := tx.Exec(ctx,
				`UPDATE board_sheets SET status = $1, bin_location_id = $2 WHERE id = $3`,
				adj.NewStatus, adj.NewLocationID, adj.EntityID,
			); execErr != nil {
				err = fmt.Errorf("adjust board_sheet %s: %w", adj.EntityID, execErr)
				return err
			}
		}

		e := adj.AuditEntry
		if _, execErr := tx.Exec(ctx,
			`INSERT INTO inventory_audit_log
			    (id, entity_type, entity_id, action, actor_id, from_location, to_location,
			     from_status, to_status, reason, session_id, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			e.ID, e.EntityType, e.EntityID, e.Action, e.ActorID,
			e.FromLocation, e.ToLocation,
			e.FromStatus, e.ToStatus,
			e.Reason, e.SessionID, e.CreatedAt,
		); execErr != nil {
			err = fmt.Errorf("insert audit log: %w", execErr)
			return err
		}
	}

	if _, execErr := tx.Exec(ctx,
		`UPDATE cycle_count_sessions
		 SET status = 'POSTED', posted_by = $1, posted_at = NOW()
		 WHERE id = $2`,
		op.PostedBy, op.SessionID,
	); execErr != nil {
		err = fmt.Errorf("update session status: %w", execErr)
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func isPGUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}

// ── BR-INV01..06: QC + supplier claim ───────────────────────────────────────

func (s *pgStore) qcPassLotAtomically(ctx context.Context, lotID uuid.UUID) (int, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE board_sheets
		    SET status = $1
		  WHERE lot_id = $2 AND status = $3`,
		SheetStatusAvailable, lotID, SheetStatusPendingQC,
	)
	if err != nil {
		return 0, fmt.Errorf("qc-pass lot: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *pgStore) rejectLotAtomically(ctx context.Context, op rejectLotOp) ([]uuid.UUID, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx,
		`SELECT id
		   FROM board_sheets
		  WHERE lot_id = $1 AND status = $2
		  ORDER BY id
		  LIMIT $3
		  FOR UPDATE`,
		op.LotID, SheetStatusPendingQC, op.Qty,
	)
	if err != nil {
		return nil, fmt.Errorf("lock pending sheets: %w", err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) < op.Qty {
		return nil, domain.NewBizError(domain.ErrPreconditionFailed,
			fmt.Sprintf("only %d PENDING_QC sheets remain, cannot reject %d", len(ids), op.Qty))
	}

	if _, err := tx.Exec(ctx,
		`UPDATE board_sheets SET status = $1 WHERE id = ANY($2)`,
		SheetStatusRejected, ids,
	); err != nil {
		return nil, fmt.Errorf("update sheets to REJECTED: %w", err)
	}

	r := op.Rejection
	if _, err := tx.Exec(ctx,
		`INSERT INTO material_rejections
		    (id, lot_id, reason_code, reason_detail, rejected_qty_sheets,
		     photo_urls, claim_amount, claim_currency, claim_status,
		     resolution_notes, reported_by, reported_at,
		     resolved_by, resolved_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, $11, $12, $13, $14)`,
		r.ID, r.LotID, r.ReasonCode, nullIfEmpty(r.ReasonDetail), r.RejectedQtySheets,
		r.PhotoURLs, r.ClaimAmount, r.ClaimCurrency, r.ClaimStatus,
		nullIfEmpty(r.ResolutionNotes), r.ReportedBy, r.ReportedAt,
		r.ResolvedBy, r.ResolvedAt,
	); err != nil {
		return nil, fmt.Errorf("insert material_rejection: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ids, nil
}

const rejectionCols = `id, lot_id, reason_code, COALESCE(reason_detail, ''),
		rejected_qty_sheets, COALESCE(photo_urls, '{}'::TEXT[]),
		claim_amount, COALESCE(claim_currency, ''), claim_status,
		COALESCE(resolution_notes, ''), reported_by, reported_at,
		resolved_by, resolved_at`

func scanRejection(row interface{ Scan(...any) error }) (MaterialRejection, error) {
	var r MaterialRejection
	var amount sql.NullInt64
	err := row.Scan(
		&r.ID, &r.LotID, &r.ReasonCode, &r.ReasonDetail,
		&r.RejectedQtySheets, &r.PhotoURLs,
		&amount, &r.ClaimCurrency, &r.ClaimStatus,
		&r.ResolutionNotes, &r.ReportedBy, &r.ReportedAt,
		&r.ResolvedBy, &r.ResolvedAt,
	)
	if amount.Valid {
		v := amount.Int64
		r.ClaimAmount = &v
	}
	if r.PhotoURLs == nil {
		r.PhotoURLs = []string{}
	}
	return r, err
}

func (s *pgStore) selectRejectionByID(ctx context.Context, id uuid.UUID) (MaterialRejection, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+rejectionCols+` FROM material_rejections WHERE id = $1`, id)
	r, err := scanRejection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return MaterialRejection{}, domain.NewBizError(domain.ErrNotFound, "material rejection not found")
	}
	return r, err
}

func (s *pgStore) selectRejectionsKeyset(ctx context.Context, f RejectionFilter, cur httpkit.Cursor, limit int) ([]MaterialRejection, error) {
	q := `SELECT ` + rejectionCols + ` FROM material_rejections WHERE 1=1`
	args := []any{}
	idx := 1
	if f.ClaimStatus != "" {
		q += fmt.Sprintf(" AND claim_status = $%d", idx)
		args = append(args, f.ClaimStatus)
		idx++
	}
	if f.LotID != nil {
		q += fmt.Sprintf(" AND lot_id = $%d", idx)
		args = append(args, *f.LotID)
		idx++
	}
	if !cur.IsZero() {
		q += fmt.Sprintf(" AND (reported_at, id) < ($%d, $%d)", idx, idx+1)
		args = append(args, cur.Ts, cur.ID)
		idx += 2
	}
	q += fmt.Sprintf(" ORDER BY reported_at DESC, id DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MaterialRejection
	for rows.Next() {
		r, err := scanRejection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) updateRejectionClaim(ctx context.Context, in updateClaimRow) (MaterialRejection, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE material_rejections
		    SET claim_status     = $1,
		        claim_amount     = COALESCE($2, claim_amount),
		        claim_currency   = COALESCE(NULLIF($3, ''), claim_currency),
		        resolution_notes = COALESCE(NULLIF($4, ''), resolution_notes),
		        resolved_by      = COALESCE($5, resolved_by),
		        resolved_at      = COALESCE($6, resolved_at)
		  WHERE id = $7`,
		in.ClaimStatus, in.ClaimAmount, in.ClaimCurrency, in.ResolutionNotes,
		in.ResolvedBy, in.ResolvedAt, in.RejectionID,
	)
	if err != nil {
		return MaterialRejection{}, err
	}
	if tag.RowsAffected() == 0 {
		return MaterialRejection{}, domain.NewBizError(domain.ErrNotFound, "material rejection not found")
	}
	return s.selectRejectionByID(ctx, in.RejectionID)
}

func (s *pgStore) selectRejectionReport(ctx context.Context, f RejectionReportFilter) ([]RejectionReport, error) {
	q := `
		SELECT COALESCE(il.supplier_ref, '') AS supplier_ref,
		       COUNT(*) FILTER (WHERE mr.claim_status = 'OPEN')                              AS open_count,
		       COALESCE(SUM(mr.claim_amount) FILTER (WHERE mr.claim_status = 'OPEN'), 0)     AS open_amount,
		       COUNT(*) FILTER (WHERE mr.claim_status = 'APPROVED')                          AS approved_count,
		       COALESCE(SUM(mr.claim_amount) FILTER (WHERE mr.claim_status = 'APPROVED'), 0) AS approved_amount,
		       COUNT(*) FILTER (WHERE mr.claim_status = 'PAID')                              AS paid_count,
		       COALESCE(SUM(mr.claim_amount) FILTER (WHERE mr.claim_status = 'PAID'), 0)     AS paid_amount,
		       COUNT(*) FILTER (WHERE mr.claim_status = 'REJECTED')                          AS rejected_count,
		       COUNT(*)                                                                       AS total_rejections
		  FROM material_rejections mr
		  JOIN inventory_lots il ON il.id = mr.lot_id
		 WHERE 1=1`
	args := []any{}
	idx := 1
	if !f.From.IsZero() {
		q += fmt.Sprintf(" AND mr.reported_at >= $%d", idx)
		args = append(args, f.From)
		idx++
	}
	if !f.To.IsZero() {
		q += fmt.Sprintf(" AND mr.reported_at < $%d", idx)
		args = append(args, f.To)
		idx++
	}
	if f.SupplierRef != "" {
		q += fmt.Sprintf(" AND il.supplier_ref ILIKE $%d", idx)
		args = append(args, "%"+f.SupplierRef+"%")
	}
	q += " GROUP BY supplier_ref ORDER BY total_rejections DESC, supplier_ref ASC"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RejectionReport
	for rows.Next() {
		var r RejectionReport
		if err := rows.Scan(
			&r.SupplierRef,
			&r.OpenCount, &r.OpenAmount,
			&r.ApprovedCount, &r.ApprovedAmount,
			&r.PaidCount, &r.PaidAmount,
			&r.RejectedCount, &r.TotalRejections,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
