#!/usr/bin/env bash
# rollback-db.sh — Script rollback DB thủ công khi cần
# Chỉ dùng trong tình huống khẩn cấp: migration mới gây lỗi và app đã được rollback code.
#
# Prerequisite: goose binary phải được cài trên server
#   curl -sSL https://github.com/pressly/goose/releases/download/v3.24.3/goose_linux_x86_64 \
#     -o /usr/local/bin/goose && chmod +x /usr/local/bin/goose
#
# Usage:
#   ./rollback-db.sh           # rollback 1 migration
#   ./rollback-db.sh 2         # rollback 2 migrations
#   ./rollback-db.sh status    # xem trạng thái migrations

set -euo pipefail

DEPLOY_DIR="/opt/vwms-staging"
ENV_FILE="${DEPLOY_DIR}/.env.staging"
MIGRATIONS_DIR="${DEPLOY_DIR}/migrations"

# Load DATABASE_URL từ .env.staging
set -a && source "$ENV_FILE" && set +a

COMMAND="${1:-down}"
N="${2:-1}"

case "$COMMAND" in
    status)
        echo "=== Migration status ==="
        goose -dir "$MIGRATIONS_DIR" postgres "$DATABASE_URL" status
        ;;
    down)
        echo "=== Rollback $N migration(s) ==="
        for i in $(seq 1 "$N"); do
            echo "--- Rollback #$i ---"
            goose -dir "$MIGRATIONS_DIR" postgres "$DATABASE_URL" down
        done
        echo "Done. Current status:"
        goose -dir "$MIGRATIONS_DIR" postgres "$DATABASE_URL" status
        ;;
    *)
        echo "Usage: $0 [status|down] [count]"
        exit 1
        ;;
esac
