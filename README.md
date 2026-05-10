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

---

## Scripts

All day-to-day operations are handled by scripts in the `scripts/` folder.

### `setup-server.sh` — Deploy VPN on a VPS

Run once on a fresh Ubuntu/Debian server (as root):

```bash
# Copy to server and run
scp scripts/setup-server.sh user@YOUR_SERVER:/tmp/
ssh user@YOUR_SERVER
sudo bash /tmp/setup-server.sh
```

What it does:
- Installs Xray-core, configures VLESS+REALITY on port 443
- Generates UUID, X25519 key pair, short ID
- Sets up systemd service (auto-starts on reboot)
- Opens firewall ports
- Saves credentials to `/root/vpn-server-credentials.env`
- Prints a `vless://` URI to import into any client app

---

### `manage-server.sh` — Day-to-day server management

Copy to server once, then run any command:

```bash
scp scripts/manage-server.sh oracle-vpn:/tmp/manage-server.sh
ssh oracle-vpn
```

| Command | What it does |
|---------|-------------|
| `sudo bash manage-server.sh status` | Check Xray is running, port is open, firewall is correct |
| `sudo bash manage-server.sh restart` | Restart Xray (use when VPN stops working) |
| `sudo bash manage-server.sh credentials` | Print your VLESS URI and all credentials |
| `sudo bash manage-server.sh fix-fw` | Re-apply iptables rules (run after server reboot) |
| `sudo bash manage-server.sh save-fw` | Persist firewall rules across reboots |
| `sudo bash manage-server.sh logs` | Show last 50 lines of Xray logs |
| `sudo bash manage-server.sh update` | Update Xray to latest version |

---

### `setup-mac-vpn.sh` — Connect your Mac to the VPN

Runs on your Mac. Automatically fetches credentials from the server, starts an Xray client in Docker, and configures the system proxy.

Requires: Docker Desktop running on your Mac, SSH access to the server configured in `~/.ssh/config`.

```bash
bash scripts/setup-mac-vpn.sh start    # connect — sets system proxy automatically
bash scripts/setup-mac-vpn.sh stop     # disconnect
bash scripts/setup-mac-vpn.sh status   # show exit IP and tunnel state
```

Verify it's working: open `https://ip.sb` in your browser — it should show your server's IP.

> **Note:** All browser and app traffic on your Mac routes through the VPN tunnel while connected.

---

### `view-vpn-report.sh` — Traffic analysis report

Runs on your Mac. SSHes into the server, analyzes Xray access logs, and opens an HTML report in your browser showing which apps used the VPN and how much.

```bash
bash scripts/view-vpn-report.sh              # analyze last 24 hours
bash scripts/view-vpn-report.sh --hours 48   # analyze last 48 hours
```

The HTML report includes:
- Total connections and unique destinations
- Doughnut chart — connections by app (YouTube, Google, Instagram, etc.)
- Line chart — connections per hour
- Full destination table with inferred app names

---

### `test-tunnel.sh` — Local tunnel test (no VPS needed)

Verifies the full VLESS+WebSocket tunnel stack works using Docker only. Useful for local development.

```bash
bash scripts/test-tunnel.sh
```

Starts an Xray server + client in Docker, routes traffic through the tunnel, and confirms the exit IP differs from the direct connection.

---

### `gen-client-config.sh` — Generate client import configs

Downloads subscription configs from the control plane API for import into client apps.

```bash
./scripts/gen-client-config.sh \
  --api   http://localhost:8080 \
  --token YOUR_JWT_TOKEN \
  --node  YOUR_NODE_UUID \
  --out   ./client-configs
```

Outputs `vless.txt`, `clash.yaml`, `singbox.json` and QR codes (requires `qrencode`).

---

### `verify.sh` — API end-to-end test suite

Runs 30 checks against the control plane API. Use after local `docker compose up`.

```bash
bash scripts/verify.sh
```

---

## Quick Start — Personal VPN

### 1. Provision a server

Any Ubuntu 22.04 VPS works. Oracle Cloud Always Free (Tokyo/Singapore) is recommended.

```bash
scp scripts/setup-server.sh oracle-vpn:/tmp/
ssh oracle-vpn
sudo bash /tmp/setup-server.sh
```

### 2. Save firewall rules (do once after setup)

```bash
ssh oracle-vpn
sudo bash /tmp/manage-server.sh save-fw
```

### 3. Connect your Mac

```bash
bash scripts/setup-mac-vpn.sh start
```

### 4. Connect your phone

Import the `vless://` URI from `manage-server.sh credentials` into:

| Platform | App |
|----------|-----|
| Android | v2RayTun / v2rayNG / Hiddify (Play Store) |
| iOS | Shadowrocket (App Store, $2.99) |

---

## Local Development

Requires: Docker Desktop

```bash
# Copy environment file and start all services
cp .env.example .env
docker compose up -d

# Services:
#   Control plane API  → http://localhost:8080
#   Grafana dashboard  → http://localhost:3001  (admin / admin)
#   Prometheus         → http://localhost:9092
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

Subscription formats: `all` (JSON) · `vless` (base64 for v2rayN) · `clash` (Clash Meta YAML) · `singbox` (sing-box JSON)

### Admin

```bash
# Create a node
curl -X POST http://localhost:8080/api/v1/admin/nodes \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"JP-01","address":"1.2.3.4","region":"Japan","country":"JP"}'

# Add a VLESS+REALITY transport profile
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

## Server Configs

Pre-built Xray server config templates in `configs/`:

| File | Protocol | Use case |
|------|----------|----------|
| `xray-server-reality.json` | VLESS+REALITY | Best GFW bypass, no cert needed |
| `xray-server-ws-tls.json` | VLESS+WS+TLS + gRPC+TLS | CDN-compatible (Cloudflare) |
| `hysteria2-server.yaml` | Hysteria2 (QUIC) | High bandwidth |

See `docs/china-setup-guide.md` for full GFW bypass instructions and `docs/server-management.md` for server operations.

---

## Project Structure

```
.
├── cmd/
│   ├── control-plane/        # Main API server
│   └── node-agent/           # Agent that runs on VPN servers
├── internal/
│   ├── auth/                 # JWT + middleware
│   ├── handler/              # HTTP handlers (gin)
│   ├── service/              # Business logic
│   ├── repository/           # Database layer (GORM)
│   ├── models/               # DB models
│   ├── transport/            # Xray/Hysteria2 process management
│   ├── metrics/              # Prometheus instrumentation
│   └── agent/                # Node agent logic
├── configs/                  # Xray + Hysteria2 server config templates
├── deployments/
│   ├── docker/               # Dockerfiles + Air hot-reload configs
│   └── monitoring/           # Prometheus + Grafana provisioning
├── scripts/
│   ├── setup-server.sh       # Deploy Xray on a VPS
│   ├── manage-server.sh      # Server status, restart, credentials, firewall
│   ├── setup-mac-vpn.sh      # Connect Mac to VPN via Docker + system proxy
│   ├── view-vpn-report.sh    # Pull traffic report from server, open in browser
│   ├── analyze-vpn-traffic.sh# Run on server: parse logs, generate HTML report
│   ├── gen-client-config.sh  # Download subscription configs from control plane
│   ├── test-tunnel.sh        # Local tunnel verification (no VPS needed)
│   └── verify.sh             # API end-to-end test suite
├── docs/
│   ├── china-setup-guide.md  # GFW bypass guide
│   └── server-management.md  # Server operations reference
├── api/proto/                # gRPC protobuf definitions
└── docker-compose.yml
```

---

## Monitoring

- **Grafana**: http://localhost:3001 (admin / admin) — 12-panel dashboard: request rate, latency, auth events, active sessions, bandwidth, memory
- **Prometheus**: http://localhost:9092

---

## Credentials

After running `setup-server.sh`, all credentials are saved on the server at `/root/vpn-server-credentials.env` (chmod 600). Retrieve them any time:

```bash
ssh oracle-vpn
sudo bash /tmp/manage-server.sh credentials
```
