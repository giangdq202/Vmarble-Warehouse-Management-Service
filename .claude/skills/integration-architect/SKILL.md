---
name: integration-architect
description: Use to ensure alignment between Backend and Frontend, guarding the Modular Monolith and API contracts.
---

# Integration Architect - System Consistency

Ensures long-term system integrity, focusing on the BE-FE bridge.

## Modular Monolith Rules (Backend)
1. **No Cross-Module Import**: Module A must not import Module B. Use `deps.go` interfaces.
2. **Shared Domain**: Only types/enums in `internal/domain/` are shared.

## BE-FE Contract Rules
1. **JSON Alignment**: Go struct tags (`snake_case`) must match `src/types/api.ts` in the Client.
2. **Error Handling**: BE returns `BizError` (422, 409) -> FE must have matching `ApiClientError` handlers.
3. **Consistency**: Ensure list endpoints use `PagedResult<T>` to match FE table expectations.

## Integration Audit
- **3-Step Scan Flow**: Verify BE provides endpoints for all checkpoints (`CNC_COMPLETE`, `FINISHING_COMPLETE`, `WAREHOUSE_SHIP`).
- **Barcode Payload**: Ensure JSON encoded in QR codes matches the `ScannerView` parsing logic in FE.

## Ongoing Maintenance
- When updating `iface.go` in Service -> Alert the user to update `src/types/api.ts` in Client.
- When adding endpoints -> Run `make swagger` and verify if the FE needs a new `api-route`.

---
*Goal: Prevent "Runtime Type Errors" and "Inconsistent UI States" during deployment.*
