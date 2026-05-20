.PHONY: dev run build test test-integration lint swagger migrate-up migrate-down migrate-create docker-build docker-up docker-down docker-rebuild seed install-hooks loadtest loadtest-build loadtest-soak

# Load .env if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

APP_NAME := warehouse-server
PG_PORT  ?= 5432
DSN      := $(DATABASE_URL)
GOOSE    ?= go run github.com/pressly/goose/v3/cmd/goose@v3.24.3
SWAG     ?= go run github.com/swaggo/swag/cmd/swag@v1.8.12

# ── Development ──────────────────────────────────────────────

dev: docker-up migrate-up run

run:
	go run ./cmd/server

build:
	go build -o bin/$(APP_NAME) ./cmd/server

# ── Testing & Linting ───────────────────────────────────────

test:
	go test ./... -race -count=1

# test-integration runs all tests tagged with `integration`.
# Requires Docker to be running — testcontainers spins up a dedicated
# PostgreSQL 17 container automatically and tears it down after the run.
# No manual DB setup is needed; the container is fully isolated from dev.
test-integration:
	go test -tags integration ./... -race -count=1 -timeout 180s -v

lint:
	golangci-lint run ./...

# ── API Docs (Swagger) ───────────────────────────────────────

swagger:
	$(SWAG) init --parseInternal --parseDependency --parseDepth 2 -g cmd/server/main.go -o docs

# ── Git hooks (one-time setup per clone) ────────────────────
# Points core.hooksPath at scripts/git-hooks so the version-controlled
# hooks (e.g. swagger regeneration) run on every commit. Repeat-safe.
install-hooks:
	@git config core.hooksPath scripts/git-hooks
	@echo "core.hooksPath -> scripts/git-hooks (active hooks: $$(ls scripts/git-hooks))"

# ── Database Migrations ─────────────────────────────────────

migrate-up:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DSN)" $(GOOSE) -dir migrations up

migrate-down:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DSN)" $(GOOSE) -dir migrations down

migrate-create:
	@read -p "Migration name: " name; \
	GOOSE_DRIVER=postgres $(GOOSE) -dir migrations create $$name sql

# ── Docker ───────────────────────────────────────────────────

docker-build:
	docker build -t $(APP_NAME):local .

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down -v

docker-rebuild:
	docker-compose build --no-cache
	docker-compose up -d --force-recreate

# ── Demo seed ────────────────────────────────────────────────

seed:
	@echo "Seeding demo data (server must be running at http://localhost:8080)..."
	@bash scripts/seed.sh

# ── Load testing ─────────────────────────────────────────────
# Drives the warehouse-server through cmd/loadtest. The server must already
# be running (use `make dev` in another terminal). Reports land in
# loadtest-results/<timestamp>-<scenario>/{report.html,summary.json}.
#
#   make loadtest                   # 60s closed-loop mixed scenario, 16 VUs
#   make loadtest SCENARIO=list-pos VUS=64 DURATION=2m
#   make loadtest MODE=open RPS=300 DURATION=5m
#
# JWT_SECRET is read from .env so the tool can mint a short-lived admin
# token automatically; pass TOKEN=… to override.

LOADTEST_BASE_URL ?= http://localhost:8080
LOADTEST_OUT_DIR  ?= loadtest-results
SCENARIO          ?= mixed
MODE              ?= closed
VUS               ?= 16
RPS               ?= 0
DURATION          ?= 60s
P95_MS            ?= 0
P99_MS            ?= 0
ERR_RATE_PCT      ?= 0
TOKEN             ?=

loadtest-build:
	go build -o bin/loadtest ./cmd/loadtest

loadtest: loadtest-build
	@mkdir -p $(LOADTEST_OUT_DIR)
	./bin/loadtest \
		-scenario=$(SCENARIO) -mode=$(MODE) -vus=$(VUS) -rps=$(RPS) \
		-duration=$(DURATION) -base-url=$(LOADTEST_BASE_URL) \
		-out-dir=$(LOADTEST_OUT_DIR) \
		-jwt-secret=$(AUTH_SECRET) -token=$(TOKEN) \
		-p95-ms=$(P95_MS) -p99-ms=$(P99_MS) -err-rate-pct=$(ERR_RATE_PCT)

# Long-running soak preset: catches connection-pool leaks and memory growth
# that a 60s smoke run cannot. Tune VUS/RPS to your box.
loadtest-soak: SCENARIO=mixed
loadtest-soak: DURATION=15m
loadtest-soak: VUS=32
loadtest-soak: loadtest
