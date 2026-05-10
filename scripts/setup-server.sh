#!/usr/bin/env bash
# setup-server.sh — Deploy Xray + VPN node-agent on a fresh Ubuntu/Debian VPS.
#
# Usage (run AS ROOT on the remote server):
#   curl -sSL https://raw.githubusercontent.com/your-org/vpn/main/scripts/setup-server.sh | \
#     CONTROL_PLANE=https://api.yourdomain.com \
#     ADMIN_TOKEN=<your-admin-jwt> \
#     bash
#
# Or interactively:
#   scp scripts/setup-server.sh root@YOUR_SERVER_IP:/tmp/
#   ssh root@YOUR_SERVER_IP "CONTROL_PLANE=... ADMIN_TOKEN=... bash /tmp/setup-server.sh"

set -euo pipefail

# ── Config (override via env) ──────────────────────────────────────────────
CONTROL_PLANE="${CONTROL_PLANE:-http://localhost:8080}"
ADMIN_TOKEN="${ADMIN_TOKEN:-}"
XRAY_VERSION="${XRAY_VERSION:-1.8.11}"
AGENT_VERSION="${AGENT_VERSION:-latest}"
VLESS_PORT="${VLESS_PORT:-443}"
CAMOUFLAGE_DOMAIN="${CAMOUFLAGE_DOMAIN:-www.microsoft.com}"  # impersonated domain for REALITY

# ── Colours ────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; RED='\033[0;31m'; NC='\033[0m'; BOLD='\033[1m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()     { echo -e "${RED}[FAIL]${NC}  $*" >&2; exit 1; }

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}   VPN Platform — Server Setup${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# ── Prereqs ────────────────────────────────────────────────────────────────
[[ $(id -u) -eq 0 ]] || die "Run as root"
command -v curl  >/dev/null || die "curl required"
command -v unzip >/dev/null 2>&1 || apt-get install -y unzip -qq

# Detect public IP
SERVER_IP=$(curl -4 -s --max-time 5 ifconfig.me || curl -4 -s --max-time 5 api.ipify.org || hostname -I | awk '{print $1}')
info "Server public IP: $SERVER_IP"

# ── 1. System hardening ────────────────────────────────────────────────────
info "Updating system packages..."
apt-get update -qq && apt-get upgrade -y -qq

info "Enabling BBR congestion control..."
if ! grep -q "net.core.default_qdisc=fq" /etc/sysctl.conf; then
  echo "net.core.default_qdisc=fq"         >> /etc/sysctl.conf
  echo "net.ipv4.tcp_congestion_control=bbr" >> /etc/sysctl.conf
  sysctl -p -q
fi

info "Configuring UFW firewall..."
apt-get install -y -qq ufw
ufw --force reset
ufw default deny incoming
ufw default allow outgoing
ufw allow ssh
ufw allow "$VLESS_PORT"/tcp   # VLESS+REALITY
ufw allow 8443/tcp            # VLESS+WS+TLS (CDN mode)
ufw allow 443/udp             # Hysteria2
ufw --force enable
success "Firewall configured (ports: 22, $VLESS_PORT/tcp, 8443/tcp, 443/udp)"

# ── 2. Install Xray ────────────────────────────────────────────────────────
XRAY_DIR="/usr/local/xray"
info "Installing Xray $XRAY_VERSION..."
mkdir -p "$XRAY_DIR"
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) XA="64" ;;
  aarch64) XA="arm64-v8a" ;;
  *) die "Unsupported arch: $ARCH" ;;
esac
XRAY_URL="https://github.com/XTLS/Xray-core/releases/download/v${XRAY_VERSION}/Xray-linux-${XA}.zip"
curl -sL "$XRAY_URL" -o /tmp/xray.zip
unzip -q -o /tmp/xray.zip -d "$XRAY_DIR"
chmod +x "$XRAY_DIR/xray"
ln -sf "$XRAY_DIR/xray" /usr/local/bin/xray
success "Xray installed: $(xray version | head -1)"

# ── 3. Generate REALITY keys ───────────────────────────────────────────────
info "Generating REALITY key pair..."
KEY_OUTPUT=$(xray x25519)
REALITY_PRIVATE_KEY=$(echo "$KEY_OUTPUT" | grep "Private key:" | awk '{print $3}')
REALITY_PUBLIC_KEY=$(echo  "$KEY_OUTPUT" | grep "Public key:"  | awk '{print $3}')
REALITY_SHORT_ID=$(openssl rand -hex 8)
VLESS_UUID=$(xray uuid)

success "REALITY private key: ${REALITY_PRIVATE_KEY:0:10}..."
success "REALITY public key:  ${REALITY_PUBLIC_KEY:0:10}..."
success "VLESS UUID:          $VLESS_UUID"
success "Short ID:            $REALITY_SHORT_ID"

# ── 4. Write Xray config: VLESS+REALITY (primary) ─────────────────────────
XRAY_CONFIG_DIR="/etc/xray"
mkdir -p "$XRAY_CONFIG_DIR"

info "Writing Xray REALITY config..."
cat > "$XRAY_CONFIG_DIR/config.json" <<EOF
{
  "log": {
    "loglevel": "warning",
    "access": "/var/log/xray/access.log",
    "error":  "/var/log/xray/error.log"
  },
  "inbounds": [
    {
      "listen": "0.0.0.0",
      "port": $VLESS_PORT,
      "protocol": "vless",
      "settings": {
        "clients": [
          {
            "id": "$VLESS_UUID",
            "flow": "xtls-rprx-vision"
          }
        ],
        "decryption": "none"
      },
      "streamSettings": {
        "network": "tcp",
        "security": "reality",
        "realitySettings": {
          "dest": "$CAMOUFLAGE_DOMAIN:443",
          "serverNames": ["$CAMOUFLAGE_DOMAIN"],
          "privateKey": "$REALITY_PRIVATE_KEY",
          "shortIds": ["$REALITY_SHORT_ID"],
          "fingerprint": "chrome"
        }
      },
      "sniffing": {
        "enabled": true,
        "destOverride": ["http", "tls", "quic"]
      }
    },
    {
      "listen": "127.0.0.1",
      "port": 10085,
      "protocol": "dokodemo-door",
      "settings": { "address": "127.0.0.1" },
      "tag": "api"
    }
  ],
  "outbounds": [
    { "protocol": "freedom", "tag": "direct" },
    { "protocol": "blackhole", "tag": "block" }
  ],
  "routing": {
    "domainStrategy": "IPIfNonMatch",
    "rules": [
      { "type": "field", "ip": ["geoip:private"],     "outboundTag": "block"  },
      { "type": "field", "ip": ["geoip:cn"],           "outboundTag": "direct" },
      { "type": "field", "domain": ["geosite:cn"],     "outboundTag": "direct" }
    ]
  },
  "api": {
    "tag": "api",
    "services": ["HandlerService", "StatsService", "LoggerService"]
  },
  "stats": {},
  "policy": {
    "levels": { "0": { "statsUserUplink": true, "statsUserDownlink": true } },
    "system": { "statsInboundUplink": true, "statsInboundDownlink": true }
  }
}
EOF
mkdir -p /var/log/xray
success "Xray config written to $XRAY_CONFIG_DIR/config.json"

# ── 5. Create Xray systemd service ────────────────────────────────────────
cat > /etc/systemd/system/xray.service <<EOF
[Unit]
Description=Xray Service
After=network.target

[Service]
User=root
ExecStart=/usr/local/bin/xray run -c /etc/xray/config.json
Restart=on-failure
RestartSec=5s
LimitNOFILE=1048576
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now xray
sleep 2
if systemctl is-active --quiet xray; then
  success "Xray service running"
else
  die "Xray failed to start. Check: journalctl -u xray -n 50"
fi

# ── 6. Save credentials to a local file ───────────────────────────────────
CREDS_FILE="/root/vpn-server-credentials.env"
cat > "$CREDS_FILE" <<EOF
# VPN Server Credentials — generated $(date -u)
SERVER_IP=$SERVER_IP
VLESS_PORT=$VLESS_PORT
VLESS_UUID=$VLESS_UUID
REALITY_PUBLIC_KEY=$REALITY_PUBLIC_KEY
REALITY_PRIVATE_KEY=$REALITY_PRIVATE_KEY
REALITY_SHORT_ID=$REALITY_SHORT_ID
CAMOUFLAGE_DOMAIN=$CAMOUFLAGE_DOMAIN
EOF
chmod 600 "$CREDS_FILE"
success "Credentials saved to $CREDS_FILE"

# ── 7. Register node with control plane ───────────────────────────────────
if [[ -n "$ADMIN_TOKEN" && -n "$CONTROL_PLANE" ]]; then
  info "Registering node with control plane at $CONTROL_PLANE..."
  REGION=$(curl -s --max-time 5 "https://ipapi.co/$SERVER_IP/region/" 2>/dev/null || echo "unknown")
  COUNTRY=$(curl -s --max-time 5 "https://ipapi.co/$SERVER_IP/country/" 2>/dev/null || echo "unknown")
  HOSTNAME=$(hostname)

  NODE_RESP=$(curl -s -w "\n%{http_code}" -X POST "$CONTROL_PLANE/api/v1/admin/nodes" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$HOSTNAME\",\"address\":\"$SERVER_IP\",\"region\":\"$REGION\",\"country\":\"$COUNTRY\"}")
  HTTP_CODE=$(echo "$NODE_RESP" | tail -1)
  NODE_BODY=$(echo "$NODE_RESP" | sed '$d')

  if [[ "$HTTP_CODE" == "201" ]]; then
    NODE_ID=$(echo "$NODE_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['node']['id'])" 2>/dev/null || echo "")
    echo "NODE_ID=$NODE_ID" >> "$CREDS_FILE"
    success "Node registered: ID=$NODE_ID"

    # Register the transport profile
    curl -s -X POST "$CONTROL_PLANE/api/v1/admin/nodes/$NODE_ID/transports" \
      -H "Authorization: Bearer $ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -d "{
        \"type\": \"xray\",
        \"port\": $VLESS_PORT,
        \"priority\": 10,
        \"config\": {
          \"protocol\": \"vless\",
          \"transport\": \"reality\",
          \"uuid\": \"$VLESS_UUID\",
          \"public_key\": \"$REALITY_PUBLIC_KEY\",
          \"short_id\": \"$REALITY_SHORT_ID\",
          \"server_name\": \"$CAMOUFLAGE_DOMAIN\",
          \"flow\": \"xtls-rprx-vision\"
        }
      }" > /dev/null
    success "Transport profile registered"
  else
    warn "Node registration returned HTTP $HTTP_CODE — register manually via admin API"
  fi
else
  warn "ADMIN_TOKEN not set — skipping control-plane registration"
  warn "Register manually: POST $CONTROL_PLANE/api/v1/admin/nodes"
fi

# ── 8. Print client connection info ───────────────────────────────────────
VLESS_URI="vless://${VLESS_UUID}@${SERVER_IP}:${VLESS_PORT}?encryption=none&flow=xtls-rprx-vision&security=reality&sni=${CAMOUFLAGE_DOMAIN}&fp=chrome&pbk=${REALITY_PUBLIC_KEY}&sid=${REALITY_SHORT_ID}&type=tcp&headerType=none#VPNPlatform-$(hostname)"

echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}${BOLD}  SERVER SETUP COMPLETE${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "  ${BOLD}Server IP:${NC}        $SERVER_IP"
echo -e "  ${BOLD}Protocol:${NC}         VLESS + REALITY (port $VLESS_PORT)"
echo -e "  ${BOLD}Camouflage:${NC}       $CAMOUFLAGE_DOMAIN"
echo ""
echo -e "  ${BOLD}VLESS URI (import into any client):${NC}"
echo ""
echo -e "  ${CYAN}$VLESS_URI${NC}"
echo ""
echo -e "  ${BOLD}Credentials file:${NC} $CREDS_FILE"
echo ""
echo -e "  ${YELLOW}Import the VLESS URI above into:${NC}"
echo -e "  • iOS:     Shadowrocket / Quantumult X / Stash"
echo -e "  • Android: v2rayNG / Clash Meta for Android"
echo -e "  • macOS:   V2RayXS / NekoRay / Clash Verge"
echo -e "  • Windows: NekoRay / Hiddify / v2rayN"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
