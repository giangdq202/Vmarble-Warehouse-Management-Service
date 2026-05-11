#!/usr/bin/env bash
# Claude Code PreToolUse hook — enforce English conventional-commit subjects.
#
# Reads a JSON event from stdin with shape:
#   { "tool_name": "Bash", "tool_input": { "command": "..." }, ... }
#
# Exit codes:
#   0 → allow tool call (no commit, or commit subject is valid).
#   2 → block tool call; stderr is shown to Claude as feedback.
#
# Convention enforced on the SUBJECT line only:
#   <type>(optional-scope)!?: <description>
# Allowed types: feat fix docs style refactor perf test build ci chore revert
# Subject must be ASCII (no Vietnamese diacritics). Body may be Vietnamese.

set -euo pipefail

input="$(cat)"

# Only inspect Bash tool calls.
tool_name="$(printf '%s' "$input" | sed -n 's/.*"tool_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
if [[ "$tool_name" != "Bash" ]]; then
  exit 0
fi

# Robust JSON unescape via python.
command="$(
  printf '%s' "$input" | python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
    print((data.get("tool_input") or {}).get("command", ""))
except Exception:
    pass
'
)"

# Only act on git commit invocations.
if ! grep -qE '(^|[[:space:];&|])git[[:space:]]+commit([[:space:]]|$)' <<<"$command"; then
  exit 0
fi

# Skip --amend without a message (opens $EDITOR; we cannot validate).
if grep -qE '(^|[[:space:]])--amend([[:space:]]|$)' <<<"$command" \
   && ! grep -qE -- '-m[[:space:]]|-m"|--message' <<<"$command"; then
  exit 0
fi

# Extract subject. Supported shapes:
#   1) git commit -m "subject"
#   2) git commit -m "$(cat <<'EOF' ... EOF)"
#   3) git commit --message="subject"
subject="$(COMMAND="$command" python3 <<'PY'
import os, re, sys

cmd = os.environ.get("COMMAND", "")

def first_nonempty(text: str) -> str:
    for line in text.splitlines():
        s = line.strip()
        if s:
            return s
    return ""

heredoc = re.search(
    r"<<[-]?\s*['\"]?([A-Za-z_][A-Za-z0-9_]*)['\"]?\s*\n(.*?)\n\s*\1\b",
    cmd, re.DOTALL,
)
if heredoc:
    print(first_nonempty(heredoc.group(2)))
    sys.exit(0)

m = re.search(r'-m\s*"((?:\\.|[^"\\])*)"', cmd, re.DOTALL)
if not m:
    m = re.search(r"-m\s*'((?:[^'\\]|\\.)*)'", cmd, re.DOTALL)
if not m:
    m = re.search(r'--message[=\s]+"((?:\\.|[^"\\])*)"', cmd, re.DOTALL)

if m:
    raw = m.group(1).encode("utf-8").decode("unicode_escape", errors="replace")
    print(first_nonempty(raw))
PY
)"

# No subject parsed → likely opens $EDITOR; allow.
if [[ -z "$subject" ]]; then
  exit 0
fi

fail() {
  cat >&2 <<EOF
✗ Commit subject violates project convention.

  Subject: $subject

  $1

  Required format:
    <type>(optional-scope)!?: <description in English>

  Allowed types: feat | fix | docs | style | refactor | perf | test | build | ci | chore | revert
  Rules:
    • Subject must be English (ASCII only — no Vietnamese diacritics).
    • Body may be Vietnamese — only the subject line is checked.
    • Description starts with a lowercase verb (add, fix, update, ...).

  Valid examples:
    feat(waste-report): add CSV export
    fix(work-orders): prevent double advance on slow network
    docs(readme): update RBAC table
EOF
  exit 2
}

# Rule 1: ASCII only (catches Vietnamese diacritics in subject).
if ! SUBJECT="$subject" python3 -c 'import os,sys; sys.exit(0 if all(ord(c) < 128 for c in os.environ["SUBJECT"]) else 1)'; then
  fail "→ Subject contains non-ASCII characters (Vietnamese diacritics)."
fi

# Rule 2: Conventional commit shape.
if ! grep -qE '^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9._/-]+\))?!?: [a-z].+' <<<"$subject"; then
  fail "→ Subject does not match conventional-commit pattern."
fi

# Rule 3: Length budget (matches gitlint default of 72).
if (( ${#subject} > 72 )); then
  fail "→ Subject is ${#subject} characters (limit is 72)."
fi

exit 0
