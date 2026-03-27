# Architecture Overview — VMARBLE Warehouse Management Service

## High-level design

The system is a **modular monolith** written in Go 1.25, backed by PostgreSQL 17. Each business domain lives in its own module under `internal/module/`. Modules are isolated black boxes that never import each other directly.

## System diagram

```
                         ┌──────────────────────────────┐
                         │        HTTP Clients           │
                         │   (Web UI, Barcode Scanner)   │
                         └──────────────┬───────────────┘
                                        │
                                        ▼
                         ┌──────────────────────────────┐
                         │     gin HTTP Router           │
                         │  internal/platform/httpkit    │
                         │  + auth middleware            │
                         └──────────────┬───────────────┘
                                        │
            ┌───────────────────────────┼───────────────────────────┐
            ▼                           ▼                           ▼
   ┌─────────────────┐      ┌─────────────────┐      ┌─────────────────┐
   │  catalog module  │      │  order module    │      │ planning module │
   │  SKU, Material   │      │  PO, LineItems   │      │ ProductionPlan  │
   │  BOM             │      │                  │      │ PlanItem        │
   └─────────────────┘      └─────────────────┘      └─────────────────┘
            │                           │                           │
            ▼                           ▼                           ▼
   ┌─────────────────┐      ┌─────────────────┐      ┌─────────────────┐
   │ inventory module │      │production module │      │ costing module  │
   │ BoardSheet,      │◄────│ WorkOrder,       │─────►│ CostingRecord   │
   │ Remnant,         │     │ Consumption      │      │ area-based      │
   │ CuttingRecord    │     │ state machine    │      │ allocation      │
   └─────────────────┘      └─────────────────┘      └─────────────────┘
            │                           │                           │
            └───────────────┬───────────┘───────────────────────────┘
                            ▼
                   ┌─────────────────┐
                   │ barcode module   │
                   │ Barcode,         │
                   │ ScanEvent        │
                   └─────────────────┘
                            │
                            ▼
                   ┌─────────────────┐
                   │   PostgreSQL 17  │
                   │   (pgx/pgxpool)  │
                   └─────────────────┘
```

## Module dependency graph

Modules communicate through dependency interfaces (`deps.go`) wired in `cmd/server/main.go`. No module directly imports another.

```
catalog   ──(no deps)──
order     ──(no deps)── stores sku_id as raw UUID, no catalog validation
planning  ──(no deps)── stores sku_id as raw UUID, no catalog validation
inventory ──(no deps)── stores material_id as raw UUID, no catalog lookup
production──depends on──► planning (plan approval check via PlanChecker)
                        ► catalog  (metal flag via SKUChecker)
costing   ──depends on──► production (work order status via WorkOrderReader)
                        ► DB directly (cutting & consumption data via raw pool adapters)
barcode   ──(no deps)── stores work_order_id as raw UUID, no production lookup
```

## Layering within each module

```
handler.go    ← HTTP binding, auth, error mapping
    │
    ▼
service.go    ← Business rules, validation, orchestration
    │
    ▼
store.go      ← Repository interface (unexported)
    │
    ▼
pgstore.go    ← PostgreSQL implementation (SQL, row mapping)
```

Key rules:
- Business logic lives **only** in `service.go`
- `handler.go` does HTTP concerns only (param binding, response shaping)
- `pgstore.go` does SQL only — no business decisions
- `iface.go` defines the public contract (Service interface + DTOs)

## Shared code

| Package | Purpose | Contains |
|---|---|---|
| `internal/domain/` | Shared value objects & enums | `Dimension`, `Money`, status enums, `BizError` |
| `internal/platform/postgres/` | DB infrastructure | Pool creation, migration runner |
| `internal/platform/httpkit/` | HTTP infrastructure | Router setup, JSON helpers, error mapping |
| `internal/platform/auth/` | Auth middleware | JWT/session validation (placeholder) |
| `internal/platform/config/` | Configuration | Env var parsing via `caarlos0/env` |

## Data flow (end-to-end)

```
1. Purchase Order (PO) created                    → order module
2. Production Plan created from PO line items     → planning module
3. Plan approved → Work Orders generated          → production module
4. CNC cutting of board sheets                    → inventory module (BR-K01–K05)
   - Area conservation validated (BR-K03)
   - Remnants tracked with lineage
5. Processing (edge banding, drilling, etc.)      → production module (BR-P01–P04)
   - Metal requirement checked (BR-P04)
6. Work Order completed                           → production module
7. Costing allocated by area                      → costing module (BR-C01–C04)
   - Finalized records are immutable (BR-C04)
8. Barcode scans at 3 checkpoints                 → barcode module
```

## Key invariants

- **WorkOrder state machine**: `PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED` (monotonic)
- **Area conservation**: `used_area + remnant_area ≤ source_area`
- **Remnant lineage**: parent → child chain supports nested cutting
- **Costing immutability**: finalized costing records cannot be modified
- **Metal requirement**: SKUs with `requires_metal=true` need METAL consumption before completion

## Technology choices

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go 1.25 | Strong typing, fast compilation, stdlib quality |
| Database | PostgreSQL 17 | ACID, JSON support, mature ecosystem |
| HTTP | gin-gonic/gin | Performance, middleware ecosystem |
| DB driver | pgx/v5 (pgxpool) | Native Go driver, connection pooling |
| Migrations | goose/v3 | Simple, SQL-based, up/down support |
| Config | env/v11 | 12-factor app, struct-based env parsing |
| Logging | log/slog (stdlib) | Structured, zero-dependency |
