#!/usr/bin/env bash
# add-node.sh — Register a new VPN server node in the database.
# Usage: ./scripts/add-node.sh
# All values are prompted interactively; nothing is written to disk.
set -euo pipefail

DB_CONTAINER="${DB_CONTAINER:-vpn-postgres-1}"
DB_USER="${DB_USER:-vpnplatform}"
DB_NAME="${DB_NAME:-vpnplatform}"

psql_exec() {
  docker exec "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -t -c "$1"
}

echo ""
echo "=== Add VPN Node ==="
echo ""

read -rp "Node name (e.g. JP-02):          " NODE_NAME
read -rp "Server IP / hostname:             " NODE_ADDR
read -rp "Region label (e.g. Tokyo):        " NODE_REGION
read -rp "Country code (e.g. JP):           " NODE_COUNTRY
read -rp "Port [443]:                       " NODE_PORT
NODE_PORT="${NODE_PORT:-443}"

echo ""
echo "--- Xray / VLESS credentials ---"
read -rp "User UUID:                        " XRAY_UUID
read -rp "Public key (x25519):              " XRAY_PUBKEY
read -rp "Short ID:                         " XRAY_SHORT_ID
read -rp "SNI / server_name [www.microsoft.com]: " XRAY_SNI
XRAY_SNI="${XRAY_SNI:-www.microsoft.com}"
read -rp "Flow [xtls-rprx-vision]:          " XRAY_FLOW
XRAY_FLOW="${XRAY_FLOW:-xtls-rprx-vision}"

echo ""
echo "--- Confirm ---"
echo "  Name:       $NODE_NAME"
echo "  Address:    $NODE_ADDR"
echo "  Region:     $NODE_REGION ($NODE_COUNTRY)"
echo "  Port:       $NODE_PORT"
echo "  Protocol:   vless+reality"
echo "  SNI:        $XRAY_SNI"
echo ""
read -rp "Insert? [y/N]: " CONFIRM
[[ "$CONFIRM" =~ ^[Yy]$ ]] || { echo "Aborted."; exit 0; }

NODE_ID=$(psql_exec "
  INSERT INTO nodes (name, address, region, country, status, is_public, created_at, updated_at)
  VALUES (
    '$NODE_NAME', '$NODE_ADDR', '$NODE_REGION', '$NODE_COUNTRY',
    'online', true, NOW(), NOW()
  )
  RETURNING id;
" | tr -d '[:space:]')

echo "Created node: $NODE_ID"

CONFIG_JSON=$(printf '{"protocol":"vless","transport":"reality","uuid":"%s","public_key":"%s","short_id":"%s","server_name":"%s","flow":"%s"}' \
  "$XRAY_UUID" "$XRAY_PUBKEY" "$XRAY_SHORT_ID" "$XRAY_SNI" "$XRAY_FLOW")

psql_exec "
  INSERT INTO transport_profiles (node_id, type, port, config, is_active, priority, created_at, updated_at)
  VALUES (
    '$NODE_ID', 'xray', $NODE_PORT,
    '$CONFIG_JSON'::jsonb,
    true, 100, NOW(), NOW()
  );
" > /dev/null

echo "Created transport profile."
echo ""
echo "Done. Node '$NODE_NAME' is registered and visible in the portal."
