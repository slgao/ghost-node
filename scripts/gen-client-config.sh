#!/usr/bin/env bash
# gen-client-config.sh — Pull a subscription from the VPN platform and generate
# ready-to-import config files for every major client app.
#
# Usage:
#   ./scripts/gen-client-config.sh \
#     --api    https://api.yourdomain.com \
#     --token  <your-jwt-access-token> \
#     --node   <node-uuid> \
#     --out    ./client-configs
#
# Output files:
#   client-configs/
#     vless.txt          — base64-encoded URI list (v2rayN / NekoRay)
#     clash.yaml         — Clash Meta / Clash Verge / Stash
#     singbox.json       — sing-box outbounds
#     qr-vless-N.png     — QR codes for each VLESS URI (requires qrencode)

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
API="${VPN_API:-http://localhost:8080}"
TOKEN="${VPN_TOKEN:-}"
NODE_ID="${VPN_NODE:-}"
OUT_DIR="${VPN_OUT:-./client-configs}"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; RED='\033[0;31m'; NC='\033[0m'; BOLD='\033[1m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()     { echo -e "${RED}[FAIL]${NC}  $*" >&2; exit 1; }

# ── Parse args ────────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --api)    API="$2";     shift 2 ;;
    --token)  TOKEN="$2";   shift 2 ;;
    --node)   NODE_ID="$2"; shift 2 ;;
    --out)    OUT_DIR="$2"; shift 2 ;;
    *) die "Unknown argument: $1" ;;
  esac
done

[[ -z "$TOKEN"   ]] && die "--token (or VPN_TOKEN env) is required"
[[ -z "$NODE_ID" ]] && die "--node  (or VPN_NODE  env) is required"

mkdir -p "$OUT_DIR"

BASE_URL="$API/api/v1/nodes/$NODE_ID/subscription"
AUTH_HDR="Authorization: Bearer $TOKEN"

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}   VPN Platform — Client Config Generator${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
info "API:     $API"
info "Node:    $NODE_ID"
info "Output:  $OUT_DIR"
echo ""

# ── 1. Download all formats ────────────────────────────────────────────────────
info "Fetching subscription data..."

# vless (base64-encoded URI list for v2rayN/NekoRay)
HTTP=$(curl -s -w "\n%{http_code}" -H "$AUTH_HDR" "$BASE_URL?format=vless")
CODE=$(echo "$HTTP" | tail -1)
BODY=$(echo "$HTTP" | sed '$d')
[[ "$CODE" == "200" ]] || die "Failed to fetch vless format (HTTP $CODE): $BODY"
echo "$BODY" > "$OUT_DIR/vless.txt"
success "vless.txt written (base64 URI list)"

# clash YAML
HTTP=$(curl -s -w "\n%{http_code}" -H "$AUTH_HDR" "$BASE_URL?format=clash")
CODE=$(echo "$HTTP" | tail -1)
BODY=$(echo "$HTTP" | sed '$d')
[[ "$CODE" == "200" ]] || die "Failed to fetch clash format (HTTP $CODE)"
echo "$BODY" > "$OUT_DIR/clash.yaml"
success "clash.yaml written"

# sing-box JSON
HTTP=$(curl -s -w "\n%{http_code}" -H "$AUTH_HDR" "$BASE_URL?format=singbox")
CODE=$(echo "$HTTP" | tail -1)
BODY=$(echo "$HTTP" | sed '$d')
[[ "$CODE" == "200" ]] || die "Failed to fetch singbox format (HTTP $CODE)"
echo "$BODY" > "$OUT_DIR/singbox.json"
success "singbox.json written"

# ── 2. Decode and display VLESS URIs ──────────────────────────────────────────
DECODED=$(base64 --decode "$OUT_DIR/vless.txt" 2>/dev/null || base64 -d "$OUT_DIR/vless.txt" 2>/dev/null || echo "")
if [[ -n "$DECODED" ]]; then
  echo ""
  info "VLESS URIs (import into any client):"
  echo ""
  N=0
  while IFS= read -r uri; do
    [[ -z "$uri" ]] && continue
    N=$((N+1))
    echo -e "  ${CYAN}[$N]${NC} $uri"
    echo "$uri" > "$OUT_DIR/vless-${N}.txt"

    # Generate QR code if qrencode is available
    if command -v qrencode &>/dev/null; then
      qrencode -t PNG -o "$OUT_DIR/qr-vless-${N}.png" "$uri" 2>/dev/null && \
        success "QR code: $OUT_DIR/qr-vless-${N}.png"
    fi
  done <<< "$DECODED"
fi

# ── 3. Print import instructions ──────────────────────────────────────────────
echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}  How to import${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${BOLD}iOS — Shadowrocket:${NC}"
echo "  1. Tap + → Type → Subscribe"
echo "  2. URL: $BASE_URL?format=vless"
echo "  3. Add auth header if required (or pre-auth via cookie)"
echo ""
echo -e "${BOLD}iOS — Stash / Quantumult X:${NC}"
echo "  1. Profiles → New → Remote Profile"
echo "  2. URL: $BASE_URL?format=clash"
echo ""
echo -e "${BOLD}Android — v2rayNG:${NC}"
echo "  1. ⋮ → Import config from clipboard"
echo "     Paste content of: $OUT_DIR/vless.txt"
echo "  OR"
echo "  1. ⋮ → Subscription group settings → Add"
echo "  2. URL: $BASE_URL?format=vless"
echo ""
echo -e "${BOLD}Android — Clash Meta for Android:${NC}"
echo "  Profiles → New Profile → URL: $BASE_URL?format=clash"
echo ""
echo -e "${BOLD}macOS — Clash Verge / Clash-Verge-Rev:${NC}"
echo "  Profiles → Import URL: $BASE_URL?format=clash"
echo ""
echo -e "${BOLD}macOS/Windows — NekoRay / NekoBox:${NC}"
echo "  Program → Import from clipboard"
echo "  Paste content of: $OUT_DIR/vless.txt"
echo ""
echo -e "${BOLD}macOS/Windows — sing-box GUI:${NC}"
echo "  Import file: $OUT_DIR/singbox.json"
echo ""
echo -e "${BOLD}Windows — v2rayN:${NC}"
echo "  Subscriptions → Add subscription → URL: $BASE_URL?format=vless"
echo ""
echo -e "${BOLD}Windows — Hiddify:${NC}"
echo "  Add Profile → URL: $BASE_URL?format=vless"
echo ""
if ! command -v qrencode &>/dev/null; then
  warn "Install qrencode to generate QR codes: brew install qrencode / apt install qrencode"
fi

echo ""
echo -e "${GREEN}${BOLD}Done!${NC} Config files saved to: $OUT_DIR/"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
