# Vmarble Warehouse Management Service — Backend

[![CI](https://github.com/giangdq202/Vmarble-Warehouse-Management-Service/actions/workflows/ci.yml/badge.svg)](https://github.com/giangdq202/Vmarble-Warehouse-Management-Service/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-336791?logo=postgresql)](https://www.postgresql.org)
[![License: PolyForm Noncommercial](https://img.shields.io/badge/License-PolyForm%20Noncommercial-blue.svg)](https://polyformproject.org/licenses/noncommercial/1.0.0/)

> **Production manufacturing execution system (MES) running live at a Vietnam-based stone & wood export factory.**
> Built end-to-end as the contracted engineer for a real, paying client. Self-funded hosting, real factory staff using it daily, real customer money on the line — not a school project.

This repo is the backend service. The accompanying [frontend](https://github.com/giangdq202/Vmarble-Warehouse-Management-Client) ships a desktop dashboard (planners, accountants, owner) and a mobile kiosk PWA (shop-floor workers).

> **Note on what's open vs. closed**
> The code is open for technical reference (architecture, patterns, testing, CI/CD). The customer-specific business logic, sales scripts, and operational runbooks live outside this repo by contractual courtesy — code stays public, business stays private.

---

## Production Snapshot

| | |
|---|---|
| **Stage** | Live in production with a paying client |
| **Engagement** | Solo contractor build; full-stack ownership (BE + FE + DB + deploy + on-call) |
| **Users** | Factory office staff (planners / accountants / owner) + floor workers on kiosk tablets |
| **Throughput** | Production orders, work orders, cutting records, scan checkpoints, costing reports — all flowing through this service |
| **Hosting** | Self-funded VPS; Docker compose; Postgres-backed; HTTPS via reverse proxy |
| **Operations** | Deployed via GitHub Actions (CD on push to `dev`); zero-downtime restarts; rolling migrations |
| **Reliability** | Row-level locking on critical writes; idempotent migrations; structured slog; CI gates (race detector + lint) |

Demo / staging URL is shared on request — production URL is held private at the customer's request.

---

## What this service does (high level)

A **modular monolith in Go** for a manufacturing workflow:

```
Sales Order  →  Production Plan  →  Work Order  →  Cutting / Processing
                                                            ↓
                                                       Costing  →  Reports
                                              ↑                       ↑
                                          Barcode scan checkpoints
                                              tracked end-to-end
```

The service owns the entire backend: order intake, planning, inventory tracking with offcut (remnant) lineage, work-order state machine, area-based costing allocation, barcode scan checkpoints, role-based auth, and aggregated dashboards. The frontend is a thin, typed UI over this contract.

> Specific business rules (validation thresholds, costing formulas, allocation strategies) live in private specs by client agreement. The implementation patterns — how those rules are enforced — are visible throughout `internal/module/*/service.go`.

---

## Why this codebase is interesting (engineer-eye view)

| Concern | Choice | Why it matters |
|---|---|---|
| **Architecture** | Modular monolith — domain modules under `internal/module/` are black boxes; cross-module calls go through `deps.go` interfaces wired only in `cmd/server/main.go` | Each module is independently replaceable; no circular imports possible by construction |
| **Persistence** | `pgx/v5` directly, no ORM | Hand-tuned SQL; prepared statements; explicit transaction boundaries on every multi-write |
| **Concurrency safety** | `SELECT … FOR UPDATE` inside `pgx.Tx` on every critical write path | Prevents inventory oversell, capacity overcommit, double-allocation under load |
| **Domain enforcement** | Sentinel errors mapped to HTTP status in `internal/platform/httpkit` | Service layer returns `ErrInvalidTransition` / `ErrInsufficientStock` / `ErrAlreadyFinalized` etc.; HTTP layer does no business logic |
| **Migrations** | `goose` with idempotent Up/Down, sequenced; never edited after merge | Forward and rollback both work in CI and on prod |
| **Auth** | HMAC-signed bearer token middleware + role guard at the route level | Tier helpers (`RequireWorkerUp / RequirePlannerUp / RequireAdminOnly`) keep new endpoints honest |
| **API contract** | OpenAPI generated from handler annotations, regenerated on every commit via pre-commit hook | Frontend regens types from the same spec — no drift |
| **CI/CD** | GitHub Actions: race-detector test + lint + build on PR; auto-build & push image on `dev` push | Every merge is verifiably testable; deploys are reproducible |
| **Testing** | Unit tests with store/deps mocked through interfaces; table-driven; deterministic | `go test ./... -race -count=1` is the gate |

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

Requires Docker 24+, Docker Compose v2, `curl`, `jq`. `make seed` walks two complete order lifecycles end-to-end so you can click through the system without configuring anything by hand. Demo credentials print at the end.

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
| Auth | HMAC bearer token (JWT-style) |
| API Docs | swaggo/swag (OpenAPI 2.0) |
| CI | GitHub Actions — race + lint + build |
| CD | GitHub Actions — Docker image to GHCR on `dev` push |

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
| `make swagger` | Regenerate `docs/` from handler annotations |
| `make migrate-up` | goose up |
| `make migrate-down` | goose down |
| `make docker-up` | `docker compose up -d` |
| `make docker-down` | `docker compose down` |

---

## Project Structure

```
cmd/server/         Entry point — wires every module together
internal/
  domain/           Shared value objects + status enums + BizError sentinel mapping
  module/<name>/    Domain modules (catalog, order, planning, inventory,
                    production, costing, barcode, authn, dashboard, …)
  platform/
    postgres/       pgxpool + goose migration runner
    httpkit/        Router setup, pagination, JSON/error helpers
    auth/           HMAC middleware + persona tier helpers
    config/         Env config loader
migrations/         Sequenced SQL (goose Up/Down)
docs/               Swagger spec, architecture deep-dive, runbooks (public);
                    customer-specific business specs are not committed.
```

Each module follows the same 5-file layering pattern:

```
iface.go    → exported Service interface + input/output DTOs (the contract)
service.go  → business rules, validation, orchestration
store.go    → unexported store interface (repository)
pgstore.go  → PostgreSQL SQL implementation
handler.go  → HTTP handlers + route registration
```

This is enforced by code review, not by a tool — but the pattern is mechanical enough that drift is obvious.

---

## API Documentation

Swagger UI: `http://localhost:8080/swagger/index.html`

Regenerate after handler annotation changes:

```bash
make swagger
```

A pre-commit hook also regenerates and re-stages the spec when Go sources change, so the contract never drifts from the code.

---

## CI/CD

- **CI** (PR to `dev`): `go test -race`, `golangci-lint`, `go build` — see `.github/workflows/ci.yml`.
- **CD** (push to `dev`): build & push Docker image to GitHub Container Registry — see `.github/workflows/cd.yml`.

Branch rules: `main` and `dev` are protected; PRs only.

| Branch | Direct push | PR required | Approvals |
|---|---|---|---|
| `main` | Blocked | Yes | 1 |
| `dev` | Blocked | Yes | 0 (self-merge allowed) |

---

## License

Licensed under [PolyForm Noncommercial 1.0.0](./LICENSE).

- Free for personal use, research, and education.
- Free to fork, modify, and study.
- Commercial use requires a separate license — contact giangdq202@gmail.com.

---

## Tài liệu tiếng Việt

Backend cho hệ thống MES (Manufacturing Execution System) đang chạy production tại nhà máy xuất khẩu đá & gỗ ở Việt Nam. Build end-to-end với vai trò engineer hợp đồng — code mở để tham khảo kỹ thuật, business logic riêng của khách giữ private theo thỏa thuận.

### Cài đặt & Chạy local

```bash
cp .env.example .env
docker compose up --build      # tự migrate + start server
make seed                       # load demo data sau khi server sẵn sàng
open http://localhost:8080/swagger/index.html
```

### Hướng dẫn làm việc với AI Agent

Xem [`CLAUDE.md`](CLAUDE.md) — workflow và skills cho Claude Code agent (chỉ commit khi được yêu cầu, branch convention, persona tier RBAC, v.v.).
