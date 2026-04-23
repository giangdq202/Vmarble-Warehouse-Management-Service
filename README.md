# Vmarble Warehouse Management Service

[![CI](https://github.com/giangdq202/Vmarble-Warehouse-Management-Service/actions/workflows/ci.yml/badge.svg)](https://github.com/giangdq202/Vmarble-Warehouse-Management-Service/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-336791?logo=postgresql)](https://www.postgresql.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Backend service for a furniture workshop's warehouse and production management system.
Tracks the full lifecycle from Purchase Orders → CNC cutting → Processing → Costing, with remnant optimisation and barcode scan checkpoints.

---

## Architecture

A **modular monolith** in Go. Nine domain modules live under `internal/module/`, each a self-contained black box with its own `iface.go / service.go / store.go / pgstore.go / handler.go`. Modules never import each other — cross-module calls go through dependency interfaces wired in `cmd/server/main.go`.

```
HTTP Clients (Web UI / Barcode Scanner)
          │
          ▼
 ┌─────────────────────────────────────────────────┐
 │  gin HTTP Router  +  JWT auth middleware         │
 │  internal/platform/httpkit + auth               │
 └────────────────────┬────────────────────────────┘
                      │
   ┌──────────────────┼──────────────────┐
   ▼                  ▼                  ▼
catalog            order            planning
SKU/Material/BOM   PO/LineItems     ProductionPlan
   │                  │                  │
   └──────────────────┼──────────────────┘
                      │
   ┌──────────────────┼──────────────────┐
   ▼                  ▼                  ▼
inventory        production          costing
BoardSheet        WorkOrder         CostingRecord
Remnant         ConsumptionRecord   area-based
CuttingRecord   state machine       allocation
   │                  │                  │
   └──────────────────┼──────────────────┘
                      │
              ┌───────┼───────┐
              ▼               ▼
           barcode         authn / dashboard
           Scan events     JWT + stats
              │
              ▼
       PostgreSQL 17  (pgx/pgxpool)
```

Key technical properties:
- **Row-level locking**: critical writes use `SELECT … FOR UPDATE` inside `pgx.Tx` to prevent inventory oversell and capacity overcommit — see `recordCutAtomically`, `allocateRemnantAtomically`, `assignSlotAtomically`
- **Domain invariants enforced in service layer**: area conservation (BR-K03), work order state machine (BR-P01-P04), costing immutability (BR-C04)
- **No ORM**: all SQL written by hand with `pgx/v5`; prepared statements and parameterised queries throughout
- **32 ordered migrations** managed by goose with idempotent Up/Down

---

## Quick Start (3 commands)

```bash
# 1. Copy env and start Postgres + app
cp .env.example .env
docker compose up --build

# 2. Seed demo data (in a separate terminal once the server is healthy)
make seed

# 3. Open Swagger UI
open http://localhost:8080/swagger/index.html
```

> Requires: Docker 24+, Docker Compose v2, `curl`, `jq`.
>
> `make seed` creates two complete work order lifecycles end-to-end:
> SKUs → BOM → PO → Approved Plan → Work Orders → Board Sheet stock →
> CNC Cutting → Processing → Costing → Barcode scans.
> Demo credentials are printed at the end.

---

## Domain Modules

| Module | Entities | Key rules |
|---|---|---|
| `catalog` | SKU, Material, BOM | Foundation data; referenced by all modules |
| `order` | PurchaseOrder, POLineItem | PO lifecycle |
| `planning` | ProductionPlan, PlanItem | DRAFT → APPROVED → CANCELED |
| `inventory` | InventoryLot, BoardSheet, Remnant, CuttingRecord | BR-K01–K05: stock, area conservation, remnant lineage, cycle count, bin transfer |
| `production` | WorkOrder, ConsumptionRecord, Machine, ShiftSlot | BR-P01–P04: state machine, metal check, capacity-aware scheduling |
| `costing` | CostingRecord | BR-C01–C04: area-based cost allocation, finalisation lock |
| `barcode` | Barcode, ScanEvent | 3 scan checkpoints with actor identity |
| `authn` | JWT | Role-based auth (admin, warehouse_staff, cnc_manager, accountant) |
| `dashboard` | — | Aggregated stats endpoints |

---

## Tech Stack

| Concern | Choice |
|---|---|
| Language | Go 1.24 |
| Database | PostgreSQL 17 |
| HTTP Router | gin-gonic/gin v1.10 |
| DB Driver | jackc/pgx v5 (pgxpool, no ORM) |
| Migrations | pressly/goose v3 |
| Config | caarlos0/env v11 (12-factor) |
| Logging | log/slog (stdlib, structured JSON) |
| Auth | JWT (HMAC-SHA256) |
| API Docs | swaggo/swag (OpenAPI 2.0) |

---

## Development Commands

| Command | Description |
|---|---|
| `make dev` | docker-up + migrate + run |
| `make run` | `go run ./cmd/server` |
| `make build` | `go build -o bin/warehouse-server` |
| `make test` | `go test ./... -race -count=1` |
| `make lint` | golangci-lint |
| `make seed` | Load end-to-end demo data (requires server running) |
| `make swagger` | regenerate `docs/` from annotations |
| `make migrate-up` | goose up |
| `make migrate-down` | goose down |
| `make docker-up` | `docker compose up -d` |
| `make docker-down` | `docker compose down` |

---

## Project Structure

```
cmd/server/         Entry point — wires all modules together
internal/
  domain/           Shared value objects: Dimension, Money, status enums, BizError
  module/
    catalog/        SKU, Material, BOM
    order/          Purchase Order, Line Items
    planning/       Production Plan
    inventory/      BoardSheet, Remnant, CuttingRecord, cycle count, bin transfer
    production/     WorkOrder, ConsumptionRecord, machine capacity scheduling
    costing/        CostingRecord (area-based allocation)
    barcode/        Barcode/QR, ScanEvent
    authn/          JWT authentication
    dashboard/      Stats aggregation
  platform/
    postgres/       pgxpool creation + goose migration runner
    httpkit/        Router setup, pagination, JSON helpers, error mapping
    auth/           Auth middleware + context helpers
    config/         Env config loader
migrations/         32 SQL migration files (goose Up/Down)
docs/               Swagger spec, architecture, business logic spec (Vietnamese)
```

Each module follows the same 5-file layering pattern:

```
iface.go    → exported Service interface + input/output DTOs (the contract)
service.go  → business rules, validation, orchestration
store.go    → unexported store interface (repository)
pgstore.go  → PostgreSQL SQL implementation
handler.go  → HTTP handlers + route registration
```

---

## API Documentation

Swagger UI: `http://localhost:8080/swagger/index.html`

Regenerate after changing handler annotations:
```bash
make swagger
```

---

## Business Logic Spec

Full Vietnamese specification: [`docs/backend-business-logic-vi.md`](docs/backend-business-logic-vi.md)

Architecture deep-dive: [`docs/architecture.md`](docs/architecture.md)

---

## CI/CD

- **CI** (Pull Request to `dev`): `go test -race`, `golangci-lint`, `go build` — see `.github/workflows/ci.yml`
- **CD** (Push to `dev`): builds + pushes Docker image to GitHub Container Registry — see `.github/workflows/cd.yml`

---

## Branch Rules

| Branch | Direct push | PR required | Approvals |
|---|---|---|---|
| `main` | Blocked | Yes | 1 |
| `dev` | Blocked | Yes | 0 (self-merge allowed) |

---

## License

[MIT](LICENSE)

---

## Tài liệu tiếng Việt

Backend cho hệ thống quản lý kho và sản xuất carcass tại xưởng gỗ.

### Mục tiêu dự án

- Quản lý luồng từ đơn hàng (PO) → kế hoạch sản xuất → cắt CNC → gia công → tính giá thành.
- Theo dõi remnant (vật tư dư) với lineage đệ quy để tối ưu sử dụng ván ép.
- Lịch ca sản xuất và phân công máy CNC (capacity-aware scheduling).
- Cycle count và kiểm kê kho với audit log.
- Chuẩn hóa dữ liệu để kế toán tính costing chính xác theo diện tích sử dụng (BR-C02/C03).

### Cài đặt & Chạy

```bash
# 1. Copy env
cp .env.example .env

# 2. Khởi động Postgres + app (docker compose tự migrate và start server)
docker compose up --build

# 3. Load demo data
make seed

# Hoặc chạy từng bước thủ công:
make docker-up      # chỉ postgres
make migrate-up
make run
make seed           # sau khi server sẵn sàng
```

### Hướng dẫn làm việc với AI Agent

Xem [`CLAUDE.md`](CLAUDE.md) — hướng dẫn workflow và skills cho Claude Code agent.
