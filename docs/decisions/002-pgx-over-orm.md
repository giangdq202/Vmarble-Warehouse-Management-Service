# ADR-002: pgx/v5 Over ORM

## Status

Accepted

## Context

We need a database access strategy for PostgreSQL 17. Options considered:
1. **GORM** — popular Go ORM, auto-migrations, magic
2. **sqlx** — thin layer over database/sql, struct scanning
3. **pgx/v5** — native PostgreSQL driver with connection pooling, no abstraction

## Decision

We chose **pgx/v5 with pgxpool** for direct PostgreSQL access:
- Raw SQL queries in `pgstore.go` files
- Manual row scanning with `QueryRow`/`Scan`
- Connection pooling via `pgxpool`
- Migrations handled separately by goose

## Consequences

### Positive

- Full control over SQL — no ORM magic or N+1 surprises
- Best PostgreSQL driver performance in Go ecosystem
- Easy to use PostgreSQL-specific features (RETURNING, CTEs, JSON ops)
- Clear separation: SQL in pgstore, business logic in service

### Negative

- More boilerplate for CRUD operations
- Manual row mapping can be error-prone
- No auto-migration — must write goose migrations manually

### Neutral

- Team must be comfortable writing SQL directly
- Testing requires real Postgres or careful mocking of store interface
