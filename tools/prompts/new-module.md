# New Module Prompt

Use this prompt when asking Claude to create a new module.

---

Create a new module `{MODULE_NAME}` under `internal/module/{MODULE_NAME}/` following the standard 5-file pattern:

1. **iface.go** — Define the `Service` interface with all public methods and input/output DTOs. Export only what consumers need.

2. **store.go** — Define the unexported `store` interface with repository methods the service needs.

3. **pgstore.go** — Implement the `store` interface with PostgreSQL queries using `pgx/pgxpool`. Use parameterized SQL, `RETURNING` for inserts, and map `pgx.ErrNoRows` to `domain.ErrNotFound`.

4. **service.go** — Implement the `Service` interface with all business logic and validation. Accept `context.Context` for IO operations. Use sentinel errors from `domain/errors.go`.

5. **handler.go** — Implement HTTP handlers using `gin`. Expose `NewHandler(svc)` and `Register(rg *gin.RouterGroup)`. Use `httpkit` for error responses.

6. **(If needed) deps.go** — Define exported interfaces for cross-module dependencies.

Wire in `cmd/server/main.go`: store → service → handler → register.

Create migration(s) in `migrations/` with the next sequence number.

Business rules to implement: {DESCRIBE_RULES}

Entities: {DESCRIBE_ENTITIES}
