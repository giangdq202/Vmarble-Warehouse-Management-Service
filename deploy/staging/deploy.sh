#!/usr/bin/env bash
# Luồng:
#   1. Lưu image hiện tại để rollback nếu cần
#   2. Pull image mới
#   3. Restart app container (postgres KHÔNG bị ảnh hưởng)
#   4. Health check tối đa 60s
#   5a. OK    → Cập nhật state, exit 0
#   5b. FAIL  → Rollback về image cũ, exit 1

set -euo pipefail

DEPLOY_DIR="/home/deploy_vwms/projects/vwms-staging"
COMPOSE_FILE="${DEPLOY_DIR}/docker-compose.staging.yml"
ENV_FILE="${DEPLOY_DIR}/.env.staging"
STATE_FILE="${DEPLOY_DIR}/.last_good_image"
HEALTH_URL="http://localhost:8080/healthz"
HEALTH_RETRIES=12
HEALTH_INTERVAL=5

NEW_IMAGE="${1:?[deploy] Thiếu argument: full image path}"

cd "$DEPLOY_DIR"

PREV_IMAGE=$(cat "$STATE_FILE" 2>/dev/null || true)
docker pull "$NEW_IMAGE"

APP_IMAGE="$NEW_IMAGE" docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d app

IS_HEALTHY=false
for i in $(seq 1 $HEALTH_RETRIES); do
    sleep "$HEALTH_INTERVAL"
    if curl -sf --max-time 3 "$HEALTH_URL" > /dev/null 2>&1; then
        IS_HEALTHY=true
        break
    fi
done

if $IS_HEALTHY; then
    echo "$NEW_IMAGE" > "$STATE_FILE"
    exit 0
fi

# ... logic rollback ...
APP_IMAGE="$PREV_IMAGE" docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" up -d app
# ... xác nhận rollback ...
exit 1
