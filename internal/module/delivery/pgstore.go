package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// ── Container code generator ────────────────────────────────────────────────

func (s *pgStore) nextContainerCode(ctx context.Context, now time.Time) (string, error) {
	dateKey := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var seq int
	err := s.pool.QueryRow(ctx,
		`INSERT INTO container_code_counters (date_key, last_seq)
		 VALUES ($1, 1)
		 ON CONFLICT (date_key) DO UPDATE
		   SET last_seq = container_code_counters.last_seq + 1
		 RETURNING last_seq`,
		dateKey,
	).Scan(&seq)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CONT%04d%02d%02d-%03d", now.Year(), now.Month(), now.Day(), seq), nil
}

// ── Container CRUD ──────────────────────────────────────────────────────────

func (s *pgStore) insertContainer(ctx context.Context, c Container) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO containers
		    (id, code, container_type, max_cbm, max_payload_kg,
		     status, sealed_at, sealed_by, note, created_by, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11)`,
		c.ID, c.Code, c.ContainerType, c.MaxCBM, c.MaxPayloadKG,
		c.Status, c.SealedAt, c.SealedBy, c.Note, c.CreatedBy, c.CreatedAt,
	)
	return err
}

func (s *pgStore) selectContainerByID(ctx context.Context, id uuid.UUID) (Container, error) {
	row := s.pool.QueryRow(ctx, selectContainerCols+` WHERE id = $1`, id)
	c, err := scanContainer(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Container{}, domain.ErrNotFound
		}
		return Container{}, err
	}
	return c, nil
}

func (s *pgStore) selectContainerLines(ctx context.Context, containerID uuid.UUID) ([]ContainerLine, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT cl.id, cl.container_id, cl.sku_id, s.code, s.name,
		        cl.qty, cl.sales_order_line_id, cl.cbm_total, cl.weight_kg_total,
		        cl.added_by, cl.added_at
		   FROM container_lines cl
		   JOIN skus s ON s.id = cl.sku_id
		  WHERE cl.container_id = $1
		  ORDER BY cl.added_at, cl.id`,
		containerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContainerLine
	for rows.Next() {
		var l ContainerLine
		if err := rows.Scan(&l.ID, &l.ContainerID, &l.SKUID, &l.SKUCode, &l.SKUName,
			&l.Qty, &l.SalesOrderLineID, &l.CBMTotal, &l.WeightKGTotal,
			&l.AddedBy, &l.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *pgStore) selectContainersPaged(ctx context.Context, p httpkit.PageParams, f ContainerListFilter) ([]Container, int, error) {
	search := "%" + p.Search + "%"

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM containers
		  WHERE ($1::text = '' OR status = $1)
		    AND ($2::text = '' OR container_type = $2)
		    AND ($3::text = '' OR code ILIKE $3)`,
		f.Status, f.ContainerType, search,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx,
		selectContainerCols+`
		  WHERE ($1::text = '' OR status = $1)
		    AND ($2::text = '' OR container_type = $2)
		    AND ($3::text = '' OR code ILIKE $3)
		  ORDER BY created_at DESC, id DESC
		 LIMIT $4 OFFSET $5`,
		f.Status, f.ContainerType, search, p.Limit, p.Offset(),
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Container
	for rows.Next() {
		c, err := scanContainer(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (s *pgStore) selectStatusLog(ctx context.Context, containerID uuid.UUID) ([]ContainerStatusLogEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, container_id, from_status, to_status, actor_id, note, created_at
		   FROM container_status_log
		  WHERE container_id = $1
		  ORDER BY created_at DESC, id DESC`,
		containerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContainerStatusLogEntry
	for rows.Next() {
		var e ContainerStatusLogEntry
		var fromStatus, note *string
		if err := rows.Scan(&e.ID, &e.ContainerID, &fromStatus, &e.ToStatus,
			&e.ActorID, &note, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.FromStatus = stringFromPtr(fromStatus)
		e.Note = stringFromPtr(note)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Transaction support ─────────────────────────────────────────────────────

func (s *pgStore) withTx(ctx context.Context, fn func(tx txStore, raw pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := fn(&pgTxStore{tx: tx}, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pgTxStore struct {
	tx pgx.Tx
}

func (t *pgTxStore) lockContainerForUpdate(ctx context.Context, id uuid.UUID) (Container, error) {
	row := t.tx.QueryRow(ctx, selectContainerCols+` WHERE id = $1 FOR UPDATE`, id)
	c, err := scanContainer(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Container{}, domain.ErrNotFound
		}
		return Container{}, err
	}
	return c, nil
}

func (t *pgTxStore) lockLineForUpdate(ctx context.Context, lineID uuid.UUID) (ContainerLine, error) {
	row := t.tx.QueryRow(ctx,
		`SELECT id, container_id, sku_id, qty, sales_order_line_id,
		        cbm_total, weight_kg_total, added_by, added_at
		   FROM container_lines
		  WHERE id = $1
		  FOR UPDATE`,
		lineID)
	var l ContainerLine
	if err := row.Scan(&l.ID, &l.ContainerID, &l.SKUID, &l.Qty, &l.SalesOrderLineID,
		&l.CBMTotal, &l.WeightKGTotal, &l.AddedBy, &l.AddedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ContainerLine{}, domain.ErrNotFound
		}
		return ContainerLine{}, err
	}
	return l, nil
}

func (t *pgTxStore) sumLinesAggregates(ctx context.Context, containerID uuid.UUID) (float64, float64, error) {
	var cbm, weight float64
	err := t.tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(cbm_total), 0), COALESCE(SUM(weight_kg_total), 0)
		   FROM container_lines WHERE container_id = $1`,
		containerID).Scan(&cbm, &weight)
	return cbm, weight, err
}

func (t *pgTxStore) listLinesForSeal(ctx context.Context, containerID uuid.UUID) ([]ShipmentItem, error) {
	rows, err := t.tx.Query(ctx,
		`SELECT sales_order_line_id, SUM(qty)::int
		   FROM container_lines
		  WHERE container_id = $1
		  GROUP BY sales_order_line_id`,
		containerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ShipmentItem
	for rows.Next() {
		var item ShipmentItem
		if err := rows.Scan(&item.SOLineID, &item.Qty); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (t *pgTxStore) insertLine(ctx context.Context, line ContainerLine) error {
	_, err := t.tx.Exec(ctx,
		`INSERT INTO container_lines
		    (id, container_id, sku_id, qty, sales_order_line_id,
		     cbm_total, weight_kg_total, added_by, added_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		line.ID, line.ContainerID, line.SKUID, line.Qty, line.SalesOrderLineID,
		line.CBMTotal, line.WeightKGTotal, line.AddedBy, line.AddedAt,
	)
	return err
}

func (t *pgTxStore) deleteLine(ctx context.Context, lineID uuid.UUID) error {
	tag, err := t.tx.Exec(ctx, `DELETE FROM container_lines WHERE id = $1`, lineID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (t *pgTxStore) updateLineQty(ctx context.Context, lineID uuid.UUID, qty int, cbm, weight float64) error {
	tag, err := t.tx.Exec(ctx,
		`UPDATE container_lines
		    SET qty = $2, cbm_total = $3, weight_kg_total = $4
		  WHERE id = $1`,
		lineID, qty, cbm, weight,
	)
	if err != nil {
		if strings.Contains(err.Error(), "container_lines_qty_check") {
			return domain.NewBizError(domain.ErrInvalidInput, "line qty must remain > 0")
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (t *pgTxStore) updateContainerStatus(ctx context.Context, in updateStatusInput) (Container, error) {
	// Single round-trip: flip status (+ sealed_at/by where relevant) and write
	// the audit row in the same statement set so a failure rolls back both.
	switch in.ToStatus {
	case ContainerStatusSealed:
		_, err := t.tx.Exec(ctx,
			`UPDATE containers
			    SET status = $2, sealed_at = $3, sealed_by = $4
			  WHERE id = $1`,
			in.ContainerID, in.ToStatus, in.Now, in.ActorID,
		)
		if err != nil {
			return Container{}, err
		}
	case ContainerStatusLoading:
		// Reopen path also lands here; clear sealed_at / sealed_by so the
		// next seal records a fresh timestamp + sealer.
		_, err := t.tx.Exec(ctx,
			`UPDATE containers
			    SET status = $2, sealed_at = NULL, sealed_by = NULL
			  WHERE id = $1`,
			in.ContainerID, in.ToStatus,
		)
		if err != nil {
			return Container{}, err
		}
	default:
		_, err := t.tx.Exec(ctx,
			`UPDATE containers SET status = $2 WHERE id = $1`,
			in.ContainerID, in.ToStatus,
		)
		if err != nil {
			return Container{}, err
		}
	}

	if _, err := t.tx.Exec(ctx,
		`INSERT INTO container_status_log
		    (id, container_id, from_status, to_status, actor_id, note, created_at)
		 VALUES ($1, $2, NULLIF($3,''), $4, $5, NULLIF($6,''), $7)`,
		uuid.New(), in.ContainerID, in.FromStatus, in.ToStatus, in.ActorID, in.Note, in.Now,
	); err != nil {
		return Container{}, err
	}

	row := t.tx.QueryRow(ctx, selectContainerCols+` WHERE id = $1`, in.ContainerID)
	c, err := scanContainer(row)
	if err != nil {
		return Container{}, err
	}
	return c, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

const selectContainerCols = `
SELECT id, code, container_type, max_cbm, max_payload_kg,
       status, sealed_at, sealed_by, note, created_by, created_at
  FROM containers`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanContainer(r rowScanner) (Container, error) {
	var c Container
	var note *string
	if err := r.Scan(&c.ID, &c.Code, &c.ContainerType, &c.MaxCBM, &c.MaxPayloadKG,
		&c.Status, &c.SealedAt, &c.SealedBy, &note, &c.CreatedBy, &c.CreatedAt); err != nil {
		return Container{}, err
	}
	c.Note = stringFromPtr(note)
	return c, nil
}

func stringFromPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ── Loading plans (#301) ────────────────────────────────────────────────────

const selectLoadingPlanCols = `
SELECT id, container_id, excel_file_url, excel_hash, parsed_at, uploaded_by,
       status, version, notes, approved_at, approved_by,
       superseded_at, superseded_by, created_at
  FROM loading_plans`

func scanLoadingPlan(r rowScanner) (LoadingPlan, error) {
	var p LoadingPlan
	if err := r.Scan(
		&p.ID, &p.ContainerID, &p.ExcelFileURL, &p.ExcelHash, &p.ParsedAt, &p.UploadedBy,
		&p.Status, &p.Version, &p.Notes, &p.ApprovedAt, &p.ApprovedBy,
		&p.SupersededAt, &p.SupersededBy, &p.CreatedAt,
	); err != nil {
		return LoadingPlan{}, err
	}
	return p, nil
}

func mapLoadingPlanPgError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		// uq_lp_container_hash_active — BR-D10 dup of currently-active hash.
		return domain.NewBizError(domain.ErrInvalidInput, "loading plan with this Excel hash is already active for the container")
	case "23503":
		switch pgErr.ConstraintName {
		case "loading_plans_container_id_fkey":
			return domain.NewBizError(domain.ErrInvalidInput, "container_id does not exist")
		case "loading_plan_lines_sku_id_fkey":
			return domain.NewBizError(domain.ErrInvalidInput, "sku_id does not exist")
		case "loading_plans_uploaded_by_fkey":
			return domain.NewBizError(domain.ErrInvalidInput, "uploaded_by user does not exist")
		}
		return domain.NewBizError(domain.ErrInvalidInput, "foreign key violation: "+pgErr.ConstraintName)
	}
	return err
}

func (s *pgStore) insertLoadingPlanWithLines(ctx context.Context, plan LoadingPlan, lines []LoadingPlanLine) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO loading_plans
		    (id, container_id, excel_file_url, excel_hash, parsed_at, uploaded_by,
		     status, version, notes, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		plan.ID, plan.ContainerID, plan.ExcelFileURL, plan.ExcelHash, plan.ParsedAt, plan.UploadedBy,
		plan.Status, plan.Version, plan.Notes, plan.CreatedAt,
	); err != nil {
		return mapLoadingPlanPgError(err)
	}

	if len(lines) > 0 {
		batch := &pgx.Batch{}
		for _, l := range lines {
			batch.Queue(
				`INSERT INTO loading_plan_lines
				    (id, loading_plan_id, sku_id, qty_planned_pieces, unit_in_excel,
				     qty_in_excel, customer_sku_code, raw_excel_row, excel_row_num, created_at)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
				l.ID, plan.ID, l.SKUID, l.QtyPlannedPieces, l.UnitInExcel,
				l.QtyInExcel, l.CustomerSKUCode, l.RawExcelRow, l.ExcelRowNum, l.CreatedAt,
			)
		}
		br := tx.SendBatch(ctx, batch)
		for range lines {
			if _, err := br.Exec(); err != nil {
				_ = br.Close()
				return mapLoadingPlanPgError(err)
			}
		}
		if err := br.Close(); err != nil {
			return mapLoadingPlanPgError(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return mapLoadingPlanPgError(err)
	}
	return nil
}

func (s *pgStore) selectLoadingPlanByID(ctx context.Context, id uuid.UUID) (LoadingPlan, error) {
	row := s.pool.QueryRow(ctx, selectLoadingPlanCols+` WHERE id = $1`, id)
	p, err := scanLoadingPlan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoadingPlan{}, domain.NewBizError(domain.ErrNotFound, "loading plan not found")
	}
	return p, err
}

func (s *pgStore) selectLoadingPlanLines(ctx context.Context, planID uuid.UUID) ([]LoadingPlanLine, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, loading_plan_id, sku_id, qty_planned_pieces, unit_in_excel,
		        qty_in_excel, customer_sku_code, raw_excel_row, excel_row_num, created_at
		   FROM loading_plan_lines
		  WHERE loading_plan_id = $1
		  ORDER BY excel_row_num ASC`,
		planID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LoadingPlanLine, 0)
	for rows.Next() {
		var l LoadingPlanLine
		if err := rows.Scan(
			&l.ID, &l.LoadingPlanID, &l.SKUID, &l.QtyPlannedPieces, &l.UnitInExcel,
			&l.QtyInExcel, &l.CustomerSKUCode, &l.RawExcelRow, &l.ExcelRowNum, &l.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *pgStore) selectActiveLoadingPlan(ctx context.Context, containerID uuid.UUID) (LoadingPlan, error) {
	row := s.pool.QueryRow(ctx,
		selectLoadingPlanCols+`
		 WHERE container_id = $1 AND status <> 'SUPERSEDED'
		 ORDER BY version DESC
		 LIMIT 1`,
		containerID,
	)
	p, err := scanLoadingPlan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoadingPlan{}, domain.NewBizError(domain.ErrNotFound, "no active loading plan for container")
	}
	return p, err
}

func (s *pgStore) nextLoadingPlanVersion(ctx context.Context, containerID uuid.UUID) (int, error) {
	var next int
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) + 1 FROM loading_plans WHERE container_id = $1`,
		containerID,
	).Scan(&next)
	if err != nil {
		return 0, err
	}
	return next, nil
}

func (s *pgStore) approveLoadingPlanTx(ctx context.Context, planID, actorID uuid.UUID, now time.Time) (LoadingPlan, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LoadingPlan{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock current row to keep two concurrent approves from racing.
	row := tx.QueryRow(ctx,
		selectLoadingPlanCols+` WHERE id = $1 FOR UPDATE`,
		planID,
	)
	current, err := scanLoadingPlan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoadingPlan{}, domain.NewBizError(domain.ErrNotFound, "loading plan not found")
	}
	if err != nil {
		return LoadingPlan{}, err
	}

	// Idempotent on already-APPROVED.
	if current.Status == LoadingPlanStatusApproved {
		if err := tx.Commit(ctx); err != nil {
			return LoadingPlan{}, err
		}
		return current, nil
	}
	if current.Status == LoadingPlanStatusSuperseded {
		return LoadingPlan{}, domain.NewBizError(domain.ErrInvalidTransition, "cannot approve a superseded plan")
	}

	// SUPERSEDE every other non-superseded plan for the same container.
	if _, err := tx.Exec(ctx,
		`UPDATE loading_plans
		    SET status        = 'SUPERSEDED',
		        superseded_at = $1,
		        superseded_by = $2
		  WHERE container_id  = $3
		    AND id            <> $2
		    AND status        <> 'SUPERSEDED'`,
		now, planID, current.ContainerID,
	); err != nil {
		return LoadingPlan{}, err
	}

	// Promote the named plan to APPROVED.
	row = tx.QueryRow(ctx,
		`UPDATE loading_plans
		    SET status      = 'APPROVED',
		        approved_at = $1,
		        approved_by = $2
		  WHERE id          = $3
		    AND status      IN ('PARSED','VALIDATED')
		 RETURNING id, container_id, excel_file_url, excel_hash, parsed_at, uploaded_by,
		           status, version, notes, approved_at, approved_by,
		           superseded_at, superseded_by, created_at`,
		now, actorID, planID,
	)
	updated, err := scanLoadingPlan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoadingPlan{}, domain.NewBizError(domain.ErrInvalidTransition, "loading plan is not in PARSED or VALIDATED status")
	}
	if err != nil {
		return LoadingPlan{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return LoadingPlan{}, err
	}
	return updated, nil
}
