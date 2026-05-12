# VPN Platform

A censorship-resistant VPN platform built on [Xray-core](https://github.com/XTLS/Xray-core), supporting VLESS+REALITY, VLESS+WebSocket+TLS, and Hysteria2. Includes a Go control plane, node management API, Prometheus/Grafana monitoring, and client subscription endpoints.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  Web Portal  :1420  (React — account management)    │
│  Client Apps (Shadowrocket / v2rayNG / NekoRay)     │
└───────────────────┬─────────────────────────────────┘
                    │ REST API (portal) / VLESS+REALITY (tunnel)
┌───────────────────▼─────────────────────────────────┐
│  Control Plane (Go API)  :8080                      │
│  ├── REST API  :8080                                │
│  ├── gRPC      :9090                                │
│  ├── Prometheus :9092                               │
│  └── Grafana   :3001                                │
└───────────────────┬─────────────────────────────────┘
                    │ gRPC
┌───────────────────▼─────────────────────────────────┐
│  Oracle/VPS Server                                  │
│  ├── Xray-core  (protocol engine)                   │
│  └── Node Agent (reports to control plane)          │
└─────────────────────────────────────────────────────┘
```

---

## Scripts

All day-to-day operations are handled by scripts in the `scripts/` folder.

### `setup-server.sh` — Deploy VPN on a VPS

Run once on a fresh Ubuntu/Debian server (as root):

```bash
# Basic — manual registration afterward
scp scripts/setup-server.sh user@YOUR_SERVER:/tmp/
ssh user@YOUR_SERVER
sudo bash /tmp/setup-server.sh

# Auto-register with the control plane in one step
CONTROL_PLANE=https://vpn.yourdomain.com \
ADMIN_TOKEN=<admin-jwt> \
bash /tmp/setup-server.sh
```

What it does:
- Installs Xray-core, configures VLESS+REALITY on port 443
- Generates UUID, X25519 key pair, short ID
- Sets up systemd service (auto-starts on reboot)
- Opens firewall ports
- Saves credentials to `/root/vpn-server-credentials.env`
- Prints a `vless://` URI to import into any client app
- If `CONTROL_PLANE` + `ADMIN_TOKEN` are set: registers the node and transport profile with the control plane automatically — the server appears in the portal immediately

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

### `deploy-control-plane.sh` — Deploy the control plane on a public server

Run as root on a fresh Ubuntu 22.04 VPS to install Docker, nginx, TLS, and start the full stack:

```bash
DOMAIN=vpn.yourdomain.com \
EMAIL=you@example.com \
bash scripts/deploy-control-plane.sh
```

What it does:
- Installs Docker, nginx, certbot
- Clones the repo, generates `.env` with random JWT and DB secrets
- Starts `docker compose` (control plane, Postgres, Redis, Grafana, web portal)
- Obtains a Let's Encrypt TLS certificate for your domain
- Configures nginx to proxy HTTPS → API (:8080) and web portal (:1420)
- Sets up auto-renewal

After deployment, point `setup-server.sh` at the domain when adding VPN nodes:

```bash
CONTROL_PLANE=https://vpn.yourdomain.com \
ADMIN_TOKEN=<admin-jwt> \
bash scripts/setup-server.sh
```

---

### `add-node.sh` — Register a new server in the database

Run locally (requires Docker running). Prompts for all credentials interactively — nothing is written to disk or committed to git.

```bash
bash scripts/add-node.sh
```

You will be asked for: node name, IP, region, Xray UUID, X25519 public key, short ID, SNI, and port. The script inserts one row in `nodes` and one in `transport_profiles`. The new server appears in the web portal immediately.

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
#   Web portal         → http://localhost:1420  (register / login)
#   Control plane API  → http://localhost:8080
#   Grafana dashboard  → http://localhost:3001  (admin / admin)
#   Prometheus         → http://localhost:9092
```

The web portal is an account management UI. Register an account, then either:
- **Select a server** from the list and click the power button, or
- Click **"Auto-select best"** — the control plane scores all online nodes by CPU, memory, and active connections and picks the least-loaded one automatically

Both paths return a VLESS URI + QR code. Import into Shadowrocket, v2rayNG, or any compatible client — the portal itself does not establish a tunnel.

To register the first admin account, use the portal's Register page. To promote it to admin role:

```bash
docker exec vpn-postgres-1 psql -U vpnplatform -d vpnplatform \
  -c "UPDATE users SET role='admin' WHERE email='you@example.com';"
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

# Get connection config for a specific node
curl http://localhost:8080/api/v1/nodes/NODE_ID/connect \
  -H "Authorization: Bearer YOUR_TOKEN"

# Auto-select least-loaded node (no node ID needed)
curl http://localhost:8080/api/v1/nodes/connect \
  -H "Authorization: Bearer YOUR_TOKEN"

# Get subscription configs for a node
curl "http://localhost:8080/api/v1/nodes/NODE_ID/subscription?format=all" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Subscription formats: `all` (JSON) · `vless` (base64 for v2rayN) · `clash` (Clash Meta YAML) · `singbox` (sing-box JSON)

Both connect endpoints return `{ profile, vless_uri, node }`. The auto-select endpoint scores all online nodes by CPU (50%) + memory (30%) + active connections (20%) and returns the lowest-scoring one.

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
├── web/                      # React web portal (account management UI)
│   ├── src/
│   │   ├── pages/            # Login, Dashboard
│   │   ├── api/              # Axios API client
│   │   └── store/            # Zustand state (auth, nodes, connection)
│   └── vite.config.ts
├── scripts/
│   ├── deploy-control-plane.sh # Deploy control plane on Ubuntu VPS with TLS
│   ├── setup-server.sh         # Deploy Xray on a VPN node VPS
│   ├── manage-server.sh        # Server status, restart, credentials, firewall
│   ├── setup-mac-vpn.sh        # Connect Mac to VPN via Docker + system proxy
│   ├── add-node.sh             # Register a new server node in the database
│   ├── view-vpn-report.sh      # Pull traffic report from server, open in browser
│   ├── analyze-vpn-traffic.sh  # Run on server: parse logs, generate HTML report
│   ├── gen-client-config.sh    # Download subscription configs from control plane
│   ├── test-tunnel.sh          # Local tunnel verification (no VPS needed)
│   └── verify.sh               # API end-to-end test suite
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
