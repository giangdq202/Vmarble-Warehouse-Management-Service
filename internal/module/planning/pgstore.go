package planning

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

func (s *pgStore) insertPlan(ctx context.Context, p Plan) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO production_plans (id, po_id, status, deadline, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		p.ID, p.POID, p.Status, p.Deadline, p.CreatedAt,
	)
	return err
}

func (s *pgStore) selectPlans(ctx context.Context) ([]Plan, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, po_id, status, deadline, created_at FROM production_plans ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []Plan
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.POID, &p.Status, &p.Deadline, &p.CreatedAt); err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (s *pgStore) selectPlanByID(ctx context.Context, id uuid.UUID) (Plan, error) {
	var p Plan
	err := s.pool.QueryRow(ctx,
		`SELECT id, po_id, status, deadline, created_at FROM production_plans WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.POID, &p.Status, &p.Deadline, &p.CreatedAt)
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
