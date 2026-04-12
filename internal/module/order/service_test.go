package order

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
// Hand-written mock satisfying the unexported store interface.

type mockStore struct {
	// insertPO
	insertPOErr error

	// selectPOs
	selectPOsResult []PO
	selectPOsErr    error

	// selectPOByID
	selectPOByIDResult PO
	selectPOByIDErr    error

	// deactivatePO
	deactivatePOErr error

	// insertLineItems
	insertLineItemsErr error

	// selectLineItemsByPO
	selectLineItemsByPOResult []LineItem
	selectLineItemsByPOErr    error

	// selectLineItemsBySKU
	selectLineItemsBySKUResult []LineItem
	selectLineItemsBySKUErr    error
}

func (m *mockStore) insertPO(_ context.Context, _ PO) error {
	return m.insertPOErr
}

func (m *mockStore) selectPOsPaged(_ context.Context, _ httpkit.PageParams) ([]PO, int, error) {
	return m.selectPOsResult, len(m.selectPOsResult), m.selectPOsErr
}

func (m *mockStore) selectPOByID(_ context.Context, _ uuid.UUID) (PO, error) {
	return m.selectPOByIDResult, m.selectPOByIDErr
}

func (m *mockStore) deactivatePO(_ context.Context, _ uuid.UUID) error {
	return m.deactivatePOErr
}

func (m *mockStore) insertLineItems(_ context.Context, _ []LineItem) error {
	return m.insertLineItemsErr
}

func (m *mockStore) selectLineItemsByPO(_ context.Context, _ uuid.UUID) ([]LineItem, error) {
	return m.selectLineItemsByPOResult, m.selectLineItemsByPOErr
}

func (m *mockStore) selectLineItemsBySKU(_ context.Context, _ uuid.UUID) ([]LineItem, error) {
	return m.selectLineItemsBySKUResult, m.selectLineItemsBySKUErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func validInput(lineItems ...CreateLineItemInput) CreatePOInput {
	if len(lineItems) == 0 {
		lineItems = []CreateLineItemInput{
			{
				SKUID:        uuid.New(),
				Quantity:     10,
				SellingPrice: domain.Money{Amount: 50_000, Currency: "VND"},
			},
		}
	}
	return CreatePOInput{
		Code:             "PO-2026-001",
		ExpectedDelivery: time.Now().Add(7 * 24 * time.Hour).UTC(),
		LineItems:        lineItems,
	}
}

// ── TestCreatePO ──────────────────────────────────────────────────────────────

func TestCreatePO_HappyPath_ReturnsWithLineItems(t *testing.T) {
	skuID := uuid.New()
	in := validInput(CreateLineItemInput{
		SKUID:        skuID,
		Quantity:     5,
		SellingPrice: domain.Money{Amount: 100_000, Currency: "VND"},
	})

	svc := NewService(&mockStore{})
	po, err := svc.CreatePO(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if po.ID == uuid.Nil {
		t.Error("PO ID must be set")
	}
	if po.Code != in.Code {
		t.Errorf("Code = %q, want %q", po.Code, in.Code)
	}
	if len(po.LineItems) != 1 {
		t.Fatalf("len(LineItems) = %d, want 1", len(po.LineItems))
	}

	li := po.LineItems[0]
	if li.ID == uuid.Nil {
		t.Error("LineItem ID must be set")
	}
	if li.POID != po.ID {
		t.Errorf("LineItem.POID = %v, want %v", li.POID, po.ID)
	}
	if li.SKUID != skuID {
		t.Errorf("LineItem.SKUID = %v, want %v", li.SKUID, skuID)
	}
	if li.Quantity != 5 {
		t.Errorf("LineItem.Quantity = %d, want 5", li.Quantity)
	}
}

func TestCreatePO_MultipleLineItems_AllPresent(t *testing.T) {
	in := validInput(
		CreateLineItemInput{SKUID: uuid.New(), Quantity: 3, SellingPrice: domain.Money{Amount: 10_000, Currency: "VND"}},
		CreateLineItemInput{SKUID: uuid.New(), Quantity: 7, SellingPrice: domain.Money{Amount: 20_000, Currency: "VND"}},
	)

	svc := NewService(&mockStore{})
	po, err := svc.CreatePO(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(po.LineItems) != 2 {
		t.Errorf("len(LineItems) = %d, want 2", len(po.LineItems))
	}
	// Every item must link back to the PO.
	for i, li := range po.LineItems {
		if li.POID != po.ID {
			t.Errorf("LineItems[%d].POID = %v, want %v", i, li.POID, po.ID)
		}
		if li.ID == uuid.Nil {
			t.Errorf("LineItems[%d].ID must not be nil", i)
		}
	}
}

// ── Validation: Code ──────────────────────────────────────────────────────────

func TestCreatePO_EmptyCode_ReturnsErrInvalidInput(t *testing.T) {
	in := validInput()
	in.Code = ""

	svc := NewService(&mockStore{})
	_, err := svc.CreatePO(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty code, got %v", err)
	}
}

// ── Validation: LineItems ─────────────────────────────────────────────────────

func TestCreatePO_NoLineItems_ReturnsErrInvalidInput(t *testing.T) {
	in := CreatePOInput{
		Code:      "PO-EMPTY",
		LineItems: []CreateLineItemInput{},
	}

	svc := NewService(&mockStore{})
	_, err := svc.CreatePO(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty line items, got %v", err)
	}
}

func TestCreatePO_NilLineItems_ReturnsErrInvalidInput(t *testing.T) {
	in := CreatePOInput{
		Code:      "PO-NIL",
		LineItems: nil,
	}

	svc := NewService(&mockStore{})
	_, err := svc.CreatePO(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for nil line items, got %v", err)
	}
}

// ── Validation: Quantity ──────────────────────────────────────────────────────

func TestCreatePO_ZeroQuantityLineItem_ReturnsErrInvalidInput(t *testing.T) {
	in := validInput(CreateLineItemInput{
		SKUID:    uuid.New(),
		Quantity: 0,
	})

	svc := NewService(&mockStore{})
	_, err := svc.CreatePO(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero quantity, got %v", err)
	}
}

func TestCreatePO_NegativeQuantityLineItem_ReturnsErrInvalidInput(t *testing.T) {
	in := validInput(CreateLineItemInput{
		SKUID:    uuid.New(),
		Quantity: -1,
	})

	svc := NewService(&mockStore{})
	_, err := svc.CreatePO(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative quantity, got %v", err)
	}
}

func TestCreatePO_QuantityOneIsValid(t *testing.T) {
	// Boundary: quantity=1 must be accepted (> 0).
	in := validInput(CreateLineItemInput{SKUID: uuid.New(), Quantity: 1})

	svc := NewService(&mockStore{})
	_, err := svc.CreatePO(context.Background(), in)

	if err != nil {
		t.Errorf("quantity=1 must be valid, got: %v", err)
	}
}

func TestCreatePO_SecondLineItemInvalidQuantity_ReturnsErrInvalidInput(t *testing.T) {
	// First item valid, second item has quantity=0 — loop must catch it.
	in := validInput(
		CreateLineItemInput{SKUID: uuid.New(), Quantity: 5},
		CreateLineItemInput{SKUID: uuid.New(), Quantity: 0},
	)

	svc := NewService(&mockStore{})
	_, err := svc.CreatePO(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for second item quantity=0, got %v", err)
	}
}

// ── Store error propagation ───────────────────────────────────────────────────

func TestCreatePO_StoreInsertPOError_Propagates(t *testing.T) {
	dbErr := errors.New("insert po failed")
	st := &mockStore{insertPOErr: dbErr}

	svc := NewService(st)
	_, err := svc.CreatePO(context.Background(), validInput())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected insertPO error to propagate, got %v", err)
	}
}

func TestCreatePO_StoreInsertLineItemsError_Propagates(t *testing.T) {
	dbErr := errors.New("insert line items failed")
	st := &mockStore{insertLineItemsErr: dbErr}

	svc := NewService(st)
	_, err := svc.CreatePO(context.Background(), validInput())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected insertLineItems error to propagate, got %v", err)
	}
}

// ── TestGetPO ─────────────────────────────────────────────────────────────────

func TestGetPO_HappyPath_ReturnsWithLineItems(t *testing.T) {
	poID := uuid.New()
	skuID := uuid.New()

	storedPO := PO{ID: poID, Code: "PO-001", CreatedAt: time.Now().UTC()}
	storedItems := []LineItem{
		{ID: uuid.New(), POID: poID, SKUID: skuID, Quantity: 3},
	}

	st := &mockStore{
		selectPOByIDResult:        storedPO,
		selectLineItemsByPOResult: storedItems,
	}

	svc := NewService(st)
	po, err := svc.GetPO(context.Background(), poID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if po.ID != poID {
		t.Errorf("PO.ID = %v, want %v", po.ID, poID)
	}
	if len(po.LineItems) != 1 {
		t.Fatalf("len(LineItems) = %d, want 1", len(po.LineItems))
	}
	if po.LineItems[0].SKUID != skuID {
		t.Errorf("LineItems[0].SKUID = %v, want %v", po.LineItems[0].SKUID, skuID)
	}
}

func TestGetPO_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectPOByIDErr: domain.NewBizError(domain.ErrNotFound, "purchase order not found"),
	}

	svc := NewService(st)
	_, err := svc.GetPO(context.Background(), uuid.New())

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

func TestGetPO_SelectLineItemsError_Propagates(t *testing.T) {
	dbErr := errors.New("line items query failed")
	st := &mockStore{
		selectPOByIDResult:     PO{ID: uuid.New()},
		selectLineItemsByPOErr: dbErr,
	}

	svc := NewService(st)
	_, err := svc.GetPO(context.Background(), uuid.New())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected selectLineItemsByPO error to propagate, got %v", err)
	}
}

// ── TestListPOs ───────────────────────────────────────────────────────────────

func TestListPOs_Empty_ReturnsNil(t *testing.T) {
	st := &mockStore{selectPOsResult: nil}

	svc := NewService(st)
	pos, err := svc.ListPOs(context.Background(), httpkit.PageParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pos.Items) != 0 {
		t.Errorf("expected empty result, got %d items", len(pos.Items))
	}
}

func TestListPOs_ReturnAll(t *testing.T) {
	stored := []PO{
		{ID: uuid.New(), Code: "PO-001"},
		{ID: uuid.New(), Code: "PO-002"},
	}
	st := &mockStore{selectPOsResult: stored}

	svc := NewService(st)
	pos, err := svc.ListPOs(context.Background(), httpkit.PageParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pos.Items) != 2 {
		t.Errorf("len = %d, want 2", len(pos.Items))
	}
}

func TestListPOs_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("select pos failed")
	st := &mockStore{selectPOsErr: dbErr}

	svc := NewService(st)
	_, err := svc.ListPOs(context.Background(), httpkit.PageParams{Page: 1, Limit: 10})

	if !errors.Is(err, dbErr) {
		t.Errorf("expected selectPOs error to propagate, got %v", err)
	}
}

// ── TestGetLineItemsByPO ──────────────────────────────────────────────────────

func TestGetLineItemsByPO_HappyPath(t *testing.T) {
	poID := uuid.New()
	items := []LineItem{
		{ID: uuid.New(), POID: poID, SKUID: uuid.New(), Quantity: 2},
	}
	st := &mockStore{selectLineItemsByPOResult: items}

	svc := NewService(st)
	got, err := svc.GetLineItemsByPO(context.Background(), poID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
}

func TestGetLineItemsByPO_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("select by po failed")
	st := &mockStore{selectLineItemsByPOErr: dbErr}

	svc := NewService(st)
	_, err := svc.GetLineItemsByPO(context.Background(), uuid.New())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected error to propagate, got %v", err)
	}
}

// ── TestGetLineItemsBySKU ─────────────────────────────────────────────────────

func TestGetLineItemsBySKU_HappyPath(t *testing.T) {
	skuID := uuid.New()
	items := []LineItem{
		{ID: uuid.New(), POID: uuid.New(), SKUID: skuID, Quantity: 4},
		{ID: uuid.New(), POID: uuid.New(), SKUID: skuID, Quantity: 6},
	}
	st := &mockStore{selectLineItemsBySKUResult: items}

	svc := NewService(st)
	got, err := svc.GetLineItemsBySKU(context.Background(), skuID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestGetLineItemsBySKU_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("select by sku failed")
	st := &mockStore{selectLineItemsBySKUErr: dbErr}

	svc := NewService(st)
	_, err := svc.GetLineItemsBySKU(context.Background(), uuid.New())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected error to propagate, got %v", err)
	}
}
