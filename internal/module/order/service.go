package order

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
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
		ID:               uuid.New(),
		Code:             in.Code,
		ExpectedDelivery: in.ExpectedDelivery,
		IsActive:         true,
		CreatedAt:        now,
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

	if err := svc.s.insertPOWithItems(ctx, po, items); err != nil {
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

func (svc *service) ListPOs(ctx context.Context, p httpkit.PageParams, f POListFilter) (httpkit.PagedResult[PO], error) {
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return httpkit.PagedResult[PO]{}, domain.NewBizError(domain.ErrInvalidInput, "from must be before to")
	}
	pos, total, err := svc.s.selectPOsPaged(ctx, p, f)
	if err != nil {
		return httpkit.PagedResult[PO]{}, err
	}
	return httpkit.NewPagedResult(pos, total, p), nil
}

func (svc *service) DeactivatePO(ctx context.Context, poID uuid.UUID) error {
	return svc.s.deactivatePO(ctx, poID)
}

func (svc *service) GetLineItemsByPO(ctx context.Context, poID uuid.UUID) ([]LineItem, error) {
	return svc.s.selectLineItemsByPO(ctx, poID)
}

func (svc *service) GetLineItemsBySKU(ctx context.Context, skuID uuid.UUID) ([]LineItem, error) {
	return svc.s.selectLineItemsBySKU(ctx, skuID)
}
