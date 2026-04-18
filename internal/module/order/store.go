package order

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	// insertPOWithItems inserts the PO header and all its line items atomically
	// in a single transaction. If any insert fails the entire operation is rolled
	// back, preventing orphan PO records.
	insertPOWithItems(ctx context.Context, p PO, items []LineItem) error
	selectPOsPaged(ctx context.Context, p httpkit.PageParams) ([]PO, int, error)
	selectPOByID(ctx context.Context, id uuid.UUID) (PO, error)
	deactivatePO(ctx context.Context, id uuid.UUID) error
	selectLineItemsByPO(ctx context.Context, poID uuid.UUID) ([]LineItem, error)
	selectLineItemsBySKU(ctx context.Context, skuID uuid.UUID) ([]LineItem, error)
}
