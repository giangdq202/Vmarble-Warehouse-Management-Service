// Package sales owns customers and sales orders. Sales orders capture
// commitments to ship to a customer; production plans/work orders are
// derived via SplitToPlan. The qty_planned column on sales_order_lines is
// mutated by this module's SplitToPlan and by the delivery module via
// IncrementQtyShipped — see deps.go for the cross-module surface.
package sales

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// SO status values; mirror the chk_so_status DB constraint.
const (
	SOStatusDraft           = "DRAFT"
	SOStatusConfirmed       = "CONFIRMED"
	SOStatusInProduction    = "IN_PRODUCTION"
	SOStatusPartiallyShipped = "PARTIALLY_SHIPPED"
	SOStatusShipped         = "SHIPPED"
	SOStatusCancelled       = "CANCELLED"
)

type Customer struct {
	ID            uuid.UUID `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	CountryCode   string    `json:"country_code,omitempty"`
	Address       string    `json:"address,omitempty"`
	ContactPerson string    `json:"contact_person,omitempty"`
	ContactPhone  string    `json:"contact_phone,omitempty"`
	ContactEmail  string    `json:"contact_email,omitempty"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreateCustomerInput accepts an optional Code. When Code is empty the
// service draws the next value from customer_code_seq and formats KH%03d.
// When Code is supplied it must be unique; collision returns ErrInvalidInput.
type CreateCustomerInput struct {
	Code          string `json:"code,omitempty"`
	Name          string `json:"name"`
	CountryCode   string `json:"country_code,omitempty"`
	Address       string `json:"address,omitempty"`
	ContactPerson string `json:"contact_person,omitempty"`
	ContactPhone  string `json:"contact_phone,omitempty"`
	ContactEmail  string `json:"contact_email,omitempty"`
}

type PatchCustomerInput struct {
	ID            uuid.UUID
	Name          *string
	CountryCode   *string
	Address       *string
	ContactPerson *string
	ContactPhone  *string
	ContactEmail  *string
	IsActive      *bool
}

type SalesOrderLine struct {
	ID            uuid.UUID    `json:"id"`
	SalesOrderID  uuid.UUID    `json:"sales_order_id"`
	SKUID         uuid.UUID    `json:"sku_id"`
	QtyOrdered    int          `json:"qty_ordered"`
	QtyPlanned    int          `json:"qty_planned"`
	QtyShipped    int          `json:"qty_shipped"`
	UnitPrice     domain.Money `json:"unit_price"`
	CreatedAt     time.Time    `json:"created_at"`
}

type SalesOrder struct {
	ID                uuid.UUID  `json:"id"`
	Code              string     `json:"code"`
	CustomerID        uuid.UUID  `json:"customer_id"`
	CustomerCode      string     `json:"customer_code,omitempty"`
	CustomerName      string     `json:"customer_name,omitempty"`
	CustomerCountry   string     `json:"customer_country_code,omitempty"`
	Incoterm          string     `json:"incoterm,omitempty"`
	PortOfLoading     string     `json:"port_of_loading,omitempty"`
	PortOfDischarge   string     `json:"port_of_discharge,omitempty"`
	Currency          string     `json:"currency"`
	Status            string     `json:"status"`
	ExpectedShipDate  *time.Time `json:"expected_ship_date,omitempty"`
	Note              string     `json:"note,omitempty"`
	CreatedBy         uuid.UUID  `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	Lines             []SalesOrderLine `json:"lines,omitempty"`
}

type CreateSOLineInput struct {
	SKUID      uuid.UUID    `json:"sku_id"`
	QtyOrdered int          `json:"qty_ordered"`
	UnitPrice  domain.Money `json:"unit_price"`
}

type CreateSOInput struct {
	CustomerID       uuid.UUID            `json:"customer_id"`
	Incoterm         string               `json:"incoterm,omitempty"`
	PortOfLoading    string               `json:"port_of_loading,omitempty"`
	PortOfDischarge  string               `json:"port_of_discharge,omitempty"`
	Currency         string               `json:"currency"`
	ExpectedShipDate *time.Time           `json:"expected_ship_date,omitempty"`
	Note             string               `json:"note,omitempty"`
	Lines            []CreateSOLineInput  `json:"lines"`
	CreatedBy        uuid.UUID            `json:"-"`
}

// PatchSOInput updates a DRAFT sales order. Nil pointers leave the column
// unchanged; non-nil pointers (including pointer-to-empty-string) overwrite.
// Lines, when supplied, fully replace the existing set — partial line edits
// are not supported on purpose to keep the patch contract simple.
type PatchSOInput struct {
	ID               uuid.UUID
	Incoterm         *string
	PortOfLoading    *string
	PortOfDischarge  *string
	Currency         *string
	ExpectedShipDate *time.Time
	ClearExpectedShipDate bool
	Note             *string
	Lines            *[]CreateSOLineInput
}

type SOListFilter struct {
	Status     string
	CustomerID *uuid.UUID
}

// SplitAllocation tells SplitToPlan how much of one sales_order_lines row to
// pull into a new production plan + work order. Quantity must be positive
// and (line.qty_planned + Quantity) must not exceed line.qty_ordered, or
// SplitToPlan returns ErrInvalidInput.
type SplitAllocation struct {
	SOLineID uuid.UUID `json:"so_line_id"`
	Quantity int       `json:"quantity"`
}

type SplitToPlanInput struct {
	SalesOrderID uuid.UUID         `json:"sales_order_id"`
	Allocations  []SplitAllocation `json:"allocations"`
	Deadline     time.Time         `json:"deadline"`
	ActorID      uuid.UUID         `json:"-"`
}

type SplitToPlanResult struct {
	PlanID       uuid.UUID   `json:"plan_id"`
	PlanCode     string      `json:"plan_code"`
	WorkOrderIDs []uuid.UUID `json:"work_order_ids"`
}

type CancelSOInput struct {
	ID      uuid.UUID
	Reason  string
	ActorID uuid.UUID
}

type Service interface {
	CreateCustomer(ctx context.Context, in CreateCustomerInput) (Customer, error)
	ListCustomers(ctx context.Context, p httpkit.PageParams, activeOnly bool) (httpkit.PagedResult[Customer], error)
	PatchCustomer(ctx context.Context, in PatchCustomerInput) (Customer, error)

	CreateSO(ctx context.Context, in CreateSOInput) (SalesOrder, error)
	GetSO(ctx context.Context, id uuid.UUID) (SalesOrder, error)
	ListSOs(ctx context.Context, p httpkit.PageParams, f SOListFilter) (httpkit.PagedResult[SalesOrder], error)
	PatchSO(ctx context.Context, in PatchSOInput) (SalesOrder, error)
	ConfirmSO(ctx context.Context, id uuid.UUID) error
	CancelSO(ctx context.Context, in CancelSOInput) error
	SplitToPlan(ctx context.Context, in SplitToPlanInput) (SplitToPlanResult, error)
}
