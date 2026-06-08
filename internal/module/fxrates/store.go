package fxrates

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	upsertFXRate(ctx context.Context, in UpsertFXRateInput) (FXRate, error)
	selectRateOnDate(ctx context.Context, currency string, date time.Time) (FXRate, error)
	selectRatesPaged(ctx context.Context, p httpkit.PageParams, f FXRateListFilter) ([]FXRate, int, error)
	deleteFXRate(ctx context.Context, id uuid.UUID) error
}
