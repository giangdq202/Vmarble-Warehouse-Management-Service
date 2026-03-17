package inventory

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type pgStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) store {
	return &pgStore{pool: pool}
}

func (s *pgStore) insertLot(ctx context.Context, lot InventoryLot) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO inventory_lots (id, material_id, quantity, cost_per_sheet_amount, cost_per_sheet_currency, supplier_ref, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		lot.ID, lot.MaterialID, lot.Quantity,
		lot.CostPerSheet.Amount, lot.CostPerSheet.Currency,
		lot.SupplierRef, lot.ReceivedAt,
	)
	return err
}

func (s *pgStore) selectLots(ctx context.Context) ([]InventoryLot, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, material_id, quantity, cost_per_sheet_amount, cost_per_sheet_currency, supplier_ref, received_at
		 FROM inventory_lots ORDER BY received_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lots []InventoryLot
	for rows.Next() {
		var l InventoryLot
		if err := rows.Scan(&l.ID, &l.MaterialID, &l.Quantity,
			&l.CostPerSheet.Amount, &l.CostPerSheet.Currency,
			&l.SupplierRef, &l.ReceivedAt); err != nil {
			return nil, err
		}
		lots = append(lots, l)
	}
	return lots, rows.Err()
}

func (s *pgStore) insertSheets(ctx context.Context, sheets []BoardSheet) error {
	batch := &pgx.Batch{}
	for _, sh := range sheets {
		batch.Queue(
			`INSERT INTO board_sheets (id, lot_id, length_mm, width_mm, cost_amount, cost_currency, status)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			sh.ID, sh.LotID, sh.Dimensions.LengthMM, sh.Dimensions.WidthMM,
			sh.CostPerSheet.Amount, sh.CostPerSheet.Currency, sh.Status,
		)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range sheets {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *pgStore) selectSheetByID(ctx context.Context, id uuid.UUID) (BoardSheet, error) {
	var sh BoardSheet
	err := s.pool.QueryRow(ctx,
		`SELECT id, lot_id, length_mm, width_mm, cost_amount, cost_currency, status, issued_to_wo_id
		 FROM board_sheets WHERE id = $1`, id).
		Scan(&sh.ID, &sh.LotID,
			&sh.Dimensions.LengthMM, &sh.Dimensions.WidthMM,
			&sh.CostPerSheet.Amount, &sh.CostPerSheet.Currency,
			&sh.Status, &sh.IssuedToWorkOrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BoardSheet{}, domain.NewBizError(domain.ErrNotFound, "board sheet not found")
	}
	return sh, err
}

func (s *pgStore) selectAvailableSheets(ctx context.Context) ([]BoardSheet, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, lot_id, length_mm, width_mm, cost_amount, cost_currency, status, issued_to_wo_id
		 FROM board_sheets WHERE status = 'AVAILABLE' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sheets []BoardSheet
	for rows.Next() {
		var sh BoardSheet
		if err := rows.Scan(&sh.ID, &sh.LotID,
			&sh.Dimensions.LengthMM, &sh.Dimensions.WidthMM,
			&sh.CostPerSheet.Amount, &sh.CostPerSheet.Currency,
			&sh.Status, &sh.IssuedToWorkOrderID); err != nil {
			return nil, err
		}
		sheets = append(sheets, sh)
	}
	return sheets, rows.Err()
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
		`INSERT INTO remnants (id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, allocated_to_wo_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		r.ID, r.ParentBoardID, r.ParentRemnantID,
		r.Dimensions.LengthMM, r.Dimensions.WidthMM,
		string(r.Status), r.AllocatedToWO, r.CreatedAt,
	)
	return err
}

func (s *pgStore) selectAvailableRemnantsByMinDimension(ctx context.Context, minDim domain.Dimension) ([]Remnant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, allocated_to_wo_id, created_at
		 FROM remnants
		 WHERE status = 'AVAILABLE' AND length_mm >= $1 AND width_mm >= $2
		 ORDER BY (length_mm * width_mm) ASC`,
		minDim.LengthMM, minDim.WidthMM)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRemnants(rows)
}

func (s *pgStore) selectRemnantsByBoardSheet(ctx context.Context, boardSheetID uuid.UUID) ([]Remnant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, allocated_to_wo_id, created_at
		 FROM remnants WHERE parent_board_id = $1 ORDER BY created_at`, boardSheetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRemnants(rows)
}

func (s *pgStore) selectRemnantByID(ctx context.Context, id uuid.UUID) (Remnant, error) {
	var r Remnant
	err := s.pool.QueryRow(ctx,
		`SELECT id, parent_board_id, parent_remnant_id, length_mm, width_mm, status, allocated_to_wo_id, created_at
		 FROM remnants WHERE id = $1`, id).
		Scan(&r.ID, &r.ParentBoardID, &r.ParentRemnantID,
			&r.Dimensions.LengthMM, &r.Dimensions.WidthMM,
			&r.Status, &r.AllocatedToWO, &r.CreatedAt)
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

func scanRemnants(rows pgx.Rows) ([]Remnant, error) {
	var remnants []Remnant
	for rows.Next() {
		var r Remnant
		if err := rows.Scan(&r.ID, &r.ParentBoardID, &r.ParentRemnantID,
			&r.Dimensions.LengthMM, &r.Dimensions.WidthMM,
			&r.Status, &r.AllocatedToWO, &r.CreatedAt); err != nil {
			return nil, err
		}
		remnants = append(remnants, r)
	}
	return remnants, rows.Err()
}
