# VPN Server Management Guide

## Your Server

| | |
|---|---|
| **IP** | see `~/.ssh/config` → `oracle-vpn` host |
| **Provider** | Oracle Cloud (Always Free) |
| **Protocol** | VLESS + REALITY |
| **Port** | 443 |
| **Credentials file** | `/root/vpn-server-credentials.env` on the server |

---

## Connecting to the Server

```bash
ssh oracle-vpn
```

Your SSH config (`~/.ssh/config`) already has the IP and key path set up. If it stops working, check the IP in the Oracle Cloud console and update `~/.ssh/config`.

---

## The Management Script

All common tasks are handled by `manage-server.sh`. Copy it to the server first:

```bash
scp /Users/apple/Projects/go/VPN/scripts/manage-server.sh oracle-vpn:/tmp/manage-server.sh
```

Then SSH in and run any command:

```bash
ssh oracle-vpn
sudo bash /tmp/manage-server.sh <command>
```

### Available Commands

| Command | What it does |
|---------|-------------|
| `status` | Show whether Xray is running, port is open, firewall is correct |
| `restart` | Restart Xray (use if VPN stops working) |
| `start` | Start Xray |
| `stop` | Stop Xray |
| `logs` | Show last 50 lines of Xray logs |
| `credentials` | Print your VLESS URI and all credentials |
| `fix-fw` | Re-apply iptables firewall rules (run after reboot if VPN stops) |
| `save-fw` | Save firewall rules so they survive reboots |
| `update` | Update Xray to the latest version |

---

## Common Situations

### VPN stopped working

```bash
ssh oracle-vpn
sudo bash /tmp/manage-server.sh status
sudo bash /tmp/manage-server.sh restart
```

### After server reboot — VPN not connecting

Oracle's iptables rules reset on reboot unless saved. Fix:

```bash
ssh oracle-vpn
sudo bash /tmp/manage-server.sh fix-fw
```

To prevent this from happening again:

```bash
sudo bash /tmp/manage-server.sh save-fw
```

### Need your VLESS URI again (lost it / new phone)

```bash
ssh oracle-vpn
sudo bash /tmp/manage-server.sh credentials
```

Copy the `vless://` URI and import it into your client app.

### Check Xray logs for errors

```bash
ssh oracle-vpn
sudo bash /tmp/manage-server.sh logs
```

---

## Client Apps

Import your `vless://` URI into any of these apps:

| Platform | App | Where to get it |
|----------|-----|----------------|
| Android | v2RayTun | Play Store |
| Android | Hiddify | Play Store |
| iOS | Shadowrocket | App Store ($2.99) |
| macOS | NekoRay | github.com/MatsuriDayo/nekoray |
| Windows | v2rayN | github.com/2dust/v2rayN |
| Windows | Hiddify | hiddify.com |

**To import**: open the app → tap `+` → Import from clipboard → paste the URI → connect.

**To verify it works**: open browser and go to `https://ip.sb` — it should show `YOUR_SERVER_IP` instead of your real IP.

---

## One-Time Setup (already done)

These steps were already completed during initial setup. Listed here for reference:

1. Installed Xray on the Oracle VM
2. Generated VLESS UUID, X25519 key pair, short ID
3. Configured VLESS + REALITY on port 443 (camouflage: `www.microsoft.com`)
4. Created systemd service (`xray.service`) — starts automatically on boot
5. Opened ports in UFW: 22/tcp, 443/tcp, 443/udp, 8443/tcp
6. Added iptables ingress rules for port 443 (Oracle's extra firewall layer)
7. Opened ports in Oracle Cloud Security List (console firewall)

---

## Important Files on the Server

| File | Contents |
|------|----------|
| `/root/vpn-server-credentials.env` | UUID, keys, short ID, VLESS URI |
| `/etc/xray/config.json` | Xray server configuration |
| `/var/log/xray/` | Access and error logs |
| `/etc/systemd/system/xray.service` | Systemd service definition |
