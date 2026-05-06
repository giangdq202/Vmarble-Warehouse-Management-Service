package purchasing

import (
	"context"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

// MaterialChecker verifies a material exists in the catalog.
type MaterialChecker interface {
	GetMaterial(ctx context.Context, materialID uuid.UUID) (MaterialInfo, error)
}

type MaterialInfo struct {
	ID   uuid.UUID
	Name string
	Unit string
}

// StockReceiver creates inventory lots and board sheets when a PO is received.
type StockReceiver interface {
	ReceiveStock(ctx context.Context, in ReceiveStockInput) (uuid.UUID, error)
}

type ReceiveStockInput struct {
	MaterialID  uuid.UUID
	LengthMM    int
	WidthMM     int
	UnitCost    domain.Money
	Quantity    int
	SupplierRef string
}
