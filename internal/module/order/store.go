package order

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertPO(ctx context.Context, p PO) error
	selectPOsPaged(ctx context.Context, p httpkit.PageParams) ([]PO, int, error)
	selectPOByID(ctx context.Context, id uuid.UUID) (PO, error)
	deactivatePO(ctx context.Context, id uuid.UUID) error
	insertLineItems(ctx context.Context, items []LineItem) error
	selectLineItemsByPO(ctx context.Context, poID uuid.UUID) ([]LineItem, error)
	selectLineItemsBySKU(ctx context.Context, skuID uuid.UUID) ([]LineItem, error)
}
