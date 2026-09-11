<p align="center">
  <img src="web/public/icon.svg" width="96" height="96" alt="Ghost Node" />
</p>

<h1 align="center">Ghost Node</h1>

<p align="center">
  A censorship-resistant VPN platform built on <a href="https://github.com/XTLS/Xray-core">Xray-core</a>.<br/>
  VLESS+REALITY · VLESS+WebSocket+TLS · Hysteria2<br/>
  Go control plane · Node management API · Prometheus/Grafana · Client subscription endpoints
</p>

---

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
scp scripts/manage-server.sh ghost-node:/tmp/manage-server.sh
ssh ghost-node   # or: ssh user@YOUR_SERVER_IP
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

### `vpn.sh` — Connect to the VPN (Linux & macOS)

Fetches credentials from the server, starts an Xray client in Docker, and configures the system proxy. Works on Linux (gsettings) and macOS (networksetup).

Requires: Docker running, SSH access to the server configured in `~/.ssh/config`.

```bash
bash scripts/vpn.sh start    # connect — sets system proxy automatically
bash scripts/vpn.sh stop     # disconnect
bash scripts/vpn.sh status   # show exit IP and tunnel state
```

Verify it's working: open `https://ip.sb` in your browser — it should show your server's IP.

> **Note:** On Linux, Firefox is automatically routed through the VPN when connected (uses system proxy). On macOS, all system traffic routes through the tunnel.

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

## `ghostctl` — rotate a blocked IP

When a node's address gets blocked, you do not need a new server. Releasing the
instance's **ephemeral public IP** and drawing a new one from Oracle's regional
pool restores service in well under a minute, costs nothing, and leaves the
instance untouched — Xray listens on `0.0.0.0` and never learns the address
changed, so there is nothing to restart.

This matters most on Oracle Cloud Always Free, where creating a replacement VM
is not a realistic response to a block: Always Free instances are pinned to your
tenancy's home region, capacity is frequently unavailable, and the quota is
fixed. Rotating the address is free, instant and unlimited.

```bash
make ghostctl                  # build bin/ghostctl
cp configs/ghostctl.example.yaml ~/.ghostctl/config.yaml
chmod 600 ~/.ghostctl/config.yaml

./bin/ghostctl status          # what address is each node on?
./bin/ghostctl rotate jp1      # draw a new address
./bin/ghostctl rotate all -y
```

### What a rotation does

1. Releases the instance's ephemeral public IP and allocates a new one.
2. **Verifies** the new address with a TCP handshake. Oracle's pool contains
   plenty of already-blocked addresses, so an unverified draw can swap a dead IP
   for another dead IP. A failed draw is discarded and another is taken
   (`rotate.attempts`, default 3).
3. Records the released address — and any address that fails verification — in a
   burned list, so neither it nor its `/24` neighbours are accepted again for
   `rotate.burn_window` (default 30 days).
4. Repoints the node's **Cloudflare A record** at the new address.
5. `PUT`s the address to the control plane so the portal, subscriptions and
   generated VLESS URIs stay correct.
6. Rewrites `HostName` in `~/.ssh/config` so `ssh ghost-node-jp1` keeps working.

Steps 4–6 are best-effort: the address has already changed by the time they run,
so a failure is reported rather than aborting the rotation.

> Run `ghostctl` **from inside the network you are trying to escape**. That is
> what turns step 2 from a routing check into a useful signal — an address
> null-routed by a censor times out there and nowhere else.

### Point clients at a name, not an IP

Give each node a DNS record (`dns_record` in the config) and use that as the
node address instead of a raw IP. REALITY treats the connect address and the
camouflage SNI as independent fields, so `n1.example.com` with
`sni=www.apple.com` works exactly as before — but rotation then changes only the
A record, and **no client config has to be re-imported**. Failover becomes DNS
TTL (60s) plus a reconnect.

Keep a raw-IP entry in your subscription too, so a client can fall through to it
if the name is ever DNS-poisoned.

### Commands

| Command | What it does |
|---------|-------------|
| `ghostctl status [node]` | Instance state, current address, IP lifetime, reachability |
| `ghostctl rotate <node\|all>` | Release the current address and draw a verified new one |
| `ghostctl rotate jp1 --dry-run` | Show what would change, touch nothing |
| `ghostctl nodes` | List configured nodes |
| `ghostctl burned` | List addresses recorded as blocked |

Useful flags: `--attempts N`, `--no-probe`, `--no-dns`, `--no-cp`, `--no-ssh`,
`-y`, `--config PATH`.

### Oracle credentials

`ghostctl` reuses `~/.oci/config` — run `oci setup config` once, or set the
fields inline in the ghostctl config. The API user needs a policy allowing it to
swap addresses in the compartment holding your nodes:

```
Allow group ghost-rotators to manage public-ips in compartment <name>
Allow group ghost-rotators to use private-ips in compartment <name>
Allow group ghost-rotators to read instance-family in compartment <name>
Allow group ghost-rotators to read virtual-network-family in compartment <name>
```

Two Oracle-specific cautions:

- **Never terminate an Always Free Ampere A1 instance.** Capacity is scarce; you
  may not get it back for weeks. Rotate the address instead — that is the whole
  point of this tool.
- Always Free instances can be **reclaimed when idle**. Upgrading the tenancy to
  Pay As You Go stops reclamation while keeping Always Free resources free.

---

## Quick Start — Personal VPN

### 0. Set up SSH access to your server

`ghost-node` used throughout this guide is a convenience alias — set it once so you don't type the IP every time:

```bash
# Add this to ~/.ssh/config (create the file if it doesn't exist)
Host ghost-node
    HostName YOUR_SERVER_IP        # e.g. 1.2.3.4
    User ubuntu                    # or root, depending on your provider
    IdentityFile ~/.ssh/id_rsa     # path to your private key
    ServerAliveInterval 60
```

After saving, `ssh ghost-node` and `scp ... ghost-node:...` will work anywhere in this guide. You can also skip the alias entirely and replace every `ghost-node` occurrence with `user@YOUR_SERVER_IP` directly.

### 1. Provision a server

Any Ubuntu 22.04 VPS works. Oracle Cloud Always Free (Tokyo/Singapore) is recommended.

```bash
scp scripts/setup-server.sh ghost-node:/tmp/
ssh ghost-node
sudo bash /tmp/setup-server.sh
```

### 2. Save firewall rules (do once after setup)

```bash
ssh ghost-node
sudo bash /tmp/manage-server.sh save-fw
```

### 3. Connect your device

```bash
bash scripts/vpn.sh start
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

# Repoint a node at a new address (used by ghostctl after an IP rotation)
curl -X PUT http://localhost:8080/api/v1/admin/nodes/NODE_ID/address \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"address":"5.6.7.8"}'

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
│   ├── node-agent/           # Agent that runs on VPN servers
│   └── ghostctl/             # CLI: rotate a blocked node's public IP
├── internal/
│   ├── auth/                 # JWT + middleware
│   ├── handler/              # HTTP handlers (gin)
│   ├── service/              # Business logic
│   ├── repository/           # Database layer (GORM)
│   ├── models/               # DB models
│   ├── transport/            # Xray/Hysteria2 process management
│   ├── metrics/              # Prometheus instrumentation
│   ├── provisioner/          # Cloud IP rotation (Oracle) + DNS + verification
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
│   ├── vpn.sh                  # Connect to VPN via Docker + system proxy (Linux & macOS)
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
ssh ghost-node   # or: ssh user@YOUR_SERVER_IP
sudo bash /tmp/manage-server.sh credentials
```
