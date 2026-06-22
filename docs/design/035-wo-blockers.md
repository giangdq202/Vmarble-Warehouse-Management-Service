# Design: WO Blockers (#35)

## Problem
WO can advance status even when external blockers exist (material delayed, machine down).
Planner needs visibility + enforcement.

## Data Model

```sql
CREATE TABLE wo_blockers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    work_order_id UUID NOT NULL REFERENCES work_orders(id),
    reason      TEXT NOT NULL CHECK (reason IN ('MATERIAL_DELAYED','MATERIAL_REJECTED','MACHINE_DOWN','OTHER')),
    detail      TEXT NOT NULL DEFAULT '',
    created_by  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_by UUID,
    resolved_at TIMESTAMPTZ
);
CREATE INDEX idx_wo_blockers_wo_open ON wo_blockers (work_order_id) WHERE resolved_at IS NULL;
```

## Service Methods (added to production.Service)

- `CreateBlocker(ctx, CreateBlockerInput) (WOBlocker, error)`
- `ResolveBlocker(ctx, ResolveBlockerInput) (WOBlocker, error)`
- `ListBlockers(ctx, woID uuid.UUID) ([]WOBlocker, error)`

## AdvanceStatus Hook

Insert check after `CanTransitionTo` but before any other gate:
```go
openBlockers, err := s.store.countOpenBlockers(ctx, wo.ID)
if openBlockers > 0 {
    return domain.NewBizError(domain.ErrPreconditionFailed, 
        fmt.Sprintf("work order has %d open blocker(s)", openBlockers))
}
```

## Business Rules

- Cannot create blocker on WO with status COMPLETED or COSTED
- Cannot resolve an already-resolved blocker
- AdvanceStatus blocked if ANY open blocker exists (resolved_at IS NULL)
- Planner+ can create/resolve blockers
- Admin can also resolve

## Endpoints

- `POST   /work-orders/:id/blockers` — RequirePlannerUp()
- `GET    /work-orders/:id/blockers` — RequireWorkerUp()
- `PATCH  /work-orders/:id/blockers/:blocker_id/resolve` — RequirePlannerUp()

## Edge Cases

- Concurrent resolve + advance: countOpenBlockers runs inside AdvanceStatus flow,
  no separate TX needed — if resolve commits first, count returns 0, advance proceeds.
  If advance reads first with open blocker, it fails, planner retries.
- Multiple open blockers: ALL must be resolved before advance.
