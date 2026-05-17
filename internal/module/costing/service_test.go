package costing

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
	// selectCostingRecordByWO
	selectByWOResult CostingRecord
	selectByWOErr    error

	// insertCostingRecord
	insertCalled bool
	insertErr    error

	// updateCostingRecord
	updateCalled bool
	updateErr    error

	// finalizeCostingRecord
	finalizeCalled bool
	finalizeErr    error

	// hasCostingRecord
	hasRecordResult bool
	hasRecordErr    error

	// selectCostingRecords
	listResult []CostingRecord
	listErr    error

	// insertCostingAdjustment
	insertAdjCalled bool
	insertAdjErr    error

	// selectAdjustmentsByRecord
	selectAdjResult []CostingAdjustment
	selectAdjErr    error

	// selectWasteReport
	wasteReportResult []WasteReportRow
	wasteReportErr    error
	wasteReportFilter WasteReportFilter
}

func (m *mockStore) insertCostingRecord(_ context.Context, _ CostingRecord) error {
	m.insertCalled = true
	return m.insertErr
}

func (m *mockStore) updateCostingRecord(_ context.Context, _ CostingRecord) error {
	m.updateCalled = true
	return m.updateErr
}

func (m *mockStore) selectCostingRecordByWO(_ context.Context, _ uuid.UUID) (CostingRecord, error) {
	return m.selectByWOResult, m.selectByWOErr
}

func (m *mockStore) selectCostingRecordsPaged(_ context.Context, _ httpkit.PageParams, _ *bool) ([]CostingRecord, int, error) {
	return m.listResult, len(m.listResult), m.listErr
}

func (m *mockStore) finalizeCostingRecord(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	m.finalizeCalled = true
	return m.finalizeErr
}

func (m *mockStore) hasCostingRecord(_ context.Context, _ uuid.UUID) (bool, error) {
	return m.hasRecordResult, m.hasRecordErr
}

func (m *mockStore) insertCostingAdjustment(_ context.Context, _ CostingAdjustment) error {
	m.insertAdjCalled = true
	return m.insertAdjErr
}

func (m *mockStore) selectAdjustmentsByRecord(_ context.Context, _ uuid.UUID) ([]CostingAdjustment, error) {
	return m.selectAdjResult, m.selectAdjErr
}

func (m *mockStore) selectWasteReport(_ context.Context, filter WasteReportFilter) ([]WasteReportRow, error) {
	m.wasteReportFilter = filter
	return m.wasteReportResult, m.wasteReportErr
}

// ── mockWOR (WorkOrderReader) ─────────────────────────────────────────────────

type mockWOR struct {
	result WOInfo
	err    error
}

func (m *mockWOR) GetWorkOrder(_ context.Context, _ uuid.UUID) (WOInfo, error) {
	return m.result, m.err
}

// ── mockCDR (CuttingDataReader) ───────────────────────────────────────────────

type mockCDR struct {
	result []CuttingData
	err    error
}

func (m *mockCDR) GetCuttingDataForWO(_ context.Context, _ uuid.UUID) ([]CuttingData, error) {
	return m.result, m.err
}

// ── mockCONR (ConsumptionDataReader) ─────────────────────────────────────────

type mockCONR struct {
	result domain.Money
	err    error
}

func (m *mockCONR) GetConsumptionCostForWO(_ context.Context, _ uuid.UUID) (domain.Money, error) {
	return m.result, m.err
}

// ── mockLBR (LaborDataReader) ────────────────────────────────────────────────

type mockLBR struct {
	result domain.Money
	err    error
}

func (m *mockLBR) GetLaborCostForWO(_ context.Context, _ uuid.UUID) (domain.Money, error) {
	return m.result, m.err
}

// ── mockAudit (AuditLogger) ──────────────────────────────────────────────────

type mockAudit struct {
	called bool
	got    AuditCostingAdjustmentInput
	err    error
}

func (m *mockAudit) LogCostingAdjustment(_ context.Context, in AuditCostingAdjustmentInput) error {
	m.called = true
	m.got = in
	return m.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func completedWO(woID, skuID uuid.UUID) WOInfo {
	return WOInfo{ID: woID, SKUID: skuID, Status: domain.WOCompleted}
}

func plannedWO(woID, skuID uuid.UUID) WOInfo {
	return WOInfo{ID: woID, SKUID: skuID, Status: domain.WOPlanned}
}

func newSvc(st *mockStore, wor *mockWOR, cdr *mockCDR, conr *mockCONR) Service {
	return NewService(st, wor, cdr, conr, nil)
}

// zeroCONR returns a ConsumptionDataReader that always returns zero cost.
func zeroCONR() *mockCONR {
	return &mockCONR{result: domain.Money{Amount: 0, Currency: "VND"}}
}

// notFoundStore returns a store whose selectCostingRecordByWO returns ErrNotFound.
func notFoundStore() *mockStore {
	return &mockStore{
		selectByWOErr: domain.NewBizError(domain.ErrNotFound, "costing record not found"),
	}
}

// ── TestComputeCost: BR-C01 status check ─────────────────────────────────────

func TestComputeCost_InvalidStatus_ReturnsPreconditionFailed(t *testing.T) {
	// Only IN_CUTTING, IN_PROCESSING, and COSTED are invalid for costing.
	// PLANNED → ESTIMATED and COMPLETED → ACTUAL are both allowed.
	invalidStatuses := []domain.WorkOrderStatus{
		domain.WOInCutting,
		domain.WOInProcessing,
		domain.WOCosted,
	}

	for _, status := range invalidStatuses {
		status := status
		t.Run(string(status), func(t *testing.T) {
			st := &mockStore{}
			wor := &mockWOR{result: WOInfo{ID: uuid.New(), SKUID: uuid.New(), Status: status}}
			svc := newSvc(st, wor, &mockCDR{}, zeroCONR())

			_, err := svc.ComputeCost(context.Background(), uuid.New())

			if !errors.Is(err, domain.ErrPreconditionFailed) {
				t.Errorf("status %s: expected ErrPreconditionFailed, got %v", status, err)
			}
			if st.insertCalled {
				t.Error("store insert must NOT be called for invalid status")
			}
		})
	}
}

func TestComputeCost_PlannedWO_CreatesEstimatedRecord(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	st := notFoundStore()
	wor := &mockWOR{result: plannedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CostingType != CostingTypeEstimated {
		t.Errorf("CostingType = %v, want ESTIMATED", got.CostingType)
	}
	if !st.insertCalled {
		t.Error("store insert must be called for new estimated record")
	}
}

func TestComputeCost_CompletedWO_CreatesActualRecord(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	// At least one non-zero cost component is required for ACTUAL costing —
	// otherwise the zero-cost guard rejects the compute.
	cdr := &mockCDR{result: []CuttingData{{
		SheetCost:    domain.Money{Amount: 50_000, Currency: "VND"},
		SheetAreaMM2: 2_000_000,
		UsedAreaMM2:  2_000_000,
	}}}
	svc := newSvc(st, wor, cdr, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.CostingType != CostingTypeActual {
		t.Errorf("CostingType = %v, want ACTUAL", got.CostingType)
	}
}

// ── Zero-cost guard (issue #255) ─────────────────────────────────────────────
//
// For ACTUAL costing on a COMPLETED WO, at least one non-zero cost component
// (material / auxiliary / labor) must exist. Empty ACTUAL records are almost
// always a data entry gap and must be rejected. Estimated runs on PLANNED WOs
// are exempt because upstream data may legitimately still be empty.

func TestComputeCost_CompletedWO_NoCosts_ReturnsPreconditionFailed(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	_, err := svc.ComputeCost(context.Background(), woID)

	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}
	if st.insertCalled || st.updateCalled {
		t.Error("store must not be written to when guard rejects")
	}
}

func TestComputeCost_PlannedWO_NoCosts_AllowsZeroTotal(t *testing.T) {
	// PLANNED costing is estimated and legitimately may start at 0đ.
	woID := uuid.New()
	skuID := uuid.New()
	st := notFoundStore()
	wor := &mockWOR{result: plannedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("PLANNED zero-cost must be allowed, got %v", err)
	}
	if got.TotalCost.Amount != 0 {
		t.Errorf("TotalCost = %d, want 0", got.TotalCost.Amount)
	}
	if !st.insertCalled {
		t.Error("PLANNED zero-cost record must still be persisted")
	}
}

// Labor-only ACTUAL compute must succeed — confirms the guard accepts any
// single non-zero component, not just material.
func TestComputeCost_CompletedWO_LaborOnly_Succeeds(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{}}
	lbr := &mockLBR{result: domain.Money{Amount: 30_000, Currency: "VND"}}
	svc := NewService(st, wor, cdr, zeroCONR(), lbr)

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.LaborCost.Amount != 30_000 {
		t.Errorf("LaborCost.Amount = %d, want 30_000", got.LaborCost.Amount)
	}
	if got.TotalCost.Amount != 30_000 {
		t.Errorf("TotalCost.Amount = %d, want 30_000", got.TotalCost.Amount)
	}
}

// Finalized check must still take precedence over the zero-cost guard —
// BR-C04 should win, not leak the VN precondition message.
func TestComputeCost_AlreadyFinalized_ZeroCost_ReturnsAlreadyFinalized(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectByWOResult: CostingRecord{
			ID:          uuid.New(),
			WorkOrderID: woID,
			SKUID:       skuID,
			Finalized:   true,
		},
	}
	wor := &mockWOR{result: completedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	_, err := svc.ComputeCost(context.Background(), woID)

	if !errors.Is(err, domain.ErrAlreadyFinalized) {
		t.Fatalf("BR-C04 must win over zero-cost guard, got %v", err)
	}
}

func TestComputeCost_WOReaderError_Propagates(t *testing.T) {
	readerErr := errors.New("db connection lost")
	wor := &mockWOR{err: readerErr}
	svc := newSvc(&mockStore{}, wor, &mockCDR{}, zeroCONR())

	_, err := svc.ComputeCost(context.Background(), uuid.New())

	if !errors.Is(err, readerErr) {
		t.Errorf("expected WOR error to propagate, got %v", err)
	}
}

// ── TestComputeCost: BR-C02/BR-C03 material cost allocation ──────────────────

func TestComputeCost_NoCuttingData_ReturnsMaterialCostZero(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	auxCost := domain.Money{Amount: 5_000, Currency: "VND"}

	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{}}
	conr := &mockCONR{result: auxCost}
	svc := newSvc(st, wor, cdr, conr)

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MaterialCost.Amount != 0 {
		t.Errorf("MaterialCost.Amount = %d, want 0", got.MaterialCost.Amount)
	}
	if got.LaborCost.Amount != 0 {
		t.Errorf("LaborCost.Amount = %d, want 0", got.LaborCost.Amount)
	}
	if got.TotalCost.Amount != auxCost.Amount {
		t.Errorf("TotalCost.Amount = %d, want %d (auxiliary only)", got.TotalCost.Amount, auxCost.Amount)
	}
}

func TestComputeCost_SingleSheet_FullArea_AllocatesFullCost(t *testing.T) {
	// SheetCost=80_000, UsedArea=SheetArea → 100% allocated → MaterialCost=80_000
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{
		{
			SheetCost:    domain.Money{Amount: 80_000, Currency: "VND"},
			SheetAreaMM2: 2_000_000,
			UsedAreaMM2:  2_000_000,
		},
	}}
	svc := newSvc(st, wor, cdr, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MaterialCost.Amount != 80_000 {
		t.Errorf("MaterialCost.Amount = %d, want 80_000 (full area)", got.MaterialCost.Amount)
	}
	if got.TotalCost.Amount != 80_000 {
		t.Errorf("TotalCost.Amount = %d, want 80_000", got.TotalCost.Amount)
	}
}

func TestComputeCost_SingleSheet_PartialArea_AllocatesProportionally(t *testing.T) {
	// BR-C03: cost_for_sku = (used_area / sheet_area) * sheet_cost
	// 80_000 * 1_000_000 / 2_000_000 = 40_000
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{
		{
			SheetCost:    domain.Money{Amount: 80_000, Currency: "VND"},
			SheetAreaMM2: 2_000_000,
			UsedAreaMM2:  1_000_000, // 50% of sheet
		},
	}}
	svc := newSvc(st, wor, cdr, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MaterialCost.Amount != 40_000 {
		t.Errorf("MaterialCost.Amount = %d, want 40_000 (50%% of 80_000)", got.MaterialCost.Amount)
	}
}

func TestComputeCost_MultipleSheets_SumsAllocatedCosts(t *testing.T) {
	// Sheet1: 80_000 full area → contributes 80_000
	// Sheet2: 40_000 half area → contributes 20_000
	// Total materialCost = 100_000
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{
		{
			SheetCost:    domain.Money{Amount: 80_000, Currency: "VND"},
			SheetAreaMM2: 2_000_000,
			UsedAreaMM2:  2_000_000,
		},
		{
			SheetCost:    domain.Money{Amount: 40_000, Currency: "VND"},
			SheetAreaMM2: 2_000_000,
			UsedAreaMM2:  1_000_000,
		},
	}}
	svc := newSvc(st, wor, cdr, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MaterialCost.Amount != 100_000 {
		t.Errorf("MaterialCost.Amount = %d, want 100_000 (80_000 + 20_000)", got.MaterialCost.Amount)
	}
}

func TestComputeCost_SheetAreaZero_IsSkipped(t *testing.T) {
	// Entry with SheetAreaMM2=0 must be skipped (not cause panic or wrong cost).
	// Only the valid entry (60_000 full area) contributes to MaterialCost.
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{
		{
			SheetCost:    domain.Money{Amount: 999_999, Currency: "VND"},
			SheetAreaMM2: 0, // must be skipped
			UsedAreaMM2:  500_000,
		},
		{
			SheetCost:    domain.Money{Amount: 60_000, Currency: "VND"},
			SheetAreaMM2: 1_000_000,
			UsedAreaMM2:  1_000_000,
		},
	}}
	svc := newSvc(st, wor, cdr, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MaterialCost.Amount != 60_000 {
		t.Errorf("MaterialCost.Amount = %d, want 60_000 (zero-area entry must be skipped)", got.MaterialCost.Amount)
	}
}

func TestComputeCost_NegativeSheetArea_IsSkipped(t *testing.T) {
	// SheetAreaMM2 < 0 must also be skipped (guard condition is <= 0).
	// Use PLANNED WO so the zero-cost guard (ACTUAL only) does not interfere.
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: plannedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{
		{
			SheetCost:    domain.Money{Amount: 50_000, Currency: "VND"},
			SheetAreaMM2: -1,
			UsedAreaMM2:  500_000,
		},
	}}
	svc := newSvc(st, wor, cdr, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MaterialCost.Amount != 0 {
		t.Errorf("MaterialCost.Amount = %d, want 0 (negative area must be skipped)", got.MaterialCost.Amount)
	}
}

func TestComputeCost_AuxiliaryCostAdded_ToTotalCost(t *testing.T) {
	// materialCost=40_000, auxiliaryCost=10_000 → totalCost=50_000
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{
		{
			SheetCost:    domain.Money{Amount: 80_000, Currency: "VND"},
			SheetAreaMM2: 2_000_000,
			UsedAreaMM2:  1_000_000,
		},
	}}
	conr := &mockCONR{result: domain.Money{Amount: 10_000, Currency: "VND"}}
	svc := newSvc(st, wor, cdr, conr)

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AuxiliaryCost.Amount != 10_000 {
		t.Errorf("AuxiliaryCost.Amount = %d, want 10_000", got.AuxiliaryCost.Amount)
	}
	if got.TotalCost.Amount != 50_000 {
		t.Errorf("TotalCost.Amount = %d, want 50_000 (40_000 + 10_000)", got.TotalCost.Amount)
	}
}

func TestComputeCost_ConsumptionReaderError_Propagates(t *testing.T) {
	readerErr := errors.New("consumption db error")
	wor := &mockWOR{result: completedWO(uuid.New(), uuid.New())}
	cdr := &mockCDR{result: []CuttingData{}}
	conr := &mockCONR{err: readerErr}
	st := &mockStore{}
	svc := newSvc(st, wor, cdr, conr)

	_, err := svc.ComputeCost(context.Background(), uuid.New())

	if !errors.Is(err, readerErr) {
		t.Errorf("expected ConsumptionDataReader error to propagate, got %v", err)
	}
	if st.insertCalled {
		t.Error("store insert must NOT be called when ConsumptionDataReader fails")
	}
}

func TestComputeCost_CuttingDataReaderError_Propagates(t *testing.T) {
	readerErr := errors.New("cutting data db error")
	wor := &mockWOR{result: completedWO(uuid.New(), uuid.New())}
	cdr := &mockCDR{err: readerErr}
	st := &mockStore{}
	svc := newSvc(st, wor, cdr, zeroCONR())

	_, err := svc.ComputeCost(context.Background(), uuid.New())

	if !errors.Is(err, readerErr) {
		t.Errorf("expected CuttingDataReader error to propagate, got %v", err)
	}
	if st.insertCalled {
		t.Error("store insert must NOT be called when CuttingDataReader fails")
	}
}

// ── TestComputeCost: create vs update path ────────────────────────────────────

func TestComputeCost_NoExistingRecord_InsertsNew(t *testing.T) {
	// Uses PLANNED WO so the ACTUAL-only zero-cost guard does not fire on the
	// empty cutting data — this test focuses on the insert path, not costing math.
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: plannedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.insertCalled {
		t.Error("store insert must be called when no existing record")
	}
	if st.updateCalled {
		t.Error("store update must NOT be called for a new record")
	}
	if got.ID == uuid.Nil {
		t.Error("returned record must have a non-nil ID")
	}
	if got.Finalized {
		t.Error("new costing record must not be finalized")
	}
	if got.SKUID != skuID {
		t.Errorf("SKUID = %v, want %v", got.SKUID, skuID)
	}
}

func TestComputeCost_ExistingUnfinalizedRecord_UpdatesInPlace(t *testing.T) {
	// BR-C04: re-computing an unfinalized record must update it, preserving ID & CreatedAt.
	woID := uuid.New()
	skuID := uuid.New()
	existingID := uuid.New()
	existingTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	st := &mockStore{
		selectByWOResult: CostingRecord{
			ID:          existingID,
			WorkOrderID: woID,
			SKUID:       skuID,
			Finalized:   false,
			CreatedAt:   existingTime,
		},
	}
	// PLANNED WO avoids the ACTUAL zero-cost guard — focus of this test is the
	// update path, not the dollar math.
	wor := &mockWOR{result: plannedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	got, err := svc.ComputeCost(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.insertCalled {
		t.Error("store insert must NOT be called when existing record exists")
	}
	if !st.updateCalled {
		t.Error("store update must be called to refresh unfinalized record")
	}
	if got.ID != existingID {
		t.Errorf("ID = %v, want existing ID %v", got.ID, existingID)
	}
	if !got.CreatedAt.Equal(existingTime) {
		t.Errorf("CreatedAt = %v, want original %v (must be preserved)", got.CreatedAt, existingTime)
	}
}

func TestComputeCost_AlreadyFinalized_ReturnsErrAlreadyFinalized(t *testing.T) {
	// BR-C04 core: once finalized, ComputeCost must return ErrAlreadyFinalized.
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectByWOResult: CostingRecord{
			ID:          uuid.New(),
			WorkOrderID: woID,
			SKUID:       skuID,
			Finalized:   true,
		},
	}
	wor := &mockWOR{result: completedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	_, err := svc.ComputeCost(context.Background(), woID)

	if !errors.Is(err, domain.ErrAlreadyFinalized) {
		t.Errorf("expected ErrAlreadyFinalized, got %v", err)
	}
	if st.updateCalled {
		t.Error("store update must NOT be called when record is finalized")
	}
	if st.insertCalled {
		t.Error("store insert must NOT be called when record is finalized")
	}
}

func TestComputeCost_StoreSelectUnexpectedError_Propagates(t *testing.T) {
	// A non-ErrNotFound store error during select must propagate, not trigger insert.
	unexpectedErr := errors.New("connection reset by peer")
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{selectByWOErr: unexpectedErr}
	wor := &mockWOR{result: completedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	_, err := svc.ComputeCost(context.Background(), woID)

	if !errors.Is(err, unexpectedErr) {
		t.Errorf("expected unexpected store error to propagate, got %v", err)
	}
	if st.insertCalled {
		t.Error("store insert must NOT be called on unexpected select error")
	}
}

func TestComputeCost_StoreInsertError_Propagates(t *testing.T) {
	dbErr := errors.New("insert failed")
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	st.insertErr = dbErr
	// PLANNED avoids the ACTUAL zero-cost guard; we only care that insert errors propagate.
	wor := &mockWOR{result: plannedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	_, err := svc.ComputeCost(context.Background(), woID)

	if !errors.Is(err, dbErr) {
		t.Errorf("expected insert error to propagate, got %v", err)
	}
}

func TestComputeCost_StoreUpdateError_Propagates(t *testing.T) {
	dbErr := errors.New("update failed")
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectByWOResult: CostingRecord{ID: uuid.New(), Finalized: false},
		updateErr:        dbErr,
	}
	// PLANNED avoids the ACTUAL zero-cost guard; we only care that update errors propagate.
	wor := &mockWOR{result: plannedWO(woID, skuID)}
	svc := newSvc(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR())

	_, err := svc.ComputeCost(context.Background(), woID)

	if !errors.Is(err, dbErr) {
		t.Errorf("expected update error to propagate, got %v", err)
	}
}

// ── TestFinalizeCost ──────────────────────────────────────────────────────────

func TestFinalizeCost_DelegatesToStore(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	err := svc.FinalizeCost(context.Background(), uuid.New(), uuid.New())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.finalizeCalled {
		t.Error("store finalizeCostingRecord must be called")
	}
}

func TestFinalizeCost_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("finalize failed")
	st := &mockStore{finalizeErr: storeErr}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	err := svc.FinalizeCost(context.Background(), uuid.New(), uuid.New())

	if !errors.Is(err, storeErr) {
		t.Errorf("expected finalize error to propagate, got %v", err)
	}
}

// ── TestCreateAdjustment — BR-C04 ────────────────────────────────────────────

func TestCreateAdjustment_NonFinalizedRecord_ReturnsPreconditionFailed(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: uuid.New(), WorkOrderID: woID, Finalized: false},
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID:   woID,
		Reason:        "price correction",
		DeltaMaterial: domain.Money{Amount: 1000, Currency: "VND"},
		CreatedBy:     uuid.New(),
	})

	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed, got %v", err)
	}
	if st.insertAdjCalled {
		t.Error("insertCostingAdjustment must NOT be called when record is not finalized")
	}
}

func TestCreateAdjustment_EmptyReason_ReturnsInvalidInput(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: uuid.New(), WorkOrderID: woID, Finalized: true},
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID:   woID,
		Reason:        "",
		DeltaMaterial: domain.Money{Amount: 1000, Currency: "VND"},
		CreatedBy:     uuid.New(),
	})

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for empty reason, got %v", err)
	}
	if st.insertAdjCalled {
		t.Error("insertCostingAdjustment must NOT be called with empty reason")
	}
}

func TestCreateAdjustment_AllZeroDeltas_ReturnsInvalidInput(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: uuid.New(), WorkOrderID: woID, Finalized: true},
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID: woID,
		Reason:      "oops",
		CreatedBy:   uuid.New(),
	})

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for all-zero deltas, got %v", err)
	}
	if st.insertAdjCalled {
		t.Error("insertCostingAdjustment must NOT be called when all deltas are zero")
	}
}

func TestCreateAdjustment_HappyPath_ReturnsSavedAdjustment(t *testing.T) {
	woID := uuid.New()
	recordID := uuid.New()
	actorID := uuid.New()
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: recordID, WorkOrderID: woID, Finalized: true},
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID:    woID,
		Reason:         "vendor credit applied",
		DeltaMaterial:  domain.Money{Amount: -5000, Currency: "VND"},
		DeltaAuxiliary: domain.Money{Amount: 0, Currency: "VND"},
		DeltaLabor:     domain.Money{Amount: 0, Currency: "VND"},
		CreatedBy:      actorID,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.insertAdjCalled {
		t.Error("insertCostingAdjustment must be called on happy path")
	}
	if got.ID == uuid.Nil {
		t.Error("returned adjustment must have a non-nil ID")
	}
	if got.CostingRecordID != recordID {
		t.Errorf("CostingRecordID = %v, want %v", got.CostingRecordID, recordID)
	}
	if got.Reason != "vendor credit applied" {
		t.Errorf("Reason = %q, want %q", got.Reason, "vendor credit applied")
	}
	if got.DeltaTotal.Amount != -5000 {
		t.Errorf("DeltaTotal.Amount = %d, want -5000", got.DeltaTotal.Amount)
	}
	if got.CreatedBy != actorID {
		t.Errorf("CreatedBy = %v, want %v", got.CreatedBy, actorID)
	}
}

func TestCreateAdjustment_RecordNotFound_Propagates(t *testing.T) {
	st := &mockStore{selectByWOErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID:   uuid.New(),
		Reason:        "test",
		DeltaMaterial: domain.Money{Amount: 100, Currency: "VND"},
		CreatedBy:     uuid.New(),
	})

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCreateAdjustment_HappyPath_WritesAuditRow(t *testing.T) {
	woID := uuid.New()
	recordID := uuid.New()
	actorID := uuid.New()
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: recordID, WorkOrderID: woID, Finalized: true},
	}
	audit := &mockAudit{}
	svc := NewServiceFull(st, &mockWOR{}, &mockCDR{}, zeroCONR(), &mockLBR{}, nil, audit)

	got, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID:   woID,
		Reason:        "vendor credit applied",
		DeltaMaterial: domain.Money{Amount: -5000, Currency: "VND"},
		DeltaLabor:    domain.Money{Amount: 1000, Currency: "VND"},
		CreatedBy:     actorID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !audit.called {
		t.Fatal("LogCostingAdjustment must be called on happy path")
	}
	if audit.got.AdjustmentID != got.ID {
		t.Errorf("audit.AdjustmentID = %v, want %v", audit.got.AdjustmentID, got.ID)
	}
	if audit.got.CostingRecordID != recordID {
		t.Errorf("audit.CostingRecordID = %v, want %v", audit.got.CostingRecordID, recordID)
	}
	if audit.got.WorkOrderID != woID {
		t.Errorf("audit.WorkOrderID = %v, want %v", audit.got.WorkOrderID, woID)
	}
	if audit.got.ActorID != actorID {
		t.Errorf("audit.ActorID = %v, want %v", audit.got.ActorID, actorID)
	}
	if audit.got.Reason != "vendor credit applied" {
		t.Errorf("audit.Reason = %q, want %q", audit.got.Reason, "vendor credit applied")
	}
	if audit.got.DeltaMaterial.Amount != -5000 {
		t.Errorf("audit.DeltaMaterial = %d, want -5000", audit.got.DeltaMaterial.Amount)
	}
	if audit.got.DeltaLabor.Amount != 1000 {
		t.Errorf("audit.DeltaLabor = %d, want 1000", audit.got.DeltaLabor.Amount)
	}
	if audit.got.DeltaTotal.Amount != -4000 {
		t.Errorf("audit.DeltaTotal = %d, want -4000", audit.got.DeltaTotal.Amount)
	}
}

func TestCreateAdjustment_AuditError_DoesNotFailRequest(t *testing.T) {
	woID := uuid.New()
	recordID := uuid.New()
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: recordID, WorkOrderID: woID, Finalized: true},
	}
	audit := &mockAudit{err: errors.New("audit table down")}
	svc := NewServiceFull(st, &mockWOR{}, &mockCDR{}, zeroCONR(), &mockLBR{}, nil, audit)

	got, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID:   woID,
		Reason:        "still recorded even when audit log down",
		DeltaMaterial: domain.Money{Amount: 100, Currency: "VND"},
		CreatedBy:     uuid.New(),
	})
	if err != nil {
		t.Fatalf("audit failure must not fail the parent request, got: %v", err)
	}
	if got.ID == uuid.Nil {
		t.Error("adjustment must still be returned when audit fails")
	}
	if !st.insertAdjCalled {
		t.Error("insertCostingAdjustment must still be called when audit fails")
	}
	if !audit.called {
		t.Error("LogCostingAdjustment must have been attempted")
	}
}

func TestCreateAdjustment_ValidationFailure_SkipsAuditWrite(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: uuid.New(), WorkOrderID: woID, Finalized: true},
	}
	audit := &mockAudit{}
	svc := NewServiceFull(st, &mockWOR{}, &mockCDR{}, zeroCONR(), &mockLBR{}, nil, audit)

	// Empty reason → ErrInvalidInput before any insert / audit.
	_, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID:   woID,
		Reason:        "",
		DeltaMaterial: domain.Money{Amount: 100, Currency: "VND"},
		CreatedBy:     uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if audit.called {
		t.Error("LogCostingAdjustment must NOT be called when validation fails")
	}
}

func TestCreateAdjustment_NoAuditLogger_StillSucceeds(t *testing.T) {
	woID := uuid.New()
	recordID := uuid.New()
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: recordID, WorkOrderID: woID, Finalized: true},
	}
	// newSvc uses NewService which leaves audit nil — exercises the nil guard.
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID:   woID,
		Reason:        "no audit wired",
		DeltaMaterial: domain.Money{Amount: 100, Currency: "VND"},
		CreatedBy:     uuid.New(),
	})
	if err != nil {
		t.Fatalf("nil audit logger must not break CreateAdjustment, got: %v", err)
	}
	if !st.insertAdjCalled {
		t.Error("insertCostingAdjustment must still be called without audit logger")
	}
}

func TestCreateAdjustment_StoreInsertFails_NoAuditWrite(t *testing.T) {
	woID := uuid.New()
	dbErr := errors.New("insert failed")
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: uuid.New(), WorkOrderID: woID, Finalized: true},
		insertAdjErr:     dbErr,
	}
	audit := &mockAudit{}
	svc := NewServiceFull(st, &mockWOR{}, &mockCDR{}, zeroCONR(), &mockLBR{}, nil, audit)

	_, err := svc.CreateAdjustment(context.Background(), CreateAdjustmentInput{
		WorkOrderID:   woID,
		Reason:        "store insert dies first",
		DeltaMaterial: domain.Money{Amount: 100, Currency: "VND"},
		CreatedBy:     uuid.New(),
	})
	if !errors.Is(err, dbErr) {
		t.Fatalf("expected store error to propagate, got %v", err)
	}
	if audit.called {
		t.Error("LogCostingAdjustment must NOT run if the canonical insert fails")
	}
}

// ── TestListAdjustments ───────────────────────────────────────────────────────

func TestListAdjustments_HappyPath_ReturnsList(t *testing.T) {
	woID := uuid.New()
	recordID := uuid.New()
	adjs := []CostingAdjustment{
		{ID: uuid.New(), CostingRecordID: recordID, Reason: "adj1"},
		{ID: uuid.New(), CostingRecordID: recordID, Reason: "adj2"},
	}
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: recordID, WorkOrderID: woID},
		selectAdjResult:  adjs,
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.ListAdjustments(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestListAdjustments_NoRecord_Propagates(t *testing.T) {
	st := &mockStore{selectByWOErr: domain.NewBizError(domain.ErrNotFound, "not found")}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.ListAdjustments(context.Background(), uuid.New())

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListAdjustments_EmptyList_ReturnsEmptySlice(t *testing.T) {
	woID := uuid.New()
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: uuid.New(), WorkOrderID: woID},
		selectAdjResult:  nil,
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.ListAdjustments(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(got))
	}
}

// ── TestGetCostingRecord ──────────────────────────────────────────────────────

func TestGetCostingRecord_HappyPath(t *testing.T) {
	woID := uuid.New()
	want := CostingRecord{
		ID:          uuid.New(),
		WorkOrderID: woID,
		SKUID:       uuid.New(),
		Finalized:   false,
	}
	st := &mockStore{selectByWOResult: want}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.GetCostingRecord(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %v, want %v", got.ID, want.ID)
	}
	if got.WorkOrderID != want.WorkOrderID {
		t.Errorf("WorkOrderID = %v, want %v", got.WorkOrderID, want.WorkOrderID)
	}
}

func TestGetCostingRecord_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectByWOErr: domain.NewBizError(domain.ErrNotFound, "costing record not found"),
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.GetCostingRecord(context.Background(), uuid.New())

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

// ── TestGetCostingRecordDetail ────────────────────────────────────────────────

func TestGetCostingRecordDetail_NoAdjustments_EffectiveEqualsRecord(t *testing.T) {
	woID := uuid.New()
	record := CostingRecord{
		ID:            uuid.New(),
		WorkOrderID:   woID,
		SKUID:         uuid.New(),
		MaterialCost:  domain.Money{Amount: 80_000, Currency: "VND"},
		AuxiliaryCost: domain.Money{Amount: 10_000, Currency: "VND"},
		LaborCost:     domain.Money{Amount: 20_000, Currency: "VND"},
		TotalCost:     domain.Money{Amount: 110_000, Currency: "VND"},
		Finalized:     true,
	}
	st := &mockStore{
		selectByWOResult: record,
		selectAdjResult:  nil,
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.GetCostingRecordDetail(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Record.ID != record.ID {
		t.Errorf("Record.ID = %v, want %v", got.Record.ID, record.ID)
	}
	if got.Adjustments == nil {
		t.Error("Adjustments must be a non-nil slice when there are zero adjustments")
	}
	if len(got.Adjustments) != 0 {
		t.Errorf("len(Adjustments) = %d, want 0", len(got.Adjustments))
	}
	if got.EffectiveMaterial.Amount != 80_000 {
		t.Errorf("EffectiveMaterial = %d, want 80_000", got.EffectiveMaterial.Amount)
	}
	if got.EffectiveAuxiliary.Amount != 10_000 {
		t.Errorf("EffectiveAuxiliary = %d, want 10_000", got.EffectiveAuxiliary.Amount)
	}
	if got.EffectiveLabor.Amount != 20_000 {
		t.Errorf("EffectiveLabor = %d, want 20_000", got.EffectiveLabor.Amount)
	}
	if got.EffectiveTotal.Amount != 110_000 {
		t.Errorf("EffectiveTotal = %d, want 110_000", got.EffectiveTotal.Amount)
	}
}

func TestGetCostingRecordDetail_WithAdjustments_FoldsDeltas(t *testing.T) {
	// record:        material=100_000, aux=20_000, labor=50_000, total=170_000
	// adjustment 1:  +5_000 material,  -2_000 aux,  0 labor   → delta_total = +3_000
	// adjustment 2:  0 material,        0 aux,     +10_000 labor → delta_total = +10_000
	// effective:     material=105_000, aux=18_000, labor=60_000, total=183_000
	woID := uuid.New()
	recordID := uuid.New()
	record := CostingRecord{
		ID:            recordID,
		WorkOrderID:   woID,
		SKUID:         uuid.New(),
		MaterialCost:  domain.Money{Amount: 100_000, Currency: "VND"},
		AuxiliaryCost: domain.Money{Amount: 20_000, Currency: "VND"},
		LaborCost:     domain.Money{Amount: 50_000, Currency: "VND"},
		TotalCost:     domain.Money{Amount: 170_000, Currency: "VND"},
		Finalized:     true,
	}
	adjs := []CostingAdjustment{
		{
			ID:              uuid.New(),
			CostingRecordID: recordID,
			Reason:          "vendor invoice correction",
			DeltaMaterial:   domain.Money{Amount: 5_000, Currency: "VND"},
			DeltaAuxiliary:  domain.Money{Amount: -2_000, Currency: "VND"},
			DeltaLabor:      domain.Money{Amount: 0, Currency: "VND"},
			DeltaTotal:      domain.Money{Amount: 3_000, Currency: "VND"},
		},
		{
			ID:              uuid.New(),
			CostingRecordID: recordID,
			Reason:          "overtime backpay",
			DeltaMaterial:   domain.Money{Amount: 0, Currency: "VND"},
			DeltaAuxiliary:  domain.Money{Amount: 0, Currency: "VND"},
			DeltaLabor:      domain.Money{Amount: 10_000, Currency: "VND"},
			DeltaTotal:      domain.Money{Amount: 10_000, Currency: "VND"},
		},
	}
	st := &mockStore{
		selectByWOResult: record,
		selectAdjResult:  adjs,
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.GetCostingRecordDetail(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Adjustments) != 2 {
		t.Fatalf("len(Adjustments) = %d, want 2", len(got.Adjustments))
	}
	if got.EffectiveMaterial.Amount != 105_000 {
		t.Errorf("EffectiveMaterial = %d, want 105_000", got.EffectiveMaterial.Amount)
	}
	if got.EffectiveAuxiliary.Amount != 18_000 {
		t.Errorf("EffectiveAuxiliary = %d, want 18_000", got.EffectiveAuxiliary.Amount)
	}
	if got.EffectiveLabor.Amount != 60_000 {
		t.Errorf("EffectiveLabor = %d, want 60_000", got.EffectiveLabor.Amount)
	}
	if got.EffectiveTotal.Amount != 183_000 {
		t.Errorf("EffectiveTotal = %d, want 183_000", got.EffectiveTotal.Amount)
	}
	// Original record numbers must remain immutable (BR-C04).
	if got.Record.MaterialCost.Amount != 100_000 {
		t.Errorf("Record.MaterialCost mutated to %d, must stay 100_000 (BR-C04)", got.Record.MaterialCost.Amount)
	}
	if got.Record.TotalCost.Amount != 170_000 {
		t.Errorf("Record.TotalCost mutated to %d, must stay 170_000 (BR-C04)", got.Record.TotalCost.Amount)
	}
}

func TestGetCostingRecordDetail_RecordNotFound_Propagates(t *testing.T) {
	st := &mockStore{
		selectByWOErr: domain.NewBizError(domain.ErrNotFound, "costing record not found"),
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.GetCostingRecordDetail(context.Background(), uuid.New())

	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

func TestGetCostingRecordDetail_AdjustmentsStoreError_Propagates(t *testing.T) {
	storeErr := errors.New("adjustments query failed")
	st := &mockStore{
		selectByWOResult: CostingRecord{ID: uuid.New(), WorkOrderID: uuid.New(), Finalized: true},
		selectAdjErr:     storeErr,
	}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.GetCostingRecordDetail(context.Background(), uuid.New())

	if !errors.Is(err, storeErr) {
		t.Errorf("expected adjustments error to propagate, got %v", err)
	}
}

// ── TestListCostingRecords ────────────────────────────────────────────────────

func TestListCostingRecords_ReturnsAll(t *testing.T) {
	records := []CostingRecord{
		{ID: uuid.New(), WorkOrderID: uuid.New()},
		{ID: uuid.New(), WorkOrderID: uuid.New()},
	}
	st := &mockStore{listResult: records}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.ListCostingRecords(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != len(records) {
		t.Errorf("len = %d, want %d", len(got.Items), len(records))
	}
}

func TestListCostingRecords_Empty_ReturnsNil(t *testing.T) {
	st := &mockStore{listResult: nil}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.ListCostingRecords(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(got.Items))
	}
}

func TestListCostingRecords_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("query failed")
	st := &mockStore{listErr: storeErr}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.ListCostingRecords(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, nil)

	if !errors.Is(err, storeErr) {
		t.Errorf("expected list error to propagate, got %v", err)
	}
}

// ── TestListWasteReport — BR-C03 waste-cost ledger ───────────────────────────

func TestListWasteReport_HappyPath_TwoMaterialsThreeCuts(t *testing.T) {
	// Models the issue's DoD scenario: two materials, three cuts
	// ── Material A: two cuts on a single sheet
	//      sheet area=2_000_000, cost=80_000 → cost-per-mm² = 0.04
	//      cut1: used=900_000, no remnant → waste = 1_100_000 → cost = 44_000
	//      Wait — but second cut wouldn't happen if sheet is consumed in first cut.
	//      Realistic: cut1: used=900_000, new_remnant=1_000_000 → waste = 100_000 → cost = 4_000
	//                 cut2 (from remnant 1_000_000): used=600_000, no new remnant → waste = 400_000 → cost = 16_000
	//      Total A: waste_area = 500_000, waste_cost = 20_000, sheets = 1
	// ── Material B: one cut on its sheet
	//      sheet area=1_000_000, cost=60_000 → cost-per-mm² = 0.06
	//      cut3: used=700_000, no remnant → waste = 300_000 → cost = 18_000
	//      Total B: waste_area = 300_000, waste_cost = 18_000, sheets = 1
	matA := uuid.New()
	matB := uuid.New()

	expected := []WasteReportRow{
		{
			MaterialID:     matA,
			MaterialName:   "Mat A",
			SheetsConsumed: 1,
			WasteAreaMM2:   500_000,
			AvgSheetCost:   domain.Money{Amount: 80_000, Currency: "VND"},
			TotalWasteCost: domain.Money{Amount: 20_000, Currency: "VND"},
		},
		{
			MaterialID:     matB,
			MaterialName:   "Mat B",
			SheetsConsumed: 1,
			WasteAreaMM2:   300_000,
			AvgSheetCost:   domain.Money{Amount: 60_000, Currency: "VND"},
			TotalWasteCost: domain.Money{Amount: 18_000, Currency: "VND"},
		},
	}

	st := &mockStore{wasteReportResult: expected}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.ListWasteReport(context.Background(), WasteReportFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].MaterialID != matA || got[0].WasteAreaMM2 != 500_000 || got[0].TotalWasteCost.Amount != 20_000 {
		t.Errorf("row 0 = %+v, want material A with waste_area=500_000 cost=20_000", got[0])
	}
	if got[1].MaterialID != matB || got[1].WasteAreaMM2 != 300_000 || got[1].TotalWasteCost.Amount != 18_000 {
		t.Errorf("row 1 = %+v, want material B with waste_area=300_000 cost=18_000", got[1])
	}
}

func TestListWasteReport_FromAfterTo_ReturnsInvalidInput(t *testing.T) {
	st := &mockStore{}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	from := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	_, err := svc.ListWasteReport(context.Background(), WasteReportFilter{From: &from, To: &to})

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
	if st.wasteReportFilter.From != nil {
		t.Error("store must NOT be called when validation fails")
	}
}

func TestListWasteReport_PassesFilterToStore(t *testing.T) {
	st := &mockStore{wasteReportResult: []WasteReportRow{}}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	matID := uuid.New()
	filter := WasteReportFilter{From: &from, To: &to, MaterialID: &matID}

	_, err := svc.ListWasteReport(context.Background(), filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.wasteReportFilter.From == nil || !st.wasteReportFilter.From.Equal(from) {
		t.Errorf("From not propagated to store: got %v, want %v", st.wasteReportFilter.From, from)
	}
	if st.wasteReportFilter.To == nil || !st.wasteReportFilter.To.Equal(to) {
		t.Errorf("To not propagated to store: got %v, want %v", st.wasteReportFilter.To, to)
	}
	if st.wasteReportFilter.MaterialID == nil || *st.wasteReportFilter.MaterialID != matID {
		t.Errorf("MaterialID not propagated to store: got %v, want %v", st.wasteReportFilter.MaterialID, matID)
	}
}

func TestListWasteReport_NilStoreResult_ReturnsEmptySlice(t *testing.T) {
	st := &mockStore{wasteReportResult: nil}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	got, err := svc.ListWasteReport(context.Background(), WasteReportFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestListWasteReport_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("aggregation failed")
	st := &mockStore{wasteReportErr: storeErr}
	svc := newSvc(st, &mockWOR{}, &mockCDR{}, zeroCONR())

	_, err := svc.ListWasteReport(context.Background(), WasteReportFilter{})

	if !errors.Is(err, storeErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── COSTING_COMPUTED notifier (#258) ─────────────────────────────────────────

type mockCostingNotifier struct {
	calls []struct {
		WO   string
		Type string
	}
	err error
}

func (m *mockCostingNotifier) NotifyCostingComputed(_ context.Context, woID, costingType string) error {
	m.calls = append(m.calls, struct {
		WO   string
		Type string
	}{woID, costingType})
	return m.err
}

func TestComputeCost_HappyPath_PublishesCostingComputed(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{{
		SheetCost:    domain.Money{Amount: 50_000, Currency: "VND"},
		SheetAreaMM2: 2_000_000,
		UsedAreaMM2:  2_000_000,
	}}}
	notifier := &mockCostingNotifier{}
	svc := NewServiceWithNotifier(st, wor, cdr, zeroCONR(), nil, notifier)

	if _, err := svc.ComputeCost(context.Background(), woID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("notifier calls = %d, want 1", len(notifier.calls))
	}
	if notifier.calls[0].WO != woID.String() {
		t.Errorf("WO = %q, want %q", notifier.calls[0].WO, woID.String())
	}
	if notifier.calls[0].Type != string(CostingTypeActual) {
		t.Errorf("Type = %q, want %q", notifier.calls[0].Type, CostingTypeActual)
	}
}

func TestComputeCost_NotifierError_DoesNotFailRequest(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	cdr := &mockCDR{result: []CuttingData{{
		SheetCost:    domain.Money{Amount: 50_000, Currency: "VND"},
		SheetAreaMM2: 2_000_000,
		UsedAreaMM2:  2_000_000,
	}}}
	notifier := &mockCostingNotifier{err: errors.New("broker down")}
	svc := NewServiceWithNotifier(st, wor, cdr, zeroCONR(), nil, notifier)

	if _, err := svc.ComputeCost(context.Background(), woID); err != nil {
		t.Errorf("notifier failure must not fail ComputeCost, got: %v", err)
	}
	if !st.insertCalled {
		t.Error("record must still commit when notifier fails")
	}
}

func TestComputeCost_GuardRejects_NotifierNotCalled(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
	notifier := &mockCostingNotifier{}
	svc := NewServiceWithNotifier(st, wor, &mockCDR{result: []CuttingData{}}, zeroCONR(), nil, notifier)

	if _, err := svc.ComputeCost(context.Background(), woID); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Errorf("notifier must not fire when guard rejects, got %v", notifier.calls)
	}
}
