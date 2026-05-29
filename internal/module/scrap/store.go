package scrap

import (
	"context"

	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// store is the unexported repository interface for scrap_sales persistence.
// Implemented by pgStore in pgstore.go.
type store interface {
	insertScrapSale(ctx context.Context, s ScrapSale) error
	selectScrapSalesKeyset(ctx context.Context, filter ListScrapSalesFilter, cursor httpkit.Cursor, limit int) ([]ScrapSale, error)
}
