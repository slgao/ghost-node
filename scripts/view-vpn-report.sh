#!/usr/bin/env bash
# view-vpn-report.sh — Run traffic analysis on the VPN server and open the report.
#
# Run on your own machine:
#   bash scripts/view-vpn-report.sh [--hours 24] [--host ghost-node-jp1]
#
# What it does:
#   1. Copies analyze-vpn-traffic.sh to the server
#   2. Runs it there (the access log never leaves the server)
#   3. Downloads the finished HTML report
#   4. Opens it in your default browser
#
# Flags:
#   --hours N     Window to analyse (default 24)
#   --host HOST   SSH host or alias (default ghost-node-jp1)
#   --out FILE    Where to save the report locally
#   --json [FILE] Also fetch the raw analysis as JSON
#   --top N       Rows kept in the report tables (default 200)
#   --no-open     Download only, do not open a browser

set -euo pipefail

HOURS="${VPN_HOURS:-24}"
SSH_HOST="${VPN_SSH_HOST:-ghost-node-jp1}"
LOCAL_OUT="${VPN_LOCAL_OUT:-$HOME/Desktop/vpn-report.html}"
LOCAL_JSON=""
WANT_JSON=""
TOP="${VPN_TOP:-200}"
OPEN=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --hours) HOURS="$2"; shift 2 ;;
    --host)  SSH_HOST="$2"; shift 2 ;;
    --out)   LOCAL_OUT="$2"; shift 2 ;;
    --top)   TOP="$2"; shift 2 ;;
    --json)
      WANT_JSON=1
      if [[ $# -ge 2 && "$2" != --* ]]; then LOCAL_JSON="$2"; shift 2; else shift; fi ;;
    --no-open) OPEN=0; shift ;;
    -h|--help)
      awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
      exit 0 ;;
    *) shift ;;
  esac
done

[[ -n "$WANT_JSON" && -z "$LOCAL_JSON" ]] && LOCAL_JSON="${LOCAL_OUT%.html}.json"

GREEN='\033[0;32m'; CYAN='\033[0;36m'; RED='\033[0;31m'; BOLD='\033[1m'; NC='\033[0m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
die()     { echo -e "${RED}[FAIL]${NC}  $*" >&2; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[[ -f "$SCRIPT_DIR/analyze-vpn-traffic.sh" ]] || die "analyze-vpn-traffic.sh not found next to this script"

REMOTE_SCRIPT="/tmp/analyze-vpn-traffic.sh"
REMOTE_HTML="/tmp/vpn-report.html"
REMOTE_JSON="/tmp/vpn-report.json"

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}   VPN Traffic Report — ${SSH_HOST}, last ${HOURS}h${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

info "Uploading analyzer to $SSH_HOST..."
scp -q "$SCRIPT_DIR/analyze-vpn-traffic.sh" "$SSH_HOST:$REMOTE_SCRIPT" \
  || die "Could not reach $SSH_HOST over SSH. Check ~/.ssh/config, or pass --host user@IP.
       If the node's IP has rotated, run: ghostctl status"

info "Analysing on the server (the access log stays there)..."
echo ""
REMOTE_ARGS="--hours $HOURS --out $REMOTE_HTML --top $TOP"
[[ -n "$WANT_JSON" ]] && REMOTE_ARGS="$REMOTE_ARGS --json $REMOTE_JSON"
ssh "$SSH_HOST" "sudo bash $REMOTE_SCRIPT $REMOTE_ARGS" || die "Analysis failed on the server"

echo ""
info "Downloading report..."
mkdir -p "$(dirname "$LOCAL_OUT")"
scp -q "$SSH_HOST:$REMOTE_HTML" "$LOCAL_OUT" || die "Could not download the report"
success "Report saved to $LOCAL_OUT"

if [[ -n "$WANT_JSON" ]]; then
  scp -q "$SSH_HOST:$REMOTE_JSON" "$LOCAL_JSON" && success "JSON saved to $LOCAL_JSON"
fi

if [[ $OPEN -eq 1 ]]; then
  info "Opening in browser..."
  if command -v open &>/dev/null; then
    open "$LOCAL_OUT"
  elif command -v xdg-open &>/dev/null; then
    xdg-open "$LOCAL_OUT" >/dev/null 2>&1 &
  else
    info "No browser opener found — open $LOCAL_OUT manually"
  fi
fi

echo ""
echo -e "${GREEN}${BOLD}Done.${NC} $LOCAL_OUT"
echo ""
