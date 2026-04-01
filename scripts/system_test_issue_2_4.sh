#!/usr/bin/env bash
# =============================================================================
# System Test — Issue 2.4: Bounding Box (usable_dimension) on Remnant
# =============================================================================
# Usage:
#   ./scripts/system_test_issue_2_4.sh [options]
#
# Options:
#   --base-url URL      API base URL          (default: http://localhost:8080)
#   --pg-port PORT      Postgres host port    (default: 5433)
#   --pg-container NAME Postgres container name (auto-detected if omitted)
#   --username USER     Login username        (default: admin)
#   --password PASS     Login password        (default: admin)
#   --no-color          Disable colour output
#   --stop-on-fail      Stop at first failure
#
# DB access strategy (auto-selected, no config needed):
#   1. local psql   — if `psql` found in PATH
#   2. docker exec  — if `docker` found and postgres container is running
#   If neither is available the DB-verification steps are skipped.
# =============================================================================
set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
BASE_URL="http://localhost:8080"
PG_HOST="localhost"
PG_PORT="5433"
PG_USER="vmarble"
PG_PASS="vmarble"
PG_DB="vmarble"
PG_CONTAINER=""         # auto-detected below when empty
AUTH_USER="admin"
AUTH_PASS="admin"
STOP_ON_FAIL=false
NO_COLOR=false

# ── Argument parsing ──────────────────────────────────────────────────────────
# strip_quotes removes surrounding single or double quotes that some shells
# or terminal emulators pass through literally (e.g. --password '"admin123"').
strip_quotes() { echo "$1" | sed "s/^['\"]//;s/['\"]$//"; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)      BASE_URL="$(strip_quotes "$2")";      shift 2 ;;
    --pg-port)       PG_PORT="$(strip_quotes "$2")";       shift 2 ;;
    --pg-container)  PG_CONTAINER="$(strip_quotes "$2")";  shift 2 ;;
    --username)      AUTH_USER="$(strip_quotes "$2")";     shift 2 ;;
    --password)      AUTH_PASS="$(strip_quotes "$2")";     shift 2 ;;
    --no-color)      NO_COLOR=true;                        shift   ;;
    --stop-on-fail)  STOP_ON_FAIL=true;                    shift   ;;
    *) echo "Unknown option: $1" >&2; exit 1               ;;
  esac
done

# ── Colours ───────────────────────────────────────────────────────────────────
if [[ "$NO_COLOR" == false && -t 1 ]]; then
  RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
  CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
else
  RED=''; GREEN=''; YELLOW=''; CYAN=''; BOLD=''; RESET=''
fi

# ── Counters ──────────────────────────────────────────────────────────────────
PASS=0; FAIL=0; SKIP=0
FAILED_SCENARIOS=()

# ── Logging helpers ───────────────────────────────────────────────────────────
log_header() { echo -e "\n${BOLD}${CYAN}━━━  $*  ━━━${RESET}"; }
log_step()   { echo -e "  ${YELLOW}▶${RESET} $*"; }
log_pass()   { echo -e "  ${GREEN}✔ PASS${RESET}  $*"; (( PASS++ )) || true; }
log_fail()   { echo -e "  ${RED}✘ FAIL${RESET}  $*"; (( FAIL++ )) || true; }
log_skip()   { echo -e "  ${YELLOW}– SKIP${RESET}  $*"; (( SKIP++ )) || true; }
log_info()   { echo -e "         ${YELLOW}$*${RESET}"; }

# ── Assertion helpers ─────────────────────────────────────────────────────────
# assert_status <name> <expected_http_code> <actual_http_code>
assert_status() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    log_pass "[$name] HTTP $actual"
  else
    log_fail "[$name] expected HTTP $expected, got HTTP $actual"
    FAILED_SCENARIOS+=("$name: expected HTTP $expected got $actual")
    if [[ "$STOP_ON_FAIL" == true ]]; then print_summary; exit 1; fi
  fi
}

# assert_json <name> <description> <jq_filter_returning_true|false> <json_body>
assert_json() {
  local name="$1" desc="$2" filter="$3" body="$4"
  local result
  result=$(echo "$body" | jq -r "$filter" 2>/dev/null || echo "JQ_ERROR")
  if [[ "$result" == "true" ]]; then
    log_pass "[$name] $desc"
  else
    log_fail "[$name] $desc  (evaluated: $result)"
    FAILED_SCENARIOS+=("$name: $desc")
    if [[ "$STOP_ON_FAIL" == true ]]; then print_summary; exit 1; fi
  fi
}

# ── DB access layer ───────────────────────────────────────────────────────────
# DB_MODE is set during the prerequisite check:
#   "psql"    — use local psql binary
#   "docker"  — use docker exec into the postgres container
#   "none"    — no DB access; DB checks will be skipped
DB_MODE="none"

# psql_scalar <sql>  — returns one trimmed value; empty string on no row.
psql_scalar() {
  local sql="$1"
  case "$DB_MODE" in
    psql)
      PGPASSWORD="$PG_PASS" psql \
        -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER" -d "$PG_DB" \
        -tAc "$sql" 2>/dev/null || true
      ;;
    docker)
      docker exec -e "PGPASSWORD=$PG_PASS" "$PG_CONTAINER" \
        psql -U "$PG_USER" -d "$PG_DB" -tAc "$sql" 2>/dev/null || true
      ;;
    none)
      echo ""
      ;;
  esac
}

# db_check_available — returns "yes" if DB_MODE != none, otherwise "no"
db_available() { [[ "$DB_MODE" != "none" ]]; }

# ── Summary printer ───────────────────────────────────────────────────────────
print_summary() {
  echo -e "\n${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo -e "${BOLD}  SUMMARY${RESET}"
  echo -e "  ${GREEN}PASS: $PASS${RESET}   ${RED}FAIL: $FAIL${RESET}   ${YELLOW}SKIP: $SKIP${RESET}"
  if [[ ${#FAILED_SCENARIOS[@]} -gt 0 ]]; then
    echo -e "\n  ${RED}Failed checks:${RESET}"
    for s in "${FAILED_SCENARIOS[@]}"; do
      echo -e "    ${RED}•${RESET} $s"
    done
  fi
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
}

# =============================================================================
# PREREQUISITE CHECK
# =============================================================================
log_header "Prerequisite check"

# curl and jq are mandatory
for tool in curl jq; do
  if command -v "$tool" &>/dev/null; then
    log_pass "tool '$tool' found"
  else
    log_fail "tool '$tool' not found — install it first"
    print_summary; exit 1
  fi
done

# ── DB access: try local psql first, then docker exec ─────────────────────────
if command -v psql &>/dev/null; then
  DB_MODE="psql"
  log_pass "DB access: local psql found"
elif command -v docker &>/dev/null && docker info &>/dev/null 2>&1; then
  # Auto-detect the postgres container from the compose project in CWD
  if [[ -z "$PG_CONTAINER" ]]; then
    PG_CONTAINER=$(docker compose ps --format '{{.Name}}' 2>/dev/null \
      | grep -i postgres | head -1 || true)
  fi
  # Fallback: search all running containers
  if [[ -z "$PG_CONTAINER" ]]; then
    PG_CONTAINER=$(docker ps --format '{{.Names}}' \
      | grep -i postgres | head -1 || true)
  fi

  if [[ -n "$PG_CONTAINER" ]]; then
    # Smoke-test: verify psql exists inside the container
    if docker exec "$PG_CONTAINER" psql --version &>/dev/null 2>&1; then
      DB_MODE="docker"
      log_pass "DB access: docker exec → $PG_CONTAINER"
    else
      log_skip "DB access: container $PG_CONTAINER found but psql not responsive — DB checks will be skipped"
    fi
  else
    log_skip "DB access: docker available but no running postgres container found"
    log_info  "Hint: run 'make dev' first, or pass --pg-container <name>"
  fi
else
  log_skip "DB access: neither psql nor docker found — DB verification steps will be skipped"
  log_info  "Install psql (brew install libpq) or start Docker to enable full checks"
fi

log_info "DB_MODE = $DB_MODE"

# =============================================================================
# S0 — INDEX
# =============================================================================
log_header "S0 — Index idx_remnants_bounding_box exists in DB"

if ! db_available; then
  log_skip "[S0] DB not accessible — skipping index check"
else
  log_step "Querying pg_indexes ..."
  INDEX_ROW=$(psql_scalar \
    "SELECT indexdef FROM pg_indexes
     WHERE tablename='remnants' AND indexname='idx_remnants_bounding_box';")

  if [[ -z "$INDEX_ROW" ]]; then
    log_fail "[S0] Index idx_remnants_bounding_box does not exist — run: make migrate-up"
    FAILED_SCENARIOS+=("S0: index missing")
    if [[ "$STOP_ON_FAIL" == true ]]; then print_summary; exit 1; fi
  else
    log_pass "[S0] Index exists"
    log_info  "$INDEX_ROW"

    if echo "$INDEX_ROW" | grep -qi "bounding_box_length_mm" \
        && echo "$INDEX_ROW" | grep -qi "bounding_box_width_mm"; then
      log_pass "[S0] Covers bounding_box_length_mm + bounding_box_width_mm"
    else
      log_fail "[S0] Index does not cover expected columns"
      FAILED_SCENARIOS+=("S0: wrong index columns")
    fi

    if echo "$INDEX_ROW" | grep -qi "status = 'AVAILABLE'"; then
      log_pass "[S0] Partial predicate WHERE status = 'AVAILABLE' present"
    else
      log_fail "[S0] Partial predicate missing"
      FAILED_SCENARIOS+=("S0: missing partial predicate")
    fi
  fi
fi

# =============================================================================
# HEALTHCHECK
# =============================================================================
log_header "Healthcheck"
log_step "GET /healthz ..."
HC_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/healthz")
assert_status "healthz" "200" "$HC_STATUS"

# =============================================================================
# AUTH — obtain Bearer token
# =============================================================================
log_header "Auth — obtain Bearer token"
log_step "POST /api/auth/login (user=$AUTH_USER) ..."
log_info  "Credentials: username=[$AUTH_USER]  password=[$(printf '%s' "$AUTH_PASS" | wc -c | tr -d ' ') chars]"

# Single curl call: body + status code separated by newline
RAW_AUTH=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$AUTH_USER\",\"password\":\"$AUTH_PASS\"}")
AUTH_HTTP=$(echo "$RAW_AUTH" | tail -1)
AUTH_RESP=$(echo "$RAW_AUTH" | head -1)

if [[ "$AUTH_HTTP" != "200" ]]; then
  log_fail "[auth] Login failed (HTTP $AUTH_HTTP) — check --username / --password"
  log_info  "Response: $AUTH_RESP"
  log_info  "Hint: default credentials are  username=admin  password=admin123"
  print_summary; exit 1
fi

# The server returns Token: "Bearer <jwt>" — strip the prefix so we can
# construct the Authorization header ourselves without doubling "Bearer ".
TOKEN_RAW=$(echo "$AUTH_RESP" | jq -r '.token // empty')
TOKEN="${TOKEN_RAW#Bearer }"   # strip leading "Bearer " if present

if [[ -z "$TOKEN" ]]; then
  log_fail "[auth] No token in login response: $AUTH_RESP"
  print_summary; exit 1
fi
log_pass "[auth] Bearer token obtained (${#TOKEN} chars)"

# Convenience wrappers (all API calls go through these)
AUTH_H="Authorization: Bearer $TOKEN"
api()        { curl -s    -H "$AUTH_H" "$@"; }
api_status() { curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_H" "$@"; }
# api_with_status: returns <body>\n<code>  — use tail/head to split
api_with_status() {
  curl -s -w "\n%{http_code}" -H "$AUTH_H" "$@"
}

# =============================================================================
# S1 — SETUP: create lots and retrieve sheet IDs
# =============================================================================
log_header "S1 — Setup: create inventory lots (3 sheets total)"

MATERIAL_ID="00000000-0000-0000-0000-000000000001"
WO_ID="00000000-0000-0000-0000-000000000099"
SKU_ID="00000000-0000-0000-0000-000000000010"

# Lot A — 1 sheet (used for S2 cut chain)
LOT_A_STATUS=$(api_status -X POST "$BASE_URL/api/v1/inventory/lots" \
  -H "Content-Type: application/json" \
  -d "{
    \"material_id\": \"$MATERIAL_ID\",
    \"dimensions\": {\"length_mm\": 2000, \"width_mm\": 1000},
    \"cost_per_sheet\": {\"amount\": 500000, \"currency\": \"VND\"},
    \"quantity\": 1,
    \"supplier_ref\": \"SYS-TEST-2.4-A\"
  }")
assert_status "S1-lot-A" "201" "$LOT_A_STATUS"

# Lot B — 2 sheets (used for S4, S5 validation tests)
LOT_B_STATUS=$(api_status -X POST "$BASE_URL/api/v1/inventory/lots" \
  -H "Content-Type: application/json" \
  -d "{
    \"material_id\": \"$MATERIAL_ID\",
    \"dimensions\": {\"length_mm\": 2000, \"width_mm\": 1000},
    \"cost_per_sheet\": {\"amount\": 500000, \"currency\": \"VND\"},
    \"quantity\": 2,
    \"supplier_ref\": \"SYS-TEST-2.4-B\"
  }")
assert_status "S1-lot-B" "201" "$LOT_B_STATUS"

# Retrieve the 3 most-recently created AVAILABLE sheets
SHEETS_RESP=$(api "$BASE_URL/api/v1/inventory/sheets?limit=3&order=desc")
SHEET_A=$(echo "$SHEETS_RESP" | jq -r '.items[0].id // empty')  # newest (Lot B sheet 2)
SHEET_B=$(echo "$SHEETS_RESP" | jq -r '.items[1].id // empty')  # Lot B sheet 1
SHEET_C=$(echo "$SHEETS_RESP" | jq -r '.items[2].id // empty')  # Lot A sheet

if [[ -z "$SHEET_A" || "$SHEET_A" == "null" \
   || -z "$SHEET_B" || "$SHEET_B" == "null" \
   || -z "$SHEET_C" || "$SHEET_C" == "null" ]]; then
  log_fail "[S1] Could not retrieve 3 sheet IDs from /inventory/sheets — aborting"
  log_info  "Response: $SHEETS_RESP"
  print_summary; exit 1
fi
log_pass "[S1] SHEET_C (Lot A, main cut chain) = $SHEET_C"
log_pass "[S1] SHEET_B (Lot B, for S4)         = $SHEET_B"
log_pass "[S1] SHEET_A (Lot B, for S5)         = $SHEET_A"

# =============================================================================
# S2 — RecordCut: NO bounding_box → default = actual dimension
# =============================================================================
log_header "S2 — RecordCut: no bounding_box → default = actual"

log_step "POST /api/v1/inventory/cuts (no bounding_box fields) ..."
RAW_S2=$(api_with_status -X POST "$BASE_URL/api/v1/inventory/cuts" \
  -H "Content-Type: application/json" \
  -d "{
    \"sheet_id\": \"$SHEET_C\",
    \"work_order_id\": \"$WO_ID\",
    \"sku_id\": \"$SKU_ID\",
    \"used_dimension\": {\"length_mm\": 1000, \"width_mm\": 600},
    \"remnant_dimension\": {\"length_mm\": 800, \"width_mm\": 400}
  }")
S2_STATUS=$(echo "$RAW_S2" | tail -1)
S2_BODY=$(echo "$RAW_S2" | head -1)

assert_status "S2-http" "201" "$S2_STATUS"
assert_json "S2-remnant-id" "remnant_id present in response" \
  '(.remnant_id != null and .remnant_id != "")' "$S2_BODY"

REMNANT_S2=$(echo "$S2_BODY" | jq -r '.remnant_id // empty')

log_step "Verifying bounding_box = actual dimension in DB ..."
if ! db_available; then
  log_skip "[S2] DB check skipped (no DB access)"
elif [[ -z "$REMNANT_S2" ]]; then
  log_skip "[S2] DB check skipped (no remnant_id in response)"
else
  BB_LEN=$(psql_scalar "SELECT bounding_box_length_mm FROM remnants WHERE id='$REMNANT_S2';")
  BB_WID=$(psql_scalar "SELECT bounding_box_width_mm  FROM remnants WHERE id='$REMNANT_S2';")
  ACT_LEN=$(psql_scalar "SELECT length_mm FROM remnants WHERE id='$REMNANT_S2';")
  ACT_WID=$(psql_scalar "SELECT width_mm  FROM remnants WHERE id='$REMNANT_S2';")
  log_info "remnant=$REMNANT_S2  actual=${ACT_LEN}×${ACT_WID}  bounding_box=${BB_LEN}×${BB_WID}"

  if [[ -z "$BB_LEN" ]]; then
    log_fail "[S2] bounding_box_length_mm is NULL — default not applied"
    FAILED_SCENARIOS+=("S2: bounding_box_length_mm is NULL")
  elif [[ "$BB_LEN" == "$ACT_LEN" ]]; then
    log_pass "[S2] bounding_box_length_mm ($BB_LEN) == actual ($ACT_LEN)"
  else
    log_fail "[S2] bounding_box_length_mm=$BB_LEN, expected $ACT_LEN"
    FAILED_SCENARIOS+=("S2: bounding_box_length_mm not defaulted")
  fi

  if [[ -z "$BB_WID" ]]; then
    log_fail "[S2] bounding_box_width_mm is NULL — default not applied"
    FAILED_SCENARIOS+=("S2: bounding_box_width_mm is NULL")
  elif [[ "$BB_WID" == "$ACT_WID" ]]; then
    log_pass "[S2] bounding_box_width_mm ($BB_WID) == actual ($ACT_WID)"
  else
    log_fail "[S2] bounding_box_width_mm=$BB_WID, expected $ACT_WID"
    FAILED_SCENARIOS+=("S2: bounding_box_width_mm not defaulted")
  fi
fi

# =============================================================================
# S3 — RecordCut: explicit bounding_box < actual → stored correctly
# =============================================================================
log_header "S3 — RecordCut: explicit bounding_box (550×280) < actual (600×300)"
# Source: remnant from S2 (actual=800×400)
# Cut: used=200×100 (20k mm²) + remnant=600×300 (180k mm²) → total 200k ≤ 320k ✓
# bb: 550×280

if [[ -z "$REMNANT_S2" ]]; then
  log_skip "[S3] Skipped — no REMNANT_S2 from S2"
else
  log_step "POST /api/v1/inventory/cuts (remnant source, bb=550×280) ..."
  RAW_S3=$(api_with_status -X POST "$BASE_URL/api/v1/inventory/cuts" \
    -H "Content-Type: application/json" \
    -d "{
      \"remnant_id\": \"$REMNANT_S2\",
      \"work_order_id\": \"$WO_ID\",
      \"sku_id\": \"$SKU_ID\",
      \"used_dimension\": {\"length_mm\": 200, \"width_mm\": 100},
      \"remnant_dimension\": {\"length_mm\": 600, \"width_mm\": 300},
      \"bounding_box_length_mm\": 550,
      \"bounding_box_width_mm\": 280
    }")
  S3_STATUS=$(echo "$RAW_S3" | tail -1)
  S3_BODY=$(echo "$RAW_S3"   | head -1)

  assert_status "S3-http" "201" "$S3_STATUS"
  assert_json "S3-remnant-id" "remnant_id present" \
    '(.remnant_id != null and .remnant_id != "")' "$S3_BODY"

  REMNANT_S3=$(echo "$S3_BODY" | jq -r '.remnant_id // empty')

  if ! db_available; then
    log_skip "[S3] DB check skipped (no DB access)"
  elif [[ -z "$REMNANT_S3" ]]; then
    log_skip "[S3] DB check skipped (no remnant_id)"
  else
    BB3_LEN=$(psql_scalar "SELECT bounding_box_length_mm FROM remnants WHERE id='$REMNANT_S3';")
    BB3_WID=$(psql_scalar "SELECT bounding_box_width_mm  FROM remnants WHERE id='$REMNANT_S3';")
    ACT3_LEN=$(psql_scalar "SELECT length_mm FROM remnants WHERE id='$REMNANT_S3';")
    ACT3_WID=$(psql_scalar "SELECT width_mm  FROM remnants WHERE id='$REMNANT_S3';")
    log_info "remnant=$REMNANT_S3  actual=${ACT3_LEN}×${ACT3_WID}  bounding_box=${BB3_LEN}×${BB3_WID}"

    if [[ "$BB3_LEN" == "550" ]]; then
      log_pass "[S3] bounding_box_length_mm = $BB3_LEN (expected 550)"
    else
      log_fail "[S3] bounding_box_length_mm = $BB3_LEN, expected 550"
      FAILED_SCENARIOS+=("S3: bounding_box_length_mm stored wrong")
    fi

    if [[ "$BB3_WID" == "280" ]]; then
      log_pass "[S3] bounding_box_width_mm = $BB3_WID (expected 280)"
    else
      log_fail "[S3] bounding_box_width_mm = $BB3_WID, expected 280"
      FAILED_SCENARIOS+=("S3: bounding_box_width_mm stored wrong")
    fi

    # Guard: explicit value must NOT have been overwritten by the default
    if [[ "$BB3_LEN" == "$ACT3_LEN" && "$ACT3_LEN" != "550" ]]; then
      log_fail "[S3] bounding_box was overwritten by default (equals actual ${ACT3_LEN}) — explicit value lost"
      FAILED_SCENARIOS+=("S3: explicit bounding_box overwritten by default")
    fi
  fi
fi

# =============================================================================
# S4 — bounding_box > actual → HTTP 400
# =============================================================================
log_header "S4 — bounding_box EXCEEDS actual dimension → HTTP 400"
log_step "POST /api/v1/inventory/cuts (bb_length=801 > remnant actual 800) ..."

RAW_S4=$(api_with_status -X POST "$BASE_URL/api/v1/inventory/cuts" \
  -H "Content-Type: application/json" \
  -d "{
    \"sheet_id\": \"$SHEET_B\",
    \"work_order_id\": \"$WO_ID\",
    \"sku_id\": \"$SKU_ID\",
    \"used_dimension\": {\"length_mm\": 1000, \"width_mm\": 500},
    \"remnant_dimension\": {\"length_mm\": 800, \"width_mm\": 400},
    \"bounding_box_length_mm\": 801,
    \"bounding_box_width_mm\": 400
  }")
S4_STATUS=$(echo "$RAW_S4" | tail -1)
S4_BODY=$(echo "$RAW_S4"   | head -1)

assert_status "S4-http" "400" "$S4_STATUS"
assert_json "S4-error-message" \
  'error body contains "usable dimension cannot exceed actual dimension"' \
  '(.error | ascii_downcase | contains("usable dimension"))' \
  "$S4_BODY"

if ! db_available; then
  log_skip "[S4] DB rollback check skipped (no DB access)"
else
  S4_SHEET_ST=$(psql_scalar "SELECT status FROM board_sheets WHERE id='$SHEET_B';")
  if [[ "$S4_SHEET_ST" == "AVAILABLE" ]]; then
    log_pass "[S4] Sheet status remains AVAILABLE — no DB write on rejected request"
  else
    log_fail "[S4] Sheet status = '$S4_SHEET_ST' — store was incorrectly called"
    FAILED_SCENARIOS+=("S4: sheet consumed despite validation failure")
  fi
fi

# =============================================================================
# S5 — partial bounding_box (one axis only) → HTTP 400
# =============================================================================
log_header "S5 — ONE bounding_box axis missing → HTTP 400"
log_step "POST /api/v1/inventory/cuts (only bounding_box_length_mm, no width) ..."

RAW_S5=$(api_with_status -X POST "$BASE_URL/api/v1/inventory/cuts" \
  -H "Content-Type: application/json" \
  -d "{
    \"sheet_id\": \"$SHEET_A\",
    \"work_order_id\": \"$WO_ID\",
    \"sku_id\": \"$SKU_ID\",
    \"used_dimension\": {\"length_mm\": 1000, \"width_mm\": 500},
    \"remnant_dimension\": {\"length_mm\": 800, \"width_mm\": 400},
    \"bounding_box_length_mm\": 700
  }")
S5_STATUS=$(echo "$RAW_S5" | tail -1)
S5_BODY=$(echo "$RAW_S5"   | head -1)

assert_status "S5-http" "400" "$S5_STATUS"
assert_json "S5-error-message" \
  'error body contains "must be provided together"' \
  '(.error | ascii_downcase | contains("together"))' \
  "$S5_BODY"

# =============================================================================
# S6 — FindAvailableRemnants: bounding_box < min → EXCLUDED
# =============================================================================
log_header "S6 — FindAvailableRemnants: bounding_box (550) < min_length (560) → excluded"

if [[ -z "${REMNANT_S3:-}" || "$REMNANT_S3" == "null" ]]; then
  log_skip "[S6] No REMNANT_S3 to test against — skipping"
else
  log_step "GET /remnants?min_length_mm=560&min_width_mm=270 ..."
  S6_RESP=$(api "$BASE_URL/api/v1/inventory/remnants?min_length_mm=560&min_width_mm=270")
  S6_MATCH=$(echo "$S6_RESP" | jq -r \
    --arg id "$REMNANT_S3" '[.[] | select(.id == $id)] | length')
  if [[ "$S6_MATCH" == "0" ]]; then
    log_pass "[S6] Remnant S3 (bb=550×280) correctly excluded for min_length=560"
  else
    log_fail "[S6] Remnant S3 found in results — filter uses length_mm instead of bounding_box_length_mm"
    FAILED_SCENARIOS+=("S6: remnant with insufficient bounding_box incorrectly included")
  fi
fi

# =============================================================================
# S7 — FindAvailableRemnants: bounding_box >= min → INCLUDED + fields serialised
# =============================================================================
log_header "S7 — FindAvailableRemnants: bounding_box (550) >= min_length (500) → included"

if [[ -z "${REMNANT_S3:-}" || "$REMNANT_S3" == "null" ]]; then
  log_skip "[S7] No REMNANT_S3 to test against — skipping"
else
  log_step "GET /remnants?min_length_mm=500&min_width_mm=250 ..."
  S7_RESP=$(api "$BASE_URL/api/v1/inventory/remnants?min_length_mm=500&min_width_mm=250")
  S7_MATCH=$(echo "$S7_RESP" | jq -r \
    --arg id "$REMNANT_S3" '[.[] | select(.id == $id)] | length')

  if [[ "$S7_MATCH" == "0" ]]; then
    log_fail "[S7] Remnant S3 not found — filter incorrectly excluded it"
    FAILED_SCENARIOS+=("S7: remnant with sufficient bounding_box excluded")
  else
    log_pass "[S7] Remnant S3 (bb=550×280) present for min=500×250"

    # Verify bounding_box fields are serialised correctly in the JSON response
    S7_BB_LEN=$(echo "$S7_RESP" | jq -r \
      --arg id "$REMNANT_S3" '.[] | select(.id == $id) | .bounding_box_length_mm')
    S7_BB_WID=$(echo "$S7_RESP" | jq -r \
      --arg id "$REMNANT_S3" '.[] | select(.id == $id) | .bounding_box_width_mm')

    if [[ "$S7_BB_LEN" == "550" ]]; then
      log_pass "[S7] bounding_box_length_mm serialised correctly (550)"
    else
      log_fail "[S7] bounding_box_length_mm in response = '$S7_BB_LEN', expected 550"
      FAILED_SCENARIOS+=("S7: bounding_box_length_mm not in response")
    fi
    if [[ "$S7_BB_WID" == "280" ]]; then
      log_pass "[S7] bounding_box_width_mm serialised correctly (280)"
    else
      log_fail "[S7] bounding_box_width_mm in response = '$S7_BB_WID', expected 280"
      FAILED_SCENARIOS+=("S7: bounding_box_width_mm not in response")
    fi
  fi
fi

# =============================================================================
# S8 — ORDER BY bounding_box area ASC (Best Fit)
# =============================================================================
log_header "S8 — FindAvailableRemnants: ORDER BY bounding_box area ASC (Best Fit)"
# After the test flow:
#   REMNANT_S3: actual=600×300, bb=550×280  → bb_area=154,000 mm²  (AVAILABLE)
#   The S2-default remnant (from SHEET_C initial cut): bb=800×400   → bb_area=320,000 mm²
#   (REMNANT_S2 was consumed by the S3 cut — it is CONSUMED)
#
# We also need the remnant created when S2 was re-cut.
# For S8 we use two AVAILABLE remnants that should both match min=400×200:
#   - REMNANT_S3  (bb=550×280  = 154k mm²)  ← smaller → must appear FIRST
#   - A remnant from Lot B sheet that defaulted to bb=800×400 = 320k mm²
#
# The Lot B sheets (SHEET_A, SHEET_B) were targeted by S4/S5 which both failed
# validation → both sheets are still AVAILABLE, no remnant was created.
# So we create one additional cut on SHEET_A to produce a remnant with bb=800×400.

if [[ -z "${REMNANT_S3:-}" || "$REMNANT_S3" == "null" ]]; then
  log_skip "[S8] No REMNANT_S3 available — skipping Best Fit order test"
else
  log_step "Creating a large remnant (bb=800×400, default) for comparison ..."
  RAW_S8_SETUP=$(api_with_status -X POST "$BASE_URL/api/v1/inventory/cuts" \
    -H "Content-Type: application/json" \
    -d "{
      \"sheet_id\": \"$SHEET_A\",
      \"work_order_id\": \"$WO_ID\",
      \"sku_id\": \"$SKU_ID\",
      \"used_dimension\": {\"length_mm\": 1000, \"width_mm\": 600},
      \"remnant_dimension\": {\"length_mm\": 800, \"width_mm\": 400}
    }")
  S8_SETUP_STATUS=$(echo "$RAW_S8_SETUP" | tail -1)
  S8_SETUP_BODY=$(echo "$RAW_S8_SETUP"   | head -1)
  REMNANT_LARGE=$(echo "$S8_SETUP_BODY"  | jq -r '.remnant_id // empty')

  if [[ "$S8_SETUP_STATUS" != "201" || -z "$REMNANT_LARGE" ]]; then
    log_skip "[S8] Could not create large remnant (HTTP $S8_SETUP_STATUS) — skipping"
  else
    log_info "Large remnant (bb=800×400=320k mm²) = $REMNANT_LARGE"

    log_step "GET /remnants?min_length_mm=400&min_width_mm=200 (both remnants should match) ..."
    S8_RESP=$(api "$BASE_URL/api/v1/inventory/remnants?min_length_mm=400&min_width_mm=200")

    # Extract ordered list of ids
    S8_IDS=$(echo "$S8_RESP" | jq -r '.[].id')
    S8_POS_S3=$(echo "$S8_IDS" | grep -n "$REMNANT_S3"    | cut -d: -f1 || echo "")
    S8_POS_LG=$(echo "$S8_IDS" | grep -n "$REMNANT_LARGE" | cut -d: -f1 || echo "")

    log_info "Result positions — S3 (154k mm²): #${S8_POS_S3:-not found}  |  large (320k mm²): #${S8_POS_LG:-not found}"

    if [[ -z "$S8_POS_S3" || -z "$S8_POS_LG" ]]; then
      log_skip "[S8] One or both remnants missing from result — cannot verify order"
    elif (( S8_POS_S3 < S8_POS_LG )); then
      log_pass "[S8] Smaller bounding_box area (S3 @#$S8_POS_S3) before larger (@#$S8_POS_LG) — Best Fit ASC ✓"
    else
      log_fail "[S8] Order wrong: S3 @#$S8_POS_S3, large @#$S8_POS_LG — ORDER BY not using bounding_box area"
      FAILED_SCENARIOS+=("S8: Best Fit order not ASC by bounding_box area")
    fi
  fi
fi

# =============================================================================
# FINAL SUMMARY
# =============================================================================
print_summary

[[ $FAIL -eq 0 ]]   # exit 0 on all pass, exit 1 on any failure
