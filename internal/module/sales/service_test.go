package sales

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── mockStore ────────────────────────────────────────────────────────────────
//
// Hand-written mock satisfying both `store` and (via mockTxStore) `txStore`.
// Each operation captures arguments + returns a configurable result/error
// so tests can wire either happy paths or specific failure branches.

type mockStore struct {
	// nextCustomerCode
	nextCustomerCodeResult string
	nextCustomerCodeErr    error

	// customerCodeExists
	customerCodeExistsResult bool
	customerCodeExistsErr    error

	// insertCustomer
	insertedCustomer *Customer
	insertCustomerErr error

	// selectCustomerByID
	selectCustomerByIDResult Customer
	selectCustomerByIDErr    error

	// selectCustomersPaged
	selectCustomersPagedResult []Customer
	selectCustomersPagedErr    error

	// updateCustomer
	updateCustomerErr error

	// nextSOCode
	nextSOCodeResult string
	nextSOCodeErr    error

	// insertSO
	insertedSO    *SalesOrder
	insertSOErr   error

	// insertSOLines
	insertedSOLines []SalesOrderLine
	insertSOLinesErr error

	// deleteSOLinesBySO
	deleteSOLinesBySOErr error

	// selectSOByID
	selectSOByIDResult SalesOrder
	selectSOByIDErr    error

	// selectSOLinesBySOID
	selectSOLinesBySOIDResult []SalesOrderLine
	selectSOLinesBySOIDErr    error

	// selectSOsPaged
	selectSOsPagedResult []SalesOrder
	selectSOsPagedErr    error

	// updateSO / updateSOStatus
	updateSOErr       error
	updateSOStatusErr error
	lastStatusUpdate  string

	// withTx — tx is the in-tx store handed to fn; fn is the closure passed
	// to withTx so tests can inject "third-tx-fails-after-second-succeeds".
	tx        *mockTxStore
	withTxErr error

	// Customer SKU mappings (#304)
	insertCSMErr           error
	insertedCSM            *CustomerSKUMapping
	selectCSMByPKResult    CustomerSKUMapping
	selectCSMByPKErr       error
	selectCSMPagedResult   []CustomerSKUMapping
	selectCSMPagedErr      error
	updateCSMErr           error
	deleteCSMErr           error
	bulkInsertCSMErr       error
	bulkInsertCSMRows      []CustomerSKUMapping
}

type mockTxStore struct {
	lockSOForUpdateResult SalesOrder
	lockSOForUpdateErr    error

	lockAndReadSOLinesResult []SalesOrderLine
	lockAndReadSOLinesErr    error

	incrementCalls       []incrementCall
	incrementQtyPlannedErr error

	updateStatusCalls       []updateStatusCall
	updateStatusIfCurrentResult bool
	updateStatusIfCurrentErr    error
}

type incrementCall struct {
	LineID uuid.UUID
	Delta  int
}

type updateStatusCall struct {
	ID       uuid.UUID
	Expected []string
	Target   string
}

func (m *mockStore) nextCustomerCode(_ context.Context) (string, error) {
	return m.nextCustomerCodeResult, m.nextCustomerCodeErr
}

func (m *mockStore) customerCodeExists(_ context.Context, _ string) (bool, error) {
	return m.customerCodeExistsResult, m.customerCodeExistsErr
}

func (m *mockStore) insertCustomer(_ context.Context, c Customer) error {
	if m.insertCustomerErr != nil {
		return m.insertCustomerErr
	}
	m.insertedCustomer = &c
	return nil
}

func (m *mockStore) selectCustomerByID(_ context.Context, _ uuid.UUID) (Customer, error) {
	return m.selectCustomerByIDResult, m.selectCustomerByIDErr
}

func (m *mockStore) selectCustomersPaged(_ context.Context, _ httpkit.PageParams, _ bool) ([]Customer, int, error) {
	return m.selectCustomersPagedResult, len(m.selectCustomersPagedResult), m.selectCustomersPagedErr
}

func (m *mockStore) updateCustomer(_ context.Context, _ Customer) error {
	return m.updateCustomerErr
}

func (m *mockStore) nextSOCode(_ context.Context, _ time.Time) (string, error) {
	return m.nextSOCodeResult, m.nextSOCodeErr
}

func (m *mockStore) insertSO(_ context.Context, so SalesOrder) error {
	if m.insertSOErr != nil {
		return m.insertSOErr
	}
	m.insertedSO = &so
	return nil
}

func (m *mockStore) insertSOLines(_ context.Context, lines []SalesOrderLine) error {
	if m.insertSOLinesErr != nil {
		return m.insertSOLinesErr
	}
	m.insertedSOLines = append(m.insertedSOLines, lines...)
	return nil
}

func (m *mockStore) deleteSOLinesBySO(_ context.Context, _ uuid.UUID) error {
	return m.deleteSOLinesBySOErr
}

func (m *mockStore) selectSOByID(_ context.Context, _ uuid.UUID) (SalesOrder, error) {
	return m.selectSOByIDResult, m.selectSOByIDErr
}

func (m *mockStore) selectSOLinesBySOID(_ context.Context, _ uuid.UUID) ([]SalesOrderLine, error) {
	return m.selectSOLinesBySOIDResult, m.selectSOLinesBySOIDErr
}

func (m *mockStore) selectSOsPaged(_ context.Context, _ httpkit.PageParams, _ SOListFilter) ([]SalesOrder, int, error) {
	return m.selectSOsPagedResult, len(m.selectSOsPagedResult), m.selectSOsPagedErr
}

func (m *mockStore) updateSO(_ context.Context, _ SalesOrder) error {
	return m.updateSOErr
}

func (m *mockStore) updateSOStatus(_ context.Context, _ uuid.UUID, status string) error {
	m.lastStatusUpdate = status
	return m.updateSOStatusErr
}

func (m *mockStore) selectSOLineByID(_ context.Context, _ uuid.UUID) (SalesOrderLine, SalesOrder, error) {
	return SalesOrderLine{}, SalesOrder{}, nil
}

func (m *mockStore) recordShipmentTx(_ context.Context, _ pgx.Tx, _ []ShipmentItemInput) error {
	return nil
}

func (m *mockStore) insertCarryOverSOLine(_ context.Context, _ CarryOverSOLineInput) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (m *mockStore) withTx(ctx context.Context, fn func(tx txStore) error) error {
	if m.withTxErr != nil {
		return m.withTxErr
	}
	if m.tx == nil {
		m.tx = &mockTxStore{}
	}
	return fn(m.tx)
}

func (m *mockStore) insertCustomerSKUMapping(_ context.Context, c CustomerSKUMapping) error {
	if m.insertCSMErr != nil {
		return m.insertCSMErr
	}
	m.insertedCSM = &c
	return nil
}

func (m *mockStore) selectCustomerSKUMappingsPaged(_ context.Context, _ httpkit.PageParams, _ CustomerSKUMappingFilter) ([]CustomerSKUMapping, int, error) {
	return m.selectCSMPagedResult, len(m.selectCSMPagedResult), m.selectCSMPagedErr
}

func (m *mockStore) selectCustomerSKUMappingByPK(_ context.Context, _ uuid.UUID, _ string) (CustomerSKUMapping, error) {
	return m.selectCSMByPKResult, m.selectCSMByPKErr
}

func (m *mockStore) updateCustomerSKUMapping(_ context.Context, _ CustomerSKUMapping) error {
	return m.updateCSMErr
}

func (m *mockStore) deleteCustomerSKUMapping(_ context.Context, _ uuid.UUID, _ string) error {
	return m.deleteCSMErr
}

func (m *mockStore) bulkInsertCustomerSKUMappings(_ context.Context, rows []CustomerSKUMapping) error {
	if m.bulkInsertCSMErr != nil {
		return m.bulkInsertCSMErr
	}
	m.bulkInsertCSMRows = append(m.bulkInsertCSMRows, rows...)
	return nil
}

func (m *mockTxStore) lockSOForUpdate(_ context.Context, _ uuid.UUID) (SalesOrder, error) {
	return m.lockSOForUpdateResult, m.lockSOForUpdateErr
}

func (m *mockTxStore) lockAndReadSOLines(_ context.Context, _ []uuid.UUID) ([]SalesOrderLine, error) {
	return m.lockAndReadSOLinesResult, m.lockAndReadSOLinesErr
}

func (m *mockTxStore) incrementQtyPlanned(_ context.Context, lineID uuid.UUID, delta int) error {
	if m.incrementQtyPlannedErr != nil {
		return m.incrementQtyPlannedErr
	}
	m.incrementCalls = append(m.incrementCalls, incrementCall{LineID: lineID, Delta: delta})
	return nil
}

func (m *mockTxStore) updateStatusIfCurrent(_ context.Context, id uuid.UUID, expected []string, target string) (bool, error) {
	m.updateStatusCalls = append(m.updateStatusCalls, updateStatusCall{ID: id, Expected: expected, Target: target})
	return m.updateStatusIfCurrentResult, m.updateStatusIfCurrentErr
}

// ── mockSplitter ────────────────────────────────────────────────────────────

type mockSplitter struct {
	calledWith CreatePlanWithWOsRequest
	result     CreatePlanWithWOsResult
	err        error
}

func (m *mockSplitter) CreatePlanWithWOs(_ context.Context, in CreatePlanWithWOsRequest) (CreatePlanWithWOsResult, error) {
	m.calledWith = in
	if m.err != nil {
		return CreatePlanWithWOsResult{}, m.err
	}
	return m.result, nil
}

// ── mockSKUChecker ──────────────────────────────────────────────────────────

type mockSKUChecker struct {
	err error
}

func (m *mockSKUChecker) GetSKU(_ context.Context, id uuid.UUID) (SKUInfo, error) {
	if m.err != nil {
		return SKUInfo{}, m.err
	}
	return SKUInfo{ID: id, Code: "SKU-1", Name: "test sku"}, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func newSvc(st store, sku SKUChecker, sp ProductionSplitter) *service {
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	return &service{s: st, skuChecker: sku, splitter: sp, now: func() time.Time { return now }}
}

func validSOInput(skuID uuid.UUID) CreateSOInput {
	return CreateSOInput{
		CustomerID: uuid.New(),
		Currency:   "VND",
		Lines: []CreateSOLineInput{
			{SKUID: skuID, QtyOrdered: 10, UnitPrice: domain.Money{Amount: 50_000, Currency: "VND"}},
		},
		CreatedBy: uuid.New(),
	}
}

// ── CreateCustomer ──────────────────────────────────────────────────────────

func TestCreateCustomer_AutoCode_DrawsFromSequence(t *testing.T) {
	st := &mockStore{nextCustomerCodeResult: "KH001"}
	svc := newSvc(st, nil, nil)

	c, err := svc.CreateCustomer(context.Background(), CreateCustomerInput{Name: "ACME Co."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Code != "KH001" {
		t.Errorf("Code = %q, want KH001", c.Code)
	}
	if !c.IsActive {
		t.Error("new customer must default to is_active = true")
	}
}

func TestCreateCustomer_ProvidedCodeUnique_Inserts(t *testing.T) {
	st := &mockStore{customerCodeExistsResult: false}
	svc := newSvc(st, nil, nil)

	c, err := svc.CreateCustomer(context.Background(), CreateCustomerInput{Code: "VIP-9", Name: "ACME"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Code != "VIP-9" {
		t.Errorf("Code = %q, want VIP-9", c.Code)
	}
}

func TestCreateCustomer_ProvidedCodeCollision_ReturnsErrInvalidInput(t *testing.T) {
	st := &mockStore{customerCodeExistsResult: true}
	svc := newSvc(st, nil, nil)

	_, err := svc.CreateCustomer(context.Background(), CreateCustomerInput{Code: "DUP", Name: "ACME"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on duplicate code, got %v", err)
	}
}

func TestCreateCustomer_EmptyName_Rejected(t *testing.T) {
	svc := newSvc(&mockStore{}, nil, nil)
	_, err := svc.CreateCustomer(context.Background(), CreateCustomerInput{})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on empty name, got %v", err)
	}
}

func TestCreateCustomer_NormalisesCountryCode(t *testing.T) {
	st := &mockStore{nextCustomerCodeResult: "KH001"}
	svc := newSvc(st, nil, nil)
	c, err := svc.CreateCustomer(context.Background(), CreateCustomerInput{Name: "X", CountryCode: "vn"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.CountryCode != "VN" {
		t.Errorf("CountryCode = %q, want VN", c.CountryCode)
	}
}

// ── PatchCustomer ───────────────────────────────────────────────────────────

func TestPatchCustomer_NameEmpty_Rejected(t *testing.T) {
	st := &mockStore{selectCustomerByIDResult: Customer{ID: uuid.New(), Name: "old"}}
	svc := newSvc(st, nil, nil)
	empty := "   "
	_, err := svc.PatchCustomer(context.Background(), PatchCustomerInput{ID: uuid.New(), Name: &empty})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on whitespace name, got %v", err)
	}
}

func TestPatchCustomer_DeactivatesViaIsActiveFalse(t *testing.T) {
	id := uuid.New()
	st := &mockStore{selectCustomerByIDResult: Customer{ID: id, Name: "old", IsActive: true}}
	svc := newSvc(st, nil, nil)
	off := false
	c, err := svc.PatchCustomer(context.Background(), PatchCustomerInput{ID: id, IsActive: &off})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.IsActive {
		t.Error("expected IsActive=false after patch")
	}
}

// ── CreateSO ────────────────────────────────────────────────────────────────

func TestCreateSO_HappyPath(t *testing.T) {
	skuID := uuid.New()
	st := &mockStore{nextSOCodeResult: "SO20260525-001"}
	svc := newSvc(st, &mockSKUChecker{}, nil)

	so, err := svc.CreateSO(context.Background(), validSOInput(skuID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.Code != "SO20260525-001" {
		t.Errorf("Code = %q, want SO20260525-001", so.Code)
	}
	if so.Status != SOStatusDraft {
		t.Errorf("Status = %q, want DRAFT", so.Status)
	}
	if len(so.Lines) != 1 || so.Lines[0].SKUID != skuID {
		t.Errorf("expected 1 line for skuID, got %+v", so.Lines)
	}
}

func TestCreateSO_DefaultsCurrencyToVND(t *testing.T) {
	st := &mockStore{nextSOCodeResult: "SO20260525-001"}
	svc := newSvc(st, &mockSKUChecker{}, nil)

	in := validSOInput(uuid.New())
	in.Currency = ""
	so, err := svc.CreateSO(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.Currency != "VND" {
		t.Errorf("Currency = %q, want VND", so.Currency)
	}
}

func TestCreateSO_RejectsNilCustomer(t *testing.T) {
	svc := newSvc(&mockStore{}, &mockSKUChecker{}, nil)
	in := validSOInput(uuid.New())
	in.CustomerID = uuid.Nil
	_, err := svc.CreateSO(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on nil customer, got %v", err)
	}
}

func TestCreateSO_RejectsZeroLines(t *testing.T) {
	svc := newSvc(&mockStore{}, &mockSKUChecker{}, nil)
	in := validSOInput(uuid.New())
	in.Lines = nil
	_, err := svc.CreateSO(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on zero lines, got %v", err)
	}
}

func TestCreateSO_RejectsZeroQty(t *testing.T) {
	svc := newSvc(&mockStore{}, &mockSKUChecker{}, nil)
	in := validSOInput(uuid.New())
	in.Lines[0].QtyOrdered = 0
	_, err := svc.CreateSO(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on zero qty, got %v", err)
	}
}

func TestCreateSO_RejectsDuplicateSKU(t *testing.T) {
	skuID := uuid.New()
	svc := newSvc(&mockStore{}, &mockSKUChecker{}, nil)
	in := validSOInput(skuID)
	in.Lines = append(in.Lines, CreateSOLineInput{SKUID: skuID, QtyOrdered: 5, UnitPrice: domain.Money{Amount: 1, Currency: "VND"}})
	_, err := svc.CreateSO(context.Background(), in)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on duplicate SKU, got %v", err)
	}
}

func TestCreateSO_PropagatesSKUCheckerError(t *testing.T) {
	missing := domain.NewBizError(domain.ErrNotFound, "sku missing")
	svc := newSvc(&mockStore{}, &mockSKUChecker{err: missing}, nil)
	_, err := svc.CreateSO(context.Background(), validSOInput(uuid.New()))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound from sku checker, got %v", err)
	}
}

// ── PatchSO BR-S01 ──────────────────────────────────────────────────────────

func TestPatchSO_NonDraft_Rejected(t *testing.T) {
	st := &mockStore{selectSOByIDResult: SalesOrder{ID: uuid.New(), Status: SOStatusConfirmed}}
	svc := newSvc(st, &mockSKUChecker{}, nil)
	_, err := svc.PatchSO(context.Background(), PatchSOInput{ID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on non-DRAFT, got %v", err)
	}
}

func TestPatchSO_DraftReplaceLines(t *testing.T) {
	soID := uuid.New()
	st := &mockStore{selectSOByIDResult: SalesOrder{ID: soID, Status: SOStatusDraft, Currency: "VND"}}
	svc := newSvc(st, &mockSKUChecker{}, nil)

	newLines := []CreateSOLineInput{
		{SKUID: uuid.New(), QtyOrdered: 4, UnitPrice: domain.Money{Amount: 12_000, Currency: "VND"}},
	}
	so, err := svc.PatchSO(context.Background(), PatchSOInput{ID: soID, Lines: &newLines})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(so.Lines) != 1 || so.Lines[0].QtyOrdered != 4 {
		t.Errorf("lines not replaced: %+v", so.Lines)
	}
}

func TestPatchSO_ClearExpectedShipDate_NilsField(t *testing.T) {
	when := time.Now().UTC()
	st := &mockStore{selectSOByIDResult: SalesOrder{
		ID: uuid.New(), Status: SOStatusDraft, Currency: "VND", ExpectedShipDate: &when,
	}}
	svc := newSvc(st, &mockSKUChecker{}, nil)
	so, err := svc.PatchSO(context.Background(), PatchSOInput{ID: uuid.New(), ClearExpectedShipDate: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.ExpectedShipDate != nil {
		t.Errorf("ExpectedShipDate should be nil, got %v", so.ExpectedShipDate)
	}
}

// ── ConfirmSO BR-S05 ────────────────────────────────────────────────────────

func TestConfirmSO_DomesticVND_NoExportFields(t *testing.T) {
	st := &mockStore{selectSOByIDResult: SalesOrder{
		ID: uuid.New(), Status: SOStatusDraft, Currency: "VND", CustomerCountry: "VN",
	}}
	svc := newSvc(st, nil, nil)
	if err := svc.ConfirmSO(context.Background(), uuid.New()); err != nil {
		t.Errorf("domestic VND confirm should pass: %v", err)
	}
	if st.lastStatusUpdate != SOStatusConfirmed {
		t.Errorf("status update = %q, want CONFIRMED", st.lastStatusUpdate)
	}
}

func TestConfirmSO_ExportMissingIncoterm_Rejected(t *testing.T) {
	st := &mockStore{selectSOByIDResult: SalesOrder{
		ID: uuid.New(), Status: SOStatusDraft, Currency: "USD", CustomerCountry: "US",
	}}
	svc := newSvc(st, nil, nil)
	err := svc.ConfirmSO(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on missing export fields, got %v", err)
	}
}

func TestConfirmSO_ExportComplete_Confirms(t *testing.T) {
	st := &mockStore{selectSOByIDResult: SalesOrder{
		ID: uuid.New(), Status: SOStatusDraft, Currency: "USD", CustomerCountry: "US",
		Incoterm: "FOB", PortOfLoading: "HCMC", PortOfDischarge: "LA",
	}}
	svc := newSvc(st, nil, nil)
	if err := svc.ConfirmSO(context.Background(), uuid.New()); err != nil {
		t.Errorf("complete export confirm should pass: %v", err)
	}
}

func TestConfirmSO_NonDraft_Rejected(t *testing.T) {
	st := &mockStore{selectSOByIDResult: SalesOrder{
		ID: uuid.New(), Status: SOStatusConfirmed, Currency: "VND",
	}}
	svc := newSvc(st, nil, nil)
	err := svc.ConfirmSO(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on non-DRAFT confirm, got %v", err)
	}
}

// ── CancelSO BR-S04 ─────────────────────────────────────────────────────────

func TestCancelSO_QtyShippedZero_Cancels(t *testing.T) {
	id := uuid.New()
	st := &mockStore{
		selectSOByIDResult:        SalesOrder{ID: id, Status: SOStatusConfirmed},
		selectSOLinesBySOIDResult: []SalesOrderLine{{ID: uuid.New(), QtyShipped: 0}},
	}
	svc := newSvc(st, nil, nil)
	if err := svc.CancelSO(context.Background(), CancelSOInput{ID: id}); err != nil {
		t.Errorf("cancel should pass: %v", err)
	}
	if st.lastStatusUpdate != SOStatusCancelled {
		t.Errorf("status = %q, want CANCELLED", st.lastStatusUpdate)
	}
}

func TestCancelSO_AnyLineShipped_Rejected(t *testing.T) {
	st := &mockStore{
		selectSOByIDResult: SalesOrder{ID: uuid.New(), Status: SOStatusInProduction},
		selectSOLinesBySOIDResult: []SalesOrderLine{
			{ID: uuid.New(), QtyShipped: 0},
			{ID: uuid.New(), QtyShipped: 3},
		},
	}
	svc := newSvc(st, nil, nil)
	err := svc.CancelSO(context.Background(), CancelSOInput{ID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on shipped line, got %v", err)
	}
}

func TestCancelSO_AlreadyShipped_Rejected(t *testing.T) {
	st := &mockStore{selectSOByIDResult: SalesOrder{ID: uuid.New(), Status: SOStatusShipped}}
	svc := newSvc(st, nil, nil)
	err := svc.CancelSO(context.Background(), CancelSOInput{ID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on SHIPPED, got %v", err)
	}
}

func TestCancelSO_AlreadyCancelled_Rejected(t *testing.T) {
	st := &mockStore{selectSOByIDResult: SalesOrder{ID: uuid.New(), Status: SOStatusCancelled}}
	svc := newSvc(st, nil, nil)
	err := svc.CancelSO(context.Background(), CancelSOInput{ID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on already-cancelled, got %v", err)
	}
}

// ── SplitToPlan ─────────────────────────────────────────────────────────────

func TestSplitToPlan_NilSplitter_PreconditionFailed(t *testing.T) {
	svc := newSvc(&mockStore{}, nil, nil)
	_, err := svc.SplitToPlan(context.Background(), SplitToPlanInput{
		SalesOrderID: uuid.New(),
		Allocations:  []SplitAllocation{{SOLineID: uuid.New(), Quantity: 1}},
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed when splitter nil, got %v", err)
	}
}

func TestSplitToPlan_EmptyAllocations_Rejected(t *testing.T) {
	svc := newSvc(&mockStore{}, nil, &mockSplitter{})
	_, err := svc.SplitToPlan(context.Background(), SplitToPlanInput{SalesOrderID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on zero allocations, got %v", err)
	}
}

func TestSplitToPlan_HappyPath_BumpsQtyAndAutoFlips(t *testing.T) {
	soID := uuid.New()
	lineID := uuid.New()
	skuID := uuid.New()

	tx := &mockTxStore{
		lockSOForUpdateResult:    SalesOrder{ID: soID, Status: SOStatusConfirmed, ExpectedShipDate: nil},
		lockAndReadSOLinesResult: []SalesOrderLine{{ID: lineID, SalesOrderID: soID, SKUID: skuID, QtyOrdered: 10, QtyPlanned: 0}},
	}
	st := &mockStore{tx: tx}
	planID := uuid.New()
	woID := uuid.New()
	splitter := &mockSplitter{result: CreatePlanWithWOsResult{
		PlanID: planID, PlanCode: "PL-2026-001", WorkOrderIDs: []uuid.UUID{woID},
	}}
	svc := newSvc(st, nil, splitter)

	res, err := svc.SplitToPlan(context.Background(), SplitToPlanInput{
		SalesOrderID: soID,
		Allocations:  []SplitAllocation{{SOLineID: lineID, Quantity: 4}},
		ActorID:      uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.PlanID != planID {
		t.Errorf("PlanID = %v, want %v", res.PlanID, planID)
	}
	if got := splitter.calledWith.Items; len(got) != 1 || got[0].SKUID != skuID || got[0].Quantity != 4 {
		t.Errorf("splitter items = %+v, want sku=%v qty=4", got, skuID)
	}
	if len(tx.incrementCalls) != 1 || tx.incrementCalls[0].Delta != 4 {
		t.Errorf("increment calls = %+v, want one call with delta=4", tx.incrementCalls)
	}
	if len(tx.updateStatusCalls) != 1 || tx.updateStatusCalls[0].Target != SOStatusInProduction {
		t.Errorf("expected one auto-flip CONFIRMED→IN_PRODUCTION, got %+v", tx.updateStatusCalls)
	}
}

func TestSplitToPlan_OverflowsOrdered_Rejected(t *testing.T) {
	soID := uuid.New()
	lineID := uuid.New()
	tx := &mockTxStore{
		lockSOForUpdateResult:    SalesOrder{ID: soID, Status: SOStatusConfirmed},
		lockAndReadSOLinesResult: []SalesOrderLine{{ID: lineID, SalesOrderID: soID, QtyOrdered: 5, QtyPlanned: 4}},
	}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, &mockSplitter{})

	_, err := svc.SplitToPlan(context.Background(), SplitToPlanInput{
		SalesOrderID: soID,
		Allocations:  []SplitAllocation{{SOLineID: lineID, Quantity: 2}}, // 4+2 > 5
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on qty overflow, got %v", err)
	}
}

func TestSplitToPlan_LineBelongsToDifferentSO_Rejected(t *testing.T) {
	soID := uuid.New()
	lineID := uuid.New()
	tx := &mockTxStore{
		lockSOForUpdateResult: SalesOrder{ID: soID, Status: SOStatusConfirmed},
		lockAndReadSOLinesResult: []SalesOrderLine{
			{ID: lineID, SalesOrderID: uuid.New(), QtyOrdered: 10}, // different SO
		},
	}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, &mockSplitter{})

	_, err := svc.SplitToPlan(context.Background(), SplitToPlanInput{
		SalesOrderID: soID,
		Allocations:  []SplitAllocation{{SOLineID: lineID, Quantity: 1}},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on cross-SO line, got %v", err)
	}
}

func TestSplitToPlan_StatusDraft_Rejected(t *testing.T) {
	tx := &mockTxStore{lockSOForUpdateResult: SalesOrder{Status: SOStatusDraft}}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, &mockSplitter{})
	_, err := svc.SplitToPlan(context.Background(), SplitToPlanInput{
		SalesOrderID: uuid.New(),
		Allocations:  []SplitAllocation{{SOLineID: uuid.New(), Quantity: 1}},
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on DRAFT split, got %v", err)
	}
}

func TestSplitToPlan_SplitterFails_NoQtyMutation(t *testing.T) {
	soID := uuid.New()
	lineID := uuid.New()
	tx := &mockTxStore{
		lockSOForUpdateResult:    SalesOrder{ID: soID, Status: SOStatusConfirmed},
		lockAndReadSOLinesResult: []SalesOrderLine{{ID: lineID, SalesOrderID: soID, QtyOrdered: 10}},
	}
	st := &mockStore{tx: tx}
	splitter := &mockSplitter{err: errors.New("planning down")}
	svc := newSvc(st, nil, splitter)

	_, err := svc.SplitToPlan(context.Background(), SplitToPlanInput{
		SalesOrderID: soID,
		Allocations:  []SplitAllocation{{SOLineID: lineID, Quantity: 3}},
	})
	if err == nil {
		t.Fatal("expected splitter error to propagate")
	}
	if len(tx.incrementCalls) != 0 {
		t.Errorf("qty_planned must NOT be bumped when splitter fails, got %+v", tx.incrementCalls)
	}
}

func TestSplitToPlan_QtyPlannedTxFails_FlagsOrphan(t *testing.T) {
	soID := uuid.New()
	lineID := uuid.New()

	// First withTx call (Phase 1 lock+validate) succeeds; second (Phase 3 bump)
	// fails. We model this with two-shot mockStore via a counter.
	tx1 := &mockTxStore{
		lockSOForUpdateResult:    SalesOrder{ID: soID, Status: SOStatusConfirmed},
		lockAndReadSOLinesResult: []SalesOrderLine{{ID: lineID, SalesOrderID: soID, QtyOrdered: 10}},
	}
	tx2 := &mockTxStore{incrementQtyPlannedErr: errors.New("constraint violated")}

	calls := 0
	st := &twoShotStore{first: tx1, second: tx2, calls: &calls}
	splitter := &mockSplitter{result: CreatePlanWithWOsResult{
		PlanID: uuid.New(), PlanCode: "PL-ORPHAN-001", WorkOrderIDs: []uuid.UUID{uuid.New()},
	}}
	svc := newSvc(st, nil, splitter)

	_, err := svc.SplitToPlan(context.Background(), SplitToPlanInput{
		SalesOrderID: soID,
		Allocations:  []SplitAllocation{{SOLineID: lineID, Quantity: 3}},
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed flagging orphan plan, got %v", err)
	}
	if msg := err.Error(); msg == "" || !contains(msg, "PL-ORPHAN-001") {
		t.Errorf("error must reference the orphaned plan code, got %q", msg)
	}
}

// twoShotStore is a tiny harness for SplitToPlan's two-tx flow: hand back
// `first` on call 1 (Phase 1) and `second` on call 2 (Phase 3) so the test
// can simulate a Phase-1-ok / Phase-3-fail trajectory.
type twoShotStore struct {
	mockStore
	first  *mockTxStore
	second *mockTxStore
	calls  *int
}

func (s *twoShotStore) withTx(_ context.Context, fn func(tx txStore) error) error {
	*s.calls++
	if *s.calls == 1 {
		return fn(s.first)
	}
	return fn(s.second)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestPickSplitDeadline_Override(t *testing.T) {
	override := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	got := pickSplitDeadline(override, nil)
	if got == nil || !got.Equal(override) {
		t.Errorf("override should win, got %v", got)
	}
}

func TestPickSplitDeadline_InheritsSOShipDate(t *testing.T) {
	ship := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	got := pickSplitDeadline(time.Time{}, &ship)
	if got == nil || !got.Equal(ship) {
		t.Errorf("should inherit SO ship date, got %v", got)
	}
}

func TestPickSplitDeadline_NilWhenBothMissing(t *testing.T) {
	if got := pickSplitDeadline(time.Time{}, nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// ── CustomerSKUMapping (#304) ───────────────────────────────────────────────

func TestCreateCustomerSKUMapping_HappyPath(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, &mockSKUChecker{}, nil)

	customerID := uuid.New()
	skuID := uuid.New()
	actor := uuid.New()
	m, err := svc.CreateCustomerSKUMapping(context.Background(), CreateCustomerSKUMappingInput{
		CustomerID:      customerID,
		CustomerSKUCode: "  ACME-RED-01  ",
		SKUID:           skuID,
		Notes:           "  red marble  ",
		ActorID:         actor,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.CustomerSKUCode != "ACME-RED-01" {
		t.Errorf("CustomerSKUCode = %q, want trimmed ACME-RED-01", m.CustomerSKUCode)
	}
	if m.Notes != "red marble" {
		t.Errorf("Notes = %q, want trimmed", m.Notes)
	}
	if st.insertedCSM == nil || st.insertedCSM.SKUID != skuID || st.insertedCSM.CustomerID != customerID {
		t.Errorf("insertCustomerSKUMapping not called with expected payload: %+v", st.insertedCSM)
	}
	if st.insertedCSM.CreatedBy == nil || *st.insertedCSM.CreatedBy != actor {
		t.Errorf("CreatedBy = %v, want %v", st.insertedCSM.CreatedBy, actor)
	}
}

func TestCreateCustomerSKUMapping_NilCustomer_Rejected(t *testing.T) {
	svc := newSvc(&mockStore{}, &mockSKUChecker{}, nil)
	_, err := svc.CreateCustomerSKUMapping(context.Background(), CreateCustomerSKUMappingInput{
		CustomerSKUCode: "X",
		SKUID:           uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on nil customer_id, got %v", err)
	}
}

func TestCreateCustomerSKUMapping_UnknownSKU_Rejected(t *testing.T) {
	missing := domain.NewBizError(domain.ErrNotFound, "sku missing")
	svc := newSvc(&mockStore{}, &mockSKUChecker{err: missing}, nil)
	_, err := svc.CreateCustomerSKUMapping(context.Background(), CreateCustomerSKUMappingInput{
		CustomerID:      uuid.New(),
		CustomerSKUCode: "X",
		SKUID:           uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput when SKU is unknown, got %v", err)
	}
}

func TestCreateCustomerSKUMapping_DuplicatePKPropagates(t *testing.T) {
	dup := domain.NewBizError(domain.ErrInvalidInput, "mapping already exists")
	st := &mockStore{insertCSMErr: dup}
	svc := newSvc(st, &mockSKUChecker{}, nil)

	_, err := svc.CreateCustomerSKUMapping(context.Background(), CreateCustomerSKUMappingInput{
		CustomerID:      uuid.New(),
		CustomerSKUCode: "DUP",
		SKUID:           uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput surface from store dup, got %v", err)
	}
	if st.insertedCSM != nil {
		t.Error("insertedCSM should remain nil when store rejects insert")
	}
}

func TestBulkImportCustomerSKUMappings_FailAll_OnRowError(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, &mockSKUChecker{}, nil)

	customerID := uuid.New()
	res, err := svc.BulkImportCustomerSKUMappings(context.Background(), BulkImportCustomerSKUMappingsInput{
		CustomerID: customerID,
		Rows: []BulkMappingRow{
			{CustomerSKUCode: "A", SKUID: uuid.New()},
			{CustomerSKUCode: "", SKUID: uuid.New()},     // invalid: blank code
			{CustomerSKUCode: "A", SKUID: uuid.New()},     // duplicate of row 1
		},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput on row error, got %v", err)
	}
	if res.Inserted != 0 {
		t.Errorf("Inserted = %d, want 0 (fail-all)", res.Inserted)
	}
	if len(res.Errors) != 2 {
		t.Errorf("expected 2 row errors, got %+v", res.Errors)
	}
	if len(st.bulkInsertCSMRows) != 0 {
		t.Errorf("bulk insert must not run when validation finds errors, got %d rows", len(st.bulkInsertCSMRows))
	}
}

func TestBulkImportCustomerSKUMappings_HappyPath(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, &mockSKUChecker{}, nil)

	customerID := uuid.New()
	rows := []BulkMappingRow{
		{CustomerSKUCode: "A", SKUID: uuid.New()},
		{CustomerSKUCode: "B", SKUID: uuid.New()},
	}
	res, err := svc.BulkImportCustomerSKUMappings(context.Background(), BulkImportCustomerSKUMappingsInput{
		CustomerID: customerID,
		Rows:       rows,
		ActorID:    uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Inserted != 2 {
		t.Errorf("Inserted = %d, want 2", res.Inserted)
	}
	if len(st.bulkInsertCSMRows) != 2 {
		t.Errorf("store should have received 2 rows, got %d", len(st.bulkInsertCSMRows))
	}
}
