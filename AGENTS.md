# AGENTS.md — VMARBLE Warehouse Management Service

Cross-tool coding standards shared across Antigravity CLI, Claude Code, and Cursor.
Stack-specific guidance lives in `CLAUDE.md` / `GEMINI.md`; this file covers portable conventions.

## Stack

- Go 1.24, PostgreSQL 17
- Router: `gin-gonic/gin`
- DB: `jackc/pgx/v5` via `pgxpool`
- Migrations: `pressly/goose/v3`
- Config: `caarlos0/env/v11`
- Logging: `log/slog` (stdlib)

## Module boundary (hard constraint)

Modules under `internal/module/<name>/` are black boxes. A module MUST NOT import another module package.
Cross-module calls go through dependency interfaces defined in the consuming module's `deps.go`.
Wire adapters only in `cmd/server/main.go`.

## Layering rules

- `handler.go` — HTTP binding, auth extraction, error → HTTP status mapping. No business logic.
- `service.go` — business rules, validation, orchestration, transactions.
- `pgstore.go` — SQL, row mapping, DB-specific concerns only.

## Coding conventions

- Run `gofmt`; use `goimports` grouping (stdlib → third-party → local).
- Package names: short, lowercase, no underscores. No stutter (`inventory.Service` not `inventory.InventoryService`).
- Export only what is part of a module contract (`iface.go`) or shared domain (`internal/domain/`).
- Accept `context.Context` in all store/service methods that touch IO.
- Never use `context.Background()` in request flows; pass `c.Request.Context()`.
- Prefer explicit names over abbreviations in business logic (`planID` ok, `pID` not).

## Error handling

- Sentinel errors from `internal/domain/errors.go`.
- Wrap with `NewBizError(sentinel, humanMessage)` for domain failures.
- No raw `pgx`/SQL errors to handlers — translate to sentinels.
- HTTP status mapping: `ErrInvalidInput` 400 · `ErrNotFound` 404 · `ErrInvalidTransition`/`ErrAlreadyFinalized` 409 · `ErrPreconditionFailed` 412 · `ErrInsufficientStock`/`ErrAreaConservation` 422.

## Database conventions

- `QueryRow`/`Scan` for single-row reads; check `pgx.ErrNoRows` → `ErrNotFound`.
- Transactions for multi-write operations that must be atomic.
- SQL always parameterized — never string-concatenate user input.
- Use `RETURNING` over follow-up SELECTs.
- `SELECT FOR UPDATE` when reading-then-writing to enforce invariants.

## Domain invariants (never violate)

- WorkOrder state machine: `PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED` (monotonic).
- Area conservation (BR-K03): `used_area + remnant_area <= source_area` — validated in `inventory/service.go`.
- Remnant lineage: `parent_board_id` + optional `parent_remnant_id`.
- Costing finalization (BR-C04): finalized records are immutable.
- Metal requirement (BR-P04): `requires_metal = true` SKUs need a METAL consumption record before `COMPLETED`.

## RBAC persona helpers

Use persona helpers for new endpoints — do not inline `RequireRole(...)`:

```go
auth.RequireWorkerUp()    // warehouse, cnc, cnc_manager, foreman +
auth.RequirePlannerUp()   // planner, accountant +
auth.RequireAdminOnly()   // admin only
```

## Branch rules

- Never push directly to `main` or `dev`.
- Feature branches from `dev`. PR: feature → `dev`, `dev` → `main` (1 approval required).
- Commit subject: ≤ 72 chars, English ASCII, Conventional Commit format (`type(scope): description`).

## Migrations

- Migrations in `migrations/` must have `Up` + `Down`, ordered by sequence number.
- Current next sequence: check latest file in `migrations/` and increment.

## Skills

Project skills live in `.agents/skills/` (Antigravity) and `.claude/skills/` (Claude Code).
Load the relevant skill before implementing any non-trivial task:

| Skill | When to use |
|---|---|
| `senior-workflow` | Any feature/fix — always the outer shell |
| `business-auditor` | Touches BR-* rules or `service.go` logic |
| `integration-architect` | New endpoint, `iface.go` change, or `deps.go` interface |
| `product-manager` | Backlog triage, sprint planning, GitLab issue management |
| `database-migrations` | Adding or modifying files in `migrations/` |
| `golang-patterns` | Idiomatic Go — zero-value, error wrapping, context |
| `golang-testing` | Table-driven tests, race detector, mock patterns |
| `security-review` | OWASP sweep, before merging auth/RBAC/PII changes |
| `rbac-hardener` | Role guard audit, before go-live |
