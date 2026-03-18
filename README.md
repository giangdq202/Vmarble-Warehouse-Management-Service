# Vmarble Warehouse Management Service

Backend cho hệ thống quản lý kho và sản xuất carcass tại xưởng gỗ.

## Mục tiêu dự án

- Quản lý luồng từ đơn hàng (PO) đến sản xuất, nhập xuất kho và tính giá thành.
- Theo dõi remnant (vật tư dư) để tối ưu sử dụng ván ép và giảm hao hụt.
- Chuẩn hóa dữ liệu để kế toán có thể tính costing chính xác theo từng SKU.

## Phạm vi hiện tại (MVP)

- PO và kế hoạch sản xuất.
- Kho vật tư: plywood, vật tư phụ, metal.
- Work Order và trạng thái sản xuất.
- Costing cơ bản theo SKU.
- Barcode/QR tracking.

## Tech Stack

- **Language**: Go 1.24
- **Database**: PostgreSQL 17
- **Router**: gin-gonic/gin v1.10
- **DB driver**: jackc/pgx v5
- **Migrations**: pressly/goose v3
- **Config**: caarlos0/env v11

## Cài đặt & Chạy

```bash
# 1. Khởi động Postgres
make docker-up

# (Khuyến nghị) 1.5. Tạo file env local
cp .env.example .env

# 2. Chạy migration + server
make dev

# 3. Hoặc chạy riêng
make migrate-up
make run
```

Server mặc định chạy tại `http://localhost:8080`. Health check: `GET /healthz`.

## API Docs (Swagger)

- Swagger UI: `http://localhost:8080/swagger/index.html`
- Regenerate swagger spec:

```bash
make swagger
```

## Lệnh phát triển

| Lệnh | Mô tả |
|---|---|
| `make dev` | Docker + migrate + run |
| `make run` | Chạy server |
| `make build` | Build binary |
| `make test` | Chạy tests |
| `make lint` | Chạy linter |
| `make swagger` | Generate swagger spec (docs/) |
| `make migrate-up` | Chạy migration lên |
| `make migrate-down` | Rollback migration |
| `make migrate-create` | Tạo migration mới |
| `make docker-up` | Bật docker compose |
| `make docker-down` | Tắt docker compose |

## Cấu trúc dự án

```
cmd/server/         Entry point, wiring tất cả modules
internal/
  domain/           Shared primitives (Dimension, Money, Status enums, Errors)
  module/
    catalog/        SKU, Material, BOM
    order/          Purchase Order, Line Items
    planning/       Production Plan
    inventory/      Kho, BoardSheet, Remnant, CuttingRecord
    production/     WorkOrder, ConsumptionRecord
    costing/        CostingRecord
    barcode/        Barcode/QR, ScanEvent
  platform/
    postgres/       DB pool + migration runner
    httpkit/        Router, JSON helpers, error mapping
    auth/           Auth middleware (placeholder)
    config/         Env config loader
migrations/         SQL migration files (goose)
```

Mỗi module tuân theo pattern: `iface.go` (interface + DTOs) → `service.go` (business logic) → `store.go` (repo interface) → `pgstore.go` (Postgres) → `handler.go` (HTTP).

## Tài liệu nghiệp vụ

- Xem tài liệu chi tiết tại [docs/backend-business-logic-vi.md](docs/backend-business-logic-vi.md).

## Hướng dẫn làm việc (Claude / Agent)

- Xem hướng dẫn cho AI agent và contributors tại [CLAUDE.md](CLAUDE.md).

## Branch Rules

Quy tắc nhánh áp dụng chung:

### Nhánh `main` (production)

- Cấm push trực tiếp.
- Bắt buộc tạo Pull Request (PR) để merge.
- Bắt buộc có ít nhất 1 approve trước khi merge.
- Chặn force push.

### Nhánh `dev` (integration)

- Cấm push trực tiếp.
- Bắt buộc tạo PR để merge.
- Không yêu cầu approve (có thể tự review và merge để giữ tốc độ).
- Chặn force push.

## Quy trình làm việc nhanh

1. Tạo nhánh tính năng từ `dev`.
2. Hoàn thành code và push nhánh tính năng.
3. Tạo PR vào `dev` và merge khi đạt yêu cầu kiểm tra.
4. Khi đủ tính năng release, tạo PR từ `dev` sang `main`.
5. Có approve hợp lệ rồi mới merge vào `main`.
