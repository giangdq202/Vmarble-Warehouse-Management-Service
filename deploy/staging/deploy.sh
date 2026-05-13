#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Load environment variables
if [[ ! -f ".env" ]]; then
  echo "Error: .env file not found in $SCRIPT_DIR"
  exit 1
fi

set -a
source .env
set +a

# Validate required variables
if [[ -z "${APP_IMAGE:-}" || -z "${SERVICE_NAME:-}" || -z "${HEALTH_URL:-}" ]]; then
  echo "Error: Missing required variables in .env (APP_IMAGE, SERVICE_NAME, HEALTH_URL)"
  exit 1
fi

COMPOSE_FILE="compose.yaml"

# Pull new image
echo "Pulling image: $APP_IMAGE"
docker compose pull "$SERVICE_NAME"

# Deploy
echo "Deploying $SERVICE_NAME with image: $APP_IMAGE"
docker compose -f "$COMPOSE_FILE" up -d "$SERVICE_NAME"

# Health check (20 seconds timeout - Go app needs more startup time than Node.js)
echo "Waiting for health check: $HEALTH_URL"
for i in $(seq 1 20); do
  if curl -fsS "$HEALTH_URL" >/dev/null 2>&1; then
    echo "✅ Health check passed (${i}s)"
    echo "✅ Deploy successful: $APP_IMAGE"
    exit 0
  fi
  sleep 1
done

# Health check failed
echo "❌ Health check failed after 20s"
echo "⚠️  Container may be unhealthy. Check logs: docker logs $SERVICE_NAME"
exit 1
