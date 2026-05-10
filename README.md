# VPN Platform

A censorship-resistant VPN platform built on [Xray-core](https://github.com/XTLS/Xray-core), supporting VLESS+REALITY, VLESS+WebSocket+TLS, and Hysteria2. Includes a Go control plane, node management API, Prometheus/Grafana monitoring, and client subscription endpoints.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  Client Apps (Shadowrocket / v2rayNG / NekoRay)     │
└───────────────────┬─────────────────────────────────┘
                    │ VLESS+REALITY / WS+TLS / Hysteria2
┌───────────────────▼─────────────────────────────────┐
│  Oracle/VPS Server                                  │
│  ├── Xray-core  (protocol engine)                   │
│  └── Node Agent (reports to control plane)          │
└───────────────────┬─────────────────────────────────┘
                    │ gRPC
┌───────────────────▼─────────────────────────────────┐
│  Control Plane (Go API)                             │
│  ├── REST API  :8080                                │
│  ├── gRPC      :9090                                │
│  ├── Prometheus :9092                               │
│  └── Grafana   :3001                                │
└─────────────────────────────────────────────────────┘
```

## Quick Start — Deploy a VPN Server

### 1. Clone the repo (on your Oracle/VPS server)

```bash
git clone https://github.com/YOUR_USERNAME/YOUR_REPO.git
cd YOUR_REPO
```

### 2. Run the setup script

```bash
sudo bash scripts/setup-server.sh
```

The script will:
- Update system packages and enable BBR congestion control
- Configure UFW firewall (ports 22, 443/tcp, 8443/tcp, 443/udp)
- Download and install Xray-core
- Generate a VLESS UUID, X25519 key pair, and short ID
- Write `/etc/xray/config.json` (VLESS+REALITY on port 443)
- Create and start a systemd service
- Save credentials to `/root/vpn-server-credentials.env`
- Print a `vless://` URI you can import directly into any client app

### 3. Import the URI into a client app

Copy the `vless://` URI printed at the end and import it into:

| Platform | App |
|----------|-----|
| iOS | Shadowrocket (App Store, $2.99) |
| Android | v2rayNG (free, Play Store / GitHub) |
| macOS | NekoRay (free) or Clash Verge |
| Windows | v2rayN (free) or Hiddify |

---

## Local Development

Requires: Docker Desktop

```bash
# Start all services (API, database, Redis, Prometheus, Grafana)
docker compose up -d

# Services:
#   Control plane API  → http://localhost:8080
#   Grafana dashboard  → http://localhost:3001  (admin / admin)
#   Prometheus         → http://localhost:9092
```

### Test the tunnel locally (no VPS needed)

```bash
bash scripts/test-tunnel.sh
```

Starts an Xray server + client in Docker and verifies the full VLESS+WebSocket tunnel end-to-end.

### Run the full API test suite

```bash
bash scripts/verify.sh
```

---

## API Reference

### Auth

```bash
# Register
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"Pass123!","name":"Your Name"}'

# Login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"Pass123!"}'
```

### Nodes

```bash
# List available nodes (requires JWT)
curl http://localhost:8080/api/v1/nodes \
  -H "Authorization: Bearer YOUR_TOKEN"

# Get subscription configs for a node
curl "http://localhost:8080/api/v1/nodes/NODE_ID/subscription?format=all" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Subscription formats: `all` (JSON overview) · `vless` (base64 for v2rayN) · `clash` (Clash Meta YAML) · `singbox` (sing-box JSON)

### Admin

```bash
# Create a node
curl -X POST http://localhost:8080/api/v1/admin/nodes \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"JP-01","address":"1.2.3.4","region":"Japan","country":"JP"}'

# Add a transport profile (VLESS+REALITY)
curl -X POST http://localhost:8080/api/v1/admin/nodes/NODE_ID/transports \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "xray",
    "port": 443,
    "priority": 10,
    "config": {
      "protocol": "vless",
      "transport": "reality",
      "uuid": "YOUR_UUID",
      "public_key": "YOUR_PUBLIC_KEY",
      "short_id": "YOUR_SHORT_ID",
      "server_name": "www.microsoft.com",
      "flow": "xtls-rprx-vision"
    }
  }'
```

---

## Generate Client Configs

After deploying a server and registering it with the control plane:

```bash
./scripts/gen-client-config.sh \
  --api   http://localhost:8080 \
  --token YOUR_JWT_TOKEN \
  --node  YOUR_NODE_UUID \
  --out   ./client-configs
```

Outputs `vless.txt`, `clash.yaml`, `singbox.json` and QR codes (if `qrencode` is installed).

---

## Server Configs

Pre-built Xray server config templates are in `configs/`:

| File | Protocol | Use case |
|------|----------|----------|
| `xray-server-reality.json` | VLESS+REALITY | Best GFW bypass, no cert needed |
| `xray-server-ws-tls.json` | VLESS+WS+TLS + gRPC+TLS | CDN-compatible (Cloudflare) |
| `hysteria2-server.yaml` | Hysteria2 (QUIC) | High bandwidth |

See `docs/china-setup-guide.md` for full GFW bypass instructions.

---

## Project Structure

```
.
├── cmd/
│   ├── control-plane/    # Main API server
│   └── node-agent/       # Agent that runs on VPN servers
├── internal/
│   ├── auth/             # JWT + middleware
│   ├── handler/          # HTTP handlers (gin)
│   ├── service/          # Business logic
│   ├── repository/       # Database layer (GORM)
│   ├── models/           # DB models
│   ├── transport/        # Xray/Hysteria2 process management
│   ├── metrics/          # Prometheus instrumentation
│   └── agent/            # Node agent logic
├── configs/              # Xray + Hysteria2 server config templates
├── deployments/
│   ├── docker/           # Dockerfiles + Air hot-reload configs
│   └── monitoring/       # Prometheus + Grafana provisioning
├── scripts/
│   ├── setup-server.sh   # Full VPS deployment (run as root)
│   ├── gen-client-config.sh  # Download subscription configs
│   ├── test-tunnel.sh    # Local tunnel verification
│   └── verify.sh         # API end-to-end test suite
├── docs/
│   └── china-setup-guide.md  # GFW bypass guide
├── api/proto/            # gRPC protobuf definitions
└── docker-compose.yml
```

---

## Monitoring

- **Grafana**: http://localhost:3001 (admin / admin) — 12-panel dashboard: request rate, latency, auth events, active sessions, bandwidth, memory
- **Prometheus**: http://localhost:9092

---

## Credentials (after server setup)

All generated credentials are saved to `/root/vpn-server-credentials.env` on the server (chmod 600):

```
SERVER_IP=...
VLESS_UUID=...
REALITY_PUBLIC_KEY=...
REALITY_PRIVATE_KEY=...   # keep secret
REALITY_SHORT_ID=...
CAMOUFLAGE_DOMAIN=www.microsoft.com
```
