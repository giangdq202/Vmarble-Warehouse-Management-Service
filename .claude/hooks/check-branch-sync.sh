#!/usr/bin/env bash
# Claude Code PreToolUse hook — enforce git fetch before branching.
#
# Blocks `git checkout -b` if origin/dev is ahead of local HEAD,
# forcing the agent to pull first and avoid stale-base branches.
#
# Exit codes:
#   0 → allow
#   2 → block; stderr shown to Claude as feedback

set -euo pipefail

input="$(cat)"

tool_name="$(printf '%s' "$input" | python3 -c '
import json,sys
try: print(json.load(sys.stdin).get("tool_name",""))
except: pass
')"
[[ "$tool_name" == "Bash" ]] || exit 0

command="$(printf '%s' "$input" | python3 -c '
import json,sys
try:
    d=json.load(sys.stdin)
    print((d.get("tool_input") or {}).get("command",""))
except: pass
')"

# Only intercept branch creation commands
if ! grep -qE '(^|[[:space:];&|])git[[:space:]]+checkout[[:space:]]+-b([[:space:]]|$)' <<<"$command"; then
  exit 0
fi

# Fetch silently to get up-to-date remote refs
git fetch origin --quiet 2>/dev/null || true

# Check how many commits origin/dev has that local HEAD doesn't
BEHIND=$(git log HEAD..origin/dev --oneline 2>/dev/null | wc -l | tr -d ' ')

if [[ "$BEHIND" -gt 0 ]]; then
  cat >&2 <<EOF
✗ Branch sync check failed — origin/dev is ahead by ${BEHIND} commit(s).

  Your local dev is stale. Run these first, then retry:

    git checkout dev
    git pull origin dev

  Then re-create your feature branch from the updated dev.
  This prevents merge conflicts from missing migrations or schema changes.
EOF
  exit 2
fi

exit 0
