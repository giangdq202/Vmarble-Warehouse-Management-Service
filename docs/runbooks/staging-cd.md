# Runbook: Staging CD Pipeline

## Tổng quan

Pipeline CD tự động deploy BE lên staging server mỗi khi có push (merge PR) vào nhánh `dev`.

| | |
|---|---|
| **Trigger** | Push vào nhánh `dev` |
| **Staging server** | Xem secret `STAGING_HOST` trong GitHub Actions |
| **Image registry** | GitHub Container Registry (GHCR) |
| **Workflow file** | `.github/workflows/staging.yml` |
| **Deploy script** | `/opt/vwms-staging/deploy.sh` (trên server) |

---

## Setup server (1 lần duy nhất)

### 1. Cài Docker

```bash
ssh root@$STAGING_HOST

curl -fsSL https://get.docker.com | sh
systemctl enable --now docker
```

### 2. Tạo thư mục staging

```bash
mkdir -p /opt/vwms-staging
cd /opt/vwms-staging
```

### 3. Upload files từ repo

```bash
# Chạy từ máy local
scp deploy/staging/docker-compose.staging.yml root@$STAGING_HOST:/opt/vwms-staging/
scp deploy/staging/deploy.sh                  root@$STAGING_HOST:/opt/vwms-staging/
scp deploy/staging/rollback-db.sh             root@$STAGING_HOST:/opt/vwms-staging/
ssh root@$STAGING_HOST "chmod +x /opt/vwms-staging/*.sh"
```

### 4. Tạo file `.env.staging`

```bash
ssh root@$STAGING_HOST

cat > /opt/vwms-staging/.env.staging <<'EOF'
DATABASE_URL=postgres://vmarble:STRONG_PASSWORD@postgres:5432/vmarble_staging?sslmode=disable
PORT=8080
LOG_LEVEL=info
POSTGRES_USER=vmarble
POSTGRES_PASSWORD=STRONG_PASSWORD
POSTGRES_DB=vmarble_staging
EOF
chmod 600 /opt/vwms-staging/.env.staging
```

### 5. Login GitHub Container Registry (GHCR)

Tạo GitHub PAT tại: **GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens**
Scope cần thiết: `read:packages`

```bash
echo "YOUR_GITHUB_PAT" | docker login ghcr.io -u giangdq202 --password-stdin
```

### 6. Tạo SSH deploy key cho GitHub Actions

```bash
# Trên server
ssh-keygen -t ed25519 -f /tmp/gh_deploy_key -C "github-actions-staging" -N ""

# Thêm public key vào authorized_keys
cat /tmp/gh_deploy_key.pub >> ~/.ssh/authorized_keys

# In private key ra để copy vào GitHub Secret
cat /tmp/gh_deploy_key
```

### 7. Khởi động PostgreSQL lần đầu

```bash
cd /opt/vwms-staging
docker compose -f docker-compose.staging.yml --env-file .env.staging up -d postgres
```

---

## GitHub Secrets & Variables

Vào **GitHub repo → Settings → Secrets and variables → Actions** và thêm:

| Tên | Loại | Giá trị |
|---|---|---|
| `DISCORD_WEBHOOK_URL` | Secret | Webhook URL của Discord channel |
| `STAGING_HOST` | Secret | IP của staging server |
| `STAGING_SSH_KEY` | Secret | Nội dung private key từ bước 6 |
| `STAGING_URL` | **Variable** | `http://<staging-host>:8080` |

> **Lấy Discord Webhook:** Discord Server → Channel Settings → Integrations → Webhooks → New Webhook → Copy URL

---

## Luồng CD tự động

```
Merge PR vào dev
      │
      ▼
Job: Build & Push
  ① docker build (multi-stage, alpine)
  ② docker push → ghcr.io/giangdq202/...:sha-<short>
  ③ Discord: 🔨 Build OK / ❌ Build FAIL + link log
      │ (chỉ tiếp tục nếu build thành công)
      ▼
Job: Deploy Staging
  ① SSH vào staging server ($STAGING_HOST)
  ② /opt/vwms-staging/deploy.sh <image>
     - Pull image mới
     - docker compose restart app (postgres không bị ảnh hưởng)
     - Health check /healthz mỗi 5s, tối đa 60s
     - OK  → lưu state, exit 0
     - FAIL→ rollback image cũ, exit 1
  ③ Discord: 🚀 Deploy OK / 💥 Deploy FAIL + đã rollback
```

---

## Trigger deploy thủ công

Nếu cần deploy lại mà không có code mới:

```bash
# Trên GitHub: Actions → Staging CD → Re-run jobs
# Hoặc tạo empty commit:
git commit --allow-empty -m "chore: trigger staging redeploy"
git push origin dev
```

---

## Xem logs deploy trên server

```bash
ssh root@$STAGING_HOST

# Logs của app container
docker logs vwms-staging-app-1 --tail 100 -f

# Logs của postgres
docker logs vwms-staging-postgres-1 --tail 50

# Trạng thái deploy gần nhất
cat /opt/vwms-staging/.last_good_image
```

---

## Rollback thủ công

### Rollback app (code)

```bash
ssh root@$STAGING_HOST
cd /opt/vwms-staging

# Xem image đang chạy
cat .last_good_image

# Rollback về image cụ thể
APP_IMAGE="ghcr.io/giangdq202/vmarble-warehouse-managment-service:sha-<prev>" \
  docker compose -f docker-compose.staging.yml --env-file .env.staging up -d app
```

### Xử lý lỗi Database Migration (Fix-forward)

Theo triết lý phát triển của dự án, chúng ta **không thực hiện rollback database** (lệnh `down`) trên các môi trường staging/production để đảm bảo tính truy vết (traceability) và an toàn dữ liệu.

Nếu một migration gây lỗi, quy trình xử lý như sau:
1. **Rollback Code:** Nếu lỗi gây sập app, thực hiện rollback code về version trước đó (xem mục trên).
2. **Tạo Migration mới:** Tạo một file migration mới (ví dụ: `000x_fix_error_in_000y.sql`) để thực hiện các lệnh SQL sửa lỗi hoặc revert thay đổi.
3. **Deploy:** Merge migration mới vào nhánh `dev` để hệ thống tự động apply lên server.

> 💡 **Lợi ích:** Mọi thay đổi và sai sót đều được lưu vết trong lịch sử Git và bảng `goose_db_version`.

---

## Kiểm tra health staging

```bash
curl http://$STAGING_HOST:8080/healthz
# Expected: 200 OK

# Swagger UI
open http://$STAGING_HOST:8080/swagger/index.html
```
