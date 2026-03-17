package production

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

func (s *pgStore) insertWorkOrder(ctx context.Context, wo WorkOrder) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO work_orders (id, plan_id, sku_id, quantity, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		wo.ID, wo.PlanID, wo.SKUID, wo.Quantity, wo.Status, wo.CreatedAt,
	)
	return err
}

func (s *pgStore) selectWorkOrders(ctx context.Context) ([]WorkOrder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, plan_id, sku_id, quantity, status, created_at FROM work_orders ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorkOrder
	for rows.Next() {
		var wo WorkOrder
		if err := rows.Scan(&wo.ID, &wo.PlanID, &wo.SKUID, &wo.Quantity, &wo.Status, &wo.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, wo)
	}
	return out, rows.Err()
}

func (s *pgStore) selectWorkOrderByID(ctx context.Context, id uuid.UUID) (WorkOrder, error) {
	var wo WorkOrder
	err := s.pool.QueryRow(ctx,
		`SELECT id, plan_id, sku_id, quantity, status, created_at FROM work_orders WHERE id = $1`,
		id,
	).Scan(&wo.ID, &wo.PlanID, &wo.SKUID, &wo.Quantity, &wo.Status, &wo.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkOrder{}, domain.ErrNotFound
		}
		return WorkOrder{}, err
	}
	return wo, nil
}

func (s *pgStore) selectWorkOrdersByPlan(ctx context.Context, planID uuid.UUID) ([]WorkOrder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, plan_id, sku_id, quantity, status, created_at FROM work_orders WHERE plan_id = $1 ORDER BY created_at`,
		planID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorkOrder
	for rows.Next() {
		var wo WorkOrder
		if err := rows.Scan(&wo.ID, &wo.PlanID, &wo.SKUID, &wo.Quantity, &wo.Status, &wo.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, wo)
	}
	return out, rows.Err()
}

func (s *pgStore) updateWorkOrderStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE work_orders SET status = $1 WHERE id = $2`,
		status, id,
	)
	return err
}

func (s *pgStore) insertConsumption(ctx context.Context, cr ConsumptionRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO consumption_records (id, work_order_id, material_id, material_type, quantity, unit, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		cr.ID, cr.WorkOrderID, cr.MaterialID, cr.MaterialType, cr.Quantity, cr.Unit, cr.CreatedAt,
	)
	return err
}

func (s *pgStore) selectConsumptionsByWO(ctx context.Context, woID uuid.UUID) ([]ConsumptionRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, work_order_id, material_id, material_type, quantity, unit, created_at
		 FROM consumption_records WHERE work_order_id = $1 ORDER BY created_at`,
		woID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ConsumptionRecord
	for rows.Next() {
		var cr ConsumptionRecord
		if err := rows.Scan(&cr.ID, &cr.WorkOrderID, &cr.MaterialID, &cr.MaterialType, &cr.Quantity, &cr.Unit, &cr.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, cr)
	}
	return out, rows.Err()
}

func (s *pgStore) hasMetalConsumption(ctx context.Context, woID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM consumption_records WHERE work_order_id = $1 AND material_type = 'METAL')`,
		woID,
	).Scan(&exists)
	return exists, err
}
