package purchasing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type POStatus string

const (
	StatusDraft     POStatus = "DRAFT"
	StatusOrdered   POStatus = "ORDERED"
	StatusReceived  POStatus = "RECEIVED"
	StatusCancelled POStatus = "CANCELLED"
)

type PurchaseOrder struct {
	ID         uuid.UUID  `json:"id"`
	Code       string     `json:"code"`
	MaterialID uuid.UUID  `json:"material_id"`
	Supplier   string     `json:"supplier"`
	Status     POStatus   `json:"status"`
	Note       string     `json:"note,omitempty"`
	OrderedAt  *time.Time `json:"ordered_at,omitempty"`
	ReceivedAt *time.Time `json:"received_at,omitempty"`
	CreatedBy  uuid.UUID  `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	Items      []POItem   `json:"items,omitempty"`
}

type POItem struct {
	ID        uuid.UUID    `json:"id"`
	POID      uuid.UUID    `json:"po_id"`
	Quantity  int          `json:"quantity"`
	LengthMM  int          `json:"length_mm"`
	WidthMM   int          `json:"width_mm"`
	UnitCost  domain.Money `json:"unit_cost"`
	LotID     *uuid.UUID   `json:"lot_id,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}

type CreatePOInput struct {
	Code       string    `json:"code"`
	MaterialID uuid.UUID `json:"material_id"`
	Supplier   string    `json:"supplier"`
	Note       string    `json:"note"`
	CreatedBy  uuid.UUID `json:"-"`
}

type AddPOItemInput struct {
	POID     uuid.UUID    `json:"-"`
	Quantity int          `json:"quantity"`
	LengthMM int          `json:"length_mm"`
	WidthMM  int          `json:"width_mm"`
	UnitCost domain.Money `json:"unit_cost"`
}

type POListFilter struct {
	Status     string
	MaterialID *uuid.UUID
}

type Service interface {
	CreatePO(ctx context.Context, in CreatePOInput) (PurchaseOrder, error)
	GetPO(ctx context.Context, id uuid.UUID) (PurchaseOrder, error)
	ListPOs(ctx context.Context, p httpkit.PageParams, f POListFilter) (httpkit.PagedResult[PurchaseOrder], error)
	AddItem(ctx context.Context, in AddPOItemInput) (POItem, error)
	RemoveItem(ctx context.Context, poID, itemID uuid.UUID) error
	OrderPO(ctx context.Context, id uuid.UUID) (PurchaseOrder, error)
	ReceivePO(ctx context.Context, id uuid.UUID) (PurchaseOrder, error)
	CancelPO(ctx context.Context, id uuid.UUID) (PurchaseOrder, error)
}
