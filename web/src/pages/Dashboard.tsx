import { useEffect, useRef, useState } from "react";
import QRCode from "qrcode";
import { useVPNStore } from "@/store/vpnStore";
import { type Node } from "@/api/client";
import { Power, LogOut, AlertCircle, Copy, Check, X, Download, Smartphone, Monitor, Router } from "lucide-react";

// ─── Utils ───────────────────────────────────────────────────────────────────

function fmt(b: number) {
  if (b >= 1e9) return (b / 1e9).toFixed(1) + " GB";
  if (b >= 1e6) return (b / 1e6).toFixed(0) + " MB";
  if (b >= 1e3) return (b / 1e3).toFixed(0) + " KB";
  return b + " B";
}

function flag(cc: string) {
  if (!cc || cc.length < 2) return "🌐";
  return [...cc.toUpperCase().slice(0, 2)]
    .map((c) => String.fromCodePoint(c.charCodeAt(0) + 127397))
    .join("");
}

// ─── Copy button ─────────────────────────────────────────────────────────────

function CopyButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };
  return (
    <button
      onClick={copy}
      style={{
        display: "flex", alignItems: "center", gap: 6,
        padding: "7px 14px", borderRadius: 6, cursor: "pointer",
        background: copied ? "rgba(34,197,94,.15)" : "var(--raised)",
        border: `1px solid ${copied ? "rgba(34,197,94,.3)" : "var(--border)"}`,
        color: copied ? "var(--green)" : "var(--text-2)",
        fontSize: 13, fontWeight: 500, transition: "all .15s",
      }}
    >
      {copied ? <Check size={14} /> : <Copy size={14} />}
      {copied ? "Copied!" : label}
    </button>
  );
}

// ─── QR canvas ───────────────────────────────────────────────────────────────

function QRCanvas({ uri }: { uri: string }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    if (ref.current && uri) {
      QRCode.toCanvas(ref.current, uri, {
        width: 200,
        margin: 2,
        color: { dark: "#ffffff", light: "#111111" },
      });
    }
  }, [uri]);
  return (
    <canvas
      ref={ref}
      style={{ borderRadius: 8, border: "1px solid var(--border)", display: "block" }}
    />
  );
}

// ─── Config modal ─────────────────────────────────────────────────────────────

function ConfigModal({ onClose }: { onClose: () => void }) {
  const { activeConfig, selectedNode } = useVPNStore();
  if (!activeConfig) return null;

  const uri = activeConfig.vless_uri;
  const nodeName = activeConfig.node.name;
  const [tab, setTab] = useState<"qr" | "uri" | "apps">("qr");

  const apps = [
    { name: "v2rayN", platform: "Windows", icon: <Monitor size={16} />, url: "https://github.com/2dust/v2rayN/releases" },
    { name: "v2rayU / Shadowrocket", platform: "macOS / iOS", icon: <Smartphone size={16} />, url: "#" },
    { name: "v2rayNG", platform: "Android", icon: <Smartphone size={16} />, url: "https://github.com/2dust/v2rayNG/releases" },
    { name: "Sing-box", platform: "All platforms", icon: <Router size={16} />, url: "https://sing-box.sagernet.org" },
  ];

  return (
    <div
      style={{
        position: "fixed", inset: 0, zIndex: 50,
        background: "rgba(0,0,0,.7)",
        display: "flex", alignItems: "center", justifyContent: "center",
        padding: 24,
      }}
      onClick={(e) => e.target === e.currentTarget && onClose()}
    >
      <div style={{
        background: "var(--surface)", border: "1px solid var(--border)",
        borderRadius: 16, width: "100%", maxWidth: 480,
        boxShadow: "0 24px 64px rgba(0,0,0,.6)",
      }}>
        {/* Header */}
        <div style={{
          display: "flex", alignItems: "center", justifyContent: "space-between",
          padding: "18px 20px", borderBottom: "1px solid var(--border)",
        }}>
          <div>
            <p style={{ fontWeight: 600, fontSize: 15, color: "var(--text-1)" }}>
              Connection Config
            </p>
            <p style={{ fontSize: 12, color: "var(--text-3)", marginTop: 2 }}>
              {flag(selectedNode?.country ?? "")} {nodeName} · Import into your VPN client
            </p>
          </div>
          <button
            onClick={onClose}
            style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-3)", display: "flex" }}
          >
            <X size={18} />
          </button>
        </div>

        {/* Tabs */}
        <div style={{ display: "flex", borderBottom: "1px solid var(--border)" }}>
          {(["qr", "uri", "apps"] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              style={{
                flex: 1, padding: "10px 0", background: "none", border: "none", cursor: "pointer",
                fontSize: 13, fontWeight: 500,
                color: tab === t ? "var(--text-1)" : "var(--text-3)",
                borderBottom: tab === t ? "2px solid var(--green)" : "2px solid transparent",
                transition: "color .15s",
              }}
            >
              {t === "qr" ? "QR Code" : t === "uri" ? "URI" : "Apps"}
            </button>
          ))}
        </div>

        {/* Body */}
        <div style={{ padding: 24 }}>
          {tab === "qr" && (
            <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 16 }}>
              {uri ? (
                <>
                  <QRCanvas uri={uri} />
                  <p style={{ fontSize: 12, color: "var(--text-3)", textAlign: "center", lineHeight: 1.5 }}>
                    Open your VPN client → Add server → Scan QR code
                  </p>
                  <CopyButton text={uri} label="Copy URI instead" />
                </>
              ) : (
                <p style={{ color: "var(--text-3)", fontSize: 13 }}>
                  No URI available for this server. Check the transport profile config.
                </p>
              )}
            </div>
          )}

          {tab === "uri" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <p style={{ fontSize: 12, color: "var(--text-3)" }}>
                Copy this URI and paste it into your VPN client (v2rayN, v2rayNG, Shadowrocket, etc.)
              </p>
              <div style={{
                background: "var(--raised)", border: "1px solid var(--border)",
                borderRadius: 8, padding: "12px 14px",
                fontFamily: "monospace", fontSize: 11,
                color: "var(--text-2)", wordBreak: "break-all", lineHeight: 1.6,
              }}>
                {uri || "No URI — transport profile may be missing required fields."}
              </div>
              {uri && (
                <div style={{ display: "flex", gap: 8 }}>
                  <CopyButton text={uri} label="Copy VLESS URI" />
                  <button
                    onClick={() => {
                      const blob = new Blob([uri], { type: "text/plain" });
                      const a = document.createElement("a");
                      a.href = URL.createObjectURL(blob);
                      a.download = `${nodeName}-config.txt`;
                      a.click();
                    }}
                    style={{
                      display: "flex", alignItems: "center", gap: 6,
                      padding: "7px 14px", borderRadius: 6, cursor: "pointer",
                      background: "var(--raised)", border: "1px solid var(--border)",
                      color: "var(--text-2)", fontSize: 13, fontWeight: 500,
                    }}
                  >
                    <Download size={14} /> Save file
                  </button>
                </div>
              )}
            </div>
          )}

          {tab === "apps" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              <p style={{ fontSize: 12, color: "var(--text-3)", marginBottom: 4 }}>
                Download a compatible VPN client, then import the QR code or URI.
              </p>
              {apps.map((app) => (
                <a
                  key={app.name}
                  href={app.url}
                  target="_blank"
                  rel="noreferrer"
                  style={{
                    display: "flex", alignItems: "center", gap: 12,
                    padding: "12px 14px", borderRadius: 8, textDecoration: "none",
                    background: "var(--raised)", border: "1px solid var(--border)",
                    transition: "border-color .15s",
                  }}
                  onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.borderColor = "var(--text-3)")}
                  onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.borderColor = "var(--border)")}
                >
                  <span style={{ color: "var(--text-3)" }}>{app.icon}</span>
                  <div>
                    <p style={{ fontSize: 13, fontWeight: 600, color: "var(--text-1)" }}>{app.name}</p>
                    <p style={{ fontSize: 11, color: "var(--text-3)", marginTop: 1 }}>{app.platform}</p>
                  </div>
                </a>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// ─── Dashboard ───────────────────────────────────────────────────────────────

export default function Dashboard() {
  const {
    user, nodes, selectedNode, isConnected, isConnecting,
    activeTransport, activeConfig, usage, fetchNodes, fetchUsage,
    selectNode, connect, connectAuto, disconnect, logout,
  } = useVPNStore();

  const [error, setError] = useState("");
  const [showConfig, setShowConfig] = useState(false);

  useEffect(() => {
    fetchNodes().catch(() => setError("Failed to load servers."));
    fetchUsage().catch(() => null);
  }, [fetchNodes, fetchUsage]);

  // Auto-open config modal when connection succeeds
  useEffect(() => {
    if (isConnected && activeConfig) setShowConfig(true);
  }, [isConnected, activeConfig]);

  const toggle = async () => {
    if (isConnected) {
      disconnect();
      setShowConfig(false);
      return;
    }
    if (!selectedNode) return;
    setError("");
    try {
      await connect(selectedNode.id);
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } }).response?.data?.error;
      setError(msg ?? "Failed to fetch config.");
    }
  };

  const handleAutoConnect = async () => {
    if (isConnected) { disconnect(); setShowConfig(false); return; }
    setError("");
    try {
      await connectAuto();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } }).response?.data?.error;
      setError(msg ?? "No online servers available.");
    }
  };

  const pct = usage ? Math.min(usage.quota.used_percent, 100) : 0;
  const onlineN = nodes.filter((n) => n.status === "online").length;

  return (
    <>
      {showConfig && <ConfigModal onClose={() => setShowConfig(false)} />}

      <div style={{ display: "flex", flexDirection: "column", height: "100vh", background: "var(--bg)" }}>

        {/* ── Nav ──────────────────────────────────────────────────── */}
        <nav style={{
          display: "flex", alignItems: "center", justifyContent: "space-between",
          padding: "0 24px", height: 52, flexShrink: 0,
          borderBottom: "1px solid var(--border)", background: "var(--surface)",
        }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <span style={{ fontSize: 15, fontWeight: 600, letterSpacing: "-0.01em", color: "var(--text-1)" }}>
              VPN Platform
            </span>
            {user?.role === "admin" && (
              <span style={{
                fontSize: 10, fontWeight: 600, letterSpacing: "0.05em", textTransform: "uppercase",
                padding: "2px 7px", borderRadius: 4,
                background: "rgba(34,197,94,.1)", color: "var(--green)",
                border: "1px solid rgba(34,197,94,.2)",
              }}>
                Admin
              </span>
            )}
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
            <span style={{ fontSize: 13, color: "var(--text-3)" }}>{user?.email}</span>
            <button
              onClick={logout}
              style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-3)", display: "flex", alignItems: "center", gap: 6, fontSize: 13 }}
              onMouseEnter={(e) => ((e.currentTarget as HTMLElement).style.color = "var(--text-2)")}
              onMouseLeave={(e) => ((e.currentTarget as HTMLElement).style.color = "var(--text-3)")}
            >
              <LogOut size={14} />
              Sign out
            </button>
          </div>
        </nav>

        {/* ── Body ─────────────────────────────────────────────────── */}
        <div style={{ flex: 1, display: "flex", overflow: "hidden" }}>

          {/* ── Sidebar ──────────────────────────────────────────── */}
          <aside style={{
            width: 280, flexShrink: 0, display: "flex", flexDirection: "column",
            borderRight: "1px solid var(--border)", overflowY: "auto",
            background: "var(--surface)",
          }}>

            {/* Connect */}
            <div style={{ padding: "32px 24px 24px", display: "flex", flexDirection: "column", alignItems: "center", gap: 16 }}>
              <div style={{ position: "relative" }}>
                {isConnecting && (
                  <span className="spin" style={{
                    position: "absolute", inset: -5, borderRadius: "50%", display: "block",
                    border: "1.5px solid transparent",
                    borderTopColor: "var(--text-3)", borderRightColor: "var(--text-3)",
                  }} />
                )}
                <button
                  onClick={toggle}
                  disabled={isConnecting || (!isConnected && !selectedNode)}
                  className={isConnected ? "glow-green" : ""}
                  style={{
                    width: 88, height: 88, borderRadius: "50%",
                    display: "flex", alignItems: "center", justifyContent: "center",
                    cursor: (!isConnected && !selectedNode) || isConnecting ? "not-allowed" : "pointer",
                    background: isConnected ? "var(--green)" : "var(--raised)",
                    border: isConnected ? "1px solid var(--green)" : "1px solid var(--border)",
                    transition: "background .2s, border-color .2s",
                    opacity: !isConnected && !selectedNode && !isConnecting ? 0.4 : 1,
                  }}
                >
                  <Power size={28} strokeWidth={1.8} color={isConnected ? "#000" : "var(--text-2)"} />
                </button>
              </div>

              <div style={{ textAlign: "center" }}>
                {isConnecting ? (
                  <p style={{ color: "var(--text-2)", fontSize: 13 }}>Fetching config…</p>
                ) : isConnected ? (
                  <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 6 }}>
                    <p style={{ fontWeight: 600, color: "var(--green)", fontSize: 14 }}>Config ready</p>
                    {activeTransport && (
                      <p style={{ color: "var(--text-3)", fontSize: 12 }}>via {activeTransport}</p>
                    )}
                    <button
                      onClick={() => setShowConfig(true)}
                      style={{
                        marginTop: 4, padding: "6px 14px", borderRadius: 6, cursor: "pointer",
                        background: "rgba(34,197,94,.1)", border: "1px solid rgba(34,197,94,.25)",
                        color: "var(--green)", fontSize: 12, fontWeight: 600,
                      }}
                    >
                      Show config →
                    </button>
                  </div>
                ) : (
                  <p style={{ color: "var(--text-3)", fontSize: 13 }}>
                    {selectedNode ? "Click to get config" : "Select a server first"}
                  </p>
                )}
              </div>
            </div>

            <div style={{ height: 1, background: "var(--border)" }} />

            {/* Selected server */}
            <div style={{ padding: "20px 24px" }}>
              <p style={{ fontSize: 11, fontWeight: 600, letterSpacing: "0.06em", textTransform: "uppercase", color: "var(--text-3)", marginBottom: 10 }}>
                Server
              </p>
              {selectedNode ? (
                <>
                  <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <span style={{ fontSize: 22 }}>{flag(selectedNode.country)}</span>
                    <div>
                      <p style={{ fontWeight: 600, fontSize: 14, color: "var(--text-1)" }}>{selectedNode.name}</p>
                      <p style={{ fontSize: 12, color: "var(--text-3)", marginTop: 1 }}>{selectedNode.region} · {selectedNode.country}</p>
                    </div>
                  </div>
                  <div style={{ display: "flex", gap: 8, marginTop: 12 }}>
                    <StatChip label="CPU" value={`${selectedNode.cpu_usage.toFixed(0)}%`} />
                    <StatChip label="Users" value={String(selectedNode.active_conns)} />
                  </div>
                </>
              ) : (
                <p style={{ fontSize: 13, color: "var(--text-3)" }}>None selected</p>
              )}
            </div>

            <div style={{ height: 1, background: "var(--border)" }} />

            {/* Quota */}
            <div style={{ padding: "20px 24px" }}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
                <p style={{ fontSize: 11, fontWeight: 600, letterSpacing: "0.06em", textTransform: "uppercase", color: "var(--text-3)" }}>
                  Bandwidth
                </p>
                {usage && (
                  <span style={{
                    fontSize: 10, fontWeight: 700, textTransform: "uppercase", letterSpacing: "0.05em",
                    color: usage.quota.plan === "free" ? "var(--text-3)" : "var(--green)",
                  }}>
                    {usage.quota.plan}
                  </span>
                )}
              </div>
              {usage ? (
                <>
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", marginBottom: 8 }}>
                    <span style={{ fontSize: 22, fontWeight: 700, letterSpacing: "-0.02em", color: "var(--text-1)" }}>
                      {fmt(usage.quota.used_bytes)}
                    </span>
                    <span style={{ fontSize: 12, color: "var(--text-3)" }}>/ {fmt(usage.quota.quota_bytes)}</span>
                  </div>
                  <div style={{ height: 3, background: "var(--raised)", borderRadius: 2, overflow: "hidden" }}>
                    <div style={{
                      height: "100%", borderRadius: 2, width: `${pct}%`,
                      background: pct >= 90 ? "var(--red)" : pct >= 70 ? "var(--amber)" : "var(--green)",
                      transition: "width 1s ease",
                    }} />
                  </div>
                  <div style={{ display: "flex", gap: 16, marginTop: 12 }}>
                    <div>
                      <p style={{ fontSize: 11, color: "var(--text-3)", marginBottom: 2 }}>↓ Down</p>
                      <p style={{ fontSize: 13, fontWeight: 600, color: "var(--text-1)" }}>{fmt(usage.period.bytes_in)}</p>
                    </div>
                    <div>
                      <p style={{ fontSize: 11, color: "var(--text-3)", marginBottom: 2 }}>↑ Up</p>
                      <p style={{ fontSize: 13, fontWeight: 600, color: "var(--text-1)" }}>{fmt(usage.period.bytes_out)}</p>
                    </div>
                  </div>
                  {usage.quota.expires_at && (
                    <p style={{ fontSize: 11, color: "var(--text-3)", marginTop: 10 }}>
                      Renews {new Date(usage.quota.expires_at).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}
                    </p>
                  )}
                </>
              ) : (
                <p style={{ fontSize: 13, color: "var(--text-3)" }}>Loading…</p>
              )}
            </div>

            {error && (
              <>
                <div style={{ height: 1, background: "var(--border)" }} />
                <div style={{ padding: "12px 24px", display: "flex", gap: 8, color: "var(--red)", fontSize: 13 }}>
                  <AlertCircle size={14} style={{ flexShrink: 0, marginTop: 1 }} />
                  {error}
                </div>
              </>
            )}
          </aside>

          {/* ── Server list ──────────────────────────────────────── */}
          <main style={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden" }}>
            <div style={{
              padding: "0 24px", height: 52, display: "flex", alignItems: "center",
              justifyContent: "space-between", borderBottom: "1px solid var(--border)", flexShrink: 0,
            }}>
              <p style={{ fontSize: 13, fontWeight: 600, color: "var(--text-1)" }}>Servers</p>
              <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                <p style={{ fontSize: 12, color: "var(--text-3)" }}>
                  <span style={{ color: "var(--green)" }}>{onlineN}</span> / {nodes.length} online
                </p>
                <button
                  onClick={handleAutoConnect}
                  disabled={isConnecting || onlineN === 0}
                  style={{
                    padding: "5px 12px", borderRadius: 6, cursor: isConnecting || onlineN === 0 ? "not-allowed" : "pointer",
                    background: isConnected ? "rgba(239,68,68,.1)" : "rgba(34,197,94,.1)",
                    border: `1px solid ${isConnected ? "rgba(239,68,68,.3)" : "rgba(34,197,94,.3)"}`,
                    color: isConnected ? "var(--red)" : "var(--green)",
                    fontSize: 12, fontWeight: 600,
                    opacity: isConnecting || (!isConnected && onlineN === 0) ? 0.4 : 1,
                    transition: "all .15s",
                  }}
                >
                  {isConnected ? "Disconnect" : "Auto-select best"}
                </button>
              </div>
            </div>

            <div style={{ flex: 1, overflowY: "auto" }}>
              {nodes.length === 0 ? (
                <div style={{ display: "flex", alignItems: "center", justifyContent: "center", height: "100%", color: "var(--text-3)", fontSize: 13 }}>
                  No servers found.
                </div>
              ) : (
                nodes.map((node, i) => (
                  <ServerRow
                    key={node.id}
                    node={node}
                    isSelected={selectedNode?.id === node.id}
                    isLast={i === nodes.length - 1}
                    onSelect={() => selectNode(node)}
                  />
                ))
              )}
            </div>
          </main>
        </div>
      </div>
    </>
  );
}

// ─── Small helpers ────────────────────────────────────────────────────────────

function StatChip({ label, value }: { label: string; value: string }) {
  return (
    <div style={{
      flex: 1, padding: "6px 10px", borderRadius: 6,
      background: "var(--raised)", border: "1px solid var(--border)",
    }}>
      <p style={{ fontSize: 11, color: "var(--text-3)", marginBottom: 2 }}>{label}</p>
      <p style={{ fontSize: 13, fontWeight: 600, color: "var(--text-1)" }}>{value}</p>
    </div>
  );
}

function ServerRow({ node, isSelected, isLast, onSelect }: {
  node: Node; isSelected: boolean; isLast: boolean; onSelect: () => void;
}) {
  const online = node.status === "online";
  return (
    <button
      onClick={onSelect}
      style={{
        width: "100%", textAlign: "left",
        padding: "14px 24px",
        background: isSelected ? "var(--surface)" : "transparent",
        border: "none",
        borderBottom: isLast ? "none" : "1px solid var(--border)",
        borderLeft: isSelected ? "2px solid var(--green)" : "2px solid transparent",
        cursor: "pointer",
        transition: "background .1s",
        display: "flex", alignItems: "center", gap: 14,
      }}
      onMouseEnter={(e) => { if (!isSelected) (e.currentTarget as HTMLElement).style.background = "var(--surface)"; }}
      onMouseLeave={(e) => { if (!isSelected) (e.currentTarget as HTMLElement).style.background = "transparent"; }}
    >
      <span style={{ fontSize: 20, lineHeight: 1, flexShrink: 0 }}>{flag(node.country)}</span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <p style={{ fontWeight: 500, fontSize: 14, color: "var(--text-1)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {node.name}
        </p>
        <p style={{ fontSize: 12, color: "var(--text-3)", marginTop: 1 }}>
          {node.region} · {node.country}
        </p>
      </div>
      {online && (
        <div style={{ display: "flex", gap: 16, flexShrink: 0, color: "var(--text-3)", fontSize: 12 }}>
          <span>{node.cpu_usage.toFixed(0)}% cpu</span>
          <span>{node.active_conns} users</span>
        </div>
      )}
      <div style={{ flexShrink: 0, display: "flex", alignItems: "center", gap: 6 }}>
        <span style={{
          width: 7, height: 7, borderRadius: "50%",
          background: online ? "var(--green)" : "var(--border)",
          boxShadow: online ? "0 0 6px var(--green)" : "none",
          display: "inline-block",
        }} />
        <span style={{ fontSize: 12, color: online ? "var(--text-2)" : "var(--text-3)" }}>
          {node.status}
        </span>
      </div>
    </button>
  );
}
