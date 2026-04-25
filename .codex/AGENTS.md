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

**Trigger**: user says **"làm task tiếp theo"** / **"Start next task"** / picks an issue from GitHub Projects.

1. **Fetch** — use `product-manager` and identify the highest-priority assigned open issue.
2. **Analyze** — read the issue requirement and DoD.
3. **Audit** — invoke `business-auditor`, map impacted BR-* rules from `docs/backend-business-logic-vi.md`, and block implementation if any rule is unclear.
4. **Implement** — invoke `senior-workflow`, starting from Phase 1. Do not skip phases.
5. **Architect** — invoke `integration-architect` if the task adds or changes endpoints, DTO contracts in `iface.go`, or cross-module interfaces in `deps.go`.
