#!/usr/bin/env bash
# analyze-vpn-traffic.sh — Analyze Xray VPN traffic and generate an HTML report.
#
# Run on the VPN server:
#   sudo bash analyze-vpn-traffic.sh [--hours 24] [--out /tmp/vpn-report.html]
#
# Flags:
#   --hours N     Window to analyse, in hours (default 24)
#   --out FILE    HTML report path (default /tmp/vpn-report.html)
#   --json FILE   Also write the analysis as JSON
#   --log FILE    Xray access log (default /var/log/xray/access.log)
#   --top N       Rows kept in the report tables (default 200)
#   --quiet       Suppress the terminal summary
#
# The report is fully self-contained: no CDN, no external fonts, no favicon
# lookups. That keeps it working offline, and — more importantly — means
# opening a report never discloses the browsing history it contains to a
# third party.

set -euo pipefail

HOURS="${VPN_HOURS:-24}"
OUT="${VPN_OUT:-/tmp/vpn-report.html}"
JSON_OUT="${VPN_JSON:-}"
XRAY_LOG="${XRAY_LOG:-/var/log/xray/access.log}"
TOP="${VPN_TOP:-200}"
QUIET=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --hours) HOURS="$2"; shift 2 ;;
    --out)   OUT="$2";   shift 2 ;;
    --json)  JSON_OUT="$2"; shift 2 ;;
    --log)   XRAY_LOG="$2"; shift 2 ;;
    --top)   TOP="$2"; shift 2 ;;
    --quiet) QUIET=1; shift ;;
    -h|--help)
      awk 'NR > 1 && /^#/ { sub(/^# ?/, ""); print; next } NR > 1 { exit }' "$0"
      exit 0 ;;
    *) shift ;;
  esac
done

GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BOLD='\033[1m'; NC='\033[0m'
info()    { [[ -n "$QUIET" ]] || echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { [[ -n "$QUIET" ]] || echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*" >&2; }
die()     { echo -e "${RED}[FAIL]${NC}  $*" >&2; exit 1; }

IS_ROOT=0
[[ $(id -u) -eq 0 ]] && IS_ROOT=1

if [[ -z "$QUIET" ]]; then
  echo ""
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BOLD}   VPN Traffic Analyzer — last ${HOURS}h${NC}"
  echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
fi

command -v python3 >/dev/null 2>&1 || die "python3 is required"

# ── 1. Make sure the access log exists ───────────────────────────────────────
if [[ ! -f "$XRAY_LOG" ]] && [[ $IS_ROOT -eq 1 ]] && [[ -f /etc/xray/config.json ]]; then
  info "Access log not found at $XRAY_LOG — enabling it in the Xray config..."
  mkdir -p "$(dirname "$XRAY_LOG")"
  XRAY_LOG="$XRAY_LOG" python3 - <<'PYEOF'
import json, os
path = '/etc/xray/config.json'
with open(path) as f:
    cfg = json.load(f)
cfg.setdefault('log', {})
cfg['log']['access'] = os.environ['XRAY_LOG']
if cfg['log'].get('loglevel') in (None, 'none'):
    cfg['log']['loglevel'] = 'warning'
with open(path, 'w') as f:
    json.dump(cfg, f, indent=2)
print('Enabled access logging in /etc/xray/config.json')
PYEOF
  systemctl restart xray || true
  sleep 2
  warn "Access logging was just enabled — this report will be empty until traffic accumulates."
fi

[[ -f "$XRAY_LOG" ]] || warn "No access log at $XRAY_LOG — the report will have no destination data."

# ── 2. System / bandwidth counters (best effort) ─────────────────────────────
IFACE=$(ip route 2>/dev/null | awk '/^default/ {print $5; exit}')
IFACE="${IFACE:-eth0}"

# /proc/net/dev runs the interface name into the byte count once the counter is
# wide enough ("eth0:123456789"), so split on the colon before reading fields.
read -r RX_BYTES TX_BYTES <<<"$(awk -v iface="$IFACE" '{gsub(/:/, " "); if ($1 == iface) {print $2, $10; found=1}} END {if (!found) print 0, 0}' /proc/net/dev 2>/dev/null || echo "0 0")"

UPTIME_SECONDS=$(awk '{print int($1)}' /proc/uptime 2>/dev/null || echo 0)
SERVER_HOST=$(hostname 2>/dev/null || echo "unknown")
SERVER_IP=$(curl -4 -s --max-time 3 ifconfig.me 2>/dev/null || \
            curl -4 -s --max-time 3 api.ipify.org 2>/dev/null || echo "unknown")

# ── 3. Xray stats API (per-user byte counters) ───────────────────────────────
XRAY_STATS=""
if command -v xray >/dev/null 2>&1; then
  XRAY_STATS=$(xray api statsquery --server=127.0.0.1:10085 -pattern "" 2>/dev/null || echo "")
fi

# ── 4. Analyse and render ────────────────────────────────────────────────────
info "Parsing $XRAY_LOG (last ${HOURS}h)..."

VPN_LOG="$XRAY_LOG" \
VPN_HOURS="$HOURS" \
VPN_OUT="$OUT" \
VPN_JSON="$JSON_OUT" \
VPN_TOP="$TOP" \
VPN_QUIET="$QUIET" \
VPN_IFACE="$IFACE" \
VPN_RX="$RX_BYTES" \
VPN_TX="$TX_BYTES" \
VPN_UPTIME="$UPTIME_SECONDS" \
VPN_SERVER_IP="$SERVER_IP" \
VPN_SERVER_HOST="$SERVER_HOST" \
VPN_XRAY_STATS="$XRAY_STATS" \
python3 - <<'PYEOF'
"""Parse an Xray access log and render a self-contained traffic report."""

import gzip
import io
import json
import os
import re
import sys
from collections import Counter, defaultdict
from datetime import datetime, timedelta

LOG_PATH = os.environ.get("VPN_LOG", "")
HOURS = float(os.environ.get("VPN_HOURS") or 24)
OUT = os.environ.get("VPN_OUT", "/tmp/vpn-report.html")
JSON_OUT = os.environ.get("VPN_JSON") or ""
TOP = int(os.environ.get("VPN_TOP") or 200)
QUIET = bool(os.environ.get("VPN_QUIET"))

# ── Domain knowledge ────────────────────────────────────────────────────────
#
# Two-label public suffixes we care about, so "bbc.co.uk" groups as one site
# rather than collapsing to "co.uk". This is a pragmatic subset of the Public
# Suffix List — enough for the traffic a personal VPN actually sees.
MULTI_SUFFIXES = {
    "co.uk", "org.uk", "me.uk", "ac.uk", "gov.uk", "co.jp", "ne.jp", "or.jp",
    "ac.jp", "go.jp", "com.cn", "net.cn", "org.cn", "gov.cn", "edu.cn",
    "com.hk", "org.hk", "com.tw", "org.tw", "co.kr", "or.kr", "com.au",
    "net.au", "org.au", "com.br", "com.mx", "com.ar", "com.sg", "com.my",
    "co.in", "co.id", "co.th", "com.tr", "com.vn", "co.nz", "co.za",
    "com.ua", "co.il", "com.sa", "com.ph", "com.pk", "com.eg",
}

# registrable domain → (label, category, kind)
#   kind "site"       — somewhere a person actually went
#   kind "background" — CDN, telemetry, updates: traffic a person caused but
#                       never chose. Separating the two is the difference
#                       between "you visited 12 websites" and "you contacted
#                       380 hosts".
CATALOG = {
    # Video
    "youtube.com": ("YouTube", "video", "site"),
    "youtu.be": ("YouTube", "video", "site"),
    "googlevideo.com": ("YouTube", "video", "background"),
    "ytimg.com": ("YouTube", "video", "background"),
    "ggpht.com": ("YouTube", "video", "background"),
    "netflix.com": ("Netflix", "video", "site"),
    "nflxvideo.net": ("Netflix", "video", "background"),
    "nflximg.net": ("Netflix", "video", "background"),
    "nflxso.net": ("Netflix", "video", "background"),
    "twitch.tv": ("Twitch", "video", "site"),
    "ttvnw.net": ("Twitch", "video", "background"),
    "disneyplus.com": ("Disney+", "video", "site"),
    "hulu.com": ("Hulu", "video", "site"),
    "primevideo.com": ("Prime Video", "video", "site"),
    "bilibili.com": ("Bilibili", "video", "site"),
    "hdslb.com": ("Bilibili", "video", "background"),
    "vimeo.com": ("Vimeo", "video", "site"),
    "dailymotion.com": ("Dailymotion", "video", "site"),
    "iqiyi.com": ("iQIYI", "video", "site"),
    "youku.com": ("Youku", "video", "site"),

    # Social
    "instagram.com": ("Instagram", "social", "site"),
    "cdninstagram.com": ("Instagram", "social", "background"),
    "facebook.com": ("Facebook", "social", "site"),
    "fbcdn.net": ("Facebook", "social", "background"),
    "fbsbx.com": ("Facebook", "social", "background"),
    "twitter.com": ("X / Twitter", "social", "site"),
    "x.com": ("X / Twitter", "social", "site"),
    "twimg.com": ("X / Twitter", "social", "background"),
    "t.co": ("X / Twitter", "social", "site"),
    "tiktok.com": ("TikTok", "social", "site"),
    "tiktokcdn.com": ("TikTok", "social", "background"),
    "tiktokv.com": ("TikTok", "social", "background"),
    "muscdn.com": ("TikTok", "social", "background"),
    "reddit.com": ("Reddit", "social", "site"),
    "redd.it": ("Reddit", "social", "background"),
    "redditmedia.com": ("Reddit", "social", "background"),
    "redditstatic.com": ("Reddit", "social", "background"),
    "linkedin.com": ("LinkedIn", "social", "site"),
    "licdn.com": ("LinkedIn", "social", "background"),
    "pinterest.com": ("Pinterest", "social", "site"),
    "pinimg.com": ("Pinterest", "social", "background"),
    "weibo.com": ("Weibo", "social", "site"),
    "zhihu.com": ("Zhihu", "social", "site"),
    "xiaohongshu.com": ("Xiaohongshu", "social", "site"),
    "threads.net": ("Threads", "social", "site"),
    "quora.com": ("Quora", "social", "site"),
    "tumblr.com": ("Tumblr", "social", "site"),

    # Messaging
    "telegram.org": ("Telegram", "messaging", "site"),
    "t.me": ("Telegram", "messaging", "site"),
    "telegram.me": ("Telegram", "messaging", "site"),
    "tdesktop.com": ("Telegram", "messaging", "background"),
    "whatsapp.com": ("WhatsApp", "messaging", "site"),
    "whatsapp.net": ("WhatsApp", "messaging", "background"),
    "discord.com": ("Discord", "messaging", "site"),
    "discordapp.com": ("Discord", "messaging", "background"),
    "discordapp.net": ("Discord", "messaging", "background"),
    "signal.org": ("Signal", "messaging", "site"),
    "line.me": ("LINE", "messaging", "site"),
    "line-scdn.net": ("LINE", "messaging", "background"),
    "kakao.com": ("KakaoTalk", "messaging", "site"),
    "slack.com": ("Slack", "productivity", "site"),
    "slack-edge.com": ("Slack", "productivity", "background"),
    "zoom.us": ("Zoom", "productivity", "site"),
    "wechat.com": ("WeChat", "messaging", "site"),
    "weixin.qq.com": ("WeChat", "messaging", "site"),

    # AI
    "openai.com": ("ChatGPT", "ai", "site"),
    "chatgpt.com": ("ChatGPT", "ai", "site"),
    "oaistatic.com": ("ChatGPT", "ai", "background"),
    "oaiusercontent.com": ("ChatGPT", "ai", "background"),
    "anthropic.com": ("Claude", "ai", "site"),
    "claude.ai": ("Claude", "ai", "site"),
    "perplexity.ai": ("Perplexity", "ai", "site"),
    "midjourney.com": ("Midjourney", "ai", "site"),
    "huggingface.co": ("Hugging Face", "ai", "site"),
    "gemini.google.com": ("Gemini", "ai", "site"),

    # Dev
    "github.com": ("GitHub", "dev", "site"),
    "githubusercontent.com": ("GitHub", "dev", "background"),
    "githubassets.com": ("GitHub", "dev", "background"),
    "ghcr.io": ("GitHub", "dev", "background"),
    "gitlab.com": ("GitLab", "dev", "site"),
    "bitbucket.org": ("Bitbucket", "dev", "site"),
    "stackoverflow.com": ("Stack Overflow", "dev", "site"),
    "stackexchange.com": ("Stack Exchange", "dev", "site"),
    "npmjs.org": ("npm", "dev", "background"),
    "npmjs.com": ("npm", "dev", "background"),
    "pypi.org": ("PyPI", "dev", "background"),
    "pythonhosted.org": ("PyPI", "dev", "background"),
    "docker.com": ("Docker Hub", "dev", "background"),
    "docker.io": ("Docker Hub", "dev", "background"),
    "golang.org": ("Go", "dev", "background"),
    "go.dev": ("Go", "dev", "site"),
    "crates.io": ("crates.io", "dev", "background"),
    "rubygems.org": ("RubyGems", "dev", "background"),
    "jsdelivr.net": ("jsDelivr", "cdn", "background"),
    "unpkg.com": ("unpkg", "cdn", "background"),
    "vercel.app": ("Vercel", "dev", "site"),
    "netlify.app": ("Netlify", "dev", "site"),

    # Search & portals
    "google.com": ("Google", "search", "site"),
    "google.co.jp": ("Google", "search", "site"),
    "googleapis.com": ("Google", "cloud", "background"),
    "gstatic.com": ("Google", "cdn", "background"),
    "googleusercontent.com": ("Google", "cdn", "background"),
    "googlesyndication.com": ("Google Ads", "telemetry", "background"),
    "doubleclick.net": ("Google Ads", "telemetry", "background"),
    "google-analytics.com": ("Google Analytics", "telemetry", "background"),
    "googletagmanager.com": ("Google Tag Manager", "telemetry", "background"),
    "bing.com": ("Bing", "search", "site"),
    "duckduckgo.com": ("DuckDuckGo", "search", "site"),
    "baidu.com": ("Baidu", "search", "site"),
    "yandex.ru": ("Yandex", "search", "site"),
    "wikipedia.org": ("Wikipedia", "news", "site"),
    "wikimedia.org": ("Wikimedia", "news", "background"),

    # News
    "nytimes.com": ("NY Times", "news", "site"),
    "bbc.co.uk": ("BBC", "news", "site"),
    "bbc.com": ("BBC", "news", "site"),
    "theguardian.com": ("The Guardian", "news", "site"),
    "cnn.com": ("CNN", "news", "site"),
    "reuters.com": ("Reuters", "news", "site"),
    "bloomberg.com": ("Bloomberg", "news", "site"),
    "ft.com": ("Financial Times", "news", "site"),
    "economist.com": ("The Economist", "news", "site"),
    "medium.com": ("Medium", "news", "site"),
    "substack.com": ("Substack", "news", "site"),
    "news.ycombinator.com": ("Hacker News", "news", "site"),

    # Music
    "spotify.com": ("Spotify", "music", "site"),
    "scdn.co": ("Spotify", "music", "background"),
    "spotifycdn.com": ("Spotify", "music", "background"),
    "soundcloud.com": ("SoundCloud", "music", "site"),
    "music.apple.com": ("Apple Music", "music", "site"),
    "bandcamp.com": ("Bandcamp", "music", "site"),

    # Shopping & finance
    "amazon.com": ("Amazon", "shopping", "site"),
    "amazon.co.jp": ("Amazon", "shopping", "site"),
    "ebay.com": ("eBay", "shopping", "site"),
    "taobao.com": ("Taobao", "shopping", "site"),
    "tmall.com": ("Tmall", "shopping", "site"),
    "jd.com": ("JD.com", "shopping", "site"),
    "aliexpress.com": ("AliExpress", "shopping", "site"),
    "shopify.com": ("Shopify", "shopping", "site"),
    "paypal.com": ("PayPal", "finance", "site"),
    "stripe.com": ("Stripe", "finance", "site"),
    "binance.com": ("Binance", "finance", "site"),
    "coinbase.com": ("Coinbase", "finance", "site"),

    # Cloud / CDN / infrastructure
    "amazonaws.com": ("AWS", "cloud", "background"),
    "cloudfront.net": ("CloudFront", "cdn", "background"),
    "cloudflare.com": ("Cloudflare", "cdn", "background"),
    "cloudflare-dns.com": ("Cloudflare DNS", "system", "background"),
    "dns.google": ("Google DNS", "system", "background"),
    "one.one.one.one": ("Cloudflare DNS", "system", "background"),
    "quad9.net": ("Quad9 DNS", "system", "background"),
    "opendns.com": ("OpenDNS", "system", "background"),
    "adguard-dns.com": ("AdGuard DNS", "system", "background"),
    "nextdns.io": ("NextDNS", "system", "background"),
    "akamai.net": ("Akamai", "cdn", "background"),
    "akamaized.net": ("Akamai", "cdn", "background"),
    "akamaitechnologies.com": ("Akamai", "cdn", "background"),
    "fastly.net": ("Fastly", "cdn", "background"),
    "azureedge.net": ("Azure CDN", "cdn", "background"),
    "azure.com": ("Azure", "cloud", "background"),
    "windows.net": ("Azure", "cloud", "background"),
    "digitaloceanspaces.com": ("DigitalOcean", "cloud", "background"),
    "oraclecloud.com": ("Oracle Cloud", "cloud", "background"),
    "gvt1.com": ("Google Updates", "telemetry", "background"),
    "gvt2.com": ("Google Updates", "telemetry", "background"),

    # OS & vendor
    "apple.com": ("Apple", "os", "site"),
    "icloud.com": ("iCloud", "os", "site"),
    "mzstatic.com": ("Apple", "os", "background"),
    "aaplimg.com": ("Apple", "os", "background"),
    "cdn-apple.com": ("Apple", "os", "background"),
    "push.apple.com": ("Apple Push", "telemetry", "background"),
    "microsoft.com": ("Microsoft", "os", "site"),
    "live.com": ("Microsoft", "os", "site"),
    "office.com": ("Microsoft 365", "productivity", "site"),
    "msftconnecttest.com": ("Windows Connectivity", "telemetry", "background"),
    "msftncsi.com": ("Windows Connectivity", "telemetry", "background"),
    "windowsupdate.com": ("Windows Update", "telemetry", "background"),
    "xboxlive.com": ("Xbox", "gaming", "background"),
    "ubuntu.com": ("Ubuntu", "os", "background"),
    "canonical.com": ("Ubuntu", "os", "background"),
    "debian.org": ("Debian", "os", "background"),
    "mozilla.org": ("Mozilla", "os", "background"),
    "mozilla.net": ("Mozilla", "os", "background"),
    "firefox.com": ("Firefox", "os", "background"),

    # Productivity
    "notion.so": ("Notion", "productivity", "site"),
    "figma.com": ("Figma", "productivity", "site"),
    "dropbox.com": ("Dropbox", "productivity", "site"),
    "dropboxusercontent.com": ("Dropbox", "productivity", "background"),
    "atlassian.net": ("Atlassian", "productivity", "site"),
    "trello.com": ("Trello", "productivity", "site"),
    "asana.com": ("Asana", "productivity", "site"),
    "airtable.com": ("Airtable", "productivity", "site"),

    # Gaming
    "steampowered.com": ("Steam", "gaming", "site"),
    "steamstatic.com": ("Steam", "gaming", "background"),
    "epicgames.com": ("Epic Games", "gaming", "site"),
    "riotgames.com": ("Riot Games", "gaming", "site"),
    "nintendo.net": ("Nintendo", "gaming", "background"),
    "playstation.net": ("PlayStation", "gaming", "background"),
}

# Substring hints for hosts we have no exact entry for.
KEYWORD_RULES = [
    (("telemetry", "analytics", "metrics", "crashlytics", "sentry", "bugsnag",
      "doubleclick", "adservice", "adsystem", "advertising", "tracking",
      "beacon", "pagead"), "telemetry", "background"),
    (("cdn", "static", "assets", "edgekey", "edgesuite", "cachefly"), "cdn", "background"),
    (("ocsp", "crl", "pki", "digicert", "letsencrypt", "sectigo", "globalsign"),
     "security", "background"),
    (("ntp", "time.", "pool.ntp"), "system", "background"),
    (("update", "upgrade", "download"), "os", "background"),
    (("mail", "smtp", "imap"), "mail", "site"),
]

CATEGORY_META = {
    "video": ("Video", "#f87171"),
    "social": ("Social", "#a78bfa"),
    "messaging": ("Messaging", "#38bdf8"),
    "ai": ("AI", "#2dd4bf"),
    "dev": ("Developer", "#4ade80"),
    "search": ("Search", "#60a5fa"),
    "news": ("News & reading", "#fbbf24"),
    "music": ("Music", "#e879f9"),
    "shopping": ("Shopping", "#fb923c"),
    "finance": ("Finance", "#34d399"),
    "productivity": ("Productivity", "#818cf8"),
    "gaming": ("Gaming", "#fb7185"),
    "cloud": ("Cloud", "#94a3b8"),
    "cdn": ("CDN", "#64748b"),
    "telemetry": ("Telemetry & ads", "#a1a1aa"),
    "security": ("Certificates", "#a3e635"),
    "os": ("OS & vendor", "#c084fc"),
    "mail": ("Mail", "#facc15"),
    "system": ("System", "#71717a"),
    "direct-ip": ("Direct IP", "#78716c"),
    "other": ("Other", "#8b96a8"),
}

IPV4_RE = re.compile(r"^\d{1,3}(?:\.\d{1,3}){3}$")


def registrable_domain(host):
    """Reduce a hostname to the domain a person would recognise as "the site"."""
    host = host.strip(".").lower()
    if not host or IPV4_RE.match(host) or ":" in host:
        return host
    parts = host.split(".")
    if len(parts) <= 2:
        return host
    last_two = ".".join(parts[-2:])
    if last_two in MULTI_SUFFIXES and len(parts) >= 3:
        return ".".join(parts[-3:])
    return last_two


def classify(domain, host):
    """Return (label, category, kind) for a destination."""
    if IPV4_RE.match(domain) or ":" in domain:
        return (domain, "direct-ip", "background")

    # Exact match on the full host first (news.ycombinator.com, music.apple.com),
    # then on the registrable domain.
    for key in (host.lower(), domain):
        if key in CATALOG:
            return CATALOG[key]

    for needles, category, kind in KEYWORD_RULES:
        if any(n in host.lower() for n in needles):
            label = domain.split(".")[0].capitalize()
            return (label, category, kind)

    return (domain.split(".")[0].capitalize(), "other", "site")


# ── Log parsing ─────────────────────────────────────────────────────────────
#
# Xray access lines look like:
#   2026/09/14 08:31:02 from 10.0.0.2:51514 accepted tcp:www.youtube.com:443 [vless-in >> direct] email: me@example.com
# Older builds insert a [Info] level and omit the source. Everything except the
# timestamp and the accepted/rejected clause is treated as optional.
TS_RE = re.compile(r"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})(?:\.\d+)?\s+(.*)$")
ACTION_RE = re.compile(
    r"\b(accepted|rejected|blocked)\s+"
    r"(?:(tcp|udp|tcp4|udp4|tcp6|udp6)[:/])?"
    r"(\[[0-9A-Fa-f:]+\]|[^\s:\[\]]+):(\d{1,5})\b"
)
# "[inbound >> outbound]" on a routed connection, bare "[outbound]" otherwise.
TAGS_RE = re.compile(r"\[([^\]\s]*)\s*>>\s*([^\]]*?)\s*\]")
TAG_SINGLE_RE = re.compile(r"\[([A-Za-z0-9_.\-]+)\]\s*$")
EMAIL_RE = re.compile(r"email:\s*(\S+)")
SOURCE_RE = re.compile(r"\bfrom\s+(\S+?)(?::\d+)?\s")
SOURCE_LEAD_RE = re.compile(r"^(\[[0-9A-Fa-f:]+\]|\d{1,3}(?:\.\d{1,3}){3}):\d+\s")


def log_files(path):
    """The active log plus any rotated siblings, oldest first."""
    if not path:
        return []
    found = []
    directory = os.path.dirname(path) or "."
    base = os.path.basename(path)
    try:
        entries = os.listdir(directory)
    except OSError:
        entries = []
    for name in entries:
        if name == base or name.startswith(base + "."):
            found.append(os.path.join(directory, name))
    # Rotated files hold older lines, so read them first.
    found.sort(key=lambda p: (p != path, p), reverse=True)
    return [p for p in found if os.path.isfile(p)]


def open_log(path):
    if path.endswith(".gz"):
        return io.TextIOWrapper(gzip.open(path, "rb"), errors="replace")
    return open(path, "r", errors="replace")


now = datetime.now()
cutoff = now - timedelta(hours=HOURS)

events = []
parse_errors = 0
lines_read = 0
# Counters that turn "the report is empty" into a specific, actionable reason.
lines_with_timestamp = 0
lines_in_window = 0
newest_seen = None
unparsed_samples = []
unmatched_samples = []

for path in log_files(LOG_PATH):
    try:
        handle = open_log(path)
    except OSError as exc:
        print("warning: cannot read %s: %s" % (path, exc), file=sys.stderr)
        continue
    with handle:
        for line in handle:
            lines_read += 1
            ts_match = TS_RE.match(line)
            if not ts_match:
                if line.strip() and len(unparsed_samples) < 3:
                    unparsed_samples.append(line.strip()[:180])
                continue
            try:
                when = datetime.strptime(ts_match.group(1), "%Y/%m/%d %H:%M:%S")
            except ValueError:
                parse_errors += 1
                continue

            lines_with_timestamp += 1
            if newest_seen is None or when > newest_seen:
                newest_seen = when

            if when < cutoff or when > now + timedelta(minutes=5):
                continue
            lines_in_window += 1

            rest = ts_match.group(2)
            action_match = ACTION_RE.search(rest)
            if not action_match:
                if "accepted" in rest or "rejected" in rest:
                    if len(unmatched_samples) < 3:
                        unmatched_samples.append(line.strip()[:180])
                continue

            action, network, host, port = action_match.groups()
            host = host.strip("[]")
            tags = TAGS_RE.search(rest)
            inbound, outbound = "", ""
            if tags:
                inbound, outbound = tags.group(1), tags.group(2)
            else:
                single = TAG_SINGLE_RE.search(rest.strip())
                if single:
                    outbound = single.group(1)

            email = EMAIL_RE.search(rest)
            source = SOURCE_RE.search(rest + " ") or SOURCE_LEAD_RE.match(rest)
            source_addr = source.group(1) if source else ""
            # Xray renders the source as a full destination: "tcp:127.0.0.1:60030".
            source_addr = re.sub(r"^(?:tcp|udp)[:/]", "", source_addr)

            events.append({
                "when": when,
                "action": "rejected" if action in ("rejected", "blocked") else "accepted",
                "network": (network or "tcp").rstrip("46"),
                "host": host,
                "port": int(port),
                "inbound": inbound,
                "outbound": outbound,
                "email": email.group(1) if email else "",
                "source": source_addr,
            })

events.sort(key=lambda e: e["when"])

# ── Aggregation ─────────────────────────────────────────────────────────────
def new_site():
    return {
        "connections": 0,
        "rejected": 0,
        "hosts": Counter(),
        "ports": Counter(),
        "networks": Counter(),
        "outbounds": Counter(),
        "clients": Counter(),
        "hourly": Counter(),
        "first": None,
        "last": None,
    }


sites = defaultdict(new_site)
site_meta = {}
hourly_total = Counter()
hourly_by_category = defaultdict(Counter)
ports = Counter()
networks = Counter()
outbounds = Counter()
clients = defaultdict(lambda: {"connections": 0, "sites": set(), "first": None, "last": None, "emails": set()})
rejected_events = []

for event in events:
    host = event["host"]
    domain = registrable_domain(host)
    label, category, kind = classify(domain, host)
    site_meta[domain] = {"label": label, "category": category, "kind": kind}

    if event["port"] == 53:
        label, category, kind = (label, "system", "background")
        site_meta[domain] = {"label": label, "category": category, "kind": kind}

    bucket = sites[domain]
    bucket["connections"] += 1
    bucket["hosts"][host] += 1
    bucket["ports"][event["port"]] += 1
    bucket["networks"][event["network"]] += 1
    if event["outbound"]:
        bucket["outbounds"][event["outbound"]] += 1

    who = event["email"] or event["source"] or "unknown"
    bucket["clients"][who] += 1

    hour_key = event["when"].strftime("%Y-%m-%d %H:00")
    bucket["hourly"][hour_key] += 1
    hourly_total[hour_key] += 1
    hourly_by_category[hour_key][category] += 1

    if bucket["first"] is None or event["when"] < bucket["first"]:
        bucket["first"] = event["when"]
    if bucket["last"] is None or event["when"] > bucket["last"]:
        bucket["last"] = event["when"]

    ports[event["port"]] += 1
    networks[event["network"]] += 1
    outbounds[event["outbound"] or "unknown"] += 1

    client = clients[who]
    client["connections"] += 1
    client["sites"].add(domain)
    if event["email"]:
        client["emails"].add(event["email"])
    if client["first"] is None or event["when"] < client["first"]:
        client["first"] = event["when"]
    if client["last"] is None or event["when"] > client["last"]:
        client["last"] = event["when"]

    if event["action"] == "rejected" or event["outbound"] == "block":
        bucket["rejected"] += 1
        rejected_events.append(event)


def iso(dt):
    return dt.strftime("%Y-%m-%d %H:%M:%S") if dt else None


total_connections = len(events)
site_rows = []
for domain, bucket in sites.items():
    meta = site_meta[domain]
    span_hours = sorted(bucket["hourly"].items())
    site_rows.append({
        "domain": domain,
        "label": meta["label"],
        "category": meta["category"],
        "kind": meta["kind"],
        "connections": bucket["connections"],
        "rejected": bucket["rejected"],
        "share": round(100.0 * bucket["connections"] / total_connections, 1) if total_connections else 0.0,
        "hostCount": len(bucket["hosts"]),
        "hosts": [{"host": h, "connections": c} for h, c in bucket["hosts"].most_common(25)],
        "ports": [{"port": p, "connections": c} for p, c in bucket["ports"].most_common()],
        "networks": dict(bucket["networks"]),
        "outbounds": dict(bucket["outbounds"]),
        "clients": [{"client": c, "connections": n} for c, n in bucket["clients"].most_common()],
        "first": iso(bucket["first"]),
        "last": iso(bucket["last"]),
        "activeHours": len(bucket["hourly"]),
        "timeline": [{"hour": h, "connections": c} for h, c in span_hours],
    })

site_rows.sort(key=lambda s: (-s["connections"], s["domain"]))

visited = [s for s in site_rows if s["kind"] == "site"]
background = [s for s in site_rows if s["kind"] != "site"]

category_rows = []
category_totals = Counter()
category_sites = Counter()
for site in site_rows:
    category_totals[site["category"]] += site["connections"]
    category_sites[site["category"]] += 1
for name, count in category_totals.most_common():
    label, colour = CATEGORY_META.get(name, (name.title(), "#8b96a8"))
    category_rows.append({
        "key": name,
        "label": label,
        "colour": colour,
        "connections": count,
        "sites": category_sites[name],
        "share": round(100.0 * count / total_connections, 1) if total_connections else 0.0,
    })

# A continuous hourly axis, so quiet hours show as gaps rather than vanishing.
timeline = []
if events:
    start = events[0]["when"].replace(minute=0, second=0, microsecond=0)
    end = events[-1]["when"].replace(minute=0, second=0, microsecond=0)
    cursor = start
    while cursor <= end:
        key = cursor.strftime("%Y-%m-%d %H:00")
        timeline.append({
            "hour": key,
            "label": cursor.strftime("%a %H:00"),
            "connections": hourly_total.get(key, 0),
            "byCategory": dict(hourly_by_category.get(key, {})),
        })
        cursor += timedelta(hours=1)

peak = max(timeline, key=lambda h: h["connections"]) if timeline else None

client_rows = sorted(
    (
        {
            "client": who,
            "connections": data["connections"],
            "sites": len(data["sites"]),
            "emails": sorted(data["emails"]),
            "first": iso(data["first"]),
            "last": iso(data["last"]),
        }
        for who, data in clients.items()
    ),
    key=lambda c: -c["connections"],
)

rejected_rows = []
rejected_counter = Counter((e["host"], e["port"]) for e in rejected_events)
for (host, port), count in rejected_counter.most_common(TOP):
    rejected_rows.append({"host": host, "port": port, "connections": count})

PORT_NAMES = {80: "HTTP", 443: "HTTPS", 53: "DNS", 22: "SSH", 853: "DNS-over-TLS",
              993: "IMAPS", 465: "SMTPS", 587: "SMTP", 8080: "HTTP-alt", 8443: "HTTPS-alt"}
port_rows = [
    {"port": p, "name": PORT_NAMES.get(p, "port %d" % p), "connections": c,
     "share": round(100.0 * c / total_connections, 1) if total_connections else 0.0}
    for p, c in ports.most_common(12)
]

# ── Xray stats API (per-user byte counters) ─────────────────────────────────
def human_bytes(value):
    value = float(value)
    for unit in ("B", "KB", "MB", "GB", "TB"):
        if value < 1024 or unit == "TB":
            return "%.1f %s" % (value, unit) if unit != "B" else "%d B" % value
        value /= 1024
    return "%.1f TB" % value


xray_rows = []
raw_stats = os.environ.get("VPN_XRAY_STATS", "")
if raw_stats.strip():
    try:
        payload = json.loads(raw_stats)
        for entry in payload.get("stat", []) or []:
            value = int(entry.get("value", 0) or 0)
            if value <= 0:
                continue
            xray_rows.append({
                "name": entry.get("name", ""),
                "bytes": value,
                "human": human_bytes(value),
            })
        xray_rows.sort(key=lambda r: -r["bytes"])
    except (ValueError, TypeError):
        pass

def build_diagnosis():
    """Explain an empty report instead of rendering a blank page."""
    if events:
        return None
    if not LOG_PATH or not os.path.exists(LOG_PATH):
        return ("No access log found at %s. Check that \"access\" is set in the \"log\" "
                "section of /etc/xray/config.json, then restart Xray." % (LOG_PATH or "(unset)"))
    if lines_read == 0:
        return ("The access log %s is empty. Xray writes it as connections happen — if "
                "logging was only just enabled, wait for traffic and run this again." % LOG_PATH)
    if lines_with_timestamp == 0:
        return ("Read %d lines from %s but none look like Xray log entries. Sample: %s"
                % (lines_read, LOG_PATH, unparsed_samples[0] if unparsed_samples else "(none)"))
    if lines_in_window == 0:
        return ("Found %d log entries, but none in the last %gh — the newest is from %s. "
                "Try a longer window, e.g. --hours %d."
                % (lines_with_timestamp, HOURS, iso(newest_seen),
                   max(48, int(HOURS * 4))))
    if unmatched_samples:
        return ("Found %d entries in the window but could not read their destinations — "
                "the log format was not recognised. Sample: %s"
                % (lines_in_window, unmatched_samples[0]))
    return ("Found %d log entries in the window but none were connection records. The log "
            "may contain only errors or startup messages." % lines_in_window)


diagnosis = build_diagnosis()

report = {
    "diagnosis": diagnosis,
    "meta": {
        "generated": now.strftime("%Y-%m-%d %H:%M:%S"),
        "windowHours": HOURS,
        "windowStart": iso(cutoff),
        "windowEnd": iso(now),
        "server": os.environ.get("VPN_SERVER_HOST", "unknown"),
        "serverIP": os.environ.get("VPN_SERVER_IP", "unknown"),
        "logPath": LOG_PATH,
        "linesRead": lines_read,
        "linesWithTimestamp": lines_with_timestamp,
        "linesInWindow": lines_in_window,
        "newestEntry": iso(newest_seen),
    },
    "summary": {
        "connections": total_connections,
        "accepted": sum(1 for e in events if e["action"] == "accepted"),
        "rejected": sum(1 for e in events if e["action"] == "rejected" or e["outbound"] == "block"),
        "sitesVisited": len(visited),
        "backgroundSites": len(background),
        "uniqueHosts": len({e["host"] for e in events}),
        "directIPs": len([s for s in site_rows if s["category"] == "direct-ip"]),
        "clients": len(client_rows),
        "peakHour": peak["label"] if peak else None,
        "peakConnections": peak["connections"] if peak else 0,
        "activeHours": sum(1 for h in timeline if h["connections"] > 0),
        "busiestSite": visited[0]["domain"] if visited else None,
    },
    "system": {
        "interface": os.environ.get("VPN_IFACE", ""),
        "rxBytes": int(os.environ.get("VPN_RX") or 0),
        "txBytes": int(os.environ.get("VPN_TX") or 0),
        "rxHuman": human_bytes(int(os.environ.get("VPN_RX") or 0)),
        "txHuman": human_bytes(int(os.environ.get("VPN_TX") or 0)),
        "uptimeSeconds": int(os.environ.get("VPN_UPTIME") or 0),
    },
    "sites": site_rows[:TOP],
    "visited": visited[:TOP],
    "background": background[:TOP],
    "categories": category_rows,
    "timeline": timeline,
    "ports": port_rows,
    "networks": dict(networks),
    "routing": [{"outbound": k, "connections": v} for k, v in outbounds.most_common()],
    "clients": client_rows,
    "rejected": rejected_rows,
    "xrayStats": xray_rows[:20],
}


# ── Terminal summary ────────────────────────────────────────────────────────
BOLD, DIM, RESET = "\033[1m", "\033[2m", "\033[0m"
CYAN, GREEN, YELLOW = "\033[0;36m", "\033[0;32m", "\033[1;33m"


def bar(value, largest, width=24):
    if largest <= 0:
        return ""
    filled = max(1, int(round(value * width / largest))) if value else 0
    return "█" * filled


if not QUIET:
    s = report["summary"]
    print("")
    if not events:
        print("  %sNo connections to report.%s" % (YELLOW, RESET))
        print("")
        print("  %s%s%s" % (DIM, diagnosis, RESET))
        print("")
        print("  %sscanned %d lines · %d with timestamps · %d in the last %gh%s"
              % (DIM, lines_read, lines_with_timestamp, lines_in_window, HOURS, RESET))
        for sample in (unmatched_samples or unparsed_samples)[:2]:
            print("  %s  | %s%s" % (DIM, sample, RESET))
        print("")
    else:
        print("  %sWebsites visited%s   %s%d%s across %d connections"
              % (BOLD, RESET, GREEN, s["sitesVisited"], RESET, s["connections"]))
        print("  %sBackground hosts%s  %d domains, %d unique hostnames"
              % (BOLD, RESET, s["backgroundSites"], s["uniqueHosts"]))
        if s["rejected"]:
            print("  %sBlocked%s           %d connections" % (BOLD, RESET, s["rejected"]))
        if s["peakHour"]:
            print("  %sBusiest hour%s      %s (%d connections)"
                  % (BOLD, RESET, s["peakHour"], s["peakConnections"]))
        print("")

        print("  %sTop websites visited%s" % (BOLD, RESET))
        print("  " + "─" * 62)
        top_visited = visited[:15]
        largest = top_visited[0]["connections"] if top_visited else 0
        for site in top_visited:
            print("  %-26s %s%-24s%s %5d  %s%s%s"
                  % (site["domain"][:26], CYAN, bar(site["connections"], largest), RESET,
                     site["connections"], DIM, CATEGORY_META.get(site["category"], ("", ""))[0], RESET))
        if not top_visited:
            print("  %s(only background traffic in this window)%s" % (DIM, RESET))
        print("")

        print("  %sBy category%s" % (BOLD, RESET))
        print("  " + "─" * 62)
        largest = category_rows[0]["connections"] if category_rows else 0
        for row in category_rows[:10]:
            print("  %-26s %s%-24s%s %5d  %s%.0f%%%s"
                  % (row["label"][:26], CYAN, bar(row["connections"], largest), RESET,
                     row["connections"], DIM, row["share"], RESET))
        print("")

        if len(client_rows) > 1:
            print("  %sClients%s" % (BOLD, RESET))
            print("  " + "─" * 62)
            for row in client_rows[:8]:
                print("  %-26s %5d connections, %d sites"
                      % (row["client"][:26], row["connections"], row["sites"]))
            print("")


# ── HTML report ─────────────────────────────────────────────────────────────
HTML_TEMPLATE = r"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>VPN Traffic Report</title>
<style>
  :root {
    --bg: #0b0e14; --panel: #131824; --panel-2: #1a2130; --border: #232c3d;
    --text: #e6edf6; --muted: #8b98ad; --dim: #5b6779;
    --accent: #5aa2ff; --accent-soft: rgba(90,162,255,0.14);
    --good: #3ddc97; --warn: #ffb545; --bad: #ff6b6b;
    --shadow: 0 1px 3px rgba(0,0,0,0.4);
  }
  html[data-theme="light"] {
    --bg: #f6f8fc; --panel: #ffffff; --panel-2: #f1f4fa; --border: #dde3ee;
    --text: #17202e; --muted: #5d6b82; --dim: #8a97ab;
    --accent: #2563eb; --accent-soft: rgba(37,99,235,0.10);
    --good: #0f9d63; --warn: #b8730a; --bad: #d13b3b;
    --shadow: 0 1px 3px rgba(20,30,50,0.08);
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    background: var(--bg); color: var(--text); line-height: 1.5;
    -webkit-font-smoothing: antialiased;
  }
  code, .mono { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; }

  .topbar {
    position: sticky; top: 0; z-index: 20;
    background: color-mix(in srgb, var(--bg) 88%, transparent);
    backdrop-filter: blur(8px);
    border-bottom: 1px solid var(--border);
    padding: 14px 24px;
    display: flex; align-items: center; gap: 16px; flex-wrap: wrap;
  }
  .brand { font-weight: 700; font-size: 15px; letter-spacing: -0.01em; }
  .brand span { color: var(--accent); }
  .topbar .meta { color: var(--muted); font-size: 12.5px; margin-left: auto;
                  display: flex; gap: 14px; flex-wrap: wrap; align-items: center; }
  .topbar .meta b { color: var(--text); font-weight: 600; }
  .icon-btn {
    background: var(--panel); border: 1px solid var(--border); color: var(--muted);
    border-radius: 8px; padding: 6px 10px; cursor: pointer; font-size: 13px;
    display: inline-flex; align-items: center; gap: 6px;
  }
  .icon-btn:hover { color: var(--text); border-color: var(--accent); }

  .wrap { max-width: 1280px; margin: 0 auto; padding: 24px; }

  .kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(168px, 1fr));
          gap: 12px; margin-bottom: 22px; }
  .kpi { background: var(--panel); border: 1px solid var(--border); border-radius: 12px;
         padding: 14px 16px; box-shadow: var(--shadow); }
  .kpi .label { font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em;
                color: var(--muted); font-weight: 600; }
  .kpi .value { font-size: 26px; font-weight: 700; margin-top: 6px; letter-spacing: -0.02em; }
  .kpi .sub { font-size: 12px; color: var(--dim); margin-top: 2px; }
  .kpi.accent .value { color: var(--accent); }
  .kpi.good .value { color: var(--good); }
  .kpi.bad .value { color: var(--bad); }

  .tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--border);
          margin-bottom: 20px; overflow-x: auto; }
  .tab { background: none; border: none; border-bottom: 2px solid transparent;
         color: var(--muted); font-size: 14px; font-weight: 500; cursor: pointer;
         padding: 10px 14px; white-space: nowrap; font-family: inherit; }
  .tab:hover { color: var(--text); }
  .tab[aria-selected="true"] { color: var(--accent); border-bottom-color: var(--accent); }
  .tab .count { font-size: 11px; color: var(--dim); margin-left: 5px; }
  .panel[hidden] { display: none; }

  .card { background: var(--panel); border: 1px solid var(--border); border-radius: 12px;
          padding: 18px 20px; margin-bottom: 18px; box-shadow: var(--shadow); }
  .card > h2 { font-size: 13px; text-transform: uppercase; letter-spacing: 0.06em;
               color: var(--muted); font-weight: 600; margin-bottom: 4px; }
  .card > .hint { font-size: 12.5px; color: var(--dim); margin-bottom: 14px; }
  .cols2 { display: grid; grid-template-columns: 1.35fr 1fr; gap: 18px; align-items: start; }
  @media (max-width: 900px) { .cols2 { grid-template-columns: 1fr; } }

  .toolbar { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; margin-bottom: 14px; }
  .search { position: relative; flex: 1 1 240px; }
  .search input {
    width: 100%; background: var(--panel-2); border: 1px solid var(--border);
    color: var(--text); border-radius: 9px; padding: 8px 12px 8px 32px;
    font-size: 14px; font-family: inherit;
  }
  .search input:focus { outline: none; border-color: var(--accent);
                        box-shadow: 0 0 0 3px var(--accent-soft); }
  .search svg { position: absolute; left: 10px; top: 50%; transform: translateY(-50%);
                width: 14px; height: 14px; stroke: var(--dim); fill: none; stroke-width: 2; }
  .search kbd { position: absolute; right: 10px; top: 50%; transform: translateY(-50%);
                font-size: 11px; color: var(--dim); border: 1px solid var(--border);
                border-radius: 4px; padding: 1px 5px; font-family: inherit; }
  select.control {
    background: var(--panel-2); border: 1px solid var(--border); color: var(--text);
    border-radius: 9px; padding: 8px 10px; font-size: 13px; font-family: inherit; cursor: pointer;
  }
  .switch { display: inline-flex; align-items: center; gap: 7px; font-size: 13px;
            color: var(--muted); cursor: pointer; user-select: none; }
  .switch input { accent-color: var(--accent); width: 15px; height: 15px; cursor: pointer; }

  .chips { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 14px; }
  .chip { background: var(--panel-2); border: 1px solid var(--border); color: var(--muted);
          border-radius: 999px; padding: 4px 11px; font-size: 12.5px; cursor: pointer;
          font-family: inherit; display: inline-flex; align-items: center; gap: 6px; }
  .chip:hover { color: var(--text); }
  .chip[aria-pressed="true"] { background: var(--accent-soft); border-color: var(--accent);
                               color: var(--accent); }
  .chip .dot { width: 8px; height: 8px; border-radius: 50%; }
  .chip .n { color: var(--dim); font-size: 11px; }

  table { width: 100%; border-collapse: collapse; font-size: 13.5px; }
  thead th { text-align: left; padding: 9px 12px; color: var(--muted); font-size: 11px;
             text-transform: uppercase; letter-spacing: 0.05em; font-weight: 600;
             border-bottom: 1px solid var(--border); white-space: nowrap; }
  thead th.sortable { cursor: pointer; user-select: none; }
  thead th.sortable:hover { color: var(--text); }
  thead th .arrow { opacity: 0.45; font-size: 9px; margin-left: 3px; }
  tbody td { padding: 9px 12px; border-bottom: 1px solid var(--border); vertical-align: middle; }
  tbody tr.row { cursor: pointer; }
  tbody tr.row:hover td { background: var(--panel-2); }
  td.num { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .table-scroll { overflow-x: auto; }

  .site-cell { display: flex; align-items: center; gap: 10px; min-width: 0; }
  .avatar { width: 26px; height: 26px; border-radius: 7px; flex: none;
            display: grid; place-items: center; font-size: 12px; font-weight: 700;
            color: #0b0e14; }
  html[data-theme="light"] .avatar { color: #fff; }
  .site-name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .site-sub { font-size: 11.5px; color: var(--dim); }
  .cat { font-size: 11.5px; padding: 2px 8px; border-radius: 999px; white-space: nowrap;
         border: 1px solid transparent; }
  .sharebar { display: block; height: 4px; border-radius: 2px; background: var(--panel-2);
              overflow: hidden; min-width: 46px; margin-top: 4px; }
  .sharebar > i { display: block; height: 100%; background: var(--accent); }
  .caret { color: var(--dim); font-size: 10px; width: 10px; display: inline-block; }

  .detail td { background: var(--panel-2); padding: 0; }
  .detail-inner { padding: 14px 18px 18px 50px; display: grid;
                  grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); gap: 18px; }
  .detail-inner h4 { font-size: 11px; text-transform: uppercase; letter-spacing: 0.05em;
                     color: var(--muted); margin-bottom: 7px; font-weight: 600; }
  .kv { font-size: 12.5px; color: var(--muted); display: flex; justify-content: space-between;
        gap: 12px; padding: 2px 0; }
  .kv b { color: var(--text); font-weight: 500; }
  .hostlist { max-height: 190px; overflow-y: auto; }
  .hostlist .kv b { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; }

  .banner { background: color-mix(in srgb, var(--warn) 12%, var(--panel));
            border: 1px solid color-mix(in srgb, var(--warn) 45%, var(--border));
            border-radius: 12px; padding: 16px 18px; margin-bottom: 18px; }
  .banner h3 { font-size: 14px; margin-bottom: 6px; color: var(--warn); }
  .banner p { font-size: 13.5px; color: var(--muted); }
  .banner .facts { font-size: 12.5px; color: var(--dim); margin-top: 8px;
                   font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }

  .empty { text-align: center; padding: 46px 20px; color: var(--dim); }
  .empty .big { font-size: 15px; color: var(--muted); margin-bottom: 6px; }

  .legend { display: flex; flex-direction: column; gap: 7px; margin-top: 4px; }
  .legend-item { display: flex; align-items: center; gap: 8px; font-size: 13px; }
  .legend-item .dot { width: 9px; height: 9px; border-radius: 50%; flex: none; }
  .legend-item .name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .legend-item .val { color: var(--muted); font-variant-numeric: tabular-nums; }

  .tooltip { position: fixed; pointer-events: none; opacity: 0; transition: opacity .1s;
             background: var(--panel); border: 1px solid var(--border); border-radius: 8px;
             padding: 7px 10px; font-size: 12.5px; box-shadow: 0 4px 14px rgba(0,0,0,0.3);
             z-index: 50; white-space: nowrap; }
  .tooltip b { color: var(--accent); }

  .foot { text-align: center; padding: 26px; color: var(--dim); font-size: 12px; }
  .foot code { color: var(--muted); }
</style>
</head>
<body>

<div class="topbar">
  <div class="brand">Ghost&nbsp;Node <span>·</span> Traffic report</div>
  <div class="meta" id="topmeta"></div>
  <button class="icon-btn" id="themeToggle" title="Toggle light / dark">◐ Theme</button>
</div>

<div class="wrap">
  <div id="banner"></div>
  <div class="kpis" id="kpis"></div>

  <div class="tabs" role="tablist" id="tabs"></div>

  <div class="panel" id="panel-overview"></div>
  <div class="panel" id="panel-websites" hidden></div>
  <div class="panel" id="panel-timeline" hidden></div>
  <div class="panel" id="panel-clients" hidden></div>
  <div class="panel" id="panel-blocked" hidden></div>
</div>

<div class="foot">
  Generated by <code>analyze-vpn-traffic.sh</code> · ghost-node ·
  no external resources are loaded by this page
</div>

<div class="tooltip" id="tooltip"></div>

<script id="report-data" type="application/json">__REPORT_JSON__</script>
<script>
(function () {
  "use strict";
  var DATA = JSON.parse(document.getElementById("report-data").textContent);
  var CATS = {};
  DATA.categories.forEach(function (c) { CATS[c.key] = c; });

  // ── helpers ──────────────────────────────────────────────────────────────
  function esc(s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }
  function num(n) { return (n == null ? 0 : n).toLocaleString(); }
  function catColour(key) { return (CATS[key] && CATS[key].colour) || "#8b96a8"; }
  function catLabel(key) { return (CATS[key] && CATS[key].label) || key; }
  function when(s) {
    if (!s) return "—";
    var d = new Date(s.replace(" ", "T"));
    if (isNaN(d)) return s;
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" }) +
           ", " + d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  }
  function initials(domain) {
    var base = domain.replace(/^www\./, "").split(".")[0];
    return base.slice(0, 2).toUpperCase();
  }
  function emptyState(title, hint) {
    return '<div class="empty"><div class="big">' + esc(title) + "</div>" +
           (hint ? "<div>" + esc(hint) + "</div>" : "") + "</div>";
  }

  // ── charts (hand-rolled SVG: the report must work with no network) ───────
  var tip = document.getElementById("tooltip");
  function showTip(evt, html) {
    tip.innerHTML = html;
    tip.style.opacity = "1";
    var pad = 14;
    var x = Math.min(evt.clientX + pad, window.innerWidth - tip.offsetWidth - 8);
    var y = Math.max(evt.clientY - tip.offsetHeight - pad, 8);
    tip.style.left = x + "px";
    tip.style.top = y + "px";
  }
  function hideTip() { tip.style.opacity = "0"; }

  function hbars(items, opts) {
    opts = opts || {};
    if (!items.length) return emptyState("Nothing to show here");
    var max = Math.max.apply(null, items.map(function (i) { return i.value; })) || 1;
    return '<div class="legend">' + items.map(function (i) {
      var pct = Math.max(2, Math.round(i.value * 100 / max));
      return '<div class="legend-item" title="' + esc(i.label) + '">' +
        '<span class="dot" style="background:' + esc(i.colour || "#5aa2ff") + '"></span>' +
        '<span class="name">' + esc(i.label) + "</span>" +
        '<span style="flex:0 0 110px"><span class="sharebar" style="margin:0">' +
        '<i style="width:' + pct + "%;background:" + esc(i.colour || "#5aa2ff") + '"></i>' +
        "</span></span>" +
        '<span class="val">' + num(i.value) + "</span></div>";
    }).join("") + "</div>";
  }

  function donut(items, size) {
    size = size || 210;
    var total = items.reduce(function (a, b) { return a + b.value; }, 0);
    if (!total) return emptyState("No traffic in this window");
    var r = size / 2 - 3, ir = r - 32, cx = size / 2, cy = size / 2, angle = -Math.PI / 2;
    var paths = items.map(function (i) {
      var slice = (i.value / total) * Math.PI * 2;
      // A full circle cannot be drawn as one arc; nudge it closed.
      var end = angle + Math.min(slice, Math.PI * 1.9999);
      var large = slice > Math.PI ? 1 : 0;
      var p = [
        "M", cx + r * Math.cos(angle), cy + r * Math.sin(angle),
        "A", r, r, 0, large, 1, cx + r * Math.cos(end), cy + r * Math.sin(end),
        "L", cx + ir * Math.cos(end), cy + ir * Math.sin(end),
        "A", ir, ir, 0, large, 0, cx + ir * Math.cos(angle), cy + ir * Math.sin(angle), "Z"
      ].join(" ");
      angle = end;
      return '<path d="' + p + '" fill="' + esc(i.colour) + '" data-label="' + esc(i.label) +
             '" data-value="' + i.value + '" data-pct="' +
             (i.value * 100 / total).toFixed(1) + '"></path>';
    }).join("");
    return '<svg class="donut" viewBox="0 0 ' + size + " " + size + '" width="' + size +
           '" height="' + size + '" role="img">' + paths +
           '<text x="' + cx + '" y="' + (cy - 2) + '" text-anchor="middle" ' +
           'fill="currentColor" font-size="21" font-weight="700">' + num(total) + "</text>" +
           '<text x="' + cx + '" y="' + (cy + 16) + '" text-anchor="middle" ' +
           'fill="#8b98ad" font-size="11">connections</text></svg>';
  }

  function area(points, height) {
    height = height || 220;
    if (!points.length) return emptyState("No activity recorded");
    var w = 900, padL = 40, padB = 26, padT = 10;
    var max = Math.max.apply(null, points.map(function (p) { return p.connections; })) || 1;
    var innerW = w - padL - 10, innerH = height - padB - padT;
    var step = points.length > 1 ? innerW / (points.length - 1) : 0;
    var xy = points.map(function (p, i) {
      return [padL + i * step, padT + innerH - (p.connections / max) * innerH];
    });
    var line = xy.map(function (p) { return p[0].toFixed(1) + "," + p[1].toFixed(1); }).join(" ");
    var fill = padL + "," + (padT + innerH) + " " + line + " " +
               (padL + (points.length - 1) * step).toFixed(1) + "," + (padT + innerH);
    var grid = "", labels = "";
    for (var g = 0; g <= 3; g++) {
      var y = padT + (innerH / 3) * g;
      var v = Math.round(max - (max / 3) * g);
      grid += '<line x1="' + padL + '" y1="' + y + '" x2="' + (w - 10) + '" y2="' + y +
              '" stroke="currentColor" stroke-opacity="0.09"></line>' +
              '<text x="' + (padL - 7) + '" y="' + (y + 4) + '" text-anchor="end" ' +
              'fill="#8b98ad" font-size="10">' + v + "</text>";
    }
    var every = Math.max(1, Math.ceil(points.length / 9));
    points.forEach(function (p, i) {
      if (i % every) return;
      labels += '<text x="' + (padL + i * step) + '" y="' + (height - 8) +
                '" text-anchor="middle" fill="#8b98ad" font-size="10">' + esc(p.label) + "</text>";
    });
    var hits = points.map(function (p, i) {
      return '<rect x="' + (padL + i * step - step / 2) + '" y="' + padT +
             '" width="' + Math.max(step, 6) + '" height="' + innerH +
             '" fill="transparent" data-label="' + esc(p.label) +
             '" data-value="' + p.connections + '"></rect>';
    }).join("");
    return '<svg class="area" viewBox="0 0 ' + w + " " + height +
           '" preserveAspectRatio="none" style="width:100%;height:' + height + 'px">' +
           grid +
           '<polygon points="' + fill + '" fill="var(--accent)" fill-opacity="0.15"></polygon>' +
           '<polyline points="' + line + '" fill="none" stroke="var(--accent)" ' +
           'stroke-width="2" stroke-linejoin="round"></polyline>' +
           labels + hits + "</svg>";
  }

  function spark(timeline, hours) {
    if (!timeline.length) return "";
    var byHour = {};
    timeline.forEach(function (t) { byHour[t.hour] = t.connections; });
    var max = Math.max.apply(null, hours.map(function (h) { return byHour[h.hour] || 0; })) || 1;
    var w = 64, h = 18, bw = hours.length ? w / hours.length : w;
    var bars = hours.map(function (hr, i) {
      var v = byHour[hr.hour] || 0;
      var bh = v ? Math.max(1.5, (v / max) * h) : 0;
      return bh ? '<rect x="' + (i * bw).toFixed(2) + '" y="' + (h - bh).toFixed(2) +
                  '" width="' + Math.max(0.8, bw - 0.6).toFixed(2) + '" height="' + bh.toFixed(2) +
                  '" fill="var(--accent)" fill-opacity="0.75"></rect>' : "";
    }).join("");
    return '<svg width="' + w + '" height="' + h + '" viewBox="0 0 ' + w + " " + h + '">' +
           bars + "</svg>";
  }

  // ── header + KPIs ────────────────────────────────────────────────────────
  var m = DATA.meta, s = DATA.summary;
  document.getElementById("topmeta").innerHTML =
    "<span>server <b>" + esc(m.server) + "</b> " + esc(m.serverIP) + "</span>" +
    "<span>window <b>last " + esc(m.windowHours) + "h</b></span>" +
    "<span>generated <b>" + esc(m.generated) + "</b></span>";

  if (DATA.diagnosis) {
    document.getElementById("banner").innerHTML =
      '<div class="banner"><h3>No connections in this report</h3><p>' +
      esc(DATA.diagnosis) + '</p><div class="facts">' +
      num(m.linesRead) + " lines scanned · " + num(m.linesWithTimestamp) +
      " with timestamps · " + num(m.linesInWindow) + " in the last " + esc(m.windowHours) + "h" +
      (m.newestEntry ? " · newest entry " + esc(m.newestEntry) : "") +
      "</div></div>";
  }

  document.getElementById("kpis").innerHTML = [
    { label: "Websites visited", value: num(s.sitesVisited),
      sub: s.busiestSite ? "most active: " + s.busiestSite : "distinct sites", cls: "accent" },
    { label: "Connections", value: num(s.connections),
      sub: num(s.uniqueHosts) + " unique hostnames" },
    { label: "Background hosts", value: num(s.backgroundSites),
      sub: "CDN, telemetry, updates" },
    { label: "Blocked", value: num(s.rejected),
      sub: s.rejected ? "rejected or routed to block" : "nothing blocked",
      cls: s.rejected ? "bad" : "" },
    { label: "Busiest hour", value: s.peakHour || "—",
      sub: s.peakConnections ? num(s.peakConnections) + " connections" : "no activity" },
    { label: "Data in / out", value: DATA.system.rxHuman,
      sub: DATA.system.txHuman + " sent · since boot" }
  ].map(function (k) {
    return '<div class="kpi ' + (k.cls || "") + '"><div class="label">' + esc(k.label) +
      '</div><div class="value">' + esc(k.value) + '</div><div class="sub">' +
      esc(k.sub) + "</div></div>";
  }).join("");

  // ── tabs ─────────────────────────────────────────────────────────────────
  var TABS = [
    { id: "overview", label: "Overview" },
    { id: "websites", label: "Websites", count: DATA.visited.length },
    { id: "timeline", label: "Timeline", count: DATA.timeline.length },
    { id: "clients", label: "Clients", count: DATA.clients.length },
    { id: "blocked", label: "Blocked", count: DATA.rejected.length }
  ];
  var tabsEl = document.getElementById("tabs");
  tabsEl.innerHTML = TABS.map(function (t, i) {
    return '<button class="tab" role="tab" id="tab-' + t.id + '" data-tab="' + t.id +
      '" aria-selected="' + (i === 0) + '" aria-controls="panel-' + t.id + '">' +
      esc(t.label) + (t.count != null ? '<span class="count">' + num(t.count) + "</span>" : "") +
      "</button>";
  }).join("");

  function selectTab(id) {
    TABS.forEach(function (t) {
      var btn = document.getElementById("tab-" + t.id);
      var panel = document.getElementById("panel-" + t.id);
      var on = t.id === id;
      btn.setAttribute("aria-selected", String(on));
      panel.hidden = !on;
    });
    if (location.hash.slice(1) !== id) history.replaceState(null, "", "#" + id);
  }
  tabsEl.addEventListener("click", function (e) {
    var btn = e.target.closest(".tab");
    if (btn) selectTab(btn.dataset.tab);
  });
  tabsEl.addEventListener("keydown", function (e) {
    if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
    var i = TABS.findIndex(function (t) {
      return document.getElementById("tab-" + t.id).getAttribute("aria-selected") === "true";
    });
    var next = TABS[(i + (e.key === "ArrowRight" ? 1 : TABS.length - 1)) % TABS.length];
    selectTab(next.id);
    document.getElementById("tab-" + next.id).focus();
  });

  // ── overview ─────────────────────────────────────────────────────────────
  var topVisited = DATA.visited.slice(0, 12).map(function (v) {
    return { label: v.domain, value: v.connections, colour: catColour(v.category) };
  });
  var catItems = DATA.categories.map(function (c) {
    return { label: c.label, value: c.connections, colour: c.colour };
  });

  document.getElementById("panel-overview").innerHTML =
    '<div class="cols2">' +
      '<div class="card"><h2>Websites visited</h2>' +
        '<div class="hint">Sites a person actually opened — CDN, telemetry and update ' +
        'traffic is counted separately.</div>' + hbars(topVisited) + "</div>" +
      '<div class="card"><h2>By category</h2><div class="hint">All destinations.</div>' +
        '<div style="display:flex;gap:16px;align-items:center;flex-wrap:wrap">' +
        '<div id="catDonut">' + donut(catItems) + "</div>" +
        '<div style="flex:1;min-width:150px">' + hbars(catItems) + "</div></div></div>" +
    "</div>" +
    '<div class="cols2">' +
      '<div class="card"><h2>Activity</h2><div class="hint">Connections per hour.</div>' +
        '<div id="miniTimeline">' + area(DATA.timeline, 180) + "</div></div>" +
      '<div class="card"><h2>Ports &amp; routing</h2>' +
        '<div class="hint">What the traffic looked like on the wire.</div>' +
        hbars(DATA.ports.map(function (p) {
          return { label: p.name + " (" + p.port + ")", value: p.connections };
        })) +
        (DATA.routing.length ? '<h2 style="margin-top:16px">Outbound</h2>' +
          hbars(DATA.routing.map(function (r) {
            return { label: r.outbound || "unknown", value: r.connections,
                     colour: r.outbound === "block" ? "#ff6b6b" : "#3ddc97" };
          })) : "") +
      "</div>" +
    "</div>";

  // ── websites table ───────────────────────────────────────────────────────
  var state = { q: "", cat: "all", sort: "connections", dir: -1, background: false, open: {} };

  var COLUMNS = [
    { key: "domain", label: "Site" },
    { key: "category", label: "Category" },
    { key: "connections", label: "Connections", num: true },
    { key: "hostCount", label: "Hosts", num: true },
    { key: "activeHours", label: "Active hrs", num: true },
    { key: "first", label: "First seen" },
    { key: "last", label: "Last seen" },
    { key: "activity", label: "Activity", sortable: false }
  ];

  function visibleSites() {
    var rows = state.background ? DATA.sites : DATA.visited;
    var q = state.q.trim().toLowerCase();
    return rows.filter(function (r) {
      if (state.cat !== "all" && r.category !== state.cat) return false;
      if (!q) return true;
      if (r.domain.toLowerCase().indexOf(q) >= 0) return true;
      if (r.label.toLowerCase().indexOf(q) >= 0) return true;
      return r.hosts.some(function (h) { return h.host.toLowerCase().indexOf(q) >= 0; });
    }).slice().sort(function (a, b) {
      var x = a[state.sort], y = b[state.sort];
      if (typeof x === "string") { x = x || ""; y = y || ""; return x < y ? state.dir : x > y ? -state.dir : 0; }
      return (y - x) * (state.dir < 0 ? 1 : -1);
    });
  }

  function detailRow(site) {
    var ports = site.ports.map(function (p) {
      return '<div class="kv"><span>port ' + p.port + "</span><b>" + num(p.connections) + "</b></div>";
    }).join("");
    var hosts = site.hosts.map(function (h) {
      return '<div class="kv"><b>' + esc(h.host) + "</b><span>" + num(h.connections) + "</span></div>";
    }).join("");
    var who = site.clients.map(function (c) {
      return '<div class="kv"><span>' + esc(c.client) + "</span><b>" + num(c.connections) + "</b></div>";
    }).join("");
    return '<tr class="detail"><td colspan="' + COLUMNS.length + '"><div class="detail-inner">' +
      "<div><h4>Hostnames (" + site.hostCount + ")</h4><div class=\"hostlist\">" + hosts + "</div></div>" +
      "<div><h4>Ports</h4>" + ports +
        '<div class="kv" style="margin-top:8px"><span>Share of traffic</span><b>' +
        site.share + "%</b></div>" +
        (site.rejected ? '<div class="kv"><span>Blocked</span><b style="color:var(--bad)">' +
          num(site.rejected) + "</b></div>" : "") + "</div>" +
      "<div><h4>Clients</h4>" + (who || '<div class="kv"><span>unknown</span></div>') +
        '<div class="kv" style="margin-top:8px"><span>First seen</span><b>' + when(site.first) + "</b></div>" +
        '<div class="kv"><span>Last seen</span><b>' + when(site.last) + "</b></div></div>" +
      "</div></td></tr>";
  }

  function renderTable() {
    var rows = visibleSites();
    var body = document.getElementById("sitesBody");
    if (!rows.length) {
      body.innerHTML = '<tr><td colspan="' + COLUMNS.length + '">' +
        emptyState("No sites match this filter",
                   state.background ? "Try clearing the search." :
                   "Turn on “include background traffic” to see CDN and telemetry hosts.") +
        "</td></tr>";
      document.getElementById("shown").textContent = "0";
      return;
    }
    var maxConns = rows.reduce(function (a, r) { return Math.max(a, r.connections); }, 1);
    body.innerHTML = rows.map(function (r) {
      var colour = catColour(r.category);
      var open = !!state.open[r.domain];
      var barPct = Math.max(3, Math.round(r.connections * 100 / maxConns));
      return '<tr class="row" data-domain="' + esc(r.domain) + '">' +
        '<td><div class="site-cell"><span class="caret">' + (open ? "▾" : "▸") + "</span>" +
          '<span class="avatar" style="background:' + esc(colour) + '">' + esc(initials(r.domain)) + "</span>" +
          '<span style="min-width:0"><div class="site-name">' + esc(r.domain) + "</div>" +
          '<div class="site-sub">' + esc(r.label) +
          (r.hostCount > 1 ? " · " + r.hostCount + " hosts" : "") + "</div></span></div></td>" +
        '<td><span class="cat" style="color:' + esc(colour) + ";background:" + esc(colour) +
          '1a;border-color:' + esc(colour) + '33">' + esc(catLabel(r.category)) + "</span></td>" +
        '<td class="num">' + num(r.connections) +
          '<div class="sharebar" title="' + r.share + '% of all connections">' +
          '<i style="width:' + barPct + "%;background:" + esc(colour) + '"></i></div></td>' +
        '<td class="num">' + num(r.hostCount) + "</td>" +
        '<td class="num">' + num(r.activeHours) + "</td>" +
        "<td>" + esc(when(r.first)) + "</td>" +
        "<td>" + esc(when(r.last)) + "</td>" +
        "<td>" + spark(r.timeline, DATA.timeline) + "</td></tr>" +
        (open ? detailRow(r) : "");
    }).join("");
    document.getElementById("shown").textContent = num(rows.length);
  }

  function categoryCounts() {
    var counts = {};
    (state.background ? DATA.sites : DATA.visited).forEach(function (r) {
      counts[r.category] = (counts[r.category] || 0) + 1;
    });
    return counts;
  }

  function renderChips() {
    var counts = categoryCounts();
    var total = (state.background ? DATA.sites : DATA.visited).length;
    var html = '<button class="chip" data-cat="all" aria-pressed="' +
      (state.cat === "all") + '">All<span class="n">' + num(total) + "</span></button>";
    DATA.categories.forEach(function (c) {
      if (!counts[c.key]) return;
      html += '<button class="chip" data-cat="' + esc(c.key) + '" aria-pressed="' +
        (state.cat === c.key) + '"><span class="dot" style="background:' + esc(c.colour) +
        '"></span>' + esc(c.label) + '<span class="n">' + num(counts[c.key]) + "</span></button>";
    });
    document.getElementById("chips").innerHTML = html;
  }

  document.getElementById("panel-websites").innerHTML =
    '<div class="card">' +
      "<h2>Websites &amp; destinations</h2>" +
      '<div class="hint">Grouped by registrable domain — click any row for its hostnames, ' +
      'ports and clients. Showing <b id="shown">0</b> of ' + num(DATA.sites.length) + " known domains.</div>" +
      '<div class="toolbar">' +
        '<label class="search"><svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"/>' +
        '<path d="m20 20-3.5-3.5"/></svg>' +
        '<input id="q" type="search" placeholder="Search domain or hostname…" ' +
        'aria-label="Search sites"><kbd>/</kbd></label>' +
        '<select class="control" id="sort">' +
          '<option value="connections">Most connections</option>' +
          '<option value="last">Most recent</option>' +
          '<option value="first">First seen</option>' +
          '<option value="hostCount">Most hostnames</option>' +
          '<option value="domain">Domain A–Z</option>' +
        "</select>" +
        '<label class="switch"><input type="checkbox" id="bg">include background traffic</label>' +
      "</div>" +
      '<div class="chips" id="chips"></div>' +
      '<div class="table-scroll"><table><thead><tr>' +
        COLUMNS.map(function (c) {
          return "<th" + (c.sortable === false ? "" : ' class="sortable" data-key="' + c.key + '"') +
            (c.num ? ' style="text-align:right"' : "") + ">" + esc(c.label) +
            (c.sortable === false ? "" : '<span class="arrow">↕</span>') + "</th>";
        }).join("") +
      '</tr></thead><tbody id="sitesBody"></tbody></table></div>' +
    "</div>";

  document.getElementById("q").addEventListener("input", function (e) {
    state.q = e.target.value; renderTable();
  });
  document.getElementById("sort").addEventListener("change", function (e) {
    state.sort = e.target.value;
    state.dir = (e.target.value === "domain" || e.target.value === "first") ? 1 : -1;
    renderTable();
  });
  document.getElementById("bg").addEventListener("change", function (e) {
    state.background = e.target.checked;
    // Dropping background rows can strip the active filter of every match.
    if (state.cat !== "all" && !categoryCounts()[state.cat]) state.cat = "all";
    renderChips();
    renderTable();
  });
  document.getElementById("chips").addEventListener("click", function (e) {
    var chip = e.target.closest(".chip");
    if (!chip) return;
    state.cat = chip.dataset.cat;
    renderChips();
    renderTable();
  });
  document.getElementById("sitesBody").addEventListener("click", function (e) {
    var row = e.target.closest("tr.row");
    if (!row) return;
    var d = row.dataset.domain;
    state.open[d] = !state.open[d];
    renderTable();
  });
  document.querySelector("#panel-websites thead").addEventListener("click", function (e) {
    var th = e.target.closest("th.sortable");
    if (!th) return;
    if (state.sort === th.dataset.key) state.dir = -state.dir;
    else { state.sort = th.dataset.key; state.dir = -1; }
    renderTable();
  });

  // ── timeline ─────────────────────────────────────────────────────────────
  var busiest = DATA.timeline.slice().sort(function (a, b) {
    return b.connections - a.connections;
  }).slice(0, 8);
  document.getElementById("panel-timeline").innerHTML =
    '<div class="card"><h2>Connections per hour</h2>' +
      '<div class="hint">Hover for exact counts. Gaps are hours with no traffic at all.</div>' +
      area(DATA.timeline, 260) + "</div>" +
    '<div class="cols2"><div class="card"><h2>Busiest hours</h2>' +
      hbars(busiest.map(function (h) { return { label: h.label, value: h.connections }; })) +
    "</div>" +
    '<div class="card"><h2>Window</h2>' +
      '<div class="kv"><span>From</span><b>' + esc(m.windowStart) + "</b></div>" +
      '<div class="kv"><span>To</span><b>' + esc(m.windowEnd) + "</b></div>" +
      '<div class="kv"><span>Hours with traffic</span><b>' + num(s.activeHours) + " of " +
        num(DATA.timeline.length) + "</b></div>" +
      '<div class="kv"><span>Log file</span><b class="mono">' + esc(m.logPath) + "</b></div>" +
      '<div class="kv"><span>Lines scanned</span><b>' + num(m.linesRead) + "</b></div>" +
    "</div></div>";

  // ── clients ──────────────────────────────────────────────────────────────
  var clientsHTML = DATA.clients.length ?
    '<div class="table-scroll"><table><thead><tr><th>Client</th>' +
    '<th style="text-align:right">Connections</th><th style="text-align:right">Sites</th>' +
    "<th>First seen</th><th>Last seen</th></tr></thead><tbody>" +
    DATA.clients.map(function (c) {
      return "<tr><td><span class=\"mono\">" + esc(c.client) + "</span></td>" +
        '<td class="num">' + num(c.connections) + "</td>" +
        '<td class="num">' + num(c.sites) + "</td>" +
        "<td>" + esc(when(c.first)) + "</td><td>" + esc(when(c.last)) + "</td></tr>";
    }).join("") + "</tbody></table></div>" :
    emptyState("No client identifiers in the log",
               "Xray records these when inbound users have an email set.");

  var statsHTML = DATA.xrayStats.length ?
    '<div class="card"><h2>Xray byte counters</h2>' +
    '<div class="hint">Live totals from the stats API, since Xray last restarted.</div>' +
    DATA.xrayStats.map(function (r) {
      return '<div class="kv"><span class="mono">' + esc(r.name) + "</span><b>" +
        esc(r.human) + "</b></div>";
    }).join("") + "</div>" : "";

  document.getElementById("panel-clients").innerHTML =
    '<div class="card"><h2>Clients</h2>' +
    '<div class="hint">Identified by inbound email tag, falling back to source address.</div>' +
    clientsHTML + "</div>" + statsHTML;

  // ── blocked ──────────────────────────────────────────────────────────────
  document.getElementById("panel-blocked").innerHTML =
    '<div class="card"><h2>Blocked &amp; rejected</h2>' +
    '<div class="hint">Destinations Xray refused — private ranges, the block outbound, ' +
    "or connections it could not complete.</div>" +
    (DATA.rejected.length ?
      '<div class="table-scroll"><table><thead><tr><th>Host</th>' +
      '<th style="text-align:right">Port</th><th style="text-align:right">Attempts</th>' +
      "</tr></thead><tbody>" +
      DATA.rejected.map(function (r) {
        return '<tr><td class="mono">' + esc(r.host) + "</td>" +
          '<td class="num">' + r.port + "</td>" +
          '<td class="num">' + num(r.connections) + "</td></tr>";
      }).join("") + "</tbody></table></div>" :
      emptyState("Nothing was blocked", "Every connection in this window was allowed through.")) +
    "</div>";

  // ── interactions ─────────────────────────────────────────────────────────
  document.addEventListener("mouseover", function (e) {
    var t = e.target;
    if (t.tagName === "path" && t.dataset.label) {
      showTip(e, "<b>" + esc(t.dataset.label) + "</b> " + num(+t.dataset.value) +
                 " (" + t.dataset.pct + "%)");
    } else if (t.tagName === "rect" && t.dataset.label) {
      showTip(e, "<b>" + esc(t.dataset.label) + "</b> " + num(+t.dataset.value) + " connections");
    }
  });
  document.addEventListener("mouseout", function (e) {
    if (e.target.dataset && e.target.dataset.label) hideTip();
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "/" && document.activeElement.tagName !== "INPUT") {
      e.preventDefault();
      selectTab("websites");
      document.getElementById("q").focus();
    }
  });

  var themeBtn = document.getElementById("themeToggle");
  var stored = null;
  try { stored = localStorage.getItem("ghost-report-theme"); } catch (err) { /* private mode */ }
  if (stored) document.documentElement.setAttribute("data-theme", stored);
  themeBtn.addEventListener("click", function () {
    var next = document.documentElement.getAttribute("data-theme") === "light" ? "dark" : "light";
    document.documentElement.setAttribute("data-theme", next);
    try { localStorage.setItem("ghost-report-theme", next); } catch (err) { /* ignore */ }
  });

  renderChips();
  renderTable();
  var initial = location.hash.slice(1);
  selectTab(TABS.some(function (t) { return t.id === initial; }) ? initial : "overview");
})();
</script>
</body>
</html>
"""

payload = json.dumps(report, ensure_ascii=False, separators=(",", ":"))
# Keep the JSON from terminating the surrounding <script> block.
payload = payload.replace("</", "<\\/")

with open(OUT, "w") as handle:
    handle.write(HTML_TEMPLATE.replace("__REPORT_JSON__", payload))

if JSON_OUT:
    with open(JSON_OUT, "w") as handle:
        json.dump(report, handle, indent=2)

if not QUIET:
    print("  report → %s" % OUT)
    if JSON_OUT:
        print("  json   → %s" % JSON_OUT)
    print("")
PYEOF

success "Report written to $OUT"
if [[ -z "$QUIET" ]]; then
  echo ""
  echo -e "${BOLD}  View it from your machine:${NC}"
  echo -e "  bash scripts/view-vpn-report.sh --hours $HOURS"
  echo ""
fi
