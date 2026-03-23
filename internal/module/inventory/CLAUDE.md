# Inventory Module — Context for Claude

## Ownership

This module owns: `InventoryLot`, `BoardSheet`, `Remnant`, `CuttingRecord`.

## Business rules (BR-K01 to BR-K05)

- **BR-K01**: Stock receipt creates lots and individual board sheets
- **BR-K02**: Sheets must be issued to a work order before cutting
- **BR-K03**: Area conservation — `used_area + remnant_area <= source_area` (validated in `service.go` `RecordCut`)
- **BR-K04**: Remnant lineage — every remnant tracks `parent_board_id` + optional `parent_remnant_id` for nested cutting
- **BR-K05**: Remnant status lifecycle: `AVAILABLE → ALLOCATED → CONSUMED` or `AVAILABLE → WASTE`

## Key operations

| Method | What it does | Critical invariant |
|---|---|---|
| `ReceiveStock` | Creates lot + N board sheets | Validates material exists |
| `RecordCut` | Cuts a sheet or remnant, may create new remnant | **BR-K03 area conservation** |
| `AllocateRemnant` | Marks remnant for a work order | Status must be AVAILABLE |
| `MarkRemnantWaste` | Marks remnant as waste | Status must be AVAILABLE |
| `GetRemnantLineage` | Returns full remnant tree for a board | Recursive parent chain |

## Cutting source logic

`RecordCut` accepts either `sheet_id` OR `remnant_id` (not both):
- From sheet: validates sheet is available, calculates area
- From remnant: validates remnant is AVAILABLE, calculates area from remnant dimensions
- Creates cutting record + optional new remnant

## Dependencies

None (does not depend on other modules).

## Testing focus

- Area conservation edge cases (exact area match, exceeding by 1mm2)
- Nested remnant cutting (remnant from remnant)
- Status transition validity
- Concurrent cutting of same sheet (should fail)
