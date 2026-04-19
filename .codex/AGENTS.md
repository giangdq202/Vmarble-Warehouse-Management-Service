# .codex for Vmarble Warehouse Management Service

This file provides Codex-specific guidance for this repository.

## Source documents (read first)

- `README.md` — project goals, stack, commands, structure, branch rules
- `docs/backend-business-logic-vi.md` — authoritative business spec

If there is a conflict, prefer `docs/backend-business-logic-vi.md`.

## .codex structure

- `.codex/config.toml`: runtime/MCP/multi-agent configuration.
- `.codex/agents/*.toml`: child-agent role configs (`explorer`, `reviewer`, `docs-researcher`).
- `.codex/skills/`: project skills mirrored from `.claude/skills`.

## Skills

| Skill | Auto-activates when… |
|-------|----------------------|
| `senior-workflow` | Starting any non-trivial Go feature/fix — run Phases 1–6 in order |
| `business-auditor` | Task mentions a BR-* rule, or touches `service.go` business logic |
| `product-manager` | Backlog management, sprint planning, GitHub issue triage/creation |
| `integration-architect` | New endpoint, API contract change, new cross-module `deps.go` interface |
| `deploy` | User says "deploy", "docker", "CI/CD", "go live", "staging", "production", "health check" |
| `rbac-hardener` | User says "RBAC", "role guard", "access control", "security audit", or before go-live |

Rule: `business-auditor` and `integration-architect` run inside `senior-workflow`; they do not replace it.

## Automation Workflow

**Trigger**: user says **"làm task tiếp theo"** / **"Start next task"** / picks an issue from GitHub Projects.

1. **Fetch** — use `product-manager` and identify highest-priority assigned open issue:
   ```bash
   gh issue list --repo giangdq202/Vmarble-Warehouse-Management-Service \
     --assignee @me --state open --json number,title,labels \
     | jq 'sort_by(.labels[].name) | .[0]'
   ```
2. **Analyze** — read full requirement and DoD:
   ```bash
   gh issue view <number> --repo giangdq202/Vmarble-Warehouse-Management-Service
   ```
3. **Audit** — invoke `business-auditor`, map impacted BR-* rules from `docs/backend-business-logic-vi.md`, and block implementation if any rule is unclear.
4. **Implement** — invoke `senior-workflow`, start from Phase 1 (Requirements Clarification), do not skip phases.
5. **Architect** — invoke `integration-architect` if the task adds/changes endpoints, DTO contracts in `iface.go`, or cross-module interfaces in `deps.go`.
