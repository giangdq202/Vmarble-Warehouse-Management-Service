# Gemini CLI Guidance — VMARBLE Backend

This file mandates the behavior of Gemini CLI to ensure strict adherence to the project's **Modular Monolith** architecture and **Vietnamese Business Logic**.

## 🛠 Skill Discovery (Claude Compatibility)
Gemini does not trigger skills automatically. Instead, you MUST manually "load" the expert guidance for the current task:
1. **Identify**: Check `.claude/skills/` for a relevant subfolder.
2. **Read**: Use `read_file` on the `SKILL.md` inside that folder.
3. **Adopt**: Treat the instructions in that file as **Foundation Mandates** for the rest of the session.

---

## 🛠 Active Skills

| Skill | Activation Trigger |
|-------|--------------------|
| `senior-workflow` | **Mandatory** for any non-trivial feature or fix. Follow Phases 1-6. |
| `business-auditor` | Any change to `service.go` or logic involving `BR-*` rules. |
| `product-manager` | Backlog analysis, sprint planning, or task breakdown. |
| `integration-architect` | Changes to `iface.go`, `deps.go`, or API contracts. |
| `deploy` / `rbac-hardener` | Deployment tasks or security/role-guard audits. |

---

## 🤖 Automation Workflow (Trigger: "Làm task tiếp theo")

When triggered, follow this sequence without further instruction:

1. **Fetch**: Run `gh issue list --limit 1` to find the next task.
2. **Analyze**: Run `gh issue view <id>` to read the requirements and DoD.
3. **Audit**: Invoke `business-auditor`. Cross-reference the issue with `docs/backend-business-logic-vi.md`. Identify all impacted `BR-*` rules.
4. **Plan**: Activate `senior-workflow` and start **Phase 1 (Clarification)**. 
   - *Example Issue Title*: `[inventory] Triển khai quy tắc BR-K03 Bảo toàn diện tích`
   - *Example Issue Description*: `Cần kiểm tra tổng diện tích used + remnant <= source_area trong service RecordCut.`
5. **Architect**: If the task changes API signatures, invoke `integration-architect` to warn about Frontend sync.

---

## 🏗 Modular Monolith Rules
- **No Cross-Module Imports**: Modules in `internal/module/` are black boxes. Use `deps.go`.
- **Service-Only Logic**: Business rules live in `service.go`. Never in `handler.go` or `pgstore.go`.
- **BizError**: Always return `domain.NewBizError` for business violations to ensure correct HTTP mapping (422, 409).

---

## 🧪 Testing Standards
- **80% Coverage**: Every PR must maintain 80% coverage on `service.go`.
- **Integration Tests**: Required for any new SQL or transaction logic in `pgstore.go`.
- **Race Check**: Use `make test` (which includes `-race`).

---

## 🚀 Commands
- `make dev`: Start Docker + Migrations + Server.
- `make test-integration`: Run DB-backed tests.
- `make swagger`: Regenerate API docs.
