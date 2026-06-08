package fxrates

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

func (s *pgStore) upsertFXRate(ctx context.Context, in UpsertFXRateInput) (FXRate, error) {
	var r FXRate
	err := s.pool.QueryRow(ctx,
		`INSERT INTO fx_rates (currency, rate_to_vnd, effective_date)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (currency, effective_date) DO UPDATE
		     SET rate_to_vnd = EXCLUDED.rate_to_vnd
		 RETURNING id, currency, rate_to_vnd, effective_date, created_at`,
		in.Currency, in.RateToVND, in.EffectiveDate.UTC(),
	).Scan(&r.ID, &r.Currency, &r.RateToVND, &r.EffectiveDate, &r.CreatedAt)
	return r, err
}

// selectRateOnDate returns the most recent rate for currency on or before date
// (closest effective_date ≤ date).
func (s *pgStore) selectRateOnDate(ctx context.Context, currency string, date time.Time) (FXRate, error) {
	var r FXRate
	err := s.pool.QueryRow(ctx,
		`SELECT id, currency, rate_to_vnd, effective_date, created_at
		   FROM fx_rates
		  WHERE currency = $1
		    AND effective_date <= $2
		  ORDER BY effective_date DESC
		  LIMIT 1`,
		currency, date.UTC(),
	).Scan(&r.ID, &r.Currency, &r.RateToVND, &r.EffectiveDate, &r.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FXRate{}, domain.ErrNotFound
		}
		return FXRate{}, err
	}
	return r, nil
}

func (s *pgStore) selectRatesPaged(ctx context.Context, p httpkit.PageParams, f FXRateListFilter) ([]FXRate, int, error) {
	where := "($1::text = '' OR currency = $1)"
	args := []any{f.Currency}

	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM fx_rates WHERE `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, p.Limit, p.Offset())
	rows, err := s.pool.Query(ctx,
		`SELECT id, currency, rate_to_vnd, effective_date, created_at
		   FROM fx_rates
		  WHERE `+where+`
		  ORDER BY effective_date DESC, currency ASC
		  LIMIT $2 OFFSET $3`,
		args...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []FXRate
	for rows.Next() {
		var r FXRate
		if err := rows.Scan(&r.ID, &r.Currency, &r.RateToVND, &r.EffectiveDate, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *pgStore) deleteFXRate(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM fx_rates WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
