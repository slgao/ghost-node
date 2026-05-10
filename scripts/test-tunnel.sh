#!/usr/bin/env bash
# test-tunnel.sh — End-to-end tunnel test using local Docker containers.
# No VPS required. Proves the full VLESS+WS tunnel stack works.
#
# Route: curl (host) → xray-client SOCKS5 :10808 → [VLESS+WS] → xray-server :10000 → internet

set -euo pipefail

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; RED='\033[0;31m'; NC='\033[0m'; BOLD='\033[1m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[PASS]${NC}  $*"; }
fail()    { echo -e "${RED}[FAIL]${NC}  $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }

PASS=0; FAIL=0

pass() { success "$1"; PASS=$((PASS+1)); }
fail_msg() { fail "$1"; echo "       $2"; FAIL=$((FAIL+1)); }

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}   VPN Platform — Local Tunnel Test${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# ── 1. Start tunnel containers ────────────────────────────────────────────────
info "Building and starting Xray containers (first run downloads Xray ~10 MB)..."
docker compose --profile tunnel-test up -d --build xray-server xray-client 2>&1 | \
  grep -E "Building|build|Started|Created|Running|error" || true

info "Waiting for Xray to initialise..."
sleep 5

# ── 2. Check containers are up ────────────────────────────────────────────────
SERVER_STATUS=$(docker compose ps xray-server 2>/dev/null | grep -c "running\|Up" || echo "0")
CLIENT_STATUS=$(docker compose ps xray-client 2>/dev/null | grep -c "running\|Up" || echo "0")

if [[ "$SERVER_STATUS" -ge 1 ]]; then
  pass "xray-server container running"
else
  fail_msg "xray-server container is not running" "$(docker compose logs xray-server --tail=5 2>/dev/null)"
  echo ""
  echo -e "${RED}Cannot continue — xray-server failed to start.${NC}"
  exit 1
fi

if [[ "$CLIENT_STATUS" -ge 1 ]]; then
  pass "xray-client container running"
else
  fail_msg "xray-client container is not running" "$(docker compose logs xray-client --tail=5 2>/dev/null)"
  exit 1
fi

# ── 3. SOCKS5 proxy responds ──────────────────────────────────────────────────
info "Testing SOCKS5 proxy (port 10808)..."
SOCKS_CODE=$(curl -s --max-time 8 --proxy socks5h://localhost:10808 \
  http://example.com -o /dev/null -w "%{http_code}" 2>/dev/null || echo "000")

if [[ "$SOCKS_CODE" == "200" ]]; then
  pass "SOCKS5 proxy accepts connections and reaches internet (HTTP $SOCKS_CODE)"
else
  fail_msg "SOCKS5 proxy failed" "HTTP response: $SOCKS_CODE (expected 200)"
fi

# ── 4. HTTP proxy responds ────────────────────────────────────────────────────
info "Testing HTTP proxy (port 10809)..."
HTTP_CODE=$(curl -s --max-time 8 --proxy http://localhost:10809 \
  http://example.com -o /dev/null -w "%{http_code}" 2>/dev/null || echo "000")

if [[ "$HTTP_CODE" == "200" ]]; then
  pass "HTTP proxy accepts connections (HTTP $HTTP_CODE)"
else
  fail_msg "HTTP proxy failed" "HTTP response: $HTTP_CODE (expected 200)"
fi

# ── 5. Traffic routes end-to-end through tunnel ───────────────────────────────
info "Fetching public IP via tunnel..."
DIRECT_IP=$(curl -s --max-time 5 https://ifconfig.me 2>/dev/null || \
            curl -s --max-time 5 https://api.ipify.org 2>/dev/null || echo "unavailable")
TUNNEL_IP=$(curl -s --max-time 12 --proxy socks5h://localhost:10808 \
            https://ifconfig.me 2>/dev/null || \
            curl -s --max-time 12 --proxy socks5h://localhost:10808 \
            https://api.ipify.org 2>/dev/null || echo "unavailable")

echo ""
echo -e "  Direct connection IP:    ${CYAN}${DIRECT_IP}${NC}"
echo -e "  Through VLESS tunnel IP: ${CYAN}${TUNNEL_IP}${NC}"
echo ""

if [[ "$TUNNEL_IP" != "unavailable" ]] && \
   [[ "$TUNNEL_IP" =~ ^[0-9a-fA-F.:]+$ ]]; then
  pass "Traffic reaches internet through VLESS+WS tunnel (IP: $TUNNEL_IP)"
  if [[ "$DIRECT_IP" == "$TUNNEL_IP" ]]; then
    warn "Both IPs match — expected for local test (traffic exits via your Mac)"
    warn "On a real VPS the tunnel IP would show the server's country/IP"
  fi
else
  fail_msg "Could not reach internet through tunnel" "tunnel IP: $TUNNEL_IP"
fi

# ── 6. DNS resolves through tunnel ────────────────────────────────────────────
info "Testing DNS resolution through tunnel..."
DNS_OUT=$(curl -s --max-time 10 --proxy socks5h://localhost:10808 \
  "https://dns.google/resolve?name=google.com&type=A" -o /dev/null \
  -w "%{http_code}" 2>/dev/null || echo "000")

if [[ "$DNS_OUT" == "200" ]]; then
  pass "DNS resolves through tunnel (socks5h = DNS proxied, not local)"
else
  warn "DNS-over-HTTPS test returned HTTP $DNS_OUT (may be blocked by ISP — non-critical)"
fi

# ── 7. Print import info ──────────────────────────────────────────────────────
VLESS_URI="vless://f47ac10b-58cc-4372-a567-0e02b2c3d479@127.0.0.1:10000?encryption=none&type=ws&path=%2Fws#LocalTest"

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}  Connect your client app to the local tunnel${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "  ${BOLD}Quick test (copy-paste):${NC}"
echo -e "  ${CYAN}curl --proxy socks5h://127.0.0.1:10808 https://ifconfig.me${NC}"
echo ""
echo -e "  ${BOLD}SOCKS5 proxy:${NC}  socks5://127.0.0.1:10808  (use in browser/app)"
echo -e "  ${BOLD}HTTP proxy:${NC}    http://127.0.0.1:10809"
echo ""
echo -e "  ${BOLD}VLESS URI (import into NekoRay / v2rayNG / Hiddify):${NC}"
echo -e "  ${CYAN}$VLESS_URI${NC}"
echo ""
echo -e "  ${YELLOW}Note:${NC} This proves the VLESS+WebSocket tunnel protocol works."
echo -e "  On a real VPS the exit IP changes to the server's country."
echo ""

# ── Summary ───────────────────────────────────────────────────────────────────
TOTAL=$((PASS+FAIL))
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
if [[ $FAIL -eq 0 ]]; then
  echo -e "${GREEN}${BOLD}  ALL $TOTAL CHECKS PASSED${NC}"
else
  echo -e "${YELLOW}${BOLD}  $PASS PASSED, $FAIL FAILED (out of $TOTAL)${NC}"
fi
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "  Stop tunnel:  ${CYAN}docker compose --profile tunnel-test down${NC}"
echo ""

[[ $FAIL -eq 0 ]]
