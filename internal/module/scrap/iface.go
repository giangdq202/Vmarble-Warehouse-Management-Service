package scrap

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ScrapSale represents a single scrap sale transaction (BR-C05).
// Scrap sales offset waste cost in the WasteReport (BR-C06).
type ScrapSale struct {
	ID             uuid.UUID    `json:"id"`
	SaleDate       time.Time    `json:"sale_date"`
	MaterialID     uuid.UUID    `json:"material_id"`
	QuantityKG     float64      `json:"quantity_kg"`
	UnitPrice      int64        `json:"unit_price"`
	Currency       string       `json:"currency"`
	TotalAmount    int64        `json:"total_amount"`
	BuyerName      string       `json:"buyer_name"`
	InvoiceNumber  string       `json:"invoice_number"`
	Notes          string       `json:"notes"`
	CreatedBy      uuid.UUID    `json:"created_by"`
	CreatedAt      time.Time    `json:"created_at"`
}

// CreateScrapSaleInput carries the payload for recording a scrap sale.
// Phase A constraint (BR-C08): only VND currency is accepted; service rejects
// currency != "VND" with 400 "only VND supported in Phase A".
type CreateScrapSaleInput struct {
	SaleDate       time.Time `json:"sale_date"`
	MaterialID     uuid.UUID `json:"material_id"`
	QuantityKG     float64   `json:"quantity_kg"`
	UnitPrice      int64     `json:"unit_price"`
	Currency       string    `json:"currency"`
	BuyerName      string    `json:"buyer_name"`
	InvoiceNumber  string    `json:"invoice_number"`
	Notes          string    `json:"notes"`
	CreatedBy      uuid.UUID `json:"-"`
}

// ListScrapSalesFilter narrows the keyset list endpoint by sale_date range
// and an optional material. Mirrors WasteReportFilter so the FE can pass the
// same bounds to both endpoints (BR-C07 — period filter match).
type ListScrapSalesFilter struct {
	From       *time.Time
	To         *time.Time
	MaterialID *uuid.UUID
}

type Service interface {
	// CreateScrapSale records a scrap sale transaction (BR-C05/C08).
	// Phase A: rejects currency != "VND" with 400.
	CreateScrapSale(ctx context.Context, in CreateScrapSaleInput) (ScrapSale, error)

	// ListScrapSales returns scrap sales ordered by created_at DESC (keyset).
	// Filter by sale_date range and/or material_id.
	ListScrapSales(ctx context.Context, params httpkit.CursorParams, filter ListScrapSalesFilter) (httpkit.CursorResult[ScrapSale], error)
}
