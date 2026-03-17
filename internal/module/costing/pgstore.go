package costing

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

func (s *pgStore) insertCostingRecord(ctx context.Context, r CostingRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO costing_records (
			id, work_order_id, sku_id,
			material_cost_amount, material_cost_currency,
			auxiliary_cost_amount, auxiliary_cost_currency,
			total_cost_amount, total_cost_currency,
			finalized, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		r.ID, r.WorkOrderID, r.SKUID,
		r.MaterialCost.Amount, r.MaterialCost.Currency,
		r.AuxiliaryCost.Amount, r.AuxiliaryCost.Currency,
		r.TotalCost.Amount, r.TotalCost.Currency,
		r.Finalized, r.CreatedAt,
	)
	return err
}

func (s *pgStore) updateCostingRecord(ctx context.Context, r CostingRecord) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE costing_records SET
			sku_id = $2,
			material_cost_amount = $3, material_cost_currency = $4,
			auxiliary_cost_amount = $5, auxiliary_cost_currency = $6,
			total_cost_amount = $7, total_cost_currency = $8
		WHERE work_order_id = $1 AND finalized = false`,
		r.WorkOrderID, r.SKUID,
		r.MaterialCost.Amount, r.MaterialCost.Currency,
		r.AuxiliaryCost.Amount, r.AuxiliaryCost.Currency,
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
	var r CostingRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, work_order_id, sku_id,
			material_cost_amount, material_cost_currency,
			auxiliary_cost_amount, auxiliary_cost_currency,
			total_cost_amount, total_cost_currency,
			finalized, created_at
		FROM costing_records WHERE work_order_id = $1`,
		woID,
	).Scan(
		&r.ID, &r.WorkOrderID, &r.SKUID,
		&r.MaterialCost.Amount, &r.MaterialCost.Currency,
		&r.AuxiliaryCost.Amount, &r.AuxiliaryCost.Currency,
		&r.TotalCost.Amount, &r.TotalCost.Currency,
		&r.Finalized, &r.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CostingRecord{}, domain.ErrNotFound
		}
		return CostingRecord{}, err
	}
	return r, nil
}

func (s *pgStore) selectCostingRecords(ctx context.Context) ([]CostingRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, work_order_id, sku_id,
			material_cost_amount, material_cost_currency,
			auxiliary_cost_amount, auxiliary_cost_currency,
			total_cost_amount, total_cost_currency,
			finalized, created_at
		FROM costing_records ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CostingRecord
	for rows.Next() {
		var r CostingRecord
		if err := rows.Scan(
			&r.ID, &r.WorkOrderID, &r.SKUID,
			&r.MaterialCost.Amount, &r.MaterialCost.Currency,
			&r.AuxiliaryCost.Amount, &r.AuxiliaryCost.Currency,
			&r.TotalCost.Amount, &r.TotalCost.Currency,
			&r.Finalized, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *pgStore) finalizeCostingRecord(ctx context.Context, woID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE costing_records SET finalized = true WHERE work_order_id = $1 AND finalized = false`,
		woID,
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
