---
name: code-review
description: Use to review Go backend changes for correctness, security, modular-monolith boundaries, error handling, SQL safety, domain invariants, and missing tests. Trigger when the user asks for code review, PR review, audit findings, or correctness/security review.
---

# Code Review Skill

You are reviewing Go code in the VMARBLE Warehouse Management Service (modular monolith).

## Review checklist

### Architecture & Module boundaries
- [ ] Module does NOT import another module package (black box rule)
- [ ] Cross-module deps use interfaces in `deps.go`, wired only in `main.go`
- [ ] Business logic is in `service.go`, NOT in `handler.go` or `pgstore.go`
- [ ] HTTP concerns stay in `handler.go` (param binding, error mapping)
- [ ] SQL stays in `pgstore.go` (queries, row mapping)

### Go conventions
- [ ] `gofmt` formatted
- [ ] Import grouping: stdlib → third-party → local
- [ ] No stutter in names (e.g., `inventory.Service` not `inventory.InventoryService`)
- [ ] Only `iface.go` exports and `internal/domain/` are exported; rest unexported
- [ ] Explicit names in business logic (`planID` not `pID`)

### Error handling
- [ ] Uses sentinel errors from `internal/domain/errors.go`
- [ ] Wraps with `NewBizError(sentinel, humanMessage)` for domain failures
- [ ] No raw `pgx`/SQL errors leaked to handlers
- [ ] Error wrapping with `fmt.Errorf("...: %w", err)`, no double-wrapping BizError
- [ ] Correct HTTP status mapping (400/404/409/412/422)

### Context & IO
- [ ] `context.Context` accepted in store/service methods touching IO
- [ ] Handlers pass `c.Request.Context()`
- [ ] No `context.Background()` in request flows

### Database
- [ ] `QueryRow`/`Scan` for single-row; checks `pgx.ErrNoRows` → `ErrNotFound`
- [ ] Transactions for multi-write atomicity
- [ ] SQL parameterized, no string concatenation with user input
- [ ] `RETURNING` preferred over follow-up selects

### Domain invariants (must NOT violate)
- [ ] WorkOrder state machine: `PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED`
- [ ] Area conservation: `used_area + remnant_area <= source_area`
- [ ] Remnant lineage: `parent_board_id` + optional `parent_remnant_id`
- [ ] Costing immutability: finalized records are immutable (BR-C04)
- [ ] Metal requirement: `requires_metal=true` needs METAL consumption before COMPLETED

### Testing
- [ ] Table-driven tests where practical
- [ ] Deterministic (no sleeps, no time dependencies)
- [ ] Unit tests for `service.go` with mocked store/deps
- [ ] `make lint` clean

## Output format

Provide findings as:
1. **Critical** — must fix (invariant violations, data loss risks)
2. **Important** — should fix (convention violations, error handling gaps)
3. **Suggestion** — nice to have (readability, naming improvements)

Include file:line references and suggested fixes.
