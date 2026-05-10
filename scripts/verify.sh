#!/usr/bin/env bash
# verify.sh — End-to-end smoke test for the VPN Platform control plane.
# Usage: ./scripts/verify.sh [BASE_URL]
# Default BASE_URL: http://localhost:8080

set -euo pipefail

BASE="${1:-http://localhost:8080}"
PASS=0
FAIL=0
EMAIL="verify-$(date +%s)@test.local"
PASSWORD="TestPass123!"

# ── Colours ───────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

ok()   { echo -e "  ${GREEN}✔${NC}  $*"; PASS=$((PASS+1)); }
fail() { echo -e "  ${RED}✗${NC}  $*"; FAIL=$((FAIL+1)); }
hdr()  { echo -e "\n${CYAN}${BOLD}▶ $*${NC}"; }

assert_status() {
  local label="$1" expected="$2" actual="$3" body="$4"
  if [[ "$actual" == "$expected" ]]; then
    ok "$label → HTTP $actual"
  else
    fail "$label → expected HTTP $expected, got $actual"
    echo -e "     ${YELLOW}Body: $body${NC}"
  fi
}

assert_field() {
  local label="$1" field="$2" value="$3"
  if [[ "$value" != "null" && -n "$value" ]]; then
    ok "$label: $field = $value"
  else
    fail "$label: field '$field' missing or null"
  fi
}

# ── Helpers ───────────────────────────────────────────────────────────────────
api() {
  # api <method> <path> [body]
  local method="$1" path="$2" body="${3:-}"
  local args=(-s -w "\n%{http_code}" -X "$method" "$BASE$path" -H "Content-Type: application/json")
  [[ -n "$ACCESS_TOKEN" ]] && args+=(-H "Authorization: Bearer $ACCESS_TOKEN")
  [[ -n "$body" ]] && args+=(-d "$body")
  curl "${args[@]}"
}

split_response() {
  # splits "body\nSTATUS_CODE" into RESP_BODY and RESP_STATUS
  RESP_STATUS=$(printf '%s' "$1" | tail -1)
  RESP_BODY=$(printf '%s' "$1" | sed '$d')
}

ACCESS_TOKEN=""

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}   VPN Platform — End-to-End Verification${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "   Target: ${CYAN}$BASE${NC}"
echo ""

# ─────────────────────────────────────────────────────────────────────────────
hdr "1. Health Check"
resp=$(api GET /healthz)
split_response "$resp"
assert_status "GET /healthz" 200 "$RESP_STATUS" "$RESP_BODY"
status=$(echo "$RESP_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
assert_field "healthz" "status" "$status"

# ─────────────────────────────────────────────────────────────────────────────
hdr "2. Prometheus Metrics Endpoint"
resp=$(api GET /metrics)
split_response "$resp"
assert_status "GET /metrics" 200 "$RESP_STATUS" ""
if echo "$RESP_BODY" | grep -q "go_goroutines"; then
  ok "Prometheus metrics: go_goroutines present"
else
  fail "Prometheus metrics: go_goroutines not found"
fi
if echo "$RESP_BODY" | grep -q "vpnplatform_http"; then
  ok "Prometheus metrics: vpnplatform_http metrics present"
else
  fail "Prometheus metrics: vpnplatform_http metrics not found"
fi

# ─────────────────────────────────────────────────────────────────────────────
hdr "3. Auth — Register"
resp=$(api POST /api/v1/auth/register "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
split_response "$resp"
assert_status "POST /api/v1/auth/register" 201 "$RESP_STATUS" "$RESP_BODY"
ACCESS_TOKEN=$(echo "$RESP_BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['tokens']['access_token'])" 2>/dev/null || echo "")
REFRESH_TOKEN=$(echo "$RESP_BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['tokens']['refresh_token'])" 2>/dev/null || echo "")
USER_ID=$(echo "$RESP_BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['user']['id'])" 2>/dev/null || echo "")
assert_field "register" "access_token" "$ACCESS_TOKEN"
assert_field "register" "user.id" "$USER_ID"

# ─────────────────────────────────────────────────────────────────────────────
hdr "4. Auth — Duplicate Registration (expect 409)"
resp=$(api POST /api/v1/auth/register "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
split_response "$resp"
assert_status "POST /api/v1/auth/register (duplicate)" 409 "$RESP_STATUS" "$RESP_BODY"

# ─────────────────────────────────────────────────────────────────────────────
hdr "5. Auth — Login"
ACCESS_TOKEN=""
resp=$(api POST /api/v1/auth/login "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
split_response "$resp"
assert_status "POST /api/v1/auth/login" 200 "$RESP_STATUS" "$RESP_BODY"
ACCESS_TOKEN=$(echo "$RESP_BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['tokens']['access_token'])" 2>/dev/null || echo "")
assert_field "login" "access_token" "$ACCESS_TOKEN"

# ─────────────────────────────────────────────────────────────────────────────
hdr "6. Auth — Wrong Password (expect 401)"
resp=$(api POST /api/v1/auth/login "{\"email\":\"$EMAIL\",\"password\":\"WrongPassword!\"}")
split_response "$resp"
assert_status "POST /api/v1/auth/login (bad creds)" 401 "$RESP_STATUS" "$RESP_BODY"

# ─────────────────────────────────────────────────────────────────────────────
hdr "7. Auth — Refresh Token"
resp=$(api POST /api/v1/auth/refresh "{\"refresh_token\":\"$REFRESH_TOKEN\"}")
split_response "$resp"
assert_status "POST /api/v1/auth/refresh" 200 "$RESP_STATUS" "$RESP_BODY"
NEW_TOKEN=$(echo "$RESP_BODY" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['tokens']['access_token'])" 2>/dev/null || echo "")
ACCESS_TOKEN="$NEW_TOKEN"
assert_field "refresh" "new access_token" "$ACCESS_TOKEN"

# ─────────────────────────────────────────────────────────────────────────────
hdr "8. Auth — GET /me (authenticated)"
resp=$(api GET /api/v1/auth/me)
split_response "$resp"
assert_status "GET /api/v1/auth/me" 200 "$RESP_STATUS" "$RESP_BODY"
me_email=$(echo "$RESP_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['email'])" 2>/dev/null || echo "")
assert_field "/me" "email" "$me_email"

# ─────────────────────────────────────────────────────────────────────────────
hdr "9. Unauthenticated Request (expect 401)"
SAVED_TOKEN="$ACCESS_TOKEN"; ACCESS_TOKEN=""
resp=$(api GET /api/v1/profile)
split_response "$resp"
assert_status "GET /api/v1/profile (no token)" 401 "$RESP_STATUS" "$RESP_BODY"
ACCESS_TOKEN="$SAVED_TOKEN"

# ─────────────────────────────────────────────────────────────────────────────
hdr "10. Profile"
resp=$(api GET /api/v1/profile)
split_response "$resp"
assert_status "GET /api/v1/profile" 200 "$RESP_STATUS" "$RESP_BODY"
prof_email=$(echo "$RESP_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['user']['email'])" 2>/dev/null || echo "")
assert_field "profile" "email" "$prof_email"

# ─────────────────────────────────────────────────────────────────────────────
hdr "11. Devices — Add"
resp=$(api POST /api/v1/devices '{"name":"test-laptop","type":"desktop","public_key":"wg-pubkey-placeholder-abc123"}')
split_response "$resp"
assert_status "POST /api/v1/devices" 201 "$RESP_STATUS" "$RESP_BODY"
DEVICE_ID=$(echo "$RESP_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['device']['id'])" 2>/dev/null || echo "")
assert_field "device" "id" "$DEVICE_ID"

# ─────────────────────────────────────────────────────────────────────────────
hdr "12. Devices — List"
resp=$(api GET /api/v1/devices)
split_response "$resp"
assert_status "GET /api/v1/devices" 200 "$RESP_STATUS" "$RESP_BODY"
count=$(echo "$RESP_BODY" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['devices']))" 2>/dev/null || echo "0")
if [[ "$count" -ge 1 ]]; then ok "devices list: $count device(s) found"; else fail "devices list: expected ≥1, got $count"; fi

# ─────────────────────────────────────────────────────────────────────────────
hdr "13. Devices — Remove"
resp=$(api DELETE "/api/v1/devices/$DEVICE_ID")
split_response "$resp"
assert_status "DELETE /api/v1/devices/:id" 200 "$RESP_STATUS" "$RESP_BODY"

# ─────────────────────────────────────────────────────────────────────────────
hdr "14. Nodes — List (public)"
resp=$(api GET /api/v1/nodes)
split_response "$resp"
assert_status "GET /api/v1/nodes" 200 "$RESP_STATUS" "$RESP_BODY"
node_count=$(echo "$RESP_BODY" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['nodes']))" 2>/dev/null || echo "0")
ok "nodes list: $node_count online node(s)"

# ─────────────────────────────────────────────────────────────────────────────
hdr "15. Admin — Forbidden for regular user"
resp=$(api GET /api/v1/admin/nodes)
split_response "$resp"
assert_status "GET /api/v1/admin/nodes (non-admin)" 403 "$RESP_STATUS" "$RESP_BODY"

# ─────────────────────────────────────────────────────────────────────────────
hdr "16. Prometheus — VPN metrics appear after activity"
resp=$(api GET /metrics)
split_response "$resp"
if echo "$RESP_BODY" | grep -q 'vpnplatform_auth_events_total.*login.*success="true"'; then
  ok "vpnplatform_auth_events_total{login,success=true} present"
else
  fail "vpnplatform_auth_events_total{login,success=true} not found in /metrics"
fi
if echo "$RESP_BODY" | grep -q 'vpnplatform_auth_events_total.*register'; then
  ok "vpnplatform_auth_events_total{register} present"
else
  fail "vpnplatform_auth_events_total{register} not found in /metrics"
fi

# ─────────────────────────────────────────────────────────────────────────────
hdr "17. Auth — Logout"
resp=$(api POST /api/v1/auth/logout "{\"refresh_token\":\"$REFRESH_TOKEN\"}")
split_response "$resp"
assert_status "POST /api/v1/auth/logout" 200 "$RESP_STATUS" "$RESP_BODY"

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
TOTAL=$((PASS + FAIL))
if [[ $FAIL -eq 0 ]]; then
  echo -e "  ${GREEN}${BOLD}ALL $TOTAL CHECKS PASSED${NC}"
else
  echo -e "  ${RED}${BOLD}$FAIL/$TOTAL CHECKS FAILED${NC}  (${GREEN}$PASS passed${NC})"
fi
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
[[ $FAIL -eq 0 ]] && exit 0 || exit 1
