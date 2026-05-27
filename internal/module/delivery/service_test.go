package delivery

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// excelizeNewFile / excelizeCoord wrap the upstream API in case excelize
// renames its constructors between minor versions; tests only call these.
func excelizeNewFile() *excelize.File { return excelize.NewFile() }
func excelizeCoord(col, row int) (string, error) {
	return excelize.CoordinatesToCellName(col, row)
}

// ── mockStore ────────────────────────────────────────────────────────────────
//
// Hand-written mock satisfying both `store` and `txStore`. Each call records
// arguments and returns whatever the test wired up. Tests that exercise the
// transactional code paths set `tx` so withTx hands the same instance to fn.

type mockStore struct {
	nextCodeResult string
	nextCodeErr    error

	insertedContainer  *Container
	insertContainerErr error

	selectByIDResult Container
	selectByIDErr    error

	selectLinesResult []ContainerLine
	selectLinesErr    error

	selectPagedResult []Container
	selectPagedErr    error

	selectStatusLogResult []ContainerStatusLogEntry
	selectStatusLogErr    error

	tx        *mockTxStore
	withTxErr error

	// Loading plans (#301)
	insertedLoadingPlan      *LoadingPlan
	insertedLoadingPlanLines []LoadingPlanLine
	insertLoadingPlanErr     error

	selectLoadingPlanByIDResult LoadingPlan
	selectLoadingPlanByIDErr    error

	selectLoadingPlanLinesResult []LoadingPlanLine
	selectLoadingPlanLinesErr    error

	selectActiveLoadingPlanResult LoadingPlan
	selectActiveLoadingPlanErr    error

	nextLoadingPlanVersionResult int
	nextLoadingPlanVersionErr    error

	approveLoadingPlanResult LoadingPlan
	approveLoadingPlanErr    error

	// Supersede / lines-history (#302)
	supersedeLoadingPlanResult LoadingPlan
	supersedeLoadingPlanCount  int
	supersedeLoadingPlanErr    error
	supersedeLoadingPlanCalled bool

	containerLinesCountResult int
	containerLinesCountErr    error

	linesHistoryResult []ContainerLineHistoryEntry
	linesHistoryErr    error
	linesHistoryFilter *uuid.UUID
}

type mockTxStore struct {
	containersByID map[uuid.UUID]Container
	linesByID      map[uuid.UUID]ContainerLine

	// aggregates is keyed by container_id and pre-loaded with the {cbm, weight}
	// pair the test wants sumLinesAggregates to return. Defaults to zero.
	aggregates map[uuid.UUID][2]float64

	// linesForSeal is the list returned by listLinesForSeal for the container
	// touched by seal — set this directly per test case.
	linesForSeal []ShipmentItem

	// errors per operation
	lockContainerErr      error
	lockLineErr           error
	insertLineErr         error
	deleteLineErr         error
	updateLineQtyErr      error
	updateStatusErr       error
	listLinesForSealErr   error
	sumLinesAggregatesErr error

	// call captures
	insertedLines  []ContainerLine
	statusUpdates  []updateStatusInput
	deletedLineIDs []uuid.UUID
	lineQtyUpdates []lineQtyUpdate
}

type lineQtyUpdate struct {
	LineID uuid.UUID
	Qty    int
	CBM    float64
	Weight float64
}

func (m *mockStore) nextContainerCode(_ context.Context, _ time.Time) (string, error) {
	return m.nextCodeResult, m.nextCodeErr
}

func (m *mockStore) insertContainer(_ context.Context, c Container) error {
	if m.insertContainerErr != nil {
		return m.insertContainerErr
	}
	m.insertedContainer = &c
	return nil
}

func (m *mockStore) selectContainerByID(_ context.Context, _ uuid.UUID) (Container, error) {
	return m.selectByIDResult, m.selectByIDErr
}

func (m *mockStore) selectContainerLines(_ context.Context, _ uuid.UUID) ([]ContainerLine, error) {
	return m.selectLinesResult, m.selectLinesErr
}

func (m *mockStore) selectContainersPaged(_ context.Context, _ httpkit.PageParams, _ ContainerListFilter) ([]Container, int, error) {
	return m.selectPagedResult, len(m.selectPagedResult), m.selectPagedErr
}

func (m *mockStore) selectStatusLog(_ context.Context, _ uuid.UUID) ([]ContainerStatusLogEntry, error) {
	return m.selectStatusLogResult, m.selectStatusLogErr
}

func (m *mockStore) withTx(_ context.Context, fn func(tx txStore, raw pgx.Tx) error) error {
	if m.withTxErr != nil {
		return m.withTxErr
	}
	if m.tx == nil {
		m.tx = newMockTx()
	}
	// raw is nil — tests that exercise Seal use a no-op shipment recorder so
	// the cross-module dep never reads from raw.
	return fn(m.tx, nil)
}

func (m *mockStore) insertLoadingPlanWithLines(_ context.Context, plan LoadingPlan, lines []LoadingPlanLine) error {
	if m.insertLoadingPlanErr != nil {
		return m.insertLoadingPlanErr
	}
	m.insertedLoadingPlan = &plan
	m.insertedLoadingPlanLines = append(m.insertedLoadingPlanLines, lines...)
	return nil
}

func (m *mockStore) selectLoadingPlanByID(_ context.Context, _ uuid.UUID) (LoadingPlan, error) {
	return m.selectLoadingPlanByIDResult, m.selectLoadingPlanByIDErr
}

func (m *mockStore) selectLoadingPlanLines(_ context.Context, _ uuid.UUID) ([]LoadingPlanLine, error) {
	return m.selectLoadingPlanLinesResult, m.selectLoadingPlanLinesErr
}

func (m *mockStore) selectActiveLoadingPlan(_ context.Context, _ uuid.UUID) (LoadingPlan, error) {
	return m.selectActiveLoadingPlanResult, m.selectActiveLoadingPlanErr
}

func (m *mockStore) nextLoadingPlanVersion(_ context.Context, _ uuid.UUID) (int, error) {
	if m.nextLoadingPlanVersionResult == 0 && m.nextLoadingPlanVersionErr == nil {
		return 1, nil
	}
	return m.nextLoadingPlanVersionResult, m.nextLoadingPlanVersionErr
}

func (m *mockStore) approveLoadingPlanTx(_ context.Context, _, _ uuid.UUID, _ time.Time) (LoadingPlan, error) {
	return m.approveLoadingPlanResult, m.approveLoadingPlanErr
}

func (m *mockStore) supersedeLoadingPlanTx(_ context.Context, _, _ uuid.UUID, _ time.Time) (LoadingPlan, int, error) {
	m.supersedeLoadingPlanCalled = true
	return m.supersedeLoadingPlanResult, m.supersedeLoadingPlanCount, m.supersedeLoadingPlanErr
}

func (m *mockStore) countContainerLines(_ context.Context, _ uuid.UUID) (int, error) {
	return m.containerLinesCountResult, m.containerLinesCountErr
}

func (m *mockStore) selectContainerLinesHistory(_ context.Context, _ uuid.UUID, planID *uuid.UUID) ([]ContainerLineHistoryEntry, error) {
	m.linesHistoryFilter = planID
	return m.linesHistoryResult, m.linesHistoryErr
}

func (m *mockStore) selectShortagesForContainer(_ context.Context, _ uuid.UUID) (ShortageReport, error) {
	return ShortageReport{}, nil
}

func newMockTx() *mockTxStore {
	return &mockTxStore{
		containersByID: map[uuid.UUID]Container{},
		linesByID:      map[uuid.UUID]ContainerLine{},
		aggregates:     map[uuid.UUID][2]float64{},
	}
}

func (t *mockTxStore) lockContainerForUpdate(_ context.Context, id uuid.UUID) (Container, error) {
	if t.lockContainerErr != nil {
		return Container{}, t.lockContainerErr
	}
	c, ok := t.containersByID[id]
	if !ok {
		return Container{}, domain.ErrNotFound
	}
	return c, nil
}

func (t *mockTxStore) lockLineForUpdate(_ context.Context, lineID uuid.UUID) (ContainerLine, error) {
	if t.lockLineErr != nil {
		return ContainerLine{}, t.lockLineErr
	}
	l, ok := t.linesByID[lineID]
	if !ok {
		return ContainerLine{}, domain.ErrNotFound
	}
	return l, nil
}

func (t *mockTxStore) sumLinesAggregates(_ context.Context, containerID uuid.UUID) (float64, float64, error) {
	if t.sumLinesAggregatesErr != nil {
		return 0, 0, t.sumLinesAggregatesErr
	}
	agg := t.aggregates[containerID]
	return agg[0], agg[1], nil
}

func (t *mockTxStore) listLinesForSeal(_ context.Context, _ uuid.UUID) ([]ShipmentItem, error) {
	if t.listLinesForSealErr != nil {
		return nil, t.listLinesForSealErr
	}
	return t.linesForSeal, nil
}

func (t *mockTxStore) insertLine(_ context.Context, line ContainerLine) error {
	if t.insertLineErr != nil {
		return t.insertLineErr
	}
	t.insertedLines = append(t.insertedLines, line)
	t.linesByID[line.ID] = line
	return nil
}

func (t *mockTxStore) deleteLine(_ context.Context, lineID uuid.UUID) error {
	if t.deleteLineErr != nil {
		return t.deleteLineErr
	}
	t.deletedLineIDs = append(t.deletedLineIDs, lineID)
	delete(t.linesByID, lineID)
	return nil
}

func (t *mockTxStore) updateLineQty(_ context.Context, lineID uuid.UUID, qty int, cbm, weight float64) error {
	if t.updateLineQtyErr != nil {
		return t.updateLineQtyErr
	}
	t.lineQtyUpdates = append(t.lineQtyUpdates, lineQtyUpdate{LineID: lineID, Qty: qty, CBM: cbm, Weight: weight})
	if line, ok := t.linesByID[lineID]; ok {
		line.Qty = qty
		line.CBMTotal = cbm
		line.WeightKGTotal = weight
		t.linesByID[lineID] = line
	}
	return nil
}

func (t *mockTxStore) updateContainerStatus(_ context.Context, in updateStatusInput) (Container, error) {
	if t.updateStatusErr != nil {
		return Container{}, t.updateStatusErr
	}
	t.statusUpdates = append(t.statusUpdates, in)
	c, ok := t.containersByID[in.ContainerID]
	if !ok {
		return Container{}, domain.ErrNotFound
	}
	c.Status = in.ToStatus
	if in.ToStatus == ContainerStatusSealed {
		now := in.Now
		c.SealedAt = &now
		actor := in.ActorID
		c.SealedBy = &actor
	}
	if in.ToStatus == ContainerStatusLoading {
		c.SealedAt = nil
		c.SealedBy = nil
	}
	t.containersByID[in.ContainerID] = c
	return c, nil
}

// ── mock cross-module deps ──────────────────────────────────────────────────

type mockSKU struct{ err error }

func (m *mockSKU) GetSKU(_ context.Context, id uuid.UUID) (SKUInfo, error) {
	if m.err != nil {
		return SKUInfo{}, m.err
	}
	return SKUInfo{ID: id, Code: "SKU-1", Name: "test"}, nil
}

type mockSOLine struct {
	info SOLineInfo
	err  error
}

func (m *mockSOLine) GetSOLine(_ context.Context, _ uuid.UUID) (SOLineInfo, error) {
	if m.err != nil {
		return SOLineInfo{}, m.err
	}
	return m.info, nil
}

type mockShip struct {
	called bool
	err    error
	items  []ShipmentItem
}

func (m *mockShip) RecordShipmentTx(_ context.Context, _ pgx.Tx, items []ShipmentItem) error {
	m.called = true
	m.items = items
	return m.err
}

// ── helpers ─────────────────────────────────────────────────────────────────

func newSvc(s store, sku SKUChecker, soLine SOLineChecker, ship ShipmentRecorder) *service {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	svc := NewService(s, sku, soLine, ship, 5).(*service)
	svc.now = func() time.Time { return now }
	return svc
}

func validSOLineInfo(skuID uuid.UUID, planned, shipped int) SOLineInfo {
	return SOLineInfo{
		ID:         uuid.New(),
		SOID:       uuid.New(),
		SOStatus:   "IN_PRODUCTION",
		SKUID:      skuID,
		QtyPlanned: planned,
		QtyShipped: shipped,
	}
}

// ── CreateContainer ─────────────────────────────────────────────────────────

func TestCreateContainer_HappyPath_DefaultsCapacity(t *testing.T) {
	st := &mockStore{nextCodeResult: "CONT20260601-001"}
	svc := newSvc(st, nil, nil, nil)
	out, err := svc.CreateContainer(context.Background(), CreateContainerInput{
		ContainerType: ContainerType20GP,
		CreatedBy:     uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Code != "CONT20260601-001" {
		t.Errorf("Code = %q, want CONT20260601-001", out.Code)
	}
	if out.MaxCBM != 33.2 {
		t.Errorf("MaxCBM = %v, want 33.2 (20GP default)", out.MaxCBM)
	}
	if out.Status != ContainerStatusOpen {
		t.Errorf("Status = %q, want OPEN", out.Status)
	}
}

func TestCreateContainer_OverrideCapacity(t *testing.T) {
	st := &mockStore{nextCodeResult: "CONT20260601-002"}
	svc := newSvc(st, nil, nil, nil)
	out, err := svc.CreateContainer(context.Background(), CreateContainerInput{
		ContainerType: ContainerType40HC,
		MaxCBM:        70.5,
		MaxPayloadKG:  20000,
		CreatedBy:     uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.MaxCBM != 70.5 || out.MaxPayloadKG != 20000 {
		t.Errorf("override capacity ignored: cbm=%v payload=%v", out.MaxCBM, out.MaxPayloadKG)
	}
}

func TestCreateContainer_UnknownType_Rejected(t *testing.T) {
	svc := newSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.CreateContainer(context.Background(), CreateContainerInput{
		ContainerType: "10FT",
		CreatedBy:     uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on unknown type, got %v", err)
	}
}

func TestCreateContainer_MissingCreatedBy_Rejected(t *testing.T) {
	svc := newSvc(&mockStore{nextCodeResult: "X"}, nil, nil, nil)
	_, err := svc.CreateContainer(context.Background(), CreateContainerInput{
		ContainerType: ContainerType20GP,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on missing created_by, got %v", err)
	}
}

// ── AddLine: BR-D01..D03 + cross-module ─────────────────────────────────────

func TestAddLine_HappyPath_FlipsOpenToLoading(t *testing.T) {
	containerID := uuid.New()
	skuID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{
		ID: containerID, Code: "CONT-1", Status: ContainerStatusOpen,
		MaxCBM: 33.2, MaxPayloadKG: 28000,
	}
	soLine := validSOLineInfo(skuID, 10, 0)
	st := &mockStore{tx: tx}
	svc := newSvc(st, &mockSKU{}, &mockSOLine{info: soLine}, nil)

	line, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID:      containerID,
		SKUID:            skuID,
		Qty:              5,
		SalesOrderLineID: soLine.ID,
		CBMTotal:         2.5,
		WeightKGTotal:    100,
		AddedBy:          uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if line.Qty != 5 {
		t.Errorf("Qty = %d, want 5", line.Qty)
	}
	if len(tx.statusUpdates) != 1 || tx.statusUpdates[0].ToStatus != ContainerStatusLoading {
		t.Errorf("expected OPEN→LOADING flip, got %+v", tx.statusUpdates)
	}
}

func TestAddLine_LoadingContainer_NoStatusFlip(t *testing.T) {
	containerID := uuid.New()
	skuID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{
		ID: containerID, Status: ContainerStatusLoading,
		MaxCBM: 33.2, MaxPayloadKG: 28000,
	}
	soLine := validSOLineInfo(skuID, 10, 0)
	st := &mockStore{tx: tx}
	svc := newSvc(st, &mockSKU{}, &mockSOLine{info: soLine}, nil)
	if _, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID:      containerID,
		SKUID:            skuID,
		Qty:              1,
		SalesOrderLineID: soLine.ID,
		CBMTotal:         1, WeightKGTotal: 50,
		AddedBy: uuid.New(),
	}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(tx.statusUpdates) != 0 {
		t.Errorf("LOADING → no status flip, got %+v", tx.statusUpdates)
	}
}

func TestAddLine_SealedContainer_Rejected(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusSealed}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	_, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID: containerID, SKUID: uuid.New(), SalesOrderLineID: uuid.New(),
		Qty: 1, AddedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on SEALED, got %v", err)
	}
}

func TestAddLine_CBMOverflow_Rejected(t *testing.T) {
	containerID := uuid.New()
	skuID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{
		ID: containerID, Status: ContainerStatusLoading,
		MaxCBM: 10, MaxPayloadKG: 28000, Code: "CONT-1",
	}
	tx.aggregates[containerID] = [2]float64{9, 100}
	soLine := validSOLineInfo(skuID, 10, 0)
	st := &mockStore{tx: tx}
	svc := newSvc(st, &mockSKU{}, &mockSOLine{info: soLine}, nil)
	// 9 + 5 = 14, > 10 * 1.05 = 10.5 → reject
	_, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID:      containerID,
		SKUID:            skuID,
		Qty:              1,
		SalesOrderLineID: soLine.ID,
		CBMTotal:         5, WeightKGTotal: 100,
		AddedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on CBM overflow, got %v", err)
	}
}

func TestAddLine_WithinOverhead_Accepted(t *testing.T) {
	// 10 cbm cap + 5% overhead = 10.5. Total 10.4 must pass.
	containerID := uuid.New()
	skuID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{
		ID: containerID, Status: ContainerStatusLoading,
		MaxCBM: 10, MaxPayloadKG: 28000,
	}
	tx.aggregates[containerID] = [2]float64{10, 100}
	soLine := validSOLineInfo(skuID, 10, 0)
	st := &mockStore{tx: tx}
	svc := newSvc(st, &mockSKU{}, &mockSOLine{info: soLine}, nil)
	if _, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID: containerID, SKUID: skuID, Qty: 1, SalesOrderLineID: soLine.ID,
		CBMTotal: 0.4, WeightKGTotal: 100, AddedBy: uuid.New(),
	}); err != nil {
		t.Errorf("within 5%% overhead must pass, got %v", err)
	}
}

func TestAddLine_WeightOverflow_Rejected(t *testing.T) {
	containerID := uuid.New()
	skuID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{
		ID: containerID, Status: ContainerStatusLoading,
		MaxCBM: 33.2, MaxPayloadKG: 1000, Code: "CONT-1",
	}
	tx.aggregates[containerID] = [2]float64{1, 999}
	soLine := validSOLineInfo(skuID, 10, 0)
	st := &mockStore{tx: tx}
	svc := newSvc(st, &mockSKU{}, &mockSOLine{info: soLine}, nil)
	_, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID: containerID, SKUID: skuID, Qty: 1, SalesOrderLineID: soLine.ID,
		CBMTotal: 1, WeightKGTotal: 200, AddedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on weight overflow, got %v", err)
	}
}

func TestAddLine_SOLineSKUMismatch_Rejected(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{
		ID: containerID, Status: ContainerStatusOpen,
		MaxCBM: 33, MaxPayloadKG: 28000,
	}
	soLine := validSOLineInfo(uuid.New(), 10, 0) // SO line carries different SKU
	st := &mockStore{tx: tx}
	svc := newSvc(st, &mockSKU{}, &mockSOLine{info: soLine}, nil)
	_, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID: containerID, SKUID: uuid.New(), Qty: 1, SalesOrderLineID: soLine.ID,
		CBMTotal: 1, WeightKGTotal: 50, AddedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on SKU mismatch, got %v", err)
	}
}

func TestAddLine_SODraft_Rejected(t *testing.T) {
	containerID := uuid.New()
	skuID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{
		ID: containerID, Status: ContainerStatusOpen,
		MaxCBM: 33, MaxPayloadKG: 28000,
	}
	soLine := validSOLineInfo(skuID, 10, 0)
	soLine.SOStatus = "DRAFT"
	st := &mockStore{tx: tx}
	svc := newSvc(st, &mockSKU{}, &mockSOLine{info: soLine}, nil)
	_, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID: containerID, SKUID: skuID, Qty: 1, SalesOrderLineID: soLine.ID,
		CBMTotal: 1, WeightKGTotal: 50, AddedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on DRAFT SO, got %v", err)
	}
}

func TestAddLine_QtyExceedsRemainingPlanned_Rejected(t *testing.T) {
	skuID := uuid.New()
	soLine := validSOLineInfo(skuID, 10, 8)
	svc := newSvc(&mockStore{}, &mockSKU{}, &mockSOLine{info: soLine}, nil)
	// qty_shipped + 5 = 13 > qty_planned 10 → reject
	_, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID: uuid.New(), SKUID: skuID, Qty: 5, SalesOrderLineID: soLine.ID,
		CBMTotal: 1, WeightKGTotal: 50, AddedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on planned overflow, got %v", err)
	}
}

func TestAddLine_ZeroQty_Rejected(t *testing.T) {
	svc := newSvc(&mockStore{}, &mockSKU{}, nil, nil)
	_, err := svc.AddLine(context.Background(), AddLineInput{
		ContainerID: uuid.New(), SKUID: uuid.New(), SalesOrderLineID: uuid.New(),
		Qty: 0, AddedBy: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on qty=0, got %v", err)
	}
}

// ── DeleteLine ──────────────────────────────────────────────────────────────

func TestDeleteLine_OpenContainer_Allowed(t *testing.T) {
	containerID := uuid.New()
	lineID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusLoading}
	tx.linesByID[lineID] = ContainerLine{ID: lineID, ContainerID: containerID, Qty: 3}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	if err := svc.DeleteLine(context.Background(), containerID, lineID, uuid.New()); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(tx.deletedLineIDs) != 1 || tx.deletedLineIDs[0] != lineID {
		t.Errorf("expected delete on lineID, got %+v", tx.deletedLineIDs)
	}
}

func TestDeleteLine_LineFromAnotherContainer_Rejected(t *testing.T) {
	containerID := uuid.New()
	otherID := uuid.New()
	lineID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusLoading}
	tx.linesByID[lineID] = ContainerLine{ID: lineID, ContainerID: otherID}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	err := svc.DeleteLine(context.Background(), containerID, lineID, uuid.New())
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on cross-container delete, got %v", err)
	}
}

func TestDeleteLine_SealedContainer_Rejected(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusSealed}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	err := svc.DeleteLine(context.Background(), containerID, uuid.New(), uuid.New())
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on SEALED delete, got %v", err)
	}
}

// ── TransferLine: BR-D04 ────────────────────────────────────────────────────

func TestTransferLine_FullMove(t *testing.T) {
	srcID := uuid.New()
	tgtID := uuid.New()
	lineID := uuid.New()
	tx := newMockTx()
	tx.containersByID[srcID] = Container{ID: srcID, Status: ContainerStatusLoading, MaxCBM: 33, MaxPayloadKG: 28000}
	tx.containersByID[tgtID] = Container{ID: tgtID, Status: ContainerStatusOpen, MaxCBM: 33, MaxPayloadKG: 28000}
	tx.linesByID[lineID] = ContainerLine{ID: lineID, ContainerID: srcID, Qty: 4, CBMTotal: 2, WeightKGTotal: 50}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)

	res, err := svc.TransferLine(context.Background(), TransferLineInput{
		ContainerID: srcID, TargetContainerID: tgtID, LineID: lineID, ActorID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if res.SourceLine != nil {
		t.Errorf("full transfer leaves SourceLine nil, got %+v", res.SourceLine)
	}
	if res.TargetLine.Qty != 4 {
		t.Errorf("TargetLine.Qty = %d, want 4", res.TargetLine.Qty)
	}
	if len(tx.deletedLineIDs) != 1 {
		t.Errorf("expected source line deleted, got %+v", tx.deletedLineIDs)
	}
	// Target was OPEN → expect OPEN→LOADING flip in addition to insertLine.
	hasFlip := false
	for _, u := range tx.statusUpdates {
		if u.ContainerID == tgtID && u.ToStatus == ContainerStatusLoading {
			hasFlip = true
		}
	}
	if !hasFlip {
		t.Errorf("expected target OPEN→LOADING flip, got %+v", tx.statusUpdates)
	}
}

func TestTransferLine_PartialQty_DecrementsSource(t *testing.T) {
	srcID := uuid.New()
	tgtID := uuid.New()
	lineID := uuid.New()
	tx := newMockTx()
	tx.containersByID[srcID] = Container{ID: srcID, Status: ContainerStatusLoading, MaxCBM: 33, MaxPayloadKG: 28000}
	tx.containersByID[tgtID] = Container{ID: tgtID, Status: ContainerStatusLoading, MaxCBM: 33, MaxPayloadKG: 28000}
	tx.linesByID[lineID] = ContainerLine{ID: lineID, ContainerID: srcID, Qty: 10, CBMTotal: 5, WeightKGTotal: 100}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)

	res, err := svc.TransferLine(context.Background(), TransferLineInput{
		ContainerID: srcID, TargetContainerID: tgtID, LineID: lineID,
		Qty: 3, CBMTotal: 1.5, WeightKGTotal: 30,
		ActorID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if res.SourceLine == nil || res.SourceLine.Qty != 7 {
		t.Errorf("partial transfer should leave source.qty=7, got %+v", res.SourceLine)
	}
	if res.TargetLine.Qty != 3 {
		t.Errorf("TargetLine.Qty = %d, want 3", res.TargetLine.Qty)
	}
	// Source line is updated, not deleted.
	if len(tx.deletedLineIDs) != 0 {
		t.Errorf("partial transfer must not delete source, got %+v", tx.deletedLineIDs)
	}
	if len(tx.lineQtyUpdates) != 1 || tx.lineQtyUpdates[0].Qty != 7 {
		t.Errorf("expected source qty bump to 7, got %+v", tx.lineQtyUpdates)
	}
}

func TestTransferLine_SealedSource_Rejected(t *testing.T) {
	srcID := uuid.New()
	tgtID := uuid.New()
	tx := newMockTx()
	tx.containersByID[srcID] = Container{ID: srcID, Status: ContainerStatusSealed}
	tx.containersByID[tgtID] = Container{ID: tgtID, Status: ContainerStatusLoading}
	tx.linesByID[uuid.UUID{}] = ContainerLine{}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	_, err := svc.TransferLine(context.Background(), TransferLineInput{
		ContainerID: srcID, TargetContainerID: tgtID, LineID: uuid.New(),
		ActorID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestTransferLine_SameContainer_Rejected(t *testing.T) {
	id := uuid.New()
	svc := newSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.TransferLine(context.Background(), TransferLineInput{
		ContainerID: id, TargetContainerID: id, LineID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestTransferLine_PartialQtyExceedsLine_Rejected(t *testing.T) {
	srcID := uuid.New()
	tgtID := uuid.New()
	lineID := uuid.New()
	tx := newMockTx()
	tx.containersByID[srcID] = Container{ID: srcID, Status: ContainerStatusLoading, MaxCBM: 33, MaxPayloadKG: 28000}
	tx.containersByID[tgtID] = Container{ID: tgtID, Status: ContainerStatusLoading, MaxCBM: 33, MaxPayloadKG: 28000}
	tx.linesByID[lineID] = ContainerLine{ID: lineID, ContainerID: srcID, Qty: 5, CBMTotal: 2, WeightKGTotal: 50}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	_, err := svc.TransferLine(context.Background(), TransferLineInput{
		ContainerID: srcID, TargetContainerID: tgtID, LineID: lineID,
		Qty: 6, CBMTotal: 1, WeightKGTotal: 10, ActorID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestTransferLine_PartialMissingCBMWeight_Rejected(t *testing.T) {
	srcID := uuid.New()
	tgtID := uuid.New()
	lineID := uuid.New()
	tx := newMockTx()
	tx.containersByID[srcID] = Container{ID: srcID, Status: ContainerStatusLoading, MaxCBM: 33, MaxPayloadKG: 28000}
	tx.containersByID[tgtID] = Container{ID: tgtID, Status: ContainerStatusLoading, MaxCBM: 33, MaxPayloadKG: 28000}
	tx.linesByID[lineID] = ContainerLine{ID: lineID, ContainerID: srcID, Qty: 5, CBMTotal: 2, WeightKGTotal: 50}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	_, err := svc.TransferLine(context.Background(), TransferLineInput{
		ContainerID: srcID, TargetContainerID: tgtID, LineID: lineID,
		Qty: 2, ActorID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// ── Seal: BR-D05 ────────────────────────────────────────────────────────────

func TestSeal_HappyPath_CallsRecorderAndFlipsStatus(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusLoading}
	tx.linesForSeal = []ShipmentItem{
		{SOLineID: uuid.New(), Qty: 5},
		{SOLineID: uuid.New(), Qty: 3},
	}
	st := &mockStore{tx: tx}
	ship := &mockShip{}
	svc := newSvc(st, nil, nil, ship)
	out, err := svc.Seal(context.Background(), SealInput{ContainerID: containerID, ActorID: uuid.New()})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !ship.called {
		t.Error("ShipmentRecorder must be called")
	}
	if len(ship.items) != 2 {
		t.Errorf("expected 2 items passed to recorder, got %d", len(ship.items))
	}
	if out.Status != ContainerStatusSealed {
		t.Errorf("Status = %q, want SEALED", out.Status)
	}
}

func TestSeal_EmptyContainer_Rejected(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusLoading}
	tx.linesForSeal = nil
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, &mockShip{})
	_, err := svc.Seal(context.Background(), SealInput{ContainerID: containerID, ActorID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on empty container, got %v", err)
	}
}

func TestSeal_ShipmentRecorderFails_RollsBack(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusLoading}
	tx.linesForSeal = []ShipmentItem{{SOLineID: uuid.New(), Qty: 1}}
	st := &mockStore{tx: tx}
	ship := &mockShip{err: errors.New("qty_shipped overflow")}
	svc := newSvc(st, nil, nil, ship)
	_, err := svc.Seal(context.Background(), SealInput{ContainerID: containerID, ActorID: uuid.New()})
	if err == nil {
		t.Fatal("expected shipment recorder error to propagate")
	}
	// Status update must NOT have been called.
	for _, u := range tx.statusUpdates {
		if u.ToStatus == ContainerStatusSealed {
			t.Errorf("seal must roll back when recorder fails, but status was flipped")
		}
	}
}

func TestSeal_SealedContainer_Rejected(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusSealed}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, &mockShip{})
	_, err := svc.Seal(context.Background(), SealInput{ContainerID: containerID, ActorID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestSeal_NoRecorder_PreconditionFailed(t *testing.T) {
	svc := newSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.Seal(context.Background(), SealInput{ContainerID: uuid.New(), ActorID: uuid.New()})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Errorf("expected ErrPreconditionFailed, got %v", err)
	}
}

// ── Reopen: BR-D06 ──────────────────────────────────────────────────────────

func TestReopen_RequiresReason(t *testing.T) {
	svc := newSvc(&mockStore{}, nil, nil, nil)
	_, err := svc.Reopen(context.Background(), ReopenInput{ContainerID: uuid.New(), ActorID: uuid.New(), Reason: "  "})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on blank reason, got %v", err)
	}
}

func TestReopen_NonSealed_Rejected(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusLoading}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	_, err := svc.Reopen(context.Background(), ReopenInput{
		ContainerID: containerID, ActorID: uuid.New(), Reason: "fix manifest",
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestReopen_SealedContainer_FlipsToLoading(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	now := time.Now()
	actor := uuid.New()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusSealed, SealedAt: &now, SealedBy: &actor}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	out, err := svc.Reopen(context.Background(), ReopenInput{
		ContainerID: containerID, ActorID: uuid.New(), Reason: "fix manifest",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Status != ContainerStatusLoading {
		t.Errorf("Status = %q, want LOADING", out.Status)
	}
	if out.SealedAt != nil || out.SealedBy != nil {
		t.Errorf("reopen must clear sealed_at/sealed_by")
	}
}

// ── Ship: BR-D07 ────────────────────────────────────────────────────────────

func TestShip_SealedToShipped(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusSealed}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	out, err := svc.Ship(context.Background(), ShipInput{ContainerID: containerID, ActorID: uuid.New()})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Status != ContainerStatusShipped {
		t.Errorf("Status = %q, want SHIPPED", out.Status)
	}
}

func TestShip_NonSealed_Rejected(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusLoading}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	_, err := svc.Ship(context.Background(), ShipInput{ContainerID: containerID, ActorID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

// ── Cancel ──────────────────────────────────────────────────────────────────

func TestCancel_Loading_Allowed(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusLoading}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	out, err := svc.Cancel(context.Background(), CancelInput{ContainerID: containerID, ActorID: uuid.New()})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.Status != ContainerStatusCancelled {
		t.Errorf("Status = %q, want CANCELLED", out.Status)
	}
}

func TestCancel_Sealed_Rejected(t *testing.T) {
	containerID := uuid.New()
	tx := newMockTx()
	tx.containersByID[containerID] = Container{ID: containerID, Status: ContainerStatusSealed}
	st := &mockStore{tx: tx}
	svc := newSvc(st, nil, nil, nil)
	_, err := svc.Cancel(context.Background(), CancelInput{ContainerID: containerID, ActorID: uuid.New()})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

// ── DefaultCapacityForType ──────────────────────────────────────────────────

func TestDefaultCapacityForType_Known(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		cbm  float64
	}{
		{"20GP", ContainerType20GP, 33.2},
		{"40GP", ContainerType40GP, 67.7},
		{"40HC", ContainerType40HC, 76.4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cbm, _, ok := DefaultCapacityForType(c.typ)
			if !ok || cbm != c.cbm {
				t.Errorf("got cbm=%v ok=%v, want cbm=%v ok=true", cbm, ok, c.cbm)
			}
		})
	}
}

func TestDefaultCapacityForType_Unknown(t *testing.T) {
	if _, _, ok := DefaultCapacityForType("10FT"); ok {
		t.Error("unknown type must return ok=false")
	}
}

// ── orderUUIDs ──────────────────────────────────────────────────────────────

func TestOrderUUIDs_StableLockOrdering(t *testing.T) {
	a := uuid.UUID{0x01}
	b := uuid.UUID{0x02}
	first, second := orderUUIDs(a, b)
	if first != a || second != b {
		t.Errorf("ordering broken: got (%v, %v)", first, second)
	}
	first, second = orderUUIDs(b, a)
	if first != a || second != b {
		t.Errorf("ordering broken (reverse args): got (%v, %v)", first, second)
	}
}

// ── Loading plans (#301) ────────────────────────────────────────────────────
//
// The service path hinges on excelize parsing real .xlsx bytes, so the helper
// below builds a minimal workbook in-memory per test. Six cases cover the DoD
// matrix from issue #301: happy, missing column, unmapped SKU, invalid unit,
// negative qty, duplicate hash.

type mockCustomerSKUResolver struct {
	mappings map[string]uuid.UUID
	err      error
}

func (m *mockCustomerSKUResolver) ResolveCustomerSKU(_ context.Context, _ uuid.UUID, code string) (uuid.UUID, error) {
	if m.err != nil {
		return uuid.Nil, m.err
	}
	if id, ok := m.mappings[code]; ok {
		return id, nil
	}
	return uuid.Nil, domain.NewBizError(domain.ErrNotFound, "no mapping for "+code)
}

// buildLoadingPlanXLSX renders a workbook with the v1 column layout, plus an
// optional knob for omitting the unit column (drives the missing-column test).
type loadingPlanRow struct {
	Code  string
	Qty   string
	Unit  string
	Notes string
}

func buildLoadingPlanXLSX(t *testing.T, header []string, rows []loadingPlanRow) *bytes.Buffer {
	t.Helper()
	f := excelizeNewFile()
	defer func() { _ = f.Close() }()

	for i, h := range header {
		cell, _ := excelizeCoord(i+1, 1)
		_ = f.SetCellValue("Sheet1", cell, h)
	}
	for ri, r := range rows {
		colA, _ := excelizeCoord(1, ri+2)
		colB, _ := excelizeCoord(2, ri+2)
		colC, _ := excelizeCoord(3, ri+2)
		colD, _ := excelizeCoord(4, ri+2)
		_ = f.SetCellValue("Sheet1", colA, r.Code)
		_ = f.SetCellValue("Sheet1", colB, r.Qty)
		_ = f.SetCellValue("Sheet1", colC, r.Unit)
		if r.Notes != "" {
			_ = f.SetCellValue("Sheet1", colD, r.Notes)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("excelize write: %v", err)
	}
	return &buf
}

func newSvcWithLoadingPlan(t *testing.T, st *mockStore, resolver CustomerSKUResolver) *service {
	t.Helper()
	svc := &service{
		s:           st,
		skuResolver: resolver,
		now:         func() time.Time { return time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC) },
	}
	return svc
}

func validContainerForLoadingPlan() Container {
	return Container{ID: uuid.New(), Status: ContainerStatusOpen}
}

func TestUploadLoadingPlan_HappyPath(t *testing.T) {
	skuID := uuid.New()
	st := &mockStore{
		selectByIDResult:           validContainerForLoadingPlan(),
		selectActiveLoadingPlanErr: domain.NewBizError(domain.ErrNotFound, "none"),
	}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{
		mappings: map[string]uuid.UUID{"CUST-001": skuID},
	})

	xlsx := buildLoadingPlanXLSX(t, []string{"customer_sku_code", "qty", "unit"}, []loadingPlanRow{
		{Code: "CUST-001", Qty: "10", Unit: "PCS"},
	})

	res, err := svc.UploadLoadingPlan(context.Background(), UploadLoadingPlanInput{
		ContainerID: st.selectByIDResult.ID,
		CustomerID:  uuid.New(),
		UploadedBy:  uuid.New(),
		File:        xlsx,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Lines) != 1 || res.Lines[0].SKUID != skuID || res.Lines[0].QtyPlannedPieces != 10 {
		t.Errorf("expected one line with sku=%v qty=10, got %+v", skuID, res.Lines)
	}
	if res.Plan.Status != LoadingPlanStatusParsed {
		t.Errorf("Status = %q, want PARSED", res.Plan.Status)
	}
	if st.insertedLoadingPlan == nil {
		t.Error("insertLoadingPlanWithLines was never called")
	}
}

func TestUploadLoadingPlan_MissingColumn_Rejected(t *testing.T) {
	st := &mockStore{
		selectByIDResult:           validContainerForLoadingPlan(),
		selectActiveLoadingPlanErr: domain.NewBizError(domain.ErrNotFound, "none"),
	}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{})

	// Header is missing the unit column.
	xlsx := buildLoadingPlanXLSX(t, []string{"customer_sku_code", "qty"}, []loadingPlanRow{
		{Code: "CUST-001", Qty: "10"},
	})

	_, err := svc.UploadLoadingPlan(context.Background(), UploadLoadingPlanInput{
		ContainerID: st.selectByIDResult.ID,
		CustomerID:  uuid.New(),
		UploadedBy:  uuid.New(),
		File:        xlsx,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput on missing header column, got %v", err)
	}
	if st.insertedLoadingPlan != nil {
		t.Error("plan must NOT be inserted when header is malformed")
	}
}

func TestUploadLoadingPlan_UnmappedSKU_Rejected(t *testing.T) {
	st := &mockStore{
		selectByIDResult:           validContainerForLoadingPlan(),
		selectActiveLoadingPlanErr: domain.NewBizError(domain.ErrNotFound, "none"),
	}
	// Resolver knows CUST-001 but the file references CUST-X.
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{
		mappings: map[string]uuid.UUID{"CUST-001": uuid.New()},
	})

	xlsx := buildLoadingPlanXLSX(t, []string{"customer_sku_code", "qty", "unit"}, []loadingPlanRow{
		{Code: "CUST-X", Qty: "10", Unit: "PCS"},
	})

	res, err := svc.UploadLoadingPlan(context.Background(), UploadLoadingPlanInput{
		ContainerID: st.selectByIDResult.ID,
		CustomerID:  uuid.New(),
		UploadedBy:  uuid.New(),
		File:        xlsx,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput on unmapped SKU, got %v", err)
	}
	if len(res.Errors) == 0 || res.Errors[0].Code != "UNMAPPED_SKU" {
		t.Errorf("expected per-row UNMAPPED_SKU error, got %+v", res.Errors)
	}
	if st.insertedLoadingPlan != nil {
		t.Error("plan must NOT be inserted when an SKU is unmapped (fail-all)")
	}
}

func TestUploadLoadingPlan_InvalidUnit_Rejected(t *testing.T) {
	st := &mockStore{
		selectByIDResult:           validContainerForLoadingPlan(),
		selectActiveLoadingPlanErr: domain.NewBizError(domain.ErrNotFound, "none"),
	}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{
		mappings: map[string]uuid.UUID{"CUST-001": uuid.New()},
	})

	xlsx := buildLoadingPlanXLSX(t, []string{"customer_sku_code", "qty", "unit"}, []loadingPlanRow{
		{Code: "CUST-001", Qty: "10", Unit: ""}, // empty unit
	})

	res, err := svc.UploadLoadingPlan(context.Background(), UploadLoadingPlanInput{
		ContainerID: st.selectByIDResult.ID,
		CustomerID:  uuid.New(),
		UploadedBy:  uuid.New(),
		File:        xlsx,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput on empty unit, got %v", err)
	}
	if len(res.Errors) == 0 || res.Errors[0].Code != "MISSING_UNIT" {
		t.Errorf("expected MISSING_UNIT error, got %+v", res.Errors)
	}
}

func TestUploadLoadingPlan_NegativeQty_Rejected(t *testing.T) {
	st := &mockStore{
		selectByIDResult:           validContainerForLoadingPlan(),
		selectActiveLoadingPlanErr: domain.NewBizError(domain.ErrNotFound, "none"),
	}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{
		mappings: map[string]uuid.UUID{"CUST-001": uuid.New()},
	})

	xlsx := buildLoadingPlanXLSX(t, []string{"customer_sku_code", "qty", "unit"}, []loadingPlanRow{
		{Code: "CUST-001", Qty: "-5", Unit: "PCS"},
	})

	res, err := svc.UploadLoadingPlan(context.Background(), UploadLoadingPlanInput{
		ContainerID: st.selectByIDResult.ID,
		CustomerID:  uuid.New(),
		UploadedBy:  uuid.New(),
		File:        xlsx,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput on negative qty, got %v", err)
	}
	if len(res.Errors) == 0 || res.Errors[0].Code != "INVALID_QTY" {
		t.Errorf("expected INVALID_QTY error, got %+v", res.Errors)
	}
}

func TestUploadLoadingPlan_DuplicateHash_Rejected(t *testing.T) {
	skuID := uuid.New()

	xlsx := buildLoadingPlanXLSX(t, []string{"customer_sku_code", "qty", "unit"}, []loadingPlanRow{
		{Code: "CUST-001", Qty: "10", Unit: "PCS"},
	})
	bodyBytes := xlsx.Bytes()

	// Pre-compute the same hash the parser would generate so the active plan
	// stub looks like a previously-uploaded copy of the same file.
	probe, err := parseExcelV1(bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("probe parse: %v", err)
	}

	st := &mockStore{
		selectByIDResult: validContainerForLoadingPlan(),
		selectActiveLoadingPlanResult: LoadingPlan{
			ID:        uuid.New(),
			Status:    LoadingPlanStatusApproved,
			ExcelHash: probe.Hash,
		},
	}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{
		mappings: map[string]uuid.UUID{"CUST-001": skuID},
	})

	_, err = svc.UploadLoadingPlan(context.Background(), UploadLoadingPlanInput{
		ContainerID: st.selectByIDResult.ID,
		CustomerID:  uuid.New(),
		UploadedBy:  uuid.New(),
		File:        bytes.NewReader(bodyBytes),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput on duplicate hash, got %v", err)
	}
	if st.insertedLoadingPlan != nil {
		t.Error("plan must NOT be inserted when active hash matches (BR-D10)")
	}
}

// ── ApproveLoadingPlan supersede + reload (#302) ────────────────────────────

type mockPlanReloadNotifier struct {
	calls   []PlanReloadNotice
	failErr error
}

func (m *mockPlanReloadNotifier) NotifyPlanReload(_ context.Context, in PlanReloadNotice) error {
	m.calls = append(m.calls, in)
	return m.failErr
}

func TestApproveLoadingPlan_NoExistingLines_NoSupersede(t *testing.T) {
	planID := uuid.New()
	containerID := uuid.New()
	st := &mockStore{
		selectLoadingPlanByIDResult: LoadingPlan{
			ID: planID, ContainerID: containerID, Status: LoadingPlanStatusParsed, Version: 1,
		},
		selectByIDResult:          Container{ID: containerID, Status: ContainerStatusOpen},
		containerLinesCountResult: 0, // no live lines → plain approve path
		approveLoadingPlanResult: LoadingPlan{
			ID: planID, ContainerID: containerID, Status: LoadingPlanStatusApproved, Version: 1,
		},
	}
	notifier := &mockPlanReloadNotifier{}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{})
	svc.planReloader = notifier

	out, err := svc.ApproveLoadingPlan(context.Background(), ApproveLoadingPlanInput{
		PlanID: planID, ActorID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("ApproveLoadingPlan: %v", err)
	}
	if out.Status != LoadingPlanStatusApproved {
		t.Errorf("status = %s, want APPROVED", out.Status)
	}
	if st.supersedeLoadingPlanCalled {
		t.Error("supersedeLoadingPlanTx must NOT be called when no live lines exist")
	}
	if len(notifier.calls) != 0 {
		t.Errorf("PLAN_RELOAD must not fire for first-time approve; got %d calls", len(notifier.calls))
	}
}

func TestApproveLoadingPlan_WithLines_RequiresConfirm(t *testing.T) {
	planID := uuid.New()
	containerID := uuid.New()
	st := &mockStore{
		selectLoadingPlanByIDResult: LoadingPlan{
			ID: planID, ContainerID: containerID, Status: LoadingPlanStatusParsed, Version: 2,
		},
		selectByIDResult:          Container{ID: containerID, Status: ContainerStatusOpen},
		containerLinesCountResult: 7, // worker scanned 7 units → must confirm
	}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{})

	_, err := svc.ApproveLoadingPlan(context.Background(), ApproveLoadingPlanInput{
		PlanID: planID, ActorID: uuid.New(),
		// ConfirmSupersede deliberately false
	})
	if !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed without confirm_supersede, got %v", err)
	}
	if st.supersedeLoadingPlanCalled {
		t.Error("supersede must NOT be invoked without confirm_supersede=true")
	}
}

func TestApproveLoadingPlan_WithLines_ConfirmedSupersede_NotifiesReload(t *testing.T) {
	planID := uuid.New()
	containerID := uuid.New()
	st := &mockStore{
		selectLoadingPlanByIDResult: LoadingPlan{
			ID: planID, ContainerID: containerID, Status: LoadingPlanStatusParsed, Version: 2,
		},
		selectByIDResult:          Container{ID: containerID, Status: ContainerStatusOpen},
		containerLinesCountResult: 5,
		supersedeLoadingPlanResult: LoadingPlan{
			ID: planID, ContainerID: containerID, Status: LoadingPlanStatusApproved, Version: 2,
		},
		supersedeLoadingPlanCount: 5,
	}
	notifier := &mockPlanReloadNotifier{}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{})
	svc.planReloader = notifier

	actor := uuid.New()
	out, err := svc.ApproveLoadingPlan(context.Background(), ApproveLoadingPlanInput{
		PlanID: planID, ActorID: actor, ConfirmSupersede: true,
	})
	if err != nil {
		t.Fatalf("ApproveLoadingPlan: %v", err)
	}
	if !st.supersedeLoadingPlanCalled {
		t.Error("supersedeLoadingPlanTx must run when lines exist + confirmed")
	}
	if out.Version != 2 {
		t.Errorf("version = %d, want 2", out.Version)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("expected 1 PLAN_RELOAD notification, got %d", len(notifier.calls))
	}
	got := notifier.calls[0]
	if got.ContainerID != containerID || got.NewPlanID != planID {
		t.Errorf("notify ids = %+v, want container=%s plan=%s", got, containerID, planID)
	}
	if got.SupersededLines != 5 {
		t.Errorf("superseded_lines = %d, want 5", got.SupersededLines)
	}
	if got.NewVersion != 2 {
		t.Errorf("new_version = %d, want 2", got.NewVersion)
	}
}

func TestApproveLoadingPlan_SealedContainer_Rejected(t *testing.T) {
	planID := uuid.New()
	containerID := uuid.New()
	st := &mockStore{
		selectLoadingPlanByIDResult: LoadingPlan{
			ID: planID, ContainerID: containerID, Status: LoadingPlanStatusParsed, Version: 2,
		},
		selectByIDResult: Container{ID: containerID, Status: ContainerStatusSealed},
	}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{})

	_, err := svc.ApproveLoadingPlan(context.Background(), ApproveLoadingPlanInput{
		PlanID: planID, ActorID: uuid.New(), ConfirmSupersede: true,
	})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition on SEALED container, got %v", err)
	}
	if st.supersedeLoadingPlanCalled {
		t.Error("supersede must not run when container is SEALED (BR-D12)")
	}
}

func TestApproveLoadingPlan_AfterForceUnseal_Accepted(t *testing.T) {
	// Simulates the admin force-unseal flow: container goes SEALED → OPEN before
	// approve runs, so the BR-D12 guard passes and supersede proceeds.
	planID := uuid.New()
	containerID := uuid.New()
	st := &mockStore{
		selectLoadingPlanByIDResult: LoadingPlan{
			ID: planID, ContainerID: containerID, Status: LoadingPlanStatusParsed, Version: 2,
		},
		selectByIDResult:          Container{ID: containerID, Status: ContainerStatusOpen},
		containerLinesCountResult: 3,
		supersedeLoadingPlanResult: LoadingPlan{
			ID: planID, ContainerID: containerID, Status: LoadingPlanStatusApproved, Version: 2,
		},
		supersedeLoadingPlanCount: 3,
	}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{})

	_, err := svc.ApproveLoadingPlan(context.Background(), ApproveLoadingPlanInput{
		PlanID: planID, ActorID: uuid.New(), ConfirmSupersede: true,
	})
	if err != nil {
		t.Fatalf("ApproveLoadingPlan after force-unseal: %v", err)
	}
	if !st.supersedeLoadingPlanCalled {
		t.Error("supersede must run after force-unseal")
	}
}

func TestListContainerLinesHistory_PlanFilterPropagated(t *testing.T) {
	planID := uuid.New()
	containerID := uuid.New()
	st := &mockStore{
		linesHistoryResult: []ContainerLineHistoryEntry{
			{ID: uuid.New(), ContainerID: containerID, SupersededByPlan: planID},
		},
	}
	svc := newSvcWithLoadingPlan(t, st, &mockCustomerSKUResolver{})

	out, err := svc.ListContainerLinesHistory(context.Background(), containerID, &planID)
	if err != nil {
		t.Fatalf("ListContainerLinesHistory: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("entries = %d, want 1", len(out))
	}
	if st.linesHistoryFilter == nil || *st.linesHistoryFilter != planID {
		t.Errorf("plan filter not propagated, got %v", st.linesHistoryFilter)
	}
}
