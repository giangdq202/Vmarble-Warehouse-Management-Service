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

- [POStatus typed string] Always define status enums as typed strings (`type POStatus string`) in the module's `iface.go`. This prevents accidental comparison with untyped string literals and makes exhaustive switch statements easier to verify during review.
