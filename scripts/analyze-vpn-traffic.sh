#!/usr/bin/env bash
# analyze-vpn-traffic.sh — Analyze Xray VPN traffic and generate an HTML report.
#
# Run on the Oracle VM:
#   sudo bash analyze-vpn-traffic.sh [--hours 24] [--out /tmp/vpn-report.html]

set -euo pipefail

HOURS="${VPN_HOURS:-24}"
OUT="${VPN_OUT:-/tmp/vpn-report.html}"
XRAY_LOG="${XRAY_LOG:-/var/log/xray/access.log}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --hours) HOURS="$2"; shift 2 ;;
    --out)   OUT="$2";   shift 2 ;;
    *) shift ;;
  esac
done

GREEN='\033[0;32m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
info() { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}   VPN Traffic Analyzer — last ${HOURS}h${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# ── Install vnstat if missing ─────────────────────────────────────────────────
if ! command -v vnstat &>/dev/null; then
  info "Installing vnstat for bandwidth tracking..."
  apt-get install -y -qq vnstat
  systemctl enable --now vnstat
  sleep 2
fi

# ── 1. Bandwidth stats (vnstat) ───────────────────────────────────────────────
info "Collecting bandwidth stats..."

IFACE=$(ip route | grep default | awk '{print $5}' | head -1)
VNSTAT_JSON=$(vnstat -i "$IFACE" --json 2>/dev/null || echo '{}')

# Total RX/TX from /proc/net/dev (always available)
RX_BYTES=$(awk -v iface="$IFACE" '$1 ~ iface {gsub(/:/, "", $1); print $2}' /proc/net/dev 2>/dev/null || echo 0)
TX_BYTES=$(awk -v iface="$IFACE" '$1 ~ iface {gsub(/:/, "", $1); print $10}' /proc/net/dev 2>/dev/null || echo 0)

rx_human() {
  local b=$1
  if   [[ $b -gt 1073741824 ]]; then echo "$(echo "scale=1; $b/1073741824" | bc) GB"
  elif [[ $b -gt 1048576 ]];    then echo "$(echo "scale=1; $b/1048576"    | bc) MB"
  else echo "$(echo "scale=0; $b/1024" | bc) KB"
  fi
}
RX_HUMAN=$(rx_human "$RX_BYTES")
TX_HUMAN=$(rx_human "$TX_BYTES")

success "Interface $IFACE — Received: $RX_HUMAN / Sent: $TX_HUMAN (since last reboot)"

# ── 2. Parse Xray access log ──────────────────────────────────────────────────
info "Parsing Xray access log (last ${HOURS}h)..."

if [[ ! -f "$XRAY_LOG" ]]; then
  info "Access log not found at $XRAY_LOG — enabling it..."
  mkdir -p /var/log/xray
  # Add access log to xray config if missing
  python3 - <<'PYEOF'
import json, sys
with open('/etc/xray/config.json') as f:
    cfg = json.load(f)
if 'log' not in cfg:
    cfg['log'] = {}
cfg['log']['access'] = '/var/log/xray/access.log'
cfg['log']['loglevel'] = 'info'
with open('/etc/xray/config.json', 'w') as f:
    json.dump(cfg, f, indent=2)
print("Updated xray config to enable access log")
PYEOF
  systemctl restart xray
  sleep 2
fi

# Calculate cutoff timestamp
CUTOFF=$(date -d "${HOURS} hours ago" '+%Y/%m/%d %H:%M:%S' 2>/dev/null || \
         date -v-${HOURS}H '+%Y/%m/%d %H:%M:%S' 2>/dev/null || \
         echo "2000/01/01 00:00:00")

# Extract destination hosts from log lines within the time window
# Xray log format: 2026/05/10 13:00:00 [Info] [id] accepted tcp:host:port [inbound >> outbound]
DESTINATIONS_RAW=$(grep -a "accepted" "$XRAY_LOG" 2>/dev/null | \
  awk -v cutoff="$CUTOFF" '
    {
      log_time = $1 " " $2
      if (log_time >= cutoff) {
        # Extract host:port from "accepted tcp:host:port" or "accepted udp:host:port"
        for (i=1; i<=NF; i++) {
          if ($i ~ /^accepted$/) {
            proto_host = $(i+1)
            # Remove protocol prefix (tcp: or udp:)
            gsub(/^[a-z]+:/, "", proto_host)
            # Remove port number
            gsub(/:[0-9]+$/, "", proto_host)
            # Remove IPv6 brackets
            gsub(/^\[|\]$/, "", proto_host)
            print proto_host
          }
        }
      }
    }
  ' | sort | uniq -c | sort -rn || echo "")

TOTAL_CONNECTIONS=$(echo "$DESTINATIONS_RAW" | awk '{sum+=$1} END {print sum+0}')
UNIQUE_DESTINATIONS=$(echo "$DESTINATIONS_RAW" | grep -c "." || echo 0)

success "Total connections: $TOTAL_CONNECTIONS across $UNIQUE_DESTINATIONS unique destinations"

# ── 3. Map domains to app names ───────────────────────────────────────────────
map_app() {
  local host="$1"
  case "$host" in
    *youtube*|*googlevideo*|*ytimg*|*yt3.ggpht*)       echo "YouTube" ;;
    *netflix*|*nflxvideo*|*nflximg*)                   echo "Netflix" ;;
    *spotify*|*scdn.co*|*spotifycdn*)                  echo "Spotify" ;;
    *instagram*|*cdninstagram*|*fbcdn*)                echo "Instagram" ;;
    *facebook*|*fb.com*|*fbsbx*)                       echo "Facebook" ;;
    *twitter*|*twimg*|*t.co*|*x.com*)                  echo "X/Twitter" ;;
    *tiktok*|*tiktokcdn*|*muscdn*)                     echo "TikTok" ;;
    *discord*|*discordapp*)                            echo "Discord" ;;
    *telegram*|*t.me*)                                 echo "Telegram" ;;
    *whatsapp*|*wa.me*)                                echo "WhatsApp" ;;
    *reddit*|*redd.it*|*redditmedia*)                  echo "Reddit" ;;
    *google.com*|*googleapis*|*googleusercontent*|*gstatic*) echo "Google" ;;
    *apple.com*|*icloud*|*mzstatic*|*apple-dns*)       echo "Apple/iCloud" ;;
    *microsoft*|*windows*|*xbox*|*msftconnecttest*)    echo "Microsoft" ;;
    *github*|*githubusercontent*)                       echo "GitHub" ;;
    *openai*|*chatgpt*)                                echo "ChatGPT" ;;
    *amazon*|*amazonaws*|*cloudfront*)                 echo "Amazon/AWS" ;;
    *cloudflare*)                                      echo "Cloudflare" ;;
    *akamai*|*akamaitechnologies*)                     echo "Akamai CDN" ;;
    *wikipedia*)                                       echo "Wikipedia" ;;
    *twitch*|*twitchsvc*)                              echo "Twitch" ;;
    *dropbox*)                                         echo "Dropbox" ;;
    *zoom*|*zoomgov*)                                  echo "Zoom" ;;
    *slack*)                                           echo "Slack" ;;
    *line.me*|*line-scdn*)                             echo "LINE" ;;
    *kakao*)                                           echo "KakaoTalk" ;;
    *) echo "Other" ;;
  esac
}

# Build top destinations list
TOP_DESTINATIONS=$(echo "$DESTINATIONS_RAW" | head -50 | while read count host; do
  app=$(map_app "$host")
  echo "$count $app $host"
done)

# Print terminal summary
echo ""
echo -e "${BOLD}Top Destinations (last ${HOURS}h):${NC}"
echo ""
printf "  %-8s %-18s %s\n" "Conns" "App" "Host"
echo "  ────────────────────────────────────────────────"
echo "$TOP_DESTINATIONS" | head -20 | while read count app host; do
  printf "  %-8s %-18s %s\n" "$count" "$app" "$host"
done
echo ""

# Aggregate by app name
APP_TOTALS=$(echo "$TOP_DESTINATIONS" | awk '{app[$2]+=$1} END {for (a in app) print app[a], a}' | sort -rn | head -15)

echo -e "${BOLD}Connections by App:${NC}"
echo ""
MAX_COUNT=$(echo "$APP_TOTALS" | head -1 | awk '{print $1}')
echo "$APP_TOTALS" | while read count app; do
  bar_len=$(( count * 30 / (MAX_COUNT + 1) ))
  bar=$(printf '%0.s█' $(seq 1 $bar_len))
  printf "  %-18s %s %-6s\n" "$app" "$bar" "($count)"
done
echo ""

# ── 4. Hourly connection breakdown ───────────────────────────────────────────
info "Building hourly breakdown..."

HOURLY_DATA=$(grep -a "accepted" "$XRAY_LOG" 2>/dev/null | \
  awk -v cutoff="$CUTOFF" '
    {
      log_time = $1 " " $2
      if (log_time >= cutoff) {
        hour = $1 " " substr($2, 1, 2) ":00"
        counts[hour]++
      }
    }
    END { for (h in counts) print h, counts[h] }
  ' | sort || echo "")

# ── 5. Xray stats API (bytes transferred) ────────────────────────────────────
info "Querying Xray stats API..."
STATS_OUTPUT=""
if command -v xray &>/dev/null; then
  STATS_OUTPUT=$(xray api statsquery --server=127.0.0.1:10085 -pattern "" 2>/dev/null | \
    python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    stats = data.get('stat', [])
    for s in stats:
        name = s.get('name', '')
        value = int(s.get('value', 0))
        if value > 0:
            if value > 1073741824:
                human = f'{value/1073741824:.1f} GB'
            elif value > 1048576:
                human = f'{value/1048576:.1f} MB'
            else:
                human = f'{value/1024:.0f} KB'
            print(f'{name}: {human}')
except:
    pass
" 2>/dev/null || echo "")
fi

# ── 6. Generate HTML report ───────────────────────────────────────────────────
info "Generating HTML report → $OUT"

# Build JS arrays for Chart.js
APP_LABELS=$(echo "$APP_TOTALS" | awk '{printf "\"%s\",", $2}' | sed 's/,$//')
APP_DATA=$(echo "$APP_TOTALS"   | awk '{printf "%s,", $1}'     | sed 's/,$//')

HOURLY_LABELS=$(echo "$HOURLY_DATA" | awk '{printf "\"%s\",", $1" "$2}' | sed 's/,$//')
HOURLY_DATA_JS=$(echo "$HOURLY_DATA" | awk '{printf "%s,", $3}' | sed 's/,$//')

# Top destinations table rows
TOP_ROWS=$(echo "$TOP_DESTINATIONS" | head -30 | while read count app host; do
  echo "<tr><td>$count</td><td><span class=\"app-badge\">$app</span></td><td class=\"host\">$host</td></tr>"
done)

REPORT_DATE=$(date '+%Y-%m-%d %H:%M:%S UTC')
SERVER_IP=$(curl -4 -s --max-time 3 ifconfig.me 2>/dev/null || echo "unknown")

cat > "$OUT" <<HTMLEOF
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>VPN Traffic Report</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
         background: #0f1117; color: #e2e8f0; min-height: 100vh; }
  .header { background: linear-gradient(135deg, #1e3a5f, #0f2744);
            padding: 32px 40px; border-bottom: 1px solid #1e3a5f; }
  .header h1 { font-size: 24px; font-weight: 700; color: #60a5fa; }
  .header p  { color: #94a3b8; margin-top: 6px; font-size: 14px; }
  .container { max-width: 1200px; margin: 0 auto; padding: 32px 24px; }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
           gap: 16px; margin-bottom: 32px; }
  .card { background: #1e2433; border: 1px solid #2d3748; border-radius: 12px;
          padding: 20px 24px; }
  .card .label { font-size: 12px; color: #64748b; text-transform: uppercase;
                 letter-spacing: 0.05em; margin-bottom: 8px; }
  .card .value { font-size: 28px; font-weight: 700; color: #60a5fa; }
  .card .sub   { font-size: 12px; color: #64748b; margin-top: 4px; }
  .grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; margin-bottom: 32px; }
  @media (max-width: 768px) { .grid2 { grid-template-columns: 1fr; } }
  .chart-box { background: #1e2433; border: 1px solid #2d3748; border-radius: 12px;
               padding: 24px; }
  .chart-box h2 { font-size: 15px; font-weight: 600; color: #94a3b8;
                  margin-bottom: 20px; text-transform: uppercase; letter-spacing: 0.05em; }
  .chart-wrap { position: relative; height: 280px; }
  .table-box { background: #1e2433; border: 1px solid #2d3748; border-radius: 12px;
               padding: 24px; margin-bottom: 32px; overflow-x: auto; }
  .table-box h2 { font-size: 15px; font-weight: 600; color: #94a3b8;
                  margin-bottom: 20px; text-transform: uppercase; letter-spacing: 0.05em; }
  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  th { text-align: left; padding: 10px 16px; color: #64748b; font-weight: 500;
       font-size: 12px; text-transform: uppercase; border-bottom: 1px solid #2d3748; }
  td { padding: 10px 16px; border-bottom: 1px solid #1a2030; }
  tr:hover td { background: #242b3d; }
  td.host { font-family: monospace; font-size: 13px; color: #94a3b8; }
  .app-badge { background: #1e3a5f; color: #60a5fa; padding: 2px 10px;
               border-radius: 12px; font-size: 12px; font-weight: 500; white-space: nowrap; }
  .footer { text-align: center; padding: 24px; color: #374151; font-size: 12px; }
</style>
</head>
<body>

<div class="header">
  <h1>VPN Traffic Report</h1>
  <p>Server: $SERVER_IP &nbsp;·&nbsp; Period: last ${HOURS} hours &nbsp;·&nbsp; Generated: $REPORT_DATE</p>
</div>

<div class="container">

  <!-- Summary cards -->
  <div class="cards">
    <div class="card">
      <div class="label">Total Connections</div>
      <div class="value">$TOTAL_CONNECTIONS</div>
      <div class="sub">last ${HOURS} hours</div>
    </div>
    <div class="card">
      <div class="label">Unique Destinations</div>
      <div class="value">$UNIQUE_DESTINATIONS</div>
      <div class="sub">hosts contacted</div>
    </div>
    <div class="card">
      <div class="label">Data Received</div>
      <div class="value">$RX_HUMAN</div>
      <div class="sub">since last reboot</div>
    </div>
    <div class="card">
      <div class="label">Data Sent</div>
      <div class="value">$TX_HUMAN</div>
      <div class="sub">since last reboot</div>
    </div>
  </div>

  <!-- Charts -->
  <div class="grid2">
    <div class="chart-box">
      <h2>Connections by App</h2>
      <div class="chart-wrap">
        <canvas id="appChart"></canvas>
      </div>
    </div>
    <div class="chart-box">
      <h2>Connections by Hour</h2>
      <div class="chart-wrap">
        <canvas id="hourlyChart"></canvas>
      </div>
    </div>
  </div>

  <!-- Top destinations table -->
  <div class="table-box">
    <h2>Top Destinations</h2>
    <table>
      <thead>
        <tr><th>Connections</th><th>App</th><th>Host</th></tr>
      </thead>
      <tbody>
        $TOP_ROWS
      </tbody>
    </table>
  </div>

</div>

<div class="footer">Generated by VPN Traffic Analyzer · ghost-node</div>

<script>
const palette = [
  '#60a5fa','#34d399','#f59e0b','#f87171','#a78bfa',
  '#fb923c','#38bdf8','#4ade80','#facc15','#e879f9',
  '#2dd4bf','#fb7185','#818cf8','#a3e635','#fbbf24'
];

// App connections doughnut
new Chart(document.getElementById('appChart'), {
  type: 'doughnut',
  data: {
    labels: [$APP_LABELS],
    datasets: [{
      data: [$APP_DATA],
      backgroundColor: palette,
      borderColor: '#0f1117',
      borderWidth: 2,
    }]
  },
  options: {
    responsive: true, maintainAspectRatio: false,
    plugins: {
      legend: { position: 'right', labels: { color: '#94a3b8', font: { size: 12 }, padding: 12 } }
    }
  }
});

// Hourly line chart
new Chart(document.getElementById('hourlyChart'), {
  type: 'line',
  data: {
    labels: [$HOURLY_LABELS],
    datasets: [{
      label: 'Connections',
      data: [$HOURLY_DATA_JS],
      borderColor: '#60a5fa',
      backgroundColor: 'rgba(96,165,250,0.15)',
      borderWidth: 2,
      pointRadius: 3,
      fill: true,
      tension: 0.3,
    }]
  },
  options: {
    responsive: true, maintainAspectRatio: false,
    plugins: { legend: { display: false } },
    scales: {
      x: { ticks: { color: '#64748b', maxTicksLimit: 8 }, grid: { color: '#1e2433' } },
      y: { ticks: { color: '#64748b' }, grid: { color: '#1e2433' }, beginAtZero: true }
    }
  }
});
</script>
</body>
</html>
HTMLEOF

success "Report saved to $OUT"
echo ""
echo -e "${BOLD}  To view on your Mac:${NC}"
echo -e "  scp ghost-node:$OUT ~/Desktop/vpn-report.html && open ~/Desktop/vpn-report.html"
echo ""
