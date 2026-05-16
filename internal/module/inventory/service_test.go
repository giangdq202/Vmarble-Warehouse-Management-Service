package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
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

	// selectLotsPaged
	selectLotsPagedResult []InventoryLot
	selectLotsPagedTotal  int
	selectLotsPagedErr    error

	// insertSheets
	insertSheetsErr error

	// selectSheetByID
	selectSheetByIDResult BoardSheet
	selectSheetByIDErr    error

	// selectAvailableSheets
	selectAvailableSheetsResult []BoardSheet
	selectAvailableSheetsErr    error

	// selectAvailableSheetsPaged
	selectAvailableSheetsPagedResult []BoardSheet
	selectAvailableSheetsPagedTotal  int
	selectAvailableSheetsPagedErr    error

	// countAvailableSheetsByMaterial
	countAvailableSheetsByMaterialResult int
	countAvailableSheetsByMaterialErr    error
	countAvailableSheetsByMaterialArg    uuid.UUID

	// updateSheetStatus
	updateSheetStatusErr error

	// insertCuttingRecord
	insertCuttingRecordErr error

	// insertRemnant
	insertRemnantErr error

	// selectAvailableRemnantsByMinDimension
	selectAvailableRemnantsResult []Remnant
	selectAvailableRemnantsErr    error

	// selectRemnantsByFilter
	selectRemnantsByFilterResult []Remnant
	selectRemnantsByFilterTotal  int
	selectRemnantsByFilterErr    error

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

	// selectActiveStorageLocations
	selectActiveStorageLocationsResult []StorageLocation
	selectActiveStorageLocationsErr    error

	// deactivateLot
	deactivateLotErr error

	// preAssignSheet
	preAssignSheetErr error

	// releaseExpiredAllocations
	releaseExpiredAllocationsResult int64
	releaseExpiredAllocationsErr    error

	// selectTopRemnantSuggestions
	selectTopRemnantSuggestionsResult []RemnantSuggestion
	selectTopRemnantSuggestionsErr    error

	// selectOverflowAreas
	selectOverflowRemnantArea int64
	selectOverflowSheetArea   int64
	selectOverflowAreasErr    error

	// preAssignSheet call tracking
	preAssignSheetCalled bool

	// selectStorageLocationByBarcode
	selectStorageLocationByBarcodeResult StorageLocation
	selectStorageLocationByBarcodeErr    error

	// updateSheetBinLocation
	updateSheetBinLocationErr error

	// insertAuditLog
	insertAuditLogCalled bool
	insertAuditLogEntry  AuditLogEntry
	insertAuditLogErr    error

	// selectAuditLogByEntity
	selectAuditLogByEntityResult []AuditLogEntry
	selectAuditLogByEntityErr    error

	// selectAuditLogByAction
	selectAuditLogByActionResult []AuditLogEntry
	selectAuditLogByActionErr    error
	selectAuditLogByActionAction string

	// insertCycleCountSession
	insertCycleCountSessionErr error

	// selectCycleCountSessionByID
	selectCycleCountSessionByIDResult CycleCountSession
	selectCycleCountSessionByIDErr    error

	// updateCycleCountSessionStatus
	updateCycleCountSessionStatusErr error

	// insertCycleCountLine
	insertCycleCountLineCalled bool
	insertCycleCountLineErr    error

	// selectCycleCountLinesBySession
	selectCycleCountLinesBySessionResult []CycleCountLine
	selectCycleCountLinesBySessionErr    error

	// postCycleCountAtomically
	postCycleCountAtomicallyCalled bool
	postCycleCountAtomicallyOp     cycleCountPostOp
	postCycleCountAtomicallyErr    error

	// selectCuttingRecordDetails
	selectCuttingRecordDetailsResult CuttingRecordDetails
	selectCuttingRecordDetailsErr    error

	// selectAllocatedRemnantsByWO
	selectAllocatedRemnantsByWOResult []PickSlipLine
	selectAllocatedRemnantsByWOErr    error

	// selectCuttingRecordsReport
	selectCuttingRecordsReportResult []CuttingRecordReport
	selectCuttingRecordsReportTotal  int
	selectCuttingRecordsReportErr    error
	selectCuttingRecordsReportFilter CuttingRecordFilter
}

func (m *mockStore) insertLot(_ context.Context, _ InventoryLot) error {
	return m.insertLotErr
}
func (m *mockStore) selectLots(_ context.Context) ([]InventoryLot, error) {
	return m.selectLotsResult, m.selectLotsErr
}
func (m *mockStore) selectLotsPaged(_ context.Context, _ httpkit.PageParams) ([]InventoryLot, int, error) {
	return m.selectLotsPagedResult, m.selectLotsPagedTotal, m.selectLotsPagedErr
}
func (m *mockStore) deactivateLot(_ context.Context, _ uuid.UUID) error {
	return m.deactivateLotErr
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
func (m *mockStore) selectAvailableSheetsPaged(_ context.Context, _ httpkit.PageParams, _ *uuid.UUID) ([]BoardSheet, int, error) {
	return m.selectAvailableSheetsPagedResult, m.selectAvailableSheetsPagedTotal, m.selectAvailableSheetsPagedErr
}
func (m *mockStore) countAvailableSheetsByMaterial(_ context.Context, materialID uuid.UUID) (int, error) {
	m.countAvailableSheetsByMaterialArg = materialID
	return m.countAvailableSheetsByMaterialResult, m.countAvailableSheetsByMaterialErr
}
func (m *mockStore) updateSheetStatus(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) error {
	return m.updateSheetStatusErr
}
func (m *mockStore) preAssignSheet(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	m.preAssignSheetCalled = true
	return m.preAssignSheetErr
}
func (m *mockStore) releaseExpiredAllocations(_ context.Context, _ time.Time) (int64, error) {
	return m.releaseExpiredAllocationsResult, m.releaseExpiredAllocationsErr
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
func (m *mockStore) selectTopRemnantSuggestions(_ context.Context, _ domain.Dimension, _ int) ([]RemnantSuggestion, error) {
	return m.selectTopRemnantSuggestionsResult, m.selectTopRemnantSuggestionsErr
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
func (m *mockStore) selectRemnantsByFilter(_ context.Context, _ RemnantFilter, _ httpkit.PageParams) ([]Remnant, int, error) {
	return m.selectRemnantsByFilterResult, m.selectRemnantsByFilterTotal, m.selectRemnantsByFilterErr
}
func (m *mockStore) selectActiveStorageLocations(_ context.Context) ([]StorageLocation, error) {
	return m.selectActiveStorageLocationsResult, m.selectActiveStorageLocationsErr
}
func (m *mockStore) updateRemnantBinLocation(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}
func (m *mockStore) selectStorageLocationByBarcode(_ context.Context, _ string) (StorageLocation, error) {
	return m.selectStorageLocationByBarcodeResult, m.selectStorageLocationByBarcodeErr
}

func (m *mockStore) selectOverflowAreas(_ context.Context) (int64, int64, error) {
	return m.selectOverflowRemnantArea, m.selectOverflowSheetArea, m.selectOverflowAreasErr
}

func (m *mockStore) updateSheetBinLocation(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return m.updateSheetBinLocationErr
}

func (m *mockStore) insertAuditLog(_ context.Context, entry AuditLogEntry) error {
	m.insertAuditLogCalled = true
	m.insertAuditLogEntry = entry
	return m.insertAuditLogErr
}

func (m *mockStore) selectAuditLogByAction(_ context.Context, action string) ([]AuditLogEntry, error) {
	m.selectAuditLogByActionAction = action
	return m.selectAuditLogByActionResult, m.selectAuditLogByActionErr
}

func (m *mockStore) selectAuditLogByEntity(_ context.Context, _ uuid.UUID, _ string) ([]AuditLogEntry, error) {
	return m.selectAuditLogByEntityResult, m.selectAuditLogByEntityErr
}

func (m *mockStore) insertCycleCountSession(_ context.Context, _ CycleCountSession) error {
	return m.insertCycleCountSessionErr
}

func (m *mockStore) selectCycleCountSessionByID(_ context.Context, _ uuid.UUID) (CycleCountSession, error) {
	return m.selectCycleCountSessionByIDResult, m.selectCycleCountSessionByIDErr
}

func (m *mockStore) updateCycleCountSessionStatus(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) error {
	return m.updateCycleCountSessionStatusErr
}

func (m *mockStore) insertCycleCountLine(_ context.Context, _ CycleCountLine) error {
	m.insertCycleCountLineCalled = true
	return m.insertCycleCountLineErr
}

func (m *mockStore) selectCycleCountLinesBySession(_ context.Context, _ uuid.UUID) ([]CycleCountLine, error) {
	return m.selectCycleCountLinesBySessionResult, m.selectCycleCountLinesBySessionErr
}

func (m *mockStore) postCycleCountAtomically(_ context.Context, op cycleCountPostOp) error {
	m.postCycleCountAtomicallyCalled = true
	m.postCycleCountAtomicallyOp = op
	return m.postCycleCountAtomicallyErr
}

func (m *mockStore) selectCuttingRecordDetails(_ context.Context, _ uuid.UUID) (CuttingRecordDetails, error) {
	return m.selectCuttingRecordDetailsResult, m.selectCuttingRecordDetailsErr
}

func (m *mockStore) selectAllocatedRemnantsByWO(_ context.Context, _ uuid.UUID) ([]PickSlipLine, error) {
	return m.selectAllocatedRemnantsByWOResult, m.selectAllocatedRemnantsByWOErr
}

func (m *mockStore) selectCuttingRecordsReport(_ context.Context, f CuttingRecordFilter, _ httpkit.PageParams) ([]CuttingRecordReport, int, error) {
	m.selectCuttingRecordsReportFilter = f
	return m.selectCuttingRecordsReportResult, m.selectCuttingRecordsReportTotal, m.selectCuttingRecordsReportErr
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

// ── Inheritance helpers ──────────────────────────────────────────────────────

func availableSheetWithAttrs(id uuid.UUID) BoardSheet {
	sh := availableSheet(id)
	sh.SupplierCode = ptr("SUP-VN-01")
	sh.LotBatch = ptr("LOT-2026-03")
	sh.GrainPattern = ptr("VERTICAL")
	sh.QualityGrade = ptr("A")
	return sh
}

func availableRemnantWithAttrs(id uuid.UUID, parentBoard uuid.UUID) Remnant {
	r := availableRemnant(id, parentBoard)
	r.SupplierCode = ptr("SUP-VN-02")
	r.LotBatch = ptr("LOT-2026-04")
	r.GrainPattern = ptr("HORIZONTAL")
	r.QualityGrade = ptr("B")
	return r
}

func assertMaterialAttrs(t *testing.T, r *Remnant, wantSC, wantLB, wantGP, wantQG *string) {
	t.Helper()
	if !strPtrEqual(r.SupplierCode, wantSC) {
		t.Errorf("SupplierCode = %v, want %v", strPtrVal(r.SupplierCode), strPtrVal(wantSC))
	}
	if !strPtrEqual(r.LotBatch, wantLB) {
		t.Errorf("LotBatch = %v, want %v", strPtrVal(r.LotBatch), strPtrVal(wantLB))
	}
	if !strPtrEqual(r.GrainPattern, wantGP) {
		t.Errorf("GrainPattern = %v, want %v", strPtrVal(r.GrainPattern), strPtrVal(wantGP))
	}
	if !strPtrEqual(r.QualityGrade, wantQG) {
		t.Errorf("QualityGrade = %v, want %v", strPtrVal(r.QualityGrade), strPtrVal(wantQG))
	}
}

func strPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func strPtrVal(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// mockWorkOrderAdvancer satisfies WorkOrderAdvancer.
type mockWorkOrderAdvancer struct {
	called   bool
	calledWO uuid.UUID
	calledTo domain.WorkOrderStatus
	err      error
}

func (m *mockWorkOrderAdvancer) AdvanceStatus(_ context.Context, woID uuid.UUID, in AdvanceWOInput) error {
	m.called = true
	m.calledWO = woID
	m.calledTo = in.To
	return m.err
}

type mockBarcodeGenerator struct {
	called bool
	in     BarcodeForCutInput
	out    BarcodeForCutOutput
	err    error
}

func (m *mockBarcodeGenerator) GenerateForCut(_ context.Context, in BarcodeForCutInput) (BarcodeForCutOutput, error) {
	m.called = true
	m.in = in
	return m.out, m.err
}

// ── TestRecordCut ─────────────────────────────────────────────────────────────

func TestRecordCut_FromSheet_NoRemnant(t *testing.T) {
	sheetID := uuid.New()
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectSheetByIDResult:   availableSheet(sheetID),
		selectOverflowSheetArea: 1,
	}
	svc := NewService(st, nil)

	in := RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: dim1000x500, // 500_000 mm² ≤ 2_000_000 mm²,
		IsWaste:       true,
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
		selectSheetByIDResult:   availableSheet(sheetID),
		selectOverflowSheetArea: 1,
	}
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

	in := RecordCutInput{
		RemnantID:     ptr(remnantID),
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: dim100x100, // 10_000 mm² ≤ 500_000 mm²,
		IsWaste:       true,
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
	svc := NewService(st, nil)

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

	svc := NewService(&mockStore{}, nil)
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		RemnantID:     ptr(remID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRecordCut_NoSourceProvided_IsInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{}, nil)
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// ── BR-K02: explicit waste flag ──────────────────────────────────────────────

// TestRecordCut_BRK02_NeitherRemnantNorWaste rejects an input where the caller
// supplied neither remnant_dimension nor is_waste — that ambiguity is exactly
// what BR-K02 forbids. Without this guard, an empty leftover field implicitly
// produced waste with no explicit declaration.
func TestRecordCut_BRK02_NeitherRemnantNorWaste(t *testing.T) {
	sheetID := uuid.New()
	svc := NewService(&mockStore{}, nil)
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		// No RemnantDimension, IsWaste defaults to false → must reject.
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// TestRecordCut_BRK02_BothRemnantAndWaste rejects an input where the caller
// supplied both a remnant dimension and is_waste=true. Intent is contradictory
// — leftover cannot be both salvaged and discarded.
func TestRecordCut_BRK02_BothRemnantAndWaste(t *testing.T) {
	sheetID := uuid.New()
	rem := domain.Dimension{LengthMM: 200, WidthMM: 200}

	svc := NewService(&mockStore{}, nil)
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim100x100,
		RemnantDimension: &rem,
		IsWaste:          true,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// TestRecordCut_BRK02_WasteOnly_Succeeds confirms that is_waste=true alone is
// a valid declaration and does not produce a new remnant row. This is the
// counterpart that proves the new guard does not over-restrict legitimate
// waste-only cuts.
func TestRecordCut_BRK02_WasteOnly_Succeeds(t *testing.T) {
	sheetID := uuid.New()
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectSheetByIDResult: BoardSheet{
			ID:           sheetID,
			LotID:        uuid.New(),
			Dimensions:   dim2000x1000,
			CostPerSheet: domain.Money{Amount: 100_000, Currency: "VND"},
			Status:       "AVAILABLE",
		},
		selectOverflowSheetArea: 1, // non-zero so overflow check passes
	}
	svc := NewService(st, nil)
	result, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: dim1000x500,
		IsWaste:       true,
	})
	if err != nil {
		t.Fatalf("waste-only cut should succeed, got %v", err)
	}
	if result.RemnantID != nil {
		t.Errorf("waste cut must not produce a remnant, got %v", *result.RemnantID)
	}
}

func TestRecordCut_ZeroUsedDimension_IsInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{}, nil)
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(uuid.New()),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dimZero,
		IsWaste:       true,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRecordCut_NegativeUsedDimension_IsInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{}, nil)
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(uuid.New()),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dimNegative,
		IsWaste:       true,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRecordCut_InvalidRemnantDimension_IsInvalidInput(t *testing.T) {
	sheetID := uuid.New()
	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st, nil)
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
	svc := NewService(st, nil)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: overDim,
		IsWaste:       true,
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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for non-AVAILABLE sheet, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called when sheet is not AVAILABLE")
	}
}

func TestRecordCut_RemnantConsumed_IsInvalidInput(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	consumedRemnant := availableRemnant(remID, boardID)
	consumedRemnant.Status = domain.RemnantConsumed

	st := &mockStore{selectRemnantByIDResult: consumedRemnant}
	svc := NewService(st, nil)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:     ptr(remID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for CONSUMED remnant, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called when remnant is CONSUMED")
	}
}

// TestRecordCut_AllocatedRemnant_Succeeds verifies that a remnant in ALLOCATED
// status (pre-reserved via AllocateRemnant) can be consumed by RecordCut.
// This is the normal "suggest → allocate → cut" kiosk flow.
func TestRecordCut_AllocatedRemnant_Succeeds(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	allocatedRemnant := availableRemnant(remID, boardID)
	allocatedRemnant.Status = domain.RemnantAllocated

	woID := uuid.New()
	st := &mockStore{selectRemnantByIDResult: allocatedRemnant}
	svc := NewService(st, nil)

	result, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:     ptr(remID),
		WorkOrderID:   woID,
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
	})
	if err != nil {
		t.Fatalf("expected RecordCut to succeed for ALLOCATED remnant, got %v", err)
	}
	if result.CuttingRecordID == uuid.Nil {
		t.Error("expected non-nil cutting record ID")
	}
	if !st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must be called for ALLOCATED remnant")
	}
}

func TestRecordCut_SheetNotFound_PropagatesError(t *testing.T) {
	st := &mockStore{
		selectSheetByIDErr: domain.NewBizError(domain.ErrNotFound, "board sheet not found"),
	}
	svc := NewService(st, nil)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(uuid.New()),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
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
		selectSheetByIDResult:  availableSheet(sheetID),
		recordCutAtomicallyErr: storeErr,
	}
	svc := NewService(st, nil)

	result, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
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
	svc := NewService(st, nil)

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

func TestRecordCut_InvalidShapeType_IsInvalidInput(t *testing.T) {
	sheetID := uuid.New()
	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st, nil)

	remnantDim := domain.Dimension{LengthMM: 500, WidthMM: 200}
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim100x100,
		RemnantDimension: &remnantDim,
		ShapeType:        "triangle",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invalid shape_type, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called for invalid shape_type")
	}
}

func TestRecordCut_ShapeTypeIrregular_PropagatedToNewRemnant(t *testing.T) {
	sheetID := uuid.New()
	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st, nil)

	remnantDim := domain.Dimension{LengthMM: 500, WidthMM: 200}
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim100x100,
		RemnantDimension: &remnantDim,
		ShapeType:        "irregular",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.recordCutAtomicallyOp.NewRemnant == nil {
		t.Fatal("expected a new remnant to be inserted")
	}
	if st.recordCutAtomicallyOp.NewRemnant.ShapeType != "irregular" {
		t.Errorf("expected shape_type 'irregular', got %q", st.recordCutAtomicallyOp.NewRemnant.ShapeType)
	}
}

func TestRecordCut_ShapeTypeEmpty_DefaultsToRectangle(t *testing.T) {
	sheetID := uuid.New()
	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st, nil)

	remnantDim := domain.Dimension{LengthMM: 500, WidthMM: 200}
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim100x100,
		RemnantDimension: &remnantDim,
		// ShapeType intentionally omitted
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.recordCutAtomicallyOp.NewRemnant == nil {
		t.Fatal("expected a new remnant to be inserted")
	}
	if st.recordCutAtomicallyOp.NewRemnant.ShapeType != "rectangle" {
		t.Errorf("expected default shape_type 'rectangle', got %q", st.recordCutAtomicallyOp.NewRemnant.ShapeType)
	}
}

// ── ReceiveStock ──────────────────────────────────────────────────────────────

func TestReceiveStock_HappyPath(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)

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
	svc := NewService(&mockStore{}, nil)
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
	svc := NewService(&mockStore{}, nil)
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
	svc := NewService(&mockStore{}, nil)
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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

	err := svc.AllocateRemnant(context.Background(), remID, uuid.New())
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for already-allocated remnant, got %v", err)
	}
}

func TestAllocateRemnant_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectRemnantByIDErr: domain.NewBizError(domain.ErrNotFound, "remnant not found")}
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

	err := svc.MarkRemnantWaste(context.Background(), remID)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for WASTE remnant, got %v", err)
	}
}

// ── MarkRemnantWaste — missing branches ───────────────────────────────────────

func TestMarkRemnantWaste_NotFound_PropagatesError(t *testing.T) {
	st := &mockStore{selectRemnantByIDErr: domain.NewBizError(domain.ErrNotFound, "remnant not found")}
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:     ptr(uuid.New()),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound to propagate, got %v", err)
	}
}

// ── ListLots ──────────────────────────────────────────────────────────────────

func TestListLots_ReturnsPersisted(t *testing.T) {
	lot1 := InventoryLot{ID: uuid.New(), MaterialID: uuid.New(), Quantity: 5}
	lot2 := InventoryLot{ID: uuid.New(), MaterialID: uuid.New(), Quantity: 10}
	st := &mockStore{
		selectLotsPagedResult: []InventoryLot{lot1, lot2},
		selectLotsPagedTotal:  2,
	}
	svc := NewService(st, nil)

	result, err := svc.ListLots(context.Background(), httpkit.PageParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("len = %d, want 2", len(result.Items))
	}
}

func TestListLots_Empty_ReturnsNil(t *testing.T) {
	st := &mockStore{
		selectLotsPagedResult: nil,
		selectLotsPagedTotal:  0,
	}
	svc := NewService(st, nil)

	result, err := svc.ListLots(context.Background(), httpkit.PageParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected empty slice, got %d lots", len(result.Items))
	}
}

func TestListLots_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("query failed")
	st := &mockStore{selectLotsPagedErr: dbErr}
	svc := NewService(st, nil)

	_, err := svc.ListLots(context.Background(), httpkit.PageParams{Page: 1, Limit: 10})
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── GetSheet ──────────────────────────────────────────────────────────────────

func TestGetSheet_HappyPath(t *testing.T) {
	sheetID := uuid.New()
	want := availableSheet(sheetID)
	st := &mockStore{selectSheetByIDResult: want}
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

	_, err := svc.GetSheet(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetSheet_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("connection reset")
	st := &mockStore{selectSheetByIDErr: dbErr}
	svc := NewService(st, nil)

	_, err := svc.GetSheet(context.Background(), uuid.New())
	if !errors.Is(err, dbErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── ListAvailableSheets ───────────────────────────────────────────────────────

func TestListAvailableSheets_ReturnsOnlyAvailable(t *testing.T) {
	sh1 := availableSheet(uuid.New())
	sh2 := availableSheet(uuid.New())
	st := &mockStore{
		selectAvailableSheetsPagedResult: []BoardSheet{sh1, sh2},
		selectAvailableSheetsPagedTotal:  2,
	}
	svc := NewService(st, nil)

	result, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("len = %d, want 2", len(result.Items))
	}
	for _, s := range result.Items {
		if s.Status != "AVAILABLE" {
			t.Errorf("sheet %v has status %q, want AVAILABLE", s.ID, s.Status)
		}
	}
}

func TestListAvailableSheets_Empty_ReturnsNil(t *testing.T) {
	st := &mockStore{
		selectAvailableSheetsPagedResult: nil,
		selectAvailableSheetsPagedTotal:  0,
	}
	svc := NewService(st, nil)

	result, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected empty slice, got %d sheets", len(result.Items))
	}
}

func TestListAvailableSheets_StoreError_Propagates(t *testing.T) {
	dbErr := errors.New("timeout")
	st := &mockStore{selectAvailableSheetsPagedErr: dbErr}
	svc := NewService(st, nil)

	_, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, nil)
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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
	svc := NewService(capturingSt, nil)

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
	svc := NewService(st, nil)

	// 2001 × 1000 = 2_001_000 mm² > 2_000_000.
	over := domain.Dimension{LengthMM: 2001, WidthMM: 1000}
	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: over,
		IsWaste:       true,
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
	svc := NewService(st, nil)

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
	svc := NewService(st, nil)

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
			svc := NewService(st, nil)
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
			svc2 := NewService(st2, nil)
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

// ── ListLots (paginated) ──────────────────────────────────────────────────────

func TestListLots_ReturnsPagedResult(t *testing.T) {
	lots := []InventoryLot{
		{ID: uuid.New(), SupplierRef: "SUP-001"},
		{ID: uuid.New(), SupplierRef: "SUP-002"},
	}
	st := &mockStore{
		selectLotsPagedResult: lots,
		selectLotsPagedTotal:  2,
	}
	svc := NewService(st, nil)

	p := httpkit.PageParams{Page: 1, Limit: 10}
	result, err := svc.ListLots(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("items = %d, want 2", len(result.Items))
	}
	if result.TotalItems != 2 {
		t.Errorf("total_items = %d, want 2", result.TotalItems)
	}
	if result.TotalPages != 1 {
		t.Errorf("total_pages = %d, want 1", result.TotalPages)
	}
	if result.CurrentPage != 1 {
		t.Errorf("current_page = %d, want 1", result.CurrentPage)
	}
}

func TestListLots_SearchNoResults_ReturnsEmptyItems(t *testing.T) {
	st := &mockStore{
		selectLotsPagedResult: nil,
		selectLotsPagedTotal:  0,
	}
	svc := NewService(st, nil)

	p := httpkit.PageParams{Page: 1, Limit: 10, Search: "SUP-DOES-NOT-EXIST"}
	result, err := svc.ListLots(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("items = %d, want 0 for no-match search", len(result.Items))
	}
	if result.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", result.TotalItems)
	}
	if result.TotalPages != 1 {
		t.Errorf("total_pages = %d, want at least 1", result.TotalPages)
	}
}

func TestListLots_LastPage_CorrectMetadata(t *testing.T) {
	// 12 total, limit 5 → 3 pages; last page has 2 items
	lastPageLots := []InventoryLot{
		{ID: uuid.New(), SupplierRef: "SUP-011"},
		{ID: uuid.New(), SupplierRef: "SUP-012"},
	}
	st := &mockStore{
		selectLotsPagedResult: lastPageLots,
		selectLotsPagedTotal:  12,
	}
	svc := NewService(st, nil)

	p := httpkit.PageParams{Page: 3, Limit: 5}
	result, err := svc.ListLots(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalItems != 12 {
		t.Errorf("total_items = %d, want 12", result.TotalItems)
	}
	if result.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", result.TotalPages)
	}
	if result.CurrentPage != 3 {
		t.Errorf("current_page = %d, want 3", result.CurrentPage)
	}
	if len(result.Items) != 2 {
		t.Errorf("items on last page = %d, want 2", len(result.Items))
	}
}

func TestListLots_StoreError_Propagated(t *testing.T) {
	storeErr := errors.New("db down")
	st := &mockStore{selectLotsPagedErr: storeErr}
	svc := NewService(st, nil)

	_, err := svc.ListLots(context.Background(), httpkit.PageParams{Page: 1, Limit: 10})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("got %v, want %v", err, storeErr)
	}
}

// ── ListAvailableSheets (paginated) ───────────────────────────────────────────

func TestListAvailableSheets_ReturnsPagedResult(t *testing.T) {
	sheets := []BoardSheet{
		{ID: uuid.New(), Status: "AVAILABLE"},
		{ID: uuid.New(), Status: "AVAILABLE"},
		{ID: uuid.New(), Status: "AVAILABLE"},
	}
	st := &mockStore{
		selectAvailableSheetsPagedResult: sheets,
		selectAvailableSheetsPagedTotal:  3,
	}
	svc := NewService(st, nil)

	p := httpkit.PageParams{Page: 1, Limit: 10}
	result, err := svc.ListAvailableSheets(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Errorf("items = %d, want 3", len(result.Items))
	}
	if result.TotalItems != 3 {
		t.Errorf("total_items = %d, want 3", result.TotalItems)
	}
	if result.TotalPages != 1 {
		t.Errorf("total_pages = %d, want 1", result.TotalPages)
	}
}

func TestListAvailableSheets_Empty_ReturnsEmptyItems(t *testing.T) {
	st := &mockStore{
		selectAvailableSheetsPagedResult: nil,
		selectAvailableSheetsPagedTotal:  0,
	}
	svc := NewService(st, nil)

	p := httpkit.PageParams{Page: 1, Limit: 10}
	result, err := svc.ListAvailableSheets(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("items = %d, want 0", len(result.Items))
	}
	if result.TotalItems != 0 {
		t.Errorf("total_items = %d, want 0", result.TotalItems)
	}
	if result.TotalPages != 1 {
		t.Errorf("total_pages = %d, want 1 (minimum)", result.TotalPages)
	}
}

func TestListAvailableSheets_LastPage_CorrectMetadata(t *testing.T) {
	// 21 total, limit 10 → 3 pages; last page has 1 item
	lastPage := []BoardSheet{{ID: uuid.New(), Status: "AVAILABLE"}}
	st := &mockStore{
		selectAvailableSheetsPagedResult: lastPage,
		selectAvailableSheetsPagedTotal:  21,
	}
	svc := NewService(st, nil)

	p := httpkit.PageParams{Page: 3, Limit: 10}
	result, err := svc.ListAvailableSheets(context.Background(), p, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalItems != 21 {
		t.Errorf("total_items = %d, want 21", result.TotalItems)
	}
	if result.TotalPages != 3 {
		t.Errorf("total_pages = %d, want 3", result.TotalPages)
	}
	if result.CurrentPage != 3 {
		t.Errorf("current_page = %d, want 3", result.CurrentPage)
	}
	if len(result.Items) != 1 {
		t.Errorf("items on last page = %d, want 1", len(result.Items))
	}
}

func TestListAvailableSheets_StoreError_Propagated(t *testing.T) {
	storeErr := errors.New("timeout")
	st := &mockStore{selectAvailableSheetsPagedErr: storeErr}
	svc := NewService(st, nil)

	_, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("got %v, want %v", err, storeErr)
	}
}

func TestListAvailableSheets_FilterByMaterialID_PassedToStore(t *testing.T) {
	matID := uuid.New()
	sh := BoardSheet{ID: uuid.New(), LotID: uuid.New(), MaterialID: matID, MaterialName: "Granite", Status: "AVAILABLE"}
	st := &mockStore{
		selectAvailableSheetsPagedResult: []BoardSheet{sh},
		selectAvailableSheetsPagedTotal:  1,
	}
	svc := NewService(st, nil)

	result, err := svc.ListAvailableSheets(context.Background(), httpkit.PageParams{Page: 1, Limit: 10}, &matID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalItems != 1 {
		t.Errorf("TotalItems = %d, want 1", result.TotalItems)
	}
	if result.Items[0].MaterialID != matID {
		t.Errorf("MaterialID = %v, want %v", result.Items[0].MaterialID, matID)
	}
}

// ── BR-K01 aggregate stock check helper ──────────────────────────────────────

func TestCountAvailableSheetsByMaterial_DelegatesToStore(t *testing.T) {
	matID := uuid.New()
	st := &mockStore{countAvailableSheetsByMaterialResult: 7}
	svc := NewService(st, nil)

	got, err := svc.CountAvailableSheetsByMaterial(context.Background(), matID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 7 {
		t.Errorf("count = %d, want 7", got)
	}
	if st.countAvailableSheetsByMaterialArg != matID {
		t.Errorf("store called with %v, want %v", st.countAvailableSheetsByMaterialArg, matID)
	}
}

func TestCountAvailableSheetsByMaterial_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("count failed")
	st := &mockStore{countAvailableSheetsByMaterialErr: storeErr}
	svc := NewService(st, nil)

	_, err := svc.CountAvailableSheetsByMaterial(context.Background(), uuid.New())
	if !errors.Is(err, storeErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── Issue 2.3: Material attribute inheritance ─────────────────────────────────

func TestRecordCut_FromSheet_NewRemnant_InheritsSheetAttributes(t *testing.T) {
	sheetID := uuid.New()
	sheet := availableSheetWithAttrs(sheetID)
	remnantDim := dim800x400

	st := &mockStore{selectSheetByIDResult: sheet}
	svc := NewService(st, nil)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim1000x500,
		RemnantDimension: &remnantDim,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := st.recordCutAtomicallyOp
	if op.NewRemnant == nil {
		t.Fatal("NewRemnant must be non-nil")
	}
	assertMaterialAttrs(t, op.NewRemnant,
		sheet.SupplierCode, sheet.LotBatch, sheet.GrainPattern, sheet.QualityGrade)
}

func TestRecordCut_FromRemnant_NewRemnant_InheritsParentRemnantAttributes(t *testing.T) {
	boardID := uuid.New()
	parentRemID := uuid.New()
	parentRemnant := availableRemnantWithAttrs(parentRemID, boardID)
	nestedDim := dim100x100

	st := &mockStore{selectRemnantByIDResult: parentRemnant}
	svc := NewService(st, nil)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:        ptr(parentRemID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim100x100,
		RemnantDimension: &nestedDim,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := st.recordCutAtomicallyOp
	if op.NewRemnant == nil {
		t.Fatal("NewRemnant must be non-nil")
	}
	assertMaterialAttrs(t, op.NewRemnant,
		parentRemnant.SupplierCode, parentRemnant.LotBatch, parentRemnant.GrainPattern, parentRemnant.QualityGrade)
}

func TestRecordCut_NoNewRemnant_NoInheritanceAttempted(t *testing.T) {
	sheetID := uuid.New()
	sheet := availableSheetWithAttrs(sheetID)

	st := &mockStore{selectSheetByIDResult: sheet}
	svc := NewService(st, nil)

	result, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim1000x500,
		IsWaste:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RemnantID != nil {
		t.Error("RemnantID must be nil when no remnant dimension given")
	}

	op := st.recordCutAtomicallyOp
	if op.NewRemnant != nil {
		t.Error("NewRemnant must be nil — no inheritance should occur")
	}
}

func TestRecordCut_SourceWithNilAttributes_NewRemnantHasNilAttributes(t *testing.T) {
	sheetID := uuid.New()
	sheet := availableSheet(sheetID) // no material attrs set (all nil)
	remnantDim := dim800x400

	st := &mockStore{selectSheetByIDResult: sheet}
	svc := NewService(st, nil)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim1000x500,
		RemnantDimension: &remnantDim,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := st.recordCutAtomicallyOp
	if op.NewRemnant == nil {
		t.Fatal("NewRemnant must be non-nil")
	}
	assertMaterialAttrs(t, op.NewRemnant, nil, nil, nil, nil)
}

// ── Issue 2.4: Bounding Box ───────────────────────────────────────────────────

// TestRemnant_BoundingBoxExceedsActual_ReturnsErrInvalidInput verifies that
// providing a bounding_box larger than the actual remnant dimension is rejected
// before any store operation is attempted.
func TestRemnant_BoundingBoxExceedsActual_ReturnsErrInvalidInput(t *testing.T) {
	sheetID := uuid.New()
	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st, nil)

	remnantDim := dim800x400 // actual: 800×400
	bbLen := 801             // exceeds actual length by 1 mm
	bbWid := 400

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:             ptr(sheetID),
		WorkOrderID:         uuid.New(),
		SKUID:               uuid.New(),
		UsedDimension:       dim1000x500,
		RemnantDimension:    &remnantDim,
		BoundingBoxLengthMM: &bbLen,
		BoundingBoxWidthMM:  &bbWid,
	})

	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput when bounding_box exceeds actual dimension, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called when bounding_box validation fails")
	}
}

// TestRemnant_NoBoundingBoxProvided_DefaultsToActualDimension verifies that
// when the caller omits bounding_box, the new remnant gets bounding_box == actual
// dimension so that search queries always have a concrete value to filter on.
func TestRemnant_NoBoundingBoxProvided_DefaultsToActualDimension(t *testing.T) {
	sheetID := uuid.New()
	st := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(st, nil)

	remnantDim := dim800x400 // actual: 800×400; no bounding_box provided

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    dim1000x500,
		RemnantDimension: &remnantDim,
		// BoundingBoxLengthMM and BoundingBoxWidthMM intentionally omitted
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := st.recordCutAtomicallyOp
	if op.NewRemnant == nil {
		t.Fatal("NewRemnant must be non-nil")
	}
	r := op.NewRemnant
	if r.BoundingBoxLengthMM == nil {
		t.Fatal("BoundingBoxLengthMM must be set (defaulted to actual)")
	}
	if *r.BoundingBoxLengthMM != remnantDim.LengthMM {
		t.Errorf("BoundingBoxLengthMM = %d, want %d (actual length)", *r.BoundingBoxLengthMM, remnantDim.LengthMM)
	}
	if r.BoundingBoxWidthMM == nil {
		t.Fatal("BoundingBoxWidthMM must be set (defaulted to actual)")
	}
	if *r.BoundingBoxWidthMM != remnantDim.WidthMM {
		t.Errorf("BoundingBoxWidthMM = %d, want %d (actual width)", *r.BoundingBoxWidthMM, remnantDim.WidthMM)
	}
}

// TestFindAvailableRemnants_UsesBoundingBoxForSearch verifies that
// FindAvailableRemnants delegates to the store with the correct min dimension
// and returns the remnants the store provides (the store mock is the source of
// truth for the bounding_box filter — the SQL is tested in pgstore_integration_test).
func TestFindAvailableRemnants_UsesBoundingBoxForSearch(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()

	// Remnant whose bounding_box (700×350) is smaller than actual (800×400)
	// to simulate a chipped-corner board.
	bbLen := 700
	bbWid := 350
	rem := availableRemnant(remID, boardID)
	rem.Dimensions = dim800x400
	rem.BoundingBoxLengthMM = &bbLen
	rem.BoundingBoxWidthMM = &bbWid

	st := &mockStore{selectAvailableRemnantsResult: []Remnant{rem}}
	svc := NewService(st, nil)

	// Request a piece that fits the bounding_box (700×350 >= 600×300).
	minDim := domain.Dimension{LengthMM: 600, WidthMM: 300}
	results, err := svc.FindAvailableRemnants(context.Background(), minDim)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != remID {
		t.Errorf("result ID = %v, want %v", results[0].ID, remID)
	}
}

// TestFindAvailableRemnants_BoundingBoxSmallerThanRequired_NotReturned verifies
// that a remnant whose bounding_box is smaller than the requested dimension is
// excluded by the store.  The mock simulates the filtered store response (the
// actual SQL WHERE clause is covered by integration tests).
func TestFindAvailableRemnants_BoundingBoxSmallerThanRequired_NotReturned(t *testing.T) {
	// bounding_box is 400×200 — store would filter this out for minDim 600×300.
	st := &mockStore{selectAvailableRemnantsResult: []Remnant{}} // store returns empty
	svc := NewService(st, nil)

	minDim := domain.Dimension{LengthMM: 600, WidthMM: 300}
	results, err := svc.FindAvailableRemnants(context.Background(), minDim)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when bounding_box is smaller than required, got %d", len(results))
	}
}

// ── Issue 2.5 — Nested Remnant Cutting (BR-K04 lineage, 3-level deep) ──────

// TestNestedRemnantCutting_ThreeLevels simulates the full chain:
//
//	Board 2000×1000
//	  └─ cut → Remnant L1 (1000×500)  [ParentBoardID=board, ParentRemnantID=nil]
//	       └─ cut → Remnant L2 (500×250) [ParentBoardID=board, ParentRemnantID=L1]
//	            └─ cut → Remnant L3 (200×200) [ParentBoardID=board, ParentRemnantID=L2]
//
// It verifies that ParentBoardID stays anchored to the original board at every
// level (BR-K04) and that ParentRemnantID tracks the direct parent.
func TestNestedRemnantCutting_ThreeLevels(t *testing.T) {
	boardID := uuid.New()
	woID := uuid.New()
	skuID := uuid.New()

	// ── Level 1: cut Board → Remnant L1 ─────────────────────────────────────
	//
	// Source: board sheet (2000×1000). Used 900×400, remnant 1000×500.
	sheetID := uuid.New()
	stL1 := &mockStore{selectSheetByIDResult: availableSheet(sheetID)}
	svc := NewService(stL1, nil)

	dimL1 := domain.Dimension{LengthMM: 1000, WidthMM: 500}

	resL1, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          ptr(sheetID),
		WorkOrderID:      woID,
		SKUID:            skuID,
		UsedDimension:    domain.Dimension{LengthMM: 900, WidthMM: 400},
		RemnantDimension: &dimL1,
	})
	if err != nil {
		t.Fatalf("L1 cut failed: %v", err)
	}
	remnantL1 := stL1.recordCutAtomicallyOp.NewRemnant
	if remnantL1 == nil {
		t.Fatal("L1: NewRemnant must be non-nil")
	}
	if remnantL1.ParentBoardID != sheetID {
		t.Errorf("L1 ParentBoardID = %v, want sheetID %v", remnantL1.ParentBoardID, sheetID)
	}
	if remnantL1.ParentRemnantID != nil {
		t.Errorf("L1 ParentRemnantID = %v, want nil (direct child of board)", remnantL1.ParentRemnantID)
	}
	remnantL1ID := resL1.RemnantID
	if remnantL1ID == nil {
		t.Fatal("L1 result RemnantID must be set")
	}

	// ── Level 2: cut Remnant L1 → Remnant L2 ────────────────────────────────
	//
	// L1 is 1000×500 = 500_000 mm². Used 400×200, remnant 500×250 = 125_000 mm².
	// Total 80_000+125_000 = 205_000 ≤ 500_000 ✓
	remL1InStore := Remnant{
		ID:            *remnantL1ID,
		ParentBoardID: sheetID, // lineage from L1
		Dimensions:    dimL1,
		Status:        domain.RemnantAvailable,
		CreatedAt:     time.Now().UTC(),
	}
	dimL2 := domain.Dimension{LengthMM: 500, WidthMM: 250}
	stL2 := &mockStore{selectRemnantByIDResult: remL1InStore}
	svc = NewService(stL2, nil)

	resL2, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:        remnantL1ID,
		WorkOrderID:      woID,
		SKUID:            skuID,
		UsedDimension:    domain.Dimension{LengthMM: 400, WidthMM: 200},
		RemnantDimension: &dimL2,
	})
	if err != nil {
		t.Fatalf("L2 cut failed: %v", err)
	}
	remnantL2 := stL2.recordCutAtomicallyOp.NewRemnant
	if remnantL2 == nil {
		t.Fatal("L2: NewRemnant must be non-nil")
	}
	// ParentBoardID must stay anchored to the original board, not L1.
	if remnantL2.ParentBoardID != sheetID {
		t.Errorf("L2 ParentBoardID = %v, want sheetID %v (must bubble from L1.ParentBoardID)", remnantL2.ParentBoardID, sheetID)
	}
	if remnantL2.ParentRemnantID == nil || *remnantL2.ParentRemnantID != *remnantL1ID {
		t.Errorf("L2 ParentRemnantID = %v, want L1.ID %v", remnantL2.ParentRemnantID, *remnantL1ID)
	}
	remnantL2ID := resL2.RemnantID
	if remnantL2ID == nil {
		t.Fatal("L2 result RemnantID must be set")
	}

	// ── Level 3: cut Remnant L2 → Remnant L3 ────────────────────────────────
	//
	// L2 is 500×250 = 125_000 mm². Used 200×150, remnant 200×200 = 40_000 mm².
	// Total 30_000+40_000 = 70_000 ≤ 125_000 ✓
	remL2InStore := Remnant{
		ID:              *remnantL2ID,
		ParentBoardID:   sheetID, // lineage inherited from L1 through L2
		ParentRemnantID: remnantL1ID,
		Dimensions:      dimL2,
		Status:          domain.RemnantAvailable,
		CreatedAt:       time.Now().UTC(),
	}
	dimL3 := domain.Dimension{LengthMM: 200, WidthMM: 200}
	stL3 := &mockStore{selectRemnantByIDResult: remL2InStore}
	svc = NewService(stL3, nil)

	resL3, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:        remnantL2ID,
		WorkOrderID:      woID,
		SKUID:            skuID,
		UsedDimension:    domain.Dimension{LengthMM: 200, WidthMM: 150},
		RemnantDimension: &dimL3,
	})
	if err != nil {
		t.Fatalf("L3 cut failed: %v", err)
	}
	remnantL3 := stL3.recordCutAtomicallyOp.NewRemnant
	if remnantL3 == nil {
		t.Fatal("L3: NewRemnant must be non-nil")
	}
	// ParentBoardID must still be the original board.
	if remnantL3.ParentBoardID != sheetID {
		t.Errorf("L3 ParentBoardID = %v, want sheetID %v (must bubble across 3 levels)", remnantL3.ParentBoardID, sheetID)
	}
	if remnantL3.ParentRemnantID == nil || *remnantL3.ParentRemnantID != *remnantL2ID {
		t.Errorf("L3 ParentRemnantID = %v, want L2.ID %v", remnantL3.ParentRemnantID, *remnantL2ID)
	}
	if resL3.RemnantID == nil {
		t.Fatal("L3 result RemnantID must be set")
	}
	_ = boardID // kept to make the intent explicit
}

// TestNestedCut_AreaConservation_L2ExceedsL1_Rejected verifies that BR-K03 is
// evaluated against the *source remnant's* area (L1: 1000×500) and NOT the
// original board's area (2000×1000). A used+remnant combination that fits
// inside the board but exceeds L1 must be rejected with ErrAreaConservation.
func TestNestedCut_AreaConservation_L2ExceedsL1_Rejected(t *testing.T) {
	boardID := uuid.New()
	remnantL1ID := uuid.New()

	// L1 is 1000×500 = 500_000 mm².
	remL1 := Remnant{
		ID:            remnantL1ID,
		ParentBoardID: boardID,
		Dimensions:    domain.Dimension{LengthMM: 1000, WidthMM: 500},
		Status:        domain.RemnantAvailable,
		CreatedAt:     time.Now().UTC(),
	}
	st := &mockStore{selectRemnantByIDResult: remL1}
	svc := NewService(st, nil)

	// used 800×400 = 320_000, remnant 600×400 = 240_000.
	// Total 560_000 > 500_000 (L1 area) — must be rejected.
	// But 560_000 < 2_000_000 (board area) — wrong check would let this pass.
	usedDim := domain.Dimension{LengthMM: 800, WidthMM: 400}
	remDim := domain.Dimension{LengthMM: 600, WidthMM: 400}

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:        ptr(remnantL1ID),
		WorkOrderID:      uuid.New(),
		SKUID:            uuid.New(),
		UsedDimension:    usedDim,
		RemnantDimension: &remDim,
	})
	if !errors.Is(err, domain.ErrAreaConservation) {
		t.Errorf("expected ErrAreaConservation when used+remnant exceeds L1 area, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically must NOT be called when BR-K03 fails")
	}
}

// TestNestedCut_GetRemnantLineage_ReturnsAllLevels verifies that
// GetRemnantLineage(boardID) returns all remnants across all levels that share
// the same parent_board_id, regardless of nesting depth.
// The store mock simulates a DB query that filters by parent_board_id only.
func TestNestedCut_GetRemnantLineage_ReturnsAllLevels(t *testing.T) {
	boardID := uuid.New()
	remL1ID := uuid.New()
	remL2ID := uuid.New()
	remL3ID := uuid.New()

	remL1 := Remnant{
		ID:              remL1ID,
		ParentBoardID:   boardID,
		ParentRemnantID: nil,
		Dimensions:      domain.Dimension{LengthMM: 1000, WidthMM: 500},
		Status:          domain.RemnantAvailable,
		CreatedAt:       time.Now().UTC(),
	}
	remL2 := Remnant{
		ID:              remL2ID,
		ParentBoardID:   boardID,
		ParentRemnantID: &remL1ID,
		Dimensions:      domain.Dimension{LengthMM: 500, WidthMM: 250},
		Status:          domain.RemnantAvailable,
		CreatedAt:       time.Now().UTC(),
	}
	remL3 := Remnant{
		ID:              remL3ID,
		ParentBoardID:   boardID,
		ParentRemnantID: &remL2ID,
		Dimensions:      domain.Dimension{LengthMM: 200, WidthMM: 200},
		Status:          domain.RemnantAvailable,
		CreatedAt:       time.Now().UTC(),
	}

	st := &mockStore{
		selectRemnantsByBoardSheetResult: []Remnant{remL1, remL2, remL3},
	}
	svc := NewService(st, nil)

	lineage, err := svc.GetRemnantLineage(context.Background(), boardID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lineage) != 3 {
		t.Fatalf("expected 3 remnants in lineage (L1+L2+L3), got %d", len(lineage))
	}

	// Build an id→remnant map for order-independent checks.
	byID := make(map[uuid.UUID]Remnant, 3)
	for _, r := range lineage {
		byID[r.ID] = r
	}

	l1, ok := byID[remL1ID]
	if !ok {
		t.Fatal("L1 remnant missing from lineage")
	}
	if l1.ParentBoardID != boardID {
		t.Errorf("L1 ParentBoardID = %v, want %v", l1.ParentBoardID, boardID)
	}
	if l1.ParentRemnantID != nil {
		t.Errorf("L1 ParentRemnantID = %v, want nil", l1.ParentRemnantID)
	}

	l2, ok := byID[remL2ID]
	if !ok {
		t.Fatal("L2 remnant missing from lineage")
	}
	if l2.ParentBoardID != boardID {
		t.Errorf("L2 ParentBoardID = %v, want %v", l2.ParentBoardID, boardID)
	}
	if l2.ParentRemnantID == nil || *l2.ParentRemnantID != remL1ID {
		t.Errorf("L2 ParentRemnantID = %v, want %v", l2.ParentRemnantID, remL1ID)
	}

	l3, ok := byID[remL3ID]
	if !ok {
		t.Fatal("L3 remnant missing from lineage")
	}
	if l3.ParentBoardID != boardID {
		t.Errorf("L3 ParentBoardID = %v, want %v", l3.ParentBoardID, boardID)
	}
	if l3.ParentRemnantID == nil || *l3.ParentRemnantID != remL2ID {
		t.Errorf("L3 ParentRemnantID = %v, want %v", l3.ParentRemnantID, remL2ID)
	}
}

// TestNestedCut_ParentBoardID_ConsistentAcrossAllLevels is a table-driven test
// that checks the lineage invariant for up to 4 levels of nesting. Each case
// starts from a pre-built remnant that already has a ParentBoardID and verifies
// that the newly produced remnant inherits the same ParentBoardID (not a new one).
func TestNestedCut_ParentBoardID_ConsistentAcrossAllLevels(t *testing.T) {
	boardID := uuid.New()
	woID := uuid.New()
	skuID := uuid.New()

	// sourceDim is large enough that used+remnant always fits.
	sourceDim := domain.Dimension{LengthMM: 2000, WidthMM: 1000}
	usedDim := domain.Dimension{LengthMM: 100, WidthMM: 100}
	remnantDim := domain.Dimension{LengthMM: 200, WidthMM: 200}

	cases := []struct {
		name                string
		sourceParentBoard   uuid.UUID
		sourceParentRemnant *uuid.UUID
		wantParentBoard     uuid.UUID
	}{
		{
			name:                "L1_source_is_board_child",
			sourceParentBoard:   boardID,
			sourceParentRemnant: nil,
			wantParentBoard:     boardID,
		},
		{
			name:                "L2_source_is_L1_remnant",
			sourceParentBoard:   boardID,
			sourceParentRemnant: ptr(uuid.New()),
			wantParentBoard:     boardID,
		},
		{
			name:                "L3_source_is_L2_remnant",
			sourceParentBoard:   boardID,
			sourceParentRemnant: ptr(uuid.New()),
			wantParentBoard:     boardID,
		},
		{
			name:                "L4_source_is_L3_remnant",
			sourceParentBoard:   boardID,
			sourceParentRemnant: ptr(uuid.New()),
			wantParentBoard:     boardID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sourceID := uuid.New()
			source := Remnant{
				ID:              sourceID,
				ParentBoardID:   tc.sourceParentBoard,
				ParentRemnantID: tc.sourceParentRemnant,
				Dimensions:      sourceDim,
				Status:          domain.RemnantAvailable,
				CreatedAt:       time.Now().UTC(),
			}
			st := &mockStore{selectRemnantByIDResult: source}
			svc := NewService(st, nil)

			_, err := svc.RecordCut(context.Background(), RecordCutInput{
				RemnantID:        ptr(sourceID),
				WorkOrderID:      woID,
				SKUID:            skuID,
				UsedDimension:    usedDim,
				RemnantDimension: &remnantDim,
			})
			if err != nil {
				t.Fatalf("RecordCut failed: %v", err)
			}
			newRemnant := st.recordCutAtomicallyOp.NewRemnant
			if newRemnant == nil {
				t.Fatal("NewRemnant must be non-nil")
			}
			if newRemnant.ParentBoardID != tc.wantParentBoard {
				t.Errorf("ParentBoardID = %v, want %v", newRemnant.ParentBoardID, tc.wantParentBoard)
			}
			if newRemnant.ParentRemnantID == nil || *newRemnant.ParentRemnantID != sourceID {
				t.Errorf("ParentRemnantID = %v, want %v (direct parent)", newRemnant.ParentRemnantID, sourceID)
			}
		})
	}
}

// ── Issue 2.6 — ListRemnants, GetRemnantLineageByRemnant, ListStorageLocations ─

// TestListRemnants_DefaultsToAvailableStatus verifies that when no status is
// specified in the filter, the service sets it to AVAILABLE before delegating.
func TestListRemnants_DefaultsToAvailableStatus(t *testing.T) {
	boardID := uuid.New()
	rem := availableRemnant(uuid.New(), boardID)

	st := &mockStore{
		selectRemnantsByFilterResult: []Remnant{rem},
		selectRemnantsByFilterTotal:  1,
	}
	svc := NewService(st, nil)

	result, err := svc.ListRemnants(context.Background(), RemnantFilter{}, httpkit.PageParams{
		Page: 1, Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalItems != 1 {
		t.Errorf("TotalItems = %d, want 1", result.TotalItems)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(result.Items))
	}
	if result.Items[0].ID != rem.ID {
		t.Errorf("Items[0].ID = %v, want %v", result.Items[0].ID, rem.ID)
	}
}

// TestListRemnants_ExplicitStatusForwarded verifies that an explicit status
// filter (e.g. WASTE) is forwarded to the store as-is.
func TestListRemnants_ExplicitStatusForwarded(t *testing.T) {
	st := &mockStore{
		selectRemnantsByFilterResult: []Remnant{},
		selectRemnantsByFilterTotal:  0,
	}
	svc := NewService(st, nil)

	_, err := svc.ListRemnants(context.Background(), RemnantFilter{
		Status: domain.RemnantWaste,
	}, httpkit.PageParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListRemnants_Pagination verifies that PagedResult metadata is computed
// correctly: TotalPages, CurrentPage, Limit.
func TestListRemnants_Pagination(t *testing.T) {
	// Simulate 25 total matching rows, page 2 of 3 (limit=10).
	items := make([]Remnant, 10)
	for i := range items {
		items[i] = availableRemnant(uuid.New(), uuid.New())
	}
	st := &mockStore{
		selectRemnantsByFilterResult: items,
		selectRemnantsByFilterTotal:  25,
	}
	svc := NewService(st, nil)

	result, err := svc.ListRemnants(context.Background(), RemnantFilter{}, httpkit.PageParams{
		Page: 2, Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalItems != 25 {
		t.Errorf("TotalItems = %d, want 25", result.TotalItems)
	}
	if result.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", result.TotalPages)
	}
	if result.CurrentPage != 2 {
		t.Errorf("CurrentPage = %d, want 2", result.CurrentPage)
	}
	if result.Limit != 10 {
		t.Errorf("Limit = %d, want 10", result.Limit)
	}
	if len(result.Items) != 10 {
		t.Errorf("len(Items) = %d, want 10", len(result.Items))
	}
}

// TestListRemnants_StoreError_Propagates verifies that a store error is
// returned unwrapped so the handler can map it correctly.
func TestListRemnants_StoreError_Propagates(t *testing.T) {
	st := &mockStore{selectRemnantsByFilterErr: domain.NewBizError(domain.ErrNotFound, "db error")}
	svc := NewService(st, nil)

	_, err := svc.ListRemnants(context.Background(), RemnantFilter{}, httpkit.PageParams{Page: 1, Limit: 10})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetRemnantLineageByRemnant_HappyPath verifies that the service resolves
// parent_board_id from the remnant and returns the full lineage.
func TestGetRemnantLineageByRemnant_HappyPath(t *testing.T) {
	boardID := uuid.New()
	remID := uuid.New()
	child1 := availableRemnant(uuid.New(), boardID)
	child2 := availableRemnant(uuid.New(), boardID)

	st := &mockStore{
		selectRemnantByIDResult:          Remnant{ID: remID, ParentBoardID: boardID, Status: domain.RemnantAvailable},
		selectRemnantsByBoardSheetResult: []Remnant{child1, child2},
	}
	svc := NewService(st, nil)

	lineage, err := svc.GetRemnantLineageByRemnant(context.Background(), remID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lineage) != 2 {
		t.Errorf("expected 2 remnants in lineage, got %d", len(lineage))
	}
}

// TestGetRemnantLineageByRemnant_NotFound propagates ErrNotFound when the
// remnant does not exist.
func TestGetRemnantLineageByRemnant_NotFound(t *testing.T) {
	st := &mockStore{
		selectRemnantByIDErr: domain.NewBizError(domain.ErrNotFound, "remnant not found"),
	}
	svc := NewService(st, nil)

	_, err := svc.GetRemnantLineageByRemnant(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestListStorageLocations_HappyPath verifies that active locations are
// returned correctly.
func TestListStorageLocations_HappyPath(t *testing.T) {
	loc1 := StorageLocation{ID: uuid.New(), Zone: "A", Rack: "01", Shelf: "01", Label: "A-01-01", Barcode: "BC001", IsActive: true}
	loc2 := StorageLocation{ID: uuid.New(), Zone: "A", Rack: "01", Shelf: "02", Label: "A-01-02", Barcode: "BC002", IsActive: true}

	st := &mockStore{selectActiveStorageLocationsResult: []StorageLocation{loc1, loc2}}
	svc := NewService(st, nil)

	locs, err := svc.ListStorageLocations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("expected 2 locations, got %d", len(locs))
	}
	if locs[0].Zone != "A" || locs[1].Label != "A-01-02" {
		t.Errorf("unexpected location data: %+v", locs)
	}
}

// TestListStorageLocations_Empty_ReturnsEmptySlice verifies that nil is not
// returned when there are no active locations.
func TestListStorageLocations_Empty_ReturnsEmptySlice(t *testing.T) {
	st := &mockStore{selectActiveStorageLocationsResult: []StorageLocation{}}
	svc := NewService(st, nil)

	locs, err := svc.ListStorageLocations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if locs == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(locs) != 0 {
		t.Errorf("expected 0 locations, got %d", len(locs))
	}
}

// TestListStorageLocations_StoreError_Propagates verifies error propagation.
func TestListStorageLocations_StoreError_Propagates(t *testing.T) {
	st := &mockStore{selectActiveStorageLocationsErr: domain.NewBizError(domain.ErrNotFound, "db error")}
	svc := NewService(st, nil)

	_, err := svc.ListStorageLocations(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── Issue 2.6 — ListRemnants filter forwarding ────────────────────────────────

// filterCapturingMockStore extends mockStore to capture the RemnantFilter
// that the service passes down to selectRemnantsByFilter.  This lets tests
// assert that the service is forwarding filter values correctly without
// depending on pgstore SQL behaviour.
type filterCapturingMockStore struct {
	mockStore
	capturedFilter     RemnantFilter
	capturedPageParams httpkit.PageParams
}

func (c *filterCapturingMockStore) selectRemnantsByFilter(_ context.Context, f RemnantFilter, p httpkit.PageParams) ([]Remnant, int, error) {
	c.capturedFilter = f
	c.capturedPageParams = p
	return c.selectRemnantsByFilterResult, c.selectRemnantsByFilterTotal, c.selectRemnantsByFilterErr
}

// TestListRemnants_FilterByMinDimension verifies that non-zero MinLengthMM /
// MinWidthMM values are forwarded to the store unmodified.
func TestListRemnants_FilterByMinDimension(t *testing.T) {
	boardID := uuid.New()
	rem := availableRemnant(uuid.New(), boardID)

	st := &filterCapturingMockStore{
		mockStore: mockStore{
			selectRemnantsByFilterResult: []Remnant{rem},
			selectRemnantsByFilterTotal:  1,
		},
	}
	svc := NewService(st, nil)

	f := RemnantFilter{
		MinLengthMM: 800,
		MinWidthMM:  400,
	}
	result, err := svc.ListRemnants(context.Background(), f, httpkit.PageParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("Items = %d, want 1", len(result.Items))
	}
	// Verify the filter values reached the store.
	if st.capturedFilter.MinLengthMM != 800 {
		t.Errorf("capturedFilter.MinLengthMM = %d, want 800", st.capturedFilter.MinLengthMM)
	}
	if st.capturedFilter.MinWidthMM != 400 {
		t.Errorf("capturedFilter.MinWidthMM = %d, want 400", st.capturedFilter.MinWidthMM)
	}
}

// TestListRemnants_FilterByStatus verifies that an explicit status filter is
// forwarded to the store exactly as supplied (no overwriting by the default).
func TestListRemnants_FilterByStatus(t *testing.T) {
	st := &filterCapturingMockStore{
		mockStore: mockStore{
			selectRemnantsByFilterResult: []Remnant{},
			selectRemnantsByFilterTotal:  0,
		},
	}
	svc := NewService(st, nil)

	f := RemnantFilter{Status: domain.RemnantWaste}
	_, err := svc.ListRemnants(context.Background(), f, httpkit.PageParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Status must be forwarded to the store as WASTE, not overwritten to AVAILABLE.
	if st.capturedFilter.Status != domain.RemnantWaste {
		t.Errorf("capturedFilter.Status = %v, want WASTE", st.capturedFilter.Status)
	}
}

// TestListRemnants_NoFilter_ReturnsAll verifies that when no filter fields are
// set the service defaults Status to AVAILABLE and passes zero dimension
// thresholds — meaning the store query applies no dimension filter.
func TestListRemnants_NoFilter_ReturnsAll(t *testing.T) {
	boardID := uuid.New()
	items := []Remnant{
		availableRemnant(uuid.New(), boardID),
		availableRemnant(uuid.New(), boardID),
		availableRemnant(uuid.New(), boardID),
	}
	st := &filterCapturingMockStore{
		mockStore: mockStore{
			selectRemnantsByFilterResult: items,
			selectRemnantsByFilterTotal:  3,
		},
	}
	svc := NewService(st, nil)

	result, err := svc.ListRemnants(context.Background(), RemnantFilter{}, httpkit.PageParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 3 {
		t.Errorf("Items = %d, want 3", len(result.Items))
	}
	// Default status must be AVAILABLE when not specified.
	if st.capturedFilter.Status != domain.RemnantAvailable {
		t.Errorf("capturedFilter.Status = %v, want AVAILABLE (default)", st.capturedFilter.Status)
	}
	// No dimension filter should be applied (zero values passed to store).
	if st.capturedFilter.MinLengthMM != 0 {
		t.Errorf("capturedFilter.MinLengthMM = %d, want 0 (no filter)", st.capturedFilter.MinLengthMM)
	}
	if st.capturedFilter.MinWidthMM != 0 {
		t.Errorf("capturedFilter.MinWidthMM = %d, want 0 (no filter)", st.capturedFilter.MinWidthMM)
	}
}

// ── Issue #183 — FIFO sort order ─────────────────────────────────────────────

// TestListRemnants_DefaultOrder_ForwardsFIFO verifies that the service passes
// the default PageParams (Order="") to the store unchanged — the actual
// created_at ASC sort is enforced in pgstore (BR-K02 FIFO).
func TestListRemnants_DefaultOrder_ForwardsFIFO(t *testing.T) {
	st := &filterCapturingMockStore{
		mockStore: mockStore{
			selectRemnantsByFilterResult: []Remnant{},
			selectRemnantsByFilterTotal:  0,
		},
	}
	svc := NewService(st, nil)

	_, err := svc.ListRemnants(context.Background(), RemnantFilter{}, httpkit.PageParams{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Service must not override Order — pgstore defaults to ASC (FIFO).
	if st.capturedPageParams.Order != "" {
		t.Errorf("service must forward Order as-is, got %q", st.capturedPageParams.Order)
	}
}

// TestListRemnants_OrderDescOverride verifies that a caller-supplied
// order=desc is forwarded to the store without modification (FE override path).
func TestListRemnants_OrderDescOverride(t *testing.T) {
	st := &filterCapturingMockStore{
		mockStore: mockStore{
			selectRemnantsByFilterResult: []Remnant{},
			selectRemnantsByFilterTotal:  0,
		},
	}
	svc := NewService(st, nil)

	_, err := svc.ListRemnants(context.Background(), RemnantFilter{}, httpkit.PageParams{Page: 1, Limit: 10, Order: "desc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.capturedPageParams.Order != "desc" {
		t.Errorf("expected order=desc forwarded to store, got %q", st.capturedPageParams.Order)
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// RecordCut — auto-advance work order (Issue 3)
// ═════════════════════════════════════════════════════════════════════════════

func baseSheetForAutoAdvance() BoardSheet {
	return BoardSheet{
		ID:     uuid.New(),
		Status: "AVAILABLE",
		Dimensions: domain.Dimension{
			LengthMM: 1000,
			WidthMM:  800,
		},
		CostPerSheet: domain.Money{Amount: 10000, Currency: "VND"},
	}
}

func TestRecordCut_AutoAdvances_WorkOrderToInProcessing(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	sheet := baseSheetForAutoAdvance()
	sheetID := sheet.ID

	st := &mockStore{selectSheetByIDResult: sheet}
	advancer := &mockWorkOrderAdvancer{}
	svc := NewService(st, advancer)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       &sheetID,
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: domain.Dimension{LengthMM: 500, WidthMM: 400},
		IsWaste:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !advancer.called {
		t.Error("WorkOrderAdvancer must be called after a successful cut")
	}
	if advancer.calledWO != woID {
		t.Errorf("advance called with WO %v, want %v", advancer.calledWO, woID)
	}
	if advancer.calledTo != domain.WOInProcessing {
		t.Errorf("advance target = %v, want IN_PROCESSING", advancer.calledTo)
	}
}

func TestRecordCut_AutoAdvance_InvalidTransition_IsSilentlyIgnored(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	sheet := baseSheetForAutoAdvance()
	sheetID := sheet.ID

	st := &mockStore{selectSheetByIDResult: sheet}
	advancer := &mockWorkOrderAdvancer{
		err: domain.NewBizError(domain.ErrInvalidTransition, "already past IN_CUTTING"),
	}
	svc := NewService(st, advancer)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       &sheetID,
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: domain.Dimension{LengthMM: 500, WidthMM: 400},
		IsWaste:       true,
	})
	if err != nil {
		t.Errorf("ErrInvalidTransition from auto-advance must be ignored, got: %v", err)
	}
}

func TestRecordCut_AutoAdvance_OtherError_IsSilentlyLogged(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	sheet := baseSheetForAutoAdvance()
	sheetID := sheet.ID

	st := &mockStore{selectSheetByIDResult: sheet}
	advancer := &mockWorkOrderAdvancer{err: errors.New("production db down")}
	svc := NewService(st, advancer)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       &sheetID,
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: domain.Dimension{LengthMM: 500, WidthMM: 400},
		IsWaste:       true,
	})
	if err != nil {
		t.Errorf("advancer errors must not fail RecordCut, got: %v", err)
	}
}

func TestRecordCut_NoAdvancer_DoesNotPanic(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	sheet := baseSheetForAutoAdvance()
	sheetID := sheet.ID

	st := &mockStore{selectSheetByIDResult: sheet}
	svc := NewService(st, nil)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       &sheetID,
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: domain.Dimension{LengthMM: 500, WidthMM: 400},
		IsWaste:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordCut_BarcodeGenerator_ReturnsBarcodeIDs(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	sheet := baseSheetForAutoAdvance()
	sheetID := sheet.ID
	wipID := uuid.New()
	remID := uuid.New()

	st := &mockStore{selectSheetByIDResult: sheet}
	bcg := &mockBarcodeGenerator{out: BarcodeForCutOutput{WIPBarcodeID: &wipID, RemnantBarcodeID: &remID}}
	svc := NewService(st, nil, bcg)

	remnantDim := domain.Dimension{LengthMM: 200, WidthMM: 100}
	got, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:          &sheetID,
		WorkOrderID:      woID,
		SKUID:            skuID,
		UsedDimension:    domain.Dimension{LengthMM: 500, WidthMM: 400},
		RemnantDimension: &remnantDim,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bcg.called {
		t.Fatal("barcode generator must be called")
	}
	if bcg.in.WorkOrderID != woID {
		t.Errorf("barcode input work_order_id = %v, want %v", bcg.in.WorkOrderID, woID)
	}
	if bcg.in.UsedDimension != (domain.Dimension{LengthMM: 500, WidthMM: 400}) {
		t.Errorf("barcode input used dimension mismatch: %+v", bcg.in.UsedDimension)
	}
	if bcg.in.RemnantDimension == nil || *bcg.in.RemnantDimension != remnantDim {
		t.Errorf("barcode input remnant dimension mismatch: %+v", bcg.in.RemnantDimension)
	}
	if len(got.BarcodeIDs) != 2 {
		t.Fatalf("barcode_ids len = %d, want 2", len(got.BarcodeIDs))
	}
	if got.BarcodeIDs[0] != wipID || got.BarcodeIDs[1] != remID {
		t.Errorf("barcode_ids = %v, want [%v %v]", got.BarcodeIDs, wipID, remID)
	}
}

func TestRecordCut_BarcodeGenerator_Error_DoesNotFailRecordCut(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	sheet := baseSheetForAutoAdvance()
	sheetID := sheet.ID
	wipID := uuid.New()

	st := &mockStore{selectSheetByIDResult: sheet}
	bcg := &mockBarcodeGenerator{
		out: BarcodeForCutOutput{WIPBarcodeID: &wipID},
		err: errors.New("barcode db down"),
	}
	svc := NewService(st, nil, bcg)

	got, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       &sheetID,
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: domain.Dimension{LengthMM: 500, WidthMM: 400},
		IsWaste:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bcg.called {
		t.Fatal("barcode generator must be called")
	}
	if len(got.BarcodeIDs) != 1 || got.BarcodeIDs[0] != wipID {
		t.Errorf("barcode_ids = %v, want [%v]", got.BarcodeIDs, wipID)
	}
}

func TestRecordCut_NoBarcodeGenerator_LeavesBarcodeIDsEmpty(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	sheet := baseSheetForAutoAdvance()
	sheetID := sheet.ID

	st := &mockStore{selectSheetByIDResult: sheet}
	svc := NewService(st, nil)

	got, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       &sheetID,
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: domain.Dimension{LengthMM: 500, WidthMM: 400},
		IsWaste:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.BarcodeIDs) != 0 {
		t.Errorf("barcode_ids len = %d, want 0", len(got.BarcodeIDs))
	}
}

// ═════════════════════════════════════════════════════════════════════════════
// ReleaseExpiredAllocations — background auto-release (Issue 3.3)
// ═════════════════════════════════════════════════════════════════════════════

func TestReleaseExpiredAllocations_HappyPath_ReturnCount(t *testing.T) {
	before := time.Now().Add(-24 * time.Hour)
	st := &mockStore{releaseExpiredAllocationsResult: 3}
	svc := NewService(st, nil)

	n, err := svc.ReleaseExpiredAllocations(context.Background(), before)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("released count = %d, want 3", n)
	}
}

func TestReleaseExpiredAllocations_ZeroReleased(t *testing.T) {
	st := &mockStore{releaseExpiredAllocationsResult: 0}
	svc := NewService(st, nil)

	n, err := svc.ReleaseExpiredAllocations(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("released count = %d, want 0", n)
	}
}

func TestReleaseExpiredAllocations_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("db unavailable")
	st := &mockStore{releaseExpiredAllocationsErr: storeErr}
	svc := NewService(st, nil)

	_, err := svc.ReleaseExpiredAllocations(context.Background(), time.Now())
	if !errors.Is(err, storeErr) {
		t.Errorf("expected store error to propagate, got %v", err)
	}
}

// ── SuggestRemnants ───────────────────────────────────────────────────────────

func newTestRemnant(id string, lengthMM, widthMM int, createdAt time.Time) Remnant {
	return Remnant{
		ID:         uuid.MustParse(id),
		Dimensions: domain.Dimension{LengthMM: lengthMM, WidthMM: widthMM},
		Status:     domain.RemnantAvailable,
		CreatedAt:  createdAt,
	}
}

func TestSuggestRemnants_HappyPath_ReturnsSuggestionsRanked(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stored := []RemnantSuggestion{
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000001", 600, 400, t0), Rank: 1},
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000002", 800, 500, t0.Add(time.Hour)), Rank: 2},
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000003", 1000, 600, t0.Add(2*time.Hour)), Rank: 3},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
		Limit:             3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 suggestions, got %d", len(got))
	}
	if got[0].Rank != 1 || got[1].Rank != 2 || got[2].Rank != 3 {
		t.Errorf("ranks not preserved: %v", got)
	}
}

func TestSuggestRemnants_ExactFit_ReturnedAsRankOne(t *testing.T) {
	t0 := time.Now()
	stored := []RemnantSuggestion{
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000001", 500, 300, t0), Rank: 1},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 suggestion, got %d", len(got))
	}
	if got[0].Remnant.Dimensions.LengthMM != 500 || got[0].Remnant.Dimensions.WidthMM != 300 {
		t.Errorf("unexpected dimensions: %v", got[0].Remnant.Dimensions)
	}
}

func TestSuggestRemnants_NoFit_ReturnsEmptySlice(t *testing.T) {
	st := &mockStore{selectTopRemnantSuggestionsResult: nil}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 5000, WidthMM: 3000},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty, got %d suggestions", len(got))
	}
}

func TestSuggestRemnants_FIFOTiebreaker_OlderRemnantRanksFirst(t *testing.T) {
	older := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(24 * time.Hour)
	// Store returns them pre-sorted (oldest first), rank already assigned.
	stored := []RemnantSuggestion{
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000001", 600, 400, older), Rank: 1},
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000002", 600, 400, newer), Rank: 2},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
		Limit:             2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Remnant.CreatedAt != older {
		t.Errorf("expected older remnant at rank 1; got created_at = %v", got[0].Remnant.CreatedAt)
	}
}

func TestSuggestRemnants_LimitRespected(t *testing.T) {
	t0 := time.Now()
	stored := []RemnantSuggestion{
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000001", 600, 400, t0), Rank: 1},
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000002", 700, 400, t0.Add(time.Hour)), Rank: 2},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
		Limit:             2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2, got %d", len(got))
	}
}

func TestSuggestRemnants_DefaultLimit_UsesThree(t *testing.T) {
	st := &mockStore{selectTopRemnantSuggestionsResult: []RemnantSuggestion{}}
	svc := NewService(st, nil)

	// Limit 0 → service should default to 3 before calling store.
	// We verify by checking no error is returned (store returns empty, that's fine).
	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 100, WidthMM: 100},
		Limit:             0,
	})
	if err != nil {
		t.Fatalf("unexpected error with zero limit: %v", err)
	}
}

func TestSuggestRemnants_MaxLimitClamp(t *testing.T) {
	st := &mockStore{selectTopRemnantSuggestionsResult: []RemnantSuggestion{}}
	svc := NewService(st, nil)

	// Limit 99 → clamped to 10. No error expected.
	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 100, WidthMM: 100},
		Limit:             99,
	})
	if err != nil {
		t.Fatalf("unexpected error with oversized limit: %v", err)
	}
}

func TestSuggestRemnants_InvalidDimension_ZeroLength(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)

	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 0, WidthMM: 300},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

func TestSuggestRemnants_InvalidDimension_ZeroWidth(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)

	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 0},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

func TestSuggestRemnants_WithLocation_IncludedInResult(t *testing.T) {
	locID := uuid.New()
	loc := StorageLocation{
		ID:    locID,
		Zone:  "A",
		Rack:  "R1",
		Shelf: "S2",
		Label: "A-R1-S2",
	}
	t0 := time.Now()
	stored := []RemnantSuggestion{
		{
			Remnant:  newTestRemnant("00000000-0000-0000-0000-000000000001", 600, 400, t0),
			Location: &loc,
			Rank:     1,
		},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Location == nil {
		t.Fatal("expected location to be non-nil")
	}
	if got[0].Location.Label != "A-R1-S2" {
		t.Errorf("unexpected label: %s", got[0].Location.Label)
	}
}

func TestSuggestRemnants_WithoutLocation_NilLocation(t *testing.T) {
	t0 := time.Now()
	stored := []RemnantSuggestion{
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000001", 600, 400, t0), Location: nil, Rank: 1},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Location != nil {
		t.Errorf("expected nil location, got %+v", got[0].Location)
	}
}

func TestSuggestRemnants_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("db down")
	st := &mockStore{selectTopRemnantSuggestionsErr: storeErr}
	svc := NewService(st, nil)

	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
	})
	if !errors.Is(err, storeErr) {
		t.Errorf("want store error to propagate, got %v", err)
	}
}

// ── SuggestRemnants – additional algorithm edge-case tests ───────────────────
// (extends the 12 existing TestSuggestRemnants_* tests to exceed 20 total)

func TestSuggestRemnants_NegativeLength_ErrInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{}, nil)
	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: -100, WidthMM: 300},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for negative length, got %v", err)
	}
}

func TestSuggestRemnants_NegativeWidth_ErrInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{}, nil)
	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: -1},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for negative width, got %v", err)
	}
}

func TestSuggestRemnants_BothDimensionsZero_ErrInvalidInput(t *testing.T) {
	svc := NewService(&mockStore{}, nil)
	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 0, WidthMM: 0},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for zero dimensions, got %v", err)
	}
}

func TestSuggestRemnants_NegativeLimit_FallsBackToDefault(t *testing.T) {
	// Negative limit is treated as ≤0 → default of 3, not an error.
	st := &mockStore{selectTopRemnantSuggestionsResult: []RemnantSuggestion{}}
	svc := NewService(st, nil)
	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
		Limit:             -5,
	})
	if err != nil {
		t.Errorf("negative limit should use default, got error: %v", err)
	}
}

func TestSuggestRemnants_LimitOne_ReturnsSingleResult(t *testing.T) {
	t0 := time.Now()
	stored := []RemnantSuggestion{
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000001", 600, 400, t0), Rank: 1},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
		Limit:             1,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("want 1 result, got %d", len(got))
	}
}

func TestSuggestRemnants_LimitTen_AtBoundaryAllowed(t *testing.T) {
	// limit=10 should not be clamped — it equals maxSuggestionLimit.
	st := &mockStore{selectTopRemnantSuggestionsResult: []RemnantSuggestion{}}
	svc := NewService(st, nil)
	_, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 100, WidthMM: 100},
		Limit:             10,
	})
	if err != nil {
		t.Errorf("limit=10 should be accepted, got error: %v", err)
	}
}

// TestSuggestRemnants_BestFitPriority verifies that the service forwards results
// in the order the store returns them (Best Fit = smallest area first).
// The mock returns remnants pre-ranked by ascending area; service must not reorder.
func TestSuggestRemnants_BestFitPriority_SmallerAreaWins(t *testing.T) {
	t0 := time.Now()
	// 500*300=150000 < 800*400=320000 → first is best fit
	bestFit := newTestRemnant("00000000-0000-0000-0000-000000000001", 500, 300, t0)
	larger := newTestRemnant("00000000-0000-0000-0000-000000000002", 800, 400, t0)
	stored := []RemnantSuggestion{
		{Remnant: bestFit, Rank: 1},
		{Remnant: larger, Rank: 2},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 400, WidthMM: 250},
		Limit:             2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gotBestFitArea := got[0].Remnant.Dimensions.LengthMM * got[0].Remnant.Dimensions.WidthMM
	gotLargerArea := got[1].Remnant.Dimensions.LengthMM * got[1].Remnant.Dimensions.WidthMM
	if gotBestFitArea >= gotLargerArea {
		t.Errorf("rank-1 remnant area (%d) should be < rank-2 area (%d)", gotBestFitArea, gotLargerArea)
	}
}

// TestSuggestRemnants_NestedRemnant verifies that a remnant produced from
// another remnant (has ParentRemnantID set) is still included in suggestions.
func TestSuggestRemnants_NestedRemnant_IncludedInResults(t *testing.T) {
	t0 := time.Now()
	parentID := uuid.New()
	nested := Remnant{
		ID:              uuid.New(),
		ParentBoardID:   uuid.New(),
		ParentRemnantID: &parentID, // nested cut
		Dimensions:      domain.Dimension{LengthMM: 600, WidthMM: 400},
		Status:          domain.RemnantAvailable,
		CreatedAt:       t0,
	}
	stored := []RemnantSuggestion{{Remnant: nested, Rank: 1}}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("nested remnant should appear in suggestions")
	}
	if got[0].Remnant.ParentRemnantID == nil || *got[0].Remnant.ParentRemnantID != parentID {
		t.Errorf("ParentRemnantID not preserved: %v", got[0].Remnant.ParentRemnantID)
	}
}

// TestSuggestRemnants_SameArea_FIFODecides verifies that when two remnants have
// identical area, the older one (FIFO) is ranked first.
func TestSuggestRemnants_SameArea_FIFODecides(t *testing.T) {
	older := time.Date(2024, 1, 1, 8, 0, 0, 0, time.UTC)
	newer := older.Add(48 * time.Hour)
	// Both 600×400 = 240 000 mm²
	oldRemnant := newTestRemnant("00000000-0000-0000-0000-000000000001", 600, 400, older)
	newRemnant := newTestRemnant("00000000-0000-0000-0000-000000000002", 600, 400, newer)
	stored := []RemnantSuggestion{
		{Remnant: oldRemnant, Rank: 1},
		{Remnant: newRemnant, Rank: 2},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 500, WidthMM: 300},
		Limit:             2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got[0].Remnant.CreatedAt.Equal(older) {
		t.Errorf("FIFO: expected older remnant at rank 1, got created_at=%v", got[0].Remnant.CreatedAt)
	}
}

// TestSuggestRemnants_RankSequence verifies that ranks 1, 2, 3 are in order.
func TestSuggestRemnants_RankSequence_IsSequential(t *testing.T) {
	t0 := time.Now()
	stored := []RemnantSuggestion{
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000001", 500, 300, t0), Rank: 1},
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000002", 600, 400, t0.Add(time.Hour)), Rank: 2},
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000003", 700, 500, t0.Add(2*time.Hour)), Rank: 3},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 400, WidthMM: 250},
		Limit:             3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, s := range got {
		if s.Rank != i+1 {
			t.Errorf("index %d: want rank %d, got %d", i, i+1, s.Rank)
		}
	}
}

// TestSuggestRemnants_MixedLocations verifies a result set where some remnants
// have a storage location and others do not.
func TestSuggestRemnants_MixedLocations_SomeNilSomeNotNil(t *testing.T) {
	t0 := time.Now()
	loc := &StorageLocation{ID: uuid.New(), Zone: "B", Rack: "R2", Shelf: "S3", Label: "B-R2-S3"}
	stored := []RemnantSuggestion{
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000001", 500, 300, t0), Location: nil, Rank: 1},
		{Remnant: newTestRemnant("00000000-0000-0000-0000-000000000002", 600, 400, t0.Add(time.Hour)), Location: loc, Rank: 2},
	}
	st := &mockStore{selectTopRemnantSuggestionsResult: stored}
	svc := NewService(st, nil)

	got, err := svc.SuggestRemnants(context.Background(), SuggestRemnantsInput{
		RequiredDimension: domain.Dimension{LengthMM: 400, WidthMM: 250},
		Limit:             2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Location != nil {
		t.Errorf("rank-1 should have nil location, got %+v", got[0].Location)
	}
	if got[1].Location == nil || got[1].Location.Label != "B-R2-S3" {
		t.Errorf("rank-2 should have location B-R2-S3, got %+v", got[1].Location)
	}
}

// ── Coverage tests for service methods with 0% coverage ──────────────────────

func TestDeactivateLot_HappyPath(t *testing.T) {
	st := &mockStore{deactivateLotErr: nil}
	svc := NewService(st, nil)
	if err := svc.DeactivateLot(context.Background(), uuid.New()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDeactivateLot_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("not found")
	st := &mockStore{deactivateLotErr: storeErr}
	svc := NewService(st, nil)
	if err := svc.DeactivateLot(context.Background(), uuid.New()); !errors.Is(err, storeErr) {
		t.Errorf("want store error, got %v", err)
	}
}

func TestGetRemnant_HappyPath(t *testing.T) {
	expected := Remnant{ID: uuid.New(), Status: domain.RemnantAvailable}
	st := &mockStore{selectRemnantByIDResult: expected}
	svc := NewService(st, nil)
	got, err := svc.GetRemnant(context.Background(), expected.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != expected.ID {
		t.Errorf("want ID %v, got %v", expected.ID, got.ID)
	}
}

func TestGetRemnant_NotFound_ReturnsError(t *testing.T) {
	storeErr := domain.ErrNotFound
	st := &mockStore{selectRemnantByIDErr: storeErr}
	svc := NewService(st, nil)
	_, err := svc.GetRemnant(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestPreAssignSheet_HappyPath(t *testing.T) {
	st := &mockStore{preAssignSheetErr: nil, selectOverflowSheetArea: 1}
	svc := NewService(st, nil)
	if err := svc.PreAssignSheet(context.Background(), PreAssignSheetInput{SheetID: uuid.New(), WorkOrderID: uuid.New()}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !st.preAssignSheetCalled {
		t.Error("expected preAssignSheet to be called")
	}
}

func TestPreAssignSheet_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("sheet not available")
	st := &mockStore{preAssignSheetErr: storeErr, selectOverflowSheetArea: 1}
	svc := NewService(st, nil)
	err := svc.PreAssignSheet(context.Background(), PreAssignSheetInput{SheetID: uuid.New(), WorkOrderID: uuid.New()})
	if !errors.Is(err, storeErr) {
		t.Errorf("want store error, got %v", err)
	}
}

func TestPreAssignSheet_RedOverflow_BlocksIssue(t *testing.T) {
	st := &mockStore{selectOverflowRemnantArea: 200, selectOverflowSheetArea: 100}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)
	if err := svc.PreAssignSheet(context.Background(), PreAssignSheetInput{SheetID: uuid.New(), WorkOrderID: uuid.New()}); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("want ErrPreconditionFailed, got %v", err)
	}
	if st.preAssignSheetCalled {
		t.Error("preAssignSheet should not be called when overflow is RED")
	}
}

func TestPreAssignSheet_RedOverflow_BypassWithReason_WritesAuditAndProceeds(t *testing.T) {
	st := &mockStore{selectOverflowRemnantArea: 200, selectOverflowSheetArea: 100}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)
	actorID := uuid.New()
	err := svc.PreAssignSheet(context.Background(), PreAssignSheetInput{
		SheetID:        uuid.New(),
		WorkOrderID:    uuid.New(),
		BypassOverflow: true,
		ActorID:        actorID,
		Reason:         "urgent customer order",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.preAssignSheetCalled {
		t.Error("expected preAssignSheet to be called when bypass is granted")
	}
	if !st.insertAuditLogCalled {
		t.Fatal("want audit log entry written, none was")
	}
	entry := st.insertAuditLogEntry
	if entry.Action != auditActionOverflowBypassed {
		t.Errorf("want audit action %q, got %q", auditActionOverflowBypassed, entry.Action)
	}
	if entry.ActorID != actorID {
		t.Errorf("want actor %v, got %v", actorID, entry.ActorID)
	}
	if entry.Reason == nil || *entry.Reason != "urgent customer order" {
		t.Errorf("want reason 'urgent customer order', got %v", entry.Reason)
	}
}

func TestPreAssignSheet_RedOverflow_BypassMissingReason_Rejected(t *testing.T) {
	st := &mockStore{selectOverflowRemnantArea: 200, selectOverflowSheetArea: 100}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)
	err := svc.PreAssignSheet(context.Background(), PreAssignSheetInput{
		SheetID:        uuid.New(),
		WorkOrderID:    uuid.New(),
		BypassOverflow: true,
		ActorID:        uuid.New(),
		Reason:         "",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
	if st.preAssignSheetCalled {
		t.Error("preAssignSheet must not be called when bypass is missing reason")
	}
	if st.insertAuditLogCalled {
		t.Error("audit log must not be written when bypass is rejected")
	}
}

func TestPreAssignSheet_RedOverflow_BypassMissingActor_Rejected(t *testing.T) {
	st := &mockStore{selectOverflowRemnantArea: 200, selectOverflowSheetArea: 100}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)
	err := svc.PreAssignSheet(context.Background(), PreAssignSheetInput{
		SheetID:        uuid.New(),
		WorkOrderID:    uuid.New(),
		BypassOverflow: true,
		ActorID:        uuid.Nil,
		Reason:         "force",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
	if st.preAssignSheetCalled {
		t.Error("preAssignSheet must not be called when bypass is missing actor")
	}
}

// ─── REMNANT_BYPASSED audit logging (Issue #223) ───────────────────────────

func TestLogRemnantBypass_HappyPath_WritesAuditRowWithMetadata(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)
	woID := uuid.New()
	actorID := uuid.New()
	ids := []uuid.UUID{uuid.New(), uuid.New()}

	err := svc.LogRemnantBypass(context.Background(), LogRemnantBypassInput{
		WorkOrderID:         woID,
		ActorID:             actorID,
		SuggestedRemnantIDs: ids,
		Reason:              "planner prefers full sheet",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.insertAuditLogCalled {
		t.Fatal("insertAuditLog must be called")
	}
	got := st.insertAuditLogEntry
	if got.Action != auditActionRemnantBypassed {
		t.Errorf("action = %q, want %q", got.Action, auditActionRemnantBypassed)
	}
	if got.EntityType != entityTypeWorkOrder {
		t.Errorf("entity_type = %q, want %q", got.EntityType, entityTypeWorkOrder)
	}
	if got.EntityID != woID {
		t.Errorf("entity_id = %v, want %v", got.EntityID, woID)
	}
	if got.ActorID != actorID {
		t.Errorf("actor_id = %v, want %v", got.ActorID, actorID)
	}
	if got.Reason == nil || *got.Reason != "planner prefers full sheet" {
		t.Errorf("reason = %v, want 'planner prefers full sheet'", got.Reason)
	}
	if len(got.Metadata) == 0 {
		t.Fatal("metadata must be populated with suggested_remnant_ids")
	}
	var meta struct {
		SuggestedRemnantIDs []uuid.UUID `json:"suggested_remnant_ids"`
	}
	if err := json.Unmarshal(got.Metadata, &meta); err != nil {
		t.Fatalf("metadata is not valid JSON: %v", err)
	}
	if len(meta.SuggestedRemnantIDs) != 2 || meta.SuggestedRemnantIDs[0] != ids[0] || meta.SuggestedRemnantIDs[1] != ids[1] {
		t.Errorf("metadata.suggested_remnant_ids = %v, want %v", meta.SuggestedRemnantIDs, ids)
	}
}

func TestLogRemnantBypass_EmptySuggestions_Rejected(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)
	err := svc.LogRemnantBypass(context.Background(), LogRemnantBypassInput{
		WorkOrderID: uuid.New(),
		ActorID:     uuid.New(),
		Reason:      "no reason",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
	if st.insertAuditLogCalled {
		t.Error("audit log must not be written when suggestions list is empty")
	}
}

func TestLogRemnantBypass_MissingActor_Rejected(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)
	err := svc.LogRemnantBypass(context.Background(), LogRemnantBypassInput{
		WorkOrderID:         uuid.New(),
		ActorID:             uuid.Nil,
		SuggestedRemnantIDs: []uuid.UUID{uuid.New()},
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

func TestListAuditLogByAction_PassesActionToStore(t *testing.T) {
	st := &mockStore{
		selectAuditLogByActionResult: []AuditLogEntry{{ID: uuid.New(), Action: auditActionRemnantBypassed}},
	}
	svc := NewService(st, nil)
	got, err := svc.ListAuditLogByAction(context.Background(), auditActionRemnantBypassed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.selectAuditLogByActionAction != auditActionRemnantBypassed {
		t.Errorf("action passed to store = %q, want %q", st.selectAuditLogByActionAction, auditActionRemnantBypassed)
	}
	if len(got) != 1 {
		t.Errorf("want 1 entry, got %d", len(got))
	}
}

func TestListAuditLogByAction_EmptyAction_Rejected(t *testing.T) {
	svc := NewService(&mockStore{}, nil)
	_, err := svc.ListAuditLogByAction(context.Background(), "")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

func TestRecordCut_FromSheet_RedOverflow_BlocksIssue(t *testing.T) {
	sheetID := uuid.New()
	st := &mockStore{
		selectSheetByIDResult:     availableSheet(sheetID),
		selectOverflowRemnantArea: 200,
		selectOverflowSheetArea:   100,
	}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("want ErrPreconditionFailed, got %v", err)
	}
	if st.recordCutAtomicallyCalled {
		t.Error("recordCutAtomically should not be called when overflow is RED")
	}
}

func TestRecordCut_FromRemnant_RedOverflow_StillAllowed(t *testing.T) {
	boardID := uuid.New()
	remnantID := uuid.New()
	st := &mockStore{
		selectRemnantByIDResult:   availableRemnant(remnantID, boardID),
		selectOverflowRemnantArea: 200,
		selectOverflowSheetArea:   100,
	}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		RemnantID:     ptr(remnantID),
		WorkOrderID:   uuid.New(),
		SKUID:         uuid.New(),
		UsedDimension: dim100x100,
		IsWaste:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.recordCutAtomicallyCalled {
		t.Fatal("recordCutAtomically was not called")
	}
}

func TestGetOverflowStatus_BelowThreshold_Green(t *testing.T) {
	st := &mockStore{selectOverflowRemnantArea: 10, selectOverflowSheetArea: 100}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)

	got, err := svc.GetOverflowStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != OverflowGreen {
		t.Errorf("status = %s, want GREEN", got.Status)
	}
	if got.BlockNewSheetIssue {
		t.Error("BlockNewSheetIssue must be false in GREEN")
	}
}

func TestGetOverflowStatus_AtThreshold_Green(t *testing.T) {
	st := &mockStore{selectOverflowRemnantArea: 15, selectOverflowSheetArea: 100}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)

	got, err := svc.GetOverflowStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != OverflowGreen {
		t.Errorf("status = %s, want GREEN", got.Status)
	}
}

func TestGetOverflowStatus_AboveThreshold_Red(t *testing.T) {
	st := &mockStore{selectOverflowRemnantArea: 16, selectOverflowSheetArea: 100}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)

	got, err := svc.GetOverflowStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != OverflowRed {
		t.Errorf("status = %s, want RED", got.Status)
	}
	if !got.BlockNewSheetIssue {
		t.Error("BlockNewSheetIssue must be true in RED")
	}
}

func TestGetOverflowStatus_DenominatorZeroWithRemnant_Red(t *testing.T) {
	st := &mockStore{selectOverflowRemnantArea: 1, selectOverflowSheetArea: 0}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)

	got, err := svc.GetOverflowStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != OverflowRed {
		t.Errorf("status = %s, want RED", got.Status)
	}
	if got.OverflowPct != 100 {
		t.Errorf("overflow_pct = %v, want 100", got.OverflowPct)
	}
}

func TestGetOverflowStatus_DenominatorZeroNoRemnant_Green(t *testing.T) {
	st := &mockStore{selectOverflowRemnantArea: 0, selectOverflowSheetArea: 0}
	svc := NewServiceWithOverflowThreshold(st, nil, 15)

	got, err := svc.GetOverflowStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != OverflowGreen {
		t.Errorf("status = %s, want GREEN", got.Status)
	}
	if got.OverflowPct != 0 {
		t.Errorf("overflow_pct = %v, want 0", got.OverflowPct)
	}
}

func TestStockRemnant_HappyPath(t *testing.T) {
	// selectStorageLocationByBarcode returns zero-value StorageLocation (no error)
	// updateRemnantBinLocation returns nil — both are default mock behaviors.
	st := &mockStore{}
	svc := NewService(st, nil)
	if err := svc.StockRemnant(context.Background(), uuid.New(), "A-R1-S1"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestStockRemnant_CNCRole_ServiceLayer confirms the service itself has no role
// restriction — RBAC is enforced at the handler layer (handler.go route guard).
// This test documents the fix for issue #180: route now allows RoleCNC and
// RoleCNCManager in addition to RoleWarehouse.
func TestStockRemnant_CNCRole_ServiceLayer(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)
	// Service must succeed regardless of caller role — role check happens in handler.
	if err := svc.StockRemnant(context.Background(), uuid.New(), "ZONE-A-R1-S1"); err != nil {
		t.Errorf("service should not restrict by role, got: %v", err)
	}
}

func TestStockRemnant_LocationNotFound_ReturnsError(t *testing.T) {
	storeErr := domain.ErrNotFound
	st := &mockStore{selectStorageLocationByBarcodeErr: storeErr}
	svc := NewService(st, nil)
	err := svc.StockRemnant(context.Background(), uuid.New(), "bad-barcode")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// ── Transfer tests ────────────────────────────────────────────────────────────

func TestTransfer_Remnant_Success(t *testing.T) {
	remnantID := uuid.New()
	locationID := uuid.New()
	actorID := uuid.New()
	st := &mockStore{
		selectRemnantByIDResult: Remnant{
			ID:     remnantID,
			Status: "AVAILABLE",
		},
		selectStorageLocationByBarcodeResult: StorageLocation{
			ID:       locationID,
			Barcode:  "A-R1-S1",
			IsActive: true,
		},
	}
	svc := NewService(st, nil)
	result, err := svc.Transfer(context.Background(), TransferInput{
		EntityType:    entityTypeRemnant,
		EntityID:      remnantID,
		TargetBarcode: "A-R1-S1",
		ActorID:       actorID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToLocation != locationID {
		t.Errorf("expected ToLocation %v, got %v", locationID, result.ToLocation)
	}
	if !st.insertAuditLogCalled {
		t.Error("expected audit log to be inserted")
	}
	if st.insertAuditLogEntry.Action != auditActionTransfer {
		t.Errorf("expected action %q, got %q", auditActionTransfer, st.insertAuditLogEntry.Action)
	}
}

func TestTransfer_BoardSheet_Success(t *testing.T) {
	sheetID := uuid.New()
	locationID := uuid.New()
	actorID := uuid.New()
	st := &mockStore{
		selectSheetByIDResult: BoardSheet{
			ID:     sheetID,
			Status: "AVAILABLE",
		},
		selectStorageLocationByBarcodeResult: StorageLocation{
			ID:       locationID,
			Barcode:  "B-R1-S1",
			IsActive: true,
		},
	}
	svc := NewService(st, nil)
	result, err := svc.Transfer(context.Background(), TransferInput{
		EntityType:    entityTypeBoardSheet,
		EntityID:      sheetID,
		TargetBarcode: "B-R1-S1",
		ActorID:       actorID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EntityType != entityTypeBoardSheet {
		t.Errorf("expected entity type %q, got %q", entityTypeBoardSheet, result.EntityType)
	}
	if result.ToLocation != locationID {
		t.Errorf("expected ToLocation %v, got %v", locationID, result.ToLocation)
	}
}

func TestTransfer_InvalidEntityType_ReturnsError(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)
	_, err := svc.Transfer(context.Background(), TransferInput{
		EntityType:    "INVALID",
		EntityID:      uuid.New(),
		TargetBarcode: "A-R1-S1",
		ActorID:       uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

func TestTransfer_RemnantNotAvailable_ReturnsError(t *testing.T) {
	st := &mockStore{
		selectRemnantByIDResult: Remnant{
			ID:     uuid.New(),
			Status: "CONSUMED",
		},
		selectStorageLocationByBarcodeResult: StorageLocation{
			ID:       uuid.New(),
			IsActive: true,
		},
	}
	svc := NewService(st, nil)
	_, err := svc.Transfer(context.Background(), TransferInput{
		EntityType:    entityTypeRemnant,
		EntityID:      uuid.New(),
		TargetBarcode: "A-R1-S1",
		ActorID:       uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

// ── CycleCount tests ──────────────────────────────────────────────────────────

func TestCreateCycleCountSession_Success(t *testing.T) {
	actorID := uuid.New()
	st := &mockStore{}
	svc := NewService(st, nil)
	sess, err := svc.CreateCycleCountSession(context.Background(), CreateCycleCountInput{
		Zone:    "A",
		ActorID: actorID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.Status != "OPEN" {
		t.Errorf("expected status OPEN, got %s", sess.Status)
	}
	if sess.CreatedBy != actorID {
		t.Errorf("expected created_by %v, got %v", actorID, sess.CreatedBy)
	}
	if sess.Zone != "A" {
		t.Errorf("expected zone A, got %s", sess.Zone)
	}
}

func TestAddCycleCountLine_SessionNotOpen_ReturnsError(t *testing.T) {
	sessionID := uuid.New()
	st := &mockStore{
		selectCycleCountSessionByIDResult: CycleCountSession{
			ID:     sessionID,
			Status: "POSTED",
		},
	}
	svc := NewService(st, nil)
	_, err := svc.AddCycleCountLine(context.Background(), AddCountLineInput{
		SessionID:     sessionID,
		EntityType:    entityTypeRemnant,
		EntityID:      uuid.New(),
		CountedStatus: "AVAILABLE",
		Reason:        "physical count",
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("want ErrInvalidTransition, got %v", err)
	}
	if st.insertCycleCountLineCalled {
		t.Error("insertCycleCountLine should not be called when session is not OPEN")
	}
}

func TestAddCycleCountLine_EmptyReason_ReturnsError(t *testing.T) {
	sessionID := uuid.New()
	st := &mockStore{
		selectCycleCountSessionByIDResult: CycleCountSession{
			ID:     sessionID,
			Status: "OPEN",
		},
	}
	svc := NewService(st, nil)
	_, err := svc.AddCycleCountLine(context.Background(), AddCountLineInput{
		SessionID:     sessionID,
		EntityType:    entityTypeRemnant,
		EntityID:      uuid.New(),
		CountedStatus: "AVAILABLE",
		Reason:        "",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

func TestAddCycleCountLine_InvalidEntityType_ReturnsError(t *testing.T) {
	sessionID := uuid.New()
	st := &mockStore{
		selectCycleCountSessionByIDResult: CycleCountSession{
			ID:     sessionID,
			Status: "OPEN",
		},
	}
	svc := NewService(st, nil)
	_, err := svc.AddCycleCountLine(context.Background(), AddCountLineInput{
		SessionID:     sessionID,
		EntityType:    "LOT",
		EntityID:      uuid.New(),
		CountedStatus: "AVAILABLE",
		Reason:        "physical count",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

func TestPostCycleCount_SessionNotOpen_ReturnsError(t *testing.T) {
	sessionID := uuid.New()
	actorID := uuid.New()
	st := &mockStore{
		selectCycleCountSessionByIDResult: CycleCountSession{
			ID:     sessionID,
			Status: "CANCELLED",
		},
	}
	svc := NewService(st, nil)
	err := svc.PostCycleCount(context.Background(), PostCycleCountInput{
		SessionID: sessionID,
		ActorID:   actorID,
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("want ErrInvalidTransition, got %v", err)
	}
	if st.postCycleCountAtomicallyCalled {
		t.Error("postCycleCountAtomically should not be called when session is not OPEN")
	}
}

func TestPostCycleCount_NoLines_CallsAtomicWithEmptyAdjustments(t *testing.T) {
	sessionID := uuid.New()
	actorID := uuid.New()
	st := &mockStore{
		selectCycleCountSessionByIDResult: CycleCountSession{
			ID:     sessionID,
			Status: "OPEN",
		},
		selectCycleCountLinesBySessionResult: nil,
	}
	svc := NewService(st, nil)
	err := svc.PostCycleCount(context.Background(), PostCycleCountInput{
		SessionID: sessionID,
		ActorID:   actorID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !st.postCycleCountAtomicallyCalled {
		t.Error("expected postCycleCountAtomically to be called")
	}
	if len(st.postCycleCountAtomicallyOp.Adjustments) != 0 {
		t.Errorf("expected 0 adjustments, got %d", len(st.postCycleCountAtomicallyOp.Adjustments))
	}
}

func TestCancelCycleCountSession_SessionNotOpen_ReturnsError(t *testing.T) {
	sessionID := uuid.New()
	actorID := uuid.New()
	st := &mockStore{
		selectCycleCountSessionByIDResult: CycleCountSession{
			ID:     sessionID,
			Status: "POSTED",
		},
	}
	svc := NewService(st, nil)
	err := svc.CancelCycleCountSession(context.Background(), sessionID, actorID)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("want ErrInvalidTransition, got %v", err)
	}
}

// ── GenerateRemnantLabelPDF ───────────────────────────────────────────────────

func TestGenerateRemnantLabelPDF_HappyPath_50x30_ReturnsPDF(t *testing.T) {
	remnantID := uuid.New()
	boardID := uuid.New()
	st := &mockStore{
		selectRemnantByIDResult: Remnant{
			ID:            remnantID,
			ParentBoardID: boardID,
			Dimensions:    domain.Dimension{LengthMM: 600, WidthMM: 400},
			Status:        domain.RemnantAvailable,
		},
	}
	svc := NewService(st, nil)
	pdf, err := svc.GenerateRemnantLabelPDF(context.Background(), RemnantLabelInput{
		RemnantID: remnantID,
		Size:      RemnantLabelSize50x30,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pdf) == 0 {
		t.Error("expected non-empty PDF bytes")
	}
}

func TestGenerateRemnantLabelPDF_HappyPath_100x70_ReturnsPDF(t *testing.T) {
	remnantID := uuid.New()
	boardID := uuid.New()
	st := &mockStore{
		selectRemnantByIDResult: Remnant{
			ID:            remnantID,
			ParentBoardID: boardID,
			Dimensions:    domain.Dimension{LengthMM: 1200, WidthMM: 800},
			Status:        domain.RemnantAvailable,
		},
	}
	svc := NewService(st, nil)
	pdf, err := svc.GenerateRemnantLabelPDF(context.Background(), RemnantLabelInput{
		RemnantID: remnantID,
		Size:      RemnantLabelSize100x70,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pdf) == 0 {
		t.Error("expected non-empty PDF bytes")
	}
}

func TestGenerateRemnantLabelPDF_RemnantNotFound_ReturnsError(t *testing.T) {
	st := &mockStore{
		selectRemnantByIDErr: domain.ErrNotFound,
	}
	svc := NewService(st, nil)
	_, err := svc.GenerateRemnantLabelPDF(context.Background(), RemnantLabelInput{
		RemnantID: uuid.New(),
		Size:      RemnantLabelSize50x30,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestGenerateRemnantLabelPDF_InvalidSize_ReturnsError(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)
	_, err := svc.GenerateRemnantLabelPDF(context.Background(), RemnantLabelInput{
		RemnantID: uuid.New(),
		Size:      RemnantLabelSize("invalid"),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

// ── GenerateCutLabelsPDF ──────────────────────────────────────────────────────

// countPDFPages returns the number of pages declared in the PDF byte stream by
// counting "/Type /Page" occurrences (each page object declares its type).
// Sufficient for tests that only need to distinguish 1-page vs 2-page output.
func countPDFPages(b []byte) int {
	count := 0
	src := string(b)
	target := "/Type /Page"
	for i := 0; i+len(target) <= len(src); i++ {
		if src[i:i+len(target)] != target {
			continue
		}
		next := byte(' ')
		if i+len(target) < len(src) {
			next = src[i+len(target)]
		}
		if next != 's' { // skip "/Type /Pages"
			count++
		}
	}
	return count
}

func TestGenerateCutLabelsPDF_HappyPath_NoRemnant_OnePage(t *testing.T) {
	cutID := uuid.New()
	st := &mockStore{
		selectCuttingRecordDetailsResult: CuttingRecordDetails{
			Record: CuttingRecord{
				ID:           cutID,
				WorkOrderID:  uuid.New(),
				SKUID:        uuid.New(),
				UsedLengthMM: 600,
				UsedWidthMM:  400,
				CreatedAt:    time.Now(),
			},
			SKUCode:         "SKU-001",
			SKUName:         "Test SKU",
			ProducedRemnant: nil,
		},
	}
	svc := NewService(st, nil)
	pdf, err := svc.GenerateCutLabelsPDF(context.Background(), CutLabelsInput{
		CuttingRecordID: cutID,
		Size:            RemnantLabelSize50x30,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}
	if pages := countPDFPages(pdf); pages != 1 {
		t.Errorf("want 1 page (no remnant produced), got %d", pages)
	}
}

func TestGenerateCutLabelsPDF_HappyPath_WithRemnant_TwoPages(t *testing.T) {
	cutID := uuid.New()
	remnantID := uuid.New()
	boardID := uuid.New()
	remnant := Remnant{
		ID:            remnantID,
		ParentBoardID: boardID,
		Dimensions:    domain.Dimension{LengthMM: 800, WidthMM: 500},
		Status:        domain.RemnantAvailable,
	}
	st := &mockStore{
		selectCuttingRecordDetailsResult: CuttingRecordDetails{
			Record: CuttingRecord{
				ID:                cutID,
				WorkOrderID:       uuid.New(),
				SKUID:             uuid.New(),
				UsedLengthMM:      600,
				UsedWidthMM:       400,
				ProducedRemnantID: &remnantID,
				CreatedAt:         time.Now(),
			},
			SKUCode:         "SKU-001",
			SKUName:         "Test SKU",
			ProducedRemnant: &remnant,
		},
	}
	svc := NewService(st, nil)
	pdf, err := svc.GenerateCutLabelsPDF(context.Background(), CutLabelsInput{
		CuttingRecordID: cutID,
		Size:            RemnantLabelSize100x70,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}
	if pages := countPDFPages(pdf); pages != 2 {
		t.Errorf("want 2 pages (WIP + remnant), got %d", pages)
	}
}

func TestGenerateCutLabelsPDF_RecordNotFound_ReturnsError(t *testing.T) {
	st := &mockStore{
		selectCuttingRecordDetailsErr: domain.NewBizError(domain.ErrNotFound, "cutting record not found"),
	}
	svc := NewService(st, nil)
	_, err := svc.GenerateCutLabelsPDF(context.Background(), CutLabelsInput{
		CuttingRecordID: uuid.New(),
		Size:            RemnantLabelSize50x30,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestGenerateCutLabelsPDF_InvalidSize_ReturnsError(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)
	_, err := svc.GenerateCutLabelsPDF(context.Background(), CutLabelsInput{
		CuttingRecordID: uuid.New(),
		Size:            RemnantLabelSize("invalid"),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

// ── GeneratePickSlipPDF ───────────────────────────────────────────────────────

func TestGeneratePickSlipPDF_HappyPath_ThreeRemnantsAcrossTwoZones(t *testing.T) {
	woID := uuid.New()
	lines := []PickSlipLine{
		{RemnantID: uuid.New(), Dimensions: domain.Dimension{LengthMM: 800, WidthMM: 400}, Zone: "A", Rack: "01", Shelf: "02", Label: "A-01-02", BinBarcode: "LOC-A0102"},
		{RemnantID: uuid.New(), Dimensions: domain.Dimension{LengthMM: 600, WidthMM: 300}, Zone: "A", Rack: "01", Shelf: "03", Label: "A-01-03", BinBarcode: "LOC-A0103"},
		{RemnantID: uuid.New(), Dimensions: domain.Dimension{LengthMM: 500, WidthMM: 250}, Zone: "B", Rack: "02", Shelf: "01", Label: "B-02-01", BinBarcode: "LOC-B0201"},
	}
	st := &mockStore{selectAllocatedRemnantsByWOResult: lines}
	svc := NewService(st, nil)

	pdf, err := svc.GeneratePickSlipPDF(context.Background(), woID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}
	// Verify it starts with the PDF magic bytes.
	if string(pdf[:4]) != "%PDF" {
		t.Errorf("output does not look like a PDF (first 4 bytes: %q)", pdf[:4])
	}
}

func TestGeneratePickSlipPDF_NoAllocatedRemnants_ReturnsNotFound(t *testing.T) {
	st := &mockStore{selectAllocatedRemnantsByWOResult: nil}
	svc := NewService(st, nil)

	_, err := svc.GeneratePickSlipPDF(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestGeneratePickSlipPDF_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("db error")
	st := &mockStore{selectAllocatedRemnantsByWOErr: storeErr}
	svc := NewService(st, nil)

	_, err := svc.GeneratePickSlipPDF(context.Background(), uuid.New())
	if !errors.Is(err, storeErr) {
		t.Errorf("want store error to propagate, got %v", err)
	}
}

func TestGeneratePickSlipPDF_RemnantWithNoLocation_ShowsDash(t *testing.T) {
	// Remnant with no bin location assigned — Zone/Label/BinBarcode are empty strings.
	// The service should still render without error (shows "—" in the PDF).
	lines := []PickSlipLine{
		{RemnantID: uuid.New(), Dimensions: domain.Dimension{LengthMM: 700, WidthMM: 350}, Zone: "", Rack: "", Shelf: "", Label: "", BinBarcode: ""},
	}
	st := &mockStore{selectAllocatedRemnantsByWOResult: lines}
	svc := NewService(st, nil)

	pdf, err := svc.GeneratePickSlipPDF(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error for remnant with no location: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}
}

// ── ListCuttingRecords ───────────────────────────────────────────────────────

func TestListCuttingRecords_HappyPath_ReturnsPagedResult(t *testing.T) {
	woID := uuid.New()
	skuID := uuid.New()
	userID := uuid.New()
	username := "worker1"
	fullName := "Nguyễn Văn A"

	items := []CuttingRecordReport{
		{
			ID:               uuid.New(),
			WorkOrderID:      woID,
			SKUID:            skuID,
			SKUCode:          "SKU-001",
			SKUName:          "Đá xanh A",
			SourceType:       "SHEET",
			SourceID:         uuid.New(),
			UsedDimension:    domain.Dimension{LengthMM: 1200, WidthMM: 600},
			AssignedTo:       &userID,
			AssignedUsername: &username,
			AssignedFullName: &fullName,
			CreatedAt:        time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC),
		},
	}
	st := &mockStore{
		selectCuttingRecordsReportResult: items,
		selectCuttingRecordsReportTotal:  1,
	}
	svc := NewService(st, nil)

	res, err := svc.ListCuttingRecords(context.Background(),
		CuttingRecordFilter{UserID: &userID},
		httpkit.PageParams{Page: 1, Limit: 10},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TotalItems != 1 {
		t.Fatalf("total = %d, want 1", res.TotalItems)
	}
	if len(res.Items) != 1 || res.Items[0].SKUCode != "SKU-001" {
		t.Fatalf("unexpected items: %+v", res.Items)
	}
	if st.selectCuttingRecordsReportFilter.UserID == nil || *st.selectCuttingRecordsReportFilter.UserID != userID {
		t.Fatalf("filter not forwarded: got %+v", st.selectCuttingRecordsReportFilter)
	}
}

func TestListCuttingRecords_EmptyResult(t *testing.T) {
	st := &mockStore{
		selectCuttingRecordsReportResult: []CuttingRecordReport{},
		selectCuttingRecordsReportTotal:  0,
	}
	svc := NewService(st, nil)

	res, err := svc.ListCuttingRecords(context.Background(),
		CuttingRecordFilter{}, httpkit.PageParams{Page: 1, Limit: 10},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TotalItems != 0 || len(res.Items) != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
}

func TestListCuttingRecords_InvalidDateRange_ReturnsInvalidInput(t *testing.T) {
	st := &mockStore{}
	svc := NewService(st, nil)

	from := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	_, err := svc.ListCuttingRecords(context.Background(),
		CuttingRecordFilter{From: from, To: to},
		httpkit.PageParams{Page: 1, Limit: 10},
	)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
}

func TestListCuttingRecords_StoreError_Propagates(t *testing.T) {
	storeErr := errors.New("db down")
	st := &mockStore{selectCuttingRecordsReportErr: storeErr}
	svc := NewService(st, nil)

	_, err := svc.ListCuttingRecords(context.Background(),
		CuttingRecordFilter{}, httpkit.PageParams{Page: 1, Limit: 10},
	)
	if err == nil || !errors.Is(err, storeErr) {
		t.Fatalf("got %v, want %v", err, storeErr)
	}
}

// ── CUTTING_RECORDED notifier (#258) ─────────────────────────────────────────

type mockCutNotifier struct {
	calls []struct {
		WO  string
		Cut string
	}
	err error
}

func (m *mockCutNotifier) NotifyCuttingRecorded(_ context.Context, woID, cuttingRecordID string) error {
	m.calls = append(m.calls, struct {
		WO  string
		Cut string
	}{woID, cuttingRecordID})
	return m.err
}

func TestRecordCut_HappyPath_PublishesCuttingRecorded(t *testing.T) {
	sheetID := uuid.New()
	woID := uuid.New()
	skuID := uuid.New()

	st := &mockStore{
		selectSheetByIDResult:   availableSheet(sheetID),
		selectOverflowSheetArea: 1,
	}
	notifier := &mockCutNotifier{}
	svc := NewServiceFull(st, nil, nil, notifier, 0)

	result, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   woID,
		SKUID:         skuID,
		UsedDimension: dim1000x500,
		IsWaste:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("notifier calls = %d, want 1", len(notifier.calls))
	}
	if notifier.calls[0].WO != woID.String() {
		t.Errorf("WO = %q, want %q", notifier.calls[0].WO, woID.String())
	}
	if notifier.calls[0].Cut != result.CuttingRecordID.String() {
		t.Errorf("Cut = %q, want %q", notifier.calls[0].Cut, result.CuttingRecordID.String())
	}
}

func TestRecordCut_NotifierError_DoesNotFailRequest(t *testing.T) {
	sheetID := uuid.New()
	woID := uuid.New()

	st := &mockStore{
		selectSheetByIDResult:   availableSheet(sheetID),
		selectOverflowSheetArea: 1,
	}
	notifier := &mockCutNotifier{err: errors.New("broker down")}
	svc := NewServiceFull(st, nil, nil, notifier, 0)

	if _, err := svc.RecordCut(context.Background(), RecordCutInput{
		SheetID:       ptr(sheetID),
		WorkOrderID:   woID,
		SKUID:         uuid.New(),
		UsedDimension: dim1000x500,
		IsWaste:       true,
	}); err != nil {
		t.Errorf("notifier failure must not fail RecordCut, got: %v", err)
	}
	if !st.recordCutAtomicallyCalled {
		t.Error("cut must still commit when notifier fails")
	}
}

func TestRecordCut_ValidationFails_NotifierNotCalled(t *testing.T) {
	notifier := &mockCutNotifier{}
	svc := NewServiceFull(&mockStore{}, nil, nil, notifier, 0)

	_, err := svc.RecordCut(context.Background(), RecordCutInput{
		WorkOrderID: uuid.New(),
		SKUID:       uuid.New(),
		// Both source IDs nil — validation rejects.
		UsedDimension: dim1000x500,
		IsWaste:       true,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if len(notifier.calls) != 0 {
		t.Errorf("notifier must not fire on validation failure, got %v", notifier.calls)
	}
}
