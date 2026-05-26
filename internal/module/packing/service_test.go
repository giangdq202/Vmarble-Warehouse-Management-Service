package packing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── State-aware mock harness ────────────────────────────────────────────────

type mockStore struct {
	fgsByID        map[uuid.UUID]FGPool
	fgsByBarcode   map[uuid.UUID]uuid.UUID // barcode -> fg_id
	fgsByWO        map[uuid.UUID][]uuid.UUID
	defectsByID    map[uuid.UUID]FGDefect
	defectsByFG    map[uuid.UUID]uuid.UUID

	insertBatchErr error
}

func newMockStore() *mockStore {
	return &mockStore{
		fgsByID:      map[uuid.UUID]FGPool{},
		fgsByBarcode: map[uuid.UUID]uuid.UUID{},
		fgsByWO:      map[uuid.UUID][]uuid.UUID{},
		defectsByID:  map[uuid.UUID]FGDefect{},
		defectsByFG:  map[uuid.UUID]uuid.UUID{},
	}
}

func (m *mockStore) insertFGBatch(_ context.Context, rows []FGPool) error {
	if m.insertBatchErr != nil {
		return m.insertBatchErr
	}
	for _, r := range rows {
		m.fgsByID[r.ID] = r
		m.fgsByBarcode[r.BarcodeID] = r.ID
		m.fgsByWO[r.WorkOrderID] = append(m.fgsByWO[r.WorkOrderID], r.ID)
	}
	return nil
}

func (m *mockStore) selectFGByID(_ context.Context, id uuid.UUID) (FGPool, error) {
	fg, ok := m.fgsByID[id]
	if !ok {
		return FGPool{}, domain.ErrNotFound
	}
	return fg, nil
}

func (m *mockStore) selectFGByBarcodeID(_ context.Context, barcodeID uuid.UUID) (FGPool, error) {
	id, ok := m.fgsByBarcode[barcodeID]
	if !ok {
		return FGPool{}, domain.ErrNotFound
	}
	return m.fgsByID[id], nil
}

func (m *mockStore) selectFGByWorkOrderID(_ context.Context, woID uuid.UUID) ([]FGPool, error) {
	ids := m.fgsByWO[woID]
	out := make([]FGPool, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.fgsByID[id])
	}
	return out, nil
}

func (m *mockStore) selectFGPaged(_ context.Context, _ httpkit.PageParams, _ FGListFilter) ([]FGPool, int, error) {
	out := make([]FGPool, 0, len(m.fgsByID))
	for _, fg := range m.fgsByID {
		out = append(out, fg)
	}
	return out, len(out), nil
}

func (m *mockStore) selectDefectByID(_ context.Context, id uuid.UUID) (FGDefect, error) {
	d, ok := m.defectsByID[id]
	if !ok {
		return FGDefect{}, domain.ErrNotFound
	}
	return d, nil
}

func (m *mockStore) selectDefectByFGID(_ context.Context, fgID uuid.UUID) (FGDefect, error) {
	id, ok := m.defectsByFG[fgID]
	if !ok {
		return FGDefect{}, domain.ErrNotFound
	}
	return m.defectsByID[id], nil
}

func (m *mockStore) withTx(ctx context.Context, fn func(tx txStore) error) error {
	return fn(&mockTxStore{ms: m})
}

type mockTxStore struct {
	ms *mockStore
}

func (t *mockTxStore) rawTx() pgx.Tx { return nil }

func (t *mockTxStore) lockFGForUpdate(_ context.Context, id uuid.UUID) (FGPool, error) {
	fg, ok := t.ms.fgsByID[id]
	if !ok {
		return FGPool{}, domain.ErrNotFound
	}
	return fg, nil
}

func (t *mockTxStore) lockFGByBarcodeForUpdate(_ context.Context, barcodeID uuid.UUID) (FGPool, error) {
	id, ok := t.ms.fgsByBarcode[barcodeID]
	if !ok {
		return FGPool{}, domain.ErrNotFound
	}
	return t.ms.fgsByID[id], nil
}

func (t *mockTxStore) lockAvailableFGsForReserve(_ context.Context, skuID, soLineID uuid.UUID, qty int) ([]FGPool, error) {
	var out []FGPool
	for _, fg := range t.ms.fgsByID {
		if fg.Status != FGStatusAvailable {
			continue
		}
		if fg.SKUID != skuID {
			continue
		}
		if fg.SalesOrderLineID == nil || *fg.SalesOrderLineID != soLineID {
			continue
		}
		out = append(out, fg)
		if len(out) == qty {
			break
		}
	}
	return out, nil
}

func (t *mockTxStore) lockReservedFGsByContainerLine(_ context.Context, containerLineID uuid.UUID) ([]FGPool, error) {
	var out []FGPool
	for _, fg := range t.ms.fgsByID {
		if fg.Status != FGStatusReserved && fg.Status != FGStatusLoaded {
			continue
		}
		if fg.ContainerLineID == nil || *fg.ContainerLineID != containerLineID {
			continue
		}
		out = append(out, fg)
	}
	return out, nil
}

func (t *mockTxStore) lockReservedFGsByContainer(_ context.Context, _ uuid.UUID) ([]FGPool, error) {
	var out []FGPool
	for _, fg := range t.ms.fgsByID {
		if fg.Status == FGStatusReserved {
			out = append(out, fg)
		}
	}
	return out, nil
}

func (t *mockTxStore) flipFGStatus(_ context.Context, in flipStatusInput) error {
	fg, ok := t.ms.fgsByID[in.FGID]
	if !ok {
		return domain.ErrNotFound
	}
	fg.Status = in.ToStatus
	fg.ContainerLineID = in.ContainerLineID
	t.ms.fgsByID[in.FGID] = fg
	return nil
}

func (t *mockTxStore) bulkFlipFGStatus(_ context.Context, ids []uuid.UUID, toStatus string, containerLineID *uuid.UUID) error {
	for _, id := range ids {
		fg, ok := t.ms.fgsByID[id]
		if !ok {
			continue
		}
		fg.Status = toStatus
		fg.ContainerLineID = containerLineID
		t.ms.fgsByID[id] = fg
	}
	return nil
}

func (t *mockTxStore) insertDefect(_ context.Context, d FGDefect) error {
	t.ms.defectsByID[d.ID] = d
	t.ms.defectsByFG[d.FGPoolID] = d.ID
	return nil
}

func (t *mockTxStore) updateDefectResolution(_ context.Context, in updateResolutionInput) error {
	d, ok := t.ms.defectsByID[in.DefectID]
	if !ok {
		return domain.NewBizError(domain.ErrInvalidTransition, "defect already resolved or not found")
	}
	if d.Resolution != "" {
		return domain.NewBizError(domain.ErrInvalidTransition, "defect already resolved or not found")
	}
	now := time.Now()
	d.Resolution = in.Resolution
	d.Note = in.Note
	d.ResolvedAt = &now
	d.ResolvedBy = &in.ResolvedBy
	t.ms.defectsByID[in.DefectID] = d
	return nil
}

// ── Cross-module mocks ──────────────────────────────────────────────────────

type mockBarcodeIssuer struct {
	calls   int
	failOn  int // fail on the Nth call (1-based); 0 = never fail
	lastIn  BarcodeIssueInput
}

func (m *mockBarcodeIssuer) GenerateBarcode(_ context.Context, in BarcodeIssueInput) (BarcodeRef, error) {
	m.calls++
	m.lastIn = in
	if m.failOn != 0 && m.calls == m.failOn {
		return BarcodeRef{}, errors.New("barcode broker down")
	}
	return BarcodeRef{ID: uuid.New()}, nil
}

type mockBarcodeResolver struct {
	known map[uuid.UUID]BarcodeLookup
}

func (m *mockBarcodeResolver) LookupBarcode(_ context.Context, barcodeID uuid.UUID) (BarcodeLookup, error) {
	if b, ok := m.known[barcodeID]; ok {
		return b, nil
	}
	return BarcodeLookup{}, domain.ErrNotFound
}

type mockWOG struct {
	statusByWO map[uuid.UUID]string
}

func (m *mockWOG) GetWorkOrderStatus(_ context.Context, woID uuid.UUID) (WorkOrderStatusInfo, error) {
	s, ok := m.statusByWO[woID]
	if !ok {
		return WorkOrderStatusInfo{}, domain.ErrNotFound
	}
	return WorkOrderStatusInfo{ID: woID, Status: s}, nil
}

type mockSuggester struct {
	suggestions []ContainerSuggestion
	gotSOLine   uuid.UUID
}

func (m *mockSuggester) SuggestForSOLine(_ context.Context, soLineID uuid.UUID) ([]ContainerSuggestion, error) {
	m.gotSOLine = soLineID
	return m.suggestions, nil
}

type mockContainerLineRemover struct {
	removed []uuid.UUID
	failOn  uuid.UUID
}

func (m *mockContainerLineRemover) DeleteLineForDefect(_ context.Context, containerLineID uuid.UUID, _ uuid.UUID) error {
	if m.failOn == containerLineID {
		return errors.New("delete line failed")
	}
	m.removed = append(m.removed, containerLineID)
	return nil
}

type mockNotifier struct {
	defectCalls   int
	resolvedCalls int
}

func (m *mockNotifier) NotifyFGDefect(_ context.Context, _ uuid.UUID, _, _ string) error {
	m.defectCalls++
	return nil
}

func (m *mockNotifier) NotifyFGDefectResolved(_ context.Context, _ uuid.UUID, _ string) error {
	m.resolvedCalls++
	return nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

type harness struct {
	store    *mockStore
	issuer   *mockBarcodeIssuer
	resolver *mockBarcodeResolver
	wog      *mockWOG
	sug      *mockSuggester
	clr      *mockContainerLineRemover
	notif    *mockNotifier
	svc      Service
}

func newHarness() *harness {
	h := &harness{
		store:    newMockStore(),
		issuer:   &mockBarcodeIssuer{},
		resolver: &mockBarcodeResolver{known: map[uuid.UUID]BarcodeLookup{}},
		wog:      &mockWOG{statusByWO: map[uuid.UUID]string{}},
		sug:      &mockSuggester{},
		clr:      &mockContainerLineRemover{},
		notif:    &mockNotifier{},
	}
	h.svc = NewService(h.store, h.issuer, h.resolver, h.wog, h.sug, h.clr, h.notif)
	// Freeze time so test names tied to QC timestamps are stable.
	frozen := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	h.svc.(*service).now = func() time.Time { return frozen }
	return h
}

func (h *harness) seedFG(woID, skuID uuid.UUID, soLineID *uuid.UUID, status string) FGPool {
	fg := FGPool{
		ID:               uuid.New(),
		WorkOrderID:      woID,
		SKUID:            skuID,
		BarcodeID:        uuid.New(),
		SalesOrderLineID: soLineID,
		Status:           status,
		QCPassedAt:       time.Now(),
		QCPassedBy:       uuid.New(),
		CreatedAt:        time.Now(),
	}
	h.store.fgsByID[fg.ID] = fg
	h.store.fgsByBarcode[fg.BarcodeID] = fg.ID
	h.store.fgsByWO[fg.WorkOrderID] = append(h.store.fgsByWO[fg.WorkOrderID], fg.ID)
	h.resolver.known[fg.BarcodeID] = BarcodeLookup{
		ID:          fg.BarcodeID,
		WorkOrderID: woID,
		SKUID:       skuID,
	}
	return fg
}

// ── CreateFromCompletedWO ───────────────────────────────────────────────────

func TestCreateFromCompletedWO_HappyPath_GeneratesQtyRows(t *testing.T) {
	h := newHarness()
	woID, skuID := uuid.New(), uuid.New()
	soLineID := uuid.New()
	rows, err := h.svc.CreateFromCompletedWO(context.Background(), CreateFromCompletedWOInput{
		WorkOrderID:      woID,
		SKUID:            skuID,
		SKUCode:          "SKU-1",
		Quantity:         3,
		SalesOrderLineID: &soLineID,
		QCPassedBy:       uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	if h.issuer.calls != 3 {
		t.Fatalf("want 3 barcode calls, got %d", h.issuer.calls)
	}
	for _, r := range rows {
		if r.Status != FGStatusAvailable {
			t.Errorf("want status AVAILABLE, got %s", r.Status)
		}
		if r.SalesOrderLineID == nil || *r.SalesOrderLineID != soLineID {
			t.Errorf("want SalesOrderLineID=%v, got %v", soLineID, r.SalesOrderLineID)
		}
	}
}

func TestCreateFromCompletedWO_Idempotent_SecondCallReturnsExisting(t *testing.T) {
	h := newHarness()
	woID, skuID := uuid.New(), uuid.New()
	in := CreateFromCompletedWOInput{
		WorkOrderID: woID,
		SKUID:       skuID,
		Quantity:    2,
		QCPassedBy:  uuid.New(),
	}
	first, err := h.svc.CreateFromCompletedWO(context.Background(), in)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := h.svc.CreateFromCompletedWO(context.Background(), in)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(second) != len(first) {
		t.Fatalf("want %d rows on idempotent re-call, got %d", len(first), len(second))
	}
	if h.issuer.calls != 2 {
		t.Errorf("want 2 barcode calls (one batch), got %d", h.issuer.calls)
	}
}

func TestCreateFromCompletedWO_ZeroQty_Rejected(t *testing.T) {
	h := newHarness()
	_, err := h.svc.CreateFromCompletedWO(context.Background(), CreateFromCompletedWOInput{
		WorkOrderID: uuid.New(),
		SKUID:       uuid.New(),
		Quantity:    0,
		QCPassedBy:  uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestCreateFromCompletedWO_LegacyPORooted_NoSOLineOK(t *testing.T) {
	h := newHarness()
	rows, err := h.svc.CreateFromCompletedWO(context.Background(), CreateFromCompletedWOInput{
		WorkOrderID: uuid.New(),
		SKUID:       uuid.New(),
		Quantity:    1,
		QCPassedBy:  uuid.New(),
		// no SalesOrderLineID
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if rows[0].SalesOrderLineID != nil {
		t.Errorf("want nil SalesOrderLineID for PO-rooted, got %v", rows[0].SalesOrderLineID)
	}
}

func TestCreateFromCompletedWO_BarcodeFails_NoRowsInserted(t *testing.T) {
	h := newHarness()
	h.issuer.failOn = 2 // fail on second iteration
	_, err := h.svc.CreateFromCompletedWO(context.Background(), CreateFromCompletedWOInput{
		WorkOrderID: uuid.New(),
		SKUID:       uuid.New(),
		Quantity:    3,
		QCPassedBy:  uuid.New(),
	})
	if err == nil {
		t.Fatal("want error from failed barcode generation")
	}
	if len(h.store.fgsByID) != 0 {
		t.Errorf("want 0 fg rows persisted on partial failure, got %d", len(h.store.fgsByID))
	}
}

// ── ScanBarcode ─────────────────────────────────────────────────────────────

func TestScanBarcode_HappyPath_ReturnsFGAndSuggestions(t *testing.T) {
	h := newHarness()
	soLineID := uuid.New()
	fg := h.seedFG(uuid.New(), uuid.New(), &soLineID, FGStatusAvailable)
	h.wog.statusByWO[fg.WorkOrderID] = string(domain.WOCompleted)
	h.sug.suggestions = []ContainerSuggestion{{Code: "CONT001", FillPctCBM: 0.42}}

	out, err := h.svc.ScanBarcode(context.Background(), fg.BarcodeID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.FG.ID != fg.ID {
		t.Errorf("want FG.ID=%v, got %v", fg.ID, out.FG.ID)
	}
	if out.WOStatus != string(domain.WOCompleted) {
		t.Errorf("want WOStatus COMPLETED, got %s", out.WOStatus)
	}
	if len(out.SuggestedContainers) != 1 || out.SuggestedContainers[0].Code != "CONT001" {
		t.Errorf("want one suggestion CONT001, got %+v", out.SuggestedContainers)
	}
}

func TestScanBarcode_WorkOrderNotCompleted_412(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusAvailable)
	h.wog.statusByWO[fg.WorkOrderID] = string(domain.WOInProcessing)

	_, err := h.svc.ScanBarcode(context.Background(), fg.BarcodeID, uuid.New())
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("want ErrPreconditionFailed, got %v", err)
	}
}

func TestScanBarcode_UnknownBarcode_404(t *testing.T) {
	h := newHarness()
	_, err := h.svc.ScanBarcode(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestScanBarcode_AlreadyReserved_NoSuggestionsReturned(t *testing.T) {
	h := newHarness()
	soLineID := uuid.New()
	fg := h.seedFG(uuid.New(), uuid.New(), &soLineID, FGStatusReserved)
	h.wog.statusByWO[fg.WorkOrderID] = string(domain.WOCompleted)
	h.sug.suggestions = []ContainerSuggestion{{Code: "X"}}

	out, err := h.svc.ScanBarcode(context.Background(), fg.BarcodeID, uuid.New())
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(out.SuggestedContainers) != 0 {
		t.Errorf("want 0 suggestions for non-AVAILABLE FG, got %d", len(out.SuggestedContainers))
	}
}

// ── ReportDefect ────────────────────────────────────────────────────────────

func TestReportDefect_AvailableFG_FlipsToDefect(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusAvailable)

	out, err := h.svc.ReportDefect(context.Background(), ReportDefectInput{
		BarcodeID:  fg.BarcodeID,
		Reason:     DefectReasonBroken,
		DetectedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Reason != DefectReasonBroken {
		t.Errorf("want reason BROKEN, got %s", out.Reason)
	}
	if got := h.store.fgsByID[fg.ID].Status; got != FGStatusDefect {
		t.Errorf("want fg status DEFECT, got %s", got)
	}
	if h.notif.defectCalls != 1 {
		t.Errorf("want 1 notify call, got %d", h.notif.defectCalls)
	}
}

func TestReportDefect_ReservedFG_RemovesContainerLine(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusReserved)
	clID := uuid.New()
	fg.ContainerLineID = &clID
	h.store.fgsByID[fg.ID] = fg

	_, err := h.svc.ReportDefect(context.Background(), ReportDefectInput{
		BarcodeID:  fg.BarcodeID,
		Reason:     DefectReasonScratched,
		DetectedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(h.clr.removed) != 1 || h.clr.removed[0] != clID {
		t.Errorf("want container line %v removed, got %v", clID, h.clr.removed)
	}
	updated := h.store.fgsByID[fg.ID]
	if updated.Status != FGStatusDefect {
		t.Errorf("want status DEFECT, got %s", updated.Status)
	}
	if updated.ContainerLineID != nil {
		t.Errorf("want container_line_id cleared, got %v", updated.ContainerLineID)
	}
}

func TestReportDefect_LoadedFG_Rejected(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusLoaded)
	_, err := h.svc.ReportDefect(context.Background(), ReportDefectInput{
		BarcodeID:  fg.BarcodeID,
		Reason:     DefectReasonBroken,
		DetectedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition for LOADED, got %v", err)
	}
}

func TestReportDefect_DisposedFG_Rejected(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusDisposed)
	_, err := h.svc.ReportDefect(context.Background(), ReportDefectInput{
		BarcodeID:  fg.BarcodeID,
		Reason:     DefectReasonBroken,
		DetectedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition for DISPOSED, got %v", err)
	}
}

func TestReportDefect_InvalidReason_Rejected(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusAvailable)
	_, err := h.svc.ReportDefect(context.Background(), ReportDefectInput{
		BarcodeID:  fg.BarcodeID,
		Reason:     "BANANA",
		DetectedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

// ── ResolveDefect ───────────────────────────────────────────────────────────

func TestResolveDefect_Discard_DisposesFG(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusAvailable)
	d, err := h.svc.ReportDefect(context.Background(), ReportDefectInput{
		BarcodeID:  fg.BarcodeID,
		Reason:     DefectReasonBroken,
		DetectedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("seed defect: %v", err)
	}
	_, err = h.svc.ResolveDefect(context.Background(), ResolveDefectInput{
		DefectID:   d.ID,
		Resolution: DefectResolutionDiscard,
		ResolvedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got := h.store.fgsByID[fg.ID].Status; got != FGStatusDisposed {
		t.Errorf("want DISPOSED, got %s", got)
	}
	if h.notif.resolvedCalls != 1 {
		t.Errorf("want 1 resolved-notify call, got %d", h.notif.resolvedCalls)
	}
}

func TestResolveDefect_Rework_ReturnsToAvailable(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusAvailable)
	d, _ := h.svc.ReportDefect(context.Background(), ReportDefectInput{
		BarcodeID:  fg.BarcodeID,
		Reason:     DefectReasonScratched,
		DetectedBy: uuid.New(),
	})
	_, err := h.svc.ResolveDefect(context.Background(), ResolveDefectInput{
		DefectID:   d.ID,
		Resolution: DefectResolutionRework,
		ResolvedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got := h.store.fgsByID[fg.ID].Status; got != FGStatusAvailable {
		t.Errorf("want AVAILABLE after rework, got %s", got)
	}
}

func TestResolveDefect_ReturnNCC_DisposesFG(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusAvailable)
	d, _ := h.svc.ReportDefect(context.Background(), ReportDefectInput{
		BarcodeID:  fg.BarcodeID,
		Reason:     DefectReasonMissingAccessory,
		DetectedBy: uuid.New(),
	})
	_, err := h.svc.ResolveDefect(context.Background(), ResolveDefectInput{
		DefectID:   d.ID,
		Resolution: DefectResolutionReturnNCC,
		Note:       "kiện NCC lô 2026-05-22",
		ResolvedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got := h.store.fgsByID[fg.ID].Status; got != FGStatusDisposed {
		t.Errorf("want DISPOSED, got %s", got)
	}
}

func TestResolveDefect_DoubleResolve_Rejected(t *testing.T) {
	h := newHarness()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusAvailable)
	d, _ := h.svc.ReportDefect(context.Background(), ReportDefectInput{
		BarcodeID:  fg.BarcodeID,
		Reason:     DefectReasonOther,
		DetectedBy: uuid.New(),
	})
	_, _ = h.svc.ResolveDefect(context.Background(), ResolveDefectInput{
		DefectID:   d.ID,
		Resolution: DefectResolutionDiscard,
		ResolvedBy: uuid.New(),
	})
	_, err := h.svc.ResolveDefect(context.Background(), ResolveDefectInput{
		DefectID:   d.ID,
		Resolution: DefectResolutionRework,
		ResolvedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition on double-resolve, got %v", err)
	}
}

func TestResolveDefect_InvalidResolution_Rejected(t *testing.T) {
	h := newHarness()
	_, err := h.svc.ResolveDefect(context.Background(), ResolveDefectInput{
		DefectID:   uuid.New(),
		Resolution: "BURN_IT_ALL",
		ResolvedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

// ── ReserveOnContainerAdd ───────────────────────────────────────────────────

func TestReserveOnContainerAdd_MatchesAvailable_FlipsToReserved(t *testing.T) {
	h := newHarness()
	skuID := uuid.New()
	soLineID := uuid.New()
	fg1 := h.seedFG(uuid.New(), skuID, &soLineID, FGStatusAvailable)
	fg2 := h.seedFG(uuid.New(), skuID, &soLineID, FGStatusAvailable)
	h.seedFG(uuid.New(), skuID, &soLineID, FGStatusAvailable) // extra; not requested

	clID := uuid.New()
	count, err := h.svc.ReserveOnContainerAdd(context.Background(), ReserveInput{
		SKUID:            skuID,
		SalesOrderLineID: soLineID,
		Qty:              2,
		ContainerLineID:  clID,
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if count != 2 {
		t.Errorf("want 2 reserved, got %d", count)
	}
	reserved := 0
	for _, fg := range h.store.fgsByID {
		if fg.Status == FGStatusReserved {
			reserved++
			if fg.ContainerLineID == nil || *fg.ContainerLineID != clID {
				t.Errorf("want container_line_id stamped on reserved row, got %v", fg.ContainerLineID)
			}
		}
	}
	if reserved != 2 {
		t.Errorf("want 2 RESERVED rows in store, got %d", reserved)
	}
	_ = fg1
	_ = fg2
}

func TestReserveOnContainerAdd_PoolShortfall_ReturnsAvailable(t *testing.T) {
	h := newHarness()
	skuID := uuid.New()
	soLineID := uuid.New()
	h.seedFG(uuid.New(), skuID, &soLineID, FGStatusAvailable) // only 1 row

	count, err := h.svc.ReserveOnContainerAdd(context.Background(), ReserveInput{
		SKUID:            skuID,
		SalesOrderLineID: soLineID,
		Qty:              5,
		ContainerLineID:  uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if count != 1 {
		t.Errorf("want 1 (shortfall), got %d", count)
	}
}

func TestReserveOnContainerAdd_ZeroQty_NoOp(t *testing.T) {
	h := newHarness()
	count, err := h.svc.ReserveOnContainerAdd(context.Background(), ReserveInput{
		SKUID:            uuid.New(),
		SalesOrderLineID: uuid.New(),
		Qty:              0,
		ContainerLineID:  uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if count != 0 {
		t.Errorf("want 0, got %d", count)
	}
}

// ── ReleaseOnContainerDelete ────────────────────────────────────────────────

func TestReleaseOnContainerDelete_FlipsBackToAvailable(t *testing.T) {
	h := newHarness()
	clID := uuid.New()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusReserved)
	fg.ContainerLineID = &clID
	h.store.fgsByID[fg.ID] = fg

	if err := h.svc.ReleaseOnContainerDelete(context.Background(), clID); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	updated := h.store.fgsByID[fg.ID]
	if updated.Status != FGStatusAvailable {
		t.Errorf("want AVAILABLE, got %s", updated.Status)
	}
	if updated.ContainerLineID != nil {
		t.Errorf("want container_line_id cleared, got %v", updated.ContainerLineID)
	}
}

func TestReleaseOnContainerDelete_NoMatchingFGs_NoOp(t *testing.T) {
	h := newHarness()
	if err := h.svc.ReleaseOnContainerDelete(context.Background(), uuid.New()); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

// ── MarkLoadedOnSeal ────────────────────────────────────────────────────────

func TestMarkLoadedOnSeal_PreservesContainerLineID(t *testing.T) {
	h := newHarness()
	clID := uuid.New()
	fg := h.seedFG(uuid.New(), uuid.New(), nil, FGStatusReserved)
	fg.ContainerLineID = &clID
	h.store.fgsByID[fg.ID] = fg

	if err := h.svc.MarkLoadedOnSeal(context.Background(), uuid.New()); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	updated := h.store.fgsByID[fg.ID]
	if updated.Status != FGStatusLoaded {
		t.Errorf("want LOADED, got %s", updated.Status)
	}
	if updated.ContainerLineID == nil || *updated.ContainerLineID != clID {
		t.Errorf("want container_line_id preserved, got %v", updated.ContainerLineID)
	}
}

// ── Helper validators ───────────────────────────────────────────────────────

func TestValidDefectReason(t *testing.T) {
	for _, r := range []string{DefectReasonBroken, DefectReasonWrongSize, DefectReasonMissingAccessory, DefectReasonScratched, DefectReasonOther} {
		if !validDefectReason(r) {
			t.Errorf("want valid for %s", r)
		}
	}
	if validDefectReason("FOO") {
		t.Error("want invalid for FOO")
	}
}

func TestValidResolution(t *testing.T) {
	for _, r := range []string{DefectResolutionDiscard, DefectResolutionRework, DefectResolutionReturnNCC} {
		if !validResolution(r) {
			t.Errorf("want valid for %s", r)
		}
	}
	if validResolution("BURN") {
		t.Error("want invalid for BURN")
	}
}
