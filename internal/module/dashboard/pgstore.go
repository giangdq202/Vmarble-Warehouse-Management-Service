package dashboard

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type pgStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) store {
	return &pgStore{pool: pool}
}

func (s *pgStore) selectRemnantKPIs(ctx context.Context) (RemnantKPIOutput, error) {
	var out RemnantKPIOutput
	err := s.pool.QueryRow(ctx,
		`SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE status = $1)::int,
			COUNT(*) FILTER (WHERE status = $2)::int,
			COUNT(*) FILTER (WHERE status = $3)::int,
			COUNT(*) FILTER (WHERE status = $4)::int
		 FROM remnants`,
		domain.RemnantAvailable,
		domain.RemnantAllocated,
		domain.RemnantConsumed,
		domain.RemnantWaste,
	).Scan(&out.Total, &out.Available, &out.Allocated, &out.Consumed, &out.Waste)
	if err != nil {
		return RemnantKPIOutput{}, err
	}
	return out, nil
}

func (s *pgStore) selectUtilizationPct(ctx context.Context) (float64, error) {
	var value float64
	err := s.pool.QueryRow(ctx,
		`WITH used AS (
			SELECT COALESCE(SUM((used_length_mm::numeric) * (used_width_mm::numeric)), 0) AS used_area
			FROM cutting_records
		), source AS (
			SELECT COALESCE(SUM((length_mm::numeric) * (width_mm::numeric)), 0) AS source_area
			FROM board_sheets
		)
		SELECT CASE
			WHEN source.source_area = 0 THEN 0
			ELSE ROUND((used.used_area / source.source_area) * 100, 2)
		END
		FROM used, source`,
	).Scan(&value)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func (s *pgStore) selectActiveWorkOrders(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int
		 FROM work_orders
		 WHERE status IN ($1, $2, $3)`,
		domain.WOPlanned,
		domain.WOInCutting,
		domain.WOInProcessing,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *pgStore) selectPendingCosting(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int
		 FROM work_orders
		 WHERE status = $1`,
		domain.WOCompleted,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *pgStore) selectRemnantTrend7D(ctx context.Context) ([]RemnantTrendPoint, error) {
	rows, err := s.pool.Query(ctx,
		`WITH days AS (
			SELECT generate_series(
				date_trunc('day', now()) - INTERVAL '6 day',
				date_trunc('day', now()),
				INTERVAL '1 day'
			)::date AS d
		), stats AS (
			SELECT
				date(created_at) AS d,
				status,
				COUNT(*)::int AS c
			FROM remnants
			WHERE created_at >= date_trunc('day', now()) - INTERVAL '6 day'
			GROUP BY date(created_at), status
		)
		SELECT
			to_char(days.d, 'YYYY-MM-DD') AS day,
			COALESCE(SUM(stats.c) FILTER (WHERE stats.status = $1), 0)::int AS available,
			COALESCE(SUM(stats.c) FILTER (WHERE stats.status = $2), 0)::int AS allocated,
			COALESCE(SUM(stats.c) FILTER (WHERE stats.status = $3), 0)::int AS waste
		FROM days
		LEFT JOIN stats ON stats.d = days.d
		GROUP BY days.d
		ORDER BY days.d`,
		domain.RemnantAvailable,
		domain.RemnantAllocated,
		domain.RemnantWaste,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RemnantTrendPoint, 0, 7)
	for rows.Next() {
		var item RemnantTrendPoint
		if err := rows.Scan(&item.Date, &item.Available, &item.Allocated, &item.Waste); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *pgStore) selectCostAllocation(ctx context.Context, limit int) ([]CostAllocationItem, error) {
	if limit <= 0 {
		return []CostAllocationItem{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT
			COALESCE(sku.code, '') AS sku_code,
			COALESCE(SUM(cr.total_cost_amount), 0)::bigint AS cost
		FROM costing_records cr
		LEFT JOIN work_orders wo ON wo.id = cr.work_order_id
		LEFT JOIN skus sku ON sku.id = wo.sku_id
		GROUP BY sku.code
		ORDER BY cost DESC, sku_code ASC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]CostAllocationItem, 0)
	for rows.Next() {
		var item CostAllocationItem
		if err := rows.Scan(&item.SKUCode, &item.Cost); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *pgStore) selectMaterialUsage7D(ctx context.Context) ([]MaterialUsagePoint, error) {
	rows, err := s.pool.Query(ctx,
		`WITH days AS (
			SELECT generate_series(
				date_trunc('day', now()) - INTERVAL '6 day',
				date_trunc('day', now()),
				INTERVAL '1 day'
			)::date AS d
		), usage_by_day AS (
			SELECT
				date(created_at) AS d,
				SUM(CASE WHEN material_type = 'PLYWOOD' THEN quantity ELSE 0 END)::float8 AS plywood,
				SUM(CASE WHEN material_type = 'METAL' THEN quantity ELSE 0 END)::float8 AS metal,
				SUM(CASE WHEN material_type NOT IN ('PLYWOOD', 'METAL') THEN quantity ELSE 0 END)::float8 AS accessory
			FROM consumption_records
			WHERE created_at >= date_trunc('day', now()) - INTERVAL '6 day'
			GROUP BY date(created_at)
		)
		SELECT
			to_char(days.d, 'YYYY-MM-DD') AS day,
			COALESCE(usage_by_day.plywood, 0),
			COALESCE(usage_by_day.metal, 0),
			COALESCE(usage_by_day.accessory, 0)
		FROM days
		LEFT JOIN usage_by_day ON usage_by_day.d = days.d
		ORDER BY days.d`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]MaterialUsagePoint, 0, 7)
	for rows.Next() {
		var item MaterialUsagePoint
		if err := rows.Scan(&item.Date, &item.Plywood, &item.Metal, &item.Accessory); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *pgStore) selectRecentCuts(ctx context.Context, limit int) ([]RecentCutItem, error) {
	if limit <= 0 {
		return []RecentCutItem{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT
			cr.id,
			cr.work_order_id,
			cr.sku_id,
			COALESCE(sku.code, '') AS sku_code,
			cr.created_at
		FROM cutting_records cr
		LEFT JOIN skus sku ON sku.id = cr.sku_id
		ORDER BY cr.created_at DESC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RecentCutItem, 0)
	for rows.Next() {
		var item RecentCutItem
		if err := rows.Scan(&item.ID, &item.WorkOrderID, &item.SKUID, &item.SKUCode, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *pgStore) selectCompletedWorkOrders(ctx context.Context, limit int) ([]RecentWorkOrderItem, error) {
	if limit <= 0 {
		return []RecentWorkOrderItem{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT
			wo.id,
			COALESCE(sku.code, '') AS sku_code,
			wo.status,
			wo.created_at
		FROM work_orders wo
		LEFT JOIN skus sku ON sku.id = wo.sku_id
		WHERE wo.status IN ($1, $2)
		ORDER BY wo.created_at DESC
		LIMIT $3`,
		domain.WOCompleted,
		domain.WOCosted,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RecentWorkOrderItem, 0)
	for rows.Next() {
		var item RecentWorkOrderItem
		if err := rows.Scan(&item.ID, &item.SKUCode, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *pgStore) selectCostingFinalizations(ctx context.Context, limit int) ([]RecentCostingFinalizationItem, error) {
	if limit <= 0 {
		return []RecentCostingFinalizationItem{}, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT
			wo.id,
			COALESCE(sku.code, '') AS sku_code,
			COALESCE(cr.total_cost_amount, 0)::bigint AS total_cost,
			COALESCE(cr.created_at, wo.created_at) AS created_at
		FROM work_orders wo
		LEFT JOIN costing_records cr ON cr.work_order_id = wo.id
		LEFT JOIN skus sku ON sku.id = wo.sku_id
		WHERE wo.status = $1
		ORDER BY COALESCE(cr.created_at, wo.created_at) DESC
		LIMIT $2`,
		domain.WOCosted,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RecentCostingFinalizationItem, 0)
	for rows.Next() {
		var item RecentCostingFinalizationItem
		if err := rows.Scan(&item.WorkOrderID, &item.SKUCode, &item.TotalCost, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}


func (s *pgStore) selectBoardStockSummary(ctx context.Context) ([]BoardStockSummaryItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			il.material_id,
			COALESCE(m.name, 'Unknown') AS material_name,
			COUNT(*) FILTER (WHERE bs.status = 'AVAILABLE')              AS available,
			COUNT(*) FILTER (WHERE bs.status = 'ALLOCATED')              AS allocated,
			COALESCE(SUM(CAST(bs.length_mm AS bigint) * CAST(bs.width_mm AS bigint))
				FILTER (WHERE bs.status = 'AVAILABLE'), 0)               AS area_mm2
		FROM board_sheets bs
		JOIN inventory_lots il ON il.id = bs.lot_id
		LEFT JOIN materials m ON m.id = il.material_id
		WHERE bs.status IN ('AVAILABLE', 'ALLOCATED')
		GROUP BY il.material_id, m.name
		ORDER BY m.name NULLS LAST
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]BoardStockSummaryItem, 0)
	for rows.Next() {
		var item BoardStockSummaryItem
		if err := rows.Scan(
			&item.MaterialID,
			&item.MaterialName,
			&item.Available,
			&item.Allocated,
			&item.AreaMM2,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
