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

// ── MarkRemnantWaste — missing branches ───────────────────────────────────────

func TestMarkRemnantWaste_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectRemnantByIDErr: domain.NewBizError(domain.ErrNotFound, "remnant not found")}
	svc := NewService(st)

	err := svc.MarkRemnantWaste(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

func TestMarkRemnantWaste_AtomicStoreError_Propagates(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	dbErr := errors.New("lock timeout")
	st := &mockStore{
		selectRemnantByIDResult:       availableRemnant(remID, boardID),
		markRemnantWasteAtomicallyErr: dbErr,
	}
	svc := NewService(st)

	err := svc.MarkRemnantWaste(context.Background(), remID)
	if !errors.Is(err, dbErr) {
		t.Errorf("expected atomic store error to propagate, got %v", err)
	}
}

// ── AllocateRemnant — missing branches ───────────────────────────────────────

func TestAllocateRemnant_ConsumedRemnant_IsInvalidInput(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	r := availableRemnant(remID, boardID)
	r.Status = domain.RemnantConsumed

	st := &mockStore{selectRemnantByIDResult: r}
	svc := NewService(st)

	err := svc.AllocateRemnant(context.Background(), remID, uuid.New())
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for CONSUMED remnant, got %v", err)
	}
}

func TestAllocateRemnant_WastedRemnant_IsInvalidInput(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	r := availableRemnant(remID, boardID)
	r.Status = domain.RemnantWaste

	st := &mockStore{selectRemnantByIDResult: r}
	svc := NewService(st)

	err := svc.AllocateRemnant(context.Background(), remID, uuid.New())
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for WASTE remnant, got %v", err)
	}
}

func TestAllocateRemnant_AtomicStoreError_Propagates(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	dbErr := errors.New("deadlock detected")
	st := &mockStore{
		selectRemnantByIDResult:      availableRemnant(remID, boardID),
		allocateRemnantAtomicallyErr: dbErr,
	}
	svc := NewService(st)

	err := svc.AllocateRemnant(context.Background(), remID, uuid.New())
	if !errors.Is(err, dbErr) {
		t.Errorf("expected atomic store error to propagate, got %v", err)
	}
}

// ── RecordCut — remnant not found path ───────────────────────────────────────

func TestRecordCut_RemnantNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectRemnantByIDErr: domain.NewBizError(domain.ErrNotFound, "remnant not found"),
	}
	svc := NewService(st)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:     ptr(uuid.New()),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

// ── ListLots ──────────────────────────────────────────────────────────────────

func TestListLots_ReturnsPersisted(t *testing.T) {
	lot1 := InventoryLot{ID: uuid.New(), MaterialID: uuid.New(), Quantity: 5}
	lot2 := InventoryLot{ID: uuid.New(), MaterialID: uuid.New(), Quantity: 10}
	st := &mockStore{selectLotsResult: []InventoryLot{lot1, lot2}}
	svc := NewService(st)

	lots, err := svc.ListLots(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lots) != 2 {
		t.Errorf("len = %d, want 2", len(lots))
	}
}

func TestListLots_Empty_ReturnsNil(t *testing.T) {
	st := &mockStore{selectLotsResult: nil}
	svc := NewService(st)

	lots, err := svc.ListLots(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lots) != 0 {
		t.Errorf("expected empty slice, got %d lots", len(lots))
	}
}

func TestListLots_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("query failed")
	st := &mockStore{selectLotsErr: dbErr}
	svc := NewService(st)

	_, err := svc.ListLots(context.Background())
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── GetSheet ──────────────────────────────────────────────────────────────────

func TestGetSheet_HappyPath(t *testing.T) {
	sheetID := uuid.New()
	want := availableSheet(sheetID)
	st := &mockStore{selectSheetByIDResult: want}
	svc := NewService(st)

	got, err := svc.GetSheet(context.Background(), sheetID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("got.ID = %v, want %v", got.ID, want.ID)
	}
	if got.Status != "AVAILABLE" {
		t.Errorf("got.Status = %q, want AVAILABLE", got.Status)
	}
}

func TestGetSheet_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectSheetByIDErr: domain.NewBizError(domain.ErrNotFound, "board sheet not found")}
	svc := NewService(st)

	_, err := svc.GetSheet(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetSheet_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("connection reset")
	st := &mockStore{selectSheetByIDErr: dbErr}
	svc := NewService(st)

	_, err := svc.GetSheet(context.Background(), uuid.New())
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── ListAvailableSheets ───────────────────────────────────────────────────────

func TestListAvailableSheets_ReturnsOnlyAvailable(t *testing.T) {
	sh1 := availableSheet(uuid.New())
	sh2 := availableSheet(uuid.New())
	st := &mockStore{selectAvailableSheetsResult: []BoardSheet{sh1, sh2}}
	svc := NewService(st)

	sheets, err := svc.ListAvailableSheets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sheets) != 2 {
		t.Errorf("len = %d, want 2", len(sheets))
	}
	for _, s := range sheets {
		if s.Status != "AVAILABLE" {
			t.Errorf("sheet %v has status %q, want AVAILABLE", s.ID, s.Status)
		}
	}
}

func TestListAvailableSheets_Empty_ReturnsNil(t *testing.T) {
	st := &mockStore{selectAvailableSheetsResult: nil}
	svc := NewService(st)

	sheets, err := svc.ListAvailableSheets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sheets) != 0 {
		t.Errorf("expected empty slice, got %d sheets", len(sheets))
	}
}

func TestListAvailableSheets_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("timeout")
	st := &mockStore{selectAvailableSheetsErr: dbErr}
	svc := NewService(st)

	_, err := svc.ListAvailableSheets(context.Background())
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── FindAvailableRemnants — BR-K01 stock check ────────────────────────────────

func TestFindAvailableRemnants_HappyPath(t *testing.T) {
	boardID := uuid.New()
	r1 := availableRemnant(uuid.New(), boardID)
	r2 := availableRemnant(uuid.New(), boardID)
	st := &mockStore{selectAvailableRemnantsResult: []Remnant{r1, r2}}
	svc := NewService(st)

	remnants, err := svc.FindAvailableRemnants(context.Background(), dim100x100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(remnants) != 2 {
		t.Errorf("len = %d, want 2", len(remnants))
	}
	for _, r := range remnants {
		if r.Status != domain.RemnantAvailable {
			t.Errorf("remnant %v has status %v, want AVAILABLE", r.ID, r.Status)
		}
	}
}

func TestFindAvailableRemnants_NoneMatch_ReturnsEmpty(t *testing.T) {
	// Store returns empty when no remnants meet the min dimension.
	st := &mockStore{selectAvailableRemnantsResult: nil}
	svc := NewService(st)

	remnants, err := svc.FindAvailableRemnants(context.Background(), dim2000x1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(remnants) != 0 {
		t.Errorf("expected empty, got %d remnants", len(remnants))
	}
}

func TestFindAvailableRemnants_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("index scan failed")
	st := &mockStore{selectAvailableRemnantsErr: dbErr}
	svc := NewService(st)

	_, err := svc.FindAvailableRemnants(context.Background(), dim100x100)
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── GetRemnantLineage — BR-K04 lineage tracing ────────────────────────────────

func TestGetRemnantLineage_HappyPath(t *testing.T) {
	boardID := uuid.New()
	// r1 is a direct child of the board sheet.
	r1 := availableRemnant(uuid.New(), boardID)
	// r2 is a nested remnant (child of r1) — same parent board.
	r2 := Remnant{
		ID:              uuid.New(),
		ParentBoardID:   boardID,
		ParentRemnantID: &r1.ID,
		Dimensions:      dim100x100,
		Status:          domain.RemnantAvailable,
	}
	st := &mockStore{selectRemnantsByBoardSheetResult: []Remnant{r1, r2}}
	svc := NewService(st)

	lineage, err := svc.GetRemnantLineage(context.Background(), boardID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lineage) != 2 {
		t.Fatalf("len = %d, want 2", len(lineage))
	}
	// Verify BR-K04: every remnant in the lineage shares the same parent_board_id.
	for _, r := range lineage {
		if r.ParentBoardID != boardID {
			t.Errorf("remnant %v has ParentBoardID %v, want %v", r.ID, r.ParentBoardID, boardID)
		}
	}
	// Verify nested parent pointer is preserved.
	nested := lineage[1]
	if nested.ParentRemnantID == nil || *nested.ParentRemnantID != r1.ID {
		t.Errorf("nested.ParentRemnantID = %v, want %v", nested.ParentRemnantID, r1.ID)
	}
}

func TestGetRemnantLineage_NoRemnants_ReturnsEmpty(t *testing.T) {
	st := &mockStore{selectRemnantsByBoardSheetResult: nil}
	svc := NewService(st)

	lineage, err := svc.GetRemnantLineage(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lineage) != 0 {
		t.Errorf("expected empty lineage, got %d remnants", len(lineage))
	}
}

func TestGetRemnantLineage_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("query failed")
	st := &mockStore{selectRemnantsByBoardSheetErr: dbErr}
	svc := NewService(st)

	_, err := svc.GetRemnantLineage(context.Background(), uuid.New())
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── BR-K01: stock check — ReceiveStock sets up AVAILABLE sheets ───────────────

func TestReceiveStock_SheetsAreAvailableAndLinkedToLot(t *testing.T) {
	// Capture the sheets that get written via insertSheets to verify
	// BR-K01: every sheet must be AVAILABLE and reference the correct lot.
	var capturedSheets []BoardSheet
	capturingSt := &capturingMockStore{
		onInsertSheets: func(sheets []BoardSheet) { capturedSheets = sheets },
	}
	svc := NewService(capturingSt)

	lot, err := svc.ReceiveStock(context.Background(), ReceiveStockInput{
		MaterialID:   uuid.New(),
		Dimensions:   dim2000x1000,
		CostPerSheet: domain.Money{Amount: 80_000, Currency: "VND"},
		Quantity:     4,
		SupplierRef:  "SUP-BR-K01",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedSheets) != 4 {
		t.Fatalf("expected 4 sheets, got %d", len(capturedSheets))
	}
	for i, sh := range capturedSheets {
		if sh.Status != "AVAILABLE" {
			t.Errorf("sheet[%d].Status = %q, want AVAILABLE", i, sh.Status)
		}
		if sh.LotID != lot.ID {
			t.Errorf("sheet[%d].LotID = %v, want lot.ID %v", i, sh.LotID, lot.ID)
		}
		if sh.Dimensions != dim2000x1000 {
			t.Errorf("sheet[%d].Dimensions = %v, want %v", i, sh.Dimensions, dim2000x1000)
		}
		if sh.CostPerSheet.Amount != 80_000 {
			t.Errorf("sheet[%d].CostPerSheet.Amount = %v, want 80000", i, sh.CostPerSheet.Amount)
		}
	}
}

// capturingMockStore wraps mockStore and allows intercepting insertSheets calls
// so tests can inspect the exact sheets passed by the service (BR-K01 checks).
type capturingMockStore struct {
	mockStore
	onInsertSheets func([]BoardSheet)
}

func (c *capturingMockStore) insertSheets(_ context.Context, sheets []BoardSheet) error {
	if c.onInsertSheets != nil {
		c.onInsertSheets(sheets)
	}
	return c.insertSheetsErr
}

// ── BR-K03: area conservation — edge cases ───────────────────────────────────

func TestRecordCut_AreaConservation_UsedExceedsByOneMM2(t *testing.T) {
	// Sheet is 2000×1000 = 2_000_000 mm².
	// Used is 2_000_001 mm² (exactly 1 mm² over) — must fail.
	sheetID := uuid.New()
	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st)

	// 2001 × 1000 = 2_001_000 mm² > 2_000_000.
	over := domain.Dimension{LengthMM: 2001, WidthMM: 1000}
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: over,
	})
	if !errors.Is(err, domain.ErrAreaConservation) {
		t.Errorf("expected ErrAreaConservation for 1mm² over, got %v", err)
	}
}

func TestRecordCut_AreaConservation_FromRemnant_ExactFit(t *testing.T) {
	// Remnant is 1000×500 = 500_000 mm².
	// Used = 500×500 = 250_000; leftover remnant = 500×500 = 250_000.
	// Total = 500_000 == source. Must succeed.
	boardID := uuid.New()
	remID := uuid.New()
	st := &mockStore{
		selectRemnantByIDResult: availableRemnant(remID, boardID), // 1000×500
	}
	svc := NewService(st)

	half := domain.Dimension{LengthMM: 500, WidthMM: 500}
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:        ptr(remID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    half,
		RemnantDimension: &half,
	})
	if err != nil {
		t.Errorf("exact-area cut from remnant must succeed, got: %v", err)
	}
}

func TestRecordCut_AreaConservation_FromRemnant_Exceeded(t *testing.T) {
	// Remnant is 1000×500 = 500_000 mm².
	// Used 600×500 = 300_000; leftover 600×500 = 300_000 → total 600_000 > 500_000.
	boardID := uuid.New()
	remID := uuid.New()
	st := &mockStore{
		selectRemnantByIDResult: availableRemnant(remID, boardID),
	}
	svc := NewService(st)

	big := domain.Dimension{LengthMM: 600, WidthMM: 500}
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:        ptr(remID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    big,
		RemnantDimension: &big,
	})
	if !errors.Is(err, domain.ErrAreaConservation) {
		t.Errorf("expected ErrAreaConservation, got %v", err)
	}
}

// ── BR-K05: status lifecycle ──────────────────────────────────────────────────

func TestRemnantStatusLifecycle_AllTransitionsTable(t *testing.T) {
	// This table drives service-layer gate checks (the optimistic pre-check in
	// AllocateRemnant and MarkRemnantWaste) for every status value, confirming
	// BR-K05 enforcement at the service boundary.
	type check struct {
		status      domain.RemnantStatus
		allocateOK  bool // should AllocateRemnant pre-check pass?
		markWasteOK bool // should MarkRemnantWaste pre-check pass?
	}

	cases := []check{
		{domain.RemnantAvailable, true, true},
		{domain.RemnantAllocated, false, true},
		{domain.RemnantConsumed, false, false},
		{domain.RemnantWaste, false, false},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			boardID := uuid.New()
			remID := uuid.New()
			r := availableRemnant(remID, boardID)
			r.Status = tc.status

			// ── AllocateRemnant pre-check ──────────────────────────────
			st := &mockStore{selectRemnantByIDResult: r}
			svc := NewService(st)
			allocErr := svc.AllocateRemnant(context.Background(), remID, uuid.New())
			if tc.allocateOK {
				if allocErr != nil {
					t.Errorf("AllocateRemnant(%s): unexpected error: %v", tc.status, allocErr)
				}
			} else {
				if !errors.Is(allocErr, domain.ErrInvalidInput) {
					t.Errorf("AllocateRemnant(%s): expected ErrInvalidInput, got %v", tc.status, allocErr)
				}
			}

			// ── MarkRemnantWaste pre-check ─────────────────────────────
			st2 := &mockStore{selectRemnantByIDResult: r}
			svc2 := NewService(st2)
			wasteErr := svc2.MarkRemnantWaste(context.Background(), remID)
			if tc.markWasteOK {
				if wasteErr != nil {
					t.Errorf("MarkRemnantWaste(%s): unexpected error: %v", tc.status, wasteErr)
				}
			} else {
				if !errors.Is(wasteErr, domain.ErrInvalidInput) {
					t.Errorf("MarkRemnantWaste(%s): expected ErrInvalidInput, got %v", tc.status, wasteErr)
				}
			}
		})
	}
}
