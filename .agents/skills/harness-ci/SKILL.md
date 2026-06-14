---
name: harness-ci
description: >
  Use when setting up, modifying, or debugging Harness CI pipelines for the
  VMARBLE project. Covers connector setup, pipeline authoring (Visual editor vs
  YAML), secrets, triggers, free-tier constraints, and common errors.
  Trigger words: "harness", "CI pipeline", "build pipeline", "connector",
  "harness cloud", "docker build ci", "pipeline trigger".
---

# Harness CI — VMARBLE Pipeline Reference

This skill encodes hard-won lessons from the initial Harness setup session for the
VMARBLE project. Read all sections before touching any pipeline or connector.

---

## Free-Tier Constraints

| Constraint | Value |
|---|---|
| Organizations | 1 (named `default`, **cannot be renamed**) |
| Build credits/month | 2,000 |
| Harness Cloud infrastructure | Requires a credit card on file |
| Alternative to Cloud | `Local` runner (needs a Harness Delegate installed on your own server) |

If you do not want to add a credit card, choose `Local` as the infrastructure type
and install a Delegate on the Dokploy/VPS host first.

---

## Projects

Two separate Harness projects map to the two GitLab repos:

| Harness project | GitLab repo |
|---|---|
| `vmarble-be` | `529-stu/Vmarble-Warehouse-Management-Service` |
| `vmarble-fe` | `529-stu/Vmarble-Warehouse-Management-Client` |

**Connectors are PROJECT-scoped.** You must create each connector twice — once in
`vmarble-be` and once in `vmarble-fe`. There is no org-level sharing on the free tier.

---

## Connector Setup

### 1. GitLab Source Code Connector

Create one per project under **Project Settings → Connectors → + New Connector → Code Repositories → GitLab**.

| Field | Value |
|---|---|
| Connection type | Account |
| URL | `https://gitlab.com` (account-level, NOT repo URL) |
| Authentication | Username + Personal Access Token (PAT) |
| PAT scopes needed | `read_repository`, `read_registry`, `write_registry` |
| API access | Enable — use the same PAT |

**Known false alarm**: The connection test may return `"Illegal character in path at index …"`.
This happens when Harness scans your GitLab account and encounters a repo whose name
contains a space (a different repo, not yours). The connector itself is fine.
Click **Finish** anyway — the error does not affect pipeline execution.

### 2. Docker Registry Connector

Create one per project under **Project Settings → Connectors → + New Connector → Artifact Repositories → Docker Registry**.

| Field | Value |
|---|---|
| Docker Registry URL | `https://registry.gitlab.com` |
| Authentication | Username + Password |
| Username | Your GitLab username (or deploy token username) |
| Password | GitLab PAT or deploy token with `read_registry` + `write_registry` |

---

## Secrets

Store sensitive values under **Project Settings → Secrets → + New Secret → Text**.

Reference in pipeline YAML or Visual editor with:

```
<+secrets.getValue("secretIdentifier")>
```

Recommended secrets to create in each project:

| Secret identifier | Value |
|---|---|
| `gitlabRegistryToken` | GitLab PAT / deploy token |
| `gitlabRegistryUser` | GitLab username |
| `dokployBeWebhookUrl` | Dokploy webhook URL for BE |
| `dokployFeWebhookUrl` | Dokploy webhook URL for FE |

---

## Pipeline Architecture

### BE Pipeline (`vmarble-be`) — 2 Stages

Mirrors the CircleCI config at `.circleci/config.yml`:

| Stage | Name | Runs on | Condition |
|---|---|---|---|
| 1 | Go Build Gate | All branches **except** `dev` | `<+trigger.branch> != "dev"` |
| 2 | Docker Build and Deploy | `dev` branch only | `<+trigger.branch> == "dev"` |

**Stage 1 — Go Build Gate**
- Step type: Run
- Container image: `golang:1.24-alpine`
- Command: `go build ./...`
- Infrastructure: Cloud (or Local delegate)

**Stage 2 — Docker Build and Deploy**
- Infrastructure: select **Propagate from an existing stage** and pick Stage 1 — this reuses the same runner without re-provisioning
- Steps:
  1. Build & push Docker image (use the Docker Registry connector)
  2. Tag: `sha-<+pipeline.sequenceId>` and `stg-latest`
  3. Trigger Dokploy webhook: `curl -sS -f -X POST <+secrets.getValue("dokployBeWebhookUrl")>`

### FE Pipeline (`vmarble-fe`) — 1 Stage

Mirrors `.circleci/config.yml` in the FE repo:

| Stage | Name | Runs on | Condition |
|---|---|---|---|
| 1 | Docker Build and Deploy | `dev` branch only | `<+trigger.branch> == "dev"` |

Steps mirror Stage 2 of the BE pipeline, substituting the FE image name and webhook secret.

---

## JEXL Conditional Expressions

Harness uses JEXL for conditional execution on stages and steps.

| Use case | Expression |
|---|---|
| Run on all branches except `dev` | `<+trigger.branch> != "dev"` |
| Run on `dev` only | `<+trigger.branch> == "dev"` |
| Run on `main` only | `<+trigger.branch> == "main"` |
| Run on feature branches | `<+trigger.branch> =~ "^feature/.*"` |

Apply conditionals on the **Stage** level under **Advanced → Conditional Execution**.

---

## Tag Strategy

| Tag | Purpose | Harness expression |
|---|---|---|
| `sha-<seq>` | Immutable per build | `sha-<+pipeline.sequenceId>` |
| `stg-latest` | Rolling staging tag | literal string |

Do NOT use `<+pipeline.executionId>` for image tags — it is a UUID, not a short
human-readable identifier. Use `sequenceId` instead (incremental integer).

---

## Visual Editor vs YAML

**Always use the Visual editor first.** The Harness YAML schema is strict and
underdocumented. Hand-written YAML routinely fails with vague validation errors:

- `namespace` field missing or wrong
- `useFromStage` not accepted where expected
- `spec` block structure differs by step type
- `connectorRef` path format varies (project vs org vs account scope)

**Workflow:**
1. Build the pipeline entirely in the Visual editor.
2. After the pipeline saves successfully, open the **YAML tab**.
3. Copy the generated YAML and save it alongside the pipeline for future reference
   (e.g. `docs/ci/harness-be-pipeline.yaml`).
4. For future edits, use the Visual editor unless the change is a simple string swap.

---

## Triggers

Create triggers under **Pipeline → Triggers → + New Trigger → Webhook → GitLab**.

| Field | Value |
|---|---|
| Connector | The GitLab connector for this project |
| Event | Push Branch |
| Branch regex | `.*` (all branches) — let stage conditionals filter |

The stage-level JEXL conditions (section above) control which stages actually run.
Using a broad trigger + tight stage conditions is simpler than maintaining multiple
narrow triggers.

---

## Common Errors and Fixes

### "Illegal character in path at index …" during connector test
**Cause**: Another repo in your GitLab account has a space in its name.
**Fix**: Ignore the error; click **Finish**. The connector works correctly.

### YAML validation: "namespace is required" / "type is required"
**Cause**: Hand-authored YAML missing fields that the Visual editor sets automatically.
**Fix**: Use the Visual editor. Copy resulting YAML from the YAML tab after saving.

### YAML validation: "useFromStage not valid here"
**Cause**: `useFromStage` block placed at wrong nesting level.
**Fix**: Use Visual editor → **Propagate from an existing stage** option on Stage 2.

### "Infrastructure type Cloud requires credit card"
**Cause**: Free tier Harness Cloud needs billing info.
**Fix**: Either add a credit card, or switch infrastructure type to `Local` and
install a Harness Delegate on the Dokploy VPS.

### Build credits exhausted
**Cause**: 2,000 credits/month consumed.
**Fix**: Switch to `Local` infrastructure (self-hosted delegate = no credit cost),
or upgrade plan.

### Docker push fails: "unauthorized"
**Cause**: Wrong registry URL or PAT missing `write_registry` scope.
**Fix**: Verify Docker Registry connector URL is `https://registry.gitlab.com`
(not `https://gitlab.com`). Verify PAT scopes include `write_registry`.

---

## Reference: CircleCI → Harness Mapping

| CircleCI concept | Harness equivalent |
|---|---|
| `executor` | Stage infrastructure (Cloud / Local) |
| `job` | Stage |
| `step` | Step inside a Stage |
| `context: lab-stg` | Secrets at project/org scope |
| `filters.branches.only: dev` | Stage conditional: `<+trigger.branch> == "dev"` |
| `filters.branches.ignore: dev` | Stage conditional: `<+trigger.branch> != "dev"` |
| `CIRCLE_SHA1` | `<+pipeline.sequenceId>` (for tags; full SHA not available by default) |
| `CIRCLE_BUILD_NUM` | `<+pipeline.sequenceId>` |
| `restore_cache` / `save_cache` | Not needed for Cloud; configure caching in Build step settings |

---

## Saved YAML Location

After creating pipelines via the Visual editor, save the generated YAML here for
reference and disaster recovery:

- BE: `docs/ci/harness-be-pipeline.yaml`
- FE: `docs/ci/harness-fe-pipeline.yaml` (save in FE repo)

---

## Quick-Start Checklist

When setting up Harness CI from scratch for this project:

- [ ] Create Harness account (free tier)
- [ ] Create project `vmarble-be`
- [ ] Create project `vmarble-fe`
- [ ] In each project: create GitLab connector (account-level, PAT)
- [ ] In each project: create Docker Registry connector (`https://registry.gitlab.com`)
- [ ] In each project: create required secrets (token, user, webhook URL)
- [ ] Build BE pipeline via Visual editor (2 stages with conditionals)
- [ ] Build FE pipeline via Visual editor (1 stage, dev-only)
- [ ] Add webhook trigger in each pipeline (GitLab Push Branch, all branches)
- [ ] Copy final YAML from YAML tab → save to `docs/ci/`
- [ ] Test by pushing a feature branch (Stage 1 should run) then merging to `dev` (Stage 2 should run)
