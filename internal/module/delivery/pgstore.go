package delivery

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
