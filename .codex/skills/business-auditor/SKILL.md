---
name: business-auditor
description: Use to verify if the code/logic aligns with the Vietnamese business spec. Mandatory cross-referencing with BR-* rules before implementation.
---

# Business Auditor - Logic Validation (Backend)

Ensures every line of Backend code complies with `docs/backend-business-logic-vi.md`.

## Core Principles
1. **Single Source of Truth**: The `docs/backend-business-logic-vi.md` file is the ultimate authority. Code must adapt to the spec, not vice versa.
2. **Rule Traceability**: Every logic change in `service.go` must be tagged or justified by a specific rule (e.g., `BR-K03`, `BR-P01`).

## Audit Workflow

### Step 1: Rule Extraction
Before modifying code, list the relevant business rules:
- "Which rules in Remnant Management, Costing, or Production are affected?"
- Look for keywords like "bắt buộc", "phải", "không được phép".

### Step 2: Invariant Verification
Check if the code violates these critical invariants:
- **BR-K03 (Area Conservation)**: `used_area + remnant_area <= source_area`.
- **BR-C04 (Costing Immutability)**: Finalized records MUST NOT be modified.
- **BR-P04 (Metal Requirement)**: SKUs requiring metal must have a METAL record before `COMPLETED` status.

### Step 3: State Machine Audit
- Validate `ValidateTransition` functions.
- Ensure `AVAILABLE` remnants don't accidentally become `CONSUMED` without a valid `RecordCut` event.

## Pre-PR Checklist
- [ ] Does this logic correctly handle area calculations without rounding errors?
- [ ] Is the "Minimum recovery area" rule applied to avoid tiny remnants?
- [ ] Are error messages mapped to `BizError` reflecting the specific rule violation?

---
*Note: If you find missing or contradictory logic in the spec, stop and ask the User (Indie Hacker) to update the documentation first.*
