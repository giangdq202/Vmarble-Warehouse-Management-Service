---
name: refactor
description: Use to refactor existing Go backend code while preserving modular-monolith boundaries, business invariants, handler-service-store separation, and public contracts. Trigger when the user asks to refactor, clean up, simplify, deduplicate, or restructure code.
---

# Refactor Skill

You are refactoring Go code in the VMARBLE Warehouse Management Service.

## Before refactoring

1. Read the target file(s) completely
2. Identify the module and its role (handler/service/store/pgstore)
3. Check `iface.go` for the public contract — do not break it
4. Check `deps.go` for cross-module interfaces — do not break them
5. Run `make test` to ensure current tests pass
6. Run `make lint` to see existing lint issues

## Refactoring rules

### Preserve invariants
- Never weaken domain invariants (state machine, area conservation, lineage, costing immutability)
- Never break the module boundary rule (no cross-module imports)
- Never move business logic out of `service.go`
- Never move SQL out of `pgstore.go`

### Code improvements to look for
- Extract repeated code into helper functions (unexported, same file or same package)
- Simplify complex conditionals with early returns
- Replace magic strings/numbers with constants
- Improve error messages with more context
- Reduce function parameters by grouping into input structs (if >4 params)
- Remove dead code / unused exports

### Naming improvements
- Fix stutter: `inventory.InventoryService` → `inventory.Service`
- Use explicit names: `pID` → `planID`
- Keep package names short, lowercase, no underscores

## After refactoring

1. Run `make test` — all tests must pass
2. Run `make lint` — no new lint issues
3. Run `gofmt` on changed files
4. Verify module boundary rule: `go vet ./...`
5. Summarize changes with rationale

## Output format

For each change:
- **What**: description of the refactoring
- **Why**: rationale (readability, maintainability, convention compliance)
- **Risk**: low/medium/high and mitigation
