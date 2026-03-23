#!/bin/bash
# .claude/hooks/pre-commit-lint.sh
# Guardrail: runs golangci-lint on staged Go files before allowing commit
# Used as a Claude Code hook to catch lint issues early

set -euo pipefail

STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$' || true)

if [ -z "$STAGED_GO_FILES" ]; then
  echo "No Go files staged, skipping lint."
  exit 0
fi

echo "Running golangci-lint on staged Go files..."
golangci-lint run --new-from-rev=HEAD ./...

echo "Lint passed."
