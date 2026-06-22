package production

import (
	"context"
	"database/sql"
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
		`INSERT INTO work_orders (id, plan_id, sku_id, quantity, status, sales_order_line_id, parent_wo_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		wo.ID, wo.PlanID, wo.SKUID, wo.Quantity, wo.Status, wo.SalesOrderLineID, wo.ParentWOID, wo.CreatedAt,
	)
	return err
}

// scanWorkOrder reads the standard projection used by all SELECT queries.
// Columns must match selectWOCols below.
func scanWorkOrder(row interface {
	Scan(...any) error
}) (WorkOrder, error) {
	var wo WorkOrder
	var estimatedHours sql.NullFloat64
	var machineSlotID, salesOrderLineID, parentWOID uuid.NullUUID
	var actualQty sql.NullInt32
	var shortfallReason, qcStatus sql.NullString
	err := row.Scan(
		&wo.ID, &wo.PlanID, &wo.SKUID, &wo.SKUCode, &wo.SKUName,
		&wo.SKUDimensions.LengthMM, &wo.SKUDimensions.WidthMM,
		&wo.Quantity, &wo.Status, &wo.AssignedTo, &wo.AssignedAt, &wo.CreatedAt,
		&estimatedHours, &machineSlotID, &salesOrderLineID,
		&actualQty, &parentWOID, &shortfallReason, &wo.PriorityBoost,
		&qcStatus,
	)
	if estimatedHours.Valid {
		wo.EstimatedHours = &estimatedHours.Float64
	}
	if machineSlotID.Valid {
		v := machineSlotID.UUID
		wo.MachineSlotID = &v
	}
	if salesOrderLineID.Valid {
		v := salesOrderLineID.UUID
		wo.SalesOrderLineID = &v
	}
	if actualQty.Valid {
		v := int(actualQty.Int32)
		wo.ActualQty = &v
	}
	if parentWOID.Valid {
		v := parentWOID.UUID
		wo.ParentWOID = &v
	}
	if shortfallReason.Valid {
		v := shortfallReason.String
		wo.ShortfallReason = &v
	}
	if qcStatus.Valid {
		v := qcStatus.String
		wo.QCStatus = &v
	}
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
	wo.quantity, wo.status, wo.assigned_to, wo.assigned_at, wo.created_at,
	wo.estimated_hours, wo.machine_slot_id, wo.sales_order_line_id,
	wo.actual_qty, wo.parent_wo_id, wo.shortfall_reason, wo.priority_boost,
	wo.qc_status
FROM work_orders wo
LEFT JOIN skus s ON s.id = wo.sku_id`

func (s *pgStore) selectWorkOrdersPaged(ctx context.Context, p httpkit.PageParams, f WorkOrderListFilter) ([]WorkOrder, int, error) {
	if f.DashboardPreset {
		return s.selectWorkOrdersDashboard(ctx, p, f)
	}

	sortCol := "created_at"
	if p.SortBy == "status" {
		sortCol = "status"
	}
	orderDir := "DESC"
	if p.Order == "asc" {
		orderDir = "ASC"
	}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM work_orders
		 WHERE ($1::text = '' OR status = $1)
		   AND ($2::uuid IS NULL OR plan_id = $2)
		   AND ($3::timestamptz IS NULL OR created_at >= $3)
		   AND ($4::timestamptz IS NULL OR created_at < $4)
		   AND (NOT $5::boolean OR assigned_to IS NULL)
		   AND ($6::uuid IS NULL OR assigned_to = $6)`,
		f.Status, f.PlanID, f.CreatedFrom, f.CreatedTo, f.AssignedNull, f.AssignedTo,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(
		`SELECT `+selectWOCols+`
		 WHERE ($1::text = '' OR wo.status = $1)
		   AND ($2::uuid IS NULL OR wo.plan_id = $2)
		   AND ($3::timestamptz IS NULL OR wo.created_at >= $3)
		   AND ($4::timestamptz IS NULL OR wo.created_at < $4)
		   AND (NOT $5::boolean OR wo.assigned_to IS NULL)
		   AND ($6::uuid IS NULL OR wo.assigned_to = $6)
		 ORDER BY wo.%s %s
		 LIMIT $7 OFFSET $8`,
		sortCol, orderDir,
	)
	rows, err := s.pool.Query(ctx, query,
		f.Status, f.PlanID, f.CreatedFrom, f.CreatedTo, f.AssignedNull, f.AssignedTo,
		p.Limit, p.Offset(),
	)
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

// selectWorkOrdersDashboard implements the operational queue preset:
//
//	bucket 0 — PLANNED created today (Asia/Ho_Chi_Minh)
//	bucket 1 — PLANNED created yesterday
//	bucket 2 — IN_CUTTING or IN_PROCESSING (active)
//	bucket 3 — PLANNED created before yesterday
//
// COMPLETED and COSTED records are excluded.
// Within each bucket records are ordered by created_at ASC (FIFO).
func (s *pgStore) selectWorkOrdersDashboard(ctx context.Context, p httpkit.PageParams, f WorkOrderListFilter) ([]WorkOrder, int, error) {
	yesterdayStart := f.TodayStart.AddDate(0, 0, -1)

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM work_orders
		 WHERE status NOT IN ('COMPLETED', 'COSTED')
		   AND ($1::uuid IS NULL OR plan_id = $1)`,
		f.PlanID,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	// $1=plan_id $2=today_start $3=today_end $4=yesterday_start $5=limit $6=offset
	const dashboardQuery = `SELECT ` + selectWOCols + `
		 WHERE wo.status NOT IN ('COMPLETED', 'COSTED')
		   AND ($1::uuid IS NULL OR wo.plan_id = $1)
		 ORDER BY
		   CASE
		     WHEN wo.status = 'PLANNED' AND wo.created_at >= $2 AND wo.created_at < $3 THEN 0
		     WHEN wo.status = 'PLANNED' AND wo.created_at >= $4 AND wo.created_at < $2 THEN 1
		     WHEN wo.status IN ('IN_CUTTING', 'IN_PROCESSING')                         THEN 2
		     ELSE 3
		   END ASC,
		   wo.created_at ASC
		 LIMIT $5 OFFSET $6`

	rows, err := s.pool.Query(ctx, dashboardQuery,
		f.PlanID, f.TodayStart, f.TodayEnd, yesterdayStart, p.Limit, p.Offset(),
	)
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

func (s *pgStore) selectWorkOrdersKeyset(ctx context.Context, f WorkOrderListFilter, cur httpkit.Cursor, limit int) ([]WorkOrder, error) {
	args := []any{f.Status, f.PlanID, f.CreatedFrom, f.CreatedTo, f.AssignedNull, f.AssignedTo}
	idx := 7

	q := `SELECT ` + selectWOCols + `
		 WHERE ($1::text = '' OR wo.status = $1)
		   AND ($2::uuid IS NULL OR wo.plan_id = $2)
		   AND ($3::timestamptz IS NULL OR wo.created_at >= $3)
		   AND ($4::timestamptz IS NULL OR wo.created_at < $4)
		   AND (NOT $5::boolean OR wo.assigned_to IS NULL)
		   AND ($6::uuid IS NULL OR wo.assigned_to = $6)`

	if !cur.IsZero() {
		q += fmt.Sprintf(" AND (wo.created_at, wo.id) < ($%d, $%d)", idx, idx+1)
		args = append(args, cur.Ts, cur.ID)
		idx += 2
	}

	q += fmt.Sprintf(" ORDER BY wo.created_at DESC, wo.id DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, q, args...)
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
		 WHERE id = $3 AND status = 'PLANNED'`,
		userID, assignedAt, woID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrPreconditionFailed, "work order has already started cutting and cannot be reassigned")
	}
	return nil
}

func (s *pgStore) reassignWorkOrderAtomically(ctx context.Context, op reassignOp) (WorkOrder, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return WorkOrder{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var wo WorkOrder
	row := tx.QueryRow(ctx,
		`SELECT `+selectWOCols+` WHERE wo.id = $1 FOR UPDATE`,
		op.WorkOrderID,
	)
	wo, err = scanWorkOrder(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkOrder{}, domain.ErrNotFound
		}
		return WorkOrder{}, err
	}
	if wo.Status != domain.WOInCutting && wo.Status != domain.WOInProcessing {
		return WorkOrder{}, domain.NewBizError(domain.ErrPreconditionFailed,
			"work order must be IN_CUTTING or IN_PROCESSING to be reassigned")
	}

	_, err = tx.Exec(ctx,
		`UPDATE work_orders SET assigned_to = $1, assigned_at = $2 WHERE id = $3`,
		op.NewUserID, op.AssignedAt, op.WorkOrderID,
	)
	if err != nil {
		return WorkOrder{}, err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO wo_reassign_log (id, work_order_id, from_user_id, to_user_id, reason, actor_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		op.LogID, op.WorkOrderID, wo.AssignedTo, op.NewUserID, op.Reason, op.ActorID, op.AssignedAt,
	)
	if err != nil {
		return WorkOrder{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WorkOrder{}, err
	}

	wo.AssignedTo = &op.NewUserID
	wo.AssignedAt = &op.AssignedAt
	return wo, nil
}

func (s *pgStore) claimWorkOrderAtomically(ctx context.Context, op claimOp) (WorkOrder, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return WorkOrder{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var wo WorkOrder
	row := tx.QueryRow(ctx,
		`SELECT `+selectWOCols+` WHERE wo.id = $1 FOR UPDATE`,
		op.WorkOrderID,
	)
	wo, err = scanWorkOrder(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkOrder{}, domain.ErrNotFound
		}
		return WorkOrder{}, err
	}
	if wo.Status != domain.WOPlanned {
		return WorkOrder{}, domain.NewBizError(domain.ErrPreconditionFailed,
			"work order must be PLANNED to be claimed")
	}
	if wo.AssignedTo != nil {
		return WorkOrder{}, domain.NewBizError(domain.ErrPreconditionFailed,
			"work order is already claimed by another operator")
	}

	_, err = tx.Exec(ctx,
		`UPDATE work_orders SET assigned_to = $1, assigned_at = $2 WHERE id = $3`,
		op.UserID, op.AssignedAt, op.WorkOrderID,
	)
	if err != nil {
		return WorkOrder{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WorkOrder{}, err
	}

	wo.AssignedTo = &op.UserID
	wo.AssignedAt = &op.AssignedAt
	return wo, nil
}

func (s *pgStore) updateWorkOrderQCStatus(ctx context.Context, woID uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE work_orders SET qc_status = $1 WHERE id = $2`,
		status, woID,
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

// --- Machine CRUD ---

func (s *pgStore) insertMachine(ctx context.Context, m Machine) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO machines (id, code, name, capacity_hours_per_shift, is_active, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		m.ID, m.Code, m.Name, m.CapacityHoursPerShift, m.IsActive, m.CreatedAt,
	)
	return err
}

func (s *pgStore) selectMachines(ctx context.Context) ([]Machine, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, name, capacity_hours_per_shift, is_active, created_at
		 FROM machines ORDER BY code`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Machine
	for rows.Next() {
		var m Machine
		if err := rows.Scan(&m.ID, &m.Code, &m.Name, &m.CapacityHoursPerShift, &m.IsActive, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *pgStore) selectMachineByID(ctx context.Context, id uuid.UUID) (Machine, error) {
	var m Machine
	err := s.pool.QueryRow(ctx,
		`SELECT id, code, name, capacity_hours_per_shift, is_active, created_at
		 FROM machines WHERE id = $1`,
		id,
	).Scan(&m.ID, &m.Code, &m.Name, &m.CapacityHoursPerShift, &m.IsActive, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Machine{}, domain.NewBizError(domain.ErrNotFound, "machine not found")
		}
		return Machine{}, err
	}
	return m, nil
}

func (s *pgStore) deactivateMachine(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE machines SET is_active = FALSE WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "machine not found")
	}
	return nil
}

// --- Slot CRUD ---

// slotCols is the common SELECT for all slot queries. assigned_hours is computed via LEFT JOIN.
const slotCols = `
	s.id, s.machine_id, m.code, m.name, s.shift_date, s.shift_name,
	s.capacity_hours,
	COALESCE(SUM(wo.estimated_hours), 0) AS assigned_hours,
	s.created_at
FROM machine_shift_slots s
JOIN machines m ON m.id = s.machine_id
LEFT JOIN work_orders wo ON wo.machine_slot_id = s.id`

func scanSlot(row interface {
	Scan(...any) error
}) (MachineShiftSlot, error) {
	var sl MachineShiftSlot
	err := row.Scan(
		&sl.ID, &sl.MachineID, &sl.MachineCode, &sl.MachineName,
		&sl.ShiftDate, &sl.ShiftName, &sl.CapacityHours, &sl.AssignedHours, &sl.CreatedAt,
	)
	return sl, err
}

func (s *pgStore) insertSlot(ctx context.Context, sl MachineShiftSlot) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO machine_shift_slots (id, machine_id, shift_date, shift_name, capacity_hours, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		sl.ID, sl.MachineID, sl.ShiftDate, sl.ShiftName, sl.CapacityHours, sl.CreatedAt,
	)
	return err
}

func (s *pgStore) selectSlotByID(ctx context.Context, id uuid.UUID) (MachineShiftSlot, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+slotCols+`
		 WHERE s.id = $1
		 GROUP BY s.id, m.code, m.name`,
		id,
	)
	sl, err := scanSlot(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MachineShiftSlot{}, domain.NewBizError(domain.ErrNotFound, "machine shift slot not found")
		}
		return MachineShiftSlot{}, err
	}
	return sl, nil
}

func (s *pgStore) selectSlotsByMachine(ctx context.Context, machineID uuid.UUID, from, to time.Time) ([]MachineShiftSlot, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+slotCols+`
		 WHERE s.machine_id = $1 AND s.shift_date >= $2 AND s.shift_date <= $3
		 GROUP BY s.id, m.code, m.name
		 ORDER BY s.shift_date, s.shift_name`,
		machineID, from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MachineShiftSlot
	for rows.Next() {
		sl, err := scanSlot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sl)
	}
	return out, rows.Err()
}

func (s *pgStore) selectFutureSlotsWithCapacity(ctx context.Context, minAvailableHours float64) ([]MachineShiftSlot, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+slotCols+`
		 WHERE s.shift_date >= CURRENT_DATE AND m.is_active = TRUE
		 GROUP BY s.id, m.code, m.name
		 HAVING s.capacity_hours - COALESCE(SUM(wo.estimated_hours), 0) >= $1
		 ORDER BY s.shift_date ASC, (s.capacity_hours - COALESCE(SUM(wo.estimated_hours), 0)) DESC`,
		minAvailableHours,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MachineShiftSlot
	for rows.Next() {
		sl, err := scanSlot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sl)
	}
	return out, rows.Err()
}

func (s *pgStore) deleteSlot(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM machine_shift_slots WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "machine shift slot not found")
	}
	return nil
}

// --- Work order scheduling ---

func (s *pgStore) updateEstimatedHours(ctx context.Context, woID uuid.UUID, hours float64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE work_orders SET estimated_hours = $1 WHERE id = $2`,
		hours, woID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "work order not found")
	}
	return nil
}

func (s *pgStore) unassignWOFromSlot(ctx context.Context, woID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE work_orders SET machine_slot_id = NULL WHERE id = $1`,
		woID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.NewBizError(domain.ErrNotFound, "work order not found")
	}
	return nil
}

// assignSlotAtomically locks the slot row, validates remaining capacity,
// then sets machine_slot_id on the work order — all inside one transaction.
func (s *pgStore) assignSlotAtomically(ctx context.Context, op assignSlotOp) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Lock slot row and read its capacity.
	var capacityHours float64
	lockErr := tx.QueryRow(ctx,
		`SELECT capacity_hours FROM machine_shift_slots WHERE id = $1 FOR UPDATE`,
		op.SlotID,
	).Scan(&capacityHours)
	if errors.Is(lockErr, pgx.ErrNoRows) {
		err = domain.NewBizError(domain.ErrNotFound, "machine shift slot not found")
		return err
	}
	if lockErr != nil {
		err = fmt.Errorf("lock slot: %w", lockErr)
		return err
	}

	// Compute currently assigned hours under the lock.
	var assignedHours float64
	if scanErr := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(estimated_hours), 0)
		 FROM work_orders WHERE machine_slot_id = $1`,
		op.SlotID,
	).Scan(&assignedHours); scanErr != nil {
		err = fmt.Errorf("sum assigned hours: %w", scanErr)
		return err
	}

	available := capacityHours - assignedHours
	if op.EstimatedHours > available {
		err = domain.NewBizError(domain.ErrPreconditionFailed,
			fmt.Sprintf("slot has %.2f available hours but work order requires %.2f", available, op.EstimatedHours))
		return err
	}

	tag, execErr := tx.Exec(ctx,
		`UPDATE work_orders SET machine_slot_id = $1 WHERE id = $2`,
		op.SlotID, op.WorkOrderID,
	)
	if execErr != nil {
		err = fmt.Errorf("assign slot: %w", execErr)
		return err
	}
	if tag.RowsAffected() == 0 {
		err = domain.NewBizError(domain.ErrNotFound, "work order not found")
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ── Labor cost entries ───────────────────────────────────────────────────────

func (s *pgStore) insertLaborEntry(ctx context.Context, e LaborEntry) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO labor_cost_entries
		   (id, work_order_id, stage, minutes, rate_per_hour, worker_id, actor_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ID, e.WorkOrderID, string(e.Stage), e.Minutes, e.RatePerHour, e.WorkerID, e.ActorID, e.CreatedAt,
	)
	return err
}

// COALESCE worker_id with actor_id so the API contract returns a non-null
// worker for every row, including rows created before migration 00039.
func (s *pgStore) selectLaborEntriesByWO(ctx context.Context, woID uuid.UUID) ([]LaborEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, work_order_id, stage, minutes, rate_per_hour,
		        COALESCE(worker_id, actor_id) AS worker_id, actor_id, created_at
		 FROM labor_cost_entries
		 WHERE work_order_id = $1
		 ORDER BY created_at ASC, id ASC`,
		woID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LaborEntry
	for rows.Next() {
		var e LaborEntry
		var stage string
		if scanErr := rows.Scan(&e.ID, &e.WorkOrderID, &stage, &e.Minutes, &e.RatePerHour, &e.WorkerID, &e.ActorID, &e.CreatedAt); scanErr != nil {
			return nil, scanErr
		}
		e.Stage = domain.LaborStage(stage)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *pgStore) sumLaborMinuteRateByWO(ctx context.Context, woID uuid.UUID) (int64, error) {
	var sum int64
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(minutes::bigint * rate_per_hour), 0)
		 FROM labor_cost_entries
		 WHERE work_order_id = $1`,
		woID,
	).Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

// listStatusesByPlan returns the current status of every work order tied to
// the plan. Used by the planning cascade-cancel precondition (#249).
func (s *pgStore) listStatusesByPlan(ctx context.Context, planID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT status FROM work_orders WHERE plan_id = $1`, planID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// cancelPlannedByPlan flips every PLANNED work order under the plan to
// CANCELED in one statement. Returns the number of rows updated. Bypasses
// the AdvanceStatus state machine deliberately — callers (planning cancel
// cascade) validate upstream that no WO has progressed past PLANNED.
func (s *pgStore) cancelPlannedByPlan(ctx context.Context, planID uuid.UUID) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE work_orders SET status = 'CANCELED'
		  WHERE plan_id = $1 AND status = 'PLANNED'`,
		planID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// partialCompleteAtomically locks the parent WO row, validates BR-P05 under
// the lock (status==IN_PROCESSING), flips status to PARTIAL_COMPLETE with
// actual_qty + shortfall_reason, and (optionally) inserts the carry-over WO
// — all inside one transaction. Two concurrent callers are serialized by the
// FOR UPDATE: the first commits, the second observes PARTIAL_COMPLETE and
// returns ErrInvalidTransition.
func (s *pgStore) partialCompleteAtomically(ctx context.Context, op partialCompleteOp) (WorkOrder, WorkOrder, error) {
	var parent, carry WorkOrder
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return parent, carry, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Lock + read the parent WO. We re-fetch through the standard projection
	// to keep the post-update WorkOrder shape consistent with selectWorkOrderByID.
	var status string
	if lockErr := tx.QueryRow(ctx,
		`SELECT status FROM work_orders WHERE id = $1 FOR UPDATE`, op.WorkOrderID,
	).Scan(&status); lockErr != nil {
		if errors.Is(lockErr, pgx.ErrNoRows) {
			err = domain.ErrNotFound
			return parent, carry, err
		}
		err = fmt.Errorf("lock work order: %w", lockErr)
		return parent, carry, err
	}
	if status != string(domain.WOInProcessing) {
		err = domain.NewBizError(domain.ErrInvalidTransition,
			fmt.Sprintf("partial-complete requires status IN_PROCESSING, current=%s", status))
		return parent, carry, err
	}

	if _, execErr := tx.Exec(ctx,
		`UPDATE work_orders
		    SET status = $1, actual_qty = $2, shortfall_reason = $3
		  WHERE id = $4`,
		string(domain.WOPartialComplete), op.ActualQty, op.ShortfallReason, op.WorkOrderID,
	); execErr != nil {
		err = fmt.Errorf("update wo partial-complete: %w", execErr)
		return parent, carry, err
	}

	if op.CarryOver {
		c := op.CarryOverWO
		if _, insErr := tx.Exec(ctx,
			`INSERT INTO work_orders (id, plan_id, sku_id, quantity, status, sales_order_line_id, parent_wo_id, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			c.ID, c.PlanID, c.SKUID, c.Quantity, c.Status, c.SalesOrderLineID, c.ParentWOID, c.CreatedAt,
		); insErr != nil {
			err = fmt.Errorf("insert carry-over wo: %w", insErr)
			return parent, carry, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return parent, carry, fmt.Errorf("commit: %w", err)
	}

	parent, err = s.selectWorkOrderByID(ctx, op.WorkOrderID)
	if err != nil {
		return parent, carry, err
	}
	if op.CarryOver {
		carry, err = s.selectWorkOrderByID(ctx, op.CarryOverWO.ID)
		if err != nil {
			return parent, carry, err
		}
	}
	return parent, carry, nil
}

func (s *pgStore) selectWOWithPlanDeadline(ctx context.Context, woID uuid.UUID) (woFeasibilityData, error) {
	var d woFeasibilityData
	var soCode sql.NullString
	err := s.pool.QueryRow(ctx, `
		SELECT wo.id, wo.quantity, wo.status, pp.deadline,
		       COALESCE(bc.material_id, '00000000-0000-0000-0000-000000000000'::uuid),
		       COALESCE(sol.code, '')
		FROM work_orders wo
		JOIN production_plans pp ON pp.id = wo.plan_id
		LEFT JOIN bom_components bc ON bc.sku_id = wo.sku_id AND bc.material_type = 'PLYWOOD'
		LEFT JOIN sales_order_lines sol ON sol.id = wo.sales_order_line_id
		WHERE wo.id = $1
		LIMIT 1`, woID,
	).Scan(&d.WOID, &d.Quantity, &d.Status, &d.Deadline, &d.MaterialID, &soCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, domain.ErrNotFound
	}
	if soCode.Valid {
		d.SOCode = soCode.String
	}
	return d, err
}

func (s *pgStore) selectFeasibilitySuggestions(ctx context.Context, materialID uuid.UUID, excludeWOID uuid.UUID, limit int) ([]woSuggestionRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT wo.id, COALESCE(sk.code,'') AS sku_code, wo.quantity, pp.deadline
		FROM work_orders wo
		JOIN production_plans pp ON pp.id = wo.plan_id
		JOIN bom_components bc ON bc.sku_id = wo.sku_id AND bc.material_type = 'PLYWOOD' AND bc.material_id = $1
		LEFT JOIN skus sk ON sk.id = wo.sku_id
		WHERE wo.status = 'PLANNED' AND wo.id <> $2
		ORDER BY
		  CASE WHEN pp.deadline IS NULL THEN 1 ELSE 0 END,
		  pp.deadline ASC
		LIMIT $3`, materialID, excludeWOID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []woSuggestionRow
	for rows.Next() {
		var r woSuggestionRow
		if err := rows.Scan(&r.WOID, &r.SKUCode, &r.Quantity, &r.Deadline); err != nil {
			return nil, err
		}
		r.FreedQty = r.Quantity
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) setPriorityBoostAtomically(ctx context.Context, op setPriorityBoostOp) (uuid.UUID, time.Time, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx,
		`UPDATE work_orders SET priority_boost = true WHERE id = $1`, op.WOID,
	); err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("set priority_boost: %w", err)
	}

	auditID := uuid.New()
	now := time.Now()
	if _, err = tx.Exec(ctx,
		`INSERT INTO wo_boost_log (id, wo_id, reason, actor_id, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		auditID, op.WOID, op.Reason, op.ActorID, now,
	); err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("insert boost log: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("commit: %w", err)
	}
	return auditID, now, nil
}

func (s *pgStore) selectPreemptCandidates(ctx context.Context, woID uuid.UUID) ([]preemptCandidateRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT other.id, other.status, COALESCE(sol.code,''), pp.deadline, other.quantity
		FROM work_orders other
		JOIN production_plans pp ON pp.id = other.plan_id
		JOIN bom_components bc_other ON bc_other.sku_id = other.sku_id AND bc_other.material_type = 'PLYWOOD'
		JOIN bom_components bc_me   ON bc_me.sku_id = (SELECT sku_id FROM work_orders WHERE id = $1)
		                           AND bc_me.material_type = 'PLYWOOD'
		                           AND bc_me.material_id = bc_other.material_id
		LEFT JOIN sales_order_lines sol ON sol.id = other.sales_order_line_id
		WHERE other.status IN ('PLANNED','IN_CUTTING') AND other.id <> $1
		ORDER BY pp.deadline ASC NULLS LAST
		LIMIT 20`, woID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []preemptCandidateRow
	for rows.Next() {
		var r preemptCandidateRow
		if err := rows.Scan(&r.WOID, &r.Status, &r.SOCode, &r.Deadline, &r.Quantity); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) preemptAtomically(ctx context.Context, op preemptOp) (uuid.UUID, time.Time, int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, time.Time{}, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Lock both rows to prevent concurrent preemption.
	var fromStatus string
	var fromQty int
	if lockErr := tx.QueryRow(ctx,
		`SELECT status, quantity FROM work_orders WHERE id = $1 FOR UPDATE`, op.FromWOID,
	).Scan(&fromStatus, &fromQty); lockErr != nil {
		if errors.Is(lockErr, pgx.ErrNoRows) {
			err = domain.ErrNotFound
			return uuid.Nil, time.Time{}, 0, err
		}
		err = fmt.Errorf("lock from_wo: %w", lockErr)
		return uuid.Nil, time.Time{}, 0, err
	}
	// Lock to_wo too (avoids deadlock by consistent lock ordering handled by caller passing lower UUID first — good-enough for MVP).
	if _, lockErr := tx.Exec(ctx,
		`SELECT 1 FROM work_orders WHERE id = $1 FOR UPDATE`, op.ToWOID,
	); lockErr != nil {
		err = fmt.Errorf("lock to_wo: %w", lockErr)
		return uuid.Nil, time.Time{}, 0, err
	}

	// BR-PL07: refuse if from_wo progressed past IN_CUTTING.
	switch fromStatus {
	case string(domain.WOPlanned), string(domain.WOInCutting):
		// allowed
	default:
		err = domain.NewBizError(domain.ErrPreconditionFailed,
			fmt.Sprintf("cannot preempt work order in status %s: only PLANNED or IN_CUTTING is allowed", fromStatus))
		return uuid.Nil, time.Time{}, 0, err
	}

	if _, execErr := tx.Exec(ctx,
		`UPDATE work_orders SET status = $1 WHERE id = $2`,
		string(domain.WOPlanned), op.FromWOID,
	); execErr != nil {
		err = fmt.Errorf("revert from_wo: %w", execErr)
		return uuid.Nil, time.Time{}, 0, err
	}

	auditID := uuid.New()
	now := time.Now()
	if _, execErr := tx.Exec(ctx,
		`INSERT INTO wo_preemption_log (id, from_wo_id, to_wo_id, material_id, freed_qty, reason, actor_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		auditID, op.FromWOID, op.ToWOID, op.MaterialID, fromQty, op.Reason, op.ActorID, now,
	); execErr != nil {
		err = fmt.Errorf("insert preemption log: %w", execErr)
		return uuid.Nil, time.Time{}, 0, err
	}

	if err = tx.Commit(ctx); err != nil {
		return uuid.Nil, time.Time{}, 0, fmt.Errorf("commit: %w", err)
	}
	return auditID, now, fromQty, nil
}

// ── WO Blockers ──────────────────────────────────────────────────────────────

func (s *pgStore) insertBlocker(ctx context.Context, b WOBlocker) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO wo_blockers (id, work_order_id, reason, detail, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		b.ID, b.WorkOrderID, string(b.Reason), b.Detail, b.CreatedBy, b.CreatedAt,
	)
	return err
}

func (s *pgStore) getBlocker(ctx context.Context, blockerID uuid.UUID) (WOBlocker, error) {
	var b WOBlocker
	var resolvedBy *uuid.UUID
	var resolvedAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT id, work_order_id, reason, detail, created_by, created_at, resolved_by, resolved_at
		 FROM wo_blockers WHERE id = $1`, blockerID,
	).Scan(&b.ID, &b.WorkOrderID, &b.Reason, &b.Detail, &b.CreatedBy, &b.CreatedAt, &resolvedBy, &resolvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return WOBlocker{}, domain.NewBizError(domain.ErrNotFound, "blocker not found")
	}
	if err != nil {
		return WOBlocker{}, err
	}
	b.ResolvedBy = resolvedBy
	b.ResolvedAt = resolvedAt
	return b, nil
}

func (s *pgStore) resolveBlocker(ctx context.Context, blockerID, resolvedBy uuid.UUID, resolvedAt time.Time) (WOBlocker, error) {
	var b WOBlocker
	var rby *uuid.UUID
	var rat *time.Time
	err := s.pool.QueryRow(ctx,
		`UPDATE wo_blockers
		 SET resolved_by = $2, resolved_at = $3
		 WHERE id = $1 AND resolved_at IS NULL
		 RETURNING id, work_order_id, reason, detail, created_by, created_at, resolved_by, resolved_at`,
		blockerID, resolvedBy, resolvedAt,
	).Scan(&b.ID, &b.WorkOrderID, &b.Reason, &b.Detail, &b.CreatedBy, &b.CreatedAt, &rby, &rat)
	if errors.Is(err, pgx.ErrNoRows) {
		return WOBlocker{}, domain.NewBizError(domain.ErrPreconditionFailed, "blocker not found or already resolved")
	}
	if err != nil {
		return WOBlocker{}, err
	}
	b.ResolvedBy = rby
	b.ResolvedAt = rat
	return b, nil
}

func (s *pgStore) listBlockers(ctx context.Context, woID uuid.UUID) ([]WOBlocker, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, work_order_id, reason, detail, created_by, created_at, resolved_by, resolved_at
		 FROM wo_blockers WHERE work_order_id = $1
		 ORDER BY created_at ASC`, woID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WOBlocker
	for rows.Next() {
		var b WOBlocker
		var rby *uuid.UUID
		var rat *time.Time
		if err := rows.Scan(&b.ID, &b.WorkOrderID, &b.Reason, &b.Detail, &b.CreatedBy, &b.CreatedAt, &rby, &rat); err != nil {
			return nil, err
		}
		b.ResolvedBy = rby
		b.ResolvedAt = rat
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *pgStore) countOpenBlockers(ctx context.Context, woID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wo_blockers WHERE work_order_id = $1 AND resolved_at IS NULL`, woID,
	).Scan(&n)
	return n, err
}
