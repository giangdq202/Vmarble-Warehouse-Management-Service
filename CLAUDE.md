# VMARBLE Warehouse Management Service — Agent Guide

This repo is currently **docs-first** (no runtime code yet). Use this file as the source of truth for how to work in the repo and how to translate the business spec into implementation later.

## Source documents (read these first)

- `README.md` (project goals + branch rules)
- `docs/backend-business-logic-vi.md` (Vietnamese business spec; this is the authority for flow, entities, and business rules)

When there is any conflict, prefer `docs/backend-business-logic-vi.md`.

## Branch rules (must follow)

- **Never push directly to `main` or `dev`.**
- Create feature branches from `dev`.
- Open PRs:
  - Feature → `dev` (speed over ceremony; approval optional per README)
  - `dev` → `main` only when release-ready (requires at least 1 approval per README)
- No force-push to protected branches.

## What you should produce in this repo (current phase)

Until implementation exists, prioritize:

- **Clarifying questions log**: maintain a running list of open business decisions from section “Các điểm cần xác nhận…” in the spec.
- **Data model drafts**: propose entities/relations and invariants consistent with the spec.
- **API contracts**: draft endpoints, request/response schemas, and error semantics for the rules below.
- **Acceptance criteria**: write testable scenarios for each business rule (BR-*).

If you add code later, also add:

- A clear setup/run/test section in `README.md`
- `docker-compose.yml` and/or a Makefile if multi-service
- A migration strategy for the schema (idempotent, reproducible)

## Domain invariants (do not violate)

### End-to-end flow (6 stages)

The system models an end-to-end flow: PO → Production Plan → CNC cutting/remnant → Processing → Costing → Finance reporting.

Do not implement “costing” as free-form manual entry. Costing reads from operational records.

### WorkOrder state machine (authoritative)

Statuses: `PLANNED -> IN_CUTTING -> IN_PROCESSING -> COMPLETED -> COSTED`

- Transitions must be validated and monotonic.
- Costing is only permitted when status is `COMPLETED`.

### Remnant lineage is core

Remnant tracking must support:

- A parent pointer (`parent_board_id` and/or `parent_remnant_id`)
- Nested cutting (remnant can generate child remnants)
- Status lifecycle: `AVAILABLE`, `ALLOCATED`, `CONSUMED`, `WASTE`

### Area conservation & validation

For each cutting event:

- `used_area + remnant_area + waste_area <= sheet_area`
- Do not allow completing a cut record without specifying the used dimensions and either remnant dimensions or waste flag.

### Costing allocation (default rule from spec)

If one sheet contributes to multiple SKUs via remnant reuse, allocate sheet cost by used area:

`cost_for_sku = (area_used_by_sku / total_area_of_sheet) * sheet_cost`

Waste is not allocated to SKUs; it is tracked separately.

If the customer confirms a different allocation rule later, treat it as a versioned policy (do not silently change historical costing).

### Metal is conditional

For SKUs with `requires_metal = true`:

- The WorkOrder must include a metal consumption record before it can become `COMPLETED`.

## Error handling expectations (when implementing)

Follow the spec’s intent:

- Inventory checks that fail should return **422** with a specific message (BR-K01).
- Validation errors (area conservation, missing required inputs) should be deterministic and explain which fields violate constraints.

## Naming & language conventions

- Keep domain names aligned with the spec: PO, SKU, ProductionPlan, WorkOrder, CuttingRecord, Remnant, ConsumptionRecord, CostingRecord, Barcode.
- Prefer English identifiers in code and schemas; Vietnamese is welcome in docs/user-facing text where helpful.
- Preserve spec terminology in comments/docs when translating (e.g., “remnant / vật tư dư”, “hao hụt / waste”).

## What to ask / confirm before implementing algorithms

From the spec’s “CÁC ĐIỂM CẦN XÁC NHẬN…” section, treat these as blockers for “smart” automation:

- Remnant selection strategy: FIFO vs Best Fit vs manual override
- Allocation rule for remnant costing (area vs weight vs custom)
- Labor costing method (time-based vs per-unit vs salaried allocation)
- Barcode printing trigger points
- Vendor & purchasing module scope
- Workforce/shift management scope
- Stone workshop integration expectations and shared entities

## PR expectations (when code exists)

- PR description must include: Summary, Business rule(s) impacted (BR-*), Test plan.
- Do not merge changes that weaken domain invariants above.

