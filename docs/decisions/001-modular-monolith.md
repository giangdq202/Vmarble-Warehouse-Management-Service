# ADR-001: Modular Monolith Architecture

## Status

Accepted

## Context

We need an architecture for the VMARBLE Warehouse Management Service that:
- Supports 7 distinct business domains (catalog, order, planning, inventory, production, costing, barcode)
- Can be developed by a small team
- Maintains clear domain boundaries
- Is simple to deploy and operate
- Can evolve toward microservices if needed in the future

Options considered:
1. **Monolith** — single package, fast to start but hard to maintain at scale
2. **Modular monolith** — isolated modules in one binary, clear boundaries
3. **Microservices** — separate deployments per domain, high operational complexity

## Decision

We chose a **modular monolith** with strict module isolation rules:

- Each domain module lives under `internal/module/<name>/`
- Modules never import each other directly
- Cross-module dependencies are defined as local interfaces in `deps.go`
- Adapters are wired only in `cmd/server/main.go`
- Every module follows a uniform 5-file structure: `iface.go`, `service.go`, `store.go`, `pgstore.go`, `handler.go`

## Consequences

### Positive

- Single binary deployment — simple operations
- Module boundaries enforced by Go import rules
- Easy to reason about each domain in isolation
- Can extract modules to microservices later by replacing adapters with RPC clients
- Uniform structure makes onboarding fast

### Negative

- All modules share one database — need discipline to avoid cross-module table access
- Single failure domain — one module crash takes down the whole service
- Must be vigilant about not breaking module isolation as codebase grows

### Neutral

- Performance is not a concern for MVP scale
- Monitoring is simpler with one process but needs per-module instrumentation later
