# VMARBLE Warehouse Management Service — Agent Guide

## Source documents (read these first)

- `README.md` — project goals, stack, commands, structure, branch rules
- `docs/backend-business-logic-vi.md` — Vietnamese business spec (authority for flow, entities, and business rules)

When there is any conflict, prefer `docs/backend-business-logic-vi.md`.

## Stack

- Go 1.24, PostgreSQL 17
- Router: `gin-gonic/gin`
- DB: `jackc/pgx/v5` via `pgxpool`
- Migrations: `pressly/goose/v3`
- Config: `caarlos0/env/v11`
- Logging: `log/slog` (stdlib)

## Commands

```bash
make dev          # docker-up + migrate-up + run
make run          # go run ./cmd/server
make build        # go build -o bin/warehouse-server ./cmd/server
make test         # go test ./... -race -count=1
make lint         # golangci-lint run ./...
make migrate-up   # goose up
make migrate-down # goose down
```

## Architecture: modular monolith

Seven domain modules under `internal/module/`, each a self-contained black box:

| Module | Owns | Key business rules |
|---|---|---|
| `catalog` | SKU, Material, BOM | Foundation data; no upstream deps |
| `order` | PO, POLineItem | Validates SKU refs |
| `planning` | ProductionPlan, PlanItem | DRAFT → APPROVED → CANCELED status |
| `inventory` | InventoryLot, BoardSheet, Remnant, CuttingRecord | BR-K01 to BR-K05: stock checks, area conservation, remnant lineage |
| `production` | WorkOrder, ConsumptionRecord | BR-P01 to BR-P04: state machine, metal check |
| `costing` | CostingRecord | BR-C01 to BR-C04: area-based allocation, finalization lock |
| `barcode` | Barcode, ScanEvent | 3 scan checkpoints |

### Module file pattern

Every module follows the same 5-file structure:

- `iface.go` — exported `Service` interface + input/output DTOs (the contract)
- `service.go` — business logic, validation, rules
- `store.go` — unexported `store` interface (repository)
- `pgstore.go` — Postgres implementation
- `handler.go` — HTTP handlers, route registration

Some modules add `deps.go` for cross-module dependency interfaces (exported).

### How modules communicate

Modules never import each other. Cross-module dependencies are defined as local interfaces in the consuming module (`deps.go`). Thin adapters in `cmd/server/main.go` bridge module boundaries. This keeps every module independently replaceable.

### Shared code (`internal/domain/`)

Only contains value objects and enums shared across modules:
- `Dimension`, `Money` — value types
- `WorkOrderStatus`, `RemnantStatus`, `PlanStatus` — status enums with transition validation
- `BizError` — domain error wrapping for HTTP status mapping

`internal/platform/` contains infrastructure (postgres, httpkit, auth, config) — never domain logic.

## Coding conventions (must follow)

### Go formatting, imports, naming

- Run `gofmt` on all Go files; keep diffs minimal and focused.
- Use `goimports`-style grouping (stdlib, third-party, local) and remove unused imports.
- Package names: short, lowercase, no underscores. Avoid stutter (e.g. `inventory.InventoryService` → prefer `inventory.Service`).
- Export only what is part of a module contract (`iface.go`) or shared domain (`internal/domain/`). Keep everything else unexported.
- Prefer explicit names over abbreviations in business logic (`planID` ok, `pID` not).

### Module boundaries and layering

- Modules under `internal/module/<name>/` are black boxes. A module must not import another module package.
- Cross-module calls must go through dependency interfaces defined in the consuming module’s `deps.go`. Wire adapters only in `cmd/server/main.go`.
- Keep responsibilities separated:
  - `handler.go`: HTTP binding (params/body), auth/claims extraction, mapping errors to HTTP, response shaping.
  - `service.go`: business rules, validation, orchestration, transactions.
  - `pgstore.go`: SQL, row mapping, DB-specific concerns (queries, constraints).
- Do not put business rules in `pgstore.go` or `internal/platform/`.

### Context, time, and determinism

- Always accept a `context.Context` in store/service methods that touch IO; handlers must pass `c.Request.Context()`.
- Do not use `context.Background()` in request flows.
- Prefer `time.Now()` only at the edges; if you need determinism in tests, inject a clock into service.

### Errors and logging

- Use sentinel errors from `internal/domain/errors.go` and wrap with `NewBizError(sentinel, humanMessage)` for domain/business failures.
- Do not return raw `pgx`/SQL errors to handlers; translate to sentinel errors where appropriate (e.g. unique violation → `ErrInvalidInput`/`ErrPreconditionFailed` depending on semantics).
- Add context to errors with wrapping (`fmt.Errorf("...: %w", err)`), but avoid double-wrapping `BizError`.
- Logging:
  - Use `log/slog` with structured fields; no `fmt.Printf` in server code.
  - Do not log PII/secrets (tokens, passwords, full request bodies).
  - Prefer logging once at the boundary (HTTP middleware/handler) rather than inside tight loops.

### Validation and DTOs

- Validate inputs in `service.go` (business validation) and only do basic shape checks in handlers (missing required fields, malformed JSON).
- Use explicit input/output DTOs in `iface.go`; avoid leaking DB models/row structs outside `pgstore.go`.

### Database usage (pgx/pgxpool)

- Prefer `QueryRow`/`Scan` for single-row reads; always check `pgx.ErrNoRows` and map to `ErrNotFound`.
- Use transactions for multi-write operations that must be atomic (especially when enforcing invariants like area conservation).
- Keep SQL readable and parameterized; never build SQL by string concatenation with user input.
- Prefer `RETURNING` over follow-up selects when you need created IDs/timestamps.

### Migrations (goose)

- Migrations must be forward/backward safe (`Up` and `Down`), ordered by sequence, and idempotent where possible.
- Add/modify constraints to enforce invariants at the DB layer when feasible (FKs, CHECKs), but keep authoritative business rules in services.

### HTTP conventions (gin)

- Keep route registration in `handler.go` and expose a constructor + registration method (e.g. `NewHandler(svc).Register(rg)`).
- Use consistent JSON envelope and error responses via `internal/platform/httpkit` (do not hand-roll).
- Return appropriate statuses: 400 invalid input, 404 not found, 409 invalid transition/finalized, 412 precondition, 422 business constraint (stock/area).

### Testing and linting

- Tests must be deterministic and table-driven where practical; avoid time-dependent sleeps.
- Prefer unit tests for `service.go` with store/deps mocked via interfaces; keep integration tests for `pgstore.go` focused.
- Keep `make lint` clean; do not ignore linter findings unless there is a documented reason.

## Branch rules (must follow)

- **Never push directly to `main` or `dev`.**
- Create feature branches from `dev`.
- PRs: feature → `dev` (approval optional), `dev` → `main` (requires 1 approval).
- No force-push to protected branches.

## Domain invariants (do not violate)

### WorkOrder state machine

`PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED`

- Transitions are monotonic and validated in `production/service.go`.
- Costing only permitted when status = `COMPLETED`.

### Remnant lineage

- Stores `parent_board_id` + optional `parent_remnant_id`.
- Supports recursive/nested cutting.
- Status lifecycle: `AVAILABLE → ALLOCATED → CONSUMED` or `AVAILABLE → WASTE`.

### Area conservation (BR-K03)

`used_area + remnant_area <= source_area`

Validated in `inventory/service.go` RecordCut before any DB writes.

### Costing allocation (BR-C02/C03)

`cost_for_sku = (area_used / total_sheet_area) * sheet_cost`

Waste is not allocated to SKUs. Finalized records are immutable (BR-C04).

### Metal requirement (BR-P04)

SKUs with `requires_metal = true` must have a METAL consumption record before WorkOrder can transition to COMPLETED.

## Error conventions

Sentinel errors in `domain/errors.go` map to HTTP status codes in `httpkit.Error()`:

| Sentinel | HTTP Status |
|---|---|
| `ErrNotFound` | 404 |
| `ErrInvalidInput` | 400 |
| `ErrInsufficientStock` | 422 |
| `ErrAreaConservation` | 422 |
| `ErrInvalidTransition` | 409 |
| `ErrAlreadyFinalized` | 409 |
| `ErrPreconditionFailed` | 412 |

Always wrap with `NewBizError(sentinel, humanMessage)`.

## Adding a new module

1. Create `internal/module/<name>/` with the 5-file pattern.
2. If it depends on other modules, define dependency interfaces in `deps.go` (exported).
3. Add a `NewPGStore(pool)` and `NewService(store, ...deps)` constructor.
4. Add a `NewHandler(svc)` and a `Register(rg *gin.RouterGroup)` method in `handler.go`.
5. Wire in `cmd/server/main.go`: create store → service → handler → `handler.Register(api)` (where `api` is a `*gin.RouterGroup`).
6. Add migration(s) in `migrations/` with the next sequence number.

## Open business decisions (blockers for automation)

From spec section 8 — confirm before implementing smart algorithms:

- Remnant selection strategy: FIFO vs Best Fit vs manual
- Costing allocation rule: area vs weight vs custom
- Labor costing method
- Barcode printing trigger points
- Vendor/purchasing module scope
- Workforce/shift management scope
- Stone workshop shared entities

## PR expectations

- Title: `[module] brief description`
- Body must include: Summary, Business rule(s) impacted (BR-*), Test plan.
- Do not merge changes that weaken domain invariants.

---

## Skills

| Skill | Auto-activates when… |
|-------|----------------------|
| `senior-workflow` | Starting ANY non-trivial Go feature or fix — always run Phases 1–6 in order |
| `business-auditor` | Task body mentions a BR-* rule, or touches `service.go` business logic |
| `product-manager` | Backlog management, sprint planning, creating/triaging GitHub issues |
| `integration-architect` | New endpoint, API contract change, new cross-module dependency (`deps.go`) |
| `deploy` | User says "deploy", "docker", "CI/CD", "go live", "staging", "production", "health check" |
| `rbac-hardener` | User says "phân quyền", "rbac", "role guard", "access control", "security audit", or before go-live |

> **Rule**: `business-auditor` and `integration-architect` run *inside* `senior-workflow` — they do not replace it. `senior-workflow` is always the outer shell. `rbac-hardener` should run at least once before first production deploy.

---

## Automation Workflow

**Trigger**: user says "Làm task tiếp theo" / "Start next task" / picks an issue from the GitHub Projects Kanban board.

1. **Fetch** — invoke `product-manager` skill to identify the highest-priority open issue:
   ```bash
   gh issue list --repo giangdq202/Vmarble-Warehouse-Management-Service \
     --assignee @me --state open --json number,title,labels \
     | jq 'sort_by(.labels[].name) | .[0]'
   ```
2. **Analyze** — read the full requirement and DoD:
   ```bash
   gh issue view <number> --repo giangdq202/Vmarble-Warehouse-Management-Service
   ```
3. **Audit** — invoke `business-auditor` skill: cross-reference the task against `docs/backend-business-logic-vi.md` and identify all BR-* rules it touches. Block implementation if a rule is unclear.
4. **Implement** — activate `senior-workflow` skill and start at **Phase 1: Requirements Clarification**. Do not skip to coding.
5. **Architect** — invoke `integration-architect` skill if the task introduces a new endpoint, modifies an existing DTO in `iface.go`, or adds a new `deps.go` interface. Verify contract consistency with the frontend before merging.
