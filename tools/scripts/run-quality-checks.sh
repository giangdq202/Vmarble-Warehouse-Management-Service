#!/bin/bash
# tools/scripts/run-quality-checks.sh
# Runs all quality checks in sequence: fmt, vet, lint, test, module boundaries
# Run: bash tools/scripts/run-quality-checks.sh

set -euo pipefail

echo "=== Step 1/5: gofmt check ==="
UNFORMATTED=$(gofmt -l ./internal/ ./cmd/ 2>/dev/null || true)
if [ -n "$UNFORMATTED" ]; then
  echo "Unformatted files:"
  echo "$UNFORMATTED"
  echo "Run: gofmt -w ./internal/ ./cmd/"
  exit 1
fi
echo "OK"

echo ""
echo "=== Step 2/5: go vet ==="
go vet ./...
echo "OK"

echo ""
echo "=== Step 3/5: golangci-lint ==="
golangci-lint run ./...
echo "OK"

echo ""
echo "=== Step 4/5: Module boundary check ==="
bash tools/scripts/check-module-boundaries.sh
echo "OK"

echo ""
echo "=== Step 5/5: Tests ==="
go test ./... -race -count=1
echo "OK"

echo ""
echo "=== All quality checks passed ==="
