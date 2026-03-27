.PHONY: dev run build test test-integration lint swagger migrate-up migrate-down migrate-create docker-up docker-down

APP_NAME := warehouse-server
PG_PORT  ?= 5433
DSN      ?= postgres://vmarble:vmarble@localhost:$(PG_PORT)/vmarble?sslmode=disable
GOOSE    ?= go run github.com/pressly/goose/v3/cmd/goose@v3.24.3
SWAG     ?= go run github.com/swaggo/swag/cmd/swag@v1.8.12

# ── Development ──────────────────────────────────────────────

dev: docker-up migrate-up run

run:
	DATABASE_URL="$${DATABASE_URL:-"$(DSN)"}" go run ./cmd/server

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
	$(SWAG) init --parseInternal --parseGoList=false -g main.go -d ./cmd/server,./internal/domain,./internal/module/barcode,./internal/module/catalog,./internal/module/costing,./internal/module/inventory,./internal/module/order,./internal/module/planning,./internal/module/production,./internal/platform/auth,./internal/platform/config,./internal/platform/httpkit,./internal/platform/postgres -o docs

# ── Database Migrations ─────────────────────────────────────

migrate-up:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DSN)" $(GOOSE) -dir migrations up

migrate-down:
	GOOSE_DRIVER=postgres GOOSE_DBSTRING="$(DSN)" $(GOOSE) -dir migrations down

migrate-create:
	@read -p "Migration name: " name; \
	GOOSE_DRIVER=postgres $(GOOSE) -dir migrations create $$name sql

# ── Docker ───────────────────────────────────────────────────

docker-up:
	docker compose up -d

docker-down:
	docker compose down -v
