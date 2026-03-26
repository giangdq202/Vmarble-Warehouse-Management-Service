package inventory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
)

// ── mockStore ────────────────────────────────────────────────────────────────
// Hand-written mock that satisfies the full store interface.
// Each field controls what the corresponding method returns.
// recordCutAtomicallyCalled / recordCutAtomicallyOp let tests inspect
// exactly what the service passed into the atomic write.

type mockStore struct {
	// insertLot
	insertLotErr error

	// selectLots
	selectLotsResult []InventoryLot
	selectLotsErr    error

	// insertSheets
	insertSheetsErr error

	// selectSheetByID
	selectSheetByIDResult BoardSheet
	selectSheetByIDErr    error

	// selectAvailableSheets
	selectAvailableSheetsResult []BoardSheet
	selectAvailableSheetsErr    error

	// updateSheetStatus
	updateSheetStatusErr error

	// insertCuttingRecord
	insertCuttingRecordErr error

	// insertRemnant
	insertRemnantErr error

	// selectAvailableRemnantsByMinDimension
	selectAvailableRemnantsResult []Remnant
	selectAvailableRemnantsErr    error

	// selectRemnantsByBoardSheet
	selectRemnantsByBoardSheetResult []Remnant
	selectRemnantsByBoardSheetErr    error

	// selectRemnantByID
	selectRemnantByIDResult Remnant
	selectRemnantByIDErr    error

	// updateRemnantStatus
	updateRemnantStatusErr error

	// recordCutAtomically
	recordCutAtomicallyCalled bool
	recordCutAtomicallyOp     cutWriteOp
	recordCutAtomicallyErr    error

	// allocateRemnantAtomically
	allocateRemnantAtomicallyErr error

	// markRemnantWasteAtomically
	markRemnantWasteAtomicallyErr error
}

func (m *mockStore) insertLot(_ context.Context, _ InventoryLot) error {
	return m.insertLotErr
}
func (m *mockStore) selectLots(_ context.Context) ([]InventoryLot, error) {
	return m.selectLotsResult, m.selectLotsErr
}
func (m *mockStore) insertSheets(_ context.Context, _ []BoardSheet) error {
	return m.insertSheetsErr
}
func (m *mockStore) selectSheetByID(_ context.Context, _ uuid.UUID) (BoardSheet, error) {
	return m.selectSheetByIDResult, m.selectSheetByIDErr
}
func (m *mockStore) selectAvailableSheets(_ context.Context) ([]BoardSheet, error) {
	return m.selectAvailableSheetsResult, m.selectAvailableSheetsErr
}
func (m *mockStore) updateSheetStatus(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) error {
	return m.updateSheetStatusErr
}
func (m *mockStore) insertCuttingRecord(_ context.Context, _ CuttingRecord) error {
	return m.insertCuttingRecordErr
}
func (m *mockStore) insertRemnant(_ context.Context, _ Remnant) error {
	return m.insertRemnantErr
}
func (m *mockStore) selectAvailableRemnantsByMinDimension(_ context.Context, _ domain.Dimension) ([]Remnant, error) {
	return m.selectAvailableRemnantsResult, m.selectAvailableRemnantsErr
}
func (m *mockStore) selectRemnantsByBoardSheet(_ context.Context, _ uuid.UUID) ([]Remnant, error) {
	return m.selectRemnantsByBoardSheetResult, m.selectRemnantsByBoardSheetErr
}
func (m *mockStore) selectRemnantByID(_ context.Context, _ uuid.UUID) (Remnant, error) {
	return m.selectRemnantByIDResult, m.selectRemnantByIDErr
}
func (m *mockStore) updateRemnantStatus(_ context.Context, _ uuid.UUID, _ domain.RemnantStatus, _ *uuid.UUID) error {
	return m.updateRemnantStatusErr
}
func (m *mockStore) recordCutAtomically(_ context.Context, op cutWriteOp) error {
	m.recordCutAtomicallyCalled = true
	m.recordCutAtomicallyOp = op
	return m.recordCutAtomicallyErr
}
func (m *mockStore) allocateRemnantAtomically(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return m.allocateRemnantAtomicallyErr
}
func (m *mockStore) markRemnantWasteAtomically(_ context.Context, _ uuid.UUID) error {
	return m.markRemnantWasteAtomicallyErr
}

// ── helpers ──────────────────────────────────────────────────────────────────

func ptr[T any](v T) *T { return &v }

var (
	dim2000x1000 = domain.Dimension{LengthMM: 2000, WidthMM: 1000}
	dim1000x500  = domain.Dimension{LengthMM: 1000, WidthMM: 500}
	dim800x400   = domain.Dimension{LengthMM: 800, WidthMM: 400}
	dim100x100   = domain.Dimension{LengthMM: 100, WidthMM: 100}
	dimZero      = domain.Dimension{LengthMM: 0, WidthMM: 0}
	dimNegative  = domain.Dimension{LengthMM: -1, WidthMM: 500}
)

func availableSheet(id uuid.UUID) BoardSheet {
	return BoardSheet{
		ID:           id,
		LotID:        uuid.New(),
		Dimensions:   dim2000x1000,
		CostPerSheet: domain.Money{Amount: 100_000, Currency: "VND"},
		Status:       "AVAILABLE",
	}
}

func availableRemnant(id uuid.UUID, parentBoard uuid.UUID) Remnant {
	return Remnant{
		ID:            id,
		ParentBoardID: parentBoard,
		Dimensions:    dim1000x500,
		Status:        domain.RemnantAvailable,
		CreatedAt:     time.Now().UTC(),
	}
}

// ── TestRecordCut ─────────────────────────────────────────────────────────────

func TestRecordCut_FromSheet_NoRemnant(t *testing.T) {
	sheetID := uuid.New()
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectSheetByIDResult: availableSheet(sheetID),
	}
	svc := NewService(st)

	in := RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: dim1000x500, // 500_000 mm² ≤ 2_000_000 mm²
	}

	result, err := svc.RecordCut(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CuttingRecordID == uuid.Nil {
		t.Error("CuttingRecordID must be set")
	}
	if result.RemnantID != nil {
		t.Error("RemnantID must be nil when no remnant dimension given")
	}

	// Verify the op sent to the atomic store method.
	if !st.recordCutAtomicallyCalled {
		t.Fatal("recordCutAtomically was not called")
	}
	op := st.recordCutAtomicallyOp
	if op.SheetUpdate == nil {
		t.Fatal("SheetUpdate must be non-nil for a sheet-based cut")
	}
	if op.SheetUpdate.ID != sheetID {
		t.Errorf("SheetUpdate.ID = %v, want %v", op.SheetUpdate.ID, sheetID)
	}
	if op.SheetUpdate.Status != "ISSUED" {
		t.Errorf("SheetUpdate.Status = %q, want %q", op.SheetUpdate.Status, "ISSUED")
	}
	if op.SheetUpdate.IssuedToWO == nil || *op.SheetUpdate.IssuedToWO != woID {
		t.Errorf("SheetUpdate.IssuedToWO = %v, want %v", op.SheetUpdate.IssuedToWO, woID)
	}
	if op.RemnantUpdate != nil {
		t.Error("RemnantUpdate must be nil for a sheet-based cut")
	}
	if op.NewRemnant != nil {
		t.Error("NewRemnant must be nil when no remnant dimension supplied")
	}
}

func TestRecordCut_FromSheet_WithRemnant(t *testing.T) {
	sheetID := uuid.New()
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectSheetByIDResult: availableSheet(sheetID),
	}
	svc := NewService(st)

	remnantDim := dim800x400

	in := RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      woID,
		SKUID:            skuID,
		UsedDimension:    dim1000x500, // 500_000 mm²
		RemnantDimension: &remnantDim, // 320_000 mm²  →  total 820_000 ≤ 2_000_000
	}

	result, err := svc.RecordCut(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RemnantID == nil {
		t.Fatal("RemnantID must be set when remnant dimension is provided")
	}

	op := st.recordCutAtomicallyOp
	if op.NewRemnant == nil {
		t.Fatal("NewRemnant must be non-nil in cutWriteOp")
	}
	if op.NewRemnant.ParentBoardID != sheetID {
		t.Errorf("NewRemnant.ParentBoardID = %v, want sheetID %v", op.NewRemnant.ParentBoardID, sheetID)
	}
	if op.NewRemnant.ParentRemnantID != nil {
		t.Error("NewRemnant.ParentRemnantID must be nil when source is a board sheet")
	}
	if op.NewRemnant.Dimensions != remnantDim {
		t.Errorf("NewRemnant.Dimensions = %v, want %v", op.NewRemnant.Dimensions, remnantDim)
	}
	if op.NewRemnant.Status != domain.RemnantAvailable {
		t.Errorf("NewRemnant.Status = %v, want AVAILABLE", op.NewRemnant.Status)
	}
	// Result ID must match what was built inside the service.
	if *result.RemnantID != op.NewRemnant.ID {
		t.Errorf("result.RemnantID %v ≠ op.NewRemnant.ID %v", *result.RemnantID, op.NewRemnant.ID)
	}
}

func TestRecordCut_FromRemnant_NoRemnant(t *testing.T) {
	boardID := uuid.New()
	remnantID := uuid.New()
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectRemnantByIDResult: availableRemnant(remnantID, boardID),
	}
	svc := NewService(st)

	in := RecordCutInput{
		RemnantID:     ptr(remnantID),
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: dim100x100, // 10_000 mm² ≤ 500_000 mm²
	}

	result, err := svc.RecordCut(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CuttingRecordID == uuid.Nil {
		t.Error("CuttingRecordID must be set")
	}

	op := st.recordCutAtomicallyOp
	if op.RemnantUpdate == nil {
		t.Fatal("RemnantUpdate must be non-nil for a remnant-based cut")
	}
	if op.RemnantUpdate.ID != remnantID {
		t.Errorf("RemnantUpdate.ID = %v, want %v", op.RemnantUpdate.ID, remnantID)
	}
	if op.RemnantUpdate.Status != domain.RemnantConsumed {
		t.Errorf("RemnantUpdate.Status = %v, want CONSUMED", op.RemnantUpdate.Status)
	}
	if op.SheetUpdate != nil {
		t.Error("SheetUpdate must be nil for a remnant-based cut")
	}
}

func TestRecordCut_FromRemnant_WithNestedRemnant(t *testing.T) {
	boardID := uuid.New()
	remnantID := uuid.New()
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectRemnantByIDResult: availableRemnant(remnantID, boardID),
	}
	svc := NewService(st)

	nestedDim := dim100x100

	in := RecordCutInput{
		RemnantID:        ptr(remnantID),
		WorkOrderID:      woID,
		SKUID:            skuID,
		UsedDimension:    dim100x100,
		RemnantDimension: &nestedDim,
	}

	result, err := svc.RecordCut(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RemnantID == nil {
		t.Fatal("RemnantID must be set")
	}

	op := st.recordCutAtomicallyOp
	if op.NewRemnant == nil {
		t.Fatal("NewRemnant must be non-nil")
	}
	// Parent board lineage must bubble up from the source remnant.
	if op.NewRemnant.ParentBoardID != boardID {
		t.Errorf("NewRemnant.ParentBoardID = %v, want %v", op.NewRemnant.ParentBoardID, boardID)
	}
	if op.NewRemnant.ParentRemnantID == nil || *op.NewRemnant.ParentRemnantID != remnantID {
		t.Errorf("NewRemnant.ParentRemnantID = %v, want %v", op.NewRemnant.ParentRemnantID, remnantID)
	}
}

// ── Validation errors ─────────────────────────────────────────────────────────

func TestRecordCut_BothSourcesProvided_IsInvalidInput(t *testing.T) {
	sheetID := uuid.New()
	remID := uuid.New()

	svc := NewService(&mockStore{})
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		RemnantID:     ptr(remID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRecordCut_NoSourceProvided_IsInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRecordCut_ZeroUsedDimension_IsInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(uuid.New()),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dimZero,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRecordCut_NegativeUsedDimension_IsInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(uuid.New()),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dimNegative,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRecordCut_InvalidRemnantDimension_IsInvalidInput(t *testing.T) {
	sheetID := uuid.New()
	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st)
	badDim := dimZero
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim100x100,
		RemnantDimension: &badDim,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// ── BR-K03: area conservation ─────────────────────────────────────────────────

func TestRecordCut_AreaConservation_UsedExceedsSource(t *testing.T) {
	sheetID := uuid.New()
	// Sheet is 2000×1000 = 2_000_000 mm²
	// Used is 2001×1000 = 2_001_000 mm² → violates BR-K03
	overDim := domain.Dimension{LengthMM: 2001, WidthMM: 1000}

	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: overDim,
	})
	if !errors.Is(err, domain.ErrAreaConservation) {
		t.Errorf("expected ErrAreaConservation, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called when area check fails")
	}
}

func TestRecordCut_AreaConservation_UsedPlusRemnantExceedsSource(t *testing.T) {
	sheetID := uuid.New()
	// Sheet area = 2_000_000 mm²
	// Used = 1200×1000 = 1_200_000; Remnant = 1000×1000 = 1_000_000
	// Total 2_200_000 > 2_000_000 → violates BR-K03
	usedDim := domain.Dimension{LengthMM: 1200, WidthMM: 1000}
	remDim := domain.Dimension{LengthMM: 1000, WidthMM: 1000}

	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    usedDim,
		RemnantDimension: &remDim,
	})
	if !errors.Is(err, domain.ErrAreaConservation) {
		t.Errorf("expected ErrAreaConservation, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called when area check fails")
	}
}

func TestRecordCut_AreaConservation_ExactlyEqualSource_IsAllowed(t *testing.T) {
	// used + remnant == source area exactly — must succeed
	sheetID := uuid.New()
	// Sheet area = 2000×1000 = 2_000_000 mm²
	usedDim := domain.Dimension{LengthMM: 1000, WidthMM: 1000} // 1_000_000
	remDim := domain.Dimension{LengthMM: 1000, WidthMM: 1000}  // 1_000_000

	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    usedDim,
		RemnantDimension: &remDim,
	})
	if err != nil {
		t.Errorf("exact-area cut must be allowed, got: %v", err)
	}
}

// ── Source status checks ──────────────────────────────────────────────────────

func TestRecordCut_SheetNotAvailable_IsInvalidInput(t *testing.T) {
	sheetID := uuid.New()
	issuedSheet := availableSheet(sheetID)
	issuedSheet.Status = "ISSUED"

	st := &mockStore{selectSheetByIDResult: issuedSheet}
	svc := NewService(st)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for non-AVAILABLE sheet, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called when sheet is not AVAILABLE")
	}
}

func TestRecordCut_RemnantNotAvailable_IsInvalidInput(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	consumedRemnant := availableRemnant(remID, boardID)
	consumedRemnant.Status = domain.RemnantConsumed

	st := &mockStore{selectRemnantByIDResult: consumedRemnant}
	svc := NewService(st)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:     ptr(remID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for non-AVAILABLE remnant, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called when remnant is not AVAILABLE")
	}
}

func TestRecordCut_SheetNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectSheetByIDErr: domain.NewBizError(domain.ErrNotFound, "board sheet not found"),
	}
	svc := NewService(st)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(uuid.New()),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

// ── Rollback path: store error after validation passes ────────────────────────

func TestRecordCut_AtomicStoreError_PropagatesAndDoesNotReturnResult(t *testing.T) {
	sheetID := uuid.New()
	storeErr := errors.New("DB connection lost")

	st := &mockStore{
		selectSheetByIDResult: availableSheet(sheetID),
		recordCutAtomicallyErr: storeErr,
	}
	svc := NewService(st)

	result, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
	})
	if err == nil {
		t.Fatal("expected error from store, got nil")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("expected wrapped storeErr, got %v", err)
	}
	if result.CuttingRecordID != uuid.Nil {
		t.Error("result must be zero-value on store error")
	}
	// Atomic method was still called — rollback is pgStore's responsibility.
	if !st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must have been called before the error was returned")
	}
}

// ── Remnant dimension does not fit inside source ───────────────────────────────

func TestRecordCut_RemnantDimDoesNotFitInSource_IsInvalidInput(t *testing.T) {
	sheetID := uuid.New()
	// Sheet is 2000×1000. Remnant 2001×100 exceeds sheet length.
	oversizedRemnant := domain.Dimension{LengthMM: 2001, WidthMM: 100}

	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim100x100,
		RemnantDimension: &oversizedRemnant,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput when remnant does not fit, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called when remnant does not fit")
	}
}

// ── ReceiveStock ──────────────────────────────────────────────────────────────

func TestReceiveStock_HappyPath(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st)

	lot, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   uuid.New(),
		Dimensions:   dim2000x1000,
		CostPerSheet: domain.Money{Amount: 50_000, Currency: "VND"},
		Quantity:     3,
		SupplierRef:  "SUP-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lot.ID == uuid.Nil {
		t.Error("lot ID must be set")
	}
	if lot.Quantity != 3 {
		t.Errorf("lot.Quantity = %d, want 3", lot.Quantity)
	}
}

func TestReceiveStock_ZeroQuantity_IsInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   uuid.New(),
		Dimensions:   dim2000x1000,
		CostPerSheet: domain.Money{Amount: 50_000, Currency: "VND"},
		Quantity:     0,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero quantity, got %v", err)
	}
}

func TestReceiveStock_NegativeQuantity_IsInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   uuid.New(),
		Dimensions:   dim2000x1000,
		CostPerSheet: domain.Money{Amount: 50_000, Currency: "VND"},
		Quantity:     -5,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for negative quantity, got %v", err)
	}
}

func TestReceiveStock_InvalidDimensions_IsInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{})
	_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   uuid.New(),
		Dimensions:   dimZero,
		CostPerSheet: domain.Money{Amount: 50_000, Currency: "VND"},
		Quantity:     1,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for zero dimensions, got %v", err)
	}
}

func TestReceiveStock_StoreInsertLotError_Propagates(t *testing.T) {
	dbErr := errors.New("insert failed")
	st := &mockStore{insertLotErr: dbErr}
	svc := NewService(st)

	_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   uuid.New(),
		Dimensions:   dim2000x1000,
		CostPerSheet: domain.Money{Amount: 50_000, Currency: "VND"},
		Quantity:     1,
	})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected insertLotErr to propagate, got %v", err)
	}
}

func TestReceiveStock_StoreInsertSheetsError_Propagates(t *testing.T) {
	dbErr := errors.New("batch insert failed")
	st := &mockStore{insertSheetsErr: dbErr}
	svc := NewService(st)

	_, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   uuid.New(),
		Dimensions:   dim2000x1000,
		CostPerSheet: domain.Money{Amount: 50_000, Currency: "VND"},
		Quantity:     2,
	})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected insertSheetsErr to propagate, got %v", err)
	}
}

// ── AllocateRemnant ───────────────────────────────────────────────────────────

func TestAllocateRemnant_HappyPath(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	woID := uuid.New()

	st := &mockStore{selectRemnantByIDResult: availableRemnant(remID, boardID)}
	svc := NewService(st)

	if err := svc.AllocateRemnant(context.Background(), remID, woID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAllocateRemnant_AlreadyAllocated_IsInvalidInput(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	r := availableRemnant(remID, boardID)
	r.Status = domain.RemnantAllocated

	st := &mockStore{selectRemnantByIDResult: r}
	svc := NewService(st)

	err := svc.AllocateRemnant(context.Background(), remID, uuid.New())
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for already-allocated remnant, got %v", err)
	}
}

func TestAllocateRemnant_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectRemnantByIDErr: domain.NewBizError(domain.ErrNotFound, "remnant not found")}
	svc := NewService(st)

	err := svc.AllocateRemnant(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

// ── MarkRemnantWaste ──────────────────────────────────────────────────────────

func TestMarkRemnantWaste_FromAvailable_Succeeds(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	st := &mockStore{selectRemnantByIDResult: availableRemnant(remID, boardID)}
	svc := NewService(st)

	if err := svc.MarkRemnantWaste(context.Background(), remID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarkRemnantWaste_FromAllocated_Succeeds(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	r := availableRemnant(remID, boardID)
	r.Status = domain.RemnantAllocated

	st := &mockStore{selectRemnantByIDResult: r}
	svc := NewService(st)

	if err := svc.MarkRemnantWaste(context.Background(), remID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarkRemnantWaste_FromConsumed_IsInvalidInput(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	r := availableRemnant(remID, boardID)
	r.Status = domain.RemnantConsumed

	st := &mockStore{selectRemnantByIDResult: r}
	svc := NewService(st)

	err := svc.MarkRemnantWaste(context.Background(), remID)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for CONSUMED remnant, got %v", err)
	}
}

func TestMarkRemnantWaste_FromWaste_IsInvalidInput(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	r := availableRemnant(remID, boardID)
	r.Status = domain.RemnantWaste

	st := &mockStore{selectRemnantByIDResult: r}
	svc := NewService(st)

	err := svc.MarkRemnantWaste(context.Background(), remID)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for WASTE remnant, got %v", err)
	}
}
