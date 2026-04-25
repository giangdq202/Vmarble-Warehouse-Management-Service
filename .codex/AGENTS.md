# .codex for Vmarble Warehouse Management Service

This file provides Codex-specific guidance for this repository.

## Source documents (read first)

- `README.md` — project goals, stack, commands, structure, branch rules
- `docs/backend-business-logic-vi.md` — authoritative business spec

If there is a conflict, prefer `docs/backend-business-logic-vi.md`.

## .codex structure

- `.codex/config.toml`: runtime, MCP, and multi-agent configuration for Codex CLI.
- `.codex/agents/*.toml`: child-agent role configs (`explorer`, `reviewer`, `docs-researcher`).
- `.codex/skills/`: project skills mirrored from `.claude/skills` using Codex's required `SKILL.md` naming.

## Skills

| Skill | Auto-activates when… |
|-------|----------------------|
| `senior-workflow` | Starting any non-trivial Go feature/fix — run Phases 1–6 in order |
| `business-auditor` | Task mentions a BR-* rule, or touches `service.go` business logic |
| `product-manager` | Backlog management, sprint planning, GitHub issue triage/creation |
| `integration-architect` | New endpoint, API contract change, new cross-module `deps.go` interface |
| `deploy` | User says "deploy", "docker", "CI/CD", "go live", "staging", "production", or "health check" |
| `rbac-hardener` | User says "RBAC", "role guard", "access control", "security audit", or before go-live |
| `code-review` | User asks for review, audit, findings, PR review, or correctness/security review |
| `refactor` | User asks to refactor, clean up, simplify, or restructure existing Go code |
| `release` | User asks for release prep, versioning, tagging, or release checklist work |
| `testing` | User asks to add/fix tests or validate service/store/handler behavior |

Rule: `business-auditor` and `integration-architect` run inside `senior-workflow`; they do not replace it.

## Codex vs Claude hooks

Codex auto-discovers project skills from `.codex/skills/**/SKILL.md`, but this repo does **not** currently have a Codex-native project hooks system equivalent to `.claude/hooks`.

Use these Codex-native substitutes instead:

1. Put durable behavior in `.codex/AGENTS.md` and `.codex/config.toml`.
2. Keep reusable project instructions in `.codex/skills/*/SKILL.md` so Codex can auto-trigger them.
3. Keep shell guardrails as executable scripts under `.codex/scripts/` and run them explicitly when needed.
4. Keep Git hooks in `.git/hooks` or a managed hook framework if you want enforcement outside the assistant.

Current repo guardrails worth preserving from Claude:
- `.claude/hooks/post-edit-format.sh` -> run `gofmt -w` on changed Go files.
- `.claude/hooks/pre-commit-lint.sh` -> run `golangci-lint run --new-from-rev=HEAD ./...` before commit.

Recommended Codex workflow:
- After Go edits: run `gofmt -w <changed-files>`.
- Before finalizing code: run `make test` and `make lint`.
- Before commit: optionally run a local Git pre-commit hook that invokes the same lint script.

## Automation workflow

**Primary triggers**: user says **"làm task tiếp theo"** / **"implement the next task"** / **"Start next task"** / picks an issue from GitHub Projects.

### Default GitHub project source
- Owner: `giangdq202`
- Project: `VWMS-project`
- Assignee focus: `thdat-vu`
- Preferred repo: `giangdq202/Vmarble-Warehouse-Management-Service`

### Definition of “next task”
When a trigger phrase is used, resolve the next task from GitHub Projects with these filters:
1. issue is open
2. issue belongs to `VWMS-project`
3. issue is assigned to `thdat-vu`
4. project Status is not `Done`

If multiple items match, sort by:
1. `Priority`: `P0` > `P1` > `P2`
2. `Status`: `Ready` > `Backlog` > `In progress` > `In review`
3. `Estimate`: smaller numeric value first when available
4. lower issue number first as deterministic tie-breaker

If no matching issue exists, say so clearly and stop rather than guessing.

### Required execution flow for “next task”
1. **Fetch** — use `product-manager` and locate the next issue from GitHub Projects.
2. **Analyze** — read the issue body, linked context, and DoD.
3. **Audit** — invoke `business-auditor`, map impacted BR-* rules from `docs/backend-business-logic-vi.md`, and block implementation if any rule is unclear.
4. **Implement** — invoke `senior-workflow`, starting from Phase 1. Do not skip phases.
5. **Architect** — invoke `integration-architect` if the task adds or changes endpoints, DTO contracts in `iface.go`, or cross-module interfaces in `deps.go`.
6. **Update project state** — if possible, move the selected project item to `In progress` when implementation begins.
7. **Create branch** — branch from latest `dev` using one of these patterns:
   - `feat/issue-<number>-<slug>`
   - `fix/issue-<number>-<slug>`
   - `chore/issue-<number>-<slug>`
8. **Validate** — run targeted tests first, then broader checks as needed (`gofmt`, `make test`, `make lint` when appropriate).
9. **Commit** — use Conventional Commit style with issue context when possible.
10. **Open PR** — create a PR against `dev` with:
   - concise title
   - summary of changes
   - test evidence
   - `Closes #<issue-number>` when the PR fully resolves the issue
11. **Report back** — summarize branch, commit, validation, PR URL, and any remaining risks.

### PR standard
- Base branch: `dev`
- Prefer PR title format: `[module] type: short summary`
- PR body should include:
  - Summary
  - Scope / files changed
  - Validation performed
  - Business rules or issue link
  - Risks / follow-ups

### Safety rules
- Do not auto-pick issues from other repositories unless the user explicitly asks.
- Do not create a PR without at least one validation step unless the user explicitly waives validation.
- Do not mark a project item `Done` until the implementation is merged or the user explicitly requests otherwise.
