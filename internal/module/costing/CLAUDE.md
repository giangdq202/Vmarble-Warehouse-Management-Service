# Costing Module — Context for Claude

## Ownership

This module owns: `CostingRecord`.

## Business rules (BR-C01 to BR-C04)

- **BR-C01**: Costing can only be computed when work order status = `COMPLETED`
- **BR-C02**: Material cost allocation is area-based:
  ```
  cost_for_sku = (area_used / total_sheet_area) * sheet_cost
  ```
- **BR-C03**: Waste area is NOT allocated to SKUs (absorbed as overhead)
- **BR-C04**: Finalized costing records are **immutable** — cannot be modified or recomputed

## Key operations

| Method | What it does | Critical invariant |
|---|---|---|
| `ComputeCost` | Calculates costing for a work order | **BR-C01** WO must be COMPLETED |
| `FinalizeCost` | Locks the costing record | **BR-C04** immutability after finalize |
| `GetCostingRecord` | Returns costing for a WO | Read-only |
| `ListCostingRecords` | Returns all costing records | Read-only |

## Cross-module dependencies (deps.go)

- `ProductionService` — to get work order status and consumption records
- `InventoryService` — to get sheet dimensions and cost data for area-based allocation

These are defined as interfaces in `deps.go` and wired in `main.go`.

## Costing calculation flow

1. Verify WO status = COMPLETED (else reject)
2. Get all cutting records for the work order's sheets
3. For each SKU cut from a sheet:
   - `sku_cost = (sku_area / sheet_total_area) * sheet_cost`
4. Sum auxiliary costs (metal, edge banding, etc.)
5. `total_cost = material_cost + auxiliary_cost`
6. Waste area contributes 0 to SKU costs

## Testing focus

- Costing only allowed for COMPLETED work orders
- Area-based allocation accuracy (floating point edge cases)
- Waste exclusion from allocation
- Finalization immutability (recompute after finalize should fail)
- Multiple SKUs cut from same sheet
