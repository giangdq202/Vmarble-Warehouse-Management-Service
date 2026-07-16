---
name: deploy
description: >
  Use when the user asks to deploy, dockerize, set up CI/CD, configure environments,
  or prepare for go-live. Covers Docker Compose, GitHub Actions, health checks,
  environment configuration, database backup, and production readiness checklist.
  Trigger words: "deploy", "docker", "CI", "CD", "go live", "production", "staging",
  "health check", "backup", "monitoring".
---

# Deploy — Production Readiness for VMARBLE WMS

## Stack reminder

- Go 1.24, PostgreSQL 17
- Router: gin-gonic/gin
- DB: pgxpool + goose migrations
- Config: caarlos0/env/v11 (environment variables)

---

## Phase 1 — Docker

### 1.1 Application Dockerfile

Build a multi-stage Dockerfile for the Go backend:

```dockerfile
# Stage 1: Build
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /warehouse-server ./cmd/server

# Stage 2: Runtime
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /warehouse-server /usr/local/bin/warehouse-server
COPY migrations/ /app/migrations/
EXPOSE 8080
ENTRYPOINT ["warehouse-server"]
```

Checklist:
- [ ] Multi-stage build (builder + runtime)
- [ ] `CGO_ENABLED=0` for static binary
- [ ] Copy `migrations/` so goose can run at startup
- [ ] No secrets baked into image

### 1.2 Docker Compose (dev + staging)

```yaml
services:
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_DB: ${DB_NAME}
      POSTGRES_USER: ${DB_USER}
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${DB_USER}"]
      interval: 5s
      retries: 5

  api:
    build: .
    depends_on:
      db:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://${DB_USER}:${DB_PASSWORD}@db:5432/${DB_NAME}?sslmode=disable
      AUTH_SECRET: ${AUTH_SECRET}
      PORT: 8080
    ports:
      - "8080:8080"

volumes:
  pgdata:
```

Checklist:
- [ ] DB healthcheck before API starts
- [ ] Environment variables via `.env` file (NOT committed)
- [ ] Volume for persistent DB data
- [ ] No hardcoded secrets

---

## Phase 2 — Health Check Endpoint

Add a `GET /healthz` endpoint (public, no auth) that returns:

```json
{"status": "ok", "db": "connected", "version": "<git-sha>"}
```

Implementation:
- Ping DB pool with `pool.Ping(ctx)`
- Return 200 if ok, 503 if DB unreachable
- Inject git SHA at build time via `-ldflags`
- Register BEFORE auth middleware group

---

## Phase 3 — CI/CD (GitHub Actions)

### 3.1 CI Pipeline (on every PR to `dev`)

```yaml
name: CI
on:
  pull_request:
    branches: [dev]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17-alpine
        env:
          POSTGRES_DB: wms_test
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
        ports: ["5432:5432"]
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.24" }
      - run: go test ./... -race -count=1
      - run: go vet ./...
      - run: go build ./...
```

### 3.2 CD Pipeline (on push to `main`)

```yaml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build Docker image
        run: docker build -t vmarble-wms:${{ github.sha }} .
      - name: Push to registry
        # Adapt to your registry (GHCR, Docker Hub, ECR)
        run: |
          echo "${{ secrets.REGISTRY_TOKEN }}" | docker login ghcr.io -u ${{ github.actor }} --password-stdin
          docker tag vmarble-wms:${{ github.sha }} ghcr.io/${{ github.repository }}:${{ github.sha }}
          docker push ghcr.io/${{ github.repository }}:${{ github.sha }}
      - name: Deploy to server
        # SSH deploy, docker compose pull, or k8s apply
        run: echo "TODO: adapt to your hosting"
```

---

## Phase 4 — Environment Configuration

### Required environment variables

| Variable | Description | Example |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://user:pass@host:5432/dbname?sslmode=disable` |
| `AUTH_SECRET` | JWT signing secret (min 32 chars) | random string |
| `PORT` | HTTP listen port | `8080` |
| `REMNANT_ALLOC_CHECK_INTERVAL` | Background task interval | `5m` |

### Environment checklist

- [ ] `.env.example` committed with placeholder values (no real secrets)
- [ ] `.env` in `.gitignore`
- [ ] Production uses real secrets via hosting env vars (not file)
- [ ] `AUTH_SECRET` is different between staging and production

---

## Phase 5 — Database Operations

### Backup strategy

```bash
# Manual backup
pg_dump -h localhost -U $DB_USER -d $DB_NAME -F c -f backup_$(date +%Y%m%d).dump

# Restore
pg_restore -h localhost -U $DB_USER -d $DB_NAME -c backup_YYYYMMDD.dump
```

### Migration safety

Before deploying a new version:
1. Run `goose status` to verify pending migrations
2. Run `goose up` — migrations must be idempotent
3. Verify `goose down` works for the new migration (rollback test)

### Zero-downtime migration rules

- **Additive only**: add columns with DEFAULT or NULL, add tables, add indexes CONCURRENTLY
- **Never in one step**: rename column, change type, drop column — use 2-phase migration
- **Always test**: `goose up && goose down && goose up` must succeed

---

## Phase 6 — Go-Live Checklist

Run through ALL items before first production deploy:

### Security
- [ ] `AUTH_SECRET` is strong (32+ chars, random)
- [ ] CORS configured (not `*` in production)
- [ ] HTTPS/TLS terminated (reverse proxy or load balancer)
- [ ] No debug endpoints exposed (`/swagger` disabled in prod)
- [ ] Rate limiting on auth endpoints

### Data
- [ ] Seed data: default users with correct roles
- [ ] Storage locations seeded
- [ ] Materials catalog seeded (PLYWOOD, GLUE, METAL, OTHER)
- [ ] Automated DB backup schedule

### Monitoring
- [ ] `GET /healthz` returns 200
- [ ] Uptime monitoring configured (UptimeRobot, Betterstack, etc.)
- [ ] Log aggregation (stdout → hosting platform logs)
- [ ] Error alerting (5xx spike → notification)

### Application
- [ ] `make test` green
- [ ] `make lint` clean
- [ ] `make build` succeeds
- [ ] Role-based access verified (run `rbac-hardener` skill first)
- [ ] All pending migrations applied
- [ ] Frontend `.env` points to correct API URL

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| API starts but returns 500 | DB not reachable | Check `DATABASE_URL`, run `pg_isready` |
| Migration fails | Out of order | `goose status`, check sequence numbers |
| Auth returns 401 on valid token | Wrong `AUTH_SECRET` | Ensure same secret between token issuer and verifier |
| Docker build fails | Go module cache | `docker builder prune`, rebuild |
