package planning

import (
	"context"
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

// nextPlanCode draws the next value from the shared sequence and formats it as
// KH-{year}-{seq padded to 3 digits}, e.g. KH-2026-001.
func (s *pgStore) nextPlanCode(ctx context.Context, year int) (string, error) {
	var seq int64
	if err := s.pool.QueryRow(ctx, `SELECT nextval('production_plan_code_seq')`).Scan(&seq); err != nil {
		return "", err
	}
	return fmt.Sprintf("KH-%d-%03d", year, seq), nil
}

func (s *pgStore) insertPlan(ctx context.Context, p Plan) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO production_plans (id, code, po_id, status, deadline, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		p.ID, p.Code, p.POID, p.Status, p.Deadline, p.CreatedAt,
	)
	return err
}

func (s *pgStore) selectPlansPaged(ctx context.Context, p httpkit.PageParams, status string) ([]Plan, int, error) {
	sortCol := "pp.created_at"
	if p.SortBy == "deadline" {
		sortCol = "pp.deadline"
	}
	orderDir := "DESC"
	if p.Order == "asc" {
		orderDir = "ASC"
	}
	search := "%" + p.Search + "%"

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		   FROM production_plans pp
		   JOIN purchase_orders po ON po.id = pp.po_id
		  WHERE ($1::text = '' OR pp.status = $1)
		    AND ($2::text = '' OR pp.code ILIKE $2 OR po.code ILIKE $2)`,
		status, search,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(
		`SELECT pp.id, pp.code, pp.po_id, po.code AS po_code, pp.status, pp.deadline, pp.created_at
		   FROM production_plans pp
		   JOIN purchase_orders po ON po.id = pp.po_id
		  WHERE ($1::text = '' OR pp.status = $1)
		    AND ($2::text = '' OR pp.code ILIKE $2 OR po.code ILIKE $2)
		  ORDER BY CASE WHEN pp.status = 'APPROVED' THEN 0 ELSE 1 END,
		           CASE WHEN pp.deadline IS NULL THEN 1 ELSE 0 END,
		           %s %s
		 LIMIT $3 OFFSET $4`,
		sortCol, orderDir,
	)
	rows, err := s.pool.Query(ctx, query, status, search, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var plan Plan
		if err := rows.Scan(&plan.ID, &plan.Code, &plan.POID, &plan.POCode, &plan.Status, &plan.Deadline, &plan.CreatedAt); err != nil {
			return nil, 0, err
		}
		plans = append(plans, plan)
	}
	return plans, total, rows.Err()
}

func (s *pgStore) selectPlanByID(ctx context.Context, id uuid.UUID) (Plan, error) {
	var p Plan
	err := s.pool.QueryRow(ctx,
		`SELECT pp.id, pp.code, pp.po_id, po.code AS po_code, pp.status, pp.deadline, pp.created_at
		   FROM production_plans pp
		   JOIN purchase_orders po ON po.id = pp.po_id
		  WHERE pp.id = $1`,
		id,
	).Scan(&p.ID, &p.Code, &p.POID, &p.POCode, &p.Status, &p.Deadline, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Plan{}, domain.ErrNotFound
		}
		return Plan{}, err
	}
	return p, nil
}

func (s *pgStore) updatePlanStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE production_plans SET status = $1 WHERE id = $2`,
		status, id,
	)
	return err
}

func (s *pgStore) insertPlanItems(ctx context.Context, items []PlanItem) error {
	for _, item := range items {
		_, err := s.pool.Exec(ctx,
			`INSERT INTO plan_items (id, plan_id, sku_id, quantity)
			 VALUES ($1, $2, $3, $4)`,
			item.ID, item.PlanID, item.SKUID, item.Quantity,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *pgStore) selectPlanItemsByPlanID(ctx context.Context, planID uuid.UUID) ([]PlanItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, plan_id, sku_id, quantity
		 FROM plan_items WHERE plan_id = $1 ORDER BY id`,
		planID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PlanItem
	for rows.Next() {
		var item PlanItem
		if err := rows.Scan(&item.ID, &item.PlanID, &item.SKUID, &item.Quantity); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// selectPlansLookup returns a lightweight slice for the async combobox.
// Filters are all optional; when nil/empty they are skipped. Ordering favours
// APPROVED plans and nearest non-null deadline for the combobox default view.
func (s *pgStore) selectPlansLookup(ctx context.Context, search, status string, deadlineFrom, deadlineTo *time.Time, limit, offset int) ([]PlanLookupItem, int, error) {
	searchPat := "%" + search + "%"

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)
		   FROM production_plans pp
		   JOIN purchase_orders po ON po.id = pp.po_id
		  WHERE ($1::text = '' OR pp.status = $1)
		    AND ($2::text = '' OR pp.code ILIKE $2 OR po.code ILIKE $2)
		    AND ($3::date IS NULL OR pp.deadline >= $3)
		    AND ($4::date IS NULL OR pp.deadline <= $4)`,
		status, searchPat, deadlineFrom, deadlineTo,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT pp.id, pp.code, po.code AS po_code, pp.status, pp.deadline
		   FROM production_plans pp
		   JOIN purchase_orders po ON po.id = pp.po_id
		  WHERE ($1::text = '' OR pp.status = $1)
		    AND ($2::text = '' OR pp.code ILIKE $2 OR po.code ILIKE $2)
		    AND ($3::date IS NULL OR pp.deadline >= $3)
		    AND ($4::date IS NULL OR pp.deadline <= $4)
		  ORDER BY
		    CASE WHEN pp.status = 'APPROVED' THEN 0 ELSE 1 END,
		    CASE WHEN pp.deadline IS NULL THEN 1 ELSE 0 END,
		    pp.deadline ASC,
		    pp.created_at DESC
		 LIMIT $5 OFFSET $6`,
		status, searchPat, deadlineFrom, deadlineTo, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []PlanLookupItem
	for rows.Next() {
		var item PlanLookupItem
		if err := rows.Scan(&item.ID, &item.Code, &item.POCode, &item.Status, &item.Deadline); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
