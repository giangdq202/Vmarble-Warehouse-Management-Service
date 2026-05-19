package purchasing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── mock store ───────────────────────────────────────────────────────────────

type mockStore struct {
	insertPOErr    error
	selectPOResult PurchaseOrder
	selectPOErr    error
	selectPOsResult []PurchaseOrder
	selectPOsTotal  int
	selectPOsErr    error
	updateStatusErr error
	insertItemErr   error
	deleteItemErr   error
	selectItemsResult []POItem
	selectItemsErr    error
	linkItemToLotErr  error

	updateStatusCalled bool
	updateStatusArg    POStatus
	linkItemCalled     bool
}

func (m *mockStore) insertPO(_ context.Context, _ PurchaseOrder) error {
	return m.insertPOErr
}
func (m *mockStore) selectPOByID(_ context.Context, _ uuid.UUID) (PurchaseOrder, error) {
	return m.selectPOResult, m.selectPOErr
}
func (m *mockStore) selectPOsPaged(_ context.Context, _ httpkit.PageParams, _ POListFilter) ([]PurchaseOrder, int, error) {
	return m.selectPOsResult, m.selectPOsTotal, m.selectPOsErr
}
func (m *mockStore) updatePOStatus(_ context.Context, _ uuid.UUID, status POStatus, _ *time.Time) error {
	m.updateStatusCalled = true
	m.updateStatusArg = status
	return m.updateStatusErr
}
func (m *mockStore) insertPOItem(_ context.Context, _ POItem) error {
	return m.insertItemErr
}
func (m *mockStore) deletePOItem(_ context.Context, _, _ uuid.UUID) error {
	return m.deleteItemErr
}
func (m *mockStore) selectPOItems(_ context.Context, _ uuid.UUID) ([]POItem, error) {
	return m.selectItemsResult, m.selectItemsErr
}
func (m *mockStore) linkItemToLot(_ context.Context, _, _ uuid.UUID) error {
	m.linkItemCalled = true
	return m.linkItemToLotErr
}

// ── mock deps ────────────────────────────────────────────────────────────────

type mockMaterialChecker struct {
	result MaterialInfo
	err    error
}

func (m *mockMaterialChecker) GetMaterial(_ context.Context, _ uuid.UUID) (MaterialInfo, error) {
	return m.result, m.err
}

type mockStockReceiver struct {
	lotID uuid.UUID
	err   error
	calls int
}

func (m *mockStockReceiver) ReceiveStock(_ context.Context, _ ReceiveStockInput) (uuid.UUID, error) {
	m.calls++
	return m.lotID, m.err
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newSvc(st store, mc MaterialChecker, sr StockReceiver) Service {
	return NewService(st, mc, sr)
}

func draftPO() PurchaseOrder {
	return PurchaseOrder{
		ID:         uuid.New(),
		Code:       "PO-001",
		MaterialID: uuid.New(),
		Status:     StatusDraft,
		CreatedBy:  uuid.New(),
		CreatedAt:  time.Now(),
	}
}

func orderedPO() PurchaseOrder {
	po := draftPO()
	po.Status = StatusOrdered
	now := time.Now()
	po.OrderedAt = &now
	po.Items = []POItem{
		{ID: uuid.New(), POID: po.ID, Quantity: 5, LengthMM: 1000, WidthMM: 600, UnitCost: domain.VND(100000), CreatedAt: time.Now()},
	}
	return po
}

// ── CreatePO ─────────────────────────────────────────────────────────────────

func TestCreatePO_HappyPath(t *testing.T) {
	materialID := uuid.New()
	st := &mockStore{}
	mc := &mockMaterialChecker{result: MaterialInfo{ID: materialID}}
	sr := &mockStockReceiver{}
	svc := newSvc(st, mc, sr)

	po, err := svc.CreatePO(context.Background(), CreatePOInput{
		Code:       "PO-001",
		MaterialID: materialID,
		Supplier:   "ABC Corp",
		CreatedBy:  uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if po.Status != StatusDraft {
		t.Errorf("status = %v, want DRAFT", po.Status)
	}
	if po.Code != "PO-001" {
		t.Errorf("code = %v, want PO-001", po.Code)
	}
}

func TestCreatePO_EmptyCode_IsInvalidInput(t *testing.T) {
	st := &mockStore{}
	mc := &mockMaterialChecker{}
	svc := newSvc(st, mc, &mockStockReceiver{})

	_, err := svc.CreatePO(context.Background(), CreatePOInput{
		MaterialID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty code, got %v", err)
	}
}

func TestCreatePO_NilMaterialID_IsInvalidInput(t *testing.T) {
	st := &mockStore{}
	mc := &mockMaterialChecker{}
	svc := newSvc(st, mc, &mockStockReceiver{})

	_, err := svc.CreatePO(context.Background(), CreatePOInput{Code: "PO-X"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for nil material_id, got %v", err)
	}
}

func TestCreatePO_MaterialNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{}
	mc := &mockMaterialChecker{err: domain.ErrNotFound}
	svc := newSvc(st, mc, &mockStockReceiver{})

	_, err := svc.CreatePO(context.Background(), CreatePOInput{
		Code:       "PO-001",
		MaterialID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound when material missing, got %v", err)
	}
}

// ── AddItem ──────────────────────────────────────────────────────────────────

func TestAddItem_HappyPath(t *testing.T) {
	po := draftPO()
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	item, err := svc.AddItem(context.Background(), AddPOItemInput{
		POID: po.ID, Quantity: 3, LengthMM: 1000, WidthMM: 600,
		UnitCost: domain.VND(200000),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Quantity != 3 {
		t.Errorf("Quantity = %v, want 3", item.Quantity)
	}
}

func TestAddItem_NonDraftPO_IsPreconditionFailed(t *testing.T) {
	for _, status := range []POStatus{StatusOrdered, StatusReceived, StatusCancelled} {
		po := draftPO()
		po.Status = status
		st := &mockStore{selectPOResult: po}
		svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

		_, err := svc.AddItem(context.Background(), AddPOItemInput{
			POID: po.ID, Quantity: 1, LengthMM: 100, WidthMM: 100,
			UnitCost: domain.VND(1000),
		})
		if !errors.Is(err, domain.ErrPreconditionFailed) {
			t.Errorf("status %v: expected ErrPreconditionFailed, got %v", status, err)
		}
	}
}

func TestAddItem_ZeroQuantity_IsInvalidInput(t *testing.T) {
	po := draftPO()
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	_, err := svc.AddItem(context.Background(), AddPOItemInput{
		POID: po.ID, Quantity: 0, LengthMM: 100, WidthMM: 100,
		UnitCost: domain.VND(1000),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero quantity, got %v", err)
	}
}

// ── OrderPO ──────────────────────────────────────────────────────────────────

func TestOrderPO_HappyPath(t *testing.T) {
	po := draftPO()
	po.Items = []POItem{{ID: uuid.New(), Quantity: 1}}
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	result, err := svc.OrderPO(context.Background(), po.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusOrdered {
		t.Errorf("status = %v, want ORDERED", result.Status)
	}
	if !st.updateStatusCalled || st.updateStatusArg != StatusOrdered {
		t.Error("store.updatePOStatus must be called with ORDERED")
	}
}

func TestOrderPO_NonDraft_IsInvalidTransition(t *testing.T) {
	po := draftPO()
	po.Status = StatusOrdered
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	_, err := svc.OrderPO(context.Background(), po.ID)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestOrderPO_NoItems_IsPreconditionFailed(t *testing.T) {
	po := draftPO()
	po.Items = []POItem{}
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	_, err := svc.OrderPO(context.Background(), po.ID)
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed when no items, got %v", err)
	}
}

// ── ReceivePO ────────────────────────────────────────────────────────────────

func TestReceivePO_HappyPath(t *testing.T) {
	po := orderedPO()
	lotID := uuid.New()
	st := &mockStore{selectPOResult: po}
	sr := &mockStockReceiver{lotID: lotID}
	svc := newSvc(st, &mockMaterialChecker{}, sr)

	result, err := svc.ReceivePO(context.Background(), po.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusReceived {
		t.Errorf("status = %v, want RECEIVED", result.Status)
	}
	if sr.calls != len(po.Items) {
		t.Errorf("ReceiveStock called %d times, want %d", sr.calls, len(po.Items))
	}
	if !st.linkItemCalled {
		t.Error("linkItemToLot must be called after ReceiveStock")
	}
}

func TestReceivePO_NonOrdered_IsInvalidTransition(t *testing.T) {
	po := draftPO()
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	_, err := svc.ReceivePO(context.Background(), po.ID)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestReceivePO_StockReceiverError_Propagates(t *testing.T) {
	po := orderedPO()
	st := &mockStore{selectPOResult: po}
	sr := &mockStockReceiver{err: errors.New("inventory unavailable")}
	svc := newSvc(st, &mockMaterialChecker{}, sr)

	_, err := svc.ReceivePO(context.Background(), po.ID)
	if err == nil {
		t.Error("expected error when stock receiver fails")
	}
}

// ── CancelPO ─────────────────────────────────────────────────────────────────

func TestCancelPO_FromDraft_Succeeds(t *testing.T) {
	po := draftPO()
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	result, err := svc.CancelPO(context.Background(), po.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusCancelled {
		t.Errorf("status = %v, want CANCELLED", result.Status)
	}
}

func TestCancelPO_FromOrdered_Succeeds(t *testing.T) {
	po := orderedPO()
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	result, err := svc.CancelPO(context.Background(), po.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusCancelled {
		t.Errorf("status = %v, want CANCELLED", result.Status)
	}
}

func TestCancelPO_FromReceived_IsInvalidTransition(t *testing.T) {
	po := draftPO()
	po.Status = StatusReceived
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	_, err := svc.CancelPO(context.Background(), po.ID)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestCancelPO_AlreadyCancelled_IsInvalidTransition(t *testing.T) {
	po := draftPO()
	po.Status = StatusCancelled
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	_, err := svc.CancelPO(context.Background(), po.ID)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

// ── RemoveItem ───────────────────────────────────────────────────────────────

func TestRemoveItem_NonDraft_IsPreconditionFailed(t *testing.T) {
	po := orderedPO()
	st := &mockStore{selectPOResult: po}
	svc := newSvc(st, &mockMaterialChecker{}, &mockStockReceiver{})

	err := svc.RemoveItem(context.Background(), po.ID, uuid.New())
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed, got %v", err)
	}
}

// ── ListPOs date filter ──────────────────────────────────────────────────────

func TestListPOs_FromAfterTo_ReturnsErrInvalidInput(t *testing.T) {
	from := time.Now()
	to := from.Add(-24 * time.Hour)
	svc := newSvc(&mockStore{}, &mockMaterialChecker{}, &mockStockReceiver{})
	_, err := svc.ListPOs(context.Background(),
		httpkit.PageParams{Page: 1, Limit: 10},
		POListFilter{From: &from, To: &to})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for from > to, got %v", err)
	}
}

func TestListPOs_OpenEndedFrom_DoesNotError(t *testing.T) {
	from := time.Now().Add(-30 * 24 * time.Hour)
	svc := newSvc(&mockStore{}, &mockMaterialChecker{}, &mockStockReceiver{})
	if _, err := svc.ListPOs(context.Background(),
		httpkit.PageParams{Page: 1, Limit: 10},
		POListFilter{From: &from}); err != nil {
		t.Errorf("from-only must not error, got %v", err)
	}
}
