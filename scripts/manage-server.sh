#!/usr/bin/env bash
# manage-server.sh — Day-to-day management of your Xray VPN server.
#
# Usage (run on the remote server):
#   bash manage-server.sh [command]
#
# Commands:
#   status      Show Xray status and connection info (default)
#   restart     Restart Xray
#   stop        Stop Xray
#   start       Start Xray
#   logs        Show recent Xray logs
#   credentials Show your VLESS URI and server credentials
#   fix-fw      Re-apply iptables rules (run if VPN stops after reboot)
#   save-fw     Persist iptables rules across reboots
#   update      Update Xray to the latest version

set -euo pipefail

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; RED='\033[0;31m'; NC='\033[0m'; BOLD='\033[1m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()     { echo -e "${RED}[FAIL]${NC}  $*" >&2; exit 1; }

[[ $(id -u) -eq 0 ]] || die "Run as root: sudo bash manage-server.sh $*"

CREDS_FILE="/root/vpn-server-credentials.env"
CMD="${1:-status}"

# ── Load credentials ──────────────────────────────────────────────────────────
load_creds() {
  [[ -f "$CREDS_FILE" ]] && source "$CREDS_FILE" || true
}

# ── Commands ──────────────────────────────────────────────────────────────────

cmd_status() {
  echo ""
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BOLD}   VPN Server Status${NC}"
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""

  # Xray service
  if systemctl is-active --quiet xray; then
    success "Xray service is running"
  else
    warn "Xray service is NOT running — run: sudo bash manage-server.sh restart"
  fi

  # Port 443 listening
  if ss -tlnp | grep -q ':443'; then
    success "Port 443 is listening"
  else
    warn "Port 443 is NOT listening"
  fi

  # Firewall
  if iptables -L INPUT -n | grep -q "dpt:443"; then
    success "iptables allows port 443"
  else
    warn "iptables rule for port 443 missing — run: sudo bash manage-server.sh fix-fw"
  fi

  load_creds
  echo ""
  echo -e "  ${BOLD}Server IP:${NC}   ${SERVER_IP:-$(curl -4 -s --max-time 5 ifconfig.me)}"
  echo -e "  ${BOLD}Protocol:${NC}    VLESS + REALITY"
  echo -e "  ${BOLD}Port:${NC}        ${VLESS_PORT:-443}"
  echo -e "  ${BOLD}Uptime:${NC}      $(systemctl show xray --property=ActiveEnterTimestamp | cut -d= -f2)"
  echo ""
}

cmd_restart() {
  info "Restarting Xray..."
  systemctl restart xray
  sleep 2
  if systemctl is-active --quiet xray; then
    success "Xray restarted successfully"
  else
    die "Xray failed to restart — check: sudo journalctl -u xray -n 50 --no-pager"
  fi
}

cmd_stop() {
  info "Stopping Xray..."
  systemctl stop xray
  success "Xray stopped"
}

cmd_start() {
  info "Starting Xray..."
  systemctl start xray
  sleep 2
  if systemctl is-active --quiet xray; then
    success "Xray started"
  else
    die "Xray failed to start — check: sudo journalctl -u xray -n 50 --no-pager"
  fi
}

cmd_logs() {
  echo ""
  journalctl -u xray -n 50 --no-pager
}

cmd_credentials() {
  load_creds
  echo ""
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BOLD}   Server Credentials${NC}"
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo -e "  ${BOLD}Server IP:${NC}          ${SERVER_IP:-unknown}"
  echo -e "  ${BOLD}Port:${NC}               ${VLESS_PORT:-443}"
  echo -e "  ${BOLD}UUID:${NC}               ${VLESS_UUID:-unknown}"
  echo -e "  ${BOLD}Public Key:${NC}         ${REALITY_PUBLIC_KEY:-unknown}"
  echo -e "  ${BOLD}Short ID:${NC}           ${REALITY_SHORT_ID:-unknown}"
  echo -e "  ${BOLD}Camouflage Domain:${NC}  ${CAMOUFLAGE_DOMAIN:-www.microsoft.com}"
  echo ""

  if [[ -n "${VLESS_UUID:-}" && -n "${SERVER_IP:-}" ]]; then
    VLESS_URI="vless://${VLESS_UUID}@${SERVER_IP}:${VLESS_PORT:-443}?encryption=none&flow=xtls-rprx-vision&security=reality&sni=${CAMOUFLAGE_DOMAIN:-www.microsoft.com}&fp=chrome&pbk=${REALITY_PUBLIC_KEY}&sid=${REALITY_SHORT_ID}&type=tcp&headerType=none#VPNServer"
    echo -e "  ${BOLD}VLESS URI (import into client app):${NC}"
    echo ""
    echo -e "  ${CYAN}${VLESS_URI}${NC}"
    echo ""

    # QR code if qrencode is available
    if command -v qrencode &>/dev/null; then
      echo -e "  ${BOLD}QR Code:${NC}"
      qrencode -t ANSIUTF8 "$VLESS_URI"
    else
      info "Install qrencode to display QR code: apt-get install -y qrencode"
    fi
  fi
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
}

cmd_fix_fw() {
  info "Applying iptables rules for ports 443 and 8443..."

  # Remove existing rules to avoid duplicates
  iptables -D INPUT -p tcp --dport 443  -j ACCEPT 2>/dev/null || true
  iptables -D INPUT -p udp --dport 443  -j ACCEPT 2>/dev/null || true
  iptables -D INPUT -p tcp --dport 8443 -j ACCEPT 2>/dev/null || true

  # Insert before the REJECT rule (position 4)
  iptables -I INPUT 4 -p tcp --dport 443  -j ACCEPT
  iptables -I INPUT 5 -p udp --dport 443  -j ACCEPT
  iptables -I INPUT 6 -p tcp --dport 8443 -j ACCEPT

  success "iptables rules applied"
  cmd_save_fw
}

cmd_save_fw() {
  info "Saving iptables rules so they persist after reboot..."
  if ! command -v netfilter-persistent &>/dev/null; then
    apt-get install -y -qq iptables-persistent
  fi
  netfilter-persistent save
  success "Firewall rules saved — will survive reboot"
}

cmd_update() {
  XRAY_VERSION="${XRAY_VERSION:-1.8.11}"
  info "Updating Xray to latest version..."
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64)  XA="64" ;;
    aarch64) XA="arm64-v8a" ;;
    *) die "Unsupported arch: $ARCH" ;;
  esac
  XRAY_URL="https://github.com/XTLS/Xray-core/releases/latest/download/Xray-linux-${XA}.zip"
  curl -sL "$XRAY_URL" -o /tmp/xray.zip
  systemctl stop xray
  unzip -q -o /tmp/xray.zip xray -d /usr/local/xray/
  chmod +x /usr/local/xray/xray
  systemctl start xray
  rm /tmp/xray.zip
  success "Xray updated: $(xray version | head -1)"
}

# ── Dispatch ──────────────────────────────────────────────────────────────────
case "$CMD" in
  status)      cmd_status ;;
  restart)     cmd_restart ;;
  stop)        cmd_stop ;;
  start)       cmd_start ;;
  logs)        cmd_logs ;;
  credentials) cmd_credentials ;;
  fix-fw)      cmd_fix_fw ;;
  save-fw)     cmd_save_fw ;;
  update)      cmd_update ;;
  *)
    echo "Usage: sudo bash manage-server.sh [status|restart|stop|start|logs|credentials|fix-fw|save-fw|update]"
    exit 1
    ;;
esac
