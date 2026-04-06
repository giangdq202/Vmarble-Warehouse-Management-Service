package production

import (
	"context"
	"errors"
	"time"

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

// scanWorkOrder reads the 8-column projection used by all SELECT queries.
// Columns: id, plan_id, sku_id, quantity, status, assigned_to, assigned_at, created_at
func scanWorkOrder(row interface {
	Scan(...any) error
}) (WorkOrder, error) {
	var wo WorkOrder
	err := row.Scan(
		&wo.ID, &wo.PlanID, &wo.SKUID, &wo.Quantity, &wo.Status,
		&wo.AssignedTo, &wo.AssignedAt, &wo.CreatedAt,
	)
	return wo, err
}

const selectWOCols = `id, plan_id, sku_id, quantity, status, assigned_to, assigned_at, created_at`

func (s *pgStore) selectWorkOrders(ctx context.Context) ([]WorkOrder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+selectWOCols+` FROM work_orders ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorkOrder
	for rows.Next() {
		wo, err := scanWorkOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wo)
	}
	return out, rows.Err()
}

func (s *pgStore) selectWorkOrderByID(ctx context.Context, id uuid.UUID) (WorkOrder, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+selectWOCols+` FROM work_orders WHERE id = $1`,
		id,
	)
	wo, err := scanWorkOrder(row)
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
		`SELECT `+selectWOCols+` FROM work_orders WHERE plan_id = $1 ORDER BY created_at`,
		planID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorkOrder
	for rows.Next() {
		wo, err := scanWorkOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wo)
	}
	return out, rows.Err()
}

func (s *pgStore) selectWorkOrdersByAssignee(ctx context.Context, userID uuid.UUID) ([]WorkOrder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+selectWOCols+` FROM work_orders WHERE assigned_to = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WorkOrder
	for rows.Next() {
		wo, err := scanWorkOrder(rows)
		if err != nil {
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

func (s *pgStore) updateWorkOrderAssignment(ctx context.Context, woID uuid.UUID, userID uuid.UUID, assignedAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE work_orders SET assigned_to = $1, assigned_at = $2 WHERE id = $3`,
		userID, assignedAt, woID,
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

// selectInCuttingCountByUser returns a map of userID → number of WOs with status IN_CUTTING
// assigned to that user. Users with zero WOs in cutting are not included in the map.
func (s *pgStore) selectInCuttingCountByUser(ctx context.Context) (map[uuid.UUID]int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT assigned_to, COUNT(*)
		 FROM work_orders
		 WHERE status = 'IN_CUTTING' AND assigned_to IS NOT NULL
		 GROUP BY assigned_to`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID]int)
	for rows.Next() {
		var userID uuid.UUID
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, err
		}
		result[userID] = count
	}
	return result, rows.Err()
}

// selectCNCUserIDs returns the IDs of all users with role 'cnc'.
// The production store queries the users table directly because it shares the
// same database; this avoids cross-module imports while staying within the
// architectural rules (no module package imports across boundaries).
func (s *pgStore) selectCNCUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM users WHERE role = 'cnc' ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
