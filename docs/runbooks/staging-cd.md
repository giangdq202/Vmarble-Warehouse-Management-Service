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

Cấu hình `docker-compose.staging.yml` sử dụng **Double Binding**: cho phép truy cập từ chính server (`127.0.0.1`) và qua mạng riêng ảo (VPN — địa chỉ cấu hình qua biến `STAGING_VPN_IP` trong `.env.staging`).

```bash
# Chạy từ máy local
scp deploy/staging/docker-compose.staging.yml root@$STAGING_HOST:/opt/vwms-staging/
scp deploy/staging/deploy.sh                  root@$STAGING_HOST:/opt/vwms-staging/
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
# IP VPN của server (WireGuard/OpenVPN) — để trống nếu không cần VPN access
STAGING_VPN_IP=<YOUR_SERVER_VPN_IP>
EOF
chmod 600 /opt/vwms-staging/.env.staging
```

---

## GitHub Secrets & Variables

Vào **GitHub repo → Settings → Secrets and variables → Actions** và thêm:

| Tên | Loại | Giá trị |
|---|---|---|
| `DISCORD_WEBHOOK_URL` | Secret | Webhook URL của Discord channel |
| `STAGING_HOST` | Secret | IP của staging server |
| `STAGING_SSH_KEY` | Secret | Nội dung private key |
| `STAGING_URL` | **Variable** | `http://<staging-host>:8080` hoặc URL qua VPN nếu có |

---

## Luồng CD tự động (Standard Deployment)

Dự án sử dụng **Immutable Tags** (Git SHA) để đảm bảo tính duy nhất của mỗi bản build.

```
Merge PR vào dev
      │
      ▼
Job: Build & Push
  ① docker build (multi-stage, alpine)
  ② docker push 2 tags:
     - ghcr.io/...:sha-<short> (Dùng để deploy/rollback - KHÔNG ĐỔI)
     - ghcr.io/...:staging-latest (Dùng để tracking bản mới nhất)
  ③ Discord: 🔨 Build OK
      │
      ▼
Job: Deploy Staging
  ① SSH vào staging server ($STAGING_HOST)
  ② /opt/vwms-staging/deploy.sh <image-path>:sha-<short>
     - Pull image SHA mới (Không ghi đè lên image cũ)
     - Restart app container
     - Health check /healthz mỗi 5s, tối đa 60s
     - OK  → Lưu SHA vào .last_good_image, exit 0
     - FAIL→ Lấy SHA cũ từ .last_good_image để rollback, exit 1
  ③ Discord: 🚀 Deploy OK / 💥 Deploy FAIL + đã rollback
```

---

## Xem logs deploy trên server

```bash
ssh root@$STAGING_HOST

# Logs của app container
docker logs vwms-staging-app-1 --tail 100 -f

# Trạng thái deploy gần nhất (Sẽ thấy tag SHA ở đây)
cat /opt/vwms-staging/.last_good_image
```

---

## Rollback thủ công

### Rollback app (code)

Vì mỗi image có tag SHA riêng, việc rollback rất đơn giản và an toàn:

```bash
ssh root@$STAGING_HOST
cd /opt/vwms-staging

# Xem image SHA đang chạy tốt nhất
cat .last_good_image

# Nếu cần rollback về một SHA cụ thể khác
APP_IMAGE="ghcr.io/giangdq202/vmarble-warehouse-management-service:sha-a1b2c3d" \
  docker compose -f docker-compose.staging.yml --env-file .env.staging up -d app
```

### Xử lý Database & Tương thích ngược

Dự án áp dụng triết lý **"Fix-forward"** và **"Append-only Schema"**:

1.  **Không chạy `migrate down` tự động:** Để đảm bảo an toàn dữ liệu và tính truy vết.
2.  **Append-only (Chỉ thêm, không xóa/sửa):**
    *   Trong các migration, ưu tiên thêm cột mới (nullable hoặc có default) thay vì đổi tên hoặc xóa cột cũ.
    *   Việc này giúp App cũ (sau khi rollback) vẫn tìm thấy cấu hình DB cần thiết và chạy bình thường (Backward Compatibility).
3.  **Quy trình dọn dẹp:** Chỉ thực hiện xóa cột cũ hoặc migration "phá hủy" sau khi phiên bản App mới đã chạy ổn định trên Production/Staging một thời gian dài.

---

## Kiểm tra health staging

```bash
# Thử từ nội bộ server hoặc qua VPN
curl http://$STAGING_HOST:8080/healthz
# Nếu có cấu hình VPN (STAGING_VPN_IP trong .env.staging):
curl http://$STAGING_VPN_IP:8080/healthz
```
