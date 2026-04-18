package production

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

func (s *pgStore) insertWorkOrder(ctx context.Context, wo WorkOrder) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO work_orders (id, plan_id, sku_id, quantity, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		wo.ID, wo.PlanID, wo.SKUID, wo.Quantity, wo.Status, wo.CreatedAt,
	)
	return err
}

// scanWorkOrder reads the 12-column projection used by all SELECT queries.
// Columns: wo.id, wo.plan_id, wo.sku_id, s.code, s.name, s.length_mm, s.width_mm,
//          wo.quantity, wo.status, wo.assigned_to, wo.assigned_at, wo.created_at
func scanWorkOrder(row interface {
	Scan(...any) error
}) (WorkOrder, error) {
	var wo WorkOrder
	err := row.Scan(
		&wo.ID, &wo.PlanID, &wo.SKUID, &wo.SKUCode, &wo.SKUName,
		&wo.SKUDimensions.LengthMM, &wo.SKUDimensions.WidthMM,
		&wo.Quantity, &wo.Status, &wo.AssignedTo, &wo.AssignedAt, &wo.CreatedAt,
	)
	return wo, err
}

// selectWOCols is the common column list for all WorkOrder SELECT queries.
// Uses LEFT JOIN so that a WorkOrder whose SKU was deleted is not silently dropped.
const selectWOCols = `
	wo.id, wo.plan_id, wo.sku_id,
	COALESCE(s.code, '') AS sku_code,
	COALESCE(s.name, '') AS sku_name,
	COALESCE(s.length_mm, 0) AS sku_length_mm,
	COALESCE(s.width_mm, 0)  AS sku_width_mm,
	wo.quantity, wo.status, wo.assigned_to, wo.assigned_at, wo.created_at
FROM work_orders wo
LEFT JOIN skus s ON s.id = wo.sku_id`

func (s *pgStore) selectWorkOrdersPaged(ctx context.Context, p httpkit.PageParams, status string, planID *uuid.UUID) ([]WorkOrder, int, error) {
	sortCol := "created_at"
	if p.SortBy == "status" {
		sortCol = "status"
	}
	orderDir := "DESC"
	if p.Order == "asc" {
		orderDir = "ASC"
	}

	// planIDArg is passed as a UUID pointer; when nil the WHERE clause ($2::uuid IS NULL OR ...) is always true.
	planIDArg := planID

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM work_orders
		 WHERE ($1::text = '' OR status = $1)
		   AND ($2::uuid IS NULL OR plan_id = $2)`,
		status, planIDArg,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(
		`SELECT `+selectWOCols+`
		 WHERE ($1::text = '' OR wo.status = $1)
		   AND ($2::uuid IS NULL OR wo.plan_id = $2)
		 ORDER BY wo.%s %s
		 LIMIT $3 OFFSET $4`,
		sortCol, orderDir,
	)
	rows, err := s.pool.Query(ctx, query, status, planIDArg, p.Limit, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []WorkOrder
	for rows.Next() {
		wo, err := scanWorkOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, wo)
	}
	return out, total, rows.Err()
}

func (s *pgStore) selectWorkOrderByID(ctx context.Context, id uuid.UUID) (WorkOrder, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+selectWOCols+` WHERE wo.id = $1`,
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
		`SELECT `+selectWOCols+` WHERE wo.plan_id = $1 ORDER BY wo.created_at`,
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
		`SELECT `+selectWOCols+` WHERE wo.assigned_to = $1 ORDER BY wo.created_at DESC`,
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
	tag, err := s.pool.Exec(ctx,
		`UPDATE work_orders SET assigned_to = $1, assigned_at = $2
		 WHERE id = $3 AND assigned_to IS NULL`,
		userID, assignedAt, woID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrPreconditionFailed, "work order is already assigned to another operator")
	}
	return nil
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
