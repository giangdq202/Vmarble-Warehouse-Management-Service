# Runbook: Staging / Production CD Pipeline (Dokploy + GitHub Actions)

## Tổng quan

Pipeline CD tự động deploy backend (`vwms-backend`) lên Dokploy mỗi khi có push (merge PR) vào nhánh `dev` hoặc `main`.

| | |
|---|---|
| **Trigger** | Push vào nhánh `dev` hoặc `main` |
| **Deploy Engine** | Dokploy (quản lý container, env, logs, health checks) |
| **Image registry** | GitHub Container Registry (GHCR: `ghcr.io/529-studio/vmarble-warehouse-management-service`) |
| **Workflow file** | `.github/workflows/deploy.yml` |
| **Dokploy Webhook** | `secrets.DOKPLOY_BE_WEBHOOK_URL` (`https://dokploy.529studio.site/api/deploy/compose/...`) |

---

## 1. Kiến Trúc Deploy & Luồng Tự Động

Hệ thống sử dụng **Dokploy Webhook Trigger** và **GHCR Immutable Tags**:

```
Push / Merge PR vào dev (hoặc main)
      │
      ▼
Job: Build & Push (.github/workflows/deploy.yml)
  ① docker build (multi-stage, alpine + curl)
  ② docker push tags lên ghcr.io:
     - ghcr.io/529-studio/...:sha-<short> (Immutable Tag dùng để truy vết/rollback)
     - ghcr.io/529-studio/...:stg-latest (hoặc :latest)
      │
      ▼
Job: Trigger Dokploy Webhook
  ① Gọi HTTP POST: curl -X POST "${{ secrets.DOKPLOY_BE_WEBHOOK_URL }}"
  ② Dokploy nhận webhook ➔ tự động pull image mới (`stg-latest` / `latest`)
  ③ Dokploy thực hiện Zero-downtime Rolling Restart container `vwms-backend`
```

---

## 2. GitHub Secrets & Variables

Vào **GitHub repo `529-studio/Vmarble-Warehouse-Management-Service` → Settings → Secrets and variables → Actions** để quản lý:

| Tên Secret | Loại | Mô tả |
|---|---|---|
| `DOKPLOY_BE_WEBHOOK_URL` | Repository Secret | Webhook URL lấy từ Dokploy UI (`vwms-backend` → Webhook Tab) |

> *Lưu ý:* Biến môi trường lúc ứng dụng chạy (`DATABASE_URL`, `AUTH_SECRET`, `PORT`, `LOG_LEVEL`...) được quản lý trực tiếp tại giao diện Dokploy Environment, **không** cần thêm vào GitHub Actions Secrets.

---

## 3. Cấu Hình Raw Compose trên Dokploy (`Source of Truth`)

Mẫu chuẩn Raw Compose cho `vwms-backend` trên Dokploy (lưu tại `deploy/dokploy/compose.yaml`):

```yaml
services:
  vwms-backend:
    image: ghcr.io/529-studio/vmarble-warehouse-management-service:stg-latest
    restart: unless-stopped
    ports:
      - "${PORT:-8080}:8080"
    environment:
      DATABASE_URL: ${DATABASE_URL}
      AUTH_SECRET: ${AUTH_SECRET}
      PORT: "${PORT:-8080}"
      LOG_LEVEL: "${LOG_LEVEL:-info}"
      REMNANT_ALLOC_TIMEOUT: "${REMNANT_ALLOC_TIMEOUT:-24h}"
      REMNANT_ALLOC_CHECK_INTERVAL: "${REMNANT_ALLOC_CHECK_INTERVAL:-1h}"
      REMNANT_OVERFLOW_THRESHOLD_PCT: "${REMNANT_OVERFLOW_THRESHOLD_PCT:-15}"
      CONTAINER_CBM_OVERHEAD_PCT: "${CONTAINER_CBM_OVERHEAD_PCT:-5}"
    healthcheck:
      test: ["CMD", "curl", "-fsS", "http://localhost:8080/healthz"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
```

---

## 4. Rollback & Xử lý Database

### Rollback Container trên Dokploy
Trong trường hợp cần rollback nhanh về một bản build cũ:
1. Vào **Dokploy UI → Application/Compose `vwms-backend` → Deployments**.
2. Hoặc chỉnh trực tiếp `image:` trong ô Raw Compose sang tag SHA cũ (ví dụ: `ghcr.io/529-studio/vmarble-warehouse-management-service:sha-a1b2c3d`) và bấm **Deploy**.

### Triết lý Database ("Fix-forward" & "Append-only Schema")
1. **Không chạy `migrate down` tự động:** Đảm bảo an toàn dữ liệu và truy vết.
2. **Append-only (Chỉ thêm, không xóa/sửa):** Trong các migration, ưu tiên thêm cột mới thay vì xóa/sửa cột cũ để đảm bảo tính tương thích ngược (Backward Compatibility) khi cần rollback code app.
