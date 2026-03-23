# Add Tests Prompt

Use this prompt when asking Claude to add tests for an existing module.

---

Add comprehensive tests for the `{MODULE_NAME}` module at `internal/module/{MODULE_NAME}/`.

## What to test

### service_test.go (unit tests)
- Create a mock struct implementing the `store` interface (and any `deps.go` interfaces)
- Test every method on the `Service` interface
- Use table-driven tests with cases for:
  - Happy path (valid input, expected output)
  - Validation errors (missing fields, invalid values → `ErrInvalidInput`)
  - Not found (entity doesn't exist → `ErrNotFound`)
  - State transition errors (invalid transitions → `ErrInvalidTransition`)
  - Business rule violations (stock, area conservation, etc.)
  - Edge cases (empty lists, boundary values, nil pointers)

### pgstore_test.go (integration tests — optional)
- Test SQL queries against a real PostgreSQL instance
- Use test helpers for setup/teardown
- Test constraint enforcement (FKs, CHECKs, unique)

## Test conventions
- Deterministic: no `time.Sleep`, no wall clock dependency
- Isolated: each test sets up its own state
- Descriptive names: `Test{Method}_{scenario}`
- Run with: `go test -race -count=1 ./internal/module/{MODULE_NAME}/...`
