package fxrates

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// FXRate is one daily exchange rate entry. rate_to_vnd expresses how many VND
// one unit of the foreign currency equals on effective_date. For example:
// USD/VND = 25000 means 1 USD = 25000 VND.
type FXRate struct {
	ID            uuid.UUID `json:"id"`
	Currency      string    `json:"currency"`
	RateToVND     float64   `json:"rate_to_vnd"`
	EffectiveDate time.Time `json:"effective_date"`
	CreatedAt     time.Time `json:"created_at"`
}

type UpsertFXRateInput struct {
	Currency      string    `json:"currency"`
	RateToVND     float64   `json:"rate_to_vnd"`
	EffectiveDate time.Time `json:"effective_date"`
}

type FXRateListFilter struct {
	Currency string
}

type Service interface {
	// UpsertRate creates or replaces the rate for (currency, effective_date).
	UpsertRate(ctx context.Context, in UpsertFXRateInput) (FXRate, error)
	// GetRateOnDate returns the most recent rate for currency on or before date.
	// Returns ErrNotFound when no rate exists.
	GetRateOnDate(ctx context.Context, currency string, date time.Time) (FXRate, error)
	ListRates(ctx context.Context, p httpkit.PageParams, f FXRateListFilter) (httpkit.PagedResult[FXRate], error)
	DeleteRate(ctx context.Context, id uuid.UUID) error
}
