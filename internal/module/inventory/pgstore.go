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
		`INSERT INTO cutting_records (id, sheet_id, remnant_source_id, work_order_id, sku_id, used_length_mm, used_width_mm, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		cr.ID, cr.SheetID, cr.RemnantSourceID,
		cr.WorkOrderID, cr.SKUID,
		cr.UsedLengthMM, cr.UsedWidthMM, cr.CreatedAt,
	)
	return err
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

	rows, err := s.pool.Query(ctx,
		`SELECT id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, shape_type, allocated_to_wo_id, allocated_at,
		        supplier_code, lot_batch, grain_pattern, quality_grade,
		        bounding_box_length_mm, bounding_box_width_mm, bin_location_id, created_at
		 FROM remnants`+baseWhere+`
		 ORDER BY (COALESCE(bounding_box_length_mm, length_mm) * COALESCE(bounding_box_width_mm, width_mm)) ASC
		 LIMIT $4 OFFSET $5`,
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
		`INSERT INTO cutting_records (id, sheet_id, remnant_source_id, work_order_id, sku_id, used_length_mm, used_width_mm, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		cr.ID, cr.SheetID, cr.RemnantSourceID,
		cr.WorkOrderID, cr.SKUID,
		cr.UsedLengthMM, cr.UsedWidthMM, cr.CreatedAt,
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
	_, err := s.pool.Exec(ctx,
		`INSERT INTO inventory_audit_log
		    (id, entity_type, entity_id, action, actor_id, from_location, to_location,
		     from_status, to_status, reason, session_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		entry.ID, entry.EntityType, entry.EntityID, entry.Action, entry.ActorID,
		entry.FromLocation, entry.ToLocation,
		entry.FromStatus, entry.ToStatus,
		entry.Reason, entry.SessionID, entry.CreatedAt,
	)
	return err
}

func (s *pgStore) selectAuditLogByEntity(ctx context.Context, entityID uuid.UUID, entityType string) ([]AuditLogEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, entity_type, entity_id, action, actor_id,
		        from_location, to_location, from_status, to_status, reason, session_id, created_at
		 FROM inventory_audit_log
		 WHERE entity_id = $1 AND entity_type = $2
		 ORDER BY created_at DESC`,
		entityID, entityType,
	)
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
	err := row.Scan(
		&e.ID, &e.EntityType, &e.EntityID, &e.Action, &e.ActorID,
		&fromLoc, &toLoc, &fromStatus, &toStatus, &reason, &sessionID, &e.CreatedAt,
	)
	if err != nil {
		return AuditLogEntry{}, err
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
