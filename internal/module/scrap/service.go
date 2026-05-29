package scrap

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

type service struct {
	st store
}

func NewService(st store) Service {
	return &service{st: st}
}

func (s *service) CreateScrapSale(ctx context.Context, in CreateScrapSaleInput) (ScrapSale, error) {
	// Phase A constraint (BR-C08): only VND currency accepted.
	if in.Currency != "VND" {
		return ScrapSale{}, domain.NewBizError(domain.ErrInvalidInput, "only VND supported in Phase A")
	}

	// Validate inputs.
	if in.SaleDate.IsZero() {
		return ScrapSale{}, domain.NewBizError(domain.ErrInvalidInput, "sale_date is required")
	}
	if in.SaleDate.After(time.Now().UTC()) {
		return ScrapSale{}, domain.NewBizError(domain.ErrInvalidInput, "sale_date cannot be in the future")
	}
	if in.MaterialID == uuid.Nil {
		return ScrapSale{}, domain.NewBizError(domain.ErrInvalidInput, "material_id is required")
	}
	if in.QuantityKG <= 0 {
		return ScrapSale{}, domain.NewBizError(domain.ErrInvalidInput, "quantity_kg must be positive")
	}
	if in.UnitPrice < 0 {
		return ScrapSale{}, domain.NewBizError(domain.ErrInvalidInput, "unit_price must be non-negative")
	}

	sale := ScrapSale{
		ID:            uuid.New(),
		SaleDate:      in.SaleDate,
		MaterialID:    in.MaterialID,
		QuantityKG:    in.QuantityKG,
		UnitPrice:     in.UnitPrice,
		Currency:      in.Currency,
		TotalAmount:   int64(in.QuantityKG * float64(in.UnitPrice)),
		BuyerName:     in.BuyerName,
		InvoiceNumber: in.InvoiceNumber,
		Notes:         in.Notes,
		CreatedBy:     in.CreatedBy,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.st.insertScrapSale(ctx, sale); err != nil {
		return ScrapSale{}, err
	}
	return sale, nil
}

func (s *service) ListScrapSales(ctx context.Context, params httpkit.CursorParams, filter ListScrapSalesFilter) (httpkit.CursorResult[ScrapSale], error) {
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return httpkit.CursorResult[ScrapSale]{}, domain.NewBizError(domain.ErrInvalidInput, "from must be before to")
	}
	cur, err := params.Decoded()
	if err != nil {
		return httpkit.CursorResult[ScrapSale]{}, err
	}
	items, err := s.st.selectScrapSalesKeyset(ctx, filter, cur, params.Limit+1)
	if err != nil {
		return httpkit.CursorResult[ScrapSale]{}, err
	}
	return httpkit.NewCursorResult(items, params.Limit, func(s ScrapSale) httpkit.Cursor {
		return httpkit.Cursor{Ts: s.CreatedAt, ID: s.ID}
	}), nil
}
