#!/bin/bash
# tools/scripts/check-module-boundaries.sh
# Verifies that no module under internal/module/ imports another module.
# Run: bash tools/scripts/check-module-boundaries.sh

set -euo pipefail

MODULES_DIR="internal/module"
VIOLATIONS=0

for module_dir in "$MODULES_DIR"/*/; do
  module_name=$(basename "$module_dir")
  module_import_path="internal/module/$module_name"

  for other_dir in "$MODULES_DIR"/*/; do
    other_name=$(basename "$other_dir")
    [ "$module_name" = "$other_name" ] && continue

    other_import_path="internal/module/$other_name"

    # Search for imports of other modules in this module's Go files
    if grep -rq "\".*${other_import_path}\"" "$module_dir" 2>/dev/null; then
      echo "VIOLATION: $module_name imports $other_name"
      grep -rn "\".*${other_import_path}\"" "$module_dir"
      VIOLATIONS=$((VIOLATIONS + 1))
    fi
  done
done

if [ "$VIOLATIONS" -eq 0 ]; then
  echo "All module boundaries are clean."
  exit 0
else
  echo ""
  echo "Found $VIOLATIONS module boundary violation(s)!"
  exit 1
fi
