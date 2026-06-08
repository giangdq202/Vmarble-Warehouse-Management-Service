package fxrates

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	s store
}

func NewService(s store) Service {
	return &service{s: s}
}

func (svc *service) UpsertRate(ctx context.Context, in UpsertFXRateInput) (FXRate, error) {
	if in.Currency == "" {
		return FXRate{}, domain.NewBizError(domain.ErrInvalidInput, "currency is required")
	}
	if in.RateToVND <= 0 {
		return FXRate{}, domain.NewBizError(domain.ErrInvalidInput, "rate_to_vnd must be positive")
	}
	if in.EffectiveDate.IsZero() {
		return FXRate{}, domain.NewBizError(domain.ErrInvalidInput, "effective_date is required")
	}
	return svc.s.upsertFXRate(ctx, in)
}

func (svc *service) GetRateOnDate(ctx context.Context, currency string, date time.Time) (FXRate, error) {
	if currency == "" {
		return FXRate{}, domain.NewBizError(domain.ErrInvalidInput, "currency is required")
	}
	return svc.s.selectRateOnDate(ctx, currency, date)
}

func (svc *service) ListRates(ctx context.Context, p httpkit.PageParams, f FXRateListFilter) (httpkit.PagedResult[FXRate], error) {
	rows, total, err := svc.s.selectRatesPaged(ctx, p, f)
	if err != nil {
		return httpkit.PagedResult[FXRate]{}, err
	}
	if rows == nil {
		rows = []FXRate{}
	}
	return httpkit.NewPagedResult(rows, total, p), nil
}

func (svc *service) DeleteRate(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return domain.NewBizError(domain.ErrInvalidInput, "id is required")
	}
	err := svc.s.deleteFXRate(ctx, id)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return err
}
