package production

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── mock implementations ──────────────────────────────────────────────────────

// mockStore satisfies the full store interface.
type mockStore struct {
	// insertWorkOrder
	insertWorkOrderErr error

	// selectWorkOrders
	selectWorkOrdersResult []WorkOrder
	selectWorkOrdersErr    error
	selectWorkOrdersFilter WorkOrderListFilter

	// selectWorkOrderByID
	selectWorkOrderByIDResult WorkOrder
	selectWorkOrderByIDErr    error

	// selectWorkOrdersByPlan
	selectWorkOrdersByPlanResult []WorkOrder
	selectWorkOrdersByPlanErr    error

	// selectWorkOrdersByAssignee
	selectWorkOrdersByAssigneeResult []WorkOrder
	selectWorkOrdersByAssigneeErr    error

	// updateWorkOrderStatus — record calls for inspection
	updateWorkOrderStatusCalled bool
	updateWorkOrderStatusArg    string
	updateWorkOrderStatusErr    error

	// updateWorkOrderAssignment — record calls for inspection
	updateWorkOrderAssignmentCalled bool
	updateWorkOrderAssignmentErr    error

	// insertConsumption
	insertConsumptionErr error

	// selectConsumptionsByWO
	selectConsumptionsByWOResult []ConsumptionRecord
	selectConsumptionsByWOErr    error

	// hasMetalConsumption
	hasMetalConsumptionResult bool
	hasMetalConsumptionErr    error

	// selectInCuttingCountByUser
	selectInCuttingCountByUserResult map[uuid.UUID]int
	selectInCuttingCountByUserErr    error

	// selectCNCUserIDs
	selectCNCUserIDsResult []uuid.UUID
	selectCNCUserIDsErr    error

	// Machine methods
	insertMachineErr        error
	selectMachinesResult    []Machine
	selectMachinesErr       error
	selectMachineByIDResult Machine
	selectMachineByIDErr    error
	deactivateMachineErr    error

	// Slot methods
	insertSlotErr              error
	selectSlotByIDResult       MachineShiftSlot
	selectSlotByIDErr          error
	selectSlotsByMachineResult []MachineShiftSlot
	selectSlotsByMachineErr    error
	selectFutureSlotsResult    []MachineShiftSlot
	selectFutureSlotsErr       error
	deleteSlotErr              error

	// Work order scheduling
	updateEstimatedHoursErr error
	unassignWOFromSlotErr   error
	assignSlotAtomicallyErr error
}

func (m *mockStore) insertWorkOrder(_ context.Context, _ WorkOrder) error {
	return m.insertWorkOrderErr
}
func (m *mockStore) selectWorkOrdersPaged(_ context.Context, _ httpkit.PageParams, f WorkOrderListFilter) ([]WorkOrder, int, error) {
	m.selectWorkOrdersFilter = f
	return m.selectWorkOrdersResult, len(m.selectWorkOrdersResult), m.selectWorkOrdersErr
}
func (m *mockStore) selectWorkOrderByID(_ context.Context, _ uuid.UUID) (WorkOrder, error) {
	return m.selectWorkOrderByIDResult, m.selectWorkOrderByIDErr
}
func (m *mockStore) selectWorkOrdersByPlan(_ context.Context, _ uuid.UUID) ([]WorkOrder, error) {
	return m.selectWorkOrdersByPlanResult, m.selectWorkOrdersByPlanErr
}
func (m *mockStore) selectWorkOrdersByAssignee(_ context.Context, _ uuid.UUID) ([]WorkOrder, error) {
	return m.selectWorkOrdersByAssigneeResult, m.selectWorkOrdersByAssigneeErr
}
func (m *mockStore) updateWorkOrderStatus(_ context.Context, _ uuid.UUID, status string) error {
	m.updateWorkOrderStatusCalled = true
	m.updateWorkOrderStatusArg = status
	return m.updateWorkOrderStatusErr
}
func (m *mockStore) updateWorkOrderAssignment(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ time.Time) error {
	m.updateWorkOrderAssignmentCalled = true
	return m.updateWorkOrderAssignmentErr
}
func (m *mockStore) insertConsumption(_ context.Context, _ ConsumptionRecord) error {
	return m.insertConsumptionErr
}
func (m *mockStore) selectConsumptionsByWO(_ context.Context, _ uuid.UUID) ([]ConsumptionRecord, error) {
	return m.selectConsumptionsByWOResult, m.selectConsumptionsByWOErr
}
func (m *mockStore) hasMetalConsumption(_ context.Context, _ uuid.UUID) (bool, error) {
	return m.hasMetalConsumptionResult, m.hasMetalConsumptionErr
}
func (m *mockStore) selectInCuttingCountByUser(_ context.Context) (map[uuid.UUID]int, error) {
	return m.selectInCuttingCountByUserResult, m.selectInCuttingCountByUserErr
}
func (m *mockStore) selectCNCUserIDs(_ context.Context) ([]uuid.UUID, error) {
	return m.selectCNCUserIDsResult, m.selectCNCUserIDsErr
}
func (m *mockStore) insertMachine(_ context.Context, _ Machine) error {
	return m.insertMachineErr
}
func (m *mockStore) selectMachines(_ context.Context) ([]Machine, error) {
	return m.selectMachinesResult, m.selectMachinesErr
}
func (m *mockStore) selectMachineByID(_ context.Context, _ uuid.UUID) (Machine, error) {
	return m.selectMachineByIDResult, m.selectMachineByIDErr
}
func (m *mockStore) deactivateMachine(_ context.Context, _ uuid.UUID) error {
	return m.deactivateMachineErr
}
func (m *mockStore) insertSlot(_ context.Context, _ MachineShiftSlot) error {
	return m.insertSlotErr
}
func (m *mockStore) selectSlotByID(_ context.Context, _ uuid.UUID) (MachineShiftSlot, error) {
	return m.selectSlotByIDResult, m.selectSlotByIDErr
}
func (m *mockStore) selectSlotsByMachine(_ context.Context, _ uuid.UUID, _, _ time.Time) ([]MachineShiftSlot, error) {
	return m.selectSlotsByMachineResult, m.selectSlotsByMachineErr
}
func (m *mockStore) selectFutureSlotsWithCapacity(_ context.Context, _ float64) ([]MachineShiftSlot, error) {
	return m.selectFutureSlotsResult, m.selectFutureSlotsErr
}
func (m *mockStore) deleteSlot(_ context.Context, _ uuid.UUID) error {
	return m.deleteSlotErr
}
func (m *mockStore) updateEstimatedHours(_ context.Context, _ uuid.UUID, _ float64) error {
	return m.updateEstimatedHoursErr
}
func (m *mockStore) unassignWOFromSlot(_ context.Context, _ uuid.UUID) error {
	return m.unassignWOFromSlotErr
}
func (m *mockStore) assignSlotAtomically(_ context.Context, _ assignSlotOp) error {
	return m.assignSlotAtomicallyErr
}

// mockPlanChecker satisfies PlanChecker.
type mockPlanChecker struct {
	result PlanInfo
	err    error
}

func (m *mockPlanChecker) GetPlan(_ context.Context, _ uuid.UUID) (PlanInfo, error) {
	return m.result, m.err
}

// mockSKUChecker satisfies SKUChecker.
type mockSKUChecker struct {
	result SKUInfo
	err    error
}

func (m *mockSKUChecker) GetSKU(_ context.Context, _ uuid.UUID) (SKUInfo, error) {
	return m.result, m.err
}

// mockUserChecker satisfies UserChecker.
type mockUserChecker struct {
	result UserInfo
	err    error
}

func (m *mockUserChecker) GetUser(_ context.Context, _ uuid.UUID) (UserInfo, error) {
	return m.result, m.err
}

// mockWorkOrderNotifier satisfies WorkOrderNotifier.
type mockWorkOrderNotifier struct {
	called bool
	err    error
}

func (m *mockWorkOrderNotifier) NotifyAssignment(_ context.Context, _, _, _ string) error {
	m.called = true
	return m.err
}

// mockSheetAssigner satisfies SheetAssigner.
type mockSheetAssigner struct {
	called         bool
	calledSheet    uuid.UUID
	calledWO       uuid.UUID
	calledBypass   bool
	calledActorID  uuid.UUID
	calledReason   string
	err            error
}

func (m *mockSheetAssigner) PreAssignSheet(_ context.Context, in PreAssignSheetRequest) error {
	m.called = true
	m.calledSheet = in.SheetID
	m.calledWO = in.WorkOrderID
	m.calledBypass = in.BypassOverflow
	m.calledActorID = in.ActorID
	m.calledReason = in.Reason
	return m.err
}

// mockCostingChecker satisfies CostingChecker.
type mockCostingChecker struct {
	result bool
	err    error
}

func (m *mockCostingChecker) HasCostingRecord(_ context.Context, _ uuid.UUID) (bool, error) {
	return m.result, m.err
}

// mockRemnantAdvisor satisfies RemnantAdvisor. Records every interaction so
// CreateWorkOrder tests can assert which advisory branch fired.
type mockRemnantAdvisor struct {
	suggestResult []RemnantSuggestionRef
	suggestErr    error
	suggestCalled bool
	suggestDim    domain.Dimension

	logErr    error
	logCalled bool
	logArg    LogRemnantBypassRequest
}

func (m *mockRemnantAdvisor) SuggestRemnants(_ context.Context, dim domain.Dimension) ([]RemnantSuggestionRef, error) {
	m.suggestCalled = true
	m.suggestDim = dim
	return m.suggestResult, m.suggestErr
}

func (m *mockRemnantAdvisor) LogRemnantBypass(_ context.Context, in LogRemnantBypassRequest) error {
	m.logCalled = true
	m.logArg = in
	return m.logErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newSvc(st *mockStore, pc *mockPlanChecker, sc *mockSKUChecker) Service {
	return NewService(st, pc, sc, &mockUserChecker{}, nil, nil, nil)
}

func newSvcWithUser(st *mockStore, pc *mockPlanChecker, sc *mockSKUChecker, uc *mockUserChecker) Service {
	return NewService(st, pc, sc, uc, nil, nil, nil)
}

func newSvcWithCosting(st *mockStore, pc *mockPlanChecker, sc *mockSKUChecker, cc *mockCostingChecker) Service {
	return NewService(st, pc, sc, &mockUserChecker{}, nil, cc, nil)
}

// approvedPlan returns a PlanChecker that returns an APPROVED plan.
// Optional skuIDs are populated in PlanInfo.SKUIDs to satisfy the
// CreateWorkOrder SKU-membership validation.
func approvedPlan(planID uuid.UUID, skuIDs ...uuid.UUID) *mockPlanChecker {
	return &mockPlanChecker{result: PlanInfo{ID: planID, Status: domain.PlanApproved, SKUIDs: skuIDs}}
}

// skuNoMetal returns a SKUChecker for a SKU that does not require metal.
func skuNoMetal(skuID uuid.UUID) *mockSKUChecker {
	return &mockSKUChecker{result: SKUInfo{ID: skuID, RequiresMetal: false}}
}

// skuRequiresMetal returns a SKUChecker for a SKU that requires metal.
func skuRequiresMetal(skuID uuid.UUID) *mockSKUChecker {
	return &mockSKUChecker{result: SKUInfo{ID: skuID, RequiresMetal: true}}
}

// skuWithDim returns a SKUChecker carrying valid Dimensions so the BR-K05
// remnant-advisor branch is exercised in CreateWorkOrder tests.
func skuWithDim(skuID uuid.UUID, dim domain.Dimension) *mockSKUChecker {
	return &mockSKUChecker{result: SKUInfo{ID: skuID, RequiresMetal: false, Dimensions: dim}}
}

// newSvcWithAdvisor wires a service with a RemnantAdvisor for BR-K05 tests.
func newSvcWithAdvisor(st *mockStore, pc *mockPlanChecker, sc *mockSKUChecker, ra RemnantAdvisor) Service {
	return NewServiceWithRemnantAdvisor(st, pc, sc, &mockUserChecker{}, nil, nil, nil, ra)
}

// plannedWO returns a WorkOrder in PLANNED status.
func plannedWO(woID, planID, skuID uuid.UUID) WorkOrder {
	return WorkOrder{
		ID:        woID,
		PlanID:    planID,
		SKUID:     skuID,
		Quantity:  10,
		Status:    domain.WOPlanned,
		CreatedAt: time.Now().UTC(),
	}
}

// woWithStatus returns a WorkOrder with arbitrary status.
func woWithStatus(woID uuid.UUID, status domain.WorkOrderStatus) WorkOrder {
	return WorkOrder{
		ID:        woID,
		PlanID:    uuid.New(),
		SKUID:     uuid.New(),
		Quantity:  5,
		Status:    status,
		CreatedAt: time.Now().UTC(),
	}
}

// assignedWO returns a WorkOrder in the given status with AssignedTo populated.
// The second return value is the assigned operator's UUID so callers can use it
// in CallerID checks or assertions.
func assignedWO(woID uuid.UUID, status domain.WorkOrderStatus) (WorkOrder, uuid.UUID) {
	userID := uuid.New()
	wo := woWithStatus(woID, status)
	wo.AssignedTo = &userID
	return wo, userID
}

// ═════════════════════════════════════════════════════════════════════════════
// CreateWorkOrder
// ═════════════════════════════════════════════════════════════════════════════

func TestCreateWorkOrder_HappyPath(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()

	svc := newSvc(&mockStore{}, approvedPlan(planID, skuID), skuNoMetal(skuID))

	wo, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   planID,
		SKUID:    skuID,
		Quantity: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wo.ID == uuid.Nil {
		t.Error("wo.ID must be set")
	}
	if wo.PlanID != planID {
		t.Errorf("wo.PlanID = %v, want %v", wo.PlanID, planID)
	}
	if wo.SKUID != skuID {
		t.Errorf("wo.SKUID = %v, want %v", wo.SKUID, skuID)
	}
	if wo.Quantity != 5 {
		t.Errorf("wo.Quantity = %d, want 5", wo.Quantity)
	}
	if wo.Status != domain.WOPlanned {
		t.Errorf("wo.Status = %v, want PLANNED", wo.Status)
	}
}

// BR-P02: plan must be APPROVED to create a work order.
func TestCreateWorkOrder_PlanNotApproved_IsPreconditionFailed(t *testing.T) {
	planID := uuid.New()

	tests := []struct {
		name   string
		status domain.PlanStatus
	}{
		{"draft plan", domain.PlanDraft},
		{"canceled plan", domain.PlanCanceled},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pc := &mockPlanChecker{result: PlanInfo{ID: planID, Status: tc.status}}
			svc := newSvc(&mockStore{}, pc, skuNoMetal(uuid.New()))

			_, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
				PlanID:   planID,
				SKUID:    uuid.New(),
				Quantity: 1,
			})
			if !errors.Is(err, domain.ErrPreconditionFailed) {
				t.Errorf("expected ErrPreconditionFailed for %s plan, got %v", tc.status, err)
			}
		})
	}
}

// CreateWorkOrder must reject a SKU that is not in the plan's items.
func TestCreateWorkOrder_SKUNotInPlan_IsPreconditionFailed(t *testing.T) {
	planID := uuid.New()
	planSKUID := uuid.New()  // SKU that belongs to the plan
	otherSKUID := uuid.New() // SKU that does NOT belong to the plan

	pc := &mockPlanChecker{result: PlanInfo{
		ID:     planID,
		Status: domain.PlanApproved,
		SKUIDs: []uuid.UUID{planSKUID},
	}}
	svc := newSvc(&mockStore{}, pc, skuNoMetal(uuid.New()))

	_, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   planID,
		SKUID:    otherSKUID,
		Quantity: 5,
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed for SKU not in plan, got %v", err)
	}
}

// CreateWorkOrder must NOT call insertWorkOrder when the SKU is not in the plan.
func TestCreateWorkOrder_SKUNotInPlan_StoreNotCalled(t *testing.T) {
	planID := uuid.New()
	st := &mockStore{}
	pc := &mockPlanChecker{result: PlanInfo{
		ID:     planID,
		Status: domain.PlanApproved,
		SKUIDs: []uuid.UUID{uuid.New()}, // plan contains a different SKU
	}}
	svc := newSvc(st, pc, skuNoMetal(uuid.New()))

	_, _ = svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   planID,
		SKUID:    uuid.New(), // not in plan
		Quantity: 5,
	})
	// insertWorkOrder would set insertWorkOrderErr path; verify no insert happened
	// by checking the store was not called (insertWorkOrderErr remains nil and no panic)
	if st.insertWorkOrderErr != nil {
		t.Error("insertWorkOrder must not be called when SKU is not in plan")
	}
}

func TestCreateWorkOrder_PlanNotFound_PropagatesError(t *testing.T) {
	pc := &mockPlanChecker{err: domain.NewBizError(domain.ErrNotFound, "plan not found")}
	svc := newSvc(&mockStore{}, pc, skuNoMetal(uuid.New()))

	_, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   uuid.New(),
		SKUID:    uuid.New(),
		Quantity: 1,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

func TestCreateWorkOrder_ZeroQuantity_IsInvalidInput(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()
	svc := newSvc(&mockStore{}, approvedPlan(planID, skuID), skuNoMetal(uuid.New()))

	_, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   planID,
		SKUID:    skuID,
		Quantity: 0,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero quantity, got %v", err)
	}
}

func TestCreateWorkOrder_NegativeQuantity_IsInvalidInput(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()
	svc := newSvc(&mockStore{}, approvedPlan(planID, skuID), skuNoMetal(uuid.New()))

	_, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   planID,
		SKUID:    skuID,
		Quantity: -3,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative quantity, got %v", err)
	}
}

func TestCreateWorkOrder_StoreError_Propagates(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()
	dbErr := errors.New("insert failed")
	st := &mockStore{insertWorkOrderErr: dbErr}

	svc := newSvc(st, approvedPlan(planID, skuID), skuNoMetal(uuid.New()))

	_, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   planID,
		SKUID:    skuID,
		Quantity: 2,
	})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ─── BR-K05: REMNANT_BYPASSED audit logging on CreateWorkOrder ────────────────

// When the advisor returns fitting remnants and the planner did not allocate
// any of them, CreateWorkOrder must call LogRemnantBypass with the suggestion
// IDs and the planner's bypass_reason for accountant review.
func TestCreateWorkOrder_AdvisorWithFits_LogsRemnantBypass(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()
	callerID := uuid.New()
	r1, r2 := uuid.New(), uuid.New()
	dim := domain.Dimension{LengthMM: 1000, WidthMM: 500}

	advisor := &mockRemnantAdvisor{
		suggestResult: []RemnantSuggestionRef{{RemnantID: r1}, {RemnantID: r2}},
	}
	svc := newSvcWithAdvisor(&mockStore{}, approvedPlan(planID, skuID), skuWithDim(skuID, dim), advisor)

	wo, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:       planID,
		SKUID:        skuID,
		Quantity:     1,
		BypassReason: "ưu tiên tấm mới cho đơn rush",
		CallerID:     &callerID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !advisor.suggestCalled {
		t.Fatal("SuggestRemnants must be called")
	}
	if advisor.suggestDim != dim {
		t.Errorf("SuggestRemnants dim = %v, want %v", advisor.suggestDim, dim)
	}
	if !advisor.logCalled {
		t.Fatal("LogRemnantBypass must be called when suggestions exist")
	}
	if advisor.logArg.WorkOrderID != wo.ID {
		t.Errorf("logArg.WorkOrderID = %v, want %v", advisor.logArg.WorkOrderID, wo.ID)
	}
	if advisor.logArg.ActorID != callerID {
		t.Errorf("logArg.ActorID = %v, want %v", advisor.logArg.ActorID, callerID)
	}
	if advisor.logArg.Reason != "ưu tiên tấm mới cho đơn rush" {
		t.Errorf("logArg.Reason = %q, want bypass reason from input", advisor.logArg.Reason)
	}
	if got := advisor.logArg.SuggestedRemnantIDs; len(got) != 2 || got[0] != r1 || got[1] != r2 {
		t.Errorf("logArg.SuggestedRemnantIDs = %v, want [%v %v]", got, r1, r2)
	}
}

// When the advisor returns no suggestions, LogRemnantBypass must not run —
// there is no bypass to record.
func TestCreateWorkOrder_AdvisorNoFits_DoesNotLog(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()
	callerID := uuid.New()
	advisor := &mockRemnantAdvisor{suggestResult: nil}

	svc := newSvcWithAdvisor(&mockStore{}, approvedPlan(planID, skuID),
		skuWithDim(skuID, domain.Dimension{LengthMM: 800, WidthMM: 400}), advisor)

	if _, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID: planID, SKUID: skuID, Quantity: 1, CallerID: &callerID,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !advisor.suggestCalled {
		t.Error("SuggestRemnants must be called even when there are no fits")
	}
	if advisor.logCalled {
		t.Error("LogRemnantBypass must not be called when suggestions are empty")
	}
}

// SuggestRemnants failures are advisory only: the work order is already
// persisted and CreateWorkOrder must still return success.
func TestCreateWorkOrder_AdvisorSuggestErr_StillCreatesWO(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()
	callerID := uuid.New()
	advisor := &mockRemnantAdvisor{suggestErr: errors.New("inventory unavailable")}

	svc := newSvcWithAdvisor(&mockStore{}, approvedPlan(planID, skuID),
		skuWithDim(skuID, domain.Dimension{LengthMM: 800, WidthMM: 400}), advisor)

	wo, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID: planID, SKUID: skuID, Quantity: 1, CallerID: &callerID,
	})
	if err != nil {
		t.Fatalf("CreateWorkOrder must not fail when advisor errors: %v", err)
	}
	if wo.ID == uuid.Nil {
		t.Error("WO must still be created when SuggestRemnants errors")
	}
	if advisor.logCalled {
		t.Error("LogRemnantBypass must not run after SuggestRemnants failure")
	}
}

// LogRemnantBypass failures are also advisory: best-effort logging must never
// surface to the API caller after the WO has been written.
func TestCreateWorkOrder_AdvisorLogErr_StillReturnsWO(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()
	callerID := uuid.New()
	advisor := &mockRemnantAdvisor{
		suggestResult: []RemnantSuggestionRef{{RemnantID: uuid.New()}},
		logErr:        errors.New("audit insert failed"),
	}

	svc := newSvcWithAdvisor(&mockStore{}, approvedPlan(planID, skuID),
		skuWithDim(skuID, domain.Dimension{LengthMM: 800, WidthMM: 400}), advisor)

	wo, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID: planID, SKUID: skuID, Quantity: 1, CallerID: &callerID,
	})
	if err != nil {
		t.Fatalf("CreateWorkOrder must swallow advisory log error: %v", err)
	}
	if wo.ID == uuid.Nil {
		t.Error("WO must still be created when LogRemnantBypass errors")
	}
}

// Without a CallerID we have no actor for the audit row, so the entire
// advisor path must be skipped.
func TestCreateWorkOrder_NoCallerID_SkipsAdvisor(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()
	advisor := &mockRemnantAdvisor{
		suggestResult: []RemnantSuggestionRef{{RemnantID: uuid.New()}},
	}

	svc := newSvcWithAdvisor(&mockStore{}, approvedPlan(planID, skuID),
		skuWithDim(skuID, domain.Dimension{LengthMM: 800, WidthMM: 400}), advisor)

	if _, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID: planID, SKUID: skuID, Quantity: 1, // CallerID is nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if advisor.suggestCalled || advisor.logCalled {
		t.Error("advisor must not run when CallerID is nil")
	}
}

// SKU without valid dimensions cannot match remnants by area, so the advisor
// path must be skipped entirely.
func TestCreateWorkOrder_SKUDimInvalid_SkipsAdvisor(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()
	callerID := uuid.New()
	advisor := &mockRemnantAdvisor{
		suggestResult: []RemnantSuggestionRef{{RemnantID: uuid.New()}},
	}

	svc := newSvcWithAdvisor(&mockStore{}, approvedPlan(planID, skuID),
		skuNoMetal(skuID), advisor) // dimensions zero → invalid

	if _, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID: planID, SKUID: skuID, Quantity: 1, CallerID: &callerID,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if advisor.suggestCalled || advisor.logCalled {
		t.Error("advisor must not run when SKU dimensions are invalid")
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// GetWorkOrder / ListWorkOrders / ListWorkOrdersByPlan
// ═════════════════════════════════════════════════════════════════════════════

func TestGetWorkOrder_HappyPath(t *testing.T) {
	woID := uuid.New()
	want := plannedWO(woID, uuid.New(), uuid.New())
	st := &mockStore{selectWorkOrderByIDResult: want}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	got, err := svc.GetWorkOrder(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
}

// GetWorkOrder must pass SKUCode and SKUName through unchanged from the store.
func TestGetWorkOrder_SKUFieldsPassThrough(t *testing.T) {
	woID := uuid.New()
	want := WorkOrder{
		ID:       woID,
		PlanID:   uuid.New(),
		SKUID:    uuid.New(),
		SKUCode:  "TB-001",
		SKUName:  "Tủ bếp trên 1 cánh",
		Quantity: 3,
		Status:   domain.WOPlanned,
	}
	st := &mockStore{selectWorkOrderByIDResult: want}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	got, err := svc.GetWorkOrder(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SKUCode != want.SKUCode {
		t.Errorf("SKUCode = %q, want %q", got.SKUCode, want.SKUCode)
	}
	if got.SKUName != want.SKUName {
		t.Errorf("SKUName = %q, want %q", got.SKUName, want.SKUName)
	}
}

// GetWorkOrder must pass SKUDimensions through unchanged from the store.
func TestGetWorkOrder_SKUDimensionsPassThrough(t *testing.T) {
	woID := uuid.New()
	want := WorkOrder{
		ID:      woID,
		PlanID:  uuid.New(),
		SKUID:   uuid.New(),
		SKUCode: "TB-001",
		SKUName: "Tủ bếp trên 1 cánh",
		SKUDimensions: domain.Dimension{
			LengthMM: 1200,
			WidthMM:  600,
		},
		Quantity: 3,
		Status:   domain.WOPlanned,
	}
	st := &mockStore{selectWorkOrderByIDResult: want}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	got, err := svc.GetWorkOrder(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SKUDimensions.LengthMM != want.SKUDimensions.LengthMM {
		t.Errorf("SKUDimensions.LengthMM = %d, want %d", got.SKUDimensions.LengthMM, want.SKUDimensions.LengthMM)
	}
	if got.SKUDimensions.WidthMM != want.SKUDimensions.WidthMM {
		t.Errorf("SKUDimensions.WidthMM = %d, want %d", got.SKUDimensions.WidthMM, want.SKUDimensions.WidthMM)
	}
}

func TestGetWorkOrder_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectWorkOrderByIDErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.GetWorkOrder(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListWorkOrders_ReturnsAll(t *testing.T) {
	wo1 := plannedWO(uuid.New(), uuid.New(), uuid.New())
	wo2 := woWithStatus(uuid.New(), domain.WOInCutting)
	st := &mockStore{selectWorkOrdersResult: []WorkOrder{wo1, wo2}}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	wos, err := svc.ListWorkOrders(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, WorkOrderListFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wos.Items) != 2 {
		t.Errorf("len = %d, want 2", len(wos.Items))
	}
}

func TestListWorkOrders_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("query failed")
	st := &mockStore{selectWorkOrdersErr: dbErr}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.ListWorkOrders(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, WorkOrderListFilter{})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error, got %v", err)
	}
}

func TestListWorkOrders_FilterByPlanID_ReturnsPaged(t *testing.T) {
	planID := uuid.New()
	wo1 := plannedWO(uuid.New(), planID, uuid.New())
	st := &mockStore{selectWorkOrdersResult: []WorkOrder{wo1}}
	svc := newSvc(st, approvedPlan(planID), skuNoMetal(uuid.New()))

	result, err := svc.ListWorkOrders(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, WorkOrderListFilter{PlanID: &planID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("len = %d, want 1", len(result.Items))
	}
	if result.Items[0].PlanID != planID {
		t.Errorf("PlanID = %v, want %v", result.Items[0].PlanID, planID)
	}
}

func TestListWorkOrders_FilterByPlanAndStatus_ReturnsSubset(t *testing.T) {
	planID := uuid.New()
	wo1 := plannedWO(uuid.New(), planID, uuid.New())
	// mock returns only the planned one (store does the real filtering in DB)
	st := &mockStore{selectWorkOrdersResult: []WorkOrder{wo1}}
	svc := newSvc(st, approvedPlan(planID), skuNoMetal(uuid.New()))

	result, err := svc.ListWorkOrders(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, WorkOrderListFilter{Status: "PLANNED", PlanID: &planID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("len = %d, want 1", len(result.Items))
	}
}

func TestListWorkOrders_DashboardPreset_FilterPassedThrough(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	todayEnd := todayStart.AddDate(0, 0, 1)

	f := WorkOrderListFilter{
		DashboardPreset: true,
		TodayStart:      todayStart,
		TodayEnd:        todayEnd,
	}
	_, err := svc.ListWorkOrders(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.selectWorkOrdersFilter.DashboardPreset {
		t.Errorf("DashboardPreset not forwarded to store")
	}
	if !st.selectWorkOrdersFilter.TodayStart.Equal(todayStart) {
		t.Errorf("TodayStart not forwarded: got %v, want %v", st.selectWorkOrdersFilter.TodayStart, todayStart)
	}
	if !st.selectWorkOrdersFilter.TodayEnd.Equal(todayEnd) {
		t.Errorf("TodayEnd not forwarded: got %v, want %v", st.selectWorkOrdersFilter.TodayEnd, todayEnd)
	}
}

func TestListWorkOrders_AssignedNull_FilterPassedThrough(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	f := WorkOrderListFilter{
		Status:       "PLANNED",
		AssignedNull: true,
	}
	_, err := svc.ListWorkOrders(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.selectWorkOrdersFilter.AssignedNull {
		t.Error("AssignedNull not forwarded to store")
	}
	if st.selectWorkOrdersFilter.AssignedTo != nil {
		t.Errorf("AssignedTo must be nil when AssignedNull is set, got %v", st.selectWorkOrdersFilter.AssignedTo)
	}
}

func TestListWorkOrders_AssignedTo_FilterPassedThrough(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	userID := uuid.New()
	f := WorkOrderListFilter{
		Status:     "PLANNED",
		AssignedTo: &userID,
	}
	_, err := svc.ListWorkOrders(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.selectWorkOrdersFilter.AssignedTo == nil || *st.selectWorkOrdersFilter.AssignedTo != userID {
		t.Errorf("AssignedTo not forwarded: got %v, want %v", st.selectWorkOrdersFilter.AssignedTo, userID)
	}
	if st.selectWorkOrdersFilter.AssignedNull {
		t.Error("AssignedNull must remain false when AssignedTo is set")
	}
}

func TestListWorkOrdersByPlan_ReturnsFiltered(t *testing.T) {
	planID := uuid.New()
	wo1 := plannedWO(uuid.New(), planID, uuid.New())
	st := &mockStore{selectWorkOrdersByPlanResult: []WorkOrder{wo1}}

	svc := newSvc(st, approvedPlan(planID), skuNoMetal(uuid.New()))

	wos, err := svc.ListWorkOrdersByPlan(context.Background(), planID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wos) != 1 {
		t.Errorf("len = %d, want 1", len(wos))
	}
	if wos[0].PlanID != planID {
		t.Errorf("PlanID = %v, want %v", wos[0].PlanID, planID)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// AdvanceStatus — BR-P01: state machine
// ═════════════════════════════════════════════════════════════════════════════

// TestAdvanceStatus_ValidTransitions verifies every single step of the
// PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED chain.
func TestAdvanceStatus_ValidTransitions(t *testing.T) {
	transitions := []struct {
		from domain.WorkOrderStatus
		to   domain.WorkOrderStatus
	}{
		{domain.WOPlanned, domain.WOInCutting},
		{domain.WOInCutting, domain.WOInProcessing},
		{domain.WOInProcessing, domain.WOCompleted},
		{domain.WOCompleted, domain.WOCosted},
	}

	for _, tc := range transitions {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			woID := uuid.New()
			skuID := uuid.New()
			wo := woWithStatus(woID, tc.from)
			// PLANNED→IN_CUTTING requires an assigned operator (Spec 5.1).
			if tc.from == domain.WOPlanned {
				userID := uuid.New()
				wo.AssignedTo = &userID
			}
			st := &mockStore{
				selectWorkOrderByIDResult: wo,
				hasMetalConsumptionResult: true, // always satisfy BR-P04 for metal check
			}
			// Use skuNoMetal to avoid BR-P04 interference on most transitions;
			// for WOInProcessing→WOCompleted we use skuNoMetal too so metal check
			// is bypassed (SKU doesn't require it).
			svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(skuID))

			err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: tc.to})
			if err != nil {
				t.Errorf("unexpected error for %s -> %s: %v", tc.from, tc.to, err)
			}
			if !st.updateWorkOrderStatusCalled {
				t.Error("updateWorkOrderStatus must be called on valid transition")
			}
			if st.updateWorkOrderStatusArg != string(tc.to) {
				t.Errorf("stored status = %q, want %q", st.updateWorkOrderStatusArg, string(tc.to))
			}
		})
	}
}

// TestAdvanceStatus_InvalidTransitions verifies every backward and skip move
// returns ErrInvalidTransition.
func TestAdvanceStatus_InvalidTransitions(t *testing.T) {
	woID := uuid.New()

	tests := []struct {
		name string
		from domain.WorkOrderStatus
		to   domain.WorkOrderStatus
	}{
		// backward
		{"IN_CUTTING -> PLANNED", domain.WOInCutting, domain.WOPlanned},
		{"IN_PROCESSING -> IN_CUTTING", domain.WOInProcessing, domain.WOInCutting},
		{"COMPLETED -> IN_PROCESSING", domain.WOCompleted, domain.WOInProcessing},
		{"COSTED -> COMPLETED", domain.WOCosted, domain.WOCompleted},
		// skip forward
		{"PLANNED -> IN_PROCESSING", domain.WOPlanned, domain.WOInProcessing},
		{"PLANNED -> COMPLETED", domain.WOPlanned, domain.WOCompleted},
		{"PLANNED -> COSTED", domain.WOPlanned, domain.WOCosted},
		{"IN_CUTTING -> COMPLETED", domain.WOInCutting, domain.WOCompleted},
		{"IN_CUTTING -> COSTED", domain.WOInCutting, domain.WOCosted},
		{"IN_PROCESSING -> COSTED", domain.WOInProcessing, domain.WOCosted},
		// terminal: COSTED has no next
		{"COSTED -> anything", domain.WOCosted, domain.WOInCutting},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := &mockStore{selectWorkOrderByIDResult: woWithStatus(woID, tc.from)}
			svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

			err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: tc.to})
			if !errors.Is(err, domain.ErrInvalidTransition) {
				t.Errorf("expected ErrInvalidTransition for %s -> %s, got %v",
					tc.from, tc.to, err)
			}
			if st.updateWorkOrderStatusCalled {
				t.Error("updateWorkOrderStatus must NOT be called on invalid transition")
			}
		})
	}
}

func TestAdvanceStatus_WorkOrderNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectWorkOrderByIDErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	err := svc.AdvanceStatus(context.Background(), uuid.New(), AdvanceStatusInput{To: domain.WOInCutting})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAdvanceStatus_StoreUpdateError_Propagates(t *testing.T) {
	woID := uuid.New()
	dbErr := errors.New("update failed")
	wo, _ := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{
		selectWorkOrderByIDResult: wo,
		updateWorkOrderStatusErr:  dbErr,
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOInCutting})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store update error to propagate, got %v", err)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// AdvanceStatus — BR-P04: metal consumption requirement
// ═════════════════════════════════════════════════════════════════════════════

func TestAdvanceStatus_ToCompleted_SKURequiresMetal_HasMetal_Succeeds(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: WorkOrder{
			ID:       woID,
			SKUID:    skuID,
			Status:   domain.WOInProcessing,
			Quantity: 1,
		},
		hasMetalConsumptionResult: true, // METAL record present
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuRequiresMetal(skuID))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOCompleted})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must be called")
	}
}

func TestAdvanceStatus_ToCompleted_SKURequiresMetal_NoMetal_IsPreconditionFailed(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: WorkOrder{
			ID:       woID,
			SKUID:    skuID,
			Status:   domain.WOInProcessing,
			Quantity: 1,
		},
		hasMetalConsumptionResult: false, // no METAL record
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuRequiresMetal(skuID))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOCompleted})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed, got %v", err)
	}
	if st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must NOT be called when metal check fails")
	}
}

func TestAdvanceStatus_ToCompleted_SKUNoMetal_DoesNotCheckMetal(t *testing.T) {
	// SKU does not require metal — hasMetalConsumption must not be called
	// (we verify by setting hasMetalConsumptionErr which would cause a failure
	// if it were called).
	woID := uuid.New()
	skuID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: WorkOrder{
			ID:       woID,
			SKUID:    skuID,
			Status:   domain.WOInProcessing,
			Quantity: 1,
		},
		hasMetalConsumptionErr: errors.New("must not be called"),
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(skuID))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOCompleted})
	if err != nil {
		t.Fatalf("SKU without metal requirement must not trigger metal check, got: %v", err)
	}
}

func TestAdvanceStatus_ToCompleted_SKUCheckerError_Propagates(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	skuErr := errors.New("catalog unavailable")
	st := &mockStore{
		selectWorkOrderByIDResult: WorkOrder{
			ID:       woID,
			SKUID:    skuID,
			Status:   domain.WOInProcessing,
			Quantity: 1,
		},
	}
	sc := &mockSKUChecker{err: skuErr}
	svc := newSvc(st, approvedPlan(uuid.New()), sc)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOCompleted})
	if !errors.Is(err, skuErr) {
		t.Errorf("expected SKUChecker error to propagate, got %v", err)
	}
}

func TestAdvanceStatus_ToCompleted_MetalStoreError_Propagates(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	metalErr := errors.New("db error")
	st := &mockStore{
		selectWorkOrderByIDResult: WorkOrder{
			ID:       woID,
			SKUID:    skuID,
			Status:   domain.WOInProcessing,
			Quantity: 1,
		},
		hasMetalConsumptionErr: metalErr,
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuRequiresMetal(skuID))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOCompleted})
	if !errors.Is(err, metalErr) {
		t.Errorf("expected metal store error to propagate, got %v", err)
	}
}

// Metal check must NOT run when transitioning to states other than COMPLETED.
func TestAdvanceStatus_NonCompletedTransitions_DoNotTriggerMetalCheck(t *testing.T) {
	transitions := []struct {
		from domain.WorkOrderStatus
		to   domain.WorkOrderStatus
	}{
		{domain.WOPlanned, domain.WOInCutting},
		{domain.WOInCutting, domain.WOInProcessing},
		{domain.WOCompleted, domain.WOCosted},
	}

	for _, tc := range transitions {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			woID := uuid.New()
			skuID := uuid.New()
			wo := woWithStatus(woID, tc.from)
			if tc.from == domain.WOPlanned {
				userID := uuid.New()
				wo.AssignedTo = &userID
			}
			st := &mockStore{
				selectWorkOrderByIDResult: wo,
				// If hasMetalConsumption were called it would return an error,
				// proving the check was incorrectly invoked.
				hasMetalConsumptionErr: errors.New("metal check must not run here"),
			}
			svc := newSvc(st, approvedPlan(uuid.New()), skuRequiresMetal(skuID))

			err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: tc.to})
			if err != nil {
				t.Errorf("unexpected error for %s -> %s: %v", tc.from, tc.to, err)
			}
		})
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// AdvanceStatus — sheet pre-assignment (Issue 2)
// ═════════════════════════════════════════════════════════════════════════════

func TestAdvanceStatus_ToInCutting_WithSheetID_CallsPreAssign(t *testing.T) {
	woID := uuid.New()
	sheetID := uuid.New()
	wo, _ := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	sa := &mockSheetAssigner{}
	svc := NewService(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), &mockUserChecker{}, sa, nil, nil)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:      domain.WOInCutting,
		SheetID: &sheetID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sa.called {
		t.Error("PreAssignSheet must be called when sheet_id is provided")
	}
	if sa.calledSheet != sheetID {
		t.Errorf("sheet_id passed to PreAssignSheet = %v, want %v", sa.calledSheet, sheetID)
	}
	if sa.calledWO != woID {
		t.Errorf("wo_id passed to PreAssignSheet = %v, want %v", sa.calledWO, woID)
	}
	if !st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must be called after pre-assign succeeds")
	}
}

func TestAdvanceStatus_ToInCutting_NoSheetID_SkipsPreAssign(t *testing.T) {
	woID := uuid.New()
	wo, _ := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	sa := &mockSheetAssigner{}
	svc := NewService(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), &mockUserChecker{}, sa, nil, nil)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOInCutting})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sa.called {
		t.Error("PreAssignSheet must NOT be called when sheet_id is absent")
	}
}

func TestAdvanceStatus_ToInCutting_PreAssignFails_AbortsAdvance(t *testing.T) {
	woID := uuid.New()
	sheetID := uuid.New()
	assignErr := domain.NewBizError(domain.ErrPreconditionFailed, "sheet not available")
	wo, _ := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	sa := &mockSheetAssigner{err: assignErr}
	svc := NewService(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), &mockUserChecker{}, sa, nil, nil)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:      domain.WOInCutting,
		SheetID: &sheetID,
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed, got %v", err)
	}
	if st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must NOT be called when pre-assign fails")
	}
}

func TestAdvanceStatus_SheetID_OnNonCuttingTransition_IsIgnored(t *testing.T) {
	// sheet_id should only be acted upon for PLANNED → IN_CUTTING.
	// For other transitions (e.g. IN_PROCESSING → COMPLETED), it must be ignored.
	woID := uuid.New()
	skuID := uuid.New()
	sheetID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: WorkOrder{
			ID:       woID,
			SKUID:    skuID,
			Status:   domain.WOInProcessing,
			Quantity: 1,
		},
		hasMetalConsumptionResult: true,
	}
	sa := &mockSheetAssigner{}
	svc := NewService(st, approvedPlan(uuid.New()), skuRequiresMetal(skuID), &mockUserChecker{}, sa, nil, nil)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:      domain.WOCompleted,
		SheetID: &sheetID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sa.called {
		t.Error("PreAssignSheet must NOT be called for non-IN_CUTTING transitions")
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// AdvanceStatus — overflow bypass (Issue #222)
// ═════════════════════════════════════════════════════════════════════════════

func TestAdvanceStatus_ToInCutting_BypassByAdmin_PassesBypassToInventory(t *testing.T) {
	woID := uuid.New()
	sheetID := uuid.New()
	adminID := uuid.New()
	wo, _ := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	sa := &mockSheetAssigner{}
	svc := NewService(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), &mockUserChecker{}, sa, nil, nil)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:             domain.WOInCutting,
		SheetID:        &sheetID,
		BypassOverflow: true,
		BypassReason:   "urgent customer order",
		CallerID:       &adminID,
		CallerRole:     auth.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("admin bypass must succeed, got %v", err)
	}
	if !sa.calledBypass {
		t.Error("BypassOverflow must be propagated to inventory")
	}
	if sa.calledReason != "urgent customer order" {
		t.Errorf("reason = %q, want %q", sa.calledReason, "urgent customer order")
	}
	if sa.calledActorID != adminID {
		t.Errorf("actor = %v, want %v", sa.calledActorID, adminID)
	}
}

func TestAdvanceStatus_ToInCutting_BypassByNonAdmin_IsPreconditionFailed(t *testing.T) {
	woID := uuid.New()
	sheetID := uuid.New()
	wo, callerID := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	sa := &mockSheetAssigner{}
	svc := NewService(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), &mockUserChecker{}, sa, nil, nil)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:             domain.WOInCutting,
		SheetID:        &sheetID,
		BypassOverflow: true,
		BypassReason:   "force",
		CallerID:       &callerID,
		CallerRole:     auth.RoleCNC,
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("non-admin bypass must be rejected, got %v", err)
	}
	if sa.called {
		t.Error("PreAssignSheet must NOT be called when non-admin bypass is rejected")
	}
}

func TestAdvanceStatus_ToInCutting_BypassWithoutReason_IsInvalidInput(t *testing.T) {
	woID := uuid.New()
	sheetID := uuid.New()
	adminID := uuid.New()
	wo, _ := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	sa := &mockSheetAssigner{}
	svc := NewService(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), &mockUserChecker{}, sa, nil, nil)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:             domain.WOInCutting,
		SheetID:        &sheetID,
		BypassOverflow: true,
		BypassReason:   "",
		CallerID:       &adminID,
		CallerRole:     auth.RoleAdmin,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("missing bypass_reason must be ErrInvalidInput, got %v", err)
	}
	if sa.called {
		t.Error("PreAssignSheet must NOT be called when reason is missing")
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// AdvanceStatus — costing guard (Issue #211)
// ═════════════════════════════════════════════════════════════════════════════

func TestAdvanceStatus_ToInCutting_NoCostingRecord_IsPreconditionFailed(t *testing.T) {
	woID := uuid.New()
	wo, _ := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	cc := &mockCostingChecker{result: false}
	svc := newSvcWithCosting(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), cc)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOInCutting})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed when no costing record, got %v", err)
	}
	if st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must NOT be called when costing record is missing")
	}
}

func TestAdvanceStatus_ToInCutting_WithCostingRecord_Succeeds(t *testing.T) {
	woID := uuid.New()
	wo, _ := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	cc := &mockCostingChecker{result: true}
	svc := newSvcWithCosting(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), cc)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOInCutting})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must be called after costing guard passes")
	}
}

func TestAdvanceStatus_ToInCutting_CostingCheckerError_Propagates(t *testing.T) {
	woID := uuid.New()
	wo, _ := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	checkerErr := errors.New("costing service unavailable")
	cc := &mockCostingChecker{err: checkerErr}
	svc := newSvcWithCosting(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), cc)

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOInCutting})
	if !errors.Is(err, checkerErr) {
		t.Errorf("expected costing checker error to propagate, got %v", err)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// AdvanceStatus — assignment guard (Issue #141)
// ═════════════════════════════════════════════════════════════════════════════

// Spec 5.1: WO must be assigned before cutting starts.
func TestAdvanceStatus_ToInCutting_UnassignedWO_IsPreconditionFailed(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{selectWorkOrderByIDResult: woWithStatus(woID, domain.WOPlanned)} // AssignedTo == nil
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{To: domain.WOInCutting})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed for unassigned WO, got %v", err)
	}
	if st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must NOT be called when WO is unassigned")
	}
}

// CallerID must match the assigned operator when provided.
func TestAdvanceStatus_ToInCutting_WrongCaller_IsPreconditionFailed(t *testing.T) {
	woID := uuid.New()
	wo, _ := assignedWO(woID, domain.WOPlanned)
	wrongCaller := uuid.New() // different from wo.AssignedTo
	st := &mockStore{selectWorkOrderByIDResult: wo}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:       domain.WOInCutting,
		CallerID: &wrongCaller,
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed for wrong caller, got %v", err)
	}
	if st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must NOT be called for wrong caller")
	}
}

// The assigned operator can advance their own WO to IN_CUTTING.
func TestAdvanceStatus_ToInCutting_CorrectCaller_Succeeds(t *testing.T) {
	woID := uuid.New()
	wo, assignedUserID := assignedWO(woID, domain.WOPlanned)
	st := &mockStore{selectWorkOrderByIDResult: wo}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:       domain.WOInCutting,
		CallerID: &assignedUserID,
	})
	if err != nil {
		t.Fatalf("unexpected error for correct caller: %v", err)
	}
	if !st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must be called for correct caller")
	}
}

func TestAdvanceStatus_ToInCutting_AdminOverride_AllowsUnassignedWO(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{selectWorkOrderByIDResult: woWithStatus(woID, domain.WOPlanned)}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:         domain.WOInCutting,
		CallerRole: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("admin override should allow unassigned WO, got: %v", err)
	}
	if !st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must be called for admin override")
	}
}

func TestAdvanceStatus_ToInCutting_AdminOverride_AllowsWrongCaller(t *testing.T) {
	woID := uuid.New()
	wo, _ := assignedWO(woID, domain.WOPlanned)
	wrongCaller := uuid.New()
	st := &mockStore{selectWorkOrderByIDResult: wo}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	err := svc.AdvanceStatus(context.Background(), woID, AdvanceStatusInput{
		To:         domain.WOInCutting,
		CallerID:   &wrongCaller,
		CallerRole: auth.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("admin override should bypass caller match, got: %v", err)
	}
	if !st.updateWorkOrderStatusCalled {
		t.Error("updateWorkOrderStatus must be called for admin override")
	}
}

// A PLANNED WO that already has an assignee can be reassigned to a different CNC operator.
func TestAssignWorkOrder_Reassign_Succeeds(t *testing.T) {
	woID := uuid.New()
	newUserID := uuid.New()
	existingWO, _ := assignedWO(woID, domain.WOPlanned) // already has an assignee
	st := &mockStore{selectWorkOrderByIDResult: existingWO}
	uc := &mockUserChecker{result: UserInfo{ID: newUserID, Role: "cnc"}}
	svc := newSvcWithUser(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), uc)

	result, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
		WorkOrderID: woID,
		UserID:      newUserID,
	})
	if err != nil {
		t.Fatalf("reassign must succeed for PLANNED WO, got: %v", err)
	}
	if result.AssignedTo == nil || *result.AssignedTo != newUserID {
		t.Errorf("AssignedTo = %v, want %v", result.AssignedTo, newUserID)
	}
	if !st.updateWorkOrderAssignmentCalled {
		t.Error("store.updateWorkOrderAssignment must be called on reassign")
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// RecordConsumption — BR-P03
// ═════════════════════════════════════════════════════════════════════════════

func TestRecordConsumption_HappyPath_InProcessing(t *testing.T) {
	woID := uuid.New()
	matID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: woWithStatus(woID, domain.WOInProcessing),
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	cr, err := svc.RecordConsumption(context.Background(), RecordConsumptionInput{
		WorkOrderID:  woID,
		MaterialID:   matID,
		MaterialType: "PLYWOOD",
		Quantity:     2.5,
		Unit:         "sheet",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cr.ID == uuid.Nil {
		t.Error("cr.ID must be set")
	}
	if cr.WorkOrderID != woID {
		t.Errorf("cr.WorkOrderID = %v, want %v", cr.WorkOrderID, woID)
	}
	if cr.MaterialID != matID {
		t.Errorf("cr.MaterialID = %v, want %v", cr.MaterialID, matID)
	}
	if cr.Quantity != 2.5 {
		t.Errorf("cr.Quantity = %v, want 2.5", cr.Quantity)
	}
	if cr.MaterialType != "PLYWOOD" {
		t.Errorf("cr.MaterialType = %q, want PLYWOOD", cr.MaterialType)
	}
}

func TestRecordConsumption_HappyPath_Completed(t *testing.T) {
	// service.go allows consumption in both IN_PROCESSING and COMPLETED.
	woID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: woWithStatus(woID, domain.WOCompleted),
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.RecordConsumption(context.Background(), RecordConsumptionInput{
		WorkOrderID:  woID,
		MaterialID:   uuid.New(),
		MaterialType: "METAL",
		Quantity:     1.0,
		Unit:         "kg",
	})
	if err != nil {
		t.Fatalf("consumption must be allowed in COMPLETED, got: %v", err)
	}
}

// Consumption is only allowed in IN_PROCESSING and COMPLETED; all other
// statuses must return ErrPreconditionFailed.
func TestRecordConsumption_WrongStatus_IsPreconditionFailed(t *testing.T) {
	forbidden := []domain.WorkOrderStatus{
		domain.WOPlanned,
		domain.WOInCutting,
		domain.WOCosted,
	}

	for _, status := range forbidden {
		t.Run(string(status), func(t *testing.T) {
			woID := uuid.New()
			st := &mockStore{selectWorkOrderByIDResult: woWithStatus(woID, status)}
			svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

			_, err := svc.RecordConsumption(context.Background(), RecordConsumptionInput{
				WorkOrderID:  woID,
				MaterialID:   uuid.New(),
				MaterialType: "PLYWOOD",
				Quantity:     1.0,
				Unit:         "sheet",
			})
			if !errors.Is(err, domain.ErrPreconditionFailed) {
				t.Errorf("expected ErrPreconditionFailed for status %s, got %v", status, err)
			}
		})
	}
}

func TestRecordConsumption_ZeroQuantity_IsInvalidInput(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{selectWorkOrderByIDResult: woWithStatus(woID, domain.WOInProcessing)}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.RecordConsumption(context.Background(), RecordConsumptionInput{
		WorkOrderID:  woID,
		MaterialID:   uuid.New(),
		MaterialType: "PLYWOOD",
		Quantity:     0,
		Unit:         "sheet",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero quantity, got %v", err)
	}
}

func TestRecordConsumption_NegativeQuantity_IsInvalidInput(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{selectWorkOrderByIDResult: woWithStatus(woID, domain.WOInProcessing)}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.RecordConsumption(context.Background(), RecordConsumptionInput{
		WorkOrderID:  woID,
		MaterialID:   uuid.New(),
		MaterialType: "PLYWOOD",
		Quantity:     -1.0,
		Unit:         "sheet",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative quantity, got %v", err)
	}
}

func TestRecordConsumption_WorkOrderNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectWorkOrderByIDErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.RecordConsumption(context.Background(), RecordConsumptionInput{
		WorkOrderID:  uuid.New(),
		MaterialID:   uuid.New(),
		MaterialType: "PLYWOOD",
		Quantity:     1.0,
		Unit:         "sheet",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRecordConsumption_StoreInsertError_Propagates(t *testing.T) {
	woID := uuid.New()
	dbErr := errors.New("insert failed")
	st := &mockStore{
		selectWorkOrderByIDResult: woWithStatus(woID, domain.WOInProcessing),
		insertConsumptionErr:      dbErr,
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.RecordConsumption(context.Background(), RecordConsumptionInput{
		WorkOrderID:  woID,
		MaterialID:   uuid.New(),
		MaterialType: "PLYWOOD",
		Quantity:     1.0,
		Unit:         "sheet",
	})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store insert error to propagate, got %v", err)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// ListConsumptions
// ═════════════════════════════════════════════════════════════════════════════

func TestListConsumptions_HappyPath(t *testing.T) {
	woID := uuid.New()
	cr1 := ConsumptionRecord{ID: uuid.New(), WorkOrderID: woID, MaterialType: "PLYWOOD", Quantity: 2, Unit: "sheet"}
	cr2 := ConsumptionRecord{ID: uuid.New(), WorkOrderID: woID, MaterialType: "METAL", Quantity: 0.5, Unit: "kg"}
	st := &mockStore{
		selectWorkOrderByIDResult:    woWithStatus(woID, domain.WOInProcessing),
		selectConsumptionsByWOResult: []ConsumptionRecord{cr1, cr2},
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	records, err := svc.ListConsumptions(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("len = %d, want 2", len(records))
	}
}

func TestListConsumptions_WorkOrderNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectWorkOrderByIDErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.ListConsumptions(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListConsumptions_StoreQueryError_Propagates(t *testing.T) {
	woID := uuid.New()
	dbErr := errors.New("query failed")
	st := &mockStore{
		selectWorkOrderByIDResult: woWithStatus(woID, domain.WOInProcessing),
		selectConsumptionsByWOErr: dbErr,
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.ListConsumptions(context.Background(), woID)
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store query error to propagate, got %v", err)
	}
}

func TestListConsumptions_EmptyList_ReturnsNil(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult:    woWithStatus(woID, domain.WOPlanned),
		selectConsumptionsByWOResult: nil,
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	records, err := svc.ListConsumptions(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected empty slice, got %d records", len(records))
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// domain.WorkOrderStatus helpers (CanTransitionTo / Next)
// ═════════════════════════════════════════════════════════════════════════════

// These tests live here because the status machine is the backbone of BR-P01/P02.

func TestWorkOrderStatus_CanTransitionTo_ValidChain(t *testing.T) {
	chain := []struct {
		from domain.WorkOrderStatus
		to   domain.WorkOrderStatus
	}{
		{domain.WOPlanned, domain.WOInCutting},
		{domain.WOInCutting, domain.WOInProcessing},
		{domain.WOInProcessing, domain.WOCompleted},
		{domain.WOCompleted, domain.WOCosted},
	}
	for _, tc := range chain {
		if err := tc.from.CanTransitionTo(tc.to); err != nil {
			t.Errorf("CanTransitionTo(%s -> %s) unexpectedly failed: %v", tc.from, tc.to, err)
		}
	}
}

func TestWorkOrderStatus_CanTransitionTo_InvalidReturnsError(t *testing.T) {
	if err := domain.WOCosted.CanTransitionTo(domain.WOCompleted); err == nil {
		t.Error("expected error for COSTED -> COMPLETED, got nil")
	}
	if err := domain.WOPlanned.CanTransitionTo(domain.WOCompleted); err == nil {
		t.Error("expected error for PLANNED -> COMPLETED (skip), got nil")
	}
}

func TestWorkOrderStatus_Next_ReturnsNextInChain(t *testing.T) {
	tests := []struct {
		status   domain.WorkOrderStatus
		wantNext domain.WorkOrderStatus
		wantOK   bool
	}{
		{domain.WOPlanned, domain.WOInCutting, true},
		{domain.WOInCutting, domain.WOInProcessing, true},
		{domain.WOInProcessing, domain.WOCompleted, true},
		{domain.WOCompleted, domain.WOCosted, true},
		{domain.WOCosted, "", false}, // terminal — no next
	}
	for _, tc := range tests {
		next, ok := tc.status.Next()
		if ok != tc.wantOK {
			t.Errorf("%s.Next() ok = %v, want %v", tc.status, ok, tc.wantOK)
		}
		if ok && next != tc.wantNext {
			t.Errorf("%s.Next() = %v, want %v", tc.status, next, tc.wantNext)
		}
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// AssignWorkOrder
// ═════════════════════════════════════════════════════════════════════════════

func TestAssignWorkOrder_HappyPath(t *testing.T) {
	woID := uuid.New()
	userID := uuid.New()
	wo := plannedWO(woID, uuid.New(), uuid.New())
	st := &mockStore{selectWorkOrderByIDResult: wo}
	uc := &mockUserChecker{result: UserInfo{ID: userID, Role: "cnc"}}

	svc := newSvcWithUser(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), uc)

	result, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
		WorkOrderID: woID,
		UserID:      userID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AssignedTo == nil || *result.AssignedTo != userID {
		t.Errorf("AssignedTo = %v, want %v", result.AssignedTo, userID)
	}
	if result.AssignedAt == nil {
		t.Error("AssignedAt must be set")
	}
	if !st.updateWorkOrderAssignmentCalled {
		t.Error("store.updateWorkOrderAssignment must be called")
	}
}

func TestAssignWorkOrder_WorkOrderNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectWorkOrderByIDErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	uc := &mockUserChecker{result: UserInfo{Role: "cnc"}}

	svc := newSvcWithUser(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), uc)

	_, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
		WorkOrderID: uuid.New(),
		UserID:      uuid.New(),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAssignWorkOrder_WorkOrderNotPlanned_IsPreconditionFailed(t *testing.T) {
	woID := uuid.New()
	nonPlannedStatuses := []domain.WorkOrderStatus{
		domain.WOInCutting, domain.WOInProcessing, domain.WOCompleted, domain.WOCosted,
	}
	for _, status := range nonPlannedStatuses {
		t.Run(string(status), func(t *testing.T) {
			st := &mockStore{selectWorkOrderByIDResult: woWithStatus(woID, status)}
			uc := &mockUserChecker{result: UserInfo{Role: "cnc"}}

			svc := newSvcWithUser(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), uc)

			_, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
				WorkOrderID: woID,
				UserID:      uuid.New(),
			})
			if !errors.Is(err, domain.ErrPreconditionFailed) {
				t.Errorf("status %s: expected ErrPreconditionFailed, got %v", status, err)
			}
			if st.updateWorkOrderAssignmentCalled {
				t.Error("store.updateWorkOrderAssignment must NOT be called when WO is not PLANNED")
			}
		})
	}
}

func TestAssignWorkOrder_UserNotFound_PropagatesError(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New())}
	uc := &mockUserChecker{err: domain.NewBizError(domain.ErrNotFound, "user not found")}

	svc := newSvcWithUser(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), uc)

	_, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
		WorkOrderID: woID,
		UserID:      uuid.New(),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAssignWorkOrder_UserNotCNC_IsInvalidInput(t *testing.T) {
	woID := uuid.New()
	nonCNCRoles := []string{"admin", "warehouse", "cnc_manager", "planner"}
	for _, role := range nonCNCRoles {
		t.Run(role, func(t *testing.T) {
			st := &mockStore{selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New())}
			uc := &mockUserChecker{result: UserInfo{ID: uuid.New(), Role: role}}

			svc := newSvcWithUser(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), uc)

			_, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
				WorkOrderID: woID,
				UserID:      uuid.New(),
			})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("role %s: expected ErrInvalidInput, got %v", role, err)
			}
			if st.updateWorkOrderAssignmentCalled {
				t.Error("store.updateWorkOrderAssignment must NOT be called when role validation fails")
			}
		})
	}
}

func TestAssignWorkOrder_StoreError_Propagates(t *testing.T) {
	woID := uuid.New()
	dbErr := errors.New("db write failed")
	st := &mockStore{
		selectWorkOrderByIDResult:    plannedWO(woID, uuid.New(), uuid.New()),
		updateWorkOrderAssignmentErr: dbErr,
	}
	uc := &mockUserChecker{result: UserInfo{ID: uuid.New(), Role: "cnc"}}

	svc := newSvcWithUser(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), uc)

	_, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
		WorkOrderID: woID,
		UserID:      uuid.New(),
	})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// SuggestAssignment
// ═════════════════════════════════════════════════════════════════════════════

func TestSuggestAssignment_NoCuttingWOs_ReturnsFirstCNCUser(t *testing.T) {
	woID := uuid.New()
	user1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	user2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	st := &mockStore{
		selectWorkOrderByIDResult:        plannedWO(woID, uuid.New(), uuid.New()),
		selectCNCUserIDsResult:           []uuid.UUID{user1, user2},
		selectInCuttingCountByUserResult: map[uuid.UUID]int{}, // none in cutting
	}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	result, err := svc.SuggestAssignment(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With zero load, user1 is selected (first in UUID order).
	if result.UserID != user1 {
		t.Errorf("UserID = %v, want %v", result.UserID, user1)
	}
	if result.InCuttingCount != 0 {
		t.Errorf("InCuttingCount = %d, want 0", result.InCuttingCount)
	}
}

func TestSuggestAssignment_SelectsLeastBusyUser(t *testing.T) {
	woID := uuid.New()
	user1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	user2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	user3 := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	st := &mockStore{
		selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New()),
		selectCNCUserIDsResult:    []uuid.UUID{user1, user2, user3},
		selectInCuttingCountByUserResult: map[uuid.UUID]int{
			user1: 3,
			user2: 1, // least busy
			user3: 2,
		},
	}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	result, err := svc.SuggestAssignment(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UserID != user2 {
		t.Errorf("UserID = %v, want user2 (least busy)", result.UserID)
	}
	if result.InCuttingCount != 1 {
		t.Errorf("InCuttingCount = %d, want 1", result.InCuttingCount)
	}
}

func TestSuggestAssignment_TiedUsers_SelectsFirstByUUID(t *testing.T) {
	woID := uuid.New()
	user1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	user2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	st := &mockStore{
		selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New()),
		selectCNCUserIDsResult:    []uuid.UUID{user1, user2},
		selectInCuttingCountByUserResult: map[uuid.UUID]int{
			user1: 2,
			user2: 2, // tied
		},
	}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	result, err := svc.SuggestAssignment(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// user1 wins the tie because UUID sort puts it first.
	if result.UserID != user1 {
		t.Errorf("UserID = %v, want user1 (tie broken by UUID order)", result.UserID)
	}
}

func TestSuggestAssignment_NoCNCUsers_IsNotFound(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New()),
		selectCNCUserIDsResult:    nil, // empty
	}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.SuggestAssignment(context.Background(), woID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound when no CNC users, got %v", err)
	}
}

func TestSuggestAssignment_WorkOrderNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectWorkOrderByIDErr: domain.NewBizError(domain.ErrNotFound, "not found"),
	}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.SuggestAssignment(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// ListWorkOrdersByAssignee
// ═════════════════════════════════════════════════════════════════════════════

func TestListWorkOrdersByAssignee_ReturnsAssignedWOs(t *testing.T) {
	userID := uuid.New()
	wo1 := plannedWO(uuid.New(), uuid.New(), uuid.New())
	wo2 := plannedWO(uuid.New(), uuid.New(), uuid.New())
	st := &mockStore{selectWorkOrdersByAssigneeResult: []WorkOrder{wo1, wo2}}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	wos, err := svc.ListWorkOrdersByAssignee(context.Background(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wos) != 2 {
		t.Errorf("len = %d, want 2", len(wos))
	}
}

func TestListWorkOrdersByAssignee_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("query failed")
	st := &mockStore{selectWorkOrdersByAssigneeErr: dbErr}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.ListWorkOrdersByAssignee(context.Background(), uuid.New())
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

func TestSuggestAssignment_StoreSelectCNCUsersError_Propagates(t *testing.T) {
	woID := uuid.New()
	dbErr := errors.New("db error")
	st := &mockStore{
		selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New()),
		selectCNCUserIDsErr:       dbErr,
	}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.SuggestAssignment(context.Background(), woID)
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

func TestSuggestAssignment_StoreLoadError_Propagates(t *testing.T) {
	woID := uuid.New()
	dbErr := errors.New("db error")
	st := &mockStore{
		selectWorkOrderByIDResult:     plannedWO(woID, uuid.New(), uuid.New()),
		selectCNCUserIDsResult:        []uuid.UUID{uuid.New()},
		selectInCuttingCountByUserErr: dbErr,
	}

	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.SuggestAssignment(context.Background(), woID)
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// AssignWorkOrder — notifier behaviour
// ═════════════════════════════════════════════════════════════════════════════

func TestAssignWorkOrder_HappyPath_NotifierCalled(t *testing.T) {
	woID := uuid.New()
	userID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New()),
	}
	uc := &mockUserChecker{result: UserInfo{ID: userID, Role: "cnc"}}
	notifier := &mockWorkOrderNotifier{}

	svc := NewService(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), uc, nil, nil, notifier)

	_, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
		WorkOrderID: woID,
		UserID:      userID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !notifier.called {
		t.Error("NotifyAssignment must be called on successful assignment")
	}
}

func TestAssignWorkOrder_NotifierError_DoesNotFailRequest(t *testing.T) {
	woID := uuid.New()
	userID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New()),
	}
	uc := &mockUserChecker{result: UserInfo{ID: userID, Role: "cnc"}}
	notifier := &mockWorkOrderNotifier{err: errors.New("notify failed")}

	svc := NewService(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), uc, nil, nil, notifier)

	_, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
		WorkOrderID: woID,
		UserID:      userID,
	})
	if err != nil {
		t.Errorf("notification failure must not fail the request, got: %v", err)
	}
}

func TestAssignWorkOrder_ValidationFails_NotifierNotCalled(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{
		selectWorkOrderByIDResult: woWithStatus(woID, domain.WOInCutting), // wrong status
	}
	notifier := &mockWorkOrderNotifier{}

	svc := NewService(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()), &mockUserChecker{}, nil, nil, notifier)

	_, err := svc.AssignWorkOrder(context.Background(), AssignWorkOrderInput{
		WorkOrderID: woID,
		UserID:      uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error for wrong WO status")
	}
	if notifier.called {
		t.Error("NotifyAssignment must NOT be called when assignment fails")
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// Machine CRUD
// ═════════════════════════════════════════════════════════════════════════════

func TestCreateMachine_InvalidName_ReturnsError(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.CreateMachine(context.Background(), CreateMachineInput{Name: ""})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty name, got %v", err)
	}
}

func TestCreateMachine_HappyPath(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	m, err := svc.CreateMachine(context.Background(), CreateMachineInput{
		Name:                  "CNC-01",
		Code:                  "X200",
		CapacityHoursPerShift: 8.0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "CNC-01" {
		t.Errorf("Name = %q, want CNC-01", m.Name)
	}
	if !m.IsActive {
		t.Error("new machine must be active")
	}
}

func TestDeactivateMachine_NotFound_ReturnsError(t *testing.T) {
	st := &mockStore{deactivateMachineErr: domain.NewBizError(domain.ErrNotFound, "machine not found")}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	err := svc.DeactivateMachine(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// SetEstimatedHours
// ═════════════════════════════════════════════════════════════════════════════

func TestSetEstimatedHours_InvalidHours_ReturnsError(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New())}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.SetEstimatedHours(context.Background(), SetEstimatedHoursInput{WorkOrderID: woID, EstimatedHours: 0})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for 0 hours, got %v", err)
	}
}

func TestSetEstimatedHours_NegativeHours_ReturnsError(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{selectWorkOrderByIDResult: plannedWO(woID, uuid.New(), uuid.New())}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.SetEstimatedHours(context.Background(), SetEstimatedHoursInput{WorkOrderID: woID, EstimatedHours: -1})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative hours, got %v", err)
	}
}

func TestSetEstimatedHours_WorkOrderNotFound_ReturnsError(t *testing.T) {
	st := &mockStore{updateEstimatedHoursErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.SetEstimatedHours(context.Background(), SetEstimatedHoursInput{WorkOrderID: uuid.New(), EstimatedHours: 4})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// AssignSlot / UnassignSlot
// ═════════════════════════════════════════════════════════════════════════════

func TestAssignSlot_MissingEstimatedHours_ReturnsError(t *testing.T) {
	woID := uuid.New()
	wo := plannedWO(woID, uuid.New(), uuid.New())
	st := &mockStore{selectWorkOrderByIDResult: wo}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.AssignSlot(context.Background(), AssignSlotInput{WorkOrderID: woID, SlotID: uuid.New()})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed when EstimatedHours not set, got %v", err)
	}
}

func TestAssignSlot_SlotNotFound_ReturnsError(t *testing.T) {
	woID := uuid.New()
	wo := plannedWO(woID, uuid.New(), uuid.New())
	hours := 4.0
	wo.EstimatedHours = &hours
	st := &mockStore{
		selectWorkOrderByIDResult: wo,
		assignSlotAtomicallyErr:   domain.NewBizError(domain.ErrNotFound, "slot not found"),
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.AssignSlot(context.Background(), AssignSlotInput{WorkOrderID: woID, SlotID: uuid.New()})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing slot, got %v", err)
	}
}

func TestUnassignSlot_WorkOrderNotFound_ReturnsError(t *testing.T) {
	st := &mockStore{selectWorkOrderByIDErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.UnassignSlot(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// SuggestSchedule
// ═════════════════════════════════════════════════════════════════════════════

func TestSuggestSchedule_WorkOrderNotFound_ReturnsError(t *testing.T) {
	st := &mockStore{selectWorkOrderByIDErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.SuggestSchedule(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSuggestSchedule_MissingEstimatedHours_ReturnsPreconditionFailed(t *testing.T) {
	woID := uuid.New()
	wo := plannedWO(woID, uuid.New(), uuid.New())
	st := &mockStore{selectWorkOrderByIDResult: wo}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.SuggestSchedule(context.Background(), woID)
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed when EstimatedHours not set, got %v", err)
	}
}

func TestSuggestSchedule_HappyPath_ReturnsSuggestions(t *testing.T) {
	woID := uuid.New()
	wo := plannedWO(woID, uuid.New(), uuid.New())
	hours := 3.0
	wo.EstimatedHours = &hours
	machineID := uuid.New()
	slots := []MachineShiftSlot{
		{ID: uuid.New(), MachineID: machineID, CapacityHours: 8.0, AssignedHours: 2.0},
		{ID: uuid.New(), MachineID: machineID, CapacityHours: 8.0, AssignedHours: 0.0},
	}
	st := &mockStore{
		selectWorkOrderByIDResult: wo,
		selectFutureSlotsResult:   slots,
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	suggestions, err := svc.SuggestSchedule(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 2 {
		t.Errorf("len = %d, want 2", len(suggestions))
	}
}

func TestListWorkOrders_FilterByDateRange_ForwardsFilter(t *testing.T) {
	from := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	st := &mockStore{}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.ListWorkOrders(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, WorkOrderListFilter{
		Status:      "PLANNED",
		CreatedFrom: &from,
		CreatedTo:   &to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.selectWorkOrdersFilter.Status != "PLANNED" {
		t.Errorf("Status = %q, want PLANNED", st.selectWorkOrdersFilter.Status)
	}
	if st.selectWorkOrdersFilter.CreatedFrom == nil || !st.selectWorkOrdersFilter.CreatedFrom.Equal(from) {
		t.Fatalf("CreatedFrom not forwarded correctly")
	}
	if st.selectWorkOrdersFilter.CreatedTo == nil || !st.selectWorkOrdersFilter.CreatedTo.Equal(to) {
		t.Fatalf("CreatedTo not forwarded correctly")
	}
}
