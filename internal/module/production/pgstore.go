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
		`INSERT INTO work_orders (id, plan_id, sku_id, quantity, status, sales_order_line_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		wo.ID, wo.PlanID, wo.SKUID, wo.Quantity, wo.Status, wo.SalesOrderLineID, wo.CreatedAt,
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
	var machineSlotID, salesOrderLineID uuid.NullUUID
	err := row.Scan(
		&wo.ID, &wo.PlanID, &wo.SKUID, &wo.SKUCode, &wo.SKUName,
		&wo.SKUDimensions.LengthMM, &wo.SKUDimensions.WidthMM,
		&wo.Quantity, &wo.Status, &wo.AssignedTo, &wo.AssignedAt, &wo.CreatedAt,
		&estimatedHours, &machineSlotID, &salesOrderLineID,
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
	wo.estimated_hours, wo.machine_slot_id, wo.sales_order_line_id
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
