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

	// selectCostingRecords
	listResult []CostingRecord
	listErr    error
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

func (m *mockStore) finalizeCostingRecord(_ context.Context, _ uuid.UUID) error {
	m.finalizeCalled = true
	return m.finalizeErr
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

// ── helpers ───────────────────────────────────────────────────────────────────

func completedWO(woID, skuID uuid.UUID) WOInfo {
	return WOInfo{ID: woID, SKUID: skuID, Status: domain.WOCompleted}
}

func newSvc(st *mockStore, wor *mockWOR, cdr *mockCDR, conr *mockCONR) Service {
	return NewService(st, wor, cdr, conr)
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

func TestComputeCost_WONotCompleted_ReturnsPreconditionFailed(t *testing.T) {
	nonCompletedStatuses := []domain.WorkOrderStatus{
		domain.WOPlanned,
		domain.WOInCutting,
		domain.WOInProcessing,
		domain.WOCosted,
	}

	for _, status := range nonCompletedStatuses {
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
				t.Error("store insert must NOT be called when WO is not completed")
			}
		})
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
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
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
	woID := uuid.New()
	skuID := uuid.New()

	st := notFoundStore()
	wor := &mockWOR{result: completedWO(woID, skuID)}
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
	wor := &mockWOR{result: completedWO(woID, skuID)}
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
	wor := &mockWOR{result: completedWO(woID, skuID)}
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
	wor := &mockWOR{result: completedWO(woID, skuID)}
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

	err := svc.FinalizeCost(context.Background(), uuid.New())

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

	err := svc.FinalizeCost(context.Background(), uuid.New())

	if !errors.Is(err, storeErr) {
		t.Errorf("expected finalize error to propagate, got %v", err)
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
