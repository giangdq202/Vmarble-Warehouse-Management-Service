# Release Skill

You are preparing a release for the VMARBLE Warehouse Management Service.

## Pre-release checklist

### Code quality
- [ ] `make test` passes (all tests green, no race conditions)
- [ ] `make lint` passes (no linter warnings)
- [ ] `make build` succeeds (binary compiles cleanly)
- [ ] No TODO/FIXME items blocking release

### Database
- [ ] All migrations have both `Up` and `Down` functions
- [ ] Migrations are forward/backward safe
- [ ] `make migrate-up` runs cleanly on a fresh database
- [ ] `make migrate-down` can rollback without data loss

### Documentation
- [ ] `README.md` is up to date
- [ ] `CLAUDE.md` reflects any new conventions or modules
- [ ] Swagger docs regenerated: `make swagger`
- [ ] New ADRs written for architectural decisions

### Branch rules
- [ ] Feature branches merged to `dev` via PR
- [ ] `dev` → `main` PR created with at least 1 approval
- [ ] No force-pushes to protected branches

## Release steps

1. Ensure `dev` branch is stable and all PRs merged
2. Create PR from `dev` → `main`
3. PR title: `Release vX.Y.Z — brief description`
4. PR body includes:
   - Summary of changes since last release
   - Business rules impacted (BR-* references)
   - Migration notes (if any new migrations)
   - Breaking changes (if any)
5. Get required approval
6. Merge to `main`
7. Tag the release: `git tag vX.Y.Z`

## Version bump

Follow semantic versioning:
- **PATCH** (0.0.X): bug fixes, no API changes
- **MINOR** (0.X.0): new features, backward compatible
- **MAJOR** (X.0.0): breaking API changes
