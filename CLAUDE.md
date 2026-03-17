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
4. Add a `NewHandler(svc) http.Handler` that returns a chi.Router.
5. Wire in `cmd/server/main.go`: create store → service → handler → `r.Mount("/", handler)`.
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
