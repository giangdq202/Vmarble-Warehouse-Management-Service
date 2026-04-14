package inventory

// service_concurrent_test.go — parallel-safety tests for row-level locking.
//
// These tests exercise the behaviour of the service layer when multiple
// goroutines race to allocate, cut, or waste the same board sheet / remnant.
//
// Because the service delegates the lock-then-write path to the store, we
// use a concurrentMockStore whose methods are goroutine-safe: they serialise
// callers through a mutex and simulate the semantics of SELECT … FOR UPDATE
// (exactly one caller wins; all others get ErrPreconditionFailed).
//
// The tests therefore verify:
//   1. The service correctly propagates ErrPreconditionFailed from the store.
//   2. Exactly one goroutine succeeds per resource.
//   3. No data race is detected by -race.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vmarble/warehouse-management-service/internal/domain"
	"github.com/vmarble/warehouse-management-service/internal/platform/httpkit"
)

// ── concurrentMockStore ───────────────────────────────────────────────────────
//
// A goroutine-safe mock that enforces "exactly one winner" semantics for the
// three atomic operations, mimicking what PostgreSQL FOR UPDATE achieves.

type concurrentMockStore struct {
	mu sync.Mutex

	// sheet state
	sheets map[uuid.UUID]*BoardSheet

	// remnant state
	remnants map[uuid.UUID]*Remnant
}

func newConcurrentMockStore() *concurrentMockStore {
	return &concurrentMockStore{
		sheets:   make(map[uuid.UUID]*BoardSheet),
		remnants: make(map[uuid.UUID]*Remnant),
	}
}

func (s *concurrentMockStore) addSheet(sh BoardSheet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := sh
	s.sheets[sh.ID] = &cp
}

func (s *concurrentMockStore) addRemnant(r Remnant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := r
	s.remnants[r.ID] = &cp
}

// ── store interface implementation ───────────────────────────────────────────

func (s *concurrentMockStore) insertLot(_ context.Context, _ InventoryLot) error { return nil }
func (s *concurrentMockStore) selectLots(_ context.Context) ([]InventoryLot, error) {
	return nil, nil
}
func (s *concurrentMockStore) selectLotsPaged(_ context.Context, _ httpkit.PageParams) ([]InventoryLot, int, error) {
	return nil, 0, nil
}
func (s *concurrentMockStore) deactivateLot(_ context.Context, _ uuid.UUID) error { return nil }
func (s *concurrentMockStore) insertSheets(_ context.Context, _ []BoardSheet) error { return nil }

func (s *concurrentMockStore) preAssignSheet(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}

func (s *concurrentMockStore) selectSheetByID(_ context.Context, id uuid.UUID) (BoardSheet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.sheets[id]
	if !ok {
		return BoardSheet{}, domain.NewBizError(domain.ErrNotFound, "board sheet not found")
	}
	return *sh, nil
}

func (s *concurrentMockStore) selectAvailableSheets(_ context.Context) ([]BoardSheet, error) {
	return nil, nil
}
func (s *concurrentMockStore) selectAvailableSheetsPaged(_ context.Context, _ httpkit.PageParams) ([]BoardSheet, int, error) {
	return nil, 0, nil
}
func (s *concurrentMockStore) updateSheetStatus(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) error {
	return nil
}
func (s *concurrentMockStore) insertCuttingRecord(_ context.Context, _ CuttingRecord) error {
	return nil
}
func (s *concurrentMockStore) insertRemnant(_ context.Context, _ Remnant) error { return nil }
func (s *concurrentMockStore) selectAvailableRemnantsByMinDimension(_ context.Context, _ domain.Dimension) ([]Remnant, error) {
	return nil, nil
}
func (s *concurrentMockStore) selectRemnantsByFilter(_ context.Context, _ RemnantFilter, _ httpkit.PageParams) ([]Remnant, int, error) {
	return nil, 0, nil
}
func (s *concurrentMockStore) selectRemnantsByBoardSheet(_ context.Context, _ uuid.UUID) ([]Remnant, error) {
	return nil, nil
}
func (s *concurrentMockStore) selectActiveStorageLocations(_ context.Context) ([]StorageLocation, error) {
	return nil, nil
}

func (s *concurrentMockStore) selectRemnantByID(_ context.Context, id uuid.UUID) (Remnant, error) {
	// Non-locking snapshot read — mirrors an ordinary SELECT without FOR UPDATE.
	// The pre-check in service.go is an optimistic fast-fail; the authoritative
	// guard is inside the atomic store methods (which do hold the mutex).
	s.mu.Lock()
	r, ok := s.remnants[id]
	if !ok {
		s.mu.Unlock()
		return Remnant{}, domain.NewBizError(domain.ErrNotFound, "remnant not found")
	}
	cp := *r // copy under lock before releasing
	s.mu.Unlock()
	return cp, nil
}

func (s *concurrentMockStore) updateRemnantStatus(_ context.Context, _ uuid.UUID, _ domain.RemnantStatus, _ *uuid.UUID) error {
	return nil
}

// recordCutAtomically: first caller to grab the lock on an AVAILABLE sheet/remnant
// wins; the rest get ErrPreconditionFailed — mirroring SELECT … FOR UPDATE.
func (s *concurrentMockStore) recordCutAtomically(_ context.Context, op cutWriteOp) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if op.SheetUpdate != nil {
		sh, ok := s.sheets[op.SheetUpdate.ID]
		if !ok {
			return domain.NewBizError(domain.ErrNotFound, "board sheet not found")
		}
		if sh.Status != "AVAILABLE" {
			return domain.NewBizError(domain.ErrPreconditionFailed, "board sheet is no longer available")
		}
		sh.Status = op.SheetUpdate.Status
		sh.IssuedToWorkOrderID = op.SheetUpdate.IssuedToWO
	} else if op.RemnantUpdate != nil {
		r, ok := s.remnants[op.RemnantUpdate.ID]
		if !ok {
			return domain.NewBizError(domain.ErrNotFound, "remnant not found")
		}
		if r.Status != domain.RemnantAvailable {
			return domain.NewBizError(domain.ErrPreconditionFailed, "remnant is no longer available")
		}
		r.Status = op.RemnantUpdate.Status
	}
	return nil
}

// allocateRemnantAtomically: exactly one concurrent caller wins.
func (s *concurrentMockStore) allocateRemnantAtomically(_ context.Context, remnantID uuid.UUID, workOrderID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.remnants[remnantID]
	if !ok {
		return domain.NewBizError(domain.ErrNotFound, "remnant not found")
	}
	if r.Status != domain.RemnantAvailable {
		return domain.NewBizError(domain.ErrPreconditionFailed, "remnant is no longer available for allocation")
	}
	r.Status = domain.RemnantAllocated
	r.AllocatedToWO = &workOrderID
	return nil
}

// markRemnantWasteAtomically: exactly one concurrent caller wins.
func (s *concurrentMockStore) markRemnantWasteAtomically(_ context.Context, remnantID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.remnants[remnantID]
	if !ok {
		return domain.NewBizError(domain.ErrNotFound, "remnant not found")
	}
	if r.Status != domain.RemnantAvailable && r.Status != domain.RemnantAllocated {
		return domain.NewBizError(domain.ErrPreconditionFailed, "remnant cannot be marked waste in its current state")
	}
	r.Status = domain.RemnantWaste
	r.AllocatedToWO = nil
	return nil
}

func (s *concurrentMockStore) releaseExpiredAllocations(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (s *concurrentMockStore) updateRemnantBinLocation(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}
func (s *concurrentMockStore) selectStorageLocationByBarcode(_ context.Context, _ string) (StorageLocation, error) {
	return StorageLocation{}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func isSuccessOrPrecondition(t *testing.T, err error) {
	t.Helper()
	if err != nil && !errors.Is(err, domain.ErrPreconditionFailed) && !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestConcurrentRecordCut_SameSheet ensures that when N goroutines all try to
// cut the same board sheet concurrently, exactly one succeeds and the rest
// receive ErrPreconditionFailed (from the store layer).
func TestConcurrentRecordCut_SameSheet(t *testing.T) {
	const workers = 20

	sheetID := uuid.New()
	st := newConcurrentMockStore()
	st.addSheet(BoardSheet{
		ID:           sheetID,
		LotID:        uuid.New(),
		Dimensions:   dim2000x1000,
		CostPerSheet: domain.Money{Amount: 100_000, Currency: "VND"},
		Status:       "AVAILABLE",
	})

	svc := NewService(st, nil)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		successes int
		failures  int
	)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.RecordCut(context.Background(), RecordCutInput{
				SheetID:       ptr(sheetID),
				WorkOrderID:   uuid.New(),
				SKUID:         uuid.New(),
				UsedDimension: dim1000x500,
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				isSuccessOrPrecondition(t, err)
				failures++
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 success, got %d (failures: %d)", successes, failures)
	}
	if successes+failures != workers {
		t.Errorf("success(%d) + failures(%d) != workers(%d)", successes, failures, workers)
	}
}

// TestConcurrentRecordCut_SameRemnant ensures concurrent cuts from the same
// remnant produce exactly one winner.
func TestConcurrentRecordCut_SameRemnant(t *testing.T) {
	const workers = 20

	boardID := uuid.New()
	remID := uuid.New()
	st := newConcurrentMockStore()
	st.addRemnant(Remnant{
		ID:            remID,
		ParentBoardID: boardID,
		Dimensions:    dim1000x500,
		Status:        domain.RemnantAvailable,
	})

	svc := NewService(st, nil)

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		failures  int
	)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.RecordCut(context.Background(), RecordCutInput{
				RemnantID:     ptr(remID),
				WorkOrderID:   uuid.New(),
				SKUID:         uuid.New(),
				UsedDimension: dim100x100,
			})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				isSuccessOrPrecondition(t, err)
				failures++
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 success, got %d (failures: %d)", successes, failures)
	}
}

// TestConcurrentAllocateRemnant_SameRemnant ensures concurrent allocation
// attempts on the same remnant result in exactly one successful allocation.
func TestConcurrentAllocateRemnant_SameRemnant(t *testing.T) {
	const workers = 20

	boardID := uuid.New()
	remID := uuid.New()
	st := newConcurrentMockStore()
	st.addRemnant(Remnant{
		ID:            remID,
		ParentBoardID: boardID,
		Dimensions:    dim1000x500,
		Status:        domain.RemnantAvailable,
	})

	svc := NewService(st, nil)

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		failures  int
	)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.AllocateRemnant(context.Background(), remID, uuid.New())
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				isSuccessOrPrecondition(t, err)
				failures++
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 success, got %d (failures: %d)", successes, failures)
	}
}

// TestConcurrentMarkRemnantWaste_SameRemnant ensures concurrent waste-marking
// attempts on the same remnant result in exactly one successful transition.
func TestConcurrentMarkRemnantWaste_SameRemnant(t *testing.T) {
	const workers = 20

	boardID := uuid.New()
	remID := uuid.New()
	st := newConcurrentMockStore()
	st.addRemnant(Remnant{
		ID:            remID,
		ParentBoardID: boardID,
		Dimensions:    dim1000x500,
		Status:        domain.RemnantAvailable,
	})

	svc := NewService(st, nil)

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
		failures  int
	)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.MarkRemnantWaste(context.Background(), remID)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				isSuccessOrPrecondition(t, err)
				failures++
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 success, got %d (failures: %d)", successes, failures)
	}
}

// TestConcurrentAllocateAndWaste_SameRemnant simulates a race between one
// worker allocating a remnant and another marking it as waste simultaneously.
//
// The service uses an optimistic pre-check (snapshot read) then delegates
// the authoritative status check to the store's atomic method (which holds
// an exclusive lock). This means both goroutines can pass the optimistic
// guard when their snapshot reads both see AVAILABLE. The real serialisation
// happens at the store/DB layer — the test therefore:
//   1. Verifies no data race is detected by -race.
//   2. Verifies the final state of the remnant is always valid (ALLOCATED or
//      WASTE — never still AVAILABLE, never an unknown state).
//   3. Verifies the final state is stable (both operations agree on the
//      outcome stored in the mock).
func TestConcurrentAllocateAndWaste_SameRemnant(t *testing.T) {
	const trials = 50 // run many times to increase race-detection coverage

	validFinalStatuses := map[domain.RemnantStatus]bool{
		domain.RemnantAllocated: true,
		domain.RemnantWaste:     true,
	}

	for range trials {
		boardID := uuid.New()
		remID := uuid.New()
		st := newConcurrentMockStore()
		st.addRemnant(Remnant{
			ID:            remID,
			ParentBoardID: boardID,
			Dimensions:    dim1000x500,
			Status:        domain.RemnantAvailable,
		})

		svc := NewService(st, nil)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			err := svc.AllocateRemnant(context.Background(), remID, uuid.New())
			if err != nil {
				isSuccessOrPrecondition(t, err)
			}
		}()

		go func() {
			defer wg.Done()
			err := svc.MarkRemnantWaste(context.Background(), remID)
			if err != nil {
				isSuccessOrPrecondition(t, err)
			}
		}()

		wg.Wait()

		// After both goroutines finish, the remnant must be in a valid terminal
		// state: ALLOCATED or WASTE. It must never still be AVAILABLE.
		finalRemnant, err := st.selectRemnantByID(context.Background(), remID)
		if err != nil {
			t.Fatalf("trial: could not read final remnant state: %v", err)
		}
		if !validFinalStatuses[finalRemnant.Status] {
			t.Fatalf("trial: remnant ended in invalid state %q (expected ALLOCATED or WASTE)", finalRemnant.Status)
		}
	}
}

