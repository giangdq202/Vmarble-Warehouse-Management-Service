# Backend Instincts and Lessons Learned

This file serves as the Dynamic Memory for the AI Agent. It records patterns, pitfalls, and instincts discovered during development to prevent regression and repeat mistakes in the Go modular monolith.

## Core Instincts

- [Concurrency] Always use SELECT FOR UPDATE inside a transaction when performing read-then-write operations to prevent race conditions.
- [Architecture] Strictly respect module boundaries. Use deps.go for cross-module communication; never import internal/module/A into internal/module/B directly.
- [Database] Migrations must be additive and idempotent. Verify both Up and Down paths before committing.
- [Error Handling] Always return domain.NewBizError for business rule violations to ensure correct HTTP status mapping (422, 409).
- [Area Conservation] For any cutting operation, always verify that used_area + remnant_area <= source_area (BR-K03).

## Session Lessons

(New lessons will be appended here at the end of every task by the AI Agent)

- [SQL Guards] When a store UPDATE uses a WHERE guard (e.g. `AND assigned_to IS NULL`), ensure it encodes the **business invariant**, not an implementation assumption. `AND assigned_to IS NULL` silently blocks reassignment; `AND status = 'PLANNED'` correctly expresses mutability. When `RowsAffected() == 0`, always audit whether the WHERE clause is too tight before assuming a real conflict.

- [New Module Wiring] When wiring a new module in `main.go`, `cmd/server` is gitignored (pattern `server` matches). Use `git add -f cmd/server/main.go` to force-stage it. Consider adding `!cmd/server/` to .gitignore to prevent this confusion.

- [Cross-module ReceiveStock] When purchasing drives inventory creation, the adapter returns only `lot.ID` (not sheet IDs) because `inventory.ReceiveStock` does not expose individual sheet IDs. Design PO item → lot relationship, not PO item → individual sheet. Individual sheets are queryable later via `inventory_lots` join.

- [Cross-module Late-binding Cycle] When module A needs to check module B, but B also depends on A (A→B→A cycle), use a late-binding adapter with an empty `svc` field (like `woAdvanceAdapter`). Wire `adapter.svc = bSvc` after both services are constructed. Guard the adapter's method with `if a.svc == nil { return safeDefault }` to avoid nil panics during startup ordering edge cases.

- [CostingType Enum] When a record can represent two semantically different states (ESTIMATED vs ACTUAL), model this as a `type CostingType string` enum in `iface.go`, not as a boolean flag. This allows exhaustive switch in the future and makes the API self-documenting.

- [Dashboard Read-only Aggregation] For read-only dashboard endpoints that aggregate across tables, place the SQL directly in `dashboard/pgstore.go` — no migration needed, no transaction required. Use `COUNT(*) FILTER (WHERE status = 'X')` for multi-status aggregation in a single pass instead of multiple queries.

- [Cross-module Input DTOs] When a cross-module call needs to grow new fields (e.g. `BypassOverflow`, `ActorID`, `Reason` for an admin override), do NOT keep adding positional args to the interface. Migrate the dependency interface to take a struct (`PreAssignSheetInput` in inventory, `PreAssignSheetRequest` in production's `deps.go`) and let the adapter in `cmd/server/main.go` translate between the two shapes. This keeps each module's public DTO local and lets either side add fields without rewriting the other.

- [Defense in depth for admin overrides] When a route guard restricts a privileged flag (e.g. `bypass_overflow` requires `RoleAdmin`), validate the same invariant again in the consuming service layer. Even if the route is mis-wired, the inner module rejects the call. For audit-bearing actions, also assert that `ActorID` and `Reason` are populated at the inventory boundary — a missing actor on a bypass row is worse than a 400.
