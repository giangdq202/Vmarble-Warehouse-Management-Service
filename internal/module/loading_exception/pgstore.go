package loading_exception

import (
	"context"
	"errors"
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

const leSelectCols = `
SELECT id, container_id, loading_plan_id, exception_type, sku_id, qty,
       reason, photo_urls, approved_by, approved_at, resolution,
       resolution_notes, carry_over_so_line_id, substitute_sku_id,
       created_by, created_at
  FROM loading_exceptions`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanException(r rowScanner) (LoadingException, error) {
	var e LoadingException
	var (
		planID, skuID, approvedBy, carryOverID, subSKUID uuid.NullUUID
		qty                                              *int
		approvedAt                                       *time.Time
		resolution                                       *string
		photoURLs                                        []string
	)
	if err := r.Scan(
		&e.ID, &e.ContainerID, &planID, &e.ExceptionType, &skuID, &qty,
		&e.Reason, &photoURLs, &approvedBy, &approvedAt, &resolution,
		&e.ResolutionNotes, &carryOverID, &subSKUID,
		&e.CreatedBy, &e.CreatedAt,
	); err != nil {
		return LoadingException{}, err
	}
	if planID.Valid {
		v := planID.UUID
		e.LoadingPlanID = &v
	}
	if skuID.Valid {
		v := skuID.UUID
		e.SKUID = &v
	}
	if qty != nil {
		v := *qty
		e.Qty = &v
	}
	if approvedBy.Valid {
		v := approvedBy.UUID
		e.ApprovedBy = &v
	}
	e.ApprovedAt = approvedAt
	e.Resolution = resolution
	if carryOverID.Valid {
		v := carryOverID.UUID
		e.CarryOverSOLineID = &v
	}
	if subSKUID.Valid {
		v := subSKUID.UUID
		e.SubstituteSKUID = &v
	}
	if photoURLs == nil {
		photoURLs = []string{}
	}
	e.PhotoURLs = photoURLs
	return e, nil
}

func (s *pgStore) insert(ctx context.Context, e LoadingException) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO loading_exceptions
		    (id, container_id, loading_plan_id, exception_type, sku_id, qty,
		     reason, photo_urls, created_by, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		e.ID, e.ContainerID, e.LoadingPlanID, e.ExceptionType, e.SKUID, e.Qty,
		e.Reason, e.PhotoURLs, e.CreatedBy, e.CreatedAt,
	)
	return err
}

func (s *pgStore) selectByID(ctx context.Context, id uuid.UUID) (LoadingException, error) {
	row := s.pool.QueryRow(ctx, leSelectCols+` WHERE id = $1`, id)
	e, err := scanException(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LoadingException{}, domain.ErrNotFound
		}
		return LoadingException{}, err
	}
	return e, nil
}

func (s *pgStore) selectByContainerKeyset(ctx context.Context, containerID uuid.UUID, status string, cur httpkit.Cursor, limit int) ([]LoadingException, error) {
	statusFilter := ""
	switch status {
	case "pending":
		statusFilter = ` AND approved_by IS NULL`
	case "approved":
		statusFilter = ` AND approved_by IS NOT NULL`
	}

	var (
		rows pgx.Rows
		err  error
	)
	if cur.IsZero() {
		rows, err = s.pool.Query(ctx,
			leSelectCols+` WHERE container_id = $1`+statusFilter+`
			               ORDER BY created_at DESC, id DESC
			               LIMIT $2`,
			containerID, limit,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			leSelectCols+` WHERE container_id = $1`+statusFilter+`
			                 AND (created_at, id) < ($2, $3)
			               ORDER BY created_at DESC, id DESC
			               LIMIT $4`,
			containerID, cur.Ts, cur.ID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LoadingException
	for rows.Next() {
		e, err := scanException(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *pgStore) pendingByContainer(ctx context.Context, containerID uuid.UUID) (PendingSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM loading_exceptions
		   WHERE container_id = $1 AND approved_by IS NULL
		   ORDER BY created_at ASC, id ASC`,
		containerID,
	)
	if err != nil {
		return PendingSummary{}, err
	}
	defer rows.Close()

	out := PendingSummary{IDs: []uuid.UUID{}}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return PendingSummary{}, err
		}
		out.IDs = append(out.IDs, id)
	}
	out.Count = len(out.IDs)
	return out, rows.Err()
}

func (s *pgStore) withTx(ctx context.Context, fn func(tx txStore) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
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

func (t *pgTxStore) lockForUpdate(ctx context.Context, id uuid.UUID) (LoadingException, error) {
	row := t.tx.QueryRow(ctx, leSelectCols+` WHERE id = $1 FOR UPDATE`, id)
	e, err := scanException(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return LoadingException{}, domain.ErrNotFound
		}
		return LoadingException{}, err
	}
	return e, nil
}

func (t *pgTxStore) approve(ctx context.Context, in approveRow) error {
	tag, err := t.tx.Exec(ctx,
		`UPDATE loading_exceptions
		    SET approved_by = $2,
		        approved_at = NOW(),
		        resolution = $3,
		        resolution_notes = $4,
		        carry_over_so_line_id = $5,
		        substitute_sku_id = $6
		  WHERE id = $1
		    AND approved_by IS NULL`,
		in.ID, in.ApprovedBy, in.Resolution, in.ResolutionNotes,
		in.CarryOverSOLineID, in.SubstituteSKUID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrInvalidTransition,
			"loading exception is already approved or does not exist")
	}
	return nil
}

func (t *pgTxStore) reject(ctx context.Context, in rejectRow) error {
	tag, err := t.tx.Exec(ctx,
		`UPDATE loading_exceptions
		    SET approved_by = $2,
		        approved_at = NOW(),
		        resolution = NULL,
		        resolution_notes = $3
		  WHERE id = $1
		    AND approved_by IS NULL`,
		in.ID, in.ApprovedBy, in.Reason,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrInvalidTransition,
			"loading exception is already approved or does not exist")
	}
	return nil
}
