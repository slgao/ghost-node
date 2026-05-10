import { useEffect, useState } from "react";
import { useVPNStore } from "@/store/vpnStore";
import { type Node } from "@/api/client";
import { clsx } from "clsx";

export default function Dashboard() {
  const {
    user,
    nodes,
    selectedNode,
    isConnected,
    isConnecting,
    activeTransport,
    fetchNodes,
    selectNode,
    connect,
    disconnect,
    logout,
  } = useVPNStore();

  const [error, setError] = useState("");

  useEffect(() => {
    fetchNodes().catch(() => setError("Failed to load nodes"));
  }, [fetchNodes]);

  const handleConnect = async () => {
    if (!selectedNode) return;
    setError("");
    try {
      await connect(selectedNode.id);
    } catch {
      setError("Connection failed — trying a different server may help");
    }
  };

  return (
    <div className="min-h-screen bg-gray-950 text-white flex flex-col">
      {/* Topbar */}
      <header className="flex items-center justify-between px-6 py-4 border-b border-gray-800">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center font-bold text-sm">V</div>
          <span className="font-semibold text-lg">VPN Platform</span>
        </div>
        <div className="flex items-center gap-4">
          <span className="text-gray-400 text-sm">{user?.email}</span>
          <button
            onClick={logout}
            className="text-sm text-gray-400 hover:text-white transition-colors"
          >
            Sign out
          </button>
        </div>
      </header>

      <main className="flex-1 grid grid-cols-1 md:grid-cols-3 gap-6 p-6">
        {/* Status card */}
        <div className="md:col-span-1 flex flex-col gap-6">
          <div className="bg-gray-900 rounded-2xl p-6 border border-gray-800 flex flex-col items-center gap-4">
            {/* Big connect button */}
            <button
              onClick={isConnected ? disconnect : handleConnect}
              disabled={isConnecting || (!isConnected && !selectedNode)}
              className={clsx(
                "w-36 h-36 rounded-full text-white font-bold text-lg transition-all shadow-lg",
                isConnected
                  ? "bg-green-600 hover:bg-green-500 shadow-green-900"
                  : "bg-blue-600 hover:bg-blue-500 shadow-blue-900",
                isConnecting && "opacity-50 cursor-not-allowed",
                !isConnected && !selectedNode && "opacity-40 cursor-not-allowed"
              )}
            >
              {isConnecting ? "Connecting…" : isConnected ? "Disconnect" : "Connect"}
            </button>

            <div className="text-center">
              <p className={clsx("font-semibold", isConnected ? "text-green-400" : "text-gray-400")}>
                {isConnected ? "Protected" : "Not connected"}
              </p>
              {isConnected && activeTransport && (
                <p className="text-xs text-gray-500 mt-1">via {activeTransport}</p>
              )}
            </div>
          </div>

          {/* Selected node info */}
          {selectedNode && (
            <div className="bg-gray-900 rounded-xl p-4 border border-gray-800">
              <p className="text-xs text-gray-500 uppercase tracking-wide mb-2">Selected server</p>
              <p className="font-semibold">{selectedNode.name}</p>
              <p className="text-sm text-gray-400">{selectedNode.region} · {selectedNode.country}</p>
              <div className="flex gap-3 mt-3 text-xs text-gray-500">
                <span>CPU {selectedNode.cpu_usage.toFixed(0)}%</span>
                <span>MEM {selectedNode.mem_usage.toFixed(0)}%</span>
                <span>{selectedNode.active_conns} users</span>
              </div>
            </div>
          )}

          {error && (
            <p className="text-red-400 text-sm bg-red-900/30 border border-red-800 rounded-lg px-3 py-2">
              {error}
            </p>
          )}
        </div>

        {/* Node list */}
        <div className="md:col-span-2 bg-gray-900 rounded-2xl border border-gray-800 overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-800">
            <h2 className="font-semibold text-lg">Servers</h2>
            <p className="text-sm text-gray-400">{nodes.length} available</p>
          </div>
          <div className="divide-y divide-gray-800 overflow-y-auto max-h-[60vh]">
            {nodes.length === 0 && (
              <p className="text-gray-500 text-sm p-6">Loading servers…</p>
            )}
            {nodes.map((node) => (
              <NodeRow
                key={node.id}
                node={node}
                isSelected={selectedNode?.id === node.id}
                onSelect={() => selectNode(node)}
              />
            ))}
          </div>
        </div>
      </main>
    </div>
  );
}

function NodeRow({
  node,
  isSelected,
  onSelect,
}: {
  node: Node;
  isSelected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      onClick={onSelect}
      className={clsx(
        "w-full flex items-center justify-between px-6 py-4 hover:bg-gray-800 transition-colors text-left",
        isSelected && "bg-gray-800 border-l-2 border-blue-500"
      )}
    >
      <div>
        <p className="font-medium">{node.name}</p>
        <p className="text-sm text-gray-400">
          {node.region} · {node.country}
        </p>
      </div>
      <div className="flex items-center gap-4 text-sm text-gray-400">
        <span>{node.active_conns} users</span>
        <span
          className={clsx(
            "inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium",
            node.status === "online" ? "bg-green-900/50 text-green-400" : "bg-gray-800 text-gray-500"
          )}
        >
          <span
            className={clsx(
              "w-1.5 h-1.5 rounded-full",
              node.status === "online" ? "bg-green-400" : "bg-gray-500"
            )}
          />
          {node.status}
        </span>
      </div>
    </button>
  );
}
