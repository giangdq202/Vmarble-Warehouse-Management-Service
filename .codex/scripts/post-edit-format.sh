#!/bin/bash
set -euo pipefail

FILE="${1:-}"
if [[ -n "$FILE" && "$FILE" == *.go ]]; then
  echo "Running gofmt on $FILE..."
  gofmt -w "$FILE"
  echo "Formatted: $FILE"
fi
