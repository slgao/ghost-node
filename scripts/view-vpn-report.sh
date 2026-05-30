#!/usr/bin/env bash
# view-vpn-report.sh — Run traffic analysis on Oracle VM and open report in browser.
#
# Run on your Mac:
#   bash scripts/view-vpn-report.sh [--hours 24]
#
# What it does:
#   1. Copies analyze-vpn-traffic.sh to the Oracle VM
#   2. Runs it remotely
#   3. Downloads the HTML report
#   4. Opens it in your default browser

set -euo pipefail

HOURS="${VPN_HOURS:-24}"
SSH_HOST="${VPN_SSH_HOST:-ghost-node-jp1}"
LOCAL_OUT="${VPN_LOCAL_OUT:-$HOME/Desktop/vpn-report.html}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --hours) HOURS="$2"; shift 2 ;;
    --host)  SSH_HOST="$2"; shift 2 ;;
    *) shift ;;
  esac
done

GREEN='\033[0;32m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}   VPN Traffic Report — last ${HOURS}h${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# 1. Copy analyzer to server
info "Uploading analyzer script to $SSH_HOST..."
scp -q "$SCRIPT_DIR/analyze-vpn-traffic.sh" "$SSH_HOST:/tmp/analyze-vpn-traffic.sh"

# 2. Run on server
info "Running analysis on server (last ${HOURS}h)..."
echo ""
ssh "$SSH_HOST" "sudo bash /tmp/analyze-vpn-traffic.sh --hours $HOURS --out /tmp/vpn-report.html"

# 3. Download report
echo ""
info "Downloading report..."
scp -q "$SSH_HOST:/tmp/vpn-report.html" "$LOCAL_OUT"
success "Report saved to $LOCAL_OUT"

# 4. Open in browser
info "Opening in browser..."
if command -v open &>/dev/null; then
  open "$LOCAL_OUT"
elif command -v xdg-open &>/dev/null; then
  xdg-open "$LOCAL_OUT"
fi

echo ""
echo -e "${GREEN}${BOLD}Done!${NC} Report: $LOCAL_OUT"
echo ""
