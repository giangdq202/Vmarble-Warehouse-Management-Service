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

- [Best-effort advisory paths] Some side effects are advisory only — e.g. writing a `REMNANT_BYPASSED` audit row when a planner ignores remnant suggestions. The primary operation (CreateWorkOrder) must succeed even if the advisor errors. Run the advisory call AFTER the primary write commits, swallow advisor errors with `slog.Warn(... "wo_id", wo.ID, "err", err)`, and never roll back. The audit miss is recoverable; failing to create the work order over a logging hiccup is not. Tests must cover both success and advisor-error branches to prove the API stays 2xx.

- [Audit log as multi-entity] An `inventory_audit_log` that originally logged remnant rows (`entity_type = 'REMNANT'`) eventually needs to log work-order events (`entity_type = 'WORK_ORDER'`) and structured payloads (`metadata jsonb`). Keep `entity_type` as a free-form string with a service-layer allow-list, and add `metadata jsonb` early — a single `reason text` column quickly becomes overloaded once you need to store an array (e.g. `suggested_remnant_ids`). Index `(action)` separately from `(entity_id)` because review queries fetch by action across many entities.

- [Re-scope before code] An issue title like "API X trống" can hide a totally different bug — before writing code, read the FE route and the BE handler that backs it. `/report-cut` was filed as "missing report data" but turned out to be a kiosk form mis-labelled "Báo cáo" in bottom-nav. Had I jumped straight to `ComputeCost`-style code, the "fix" would not have solved the user's actual pain. Spend 5 minutes verifying the cause before picking up the DoD verbatim.

- [Typed-nil params for optional SQL filters] For pgx queries with optional filters (`$1::uuid IS NULL OR col = $1`), declare the placeholder as `var userID any = nil` and reassign the concrete `uuid.UUID` / `time.Time` when set. Never pass a `uuid.UUID{}` zero-value — pgx will bind it as an all-zeros UUID and your `IS NULL` branch never fires. This pattern scales cleanly for 3–4 optional filters without resorting to dynamic SQL building.

- [Guard placement with BR precedence] When adding a new compute-time guard (zero-cost, invalid state) to a service method that already has BR guards (finalized immutability, transition checks), **place the new guard AFTER the higher-precedence BR checks**. Otherwise a "nicer" VN error message leaks and masks the real violation. In `ComputeCost`: Finalized check must fire before the zero-cost guard so BR-C04 wins when both conditions hold. Add an explicit test `..._ZeroCost_ReturnsAlreadyFinalized` to lock this ordering.

- [Test migration strategy when tightening a guard] When a new guard fails on pre-existing tests that were coincidentally zero-cost COMPLETED fixtures, do NOT dilute the guard with exceptions. Instead, identify which tests are testing the guarded condition vs. testing unrelated subjects (insert path, update path, error propagation) — migrate the unrelated ones to use `plannedWO` (exempt from the guard) so they keep focusing on their real subject. Only the handful of ACTUAL-specific tests need to be updated with real cost data.

- [Proxy attribution when schema lags] For reports that need "who did X" but the source table lacks a `created_by` column (like `cutting_records`), use the existing FK to another table that holds assignment data (`work_orders.assigned_to`) as a proxy. Document the proxy clearly in the DTO doc comment + issue body so future readers know the semantic limitation: reassignment will rewrite history. File a follow-up issue for the proper migration if precision matters.

- [Polling vs SSE discovery] Backend already had Postgres LISTEN/NOTIFY + SSE endpoint `/notifications/stream` with broker fan-out, but FE never connected — so user-facing "realtime" was pure 30-60s polling. When the user complains "update chậm," grep FE for `EventSource` / `notifications/stream` / `refetchInterval` before assuming the pipe is missing. The fix split naturally into BE (expand event types + topic/roles broadcast) and FE (wire consumer + invalidate queries). Native EventSource cannot send Bearer headers — use `@microsoft/fetch-event-source` or token via URL query.
