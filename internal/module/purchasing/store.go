package purchasing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type store interface {
	insertPO(ctx context.Context, po PurchaseOrder) error
	selectPOByID(ctx context.Context, id uuid.UUID) (PurchaseOrder, error)
	selectPOsPaged(ctx context.Context, p httpkit.PageParams, f POListFilter) ([]PurchaseOrder, int, error)
	updatePOStatus(ctx context.Context, id uuid.UUID, status POStatus, ts *time.Time) error
	insertPOItem(ctx context.Context, item POItem) error
	deletePOItem(ctx context.Context, poID, itemID uuid.UUID) error
	selectPOItems(ctx context.Context, poID uuid.UUID) ([]POItem, error)
	linkItemToLot(ctx context.Context, itemID, lotID uuid.UUID) error
}
