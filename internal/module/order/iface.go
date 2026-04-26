package order

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type CreateLineItemInput struct {
	SKUID        uuid.UUID    `json:"sku_id"`
	Quantity     int          `json:"quantity"`
	SellingPrice domain.Money `json:"selling_price"`
}

type CreatePOInput struct {
	Code             string                `json:"code"`
	ExpectedDelivery time.Time             `json:"expected_delivery"`
	LineItems        []CreateLineItemInput `json:"line_items"`
}

type LineItem struct {
	ID           uuid.UUID    `json:"id"`
	POID         uuid.UUID    `json:"po_id"`
	SKUID        uuid.UUID    `json:"sku_id"`
	Quantity     int          `json:"quantity"`
	SellingPrice domain.Money `json:"selling_price"`
}

type PO struct {
	ID               uuid.UUID  `json:"id"`
	Code             string     `json:"code"`
	ExpectedDelivery time.Time  `json:"expected_delivery"`
	IsActive         bool       `json:"is_active"`
	CreatedAt        time.Time  `json:"created_at"`
	ItemCount        int        `json:"item_count,omitempty"`
	TotalQuantity    int        `json:"total_quantity,omitempty"`
	TotalSKUs        int        `json:"total_skus,omitempty"`
	LineItems        []LineItem `json:"line_items,omitempty"`
}

type Service interface {
	CreatePO(ctx context.Context, in CreatePOInput) (PO, error)
	GetPO(ctx context.Context, poID uuid.UUID) (PO, error)
	ListPOs(ctx context.Context, p httpkit.PageParams) (httpkit.PagedResult[PO], error)
	DeactivatePO(ctx context.Context, poID uuid.UUID) error
	GetLineItemsByPO(ctx context.Context, poID uuid.UUID) ([]LineItem, error)
	GetLineItemsBySKU(ctx context.Context, skuID uuid.UUID) ([]LineItem, error)
}
