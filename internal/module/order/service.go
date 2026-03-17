package order

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

type service struct {
	s store
}

func NewService(s store) Service {
	return &service{s: s}
}

func (svc *service) CreatePO(ctx context.Context, in CreatePOInput) (PO, error) {
	if in.Code == "" {
		return PO{}, domain.NewBizError(domain.ErrInvalidInput, "code is required")
	}
	if len(in.LineItems) == 0 {
		return PO{}, domain.NewBizError(domain.ErrInvalidInput, "at least one line item is required")
	}
	for _, li := range in.LineItems {
		if li.Quantity <= 0 {
			return PO{}, domain.NewBizError(domain.ErrInvalidInput, "line item quantity must be greater than 0")
		}
	}

	now := time.Now()
	po := PO{
		ID:                 uuid.New(),
		Code:               in.Code,
		ExpectedDelivery:   in.ExpectedDelivery,
		CreatedAt:          now,
	}

	if err := svc.s.insertPO(ctx, po); err != nil {
		return PO{}, err
	}

	items := make([]LineItem, len(in.LineItems))
	for i, li := range in.LineItems {
		items[i] = LineItem{
			ID:           uuid.New(),
			POID:         po.ID,
			SKUID:        li.SKUID,
			Quantity:     li.Quantity,
			SellingPrice: li.SellingPrice,
		}
	}
	if err := svc.s.insertLineItems(ctx, items); err != nil {
		return PO{}, err
	}

	po.LineItems = items
	return po, nil
}

func (svc *service) GetPO(ctx context.Context, poID uuid.UUID) (PO, error) {
	po, err := svc.s.selectPOByID(ctx, poID)
	if err != nil {
		return PO{}, err
	}
	items, err := svc.s.selectLineItemsByPO(ctx, poID)
	if err != nil {
		return PO{}, err
	}
	po.LineItems = items
	return po, nil
}

func (svc *service) ListPOs(ctx context.Context) ([]PO, error) {
	return svc.s.selectPOs(ctx)
}

func (svc *service) GetLineItemsByPO(ctx context.Context, poID uuid.UUID) ([]LineItem, error) {
	return svc.s.selectLineItemsByPO(ctx, poID)
}

func (svc *service) GetLineItemsBySKU(ctx context.Context, skuID uuid.UUID) ([]LineItem, error) {
	return svc.s.selectLineItemsBySKU(ctx, skuID)
}
