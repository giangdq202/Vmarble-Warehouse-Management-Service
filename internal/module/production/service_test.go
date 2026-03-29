package production

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

// ── mock implementations ──────────────────────────────────────────────────────

// mockStore satisfies the full store interface.
type mockStore struct {
	// insertWorkOrder
	insertWorkOrderErr error

	// selectWorkOrders
	selectWorkOrdersResult []WorkOrder
	selectWorkOrdersErr    error

	// selectWorkOrderByID
	selectWorkOrderByIDResult WorkOrder
	selectWorkOrderByIDErr    error

	// selectWorkOrdersByPlan
	selectWorkOrdersByPlanResult []WorkOrder
	selectWorkOrdersByPlanErr    error

	// updateWorkOrderStatus — record calls for inspection
	updateWorkOrderStatusCalled bool
	updateWorkOrderStatusArg    string
	updateWorkOrderStatusErr    error

	// insertConsumption
	insertConsumptionErr error

	// selectConsumptionsByWO
	selectConsumptionsByWOResult []ConsumptionRecord
	selectConsumptionsByWOErr    error

	// hasMetalConsumption
	hasMetalConsumptionResult bool
	hasMetalConsumptionErr    error
}

func (m *mockStore) insertWorkOrder(_ context.Context, _ WorkOrder) error {
	return m.insertWorkOrderErr
}
func (m *mockStore) selectWorkOrders(_ context.Context) ([]WorkOrder, error) {
	return m.selectWorkOrdersResult, m.selectWorkOrdersErr
}
func (m *mockStore) selectWorkOrderByID(_ context.Context, _ uuid.UUID) (WorkOrder, error) {
	return m.selectWorkOrderByIDResult, m.selectWorkOrderByIDErr
}
func (m *mockStore) selectWorkOrdersByPlan(_ context.Context, _ uuid.UUID) ([]WorkOrder, error) {
	return m.selectWorkOrdersByPlanResult, m.selectWorkOrdersByPlanErr
}
func (m *mockStore) updateWorkOrderStatus(_ context.Context, _ uuid.UUID, status string) error {
	m.updateWorkOrderStatusCalled = true
	m.updateWorkOrderStatusArg = status
	return m.updateWorkOrderStatusErr
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

// ── helpers ───────────────────────────────────────────────────────────────────

func newSvc(st *mockStore, pc *mockPlanChecker, sc *mockSKUChecker) Service {
	return NewService(st, pc, sc)
}

// approvedPlan returns a PlanChecker that returns an APPROVED plan.
func approvedPlan(planID uuid.UUID) *mockPlanChecker {
	return &mockPlanChecker{result: PlanInfo{ID: planID, Status: domain.PlanApproved}}
}

// skuNoMetal returns a SKUChecker for a SKU that does not require metal.
func skuNoMetal(skuID uuid.UUID) *mockSKUChecker {
	return &mockSKUChecker{result: SKUInfo{ID: skuID, RequiresMetal: false}}
}

// skuRequiresMetal returns a SKUChecker for a SKU that requires metal.
func skuRequiresMetal(skuID uuid.UUID) *mockSKUChecker {
	return &mockSKUChecker{result: SKUInfo{ID: skuID, RequiresMetal: true}}
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

// ═════════════════════════════════════════════════════════════════════════════
// CreateWorkOrder
// ═════════════════════════════════════════════════════════════════════════════

func TestCreateWorkOrder_HappyPath(t *testing.T) {
	planID := uuid.New()
	skuID := uuid.New()

	svc := newSvc(&mockStore{}, approvedPlan(planID), skuNoMetal(skuID))

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
	svc := newSvc(&mockStore{}, approvedPlan(planID), skuNoMetal(uuid.New()))

	_, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   planID,
		SKUID:    uuid.New(),
		Quantity: 0,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero quantity, got %v", err)
	}
}

func TestCreateWorkOrder_NegativeQuantity_IsInvalidInput(t *testing.T) {
	planID := uuid.New()
	svc := newSvc(&mockStore{}, approvedPlan(planID), skuNoMetal(uuid.New()))

	_, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   planID,
		SKUID:    uuid.New(),
		Quantity: -3,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative quantity, got %v", err)
	}
}

func TestCreateWorkOrder_StoreError_Propagates(t *testing.T) {
	planID := uuid.New()
	dbErr := errors.New("insert failed")
	st := &mockStore{insertWorkOrderErr: dbErr}

	svc := newSvc(st, approvedPlan(planID), skuNoMetal(uuid.New()))

	_, err := svc.CreateWorkOrder(context.Background(), CreateWOInput{
		PlanID:   planID,
		SKUID:    uuid.New(),
		Quantity: 2,
	})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
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

	wos, err := svc.ListWorkOrders(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wos) != 2 {
		t.Errorf("len = %d, want 2", len(wos))
	}
}

func TestListWorkOrders_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("query failed")
	st := &mockStore{selectWorkOrdersErr: dbErr}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	_, err := svc.ListWorkOrders(context.Background())
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error, got %v", err)
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
			st := &mockStore{
				selectWorkOrderByIDResult: woWithStatus(woID, tc.from),
				hasMetalConsumptionResult: true, // always satisfy BR-P04 for metal check
			}
			// Use skuNoMetal to avoid BR-P04 interference on most transitions;
			// for WOInProcessing→WOCompleted we use skuNoMetal too so metal check
			// is bypassed (SKU doesn't require it).
			svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(skuID))

			err := svc.AdvanceStatus(context.Background(), woID, tc.to)
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

			err := svc.AdvanceStatus(context.Background(), woID, tc.to)
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

	err := svc.AdvanceStatus(context.Background(), uuid.New(), domain.WOInCutting)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAdvanceStatus_StoreUpdateError_Propagates(t *testing.T) {
	woID := uuid.New()
	dbErr := errors.New("update failed")
	st := &mockStore{
		selectWorkOrderByIDResult: woWithStatus(woID, domain.WOPlanned),
		updateWorkOrderStatusErr:  dbErr,
	}
	svc := newSvc(st, approvedPlan(uuid.New()), skuNoMetal(uuid.New()))

	err := svc.AdvanceStatus(context.Background(), woID, domain.WOInCutting)
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

	err := svc.AdvanceStatus(context.Background(), woID, domain.WOCompleted)
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

	err := svc.AdvanceStatus(context.Background(), woID, domain.WOCompleted)
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

	err := svc.AdvanceStatus(context.Background(), woID, domain.WOCompleted)
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

	err := svc.AdvanceStatus(context.Background(), woID, domain.WOCompleted)
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

	err := svc.AdvanceStatus(context.Background(), woID, domain.WOCompleted)
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
			st := &mockStore{
				selectWorkOrderByIDResult: woWithStatus(woID, tc.from),
				// If hasMetalConsumption were called it would return an error,
				// proving the check was incorrectly invoked.
				hasMetalConsumptionErr: errors.New("metal check must not run here"),
			}
			svc := newSvc(st, approvedPlan(uuid.New()), skuRequiresMetal(skuID))

			err := svc.AdvanceStatus(context.Background(), woID, tc.to)
			if err != nil {
				t.Errorf("unexpected error for %s -> %s: %v", tc.from, tc.to, err)
			}
		})
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
		selectWorkOrderByIDResult: woWithStatus(woID, domain.WOInProcessing),
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
		selectWorkOrderByIDResult:  woWithStatus(woID, domain.WOInProcessing),
		selectConsumptionsByWOErr:  dbErr,
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
		selectWorkOrderByIDResult:   woWithStatus(woID, domain.WOPlanned),
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
