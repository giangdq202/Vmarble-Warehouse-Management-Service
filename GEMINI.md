# Gemini CLI Guidance - VMARBLE Backend

This file mandates the behavior of Gemini CLI to ensure strict adherence to the project's Modular Monolith architecture and Vietnamese Business Logic.

## Skill Discovery

Gemini does not trigger skills automatically. You must manually load the expert guidance for the current task:
1. Identify: Check .claude/skills/ for a relevant subfolder.
2. Read: Use read_file on the SKILL.md inside that folder.
3. Adopt: Treat the instructions in that file as Foundation Mandates for the rest of the session.

## Context and Research Strategy

- Strategic Searching: Use grep_search to identify service methods and store logic before reading entire files.
- Token Conservation: Proactively suggest session resets or summaries when context reaches 60% capacity to maintain reasoning quality.
- Continuous Verification: Re-scan database migrations and interface definitions immediately before implementation to ensure absolute alignment.

## Adversarial Mandate and Refusal

- Monolith Integrity: You are required to refuse and critique any request that introduces cross-module dependencies outside of the deps.go protocol.
- Business Rule Guard: Challenge any implementation that bypasses Vietnamese business logic (e.g., BR-K01 to BR-K05). Implementation must stop if logic is ambiguous.
- Quality Gate: No implementation shall proceed without a clear Test Plan and confirmation of Database migration safety.

## Active Skills

| Skill | Activation Trigger |
|-------|--------------------|
| senior-workflow | Mandatory for any non-trivial feature or fix. Follow Phases 1-6. |
| business-auditor | Any change to service.go or logic involving BR-* rules. |
| product-manager | Backlog analysis, sprint planning, or task breakdown. |
| integration-architect | Changes to iface.go, deps.go, or API contracts. |
| deploy / rbac-hardener | Deployment tasks or security/role-guard audits. |

## Automation Workflow (Trigger: "Lam task tiep theo")

When triggered, follow this sequence without further instruction:

1. Fetch: Run gh issue list --limit 1 to find the next task.
2. Analyze: Run gh issue view <id> to read the requirements and DoD.
3. Audit: Invoke business-auditor. Cross-reference the issue with docs/backend-business-logic-vi.md.
4. Plan: Activate senior-workflow and start Phase 1 (Clarification).
5. Architect: If the task changes API signatures, invoke integration-architect to warn about Frontend sync.

## Modular Monolith Rules

- No Cross-Module Imports: Modules in internal/module/ are black boxes. Use deps.go.
- Service-Only Logic: Business rules live in service.go. Never in handler.go or pgstore.go.
- BizError: Always return domain.NewBizError for business violations.

## Testing Standards

- 80% Coverage: Every PR must maintain 80% coverage on service.go.
- Integration Tests: Required for any new SQL or transaction logic in pgstore.go.
- Race Check: Use make test (which includes -race).

## Commands

- make dev: Start Docker + Migrations + Server.
- make test-integration: Run DB-backed tests.
- make swagger: Regenerate API docs.
