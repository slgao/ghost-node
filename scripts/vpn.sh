#!/usr/bin/env bash
# vpn.sh — Connect to the Oracle VPN server (Linux & macOS).
#
# Pulls credentials from the server, generates a VLESS+REALITY client config,
# runs the Xray client in Docker, and configures the system proxy.
#
# Usage:
#   bash scripts/vpn.sh [start|stop|status]

set -euo pipefail

SSH_HOST="${VPN_SSH_HOST:-ghost-node-jp1}"
SOCKS_PORT="10808"
HTTP_PORT="10809"
CONFIG_FILE="$HOME/.vpn/xray-client.json"
XRAY_BIN="$HOME/.vpn/xray"
XRAY_PID_FILE="$HOME/.vpn/xray.pid"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; RED='\033[0;31m'; NC='\033[0m'; BOLD='\033[1m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()     { echo -e "${RED}[FAIL]${NC}  $*" >&2; exit 1; }

CMD="${1:-start}"

# ── Helpers ───────────────────────────────────────────────────────────────────

set_proxy() {
  if command -v gsettings &>/dev/null; then
    gsettings set org.gnome.system.proxy mode 'manual'
    gsettings set org.gnome.system.proxy.socks host '127.0.0.1'
    gsettings set org.gnome.system.proxy.socks port "$SOCKS_PORT"
    gsettings set org.gnome.system.proxy ignore-hosts "['localhost', '127.0.0.1', '192.168.*', '10.*']"
    success "System proxy set → 127.0.0.1:$SOCKS_PORT (Firefox will update live)"
  elif command -v networksetup &>/dev/null; then
    local service
    service=$(networksetup -listallnetworkservices 2>/dev/null | grep -iE "wi-fi|wifi|airport" | head -1)
    [[ -z "$service" ]] && { warn "No Wi-Fi service found — set proxy manually"; return; }
    networksetup -setsocksfirewallproxy      "$service" 127.0.0.1 "$SOCKS_PORT"
    networksetup -setsocksfirewallproxystate "$service" on
    networksetup -setproxybypassdomains      "$service" "localhost" "127.0.0.1" "192.168.*" "10.*"
    success "System SOCKS5 proxy set → 127.0.0.1:$SOCKS_PORT (service: $service)"
  else
    warn "No proxy manager found — set SOCKS5 127.0.0.1:$SOCKS_PORT manually"
  fi
}

clear_proxy() {
  if command -v gsettings &>/dev/null; then
    gsettings set org.gnome.system.proxy mode 'none'
    success "System proxy cleared (Firefox updated live)"
  elif command -v networksetup &>/dev/null; then
    local service
    service=$(networksetup -listallnetworkservices 2>/dev/null | grep -iE "wi-fi|wifi|airport" | head -1)
    [[ -z "$service" ]] && return
    networksetup -setsocksfirewallproxystate "$service" off
    success "System proxy cleared"
  fi
}

# ── Start ─────────────────────────────────────────────────────────────────────
cmd_start() {
  echo ""
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BOLD}   Mac VPN — Connecting to Oracle server${NC}"
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""

  # Stop any existing xray client instance
  if [[ -f "$XRAY_PID_FILE" ]]; then
    kill "$(cat "$XRAY_PID_FILE")" 2>/dev/null || true
    rm -f "$XRAY_PID_FILE"
  fi
  # Also kill any stray Docker-based vpn-client from previous runs
  docker rm -f vpn-client 2>/dev/null || true

  # Kill any other process holding port 10808 or 10809
  for port in $SOCKS_PORT $HTTP_PORT; do
    pid=$(lsof -ti tcp:"$port" 2>/dev/null || true)
    if [[ -n "$pid" ]]; then
      warn "Port $port in use by PID $pid — stopping it..."
      kill "$pid" 2>/dev/null || true
      sleep 1
    fi
  done

  # Fetch credentials from server
  info "Fetching credentials from $SSH_HOST..."
  CREDS=$(ssh "$SSH_HOST" "sudo cat /root/vpn-server-credentials.env" 2>/dev/null) \
    || die "Cannot reach $SSH_HOST — is your SSH config set up? (see ~/.ssh/config)"

  SERVER_IP=$(echo     "$CREDS" | grep SERVER_IP            | cut -d= -f2 | tr -d '"')
  VLESS_PORT=$(echo    "$CREDS" | grep VLESS_PORT           | cut -d= -f2 | tr -d '"')
  VLESS_UUID=$(echo    "$CREDS" | grep VLESS_UUID           | cut -d= -f2 | tr -d '"')
  PUBLIC_KEY=$(echo    "$CREDS" | grep REALITY_PUBLIC_KEY   | cut -d= -f2 | tr -d '"')
  SHORT_ID=$(echo      "$CREDS" | grep REALITY_SHORT_ID     | cut -d= -f2 | tr -d '"')
  CAMOUFLAGE=$(echo    "$CREDS" | grep CAMOUFLAGE_DOMAIN    | cut -d= -f2 | tr -d '"')
  CAMOUFLAGE="${CAMOUFLAGE:-www.microsoft.com}"
  VLESS_PORT="${VLESS_PORT:-443}"

  [[ -z "$SERVER_IP"   ]] && die "Could not read SERVER_IP from credentials"
  [[ -z "$VLESS_UUID"  ]] && die "Could not read VLESS_UUID from credentials"
  [[ -z "$PUBLIC_KEY"  ]] && die "Could not read REALITY_PUBLIC_KEY from credentials"
  [[ -z "$SHORT_ID"    ]] && die "Could not read REALITY_SHORT_ID from credentials"

  success "Credentials loaded (server: $SERVER_IP)"

  # Write client config
  mkdir -p "$HOME/.vpn"
  cat > "$CONFIG_FILE" <<JSON
{
  "log": { "loglevel": "warning" },
  "inbounds": [
    {
      "tag": "socks5-in",
      "listen": "0.0.0.0",
      "port": $SOCKS_PORT,
      "protocol": "socks",
      "settings": { "auth": "noauth", "udp": true }
    },
    {
      "tag": "http-in",
      "listen": "0.0.0.0",
      "port": $HTTP_PORT,
      "protocol": "http"
    }
  ],
  "outbounds": [
    {
      "tag": "vless-reality",
      "protocol": "vless",
      "settings": {
        "vnext": [
          {
            "address": "$SERVER_IP",
            "port": $VLESS_PORT,
            "users": [
              {
                "id": "$VLESS_UUID",
                "encryption": "none",
                "flow": "xtls-rprx-vision"
              }
            ]
          }
        ]
      },
      "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
          "serverName": "$CAMOUFLAGE",
          "fingerprint": "chrome",
          "publicKey": "$PUBLIC_KEY",
          "shortId": "$SHORT_ID"
        }
      }
    },
    { "tag": "direct", "protocol": "freedom" }
  ]
}
JSON

  success "Client config written to $CONFIG_FILE"

  # Download xray binary from server; re-download if version differs from server
  mkdir -p "$HOME/.vpn"
  SERVER_VER=$(ssh "$SSH_HOST" "/usr/local/xray/xray version 2>/dev/null | head -1 | awk '{print \$2}'" 2>/dev/null || echo "")
  CLIENT_VER=$("$XRAY_BIN" version 2>/dev/null | head -1 | awk '{print $2}' 2>/dev/null || echo "")
  if [[ ! -x "$XRAY_BIN" ]] || [[ -n "$SERVER_VER" && "$SERVER_VER" != "$CLIENT_VER" ]]; then
    info "Downloading xray binary from $SSH_HOST (server: $SERVER_VER, local: ${CLIENT_VER:-none})..."
    scp "$SSH_HOST:/usr/local/xray/xray" "$XRAY_BIN" \
      || die "Could not copy xray from server — check SSH access to $SSH_HOST"
    chmod +x "$XRAY_BIN"
    success "xray binary ready at $XRAY_BIN (v${SERVER_VER})"
  fi

  # Start xray client in background
  info "Starting xray client..."
  "$XRAY_BIN" run -c "$CONFIG_FILE" > "$HOME/.vpn/xray-client.log" 2>&1 &
  echo $! > "$XRAY_PID_FILE"
  sleep 3

  # Test the tunnel
  info "Testing tunnel..."
  TUNNEL_IP=$(curl -s --max-time 10 --proxy socks5h://127.0.0.1:$SOCKS_PORT https://ifconfig.me 2>/dev/null || echo "")
  if [[ -n "$TUNNEL_IP" ]]; then
    success "Tunnel working — exit IP: $TUNNEL_IP"
  else
    warn "Tunnel test failed — check logs: $HOME/.vpn/xray-client.log"
  fi

  # Set system proxy
  info "Configuring system proxy..."
  set_proxy

  echo ""
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${GREEN}${BOLD}  VPN connected!${NC}"
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo -e "  ${BOLD}Exit IP:${NC}       $TUNNEL_IP"
  echo -e "  ${BOLD}SOCKS5:${NC}        127.0.0.1:$SOCKS_PORT"
  echo -e "  ${BOLD}HTTP proxy:${NC}    127.0.0.1:$HTTP_PORT"
  echo ""
  echo -e "  Verify: open ${CYAN}https://ip.sb${NC} in your browser"
  echo -e "  Stop:   ${CYAN}bash scripts/vpn.sh stop${NC}"
  echo ""
}

# ── Stop ──────────────────────────────────────────────────────────────────────
cmd_stop() {
  info "Stopping VPN client..."
  if [[ -f "$XRAY_PID_FILE" ]]; then
    kill "$(cat "$XRAY_PID_FILE")" 2>/dev/null && success "xray stopped" || warn "xray was not running"
    rm -f "$XRAY_PID_FILE"
  else
    warn "No xray PID file found"
  fi
  docker rm -f vpn-client 2>/dev/null || true
  clear_proxy
  echo ""
  echo -e "${BOLD}VPN disconnected.${NC}"
  echo ""
}

# ── Status ────────────────────────────────────────────────────────────────────
cmd_status() {
  echo ""
  if [[ -f "$XRAY_PID_FILE" ]] && kill -0 "$(cat "$XRAY_PID_FILE")" 2>/dev/null; then
    success "VPN client is running (PID $(cat "$XRAY_PID_FILE"))"
    TUNNEL_IP=$(curl -s --max-time 8 --proxy socks5h://127.0.0.1:$SOCKS_PORT https://ifconfig.me 2>/dev/null || echo "unreachable")
    echo -e "  Exit IP: ${CYAN}$TUNNEL_IP${NC}"
    echo -e "  SOCKS5:  127.0.0.1:$SOCKS_PORT"
  else
    warn "VPN client is NOT running"
    echo -e "  Start:  ${CYAN}bash scripts/vpn.sh start${NC}"
  fi

  echo -e "  Verify in browser: ${CYAN}https://ip.sb${NC} should show the exit IP above"
  echo ""
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
case "$CMD" in
  start)  cmd_start  ;;
  stop)   cmd_stop   ;;
  status) cmd_status ;;
  *)
    echo "Usage: bash scripts/vpn.sh [start|stop|status]"
    echo "Usage: bash scripts/vpn.sh [start|stop|status]"
    exit 1
    ;;
esac
