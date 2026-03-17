package order

import (
	"context"

	"github.com/google/uuid"
)

type store interface {
	insertPO(ctx context.Context, p PO) error
	selectPOs(ctx context.Context) ([]PO, error)
	selectPOByID(ctx context.Context, id uuid.UUID) (PO, error)
	insertLineItems(ctx context.Context, items []LineItem) error
	selectLineItemsByPO(ctx context.Context, poID uuid.UUID) ([]LineItem, error)
	selectLineItemsBySKU(ctx context.Context, skuID uuid.UUID) ([]LineItem, error)
}
