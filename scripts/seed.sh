#!/usr/bin/env bash
# scripts/seed.sh — Demo data seed for Vmarble Warehouse Management Service
#
# Populates a fresh database with a complete end-to-end workflow:
#   Materials → SKUs → BOM → Purchase Order → Production Plan → Work Orders
#   → Board Sheet stock → CNC Cutting → Processing → Costing → Barcode scans
#
# Requires: curl, jq, a running server (default: http://localhost:8080)
# Usage:    VMARBLE_BASE_URL=http://localhost:8080 bash scripts/seed.sh

set -euo pipefail

BASE="${VMARBLE_BASE_URL:-http://localhost:8080}"

# ── Colours ──────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

step()  { echo -e "\n${BLUE}${BOLD}▶ $1${NC}"; }
ok()    { echo -e "${GREEN}  ✓ $1${NC}"; }
info()  { echo -e "${YELLOW}  ℹ $1${NC}"; }
fail()  { echo -e "${RED}  ✗ $1${NC}" >&2; exit 1; }

# ── Helpers ───────────────────────────────────────────────────────────────────

# api_post <token> <path> <json-body> — returns raw response body
api_post() {
    local token="$1" path="$2" body="$3"
    local resp http_code
    resp=$(curl -s -w "\n%{http_code}" -X POST "${BASE}${path}" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${token}" \
        -d "${body}")
    http_code=$(echo "${resp}" | tail -1)
    body_out=$(echo "${resp}" | sed '$d')
    if [[ "${http_code}" -lt 200 || "${http_code}" -ge 300 ]]; then
        echo -e "${RED}  HTTP ${http_code} on POST ${path}${NC}" >&2
        echo -e "${RED}  Response: ${body_out}${NC}" >&2
        exit 1
    fi
    echo "${body_out}"
}

# login <username> <password> — returns token string
login() {
    local user="$1" pass="$2"
    local resp http_code body_out
    resp=$(curl -s -w "\n%{http_code}" -X POST "${BASE}/api/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"${user}\",\"password\":\"${pass}\"}")
    http_code=$(echo "${resp}" | tail -1)
    body_out=$(echo "${resp}" | sed '$d')
    if [[ "${http_code}" -lt 200 || "${http_code}" -ge 300 ]]; then
        fail "Login as '${user}' failed (HTTP ${http_code}): ${body_out}"
    fi
    echo "${body_out}" | jq -r '.token'
}

# future_date <days> — portable date +N days (works on macOS and Linux)
future_date() {
    local days="$1"
    if date -v +"${days}"d +"%Y-%m-%dT00:00:00Z" 2>/dev/null; then
        return
    fi
    date -d "+${days} days" +"%Y-%m-%dT00:00:00Z"
}

# ── Pre-flight ────────────────────────────────────────────────────────────────
step "Checking dependencies"
command -v curl >/dev/null || fail "curl is required but not found"
command -v jq   >/dev/null || fail "jq is required but not found"
ok "curl and jq available"

step "Waiting for server at ${BASE}"
for i in $(seq 1 30); do
    if curl -sf "${BASE}/healthz" >/dev/null 2>&1; then
        ok "Server is up"
        break
    fi
    if [[ "$i" -eq 30 ]]; then
        fail "Server not reachable at ${BASE} after 30 attempts. Is it running?"
    fi
    echo "  Attempt ${i}/30 — retrying in 2s..."
    sleep 2
done

# ── Authentication ────────────────────────────────────────────────────────────
step "Authenticating"
TOKEN_ADMIN=$(login "admin" "admin123")
ok "admin token obtained"

TOKEN_PLANNER=$(login "planner" "worker123")
ok "planner token obtained"

TOKEN_WAREHOUSE=$(login "warehouse" "worker123")
ok "warehouse token obtained"

TOKEN_ACCOUNTANT=$(login "accountant" "acc123")
ok "accountant token obtained"

TOKEN_FOREMAN=$(login "foreman" "fore123")
ok "foreman token obtained"

TOKEN_CNC=$(login "worker" "worker123")
ok "cnc (worker) token obtained"

# Create cnc_manager user — migrations don't seed one; ignore conflict if already exists
step "Creating cnc_manager demo user (cncmgr / cncmgr123)"
curl -s -X POST "${BASE}/api/v1/admin/users" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${TOKEN_ADMIN}" \
    -d '{"username":"cncmgr","password":"cncmgr123","role":"cnc_manager","full_name":"CNC Manager Demo"}' \
    >/dev/null 2>&1 || true   # 409 conflict = user already exists, that is fine
TOKEN_CNCMGR=$(login "cncmgr" "cncmgr123")
ok "cnc_manager token obtained"

# ── CATALOG: materials ────────────────────────────────────────────────────────
step "Creating materials"

MAT_PLYWOOD=$(api_post "${TOKEN_ADMIN}" "/api/v1/materials" \
    '{"type":"PLYWOOD","name":"Plywood 18mm","unit":"sheet"}' | jq -r '.id')
ok "PLYWOOD material: ${MAT_PLYWOOD}"

MAT_METAL=$(api_post "${TOKEN_ADMIN}" "/api/v1/materials" \
    '{"type":"METAL","name":"Metal Bracket","unit":"pcs"}' | jq -r '.id')
ok "METAL material: ${MAT_METAL}"

MAT_GLUE=$(api_post "${TOKEN_ADMIN}" "/api/v1/materials" \
    '{"type":"GLUE","name":"PVA Glue","unit":"kg"}' | jq -r '.id')
ok "GLUE material: ${MAT_GLUE}"

# ── CATALOG: SKUs ─────────────────────────────────────────────────────────────
step "Creating SKUs"

SKU_A=$(api_post "${TOKEN_ADMIN}" "/api/v1/skus" \
    '{"code":"CAB-DOOR-600","name":"Cabinet Door 600x400","dimensions":{"length_mm":600,"width_mm":400},"requires_metal":false}' \
    | jq -r '.id')
ok "SKU A — Cabinet Door 600x400 (no metal): ${SKU_A}"

SKU_B=$(api_post "${TOKEN_ADMIN}" "/api/v1/skus" \
    '{"code":"SHELF-PANEL-800","name":"Shelf Panel 800x300","dimensions":{"length_mm":800,"width_mm":300},"requires_metal":true}' \
    | jq -r '.id')
ok "SKU B — Shelf Panel 800x300 (requires_metal=true): ${SKU_B}"

# ── CATALOG: BOMs ─────────────────────────────────────────────────────────────
step "Setting Bills of Materials"

curl -sf -X PUT "${BASE}/api/v1/skus/${SKU_A}/bom" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${TOKEN_ADMIN}" \
    -d "{
      \"components\": [
        {\"material_id\":\"${MAT_PLYWOOD}\",\"material_type\":\"PLYWOOD\",\"quantity_per_unit\":0.25,\"unit\":\"sheet\"},
        {\"material_id\":\"${MAT_GLUE}\",\"material_type\":\"GLUE\",\"quantity_per_unit\":0.1,\"unit\":\"kg\"}
      ]
    }" >/dev/null
ok "BOM set for SKU A (plywood + glue)"

curl -sf -X PUT "${BASE}/api/v1/skus/${SKU_B}/bom" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer ${TOKEN_ADMIN}" \
    -d "{
      \"components\": [
        {\"material_id\":\"${MAT_PLYWOOD}\",\"material_type\":\"PLYWOOD\",\"quantity_per_unit\":0.3,\"unit\":\"sheet\"},
        {\"material_id\":\"${MAT_METAL}\",\"material_type\":\"METAL\",\"quantity_per_unit\":2.0,\"unit\":\"pcs\"}
      ]
    }" >/dev/null
ok "BOM set for SKU B (plywood + metal brackets)"

# ── ORDER: Purchase Order ─────────────────────────────────────────────────────
step "Creating Purchase Order PO-2026-DEMO"

PO_ID=$(api_post "${TOKEN_ACCOUNTANT}" "/api/v1/pos" \
    "{
      \"code\": \"PO-2026-DEMO\",
      \"expected_delivery\": \"$(future_date 30)\",
      \"line_items\": [
        {\"sku_id\":\"${SKU_A}\",\"quantity\":20,\"selling_price\":{\"amount\":1200000,\"currency\":\"VND\"}},
        {\"sku_id\":\"${SKU_B}\",\"quantity\":15,\"selling_price\":{\"amount\":980000,\"currency\":\"VND\"}}
      ]
    }" | jq -r '.id')
ok "PO created: ${PO_ID}"

# ── PLANNING: plan + approve ──────────────────────────────────────────────────
step "Creating and approving production plan"

PLAN_ID=$(api_post "${TOKEN_PLANNER}" "/api/v1/plans" \
    "{
      \"po_id\": \"${PO_ID}\",
      \"items\": [
        {\"sku_id\":\"${SKU_A}\",\"quantity\":20},
        {\"sku_id\":\"${SKU_B}\",\"quantity\":15}
      ],
      \"deadline\": \"$(future_date 25)\"
    }" | jq -r '.id')
ok "Plan created: ${PLAN_ID} (DRAFT)"

curl -sf -X POST "${BASE}/api/v1/plans/${PLAN_ID}/approve" \
    -H "Authorization: Bearer ${TOKEN_PLANNER}" >/dev/null
ok "Plan approved → APPROVED"

# ── PRODUCTION: CNC machine + shift slot ─────────────────────────────────────
step "Creating CNC machine and morning shift slot"

MACHINE_ID=$(api_post "${TOKEN_ADMIN}" "/api/v1/machines" \
    '{"code":"CNC-01","name":"CNC Machine #1","capacity_hours_per_shift":8.0}' \
    | jq -r '.id')
ok "Machine CNC-01 created: ${MACHINE_ID}"

SLOT_ID=$(api_post "${TOKEN_CNCMGR}" "/api/v1/machines/${MACHINE_ID}/slots" \
    "{\"shift_date\":\"$(future_date 7)\",\"shift_name\":\"morning\",\"capacity_hours\":8.0}" \
    | jq -r '.id')
ok "Shift slot (morning, +7 days): ${SLOT_ID}"

# ── PRODUCTION: Work Orders ───────────────────────────────────────────────────
step "Creating Work Orders"

WO_A=$(api_post "${TOKEN_PLANNER}" "/api/v1/work-orders" \
    "{\"plan_id\":\"${PLAN_ID}\",\"sku_id\":\"${SKU_A}\",\"quantity\":10}" \
    | jq -r '.id')
ok "WO A (Cabinet Door, qty 10): ${WO_A}"

WO_B=$(api_post "${TOKEN_PLANNER}" "/api/v1/work-orders" \
    "{\"plan_id\":\"${PLAN_ID}\",\"sku_id\":\"${SKU_B}\",\"quantity\":8}" \
    | jq -r '.id')
ok "WO B (Shelf Panel, qty 8, requires_metal): ${WO_B}"

# ── PRODUCTION: capacity scheduling ──────────────────────────────────────────
step "Scheduling WO A on machine slot (capacity-aware)"

api_post "${TOKEN_CNCMGR}" "/api/v1/work-orders/${WO_A}/estimated-hours" \
    '{"estimated_hours":3.5}' >/dev/null
ok "Estimated hours set: 3.5h"

api_post "${TOKEN_CNCMGR}" "/api/v1/work-orders/${WO_A}/assign-slot" \
    "{\"slot_id\":\"${SLOT_ID}\"}" >/dev/null
ok "Slot assigned → remaining capacity: 4.5h"

# ── INVENTORY: receive board sheets ──────────────────────────────────────────
step "Receiving plywood inventory (2 lots)"

api_post "${TOKEN_WAREHOUSE}" "/api/v1/inventory/lots" \
    "{
      \"material_id\": \"${MAT_PLYWOOD}\",
      \"dimensions\": {\"length_mm\":2440,\"width_mm\":1220},
      \"cost_per_sheet\": {\"amount\":280000,\"currency\":\"VND\"},
      \"quantity\": 10,
      \"supplier_ref\": \"SUP-BATCH-001\"
    }" >/dev/null
ok "Lot 1: 10 × 2440×1220 sheets @ 280,000 VND"

api_post "${TOKEN_WAREHOUSE}" "/api/v1/inventory/lots" \
    "{
      \"material_id\": \"${MAT_PLYWOOD}\",
      \"dimensions\": {\"length_mm\":2440,\"width_mm\":1220},
      \"cost_per_sheet\": {\"amount\":295000,\"currency\":\"VND\"},
      \"quantity\": 5,
      \"supplier_ref\": \"SUP-BATCH-002\"
    }" >/dev/null
ok "Lot 2: 5 × 2440×1220 sheets @ 295,000 VND"

SHEETS=$(curl -sf "${BASE}/api/v1/inventory/sheets?limit=2" \
    -H "Authorization: Bearer ${TOKEN_WAREHOUSE}")
SHEET_ID=$(echo "${SHEETS}" | jq -r '.items[0].id')
SHEET_ID2=$(echo "${SHEETS}" | jq -r '.items[1].id')
ok "Sheet 1: ${SHEET_ID}"
ok "Sheet 2: ${SHEET_ID2}"

# ── WO A: full lifecycle (no metal requirement) ───────────────────────────────
step "Work Order A — full lifecycle  [PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED]"

api_post "${TOKEN_CNC}" "/api/v1/work-orders/${WO_A}/advance" \
    "{\"status\":\"IN_CUTTING\",\"sheet_id\":\"${SHEET_ID}\"}" >/dev/null
ok "→ IN_CUTTING (sheet pre-assigned)"

CUT_RESP_A=$(api_post "${TOKEN_CNC}" "/api/v1/inventory/cuts" \
    "{
      \"sheet_id\": \"${SHEET_ID}\",
      \"work_order_id\": \"${WO_A}\",
      \"sku_id\": \"${SKU_A}\",
      \"used_dimension\": {\"length_mm\":600,\"width_mm\":400},
      \"remnant_dimension\": {\"length_mm\":1840,\"width_mm\":1220},
      \"bounding_box_length_mm\": 1840,
      \"bounding_box_width_mm\": 1220,
      \"shape_type\": \"rectangle\"
    }")
BARCODE_A=$(echo "${CUT_RESP_A}" | jq -r '.barcode_ids[0] // empty')
ok "Cut recorded — used 600×400, remnant 1840×1220 (BR-K03 satisfied)"

api_post "${TOKEN_FOREMAN}" "/api/v1/work-orders/${WO_A}/consumptions" \
    "{\"material_id\":\"${MAT_GLUE}\",\"material_type\":\"GLUE\",\"quantity\":0.5,\"unit\":\"kg\"}" >/dev/null
ok "Glue consumption recorded"

api_post "${TOKEN_CNC}" "/api/v1/work-orders/${WO_A}/advance" \
    '{"status":"IN_PROCESSING"}' >/dev/null
ok "→ IN_PROCESSING"

api_post "${TOKEN_CNC}" "/api/v1/work-orders/${WO_A}/advance" \
    '{"status":"COMPLETED"}' >/dev/null
ok "→ COMPLETED"

api_post "${TOKEN_ACCOUNTANT}" "/api/v1/costing/${WO_A}/compute" '{}' >/dev/null
ok "Cost computed (area-based, BR-C01)"

api_post "${TOKEN_ACCOUNTANT}" "/api/v1/costing/${WO_A}/finalize" '{}' >/dev/null
ok "→ COSTED (finalized, immutable — BR-C04)"

# ── WO B: full lifecycle (requires_metal=true) ────────────────────────────────
step "Work Order B — full lifecycle  [requires METAL consumption — BR-P04]"

api_post "${TOKEN_CNC}" "/api/v1/work-orders/${WO_B}/advance" \
    "{\"status\":\"IN_CUTTING\",\"sheet_id\":\"${SHEET_ID2}\"}" >/dev/null
ok "→ IN_CUTTING"

api_post "${TOKEN_CNC}" "/api/v1/inventory/cuts" \
    "{
      \"sheet_id\": \"${SHEET_ID2}\",
      \"work_order_id\": \"${WO_B}\",
      \"sku_id\": \"${SKU_B}\",
      \"used_dimension\": {\"length_mm\":800,\"width_mm\":300},
      \"remnant_dimension\": {\"length_mm\":1640,\"width_mm\":1220},
      \"bounding_box_length_mm\": 1640,
      \"bounding_box_width_mm\": 1220,
      \"shape_type\": \"rectangle\"
    }" >/dev/null
ok "Cut recorded — used 800×300, remnant 1640×1220"

api_post "${TOKEN_FOREMAN}" "/api/v1/work-orders/${WO_B}/consumptions" \
    "{\"material_id\":\"${MAT_METAL}\",\"material_type\":\"METAL\",\"quantity\":2.0,\"unit\":\"pcs\"}" >/dev/null
ok "METAL consumption recorded (BR-P04 — required for COMPLETED)"

api_post "${TOKEN_CNC}" "/api/v1/work-orders/${WO_B}/advance" \
    '{"status":"IN_PROCESSING"}' >/dev/null
ok "→ IN_PROCESSING"

api_post "${TOKEN_CNC}" "/api/v1/work-orders/${WO_B}/advance" \
    '{"status":"COMPLETED"}' >/dev/null
ok "→ COMPLETED"

api_post "${TOKEN_ACCOUNTANT}" "/api/v1/costing/${WO_B}/compute" '{}' >/dev/null
ok "Cost computed"

api_post "${TOKEN_ACCOUNTANT}" "/api/v1/costing/${WO_B}/finalize" '{}' >/dev/null
ok "→ COSTED"

# ── BARCODE SCANS ─────────────────────────────────────────────────────────────
if [[ -n "${BARCODE_A:-}" ]]; then
    step "Barcode scan checkpoints for WO A  [CNC_COMPLETE → FINISHED_GOODS → SHIPPED]"

    api_post "${TOKEN_CNC}" "/api/v1/barcodes/${BARCODE_A}/scans" \
        '{"checkpoint":"CNC_COMPLETE","location":"CNC Bay 1","shift":"morning"}' >/dev/null
    ok "Scan 1: CNC_COMPLETE"

    api_post "${TOKEN_FOREMAN}" "/api/v1/barcodes/${BARCODE_A}/scans" \
        '{"checkpoint":"FINISHED_GOODS","location":"FG Zone A","shift":"morning"}' >/dev/null
    ok "Scan 2: FINISHED_GOODS"

    api_post "${TOKEN_FOREMAN}" "/api/v1/barcodes/${BARCODE_A}/scans" \
        '{"checkpoint":"SHIPPED","location":"Loading Bay","shift":"afternoon","note":"Delivered to customer"}' >/dev/null
    ok "Scan 3: SHIPPED"
else
    info "No barcode ID from cut response — skipping scan checkpoints"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}${BOLD}  Seed complete — demo data is ready!${NC}"
echo -e "${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "  ${BOLD}Endpoints${NC}"
echo "    Swagger UI  : ${BASE}/swagger/index.html"
echo "    Health      : ${BASE}/healthz"
echo ""
echo -e "  ${BOLD}Demo credentials${NC}"
echo "    admin       / admin123    (role: admin)"
echo "    planner     / worker123   (role: planner)"
echo "    warehouse   / worker123   (role: warehouse)"
echo "    worker      / worker123   (role: cnc)"
echo "    accountant  / acc123      (role: accountant)"
echo "    foreman     / fore123     (role: foreman)"
echo "    cncmgr      / cncmgr123   (role: cnc_manager)"
echo ""
echo -e "  ${BOLD}Seeded resources${NC}"
echo "    PO          : ${PO_ID}"
echo "    Plan        : ${PLAN_ID}  [APPROVED]"
echo "    WO A        : ${WO_A}  [COSTED — Cabinet Door, no metal]"
echo "    WO B        : ${WO_B}  [COSTED — Shelf Panel, BR-P04 metal]"
echo "    Machine     : ${MACHINE_ID}  [CNC-01, 8h/shift]"
echo "    Slot        : ${SLOT_ID}  [morning, +7 days]"
echo ""
