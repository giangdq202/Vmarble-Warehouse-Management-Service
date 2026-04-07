package planning

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
	// insertPlan
	insertPlanErr error

	// selectPlans
	selectPlansResult []Plan
	selectPlansErr    error

	// selectPlanByID
	selectPlanByIDResult Plan
	selectPlanByIDErr    error

	// updatePlanStatus — captures what was last passed in for assertion
	updatePlanStatusCalled bool
	updatePlanStatusID     uuid.UUID
	updatePlanStatusValue  string
	updatePlanStatusErr    error

	// insertPlanItems
	insertPlanItemsErr error

	// selectPlanItemsByPlanID
	selectPlanItemsByPlanIDResult []PlanItem
	selectPlanItemsByPlanIDErr    error
}

func (m *mockStore) insertPlan(_ context.Context, _ Plan) error {
	return m.insertPlanErr
}

func (m *mockStore) selectPlans(_ context.Context) ([]Plan, error) {
	return m.selectPlansResult, m.selectPlansErr
}

func (m *mockStore) selectPlansPaged(_ context.Context, _ httpkit.PageParams, _ string) ([]Plan, int, error) {
	return m.selectPlansResult, len(m.selectPlansResult), m.selectPlansErr
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

// ── helpers ───────────────────────────────────────────────────────────────────

func validCreateInput(items ...PlanItemInput) CreatePlanInput {
	if len(items) == 0 {
		items = []PlanItemInput{
			{SKUID: uuid.New(), Quantity: 5},
		}
	}
	return CreatePlanInput{
		POID:  uuid.New(),
		Items: items,
	}
}

func draftPlan(id uuid.UUID) Plan {
	return Plan{ID: id, POID: uuid.New(), Status: domain.PlanDraft, CreatedAt: time.Now().UTC()}
}

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
	in := CreatePlanInput{POID: uuid.New(), Items: []PlanItemInput{}}

	svc := NewService(&mockStore{})
	_, err := svc.CreatePlan(context.Background(), in)

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty items, got %v", err)
	}
}

func TestCreatePlan_NilItems_ReturnsErrInvalidInput(t *testing.T) {
	in := CreatePlanInput{POID: uuid.New(), Items: nil}

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
		selectPlanByIDResult:         draftPlan(uuid.New()),
		selectPlanItemsByPlanIDErr:   dbErr,
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
	plans, err := svc.ListPlans(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, "")
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
	plans, err := svc.ListPlans(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, "")
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
	_, err := svc.ListPlans(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, "")

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
	_, err := svc.ListPlans(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, "")

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
	err := svc.CancelPlan(context.Background(), planID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.updatePlanStatusCalled {
		t.Error("updatePlanStatus must be called")
	}
	if st.updatePlanStatusValue != string(domain.PlanCanceled) {
		t.Errorf("status written = %q, want %q", st.updatePlanStatusValue, domain.PlanCanceled)
	}
}

func TestCancelPlan_FromApproved_ReturnsErrInvalidTransition(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{
		selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanApproved},
	}

	svc := NewService(st)
	err := svc.CancelPlan(context.Background(), planID)

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition from APPROVED, got %v", err)
	}
	if st.updatePlanStatusCalled {
		t.Error("updatePlanStatus must NOT be called on invalid transition")
	}
}

func TestCancelPlan_FromCanceled_ReturnsErrInvalidTransition(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{
		selectPlanByIDResult: Plan{ID: planID, Status: domain.PlanCanceled},
	}

	svc := NewService(st)
	err := svc.CancelPlan(context.Background(), planID)

	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition from CANCELED, got %v", err)
	}
	if st.updatePlanStatusCalled {
		t.Error("updatePlanStatus must NOT be called on invalid transition")
	}
}

func TestCancelPlan_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectPlanByIDErr: domain.NewBizError(domain.ErrNotFound, "plan not found"),
	}

	svc := NewService(st)
	err := svc.CancelPlan(context.Background(), uuid.New())

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

func TestCancelPlan_UpdateStatusError_Propagates(t *testing.T) {
	dbErr := errors.New("update status failed")
	planID := uuid.New()
	st := &mockStore{
		selectPlanByIDResult: draftPlan(planID),
		updatePlanStatusErr:  dbErr,
	}

	svc := NewService(st)
	err := svc.CancelPlan(context.Background(), planID)

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
		from        domain.PlanStatus
		act         action
		wantErr     error // nil means success
		wantStatus  string // only checked when wantErr == nil
	}{
		// Valid transitions (happy paths)
		{domain.PlanDraft, approve, nil, string(domain.PlanApproved)},
		{domain.PlanDraft, cancel, nil, string(domain.PlanCanceled)},

		// Invalid transitions — ApprovePlan
		{domain.PlanApproved, approve, domain.ErrInvalidTransition, ""},
		{domain.PlanCanceled, approve, domain.ErrInvalidTransition, ""},

		// Invalid transitions — CancelPlan
		{domain.PlanApproved, cancel, domain.ErrInvalidTransition, ""},
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
				err = svc.CancelPlan(context.Background(), planID)
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
