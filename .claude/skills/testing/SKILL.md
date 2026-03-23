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

### Table-driven test template
```go
func TestService_MethodName(t *testing.T) {
    tests := []struct {
        name    string
        input   InputDTO
        setup   func(mockStore *mockStore)
        want    OutputDTO
        wantErr error
    }{
        {
            name:  "success case",
            input: InputDTO{...},
            setup: func(ms *mockStore) {
                ms.findByIDFn = func(ctx context.Context, id uuid.UUID) (*Entity, error) {
                    return &Entity{...}, nil
                }
            },
            want: OutputDTO{...},
        },
        {
            name:    "not found",
            input:   InputDTO{ID: unknownID},
            setup:   func(ms *mockStore) {
                ms.findByIDFn = func(ctx context.Context, id uuid.UUID) (*Entity, error) {
                    return nil, domain.ErrNotFound
                }
            },
            wantErr: domain.ErrNotFound,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ms := &mockStore{}
            if tt.setup != nil {
                tt.setup(ms)
            }
            svc := NewService(ms)
            got, err := svc.Method(context.Background(), tt.input)
            if tt.wantErr != nil {
                assert.ErrorIs(t, err, tt.wantErr)
                return
            }
            assert.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
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
