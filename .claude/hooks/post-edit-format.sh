#!/bin/bash
# .claude/hooks/post-edit-format.sh
# Guardrail: runs gofmt on edited Go files to ensure consistent formatting
# Used as a Claude Code hook after file edits

set -euo pipefail

FILE="$1"

if [[ "$FILE" == *.go ]]; then
  echo "Running gofmt on $FILE..."
  gofmt -w "$FILE"
  echo "Formatted: $FILE"
fi
