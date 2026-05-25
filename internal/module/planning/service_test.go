package planning

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── mockStore ─────────────────────────────────────────────────────────────────

type mockStore struct {
	// nextPlanCode
	nextPlanCodeSeq int
	nextPlanCodeErr error

	// insertPlan
	insertPlanErr error

	// selectPlans
	selectPlansResult []Plan
	selectPlansErr    error
	selectPlansSearch string
	selectPlansStatus string
	selectPlansFrom   *time.Time
	selectPlansTo     *time.Time

	// selectPlansLookup
	selectLookupResult       []PlanLookupItem
	selectLookupTotal        int
	selectLookupErr          error
	selectLookupSearch       string
	selectLookupStatus       string
	selectLookupDeadlineFrom *time.Time
	selectLookupDeadlineTo   *time.Time
	selectLookupLimit        int
	selectLookupOffset       int

	// selectPlanByID
	selectPlanByIDResult Plan
	selectPlanByIDErr    error

	// updatePlanStatus — captures what was last passed in for assertion
	updatePlanStatusCalled bool
	updatePlanStatusID     uuid.UUID
	updatePlanStatusValue  string
	updatePlanStatusErr    error

	// cancelPlanWithMetadata — captures audit-trail UPDATE
	cancelPlanWithMetadataCalled  bool
	cancelPlanWithMetadataID      uuid.UUID
	cancelPlanWithMetadataReason  string
	cancelPlanWithMetadataActorID uuid.UUID
	cancelPlanWithMetadataAt      time.Time
	cancelPlanWithMetadataErr     error

	// insertPlanItems
	insertPlanItemsErr error

	// selectPlanItemsByPlanID
	selectPlanItemsByPlanIDResult []PlanItem
	selectPlanItemsByPlanIDErr    error
}

func (m *mockStore) nextPlanCode(_ context.Context, year int) (string, error) {
	if m.nextPlanCodeErr != nil {
		return "", m.nextPlanCodeErr
	}
	m.nextPlanCodeSeq++
	return fmt.Sprintf("KH-%d-%03d", year, m.nextPlanCodeSeq), nil
}

func (m *mockStore) insertPlan(_ context.Context, _ Plan) error {
	return m.insertPlanErr
}

func (m *mockStore) selectPlansPaged(_ context.Context, p httpkit.PageParams, status string, createdFrom, createdTo *time.Time) ([]Plan, int, error) {
	m.selectPlansSearch = p.Search
	m.selectPlansStatus = status
	m.selectPlansFrom = createdFrom
	m.selectPlansTo = createdTo
	return m.selectPlansResult, len(m.selectPlansResult), m.selectPlansErr
}

func (m *mockStore) selectPlansLookup(_ context.Context, search, status string, deadlineFrom, deadlineTo *time.Time, limit, offset int) ([]PlanLookupItem, int, error) {
	m.selectLookupSearch = search
	m.selectLookupStatus = status
	m.selectLookupDeadlineFrom = deadlineFrom
	m.selectLookupDeadlineTo = deadlineTo
	m.selectLookupLimit = limit
	m.selectLookupOffset = offset
	total := m.selectLookupTotal
	if total == 0 {
		total = len(m.selectLookupResult)
	}
	return m.selectLookupResult, total, m.selectLookupErr
}

func (m *mockStore) selectPlanByID(_ context.Context, _ uuid.UUID) (Plan, error) {
	return m.selectPlanByIDResult, m.selectPlanByIDErr
}

func (m *mockStore) updatePlanStatus(_ context.Context, id uuid.UUID, status string) error {
	m.updatePlanStatusCalled = true
	m.updatePlanStatusID = id
	m.updatePlanStatusValue = status
	return m.updatePlanStatusErr
}

func (m *mockStore) insertPlanItems(_ context.Context, _ []PlanItem) error {
	return m.insertPlanItemsErr
}

func (m *mockStore) selectPlanItemsByPlanID(_ context.Context, _ uuid.UUID) ([]PlanItem, error) {
	return m.selectPlanItemsByPlanIDResult, m.selectPlanItemsByPlanIDErr
}

func (m *mockStore) cancelPlanWithMetadata(_ context.Context, id uuid.UUID, reason string, actorID uuid.UUID, at time.Time) error {
	m.cancelPlanWithMetadataCalled = true
	m.cancelPlanWithMetadataID = id
	m.cancelPlanWithMetadataReason = reason
	m.cancelPlanWithMetadataActorID = actorID
	m.cancelPlanWithMetadataAt = at
	return m.cancelPlanWithMetadataErr
}

// ── mockWOCanceller ───────────────────────────────────────────────────────────

type mockWOCanceller struct {
	listResult         []domain.WorkOrderStatus
	listErr            error
	listCalled         bool
	listCalledWithID   uuid.UUID
	cancelResult       int64
	cancelErr          error
	cancelCalled       bool
	cancelCalledWithID uuid.UUID
}

func (m *mockWOCanceller) ListStatusesByPlan(_ context.Context, planID uuid.UUID) ([]domain.WorkOrderStatus, error) {
	m.listCalled = true
	m.listCalledWithID = planID
	return m.listResult, m.listErr
}

func (m *mockWOCanceller) CancelPlannedByPlan(_ context.Context, planID uuid.UUID) (int64, error) {
	m.cancelCalled = true
	m.cancelCalledWithID = planID
	return m.cancelResult, m.cancelErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func validCreateInput(items ...PlanItemInput) CreatePlanInput {
	if len(items) == 0 {
		items = []PlanItemInput{
			{SKUID: uuid.New(), Quantity: 5},
		}
	}
	return CreatePlanInput{
		POID:  newPOID(),
		Items: items,
	}
}

func draftPlan(id uuid.UUID) Plan {
	return Plan{ID: id, POID: newPOID(), Status: domain.PlanDraft, CreatedAt: time.Now().UTC()}
}

// newPOID returns a fresh *uuid.UUID for tests where exactly one of POID/SOID
// must be non-nil per chk_plan_root. Inlining `id := uuid.New(); &id` would
// drown the table-driven tests in noise.
func newPOID() *uuid.UUID { id := uuid.New(); return &id }

// ── TestCreatePlan ────────────────────────────────────────────────────────────

func TestCreatePlan_HappyPath_ReturnsWithItems(t *testing.T) {
	skuID := uuid.New()
	in := validCreateInput(PlanItemInput{SKUID: skuID, Quantity: 10})

	svc := NewService(&mockStore{})
	plan, err := svc.CreatePlan(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.ID == uuid.Nil {
		t.Error("Plan ID must be set")
	}
	if plan.Status != domain.PlanDraft {
		t.Errorf("Status = %q, want DRAFT", plan.Status)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(plan.Items))
	}

	item := plan.Items[0]
	if item.ID == uuid.Nil {
		t.Error("PlanItem ID must be set")
	}
	if item.PlanID != plan.ID {
		t.Errorf("item.PlanID = %v, want %v", item.PlanID, plan.ID)
	}
	if item.SKUID != skuID {
		t.Errorf("item.SKUID = %v, want %v", item.SKUID, skuID)
	}
	if item.Quantity != 10 {
		t.Errorf("item.Quantity = %d, want 10", item.Quantity)
	}
}

func TestCreatePlan_CodeIsAutoGenerated(t *testing.T) {
	in := validCreateInput()
	svc := NewService(&mockStore{})

	plan, err := svc.CreatePlan(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Code == "" {
		t.Error("plan.Code must not be empty")
	}
	// Must follow pattern KH-YYYY-NNN
	year := time.Now().Year()
	prefix := fmt.Sprintf("KH-%d-", year)
	if !strings.HasPrefix(plan.Code, prefix) {
		t.Errorf("plan.Code = %q, want prefix %q", plan.Code, prefix)
	}
}

func TestCreatePlan_NextPlanCodeError_Propagates(t *testing.T) {
	dbErr := errors.New("sequence exhausted")
	st := &mockStore{nextPlanCodeErr: dbErr}
	svc := NewService(st)

	_, err := svc.CreatePlan(context.Background(), validCreateInput())
	if !errors.Is(err, dbErr) {
		t.Errorf("expected sequence error, got %v", err)
	}
}

func TestCreatePlan_MultipleItems_AllLinkedToPlan(t *testing.T) {
	in := validCreateInput(
		PlanItemInput{SKUID: uuid.New(), Quantity: 3},
		PlanItemInput{SKUID: uuid.New(), Quantity: 7},
	)

	svc := NewService(&mockStore{})
	plan, err := svc.CreatePlan(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(plan.Items))
	}
	for i, item := range plan.Items {
		if item.PlanID != plan.ID {
			t.Errorf("Items[%d].PlanID = %v, want %v", i, item.PlanID, plan.ID)
		}
		if item.ID == uuid.Nil {
			t.Errorf("Items[%d].ID must not be nil", i)
		}
	}
}

func TestCreatePlan_NoItems_ReturnsErrInvalidInput(t *testing.T) {
	in := CreatePlanInput{POID: newPOID(), Items: []PlanItemInput{}}

	svc := NewService(&mockStore{})
	_, err := svc.CreatePlan(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty items, got %v", err)
	}
}

func TestCreatePlan_NilItems_ReturnsErrInvalidInput(t *testing.T) {
	in := CreatePlanInput{POID: newPOID(), Items: nil}

	svc := NewService(&mockStore{})
	_, err := svc.CreatePlan(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for nil items, got %v", err)
	}
}

func TestCreatePlan_ZeroQuantityItem_ReturnsErrInvalidInput(t *testing.T) {
	in := validCreateInput(PlanItemInput{SKUID: uuid.New(), Quantity: 0})

	svc := NewService(&mockStore{})
	_, err := svc.CreatePlan(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for quantity=0, got %v", err)
	}
}

func TestCreatePlan_NegativeQuantityItem_ReturnsErrInvalidInput(t *testing.T) {
	in := validCreateInput(PlanItemInput{SKUID: uuid.New(), Quantity: -3})

	svc := NewService(&mockStore{})
	_, err := svc.CreatePlan(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative quantity, got %v", err)
	}
}

func TestCreatePlan_SecondItemInvalidQuantity_ReturnsErrInvalidInput(t *testing.T) {
	// Loop must inspect every item, not just the first.
	in := validCreateInput(
		PlanItemInput{SKUID: uuid.New(), Quantity: 5},
		PlanItemInput{SKUID: uuid.New(), Quantity: 0},
	)

	svc := NewService(&mockStore{})
	_, err := svc.CreatePlan(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for second item quantity=0, got %v", err)
	}
}

func TestCreatePlan_QuantityOneIsValid(t *testing.T) {
	// Boundary: quantity=1 must pass (> 0).
	in := validCreateInput(PlanItemInput{SKUID: uuid.New(), Quantity: 1})

	svc := NewService(&mockStore{})
	_, err := svc.CreatePlan(context.Background(), in)

	if err != nil {
		t.Errorf("quantity=1 must be valid, got: %v", err)
	}
}

func TestCreatePlan_StoreInsertPlanError_Propagates(t *testing.T) {
	dbErr := errors.New("insert plan failed")
	st := &mockStore{insertPlanErr: dbErr}

	svc := NewService(st)
	_, err := svc.CreatePlan(context.Background(), validCreateInput())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected insertPlan error to propagate, got %v", err)
	}
}

func TestCreatePlan_StoreInsertItemsError_Propagates(t *testing.T) {
	dbErr := errors.New("insert plan items failed")
	st := &mockStore{insertPlanItemsErr: dbErr}

	svc := NewService(st)
	_, err := svc.CreatePlan(context.Background(), validCreateInput())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected insertPlanItems error to propagate, got %v", err)
	}
}

// ── TestGetPlan ───────────────────────────────────────────────────────────────

func TestGetPlan_HappyPath_PopulatesItems(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()

	storedPlan := draftPlan(planID)
	storedItems := []PlanItem{
		{ID: uuid.New(), PlanID: planID, SKUID: skuID, Quantity: 4},
	}

	st := &mockStore{
		selectPlanByIDResult:          storedPlan,
		selectPlanItemsByPlanIDResult: storedItems,
	}

	svc := NewService(st)
	plan, err := svc.GetPlan(context.Background(), planID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.ID != planID {
		t.Errorf("Plan.ID = %v, want %v", plan.ID, planID)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(plan.Items))
	}
	if plan.Items[0].SKUID != skuID {
		t.Errorf("Items[0].SKUID = %v, want %v", plan.Items[0].SKUID, skuID)
	}
}

func TestGetPlan_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectPlanByIDErr: domain.NewBizError(domain.ErrNotFound, "plan not found"),
	}

	svc := NewService(st)
	_, err := svc.GetPlan(context.Background(), uuid.New())

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

func TestGetPlan_SelectItemsError_Propagates(t *testing.T) {
	dbErr := errors.New("items query failed")
	st := &mockStore{
		selectPlanByIDResult:       draftPlan(uuid.New()),
		selectPlanItemsByPlanIDErr: dbErr,
	}

	svc := NewService(st)
	_, err := svc.GetPlan(context.Background(), uuid.New())

	if !errors.Is(err, dbErr) {
		t.Errorf("expected selectPlanItemsByPlanID error to propagate, got %v", err)
	}
}

// ── TestListPlans ─────────────────────────────────────────────────────────────

func TestListPlans_PopulatesItems(t *testing.T) {
	planID1 := uuid.New()
	planID2 := uuid.New()

	st := &mockStore{
		selectPlansResult: []Plan{
			draftPlan(planID1),
			draftPlan(planID2),
		},
		// selectPlanItemsByPlanID returns same slice for both calls (sufficient for unit test)
		selectPlanItemsByPlanIDResult: []PlanItem{
			{ID: uuid.New(), PlanID: planID1, SKUID: uuid.New(), Quantity: 2},
		},
	}

	svc := NewService(st)
	plans, err := svc.ListPlans(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, "", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans.Items) != 2 {
		t.Fatalf("len(plans.Items) = %d, want 2", len(plans.Items))
	}
	// Each plan must have its Items populated.
	for i, p := range plans.Items {
		if len(p.Items) == 0 {
			t.Errorf("plans.Items[%d].Items must be populated by ListPlans", i)
		}
	}
}

func TestListPlans_Empty_ReturnsNil(t *testing.T) {
	st := &mockStore{selectPlansResult: nil}

	svc := NewService(st)
	plans, err := svc.ListPlans(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, "", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plans.Items) != 0 {
		t.Errorf("expected empty result, got %d plans", len(plans.Items))
	}
}

func TestListPlans_SelectPlansError_Propagates(t *testing.T) {
	dbErr := errors.New("select plans failed")
	st := &mockStore{selectPlansErr: dbErr}

	svc := NewService(st)
	_, err := svc.ListPlans(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, "", nil, nil)

	if !errors.Is(err, dbErr) {
		t.Errorf("expected selectPlans error to propagate, got %v", err)
	}
}

func TestListPlans_SelectItemsError_Propagates(t *testing.T) {
	// selectPlans succeeds but fetching items for the first plan fails.
	dbErr := errors.New("items query failed")
	st := &mockStore{
		selectPlansResult:          []Plan{draftPlan(uuid.New())},
		selectPlanItemsByPlanIDErr: dbErr,
	}

	svc := NewService(st)
	_, err := svc.ListPlans(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, "", nil, nil)

	if !errors.Is(err, dbErr) {
		t.Errorf("expected selectPlanItemsByPlanID error to propagate in ListPlans, got %v", err)
	}
}

// ── TestApprovePlan ───────────────────────────────────────────────────────────

func TestApprovePlan_FromDraft_Succeeds(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{selectPlanByIDResult: draftPlan(planID)}

	svc := NewService(st)
	err := svc.ApprovePlan(context.Background(), planID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.updatePlanStatusCalled {
		t.Error("updatePlanStatus must be called")
	}
	if st.updatePlanStatusValue != string(domain.PlanApproved) {
		t.Errorf("status written = %q, want %q", st.updatePlanStatusValue, domain.PlanApproved)
	}
}

func TestApprovePlan_FromApproved_ReturnsErrInvalidTransition(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{
		selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanApproved},
	}

	svc := NewService(st)
	err := svc.ApprovePlan(context.Background(), planID)

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition from APPROVED, got %v", err)
	}
	if st.updatePlanStatusCalled {
		t.Error("updatePlanStatus must NOT be called on invalid transition")
	}
}

func TestApprovePlan_FromCanceled_ReturnsErrInvalidTransition(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{
		selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanCanceled},
	}

	svc := NewService(st)
	err := svc.ApprovePlan(context.Background(), planID)

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition from CANCELED, got %v", err)
	}
	if st.updatePlanStatusCalled {
		t.Error("updatePlanStatus must NOT be called on invalid transition")
	}
}

func TestApprovePlan_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectPlanByIDErr: domain.NewBizError(domain.ErrNotFound, "plan not found"),
	}

	svc := NewService(st)
	err := svc.ApprovePlan(context.Background(), uuid.New())

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

func TestApprovePlan_UpdateStatusError_Propagates(t *testing.T) {
	dbErr := errors.New("update status failed")
	planID := uuid.New()
	st := &mockStore{
		selectPlanByIDResult: draftPlan(planID),
		updatePlanStatusErr:  dbErr,
	}

	svc := NewService(st)
	err := svc.ApprovePlan(context.Background(), planID)

	if !errors.Is(err, dbErr) {
		t.Errorf("expected updatePlanStatus error to propagate, got %v", err)
	}
}

// ── TestCancelPlan ────────────────────────────────────────────────────────────

func TestCancelPlan_FromDraft_Succeeds(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{selectPlanByIDResult: draftPlan(planID)}

	svc := NewService(st)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{PlanID: planID})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.updatePlanStatusCalled {
		t.Error("updatePlanStatus must be called for DRAFT cancel")
	}
	if st.updatePlanStatusValue != string(domain.PlanCanceled) {
		t.Errorf("status written = %q, want %q", st.updatePlanStatusValue, domain.PlanCanceled)
	}
	if st.cancelPlanWithMetadataCalled {
		t.Error("cancelPlanWithMetadata must NOT be called for DRAFT cancel")
	}
}

func TestCancelPlan_FromApproved_NoWorkOrders_Succeeds(t *testing.T) {
	planID := uuid.New()
	actor := uuid.New()
	st := &mockStore{selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanApproved}}
	wo := &mockWOCanceller{listResult: nil}

	svc := NewServiceWithDeps(st, wo)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{
		PlanID:  planID,
		Reason:  "PO canceled by customer",
		ActorID: actor,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wo.listCalled {
		t.Error("ListStatusesByPlan must be called as precondition")
	}
	if !wo.cancelCalled {
		t.Error("CancelPlannedByPlan must be called even when there are no WOs (idempotent)")
	}
	if !st.cancelPlanWithMetadataCalled {
		t.Fatal("cancelPlanWithMetadata must be called for APPROVED cancel")
	}
	if st.cancelPlanWithMetadataReason != "PO canceled by customer" {
		t.Errorf("reason = %q, want %q", st.cancelPlanWithMetadataReason, "PO canceled by customer")
	}
	if st.cancelPlanWithMetadataActorID != actor {
		t.Errorf("actor = %v, want %v", st.cancelPlanWithMetadataActorID, actor)
	}
	if st.cancelPlanWithMetadataAt.IsZero() {
		t.Error("cancelPlanWithMetadata timestamp must be set")
	}
}

func TestCancelPlan_FromApproved_AllWorkOrdersPlanned_CascadesAndCancels(t *testing.T) {
	planID := uuid.New()
	actor := uuid.New()
	st := &mockStore{selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanApproved}}
	wo := &mockWOCanceller{
		listResult:   []domain.WorkOrderStatus{domain.WOPlanned, domain.WOPlanned, domain.WOCanceled},
		cancelResult: 2,
	}

	svc := NewServiceWithDeps(st, wo)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{
		PlanID:  planID,
		Reason:  "production stopped",
		ActorID: actor,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !wo.cancelCalled || wo.cancelCalledWithID != planID {
		t.Error("CancelPlannedByPlan must be called with the plan id")
	}
	if !st.cancelPlanWithMetadataCalled {
		t.Error("cancelPlanWithMetadata must be called after WO cascade succeeds")
	}
}

func TestCancelPlan_FromApproved_HasStartedWorkOrder_RefusedNoCascade(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanApproved}}
	wo := &mockWOCanceller{
		listResult: []domain.WorkOrderStatus{domain.WOPlanned, domain.WOInCutting},
	}

	svc := NewServiceWithDeps(st, wo)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{
		PlanID:  planID,
		Reason:  "tried to cancel mid-cut",
		ActorID: uuid.New(),
	})

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition when a WO has started, got %v", err)
	}
	if wo.cancelCalled {
		t.Error("CancelPlannedByPlan must NOT be called when precondition fails")
	}
	if st.cancelPlanWithMetadataCalled {
		t.Error("cancelPlanWithMetadata must NOT be called when precondition fails")
	}
}

func TestCancelPlan_FromApproved_EmptyReason_RejectedAsInvalidInput(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanApproved}}
	wo := &mockWOCanceller{}

	svc := NewServiceWithDeps(st, wo)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{PlanID: planID, ActorID: uuid.New()})

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for missing reason on APPROVED cancel, got %v", err)
	}
	if wo.listCalled || wo.cancelCalled {
		t.Error("WOCanceller must NOT be called when reason is missing")
	}
	if st.cancelPlanWithMetadataCalled {
		t.Error("cancelPlanWithMetadata must NOT be called when reason is missing")
	}
}

func TestCancelPlan_FromApproved_NoCanceller_PreconditionFailed(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanApproved}}

	svc := NewService(st) // no WO canceller wired
	err := svc.CancelPlan(context.Background(), CancelPlanInput{
		PlanID:  planID,
		Reason:  "no canceller",
		ActorID: uuid.New(),
	})

	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed when canceller is unwired, got %v", err)
	}
}

func TestCancelPlan_FromApproved_ListStatusesError_Propagates(t *testing.T) {
	planID := uuid.New()
	dbErr := errors.New("statuses query failed")
	st := &mockStore{selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanApproved}}
	wo := &mockWOCanceller{listErr: dbErr}

	svc := NewServiceWithDeps(st, wo)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{
		PlanID:  planID,
		Reason:  "x",
		ActorID: uuid.New(),
	})

	if !errors.Is(err, dbErr) {
		t.Errorf("expected ListStatusesByPlan error to propagate, got %v", err)
	}
	if st.cancelPlanWithMetadataCalled {
		t.Error("cancelPlanWithMetadata must NOT be called when precondition errored")
	}
}

func TestCancelPlan_FromApproved_CancelWOsError_Propagates(t *testing.T) {
	planID := uuid.New()
	dbErr := errors.New("cascade failed")
	st := &mockStore{selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanApproved}}
	wo := &mockWOCanceller{
		listResult: []domain.WorkOrderStatus{domain.WOPlanned},
		cancelErr:  dbErr,
	}

	svc := NewServiceWithDeps(st, wo)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{
		PlanID:  planID,
		Reason:  "x",
		ActorID: uuid.New(),
	})

	if !errors.Is(err, dbErr) {
		t.Errorf("expected CancelPlannedByPlan error to propagate, got %v", err)
	}
	if st.cancelPlanWithMetadataCalled {
		t.Error("cancelPlanWithMetadata must NOT be called when cascade failed")
	}
}

func TestCancelPlan_FromCanceled_ReturnsErrInvalidTransition(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{
		selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanCanceled},
	}

	svc := NewService(st)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{PlanID: planID})

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition from CANCELED, got %v", err)
	}
	if st.updatePlanStatusCalled || st.cancelPlanWithMetadataCalled {
		t.Error("no status updates may happen on CANCELED → CANCELED")
	}
}

func TestCancelPlan_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectPlanByIDErr: domain.NewBizError(domain.ErrNotFound, "plan not found"),
	}

	svc := NewService(st)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{PlanID: uuid.New()})

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

func TestCancelPlan_DraftUpdateStatusError_Propagates(t *testing.T) {
	dbErr := errors.New("update status failed")
	planID := uuid.New()
	st := &mockStore{
		selectPlanByIDResult: draftPlan(planID),
		updatePlanStatusErr:  dbErr,
	}

	svc := NewService(st)
	err := svc.CancelPlan(context.Background(), CancelPlanInput{PlanID: planID})

	if !errors.Is(err, dbErr) {
		t.Errorf("expected updatePlanStatus error to propagate, got %v", err)
	}
}

// ── TestStatusLifecycle_AllTransitionsTable ───────────────────────────────────
// Table-driven exhaustive check of every (from, action, expected) combination
// for the PlanStatus state machine.

func TestStatusLifecycle_AllTransitionsTable(t *testing.T) {
	type action string
	const (
		approve action = "approve"
		cancel  action = "cancel"
	)

	tests := []struct {
		from       domain.PlanStatus
		act        action
		wantErr    error  // nil means success
		wantStatus string // only checked when wantErr == nil
	}{
		// Valid transitions (happy paths)
		{domain.PlanDraft, approve, nil, string(domain.PlanApproved)},
		{domain.PlanDraft, cancel, nil, string(domain.PlanCanceled)},

		// Invalid transitions — ApprovePlan
		{domain.PlanApproved, approve, domain.ErrInvalidTransition, ""},
		{domain.PlanCanceled, approve, domain.ErrInvalidTransition, ""},

		// Invalid transitions — CancelPlan. APPROVED is now a valid source
		// state (#249) so it is no longer in this table; covered explicitly by
		// the dedicated APPROVED-cancel tests above.
		{domain.PlanCanceled, cancel, domain.ErrInvalidTransition, ""},
	}

	for _, tc := range tests {
		name := string(tc.from) + "_" + string(tc.act)
		t.Run(name, func(t *testing.T) {
			planID := uuid.New()
			st := &mockStore{
				selectPlanByIDResult: Plan{ID: planID, Status: tc.from},
			}
			svc := NewService(st)

			var err error
			switch tc.act {
			case approve:
				err = svc.ApprovePlan(context.Background(), planID)
			case cancel:
				err = svc.CancelPlan(context.Background(), CancelPlanInput{PlanID: planID})
			}

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("got error %v, want %v", err, tc.wantErr)
				}
				if st.updatePlanStatusCalled {
					t.Error("updatePlanStatus must NOT be called on invalid transition")
				}
				return
			}

			// Happy path assertions.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !st.updatePlanStatusCalled {
				t.Error("updatePlanStatus must be called on valid transition")
			}
			if st.updatePlanStatusValue != tc.wantStatus {
				t.Errorf("status written = %q, want %q", st.updatePlanStatusValue, tc.wantStatus)
			}
		})
	}
}

func TestListPlans_SearchAndPOCodeContract(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{
		selectPlansResult: []Plan{{
			ID:        planID,
			Code:      "KH-2026-001",
			POID:      newPOID(),
			POCode:    "PO-001",
			Status:    domain.PlanApproved,
			CreatedAt: time.Now().UTC(),
		}},
		selectPlanItemsByPlanIDResult: []PlanItem{{ID: uuid.New(), PlanID: planID, SKUID: uuid.New(), Quantity: 2}},
	}

	svc := NewService(st)
	plans, err := svc.ListPlans(context.Background(), httpkit.PageParams{Page: 1, Limit: 20, Search: "PO-001"}, "APPROVED", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.selectPlansSearch != "PO-001" {
		t.Errorf("Search = %q, want PO-001", st.selectPlansSearch)
	}
	if st.selectPlansStatus != "APPROVED" {
		t.Errorf("Status = %q, want APPROVED", st.selectPlansStatus)
	}
	if len(plans.Items) != 1 {
		t.Fatalf("len(plans.Items) = %d, want 1", len(plans.Items))
	}
	if plans.Items[0].POCode != "PO-001" {
		t.Errorf("POCode = %q, want PO-001", plans.Items[0].POCode)
	}
	if len(plans.Items[0].Items) == 0 {
		t.Error("Items must still be populated")
	}
}

func TestListPlans_DateRange_ForwardedToStore(t *testing.T) {
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)

	st := &mockStore{selectPlansResult: nil}
	svc := NewService(st)

	if _, err := svc.ListPlans(
		context.Background(),
		httpkit.PageParams{Page: 1, Limit: 10},
		"",
		&from,
		&to,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.selectPlansFrom == nil || !st.selectPlansFrom.Equal(from) {
		t.Errorf("from forwarded = %v, want %v", st.selectPlansFrom, from)
	}
	if st.selectPlansTo == nil || !st.selectPlansTo.Equal(to) {
		t.Errorf("to forwarded = %v, want %v", st.selectPlansTo, to)
	}
}

func TestListPlans_FromAfterTo_Returns400(t *testing.T) {
	from := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	st := &mockStore{}
	svc := NewService(st)

	_, err := svc.ListPlans(
		context.Background(),
		httpkit.PageParams{Page: 1, Limit: 10},
		"",
		&from,
		&to,
	)
	if err == nil {
		t.Fatal("expected error for from > to, got nil")
	}
	var bizErr *domain.BizError
	if !errors.As(err, &bizErr) {
		t.Fatalf("expected BizError, got %T (%v)", err, err)
	}
	if bizErr.Sentinel != domain.ErrInvalidInput {
		t.Errorf("sentinel = %v, want ErrInvalidInput", bizErr.Sentinel)
	}
}

// ── TestLookupPlans ───────────────────────────────────────────────────────────

func TestLookupPlans_HappyPath_ReturnsItems(t *testing.T) {
	deadline := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	st := &mockStore{
		selectLookupResult: []PlanLookupItem{
			{ID: uuid.New(), Code: "KH-2026-001", POCode: "PO-001", Status: domain.PlanApproved, Deadline: &deadline},
		},
	}

	svc := NewService(st)
	result, err := svc.LookupPlans(context.Background(), LookupPlansInput{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(result.Items))
	}
	if result.Items[0].Code != "KH-2026-001" {
		t.Errorf("Code = %q, want KH-2026-001", result.Items[0].Code)
	}
}

func TestLookupPlans_SearchAndStatusPassedThrough(t *testing.T) {
	st := &mockStore{selectLookupResult: nil}

	svc := NewService(st)
	_, err := svc.LookupPlans(context.Background(), LookupPlansInput{
		Search: "KH-2026",
		Status: "APPROVED",
		Page:   1, Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.selectLookupSearch != "KH-2026" {
		t.Errorf("search passed = %q, want KH-2026", st.selectLookupSearch)
	}
	if st.selectLookupStatus != "APPROVED" {
		t.Errorf("status passed = %q, want APPROVED", st.selectLookupStatus)
	}
}

func TestLookupPlans_DeadlineFilterPassedThrough(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	st := &mockStore{selectLookupResult: nil}

	svc := NewService(st)
	_, err := svc.LookupPlans(context.Background(), LookupPlansInput{
		DeadlineFrom: &from,
		DeadlineTo:   &to,
		Page:         1, Limit: 20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.selectLookupDeadlineFrom == nil || !st.selectLookupDeadlineFrom.Equal(from) {
		t.Errorf("deadline_from not passed correctly")
	}
	if st.selectLookupDeadlineTo == nil || !st.selectLookupDeadlineTo.Equal(to) {
		t.Errorf("deadline_to not passed correctly")
	}
}

func TestLookupPlans_DeadlineFromAfterTo_ReturnsErrInvalidInput(t *testing.T) {
	from := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	svc := NewService(&mockStore{})
	_, err := svc.LookupPlans(context.Background(), LookupPlansInput{
		DeadlineFrom: &from,
		DeadlineTo:   &to,
		Page:         1, Limit: 20,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for inverted deadline range, got %v", err)
	}
}

func TestLookupPlans_LimitCappedAt50(t *testing.T) {
	st := &mockStore{selectLookupResult: nil}

	svc := NewService(st)
	_, err := svc.LookupPlans(context.Background(), LookupPlansInput{Page: 1, Limit: 9999})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.selectLookupLimit != maxLookupLimit {
		t.Errorf("limit passed to store = %d, want %d", st.selectLookupLimit, maxLookupLimit)
	}
}

func TestLookupPlans_ZeroLimitDefaultsTo50(t *testing.T) {
	st := &mockStore{selectLookupResult: nil}

	svc := NewService(st)
	_, err := svc.LookupPlans(context.Background(), LookupPlansInput{Page: 1, Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.selectLookupLimit != maxLookupLimit {
		t.Errorf("limit passed to store = %d, want %d", st.selectLookupLimit, maxLookupLimit)
	}
}

func TestLookupPlans_PaginationOffset(t *testing.T) {
	st := &mockStore{selectLookupResult: nil, selectLookupTotal: 100}

	svc := NewService(st)
	result, err := svc.LookupPlans(context.Background(), LookupPlansInput{Page: 3, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.selectLookupOffset != 20 {
		t.Errorf("offset = %d, want 20 (page 3, limit 10)", st.selectLookupOffset)
	}
	if result.CurrentPage != 3 {
		t.Errorf("CurrentPage = %d, want 3", result.CurrentPage)
	}
}

func TestLookupPlans_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("lookup query failed")
	st := &mockStore{selectLookupErr: dbErr}

	svc := NewService(st)
	_, err := svc.LookupPlans(context.Background(), LookupPlansInput{Page: 1, Limit: 20})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

func TestLookupPlans_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	st := &mockStore{selectLookupResult: nil}

	svc := NewService(st)
	result, err := svc.LookupPlans(context.Background(), LookupPlansInput{Page: 1, Limit: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected empty result, got %d items", len(result.Items))
	}
}
