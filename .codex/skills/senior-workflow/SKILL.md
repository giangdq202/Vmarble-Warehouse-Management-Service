---
name: senior-workflow
description: >
  Use when starting ANY non-trivial issue or feature on the Go backend — covers
  the full Senior Engineer workflow: requirements clarification → technical design
  → task breakdown → implement + test → self-QA → PR.
  ALWAYS trigger this skill when the user says "implement", "add feature",
  "fix issue", "build", "create endpoint", or pastes a GitHub issue/ticket.
  Do NOT skip phases — especially Phase 5 (Self-QA) which is the most commonly
  forgotten step before submitting code.
---

# Senior Engineer Workflow — Go Backend

Run these 6 phases **in order**. Mark each one done before moving to the next.
Never skip Phase 5 — it is the gate before PR.

---

## Phase 1 — Requirements Clarification

Before writing a single line of code, understand the *why*.

**Questions to answer (ask the user if unclear):**
- What problem does this solve for the business? Which BR-* rule does it touch?
- What is the exact Definition of Done?
  - Code only? Or code + migration + swagger + tests?
- Adversarial edge cases to surface:
  - "What happens if this is called twice concurrently?"
  - "What if the source entity is in the wrong status?"
  - "Does this touch area conservation (BR-K03) or costing (BR-C02)?"
- Is there a Sprint issue number? (for commit/PR tagging)

**Output of this phase:** a short bullet list: *Why / DoD / Edge cases identified*

---

## Phase 2 — Technical Design (Go Backend)

Design before coding. Sketch the solution on paper first.

### Module impact analysis
- Which of the 7 modules is affected? (`inventory`, `production`, `costing`, `planning`, `order`, `catalog`, `barcode`)
- Does this cross module boundaries? If yes → must define interface in `deps.go`, wired in `main.go`
- Which domain invariants are at risk?
  - WorkOrder state machine: `PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED`
  - Area conservation: `used_area + remnant_area ≤ source_area` (BR-K03)
  - Remnant lineage: `parent_board_id` + `parent_remnant_id` (BR-K04)
  - Costing immutability: finalized records cannot change (BR-C04)

### Data model decisions
- New table? → write migration with both `Up` and `Down`
- New columns? → additive (non-breaking) preferred; nullable or with DEFAULT
- New FK? → check cascade behavior; add index on FK column

### API design (for new endpoints)
- Route: `METHOD /api/v1/<resource>`
- Request/Response DTO → goes in `iface.go`
- HTTP status codes: 400/404/409/412/422 — pick the right one
- Does it need pagination? Use `httpkit.PageParams` + `httpkit.PagedResult[T]`

### store interface changes
- New store method? → add to `store.go` interface first
- Atomic write needed? → define a `*WriteOp` struct + `*Atomically` method pattern
- Read-then-write? → must use `SELECT … FOR UPDATE` inside a transaction

### Sequence diagram (text is fine)
```
Handler → Service → Store (SELECT FOR UPDATE) → DB
                  ↓ validate invariants
                  → Store (INSERT/UPDATE in tx)
```

**Output of this phase:** interface signatures, migration sketch, sequence diagram

---

## Phase 3 — Task Breakdown

Split into sub-tasks of 2–4 hours each. Create a TodoList.

Typical order for a new feature:
1. Write migration (`migrations/NNNN_*.sql`)
2. Update `iface.go` (new DTOs, new Service methods)
3. Update `store.go` (new store interface methods)
4. Implement `pgstore.go` (SQL)
5. Implement `service.go` (business logic + validation)
6. Update `handler.go` (HTTP binding)
7. Update `mockStore` in `service_test.go`
8. Write unit tests (`service_test.go`)
9. Write integration tests (`pgstore_integration_test.go`)
10. Run `make swagger` if endpoints changed

**Rule:** Never implement step N+1 before step N is done and tests pass.

---

## Phase 4 — Implement & Test (Go Backend)

### Order of implementation within a file

**`service.go`** — implement in this order:
1. Input validation (return `ErrInvalidInput` early)
2. Fetch required entities (check status, return `ErrNotFound` / `ErrInvalidInput`)
3. Business rule checks (return domain-specific errors: `ErrAreaConservation`, `ErrInvalidTransition`…)
4. Build the write operation struct
5. Call atomic store method
6. Return result

**`pgstore.go`** — SQL patterns to follow:
```go
// Single row read
row := p.pool.QueryRow(ctx, `SELECT ... WHERE id=$1`, id)
if err := row.Scan(&...); errors.Is(err, pgx.ErrNoRows) {
    return domain.NewBizError(domain.ErrNotFound, "entity not found")
}

// Atomic write with FOR UPDATE
tx, _ := p.pool.Begin(ctx)
defer tx.Rollback(ctx)
// SELECT ... FOR UPDATE
// validate status under lock
// INSERT / UPDATE
tx.Commit(ctx)
```

### Test coverage rules
- Every new `service.go` method → at least:
  - 1 happy-path unit test
  - 1 test per error branch (`ErrNotFound`, `ErrInvalidInput`, domain errors)
  - 1 test that verifies `recordCutAtomically` / atomic method is NOT called on validation failure
- Every new `pgstore.go` method → integration test in `pgstore_integration_test.go`
- Concurrent tests for any new atomic method (use goroutines, verify exactly 1 success)

### mockStore update
When adding a new store method, add to `mockStore` in `service_test.go`:
```go
// field
newMethodResult SomeType
newMethodErr    error

// implementation
func (m *mockStore) newMethod(_ context.Context, ...) (SomeType, error) {
    return m.newMethodResult, m.newMethodErr
}
```

---

## Phase 5 — Self-QA & Refactor ⛔ DO NOT SKIP

This phase is the gate before PR. Run through **all** of these — not just the ones that seem relevant.

### Code smell checklist (read your own code aloud)
- [ ] Any magic strings or numbers? → extract to `const`
- [ ] Function longer than ~40 lines? → extract helper
- [ ] Error message vague ("invalid input") → add context ("sheet must be AVAILABLE, got ISSUED")
- [ ] `context.Background()` in request flow? → replace with `c.Request.Context()`
- [ ] Raw `pgx` error returned to handler? → wrap with sentinel + `NewBizError`
- [ ] Business logic in `handler.go` or `pgstore.go`? → move to `service.go`
- [ ] Module importing another module? → violates black box rule, use `deps.go`
- [ ] Double-wrapped `BizError`? → check error chain

### Silly bug checklist
- [ ] Off-by-one in area calculation? (use `AreaSqMM()`, don't inline `L*W`)
- [ ] Forgot `defer tx.Rollback(ctx)`?
- [ ] New remnant missing `ParentBoardID` → lineage broken
- [ ] `BoundingBoxLengthMM` default not set? → `FindAvailableRemnants` will silently miss it
- [ ] Forgot to set `CreatedAt: time.Now().UTC()`?
- [ ] `RETURNING` clause missing → ID is nil uuid

### Automated checks (must all pass before PR)
```bash
make test        # go test ./... -race -count=1 — all green
make lint        # golangci-lint — no new warnings
make build       # binary compiles cleanly
go vet ./...     # no module boundary violations
```

If coverage drops below 80% for the changed module:
```bash
go test ./internal/module/<name>/... -cover
```
Add tests until green.

---

## Phase 6 — PR

### Commit message format
```
[module] verb: brief description

- bullet point detail 1
- bullet point detail 2
- BR-* impacted (if any)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

Example:
```
[inventory] feat: add bounding box search filter for remnants

- Add BoundingBoxLengthMM/WidthMM fields to RecordCutInput
- Default to actual dimension when not provided (BR-K04)
- COALESCE in selectAvailableRemnantsByMinDimension query

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

### Branch rules
- Feature branch from `dev`: `git checkout -b feat/[module]-brief-description dev`
- Never push directly to `main` or `dev`
- PR: feature → `dev` (approval optional)
- `dev` → `main` requires 1 approval

### PR body template
```markdown
## Summary
- What was changed and why
- Business rules impacted (BR-* references)

## Technical notes
- Migration: yes/no — describe if yes
- Breaking changes: yes/no

## Test plan
- [ ] `make test` green
- [ ] `make lint` clean
- [ ] Integration tests cover new pgstore methods
- [ ] Coverage ≥ 80% for changed module
- [ ] Tested manually: describe scenario
```
