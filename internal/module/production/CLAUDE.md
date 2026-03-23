# Production Module — Context for Claude

## Ownership

This module owns: `WorkOrder`, `ConsumptionRecord`.

## Business rules (BR-P01 to BR-P04)

- **BR-P01**: Work order creation requires valid plan and SKU
- **BR-P02**: State machine is strict and monotonic:
  ```
  PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED
  ```
  No skipping, no going backward.
- **BR-P03**: Consumption records track material usage per work order
- **BR-P04**: Metal requirement — SKUs with `requires_metal = true` **must** have at least one METAL-type consumption record before transitioning to `COMPLETED`

## Key operations

| Method | What it does | Critical invariant |
|---|---|---|
| `CreateWorkOrder` | Creates WO from plan + SKU | Validates plan exists, SKU exists |
| `AdvanceStatus` | Transitions WO to next state | **BR-P02 state machine** + **BR-P04 metal check** |
| `RecordConsumption` | Records material usage | WO must be in IN_CUTTING or IN_PROCESSING |
| `ListConsumptions` | Returns all consumption for a WO | Read-only |

## Cross-module dependencies (deps.go)

- `CatalogService` — to check if SKU exists and if it requires metal
- `InventoryService` — to check stock availability

These are defined as interfaces in `deps.go` and wired in `main.go`.

## State machine detail

```
AdvanceStatus(woID, IN_CUTTING):    requires current = PLANNED
AdvanceStatus(woID, IN_PROCESSING): requires current = IN_CUTTING
AdvanceStatus(woID, COMPLETED):     requires current = IN_PROCESSING + metal check (BR-P04)
AdvanceStatus(woID, COSTED):        requires current = COMPLETED (called by costing module)
```

## Testing focus

- Every valid state transition
- Every invalid state transition (backwards, skipping)
- Metal requirement enforcement (with and without METAL consumption)
- Consumption recording at invalid states
