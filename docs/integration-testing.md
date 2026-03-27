# Integration Testing Guide

## Overview

Integration tests verify the cooperation between the backend and a **real PostgreSQL 17 database**. They exercise the full stack from service → pgstore → SQL → database, ensuring business rules, transactions, and row-level locks behave correctly end-to-end.

Unit tests (no build tag) remain in place to cover pure business logic with mock stores. Integration tests complement them by catching SQL correctness, constraint violations, and concurrency issues that mocks cannot simulate.

---

## Prerequisites

| Requirement | Version | Notes |
|---|---|---|
| Docker | ≥ 24 | Must be running and socket accessible |
| Go | ≥ 1.24 | Matches project `go.mod` |
| Internet access | — | Required on first run to pull `postgres:17-alpine` image |

No manual database setup, migrations, or environment variables are needed. The test harness manages everything automatically.

---

## Running integration tests

```bash
# Recommended: use the Makefile target
make test-integration

# Equivalent direct command
go test -tags integration ./... -race -count=1 -timeout 180s -v
```

### What happens when you run this

1. Go collects all test files with the `//go:build integration` build tag.
2. For each package that has integration tests, `testhelper.StartTestDB` is called once per package (via `sync.Once` in `TestMain`).
3. `testcontainers-go` pulls (cached after first run) and starts a `postgres:17-alpine` container.
4. Goose runs all migrations from `migrations/` against the container.
5. Tests run against a fully migrated, isolated database.
6. Each test calls `truncateXxx(t)` before running to reset state.
7. After all tests complete, the container is terminated and removed automatically.

**Total runtime:** approximately 30–60 seconds (dominated by container startup; subsequent runs reuse the cached image).

---

## Test structure

### Build tag

Every integration test file starts with:

```go
//go:build integration

package <module_name>
```

The build tag ensures integration tests are **never included** in `make test` (the standard unit-test run).

### Package placement

Integration tests live next to the code they test, in the **same Go package** (white-box):

```
internal/module/inventory/
├── pgstore.go                      ← implementation
├── pgstore_integration_test.go     ← integration tests (build tag: integration)
├── service.go
└── service_test.go                 ← unit tests (no build tag)
```

### Shared testhelper

`internal/testhelper/db.go` provides:

| Function | Description |
|---|---|
| `StartTestDB(t *testing.T) *pgxpool.Pool` | Starts a PostgreSQL container, runs all migrations, returns a ready pool. Registers cleanup via `t.Cleanup`. |
| `TruncateAll(t, pool)` | Deletes all rows in FK-safe order. Use between tests when a single container is shared across a whole package. |

---

## Adding integration tests to a new module

1. Create `internal/module/<name>/pgstore_integration_test.go`.

2. Add the build tag and use the same package:

```go
//go:build integration

package <name>
```

3. Set up a shared pool in `TestMain`:

```go
var (
    sharedPool *pgxpool.Pool
    setupOnce  sync.Once
)

func getPool(t *testing.T) *pgxpool.Pool {
    t.Helper()
    setupOnce.Do(func() {
        sharedPool = testhelper.StartTestDB(t)
    })
    return sharedPool
}

func TestMain(m *testing.M) {
    os.Exit(m.Run())
}
```

4. Reset state at the start of each test using `DELETE FROM` in FK-safe order.

5. Write table-driven or individual tests using `NewPGStore(pool)` and `NewService(...)`.

---

## Existing integration test coverage

| Package | Tests | Key scenarios |
|---|---|---|
| `internal/module/inventory` | 11 tests | ReceiveStock → lot+sheets persisted; RecordCut (area conservation BR-K03); nested remnant lineage (BR-K04); AllocateRemnant; MarkRemnantWaste; **concurrent RecordCut on same sheet** (FOR UPDATE lock); **concurrent AllocateRemnant** (FOR UPDATE lock) |
| `internal/module/catalog` | 10 tests | CreateMaterial/SKU round-trips; duplicate SKU code rejection (DB constraint); BOM set/replace/get; ErrNotFound for missing entities; CreatedAt timestamp |

---

## Running only specific packages

```bash
# Only inventory integration tests
go test -tags integration ./internal/module/inventory/... -v -count=1

# Only catalog integration tests
go test -tags integration ./internal/module/catalog/... -v -count=1
```

---

## Skipping long-running concurrent tests

Concurrent locking tests are skipped when `-short` is passed:

```bash
go test -tags integration ./... -short -count=1
```

---

## CI integration

In GitHub Actions (or any CI with Docker), add a step:

```yaml
- name: Run integration tests
  run: make test-integration
```

No service containers need to be declared — testcontainers-go manages its own Docker containers using the runner's Docker socket.

If the CI runner uses **Docker-in-Docker** (`docker:dind`), set:

```yaml
env:
  DOCKER_HOST: tcp://docker:2376
  DOCKER_TLS_CERTDIR: /certs
```

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `Cannot connect to the Docker daemon` | Docker is not running | Start Docker Desktop / Docker daemon |
| `context deadline exceeded` after ~60 s | Container pull timed out | Check internet connection; image is cached after first pull |
| `relation "xxx" does not exist` | Migration ran only partially | Check `migrations/` for a failed migration; run `make migrate-down` + `make migrate-up` on dev DB to validate |
| `duplicate key value violates unique constraint` in tests | Test did not truncate before running | Add `truncateXxx(t)` call at the start of the test |
| Unit tests suddenly slow | Integration test file missing build tag | Ensure `//go:build integration` is the **first line** of the file |
