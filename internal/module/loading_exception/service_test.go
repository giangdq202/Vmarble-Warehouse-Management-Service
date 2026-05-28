package loading_exception

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── mocks ────────────────────────────────────────────────────────────────────

type mockStore struct {
	insertedRows []LoadingException
	insertErr    error

	selectByIDResult LoadingException
	selectByIDErr    error

	selectByContainerKeysetResult []LoadingException
	selectByContainerKeysetErr    error

	pendingByContainerResult PendingSummary
	pendingByContainerErr    error

	selectCrossContainerKeysetResult []LoadingException
	selectCrossContainerKeysetErr    error

	crossContainerSummaryResult CrossContainerSummary
	crossContainerSummaryErr    error

	tx        *mockTxStore
	withTxErr error
}

func (m *mockStore) insert(_ context.Context, e LoadingException) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.insertedRows = append(m.insertedRows, e)
	return nil
}

func (m *mockStore) selectByID(_ context.Context, _ uuid.UUID) (LoadingException, error) {
	return m.selectByIDResult, m.selectByIDErr
}

func (m *mockStore) selectByContainerKeyset(_ context.Context, _ uuid.UUID, _ string, _ httpkit.Cursor, _ int) ([]LoadingException, error) {
	return m.selectByContainerKeysetResult, m.selectByContainerKeysetErr
}

func (m *mockStore) pendingByContainer(_ context.Context, _ uuid.UUID) (PendingSummary, error) {
	return m.pendingByContainerResult, m.pendingByContainerErr
}

func (m *mockStore) selectCrossContainerKeyset(_ context.Context, _ CrossContainerFilter, _ httpkit.Cursor, _ int) ([]LoadingException, error) {
	return m.selectCrossContainerKeysetResult, m.selectCrossContainerKeysetErr
}

func (m *mockStore) crossContainerSummary(_ context.Context, _ CrossContainerFilter) (CrossContainerSummary, error) {
	return m.crossContainerSummaryResult, m.crossContainerSummaryErr
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

type mockTxStore struct {
	lockResult    LoadingException
	lockResults   []LoadingException
	lockCallIdx   int
	lockErr       error
	approveCalled bool
	approvedRow   approveRow
	approveErr    error
	rejectCalled  bool
	rejectedRow   rejectRow
	rejectErr     error
}

func (m *mockTxStore) lockForUpdate(_ context.Context, _ uuid.UUID) (LoadingException, error) {
	if m.lockErr != nil {
		return LoadingException{}, m.lockErr
	}
	if len(m.lockResults) > 0 {
		idx := m.lockCallIdx
		if idx >= len(m.lockResults) {
			idx = len(m.lockResults) - 1
		}
		m.lockCallIdx++
		return m.lockResults[idx], nil
	}
	return m.lockResult, nil
}

func (m *mockTxStore) approve(_ context.Context, in approveRow) error {
	if m.approveErr != nil {
		return m.approveErr
	}
	m.approveCalled = true
	m.approvedRow = in
	return nil
}

func (m *mockTxStore) reject(_ context.Context, in rejectRow) error {
	if m.rejectErr != nil {
		return m.rejectErr
	}
	m.rejectCalled = true
	m.rejectedRow = in
	return nil
}

type mockSKU struct {
	getErr error
}

func (m *mockSKU) GetSKU(_ context.Context, id uuid.UUID) (SKUInfo, error) {
	if m.getErr != nil {
		return SKUInfo{}, m.getErr
	}
	return SKUInfo{ID: id, Code: "SKU-1", Name: "Test"}, nil
}

type mockCarryOver struct {
	called    bool
	gotInput  CarryOverInput
	resultID  uuid.UUID
	createErr error
}

func (m *mockCarryOver) CreateCarryOver(_ context.Context, in CarryOverInput) (uuid.UUID, error) {
	if m.createErr != nil {
		return uuid.Nil, m.createErr
	}
	m.called = true
	m.gotInput = in
	if m.resultID == uuid.Nil {
		m.resultID = uuid.New()
	}
	return m.resultID, nil
}

type mockAudit struct {
	logged []AuditInput
	err    error
}

func (m *mockAudit) LogException(_ context.Context, in AuditInput) error {
	if m.err != nil {
		return m.err
	}
	m.logged = append(m.logged, in)
	return nil
}

type mockNotifier struct {
	created  []NotifyCreatedInput
	approved []NotifyApprovedInput
	rejected []NotifyRejectedInput
	err      error
}

func (m *mockNotifier) NotifyCreated(_ context.Context, in NotifyCreatedInput) error {
	if m.err != nil {
		return m.err
	}
	m.created = append(m.created, in)
	return nil
}

func (m *mockNotifier) NotifyApproved(_ context.Context, in NotifyApprovedInput) error {
	if m.err != nil {
		return m.err
	}
	m.approved = append(m.approved, in)
	return nil
}

func (m *mockNotifier) NotifyRejected(_ context.Context, in NotifyRejectedInput) error {
	if m.err != nil {
		return m.err
	}
	m.rejected = append(m.rejected, in)
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newTestSvc(s store, sku SKUChecker, carry CarryOverCreator, audit AuditLogger) *service {
	return &service{
		s:          s,
		skuChecker: sku,
		carryOver:  carry,
		audit:      audit,
		now:        func() time.Time { return time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC) },
		newID:      func() uuid.UUID { return uuid.MustParse("11111111-1111-1111-1111-111111111111") },
	}
}

func newTestSvcWithNotifier(s store, sku SKUChecker, carry CarryOverCreator, audit AuditLogger, notifier ExceptionNotifier) *service {
	svc := newTestSvc(s, sku, carry, audit)
	svc.notifier = notifier
	return svc
}

func intPtr(v int) *int              { return &v }
func uuidPtr(v uuid.UUID) *uuid.UUID { return &v }

// ── Create: 1 case per ExceptionType (DoD: 7 cases) ─────────────────────────

func TestService_Create_AllTypes(t *testing.T) {
	cases := []struct {
		name    string
		excType string
		qty     *int
		skuID   *uuid.UUID
	}{
		{"short_shipped", TypeShortShipped, intPtr(2), uuidPtr(uuid.New())},
		{"over_loaded", TypeOverLoaded, intPtr(1), uuidPtr(uuid.New())},
		{"wrong_sku", TypeWrongSKU, intPtr(1), uuidPtr(uuid.New())},
		{"substitution", TypeSubstitution, intPtr(1), uuidPtr(uuid.New())},
		{"damaged_at_loading", TypeDamagedAtLoading, intPtr(3), uuidPtr(uuid.New())},
		{"unplanned_unit", TypeUnplannedUnit, intPtr(1), uuidPtr(uuid.New())},
		{"customer_change", TypeCustomerChange, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &mockStore{}
			au := &mockAudit{}
			svc := newTestSvc(st, &mockSKU{}, nil, au)

			out, err := svc.Create(context.Background(), CreateInput{
				ContainerID:   uuid.New(),
				ExceptionType: tc.excType,
				SKUID:         tc.skuID,
				Qty:           tc.qty,
				Reason:        "test reason",
				CreatedBy:     uuid.New(),
			})
			if err != nil {
				t.Fatalf("Create(%s) err = %v", tc.excType, err)
			}
			if out.ExceptionType != tc.excType {
				t.Fatalf("ExceptionType = %q, want %q", out.ExceptionType, tc.excType)
			}
			if len(st.insertedRows) != 1 {
				t.Fatalf("insertedRows = %d, want 1", len(st.insertedRows))
			}
			if len(au.logged) != 1 || au.logged[0].Action != AuditActionCreated {
				t.Fatalf("audit not LE_CREATED, got %+v", au.logged)
			}
		})
	}
}

func TestService_Create_RejectsInvalidType(t *testing.T) {
	svc := newTestSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.Create(context.Background(), CreateInput{
		ContainerID:   uuid.New(),
		ExceptionType: "BOGUS",
		Reason:        "x",
		CreatedBy:     uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestService_Create_RequiresFields(t *testing.T) {
	svc := newTestSvc(&mockStore{}, nil, nil, nil)
	cases := []struct {
		name string
		in   CreateInput
	}{
		{"missing_container", CreateInput{ExceptionType: TypeShortShipped, Reason: "x", CreatedBy: uuid.New()}},
		{"missing_reason", CreateInput{ExceptionType: TypeShortShipped, ContainerID: uuid.New(), CreatedBy: uuid.New()}},
		{"missing_creator", CreateInput{ExceptionType: TypeShortShipped, ContainerID: uuid.New(), Reason: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), tc.in)
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

// ── Approve ──────────────────────────────────────────────────────────────────

func TestService_Approve_Backorder_CreatesCarryOver(t *testing.T) {
	soLineID := uuid.New()
	carryID := uuid.New()
	current := LoadingException{
		ID:            uuid.New(),
		ContainerID:   uuid.New(),
		ExceptionType: TypeShortShipped,
		Qty:           intPtr(5),
	}
	st := &mockStore{tx: &mockTxStore{lockResult: current}}
	carry := &mockCarryOver{resultID: carryID}
	au := &mockAudit{}
	svc := newTestSvc(st, &mockSKU{}, carry, au)

	_, err := svc.Approve(context.Background(), ApproveInput{
		ID:             current.ID,
		Resolution:     ResolutionBackorder,
		ParentSOLineID: &soLineID,
		ApprovedBy:     uuid.New(),
	})
	if err != nil {
		t.Fatalf("Approve err = %v", err)
	}
	if !carry.called {
		t.Fatalf("CarryOver.CreateCarryOver was not invoked")
	}
	if carry.gotInput.Qty != 5 {
		t.Fatalf("carry qty = %d, want 5", carry.gotInput.Qty)
	}
	if !st.tx.approveCalled {
		t.Fatalf("tx.approve was not invoked")
	}
	if st.tx.approvedRow.CarryOverSOLineID == nil || *st.tx.approvedRow.CarryOverSOLineID != carryID {
		t.Fatalf("approvedRow.CarryOverSOLineID = %v, want %v", st.tx.approvedRow.CarryOverSOLineID, carryID)
	}
	if len(au.logged) != 1 || au.logged[0].Action != AuditActionApproved {
		t.Fatalf("audit not LE_APPROVED, got %+v", au.logged)
	}
}

func TestService_Approve_Backorder_RequiresParentSOLine(t *testing.T) {
	current := LoadingException{ID: uuid.New(), Qty: intPtr(5)}
	st := &mockStore{tx: &mockTxStore{lockResult: current}}
	svc := newTestSvc(st, nil, &mockCarryOver{}, nil)
	_, err := svc.Approve(context.Background(), ApproveInput{
		ID:         current.ID,
		Resolution: ResolutionBackorder,
		ApprovedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestService_Approve_SubstituteAccepted_RequiresSubSKU(t *testing.T) {
	current := LoadingException{ID: uuid.New()}
	st := &mockStore{tx: &mockTxStore{lockResult: current}}
	svc := newTestSvc(st, &mockSKU{}, nil, nil)
	_, err := svc.Approve(context.Background(), ApproveInput{
		ID:         current.ID,
		Resolution: ResolutionSubstituteAccepted,
		ApprovedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestService_Approve_SubstituteAccepted_StampsSubSKU(t *testing.T) {
	current := LoadingException{ID: uuid.New()}
	subSKU := uuid.New()
	st := &mockStore{tx: &mockTxStore{lockResult: current}}
	svc := newTestSvc(st, &mockSKU{}, nil, &mockAudit{})
	_, err := svc.Approve(context.Background(), ApproveInput{
		ID:              current.ID,
		Resolution:      ResolutionSubstituteAccepted,
		SubstituteSKUID: &subSKU,
		ApprovedBy:      uuid.New(),
	})
	if err != nil {
		t.Fatalf("Approve err = %v", err)
	}
	if st.tx.approvedRow.SubstituteSKUID == nil || *st.tx.approvedRow.SubstituteSKUID != subSKU {
		t.Fatalf("approvedRow.SubstituteSKUID = %v, want %v", st.tx.approvedRow.SubstituteSKUID, subSKU)
	}
}

func TestService_Approve_RejectsAlreadyApproved(t *testing.T) {
	already := uuid.New()
	current := LoadingException{ID: uuid.New(), ApprovedBy: &already}
	st := &mockStore{tx: &mockTxStore{lockResult: current}}
	svc := newTestSvc(st, nil, nil, nil)
	_, err := svc.Approve(context.Background(), ApproveInput{
		ID:         current.ID,
		Resolution: ResolutionWriteOff,
		ApprovedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
}

func TestService_Approve_WriteOff_NoSideEffects(t *testing.T) {
	current := LoadingException{ID: uuid.New()}
	st := &mockStore{tx: &mockTxStore{lockResult: current}}
	carry := &mockCarryOver{}
	svc := newTestSvc(st, nil, carry, &mockAudit{})
	_, err := svc.Approve(context.Background(), ApproveInput{
		ID:         current.ID,
		Resolution: ResolutionWriteOff,
		ApprovedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Approve err = %v", err)
	}
	if carry.called {
		t.Fatalf("CarryOver should not be invoked for WRITE_OFF")
	}
	if st.tx.approvedRow.CarryOverSOLineID != nil {
		t.Fatalf("CarryOverSOLineID should be nil for WRITE_OFF")
	}
}

func TestService_Approve_RejectsInvalidResolution(t *testing.T) {
	svc := newTestSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.Approve(context.Background(), ApproveInput{
		ID:         uuid.New(),
		Resolution: "BOGUS",
		ApprovedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// ── Reject ───────────────────────────────────────────────────────────────────

func TestService_Reject_OK(t *testing.T) {
	current := LoadingException{ID: uuid.New(), ContainerID: uuid.New(), ExceptionType: TypeWrongSKU}
	st := &mockStore{tx: &mockTxStore{lockResult: current}}
	au := &mockAudit{}
	svc := newTestSvc(st, nil, nil, au)
	_, err := svc.Reject(context.Background(), RejectInput{
		ID:         current.ID,
		Reason:     "not actually wrong",
		ApprovedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Reject err = %v", err)
	}
	if !st.tx.rejectCalled {
		t.Fatalf("tx.reject not invoked")
	}
	if len(au.logged) != 1 || au.logged[0].Action != AuditActionRejected {
		t.Fatalf("audit not LE_REJECTED, got %+v", au.logged)
	}
}

func TestService_Reject_RequiresReason(t *testing.T) {
	svc := newTestSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.Reject(context.Background(), RejectInput{
		ID:         uuid.New(),
		ApprovedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// ── PendingForContainer (BR-D14) ─────────────────────────────────────────────

func TestService_PendingForContainer(t *testing.T) {
	id := uuid.New()
	st := &mockStore{
		pendingByContainerResult: PendingSummary{Count: 2, IDs: []uuid.UUID{uuid.New(), uuid.New()}},
	}
	svc := newTestSvc(st, nil, nil, nil)
	out, err := svc.PendingForContainer(context.Background(), id)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.Count != 2 || len(out.IDs) != 2 {
		t.Fatalf("out = %+v, want Count=2 IDs=2", out)
	}
}

func TestService_PendingForContainer_RequiresID(t *testing.T) {
	svc := newTestSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.PendingForContainer(context.Background(), uuid.Nil)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// ── #328 cross-container list + summary ─────────────────────────────────────

func TestListCrossContainer_PassesFilterToStore(t *testing.T) {
	expected := []LoadingException{{ID: uuid.New(), CreatedAt: time.Now().UTC()}}
	st := &mockStore{selectCrossContainerKeysetResult: expected}
	svc := newTestSvc(st, nil, nil, nil)

	customerID := uuid.New()
	res, err := svc.ListCrossContainer(context.Background(),
		CrossContainerFilter{Status: "pending", CustomerID: &customerID, ExceptionType: TypeShortShipped},
		httpkit.CursorParams{Limit: 50},
	)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != expected[0].ID {
		t.Errorf("rows = %+v, want 1 row matching store result", res.Items)
	}
}

func TestCrossContainerSummary_DelegatesToStore(t *testing.T) {
	st := &mockStore{
		crossContainerSummaryResult: CrossContainerSummary{PendingCount: 7, BlockedContainers: 3},
	}
	svc := newTestSvc(st, nil, nil, nil)

	got, err := svc.CrossContainerSummary(context.Background(), CrossContainerFilter{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.PendingCount != 7 || got.BlockedContainers != 3 {
		t.Errorf("summary = %+v, want {7,3}", got)
	}
}

// ── #330 BulkApprove ────────────────────────────────────────────────────────

func TestBulkApprove_RejectsEmptyIDs(t *testing.T) {
	svc := newTestSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.BulkApprove(context.Background(), BulkApproveInput{
		Resolution: ResolutionWriteOff,
		ApprovedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBulkApprove_RejectsOver50IDs(t *testing.T) {
	ids := make([]uuid.UUID, 51)
	for i := range ids {
		ids[i] = uuid.New()
	}
	svc := newTestSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.BulkApprove(context.Background(), BulkApproveInput{
		IDs:        ids,
		Resolution: ResolutionWriteOff,
		ApprovedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBulkApprove_RejectsBackorderResolution(t *testing.T) {
	svc := newTestSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.BulkApprove(context.Background(), BulkApproveInput{
		IDs:        []uuid.UUID{uuid.New()},
		Resolution: ResolutionBackorder,
		ApprovedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// ── Notifier hooks ──────────────────────────────────────────────────────────

func TestCreate_FiresNotifyCreated(t *testing.T) {
	st := &mockStore{}
	notifier := &mockNotifier{}
	svc := newTestSvcWithNotifier(st, nil, nil, nil, notifier)

	containerID := uuid.New()
	_, err := svc.Create(context.Background(), CreateInput{
		ContainerID:   containerID,
		ExceptionType: TypeDamagedAtLoading,
		Reason:        "edge chipped",
		CreatedBy:     uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(notifier.created) != 1 {
		t.Fatalf("notifier.created = %d, want 1", len(notifier.created))
	}
	if notifier.created[0].ContainerID != containerID {
		t.Errorf("notify container_id = %v, want %v", notifier.created[0].ContainerID, containerID)
	}
	if notifier.created[0].ExceptionType != TypeDamagedAtLoading {
		t.Errorf("notify type = %q, want %q", notifier.created[0].ExceptionType, TypeDamagedAtLoading)
	}
}

func TestCreate_NotifyError_DoesNotAbortBusinessWrite(t *testing.T) {
	st := &mockStore{}
	notifier := &mockNotifier{err: errors.New("publish fail")}
	svc := newTestSvcWithNotifier(st, nil, nil, nil, notifier)

	_, err := svc.Create(context.Background(), CreateInput{
		ContainerID:   uuid.New(),
		ExceptionType: TypeDamagedAtLoading,
		Reason:        "edge chipped",
		CreatedBy:     uuid.New(),
	})
	if err != nil {
		t.Fatalf("publish failure must not abort Create, got %v", err)
	}
	if len(st.insertedRows) != 1 {
		t.Errorf("expected 1 inserted row even with publish error, got %d", len(st.insertedRows))
	}
}
