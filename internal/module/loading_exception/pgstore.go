package loading_exception

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// crossContainerWhere builds the WHERE fragment + arg slice shared by the
// list endpoint and the summary counter. Customer filter joins through
// container_lines → sales_order_lines → sales_orders so the row-level filter
// works without changing schema.
func crossContainerWhere(f CrossContainerFilter, startIdx int) (string, []any) {
	clauses := []string{}
	args := []any{}
	idx := startIdx

	switch f.Status {
	case "pending":
		clauses = append(clauses, "le.approved_by IS NULL")
	case "approved":
		clauses = append(clauses, "le.approved_by IS NOT NULL AND le.resolution IS NOT NULL")
	case "rejected":
		clauses = append(clauses, "le.approved_by IS NOT NULL AND le.resolution IS NULL")
	}
	if f.ContainerID != nil {
		clauses = append(clauses, fmt.Sprintf("le.container_id = $%d", idx))
		args = append(args, *f.ContainerID)
		idx++
	}
	if f.ExceptionType != "" {
		clauses = append(clauses, fmt.Sprintf("le.exception_type = $%d", idx))
		args = append(args, f.ExceptionType)
		idx++
	}
	if !f.From.IsZero() {
		clauses = append(clauses, fmt.Sprintf("le.created_at >= $%d", idx))
		args = append(args, f.From)
		idx++
	}
	if !f.To.IsZero() {
		clauses = append(clauses, fmt.Sprintf("le.created_at < $%d", idx))
		args = append(args, f.To)
		idx++
	}
	if f.CustomerID != nil {
		clauses = append(clauses, fmt.Sprintf(`EXISTS (
			SELECT 1
			  FROM container_lines cl
			  JOIN sales_order_lines sol ON sol.id = cl.sales_order_line_id
			  JOIN sales_orders so       ON so.id  = sol.sales_order_id
			 WHERE cl.container_id = le.container_id
			   AND so.customer_id = $%d
		)`, idx))
		args = append(args, *f.CustomerID)
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

const leSelectColsAliased = `
SELECT le.id, le.container_id, le.loading_plan_id, le.exception_type, le.sku_id, le.qty,
       le.reason, le.photo_urls, le.approved_by, le.approved_at, le.resolution,
       le.resolution_notes, le.carry_over_so_line_id, le.substitute_sku_id,
       le.created_by, le.created_at
  FROM loading_exceptions le`

func (s *pgStore) selectCrossContainerKeyset(ctx context.Context, f CrossContainerFilter, cur httpkit.Cursor, limit int) ([]LoadingException, error) {
	whereClause, args := crossContainerWhere(f, 1)
	q := leSelectColsAliased + whereClause
	idx := len(args) + 1
	if !cur.IsZero() {
		if whereClause == "" {
			q += fmt.Sprintf(" WHERE (le.created_at, le.id) < ($%d, $%d)", idx, idx+1)
		} else {
			q += fmt.Sprintf(" AND (le.created_at, le.id) < ($%d, $%d)", idx, idx+1)
		}
		args = append(args, cur.Ts, cur.ID)
		idx += 2
	}
	q += fmt.Sprintf(" ORDER BY le.created_at DESC, le.id DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, q, args...)
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

func (s *pgStore) crossContainerSummary(ctx context.Context, f CrossContainerFilter) (CrossContainerSummary, error) {
	// Summary always counts pending rows regardless of caller filter — the
	// pinned counter is "X exceptions blocking N containers" by definition.
	scoped := f
	scoped.Status = "pending"
	whereClause, args := crossContainerWhere(scoped, 1)

	q := `SELECT COUNT(*) AS pending_count,
	             COUNT(DISTINCT le.container_id) AS blocked_containers
	        FROM loading_exceptions le` + whereClause

	var sum CrossContainerSummary
	err := s.pool.QueryRow(ctx, q, args...).Scan(&sum.PendingCount, &sum.BlockedContainers)
	return sum, err
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
