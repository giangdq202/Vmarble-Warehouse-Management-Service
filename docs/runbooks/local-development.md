# Runbook: Local Development Setup

## Prerequisites

- Go 1.24+
- Docker & Docker Compose
- make
- golangci-lint (for linting)

## First-time setup

```bash
# 1. Clone the repo
git clone <repo-url>
cd Vmarble-Warehouse-Management-Service

# 2. Copy environment file
cp .env.example .env

# 3. Start everything (Postgres + migrations + server)
make dev
```

Server runs at `http://localhost:8080`. Health check: `GET /healthz`.

## Daily workflow

```bash
# Start Postgres (if not running)
make docker-up

# Run migrations + server
make dev

# Or just the server (if migrations are current)
make run
```

## Common tasks

### Run tests
```bash
make test
```

### Run linter
```bash
make lint
```

### Create a new migration
```bash
make migrate-create
# Edit the generated file in migrations/
```

### Apply/rollback migrations
```bash
make migrate-up
make migrate-down
```

### Regenerate Swagger docs
```bash
make swagger
# Then visit http://localhost:8080/swagger/index.html
```

## Troubleshooting

### Port 5433 already in use
```bash
# Check what's using the port
lsof -i :5433
# Stop conflicting Postgres or change the port in .env
```

### Migration failures
```bash
# Check migration status
make migrate-status
# If stuck, manually fix the goose_db_version table
```

### Cannot connect to database
```bash
# Verify Docker is running
docker ps
# Restart Postgres
make docker-down && make docker-up
# Check .env has correct DATABASE_URL
```
