.PHONY: dev run build test lint migrate-up migrate-down migrate-create docker-up docker-down

APP_NAME := warehouse-server
PG_PORT  ?= 5433
DSN      ?= postgres://vmarble:vmarble@localhost:$(PG_PORT)/vmarble?sslmode=disable
GOOSE    ?= go run github.com/pressly/goose/v3/cmd/goose@v3.24.3

# ── Development ──────────────────────────────────────────────

dev: docker-up migrate-up run

run:
	go run ./cmd/server

build:
	go build -o bin/$(APP_NAME) ./cmd/server

# ── Testing & Linting ───────────────────────────────────────

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

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
