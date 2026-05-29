package scrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── mockStore ─────────────────────────────────────────────────────────────────

type mockStore struct {
	insertErr    error
	insertCalled bool

	listResult []ScrapSale
	listErr    error
}

func (m *mockStore) insertScrapSale(_ context.Context, _ ScrapSale) error {
	m.insertCalled = true
	return m.insertErr
}

func (m *mockStore) selectScrapSalesKeyset(_ context.Context, _ ListScrapSalesFilter, _ httpkit.Cursor, _ int) ([]ScrapSale, error) {
	return m.listResult, m.listErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func validInput() CreateScrapSaleInput {
	return CreateScrapSaleInput{
		SaleDate:   time.Now().UTC().AddDate(0, 0, -1), // yesterday
		MaterialID: uuid.New(),
		QuantityKG: 100.0,
		UnitPrice:  5000,
		Currency:   "VND",
		CreatedBy:  uuid.New(),
	}
}

// ── CreateScrapSale tests ─────────────────────────────────────────────────────

func TestCreateScrapSale_HappyPath(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	sale, err := svc.CreateScrapSale(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.insertCalled {
		t.Error("expected insertScrapSale to be called")
	}
	if sale.ID == uuid.Nil {
		t.Error("expected non-nil ID")
	}
	if sale.Currency != "VND" {
		t.Errorf("currency = %q, want VND", sale.Currency)
	}
	// TotalAmount = quantity_kg * unit_price
	wantTotal := int64(in.QuantityKG * float64(in.UnitPrice))
	if sale.TotalAmount != wantTotal {
		t.Errorf("total_amount = %d, want %d", sale.TotalAmount, wantTotal)
	}
}

func TestCreateScrapSale_CurrencyGuard_RejectsNonVND(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	in.Currency = "USD"
	_, err := svc.CreateScrapSale(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for non-VND currency, got %v", err)
	}
	if st.insertCalled {
		t.Error("insertScrapSale must not be called when currency guard rejects")
	}
}

func TestCreateScrapSale_MissingCurrency_RejectsNonVND(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	in.Currency = ""
	_, err := svc.CreateScrapSale(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty currency, got %v", err)
	}
}

func TestCreateScrapSale_ZeroSaleDate_Rejects(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	in.SaleDate = time.Time{}
	_, err := svc.CreateScrapSale(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero sale_date, got %v", err)
	}
}

func TestCreateScrapSale_FutureSaleDate_Rejects(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	in.SaleDate = time.Now().UTC().AddDate(0, 0, 1) // tomorrow
	_, err := svc.CreateScrapSale(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for future sale_date, got %v", err)
	}
}

func TestCreateScrapSale_NilMaterialID_Rejects(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	in.MaterialID = uuid.Nil
	_, err := svc.CreateScrapSale(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for nil material_id, got %v", err)
	}
}

func TestCreateScrapSale_NegativeQuantity_Rejects(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	in.QuantityKG = -10
	_, err := svc.CreateScrapSale(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative quantity_kg, got %v", err)
	}
}

func TestCreateScrapSale_ZeroQuantity_Rejects(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	in.QuantityKG = 0
	_, err := svc.CreateScrapSale(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero quantity_kg, got %v", err)
	}
}

func TestCreateScrapSale_NegativeUnitPrice_Rejects(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	in.UnitPrice = -1
	_, err := svc.CreateScrapSale(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative unit_price, got %v", err)
	}
}

func TestCreateScrapSale_ZeroUnitPrice_Allowed(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	in := validInput()
	in.UnitPrice = 0
	_, err := svc.CreateScrapSale(context.Background(), in)
	if err != nil {
		t.Errorf("zero unit_price should be allowed (donation/write-off), got: %v", err)
	}
}

func TestCreateScrapSale_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("db down")
	st := &mockStore{insertErr: storeErr}
	svc := NewService(st)

	_, err := svc.CreateScrapSale(context.Background(), validInput())
	if !errors.Is(err, storeErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── ListScrapSales tests ──────────────────────────────────────────────────────

func TestListScrapSales_HappyPath_ReturnsSales(t *testing.T) {
	matID := uuid.New()
	sales := []ScrapSale{
		{ID: uuid.New(), MaterialID: matID, QuantityKG: 50, UnitPrice: 3000, Currency: "VND", TotalAmount: 150000, CreatedAt: time.Now().UTC()},
		{ID: uuid.New(), MaterialID: matID, QuantityKG: 80, UnitPrice: 3000, Currency: "VND", TotalAmount: 240000, CreatedAt: time.Now().UTC().Add(-time.Hour)},
	}
	st := &mockStore{listResult: sales}
	svc := NewService(st)

	result, err := svc.ListScrapSales(context.Background(), httpkit.CursorParams{Limit: 10}, ListScrapSalesFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("items = %d, want 2", len(result.Items))
	}
}

func TestListScrapSales_FromAfterTo_Rejects(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	from := time.Now().UTC()
	to := from.AddDate(0, 0, -1)
	_, err := svc.ListScrapSales(context.Background(), httpkit.CursorParams{Limit: 10}, ListScrapSalesFilter{
		From: &from,
		To:   &to,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput when from > to, got %v", err)
	}
}

func TestListScrapSales_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("query failed")
	st := &mockStore{listErr: storeErr}
	svc := NewService(st)

	_, err := svc.ListScrapSales(context.Background(), httpkit.CursorParams{Limit: 10}, ListScrapSalesFilter{})
	if !errors.Is(err, storeErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

func TestListScrapSales_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	st := &mockStore{listResult: nil}
	svc := NewService(st)

	result, err := svc.ListScrapSales(context.Background(), httpkit.CursorParams{Limit: 10}, ListScrapSalesFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Items == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(result.Items) != 0 {
		t.Errorf("items = %d, want 0", len(result.Items))
	}
}
