package costing

import (
	"context"
	"errors"
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

func (s *pgStore) insertCostingRecord(ctx context.Context, r CostingRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO costing_records (
			id, work_order_id, sku_id, costing_type,
			material_cost_amount, material_cost_currency,
			auxiliary_cost_amount, auxiliary_cost_currency,
			labor_cost_amount, labor_cost_currency,
			total_cost_amount, total_cost_currency,
			finalized, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		r.ID, r.WorkOrderID, r.SKUID, r.CostingType,
		r.MaterialCost.Amount, r.MaterialCost.Currency,
		r.AuxiliaryCost.Amount, r.AuxiliaryCost.Currency,
		r.LaborCost.Amount, r.LaborCost.Currency,
		r.TotalCost.Amount, r.TotalCost.Currency,
		r.Finalized, r.CreatedAt,
	)
	return err
}

func (s *pgStore) updateCostingRecord(ctx context.Context, r CostingRecord) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE costing_records SET
			sku_id = $2, costing_type = $3,
			material_cost_amount = $4, material_cost_currency = $5,
			auxiliary_cost_amount = $6, auxiliary_cost_currency = $7,
			labor_cost_amount = $8, labor_cost_currency = $9,
			total_cost_amount = $10, total_cost_currency = $11
		WHERE work_order_id = $1 AND finalized = false`,
		r.WorkOrderID, r.SKUID, r.CostingType,
		r.MaterialCost.Amount, r.MaterialCost.Currency,
		r.AuxiliaryCost.Amount, r.AuxiliaryCost.Currency,
		r.LaborCost.Amount, r.LaborCost.Currency,
		r.TotalCost.Amount, r.TotalCost.Currency,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return domain.ErrAlreadyFinalized
	}
	return nil
}

func (s *pgStore) selectCostingRecordByWO(ctx context.Context, woID uuid.UUID) (CostingRecord, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+selectCostingCols+` FROM costing_records WHERE work_order_id = $1`,
		woID,
	)
	r, err := scanCostingRecord(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CostingRecord{}, domain.ErrNotFound
		}
		return CostingRecord{}, err
	}
	return r, nil
}

const selectCostingCols = `id, work_order_id, sku_id, costing_type,
	material_cost_amount, material_cost_currency,
	auxiliary_cost_amount, auxiliary_cost_currency,
	labor_cost_amount, labor_cost_currency,
	total_cost_amount, total_cost_currency,
	finalized, finalized_at, finalized_by, created_at`

// prefixedCostingCols is selectCostingCols with a `cr.` table alias on every
// column — used by selectCostingRecordsKeyset which JOINs skus for the
// search ILIKE filter (#313).
const prefixedCostingCols = `cr.id, cr.work_order_id, cr.sku_id, cr.costing_type,
	cr.material_cost_amount, cr.material_cost_currency,
	cr.auxiliary_cost_amount, cr.auxiliary_cost_currency,
	cr.labor_cost_amount, cr.labor_cost_currency,
	cr.total_cost_amount, cr.total_cost_currency,
	cr.finalized, cr.finalized_at, cr.finalized_by, cr.created_at`

func scanCostingRecord(row interface{ Scan(...any) error }) (CostingRecord, error) {
	var r CostingRecord
	var finalizedAt *time.Time
	var finalizedBy *uuid.UUID
	err := row.Scan(
		&r.ID, &r.WorkOrderID, &r.SKUID, &r.CostingType,
		&r.MaterialCost.Amount, &r.MaterialCost.Currency,
		&r.AuxiliaryCost.Amount, &r.AuxiliaryCost.Currency,
		&r.LaborCost.Amount, &r.LaborCost.Currency,
		&r.TotalCost.Amount, &r.TotalCost.Currency,
		&r.Finalized, &finalizedAt, &finalizedBy, &r.CreatedAt,
	)
	r.FinalizedAt = finalizedAt
	r.FinalizedBy = finalizedBy
	return r, err
}

// selectCostingRecordsKeyset implements keyset (cursor) pagination over
// costing_records.created_at DESC, id DESC. The store fetches limit rows;
// callers pass limit+1 so the service can detect has_more without a second
// query. The (created_at, id) tuple comparison handles the boundary case
// where two rows share the exact same timestamp — without the id tie-break
// burst writes can either re-emit a row or skip one.
//
// Filters (#313): finalized flag, sku_id, created_at range, and ILIKE search
// across sku code/name. The LEFT JOIN to skus is unconditional so the search
// predicate can hit even when the caller only filters by date.
//
// The cursor predicate is NULL-guarded so the same SQL serves first-page
// (cur.Ts.IsZero()) and subsequent pages — Postgres can use the
// (created_at DESC, id DESC) composite index in either case.
func (s *pgStore) selectCostingRecordsKeyset(ctx context.Context, filter CostingListFilter, cur httpkit.Cursor, limit int) ([]CostingRecord, error) {
	var curTs any
	var curID any
	if !cur.IsZero() {
		curTs = cur.Ts
		curID = cur.ID
	}
	var search any
	if filter.Search != "" {
		search = "%" + filter.Search + "%"
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+prefixedCostingCols+`
		 FROM costing_records cr
		 LEFT JOIN skus s ON s.id = cr.sku_id
		 WHERE ($1::boolean IS NULL OR cr.finalized = $1)
		   AND ($2::timestamptz IS NULL OR (cr.created_at, cr.id) < ($2, $3))
		   AND ($4::uuid IS NULL OR cr.sku_id = $4)
		   AND ($5::timestamptz IS NULL OR cr.created_at >= $5)
		   AND ($6::timestamptz IS NULL OR cr.created_at < $6)
		   AND ($7::text IS NULL OR s.code ILIKE $7 OR s.name ILIKE $7)
		 ORDER BY cr.created_at DESC, cr.id DESC
		 LIMIT $8`,
		filter.Finalized, curTs, curID, filter.SKUID, filter.From, filter.To, search, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CostingRecord
	for rows.Next() {
		r, err := scanCostingRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) finalizeCostingRecord(ctx context.Context, woID uuid.UUID, actorID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE costing_records SET finalized = true, finalized_at = NOW(), finalized_by = $2
		 WHERE work_order_id = $1 AND finalized = false`,
		woID, actorID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		existing, err := s.selectCostingRecordByWO(ctx, woID)
		if err != nil {
			return err
		}
		if existing.Finalized {
			return domain.ErrAlreadyFinalized
		}
		return domain.ErrNotFound
	}
	return nil
}

func (s *pgStore) hasCostingRecord(ctx context.Context, woID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM costing_records WHERE work_order_id = $1)`,
		woID,
	).Scan(&exists)
	return exists, err
}

func (s *pgStore) insertCostingAdjustment(ctx context.Context, a CostingAdjustment) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO costing_adjustments (
			id, costing_record_id, reason,
			delta_material_amount, delta_material_currency,
			delta_auxiliary_amount, delta_auxiliary_currency,
			delta_labor_amount, delta_labor_currency,
			delta_total_amount, delta_total_currency,
			created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		a.ID, a.CostingRecordID, a.Reason,
		a.DeltaMaterial.Amount, a.DeltaMaterial.Currency,
		a.DeltaAuxiliary.Amount, a.DeltaAuxiliary.Currency,
		a.DeltaLabor.Amount, a.DeltaLabor.Currency,
		a.DeltaTotal.Amount, a.DeltaTotal.Currency,
		a.CreatedBy, a.CreatedAt,
	)
	return err
}

// selectWasteReport aggregates per-cut waste area into a per-material ledger,
// then offsets with scrap sale revenue (BR-C05/C06).
//
// Per-cut waste = source_area - used_area - new_remnant_area, where:
//   - source_area is the area of the immediate source (sheet for direct cuts,
//     remnant for nested cuts);
//   - new_remnant_area is the area of the at-most-one remnant produced by the
//     cut (sheet/remnant becomes ISSUED/CONSUMED after a cut, so the source is
//     a one-shot — guaranteeing 1:0..1 between cut and new_remnant via the
//     parent_board_id / parent_remnant_id linkage).
//
// Cost is allocated per-cut using the originating board_sheet's cost_per_mm²
// (sheet_cost / sheet_area), independent of the immediate source — every
// remnant in the lineage shares the original sheet's per-area cost.
//
// Date filter is applied on cutting_records.created_at (half-open [from, to)).
// Scrap revenue filter uses sale_date with the same bounds (BR-C07).
//
// Materials with scrap sales but no cuts in the period are UNIONed in so
// accounting sees the full picture (user confirm #2).
//
// net_waste_cost = GREATEST(total_waste_cost - scrap_sale_revenue, 0) (BR-C06).
func (s *pgStore) selectWasteReport(ctx context.Context, filter WasteReportFilter) ([]WasteReportRow, error) {
	const query = `
WITH cuts_with_waste AS (
    SELECT
        cr.id AS cut_id,
        COALESCE(cr.sheet_id, r.parent_board_id) AS root_sheet_id,
        CAST(cr.used_length_mm AS bigint) * CAST(cr.used_width_mm AS bigint) AS used_area_mm2,
        CASE
            WHEN cr.sheet_id IS NOT NULL THEN
                CAST(bs_direct.length_mm AS bigint) * CAST(bs_direct.width_mm AS bigint)
            ELSE
                CAST(r.length_mm AS bigint) * CAST(r.width_mm AS bigint)
        END AS source_area_mm2,
        COALESCE((
            SELECT CAST(nr.length_mm AS bigint) * CAST(nr.width_mm AS bigint)
            FROM remnants nr
            WHERE (
                (cr.sheet_id IS NOT NULL AND nr.parent_board_id = cr.sheet_id AND nr.parent_remnant_id IS NULL)
                OR
                (cr.remnant_source_id IS NOT NULL AND nr.parent_remnant_id = cr.remnant_source_id)
            )
            LIMIT 1
        ), 0) AS new_remnant_area_mm2
    FROM cutting_records cr
    LEFT JOIN board_sheets bs_direct ON bs_direct.id = cr.sheet_id
    LEFT JOIN remnants r ON r.id = cr.remnant_source_id
    WHERE ($1::timestamptz IS NULL OR cr.created_at >= $1)
      AND ($2::timestamptz IS NULL OR cr.created_at < $2)
),
sheet_costs_per_material AS (
    SELECT
        il.material_id,
        AVG(bs.cost_amount)::bigint AS avg_sheet_cost,
        MAX(bs.cost_currency) AS currency
    FROM (SELECT DISTINCT root_sheet_id FROM cuts_with_waste WHERE root_sheet_id IS NOT NULL) ds
    JOIN board_sheets bs ON bs.id = ds.root_sheet_id
    JOIN inventory_lots il ON il.id = bs.lot_id
    GROUP BY il.material_id
),
waste_from_cuts AS (
    SELECT
        il.material_id,
        COALESCE(m.name, 'Unknown') AS material_name,
        COUNT(DISTINCT cwnr.root_sheet_id) AS sheets_consumed,
        COALESCE(SUM(GREATEST(cwnr.source_area_mm2 - cwnr.used_area_mm2 - cwnr.new_remnant_area_mm2, 0)), 0) AS waste_area_mm2,
        COALESCE(scpm.avg_sheet_cost, 0) AS avg_sheet_cost,
        COALESCE(scpm.currency, 'VND') AS currency,
        COALESCE(SUM(
            CASE
                WHEN CAST(bs.length_mm AS bigint) * CAST(bs.width_mm AS bigint) > 0 THEN
                    GREATEST(cwnr.source_area_mm2 - cwnr.used_area_mm2 - cwnr.new_remnant_area_mm2, 0)
                    * bs.cost_amount
                    / (CAST(bs.length_mm AS bigint) * CAST(bs.width_mm AS bigint))
                ELSE 0
            END
        ), 0) AS total_waste_cost
    FROM cuts_with_waste cwnr
    JOIN board_sheets bs ON bs.id = cwnr.root_sheet_id
    JOIN inventory_lots il ON il.id = bs.lot_id
    LEFT JOIN materials m ON m.id = il.material_id
    LEFT JOIN sheet_costs_per_material scpm ON scpm.material_id = il.material_id
    WHERE ($3::uuid IS NULL OR il.material_id = $3)
    GROUP BY il.material_id, m.name, scpm.avg_sheet_cost, scpm.currency
),
scrap_revenue_per_material AS (
    SELECT
        material_id,
        SUM(total_amount) AS scrap_revenue_amount
    FROM scrap_sales
    WHERE currency = 'VND'
      AND ($1::timestamptz IS NULL OR sale_date >= $1::date)
      AND ($2::timestamptz IS NULL OR sale_date < $2::date)
      AND ($3::uuid IS NULL OR material_id = $3)
    GROUP BY material_id
),
all_materials AS (
    SELECT material_id FROM waste_from_cuts
    UNION
    SELECT material_id FROM scrap_revenue_per_material
)
SELECT
    am.material_id,
    COALESCE(wfc.material_name, COALESCE(m.name, 'Unknown')) AS material_name,
    COALESCE(wfc.sheets_consumed, 0) AS sheets_consumed,
    COALESCE(wfc.waste_area_mm2, 0) AS waste_area_mm2,
    COALESCE(wfc.avg_sheet_cost, 0) AS avg_sheet_cost,
    COALESCE(wfc.currency, 'VND') AS currency,
    COALESCE(wfc.total_waste_cost, 0) AS total_waste_cost,
    COALESCE(srpm.scrap_revenue_amount, 0) AS scrap_revenue_amount,
    GREATEST(COALESCE(wfc.total_waste_cost, 0) - COALESCE(srpm.scrap_revenue_amount, 0), 0) AS net_waste_cost
FROM all_materials am
LEFT JOIN waste_from_cuts wfc ON wfc.material_id = am.material_id
LEFT JOIN scrap_revenue_per_material srpm ON srpm.material_id = am.material_id
LEFT JOIN materials m ON m.id = am.material_id
ORDER BY net_waste_cost DESC, material_name ASC NULLS LAST
`
	rows, err := s.pool.Query(ctx, query, filter.From, filter.To, filter.MaterialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]WasteReportRow, 0)
	for rows.Next() {
		var r WasteReportRow
		var avgAmount, totalWasteAmount, scrapRevenueAmount, netWasteAmount int64
		var currency string
		if err := rows.Scan(
			&r.MaterialID,
			&r.MaterialName,
			&r.SheetsConsumed,
			&r.WasteAreaMM2,
			&avgAmount,
			&currency,
			&totalWasteAmount,
			&scrapRevenueAmount,
			&netWasteAmount,
		); err != nil {
			return nil, err
		}
		r.AvgSheetCost = domain.Money{Amount: avgAmount, Currency: currency}
		r.TotalWasteCost = domain.Money{Amount: totalWasteAmount, Currency: currency}
		r.ScrapSaleRevenue = domain.Money{Amount: scrapRevenueAmount, Currency: "VND"}
		r.NetWasteCost = domain.Money{Amount: netWasteAmount, Currency: currency}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) selectAdjustmentsByRecord(ctx context.Context, costingRecordID uuid.UUID) ([]CostingAdjustment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, costing_record_id, reason,
			delta_material_amount, delta_material_currency,
			delta_auxiliary_amount, delta_auxiliary_currency,
			delta_labor_amount, delta_labor_currency,
			delta_total_amount, delta_total_currency,
			created_by, created_at
		 FROM costing_adjustments
		 WHERE costing_record_id = $1
		 ORDER BY created_at`,
		costingRecordID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CostingAdjustment
	for rows.Next() {
		var a CostingAdjustment
		if err := rows.Scan(
			&a.ID, &a.CostingRecordID, &a.Reason,
			&a.DeltaMaterial.Amount, &a.DeltaMaterial.Currency,
			&a.DeltaAuxiliary.Amount, &a.DeltaAuxiliary.Currency,
			&a.DeltaLabor.Amount, &a.DeltaLabor.Currency,
			&a.DeltaTotal.Amount, &a.DeltaTotal.Currency,
			&a.CreatedBy, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
