---
name: senior-workflow
description: >
  Use when starting ANY non-trivial issue or feature on the Go backend — covers
  the full Senior Engineer workflow: requirements clarification -> technical design
  -> task breakdown -> implement + test -> self-QA -> PR.
  ALWAYS trigger this skill when the user says "implement", "add feature",
  "fix issue", "build", "create endpoint", or pastes a GitHub issue/ticket.
  Do NOT skip phases — especially Phase 5 (Self-QA) which is the most commonly
  forgotten step before submitting code.
---

# Senior Engineer Workflow - Go Backend

Run these 6 phases in order. Mark each one done before moving to the next.
Never skip Phase 5 — it is the gate before PR.

## Phase 1 - Requirements Clarification

Before writing a single line of code, understand the why.

Questions to answer (ask the user if unclear):
- What problem does this solve for the business? Which BR-* rule does it touch?
- What is the exact Definition of Done?
- Adversarial edge cases to surface:
  - "What happens if this is called twice concurrently?"
  - "Does this touch area conservation (BR-K03) or costing (BR-C02)?"
- Is there a Sprint issue number?

Output of this phase: a short bullet list: Why / DoD / Edge cases identified.

## Phase 2 - Technical Design and Artifact Generation

Design before coding. Every non-trivial task must have a design record.

Task: Generate a Design Artifact
- Create a temporary markdown file detailing the architecture, store changes, and invariants before implementation.

Module impact analysis:
- Cross-module boundaries? -> Define interface in deps.go.
- Domain invariants: Area conservation (BR-K03), Remnant lineage (BR-K04), Costing immutability (BR-C04).

Data model decisions:
- Additive migrations only.
- Read-then-write must use SELECT FOR UPDATE inside a transaction.

Output of this phase: A summary of the technical design and confirmation of the design artifact.

## Phase 3 - Task Breakdown

Split into discrete tasks of 2–4 hours.

Typical order:
1. Write migration.
2. Update iface.go and store.go interfaces.
3. Implement pgstore.go (SQL).
4. Implement service.go (business logic + validation).
5. Update handler.go (HTTP binding).
6. Update mockStore and write tests.

## Phase 4 - Implement and Test

Service implementation order:
1. Input validation.
2. Fetch entities (FOR UPDATE if writing).
3. Business rule checks (BR-*).
4. Atomic store call.

Test coverage rules:
- Happy-path and error branches for service.go.
- Integration tests for all new pgstore logic.
- Concurrent tests for atomic methods.

## Phase 5 - Self-QA and Continuous Verification

This is the gate before PR.

Code smell checklist:
- Extracted constants for magic values.
- Meaningful error context.
- Context handled correctly from request flow.
- No business logic in handler or store.

Continuous Verification:
- Re-scan all modified files and migrations immediately before finalization to ensure no structural regressions.

Automated checks:
- make test (all green)
- make lint (clean)
- make build (compiles)

## Phase 6 - PR and Knowledge Extraction

PR body template must be followed.

Final Step: Update Instincts
- Analyze the completed task for any new Go patterns, concurrency pitfalls, or monolith instincts discovered.
- Append these lessons to Vmarble-Warehouse-Management-Service/INSTINCTS.md.

Commit message format: `<type>(scope): brief description` (conventional-commit, ASCII subject ≤72 chars).

Authorship policy (strict):
- Do NOT add `Co-Authored-By: Claude …` to commit messages.
- Do NOT add `🤖 Generated with [Claude Code]` (or any equivalent attribution) to commit messages or PR descriptions.
- Commits and PRs are authored by the human running the session — keep them clean.
