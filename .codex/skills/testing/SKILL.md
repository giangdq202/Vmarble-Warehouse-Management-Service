---
name: testing
description: Use to add, fix, or review tests for the Go backend, especially unit tests for service.go, integration tests for pgstore.go, and handler tests. Trigger when the user asks to write tests, improve coverage, reproduce a bug with tests, or validate behavior.
---

# Testing Skill

You are writing tests for the VMARBLE Warehouse Management Service (Go 1.24).

## Test structure

### Unit tests for `service.go`
- Mock the `store` interface and any `deps.go` interfaces
- Test business rules, validation, and state transitions
- Use table-driven tests for multiple input scenarios
- File naming: `service_test.go` in the same package

### Integration tests for `pgstore.go`
- Test against a real PostgreSQL instance
- Use test helpers to set up/tear down test data
- Focus on SQL correctness, constraint enforcement
- File naming: `pgstore_test.go` in the same package

### Handler tests (optional)
- Test HTTP binding, status codes, and response shapes
- Use `httptest` + gin's test mode
- File naming: `handler_test.go`

## Test patterns

### mockStore pattern

The `mockStore` in `service_test.go` uses **plain struct fields** — not function
callbacks. Each store method reads from a matching field and returns it.

```go
// ── Declaration (add fields per store method) ─────────────────────────────────
type mockStore struct {
    // selectEntityByID
    selectEntityByIDResult SomeEntity
    selectEntityByIDErr    error

    // insertEntity
    insertEntityErr error

    // recordWriteAtomically — inspect what the service passed in
    recordWriteAtomicallyCalled bool
    recordWriteAtomicallyOp    writeOp
    recordWriteAtomicallyErr   error
}

// ── Implementation ────────────────────────────────────────────────────────────
func (m *mockStore) selectEntityByID(_ context.Context, _ uuid.UUID) (SomeEntity, error) {
    return m.selectEntityByIDResult, m.selectEntityByIDErr
}
func (m *mockStore) insertEntity(_ context.Context, _ SomeEntity) error {
    return m.insertEntityErr
}
func (m *mockStore) recordWriteAtomically(_ context.Context, op writeOp) error {
    m.recordWriteAtomicallyCalled = true
    m.recordWriteAtomicallyOp = op
    return m.recordWriteAtomicallyErr
}
```

### Unit test pattern (no testify — stdlib only)

```go
func TestService_HappyPath(t *testing.T) {
    entityID := uuid.New()
    st := &mockStore{
        selectEntityByIDResult: SomeEntity{ID: entityID, Status: "AVAILABLE"},
    }
    svc := NewService(st)

    result, err := svc.DoSomething(context.Background(), DoSomethingInput{
        EntityID: entityID,
        // ...
    })
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.ID == uuid.Nil {
        t.Error("result.ID must be set")
    }
}

func TestService_NotFound_PropagatesError(t *testing.T) {
    st := &mockStore{
        selectEntityByIDErr: domain.NewBizError(domain.ErrNotFound, "not found"),
    }
    svc := NewService(st)

    _, err := svc.DoSomething(context.Background(), DoSomethingInput{EntityID: uuid.New()})
    if !errors.Is(err, domain.ErrNotFound) {
        t.Errorf("expected ErrNotFound, got %v", err)
    }
}

func TestService_AtomicNotCalledOnValidationFailure(t *testing.T) {
    st := &mockStore{
        selectEntityByIDResult: SomeEntity{Status: "ISSUED"}, // wrong status
    }
    svc := NewService(st)

    _, err := svc.DoSomething(context.Background(), DoSomethingInput{EntityID: uuid.New()})
    if !errors.Is(err, domain.ErrInvalidInput) {
        t.Errorf("expected ErrInvalidInput, got %v", err)
    }
    if st.recordWriteAtomicallyCalled {
        t.Error("atomic method must NOT be called when validation fails")
    }
}
```

### Helper utilities already in `service_test.go`

```go
// Generic pointer helper — use instead of &literal
func ptr[T any](v T) *T { return &v }

// Example dimension fixtures
var (
    dim2000x1000 = domain.Dimension{LengthMM: 2000, WidthMM: 1000}
    dim1000x500  = domain.Dimension{LengthMM: 1000, WidthMM: 500}
)
```

## Test rules

- **Deterministic**: no `time.Sleep`, no reliance on wall clock
- **Isolated**: each test sets up its own state
- **Named**: descriptive test names that explain the scenario
- **Fast**: unit tests should run in milliseconds
- Run with: `make test` (includes `-race -count=1`)

## Domain invariants to always test

- WorkOrder state transitions (valid and invalid)
- Area conservation in cutting operations
- Remnant lineage correctness
- Costing finalization immutability
- Metal requirement enforcement

## Error scenarios to cover

- `ErrNotFound` when entity does not exist
- `ErrInvalidInput` for malformed data
- `ErrInvalidTransition` for illegal state changes
- `ErrInsufficientStock` for inventory shortages
- `ErrAreaConservation` for area violations
- `ErrAlreadyFinalized` for immutable records
