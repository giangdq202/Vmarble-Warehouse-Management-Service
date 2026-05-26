package packing

import (
	"context"
	"errors"
	"strconv"
	"strings"

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

// ── FG reads ────────────────────────────────────────────────────────────────

const fgSelectCols = `
SELECT fp.id, fp.work_order_id, fp.sku_id, s.code, s.name, fp.barcode_id,
       fp.sales_order_line_id, fp.status, fp.container_line_id,
       fp.qc_passed_at, fp.qc_passed_by, fp.created_at
  FROM fg_pool fp
  JOIN skus s ON s.id = fp.sku_id`

type fgScanner interface {
	Scan(dest ...any) error
}

func scanFG(r fgScanner) (FGPool, error) {
	var fg FGPool
	if err := r.Scan(&fg.ID, &fg.WorkOrderID, &fg.SKUID, &fg.SKUCode, &fg.SKUName,
		&fg.BarcodeID, &fg.SalesOrderLineID, &fg.Status, &fg.ContainerLineID,
		&fg.QCPassedAt, &fg.QCPassedBy, &fg.CreatedAt); err != nil {
		return FGPool{}, err
	}
	return fg, nil
}

func (s *pgStore) insertFGBatch(ctx context.Context, rows []FGPool) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, r := range rows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO fg_pool
			    (id, work_order_id, sku_id, barcode_id, sales_order_line_id,
			     status, container_line_id, qc_passed_at, qc_passed_by, created_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			r.ID, r.WorkOrderID, r.SKUID, r.BarcodeID, r.SalesOrderLineID,
			r.Status, r.ContainerLineID, r.QCPassedAt, r.QCPassedBy, r.CreatedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *pgStore) selectFGByID(ctx context.Context, id uuid.UUID) (FGPool, error) {
	row := s.pool.QueryRow(ctx, fgSelectCols+` WHERE fp.id = $1`, id)
	fg, err := scanFG(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FGPool{}, domain.ErrNotFound
		}
		return FGPool{}, err
	}
	return fg, nil
}

func (s *pgStore) selectFGByBarcodeID(ctx context.Context, barcodeID uuid.UUID) (FGPool, error) {
	row := s.pool.QueryRow(ctx, fgSelectCols+` WHERE fp.barcode_id = $1`, barcodeID)
	fg, err := scanFG(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FGPool{}, domain.ErrNotFound
		}
		return FGPool{}, err
	}
	return fg, nil
}

func (s *pgStore) selectFGByWorkOrderID(ctx context.Context, woID uuid.UUID) ([]FGPool, error) {
	rows, err := s.pool.Query(ctx, fgSelectCols+` WHERE fp.work_order_id = $1 ORDER BY fp.created_at, fp.id`, woID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FGPool
	for rows.Next() {
		fg, err := scanFG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, fg)
	}
	return out, rows.Err()
}

func (s *pgStore) selectFGPaged(ctx context.Context, p httpkit.PageParams, f FGListFilter) ([]FGPool, int, error) {
	args := []any{f.Status}
	clauses := []string{`($1::text = '' OR fp.status = $1)`}

	addNullable := func(col string, val *uuid.UUID) {
		if val == nil {
			return
		}
		args = append(args, *val)
		clauses = append(clauses, col+" = $"+strconv.Itoa(len(args)))
	}
	addNullable("fp.sku_id", f.SKUID)
	addNullable("fp.sales_order_line_id", f.SOLineID)
	addNullable("fp.work_order_id", f.WorkOrderID)

	where := strings.Join(clauses, " AND ")

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM fg_pool fp WHERE `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, p.Limit, p.Offset())
	limitArg := "$" + strconv.Itoa(len(args)-1)
	offsetArg := "$" + strconv.Itoa(len(args))

	rows, err := s.pool.Query(ctx,
		fgSelectCols+` WHERE `+where+` ORDER BY fp.created_at DESC, fp.id DESC LIMIT `+limitArg+` OFFSET `+offsetArg,
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []FGPool
	for rows.Next() {
		fg, err := scanFG(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, fg)
	}
	return out, total, rows.Err()
}

// ── Defect reads ────────────────────────────────────────────────────────────

const defectSelectCols = `
SELECT id, fg_pool_id, reason, COALESCE(detail, ''), photo_urls,
       detected_by, detected_at,
       COALESCE(resolution, ''), resolved_by, resolved_at, COALESCE(note, '')
  FROM fg_defect`

func scanDefect(r fgScanner) (FGDefect, error) {
	var d FGDefect
	if err := r.Scan(&d.ID, &d.FGPoolID, &d.Reason, &d.Detail, &d.PhotoURLs,
		&d.DetectedBy, &d.DetectedAt, &d.Resolution, &d.ResolvedBy, &d.ResolvedAt, &d.Note); err != nil {
		return FGDefect{}, err
	}
	return d, nil
}

func (s *pgStore) selectDefectByID(ctx context.Context, id uuid.UUID) (FGDefect, error) {
	row := s.pool.QueryRow(ctx, defectSelectCols+` WHERE id = $1`, id)
	d, err := scanDefect(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FGDefect{}, domain.ErrNotFound
		}
		return FGDefect{}, err
	}
	return d, nil
}

func (s *pgStore) selectDefectByFGID(ctx context.Context, fgID uuid.UUID) (FGDefect, error) {
	row := s.pool.QueryRow(ctx, defectSelectCols+` WHERE fg_pool_id = $1`, fgID)
	d, err := scanDefect(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FGDefect{}, domain.ErrNotFound
		}
		return FGDefect{}, err
	}
	return d, nil
}

// ── Tx surface ──────────────────────────────────────────────────────────────

func (s *pgStore) withTx(ctx context.Context, fn func(tx txStore) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := fn(&pgTxStore{tx: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgTxStore struct {
	tx pgx.Tx
}

func (t *pgTxStore) rawTx() pgx.Tx { return t.tx }

func (t *pgTxStore) lockFGForUpdate(ctx context.Context, id uuid.UUID) (FGPool, error) {
	row := t.tx.QueryRow(ctx, fgSelectCols+` WHERE fp.id = $1 FOR UPDATE OF fp`, id)
	fg, err := scanFG(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FGPool{}, domain.ErrNotFound
		}
		return FGPool{}, err
	}
	return fg, nil
}

func (t *pgTxStore) lockFGByBarcodeForUpdate(ctx context.Context, barcodeID uuid.UUID) (FGPool, error) {
	row := t.tx.QueryRow(ctx, fgSelectCols+` WHERE fp.barcode_id = $1 FOR UPDATE OF fp`, barcodeID)
	fg, err := scanFG(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FGPool{}, domain.ErrNotFound
		}
		return FGPool{}, err
	}
	return fg, nil
}

// lockAvailableFGsForReserve locks `qty` AVAILABLE rows whose (sku_id,
// sales_order_line_id) match. Skips rows already locked by other tx so
// concurrent AddLine flows do not stall waiting on each other; the qty
// they actually get back may be less than requested and the caller decides
// whether that is fatal (BR-PK soft-allocation note).
func (t *pgTxStore) lockAvailableFGsForReserve(ctx context.Context, skuID, soLineID uuid.UUID, qty int) ([]FGPool, error) {
	rows, err := t.tx.Query(ctx,
		fgSelectCols+`
		 WHERE fp.sku_id = $1
		   AND fp.sales_order_line_id = $2
		   AND fp.status = 'AVAILABLE'
		 ORDER BY fp.created_at, fp.id
		 LIMIT $3
		 FOR UPDATE OF fp SKIP LOCKED`,
		skuID, soLineID, qty)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FGPool
	for rows.Next() {
		fg, err := scanFG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, fg)
	}
	return out, rows.Err()
}

func (t *pgTxStore) lockReservedFGsByContainerLine(ctx context.Context, containerLineID uuid.UUID) ([]FGPool, error) {
	rows, err := t.tx.Query(ctx,
		fgSelectCols+`
		 WHERE fp.container_line_id = $1
		   AND fp.status IN ('RESERVED','LOADED')
		 ORDER BY fp.id
		 FOR UPDATE OF fp`,
		containerLineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FGPool
	for rows.Next() {
		fg, err := scanFG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, fg)
	}
	return out, rows.Err()
}

func (t *pgTxStore) lockReservedFGsByContainer(ctx context.Context, containerID uuid.UUID) ([]FGPool, error) {
	rows, err := t.tx.Query(ctx,
		fgSelectCols+`
		  JOIN container_lines cl ON cl.id = fp.container_line_id
		 WHERE cl.container_id = $1
		   AND fp.status = 'RESERVED'
		 ORDER BY fp.id
		 FOR UPDATE OF fp`,
		containerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FGPool
	for rows.Next() {
		fg, err := scanFG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, fg)
	}
	return out, rows.Err()
}

func (t *pgTxStore) flipFGStatus(ctx context.Context, in flipStatusInput) error {
	tag, err := t.tx.Exec(ctx,
		`UPDATE fg_pool SET status = $2, container_line_id = $3 WHERE id = $1`,
		in.FGID, in.ToStatus, in.ContainerLineID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "chk_fg_status") {
			return domain.NewBizError(domain.ErrInvalidInput, "invalid fg status transition")
		}
		if strings.Contains(err.Error(), "chk_fg_reserved_has_line") {
			return domain.NewBizError(domain.ErrInvalidInput, "RESERVED/LOADED FG must reference a container_line")
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (t *pgTxStore) bulkFlipFGStatus(ctx context.Context, ids []uuid.UUID, toStatus string, containerLineID *uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := t.tx.Exec(ctx,
		`UPDATE fg_pool SET status = $2, container_line_id = $3 WHERE id = ANY($1)`,
		ids, toStatus, containerLineID,
	)
	return err
}

func (t *pgTxStore) insertDefect(ctx context.Context, d FGDefect) error {
	_, err := t.tx.Exec(ctx,
		`INSERT INTO fg_defect
		    (id, fg_pool_id, reason, detail, photo_urls, detected_by, detected_at)
		 VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7)`,
		d.ID, d.FGPoolID, d.Reason, d.Detail, d.PhotoURLs, d.DetectedBy, d.DetectedAt,
	)
	return err
}

func (t *pgTxStore) updateDefectResolution(ctx context.Context, in updateResolutionInput) error {
	tag, err := t.tx.Exec(ctx,
		`UPDATE fg_defect
		    SET resolution = $2, note = NULLIF($3,''), resolved_by = $4, resolved_at = NOW()
		  WHERE id = $1
		    AND resolution IS NULL`,
		in.DefectID, in.Resolution, in.Note, in.ResolvedBy,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrInvalidTransition, "defect already resolved or not found")
	}
	return nil
}
