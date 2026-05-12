import { create } from "zustand";
import { persist } from "zustand/middleware";
import { authAPI, nodesAPI, usageAPI, type Node, type User, type UsageSummary, type ConnectResult } from "@/api/client";

interface VPNState {
  // Auth
  user: User | null;
  accessToken: string | null;
  refreshToken: string | null;
  isAuthenticated: boolean;

  // Nodes
  nodes: Node[];
  selectedNode: Node | null;

  // Connection
  isConnected: boolean;
  isConnecting: boolean;
  connectedNodeId: string | null;
  activeTransport: string | null;
  bytesIn: number;
  bytesOut: number;

  // Usage
  usage: UsageSummary | null;

  // Active config (after Connect)
  activeConfig: ConnectResult | null;

  // Actions
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  fetchNodes: () => Promise<void>;
  selectNode: (node: Node) => void;
  connect: (nodeId: string) => Promise<void>;
  connectAuto: () => Promise<void>;
  disconnect: () => void;
  fetchUsage: () => Promise<void>;
}

export const useVPNStore = create<VPNState>()(
  persist(
    (set, get) => ({
      user: null,
      accessToken: null,
      refreshToken: null,
      isAuthenticated: false,
      nodes: [],
      selectedNode: null,
      isConnected: false,
      isConnecting: false,
      connectedNodeId: null,
      activeTransport: null,
      bytesIn: 0,
      bytesOut: 0,
      usage: null,
      activeConfig: null,

      login: async (email, password) => {
        const { data } = await authAPI.login(email, password);
        const { access_token, refresh_token } = data.tokens;
        localStorage.setItem("access_token", access_token);
        localStorage.setItem("refresh_token", refresh_token);

        const meResp = await authAPI.me();
        set({
          accessToken: access_token,
          refreshToken: refresh_token,
          isAuthenticated: true,
          user: {
            id: meResp.data.user_id,
            email: meResp.data.email,
            role: meResp.data.role,
            is_active: true,
            created_at: "",
          },
        });
      },

      register: async (email, password) => {
        const { data } = await authAPI.register(email, password);
        const { access_token, refresh_token } = data.tokens;
        localStorage.setItem("access_token", access_token);
        localStorage.setItem("refresh_token", refresh_token);
        set({
          accessToken: access_token,
          refreshToken: refresh_token,
          isAuthenticated: true,
          user: data.user,
        });
      },

      logout: async () => {
        const rt = get().refreshToken;
        if (rt) await authAPI.logout(rt).catch(() => null);
        localStorage.removeItem("access_token");
        localStorage.removeItem("refresh_token");
        set({
          user: null,
          accessToken: null,
          refreshToken: null,
          isAuthenticated: false,
          isConnected: false,
          connectedNodeId: null,
        });
      },

      fetchNodes: async () => {
        const { data } = await nodesAPI.list();
        set({ nodes: data.nodes });
      },

      selectNode: (node) => set({ selectedNode: node }),

      connect: async (nodeId) => {
        set({ isConnecting: true });
        try {
          const { data } = await nodesAPI.connect(nodeId);
          set({
            isConnected: true,
            isConnecting: false,
            connectedNodeId: nodeId,
            activeTransport: data.profile.type,
            activeConfig: data,
          });
        } catch (err) {
          set({ isConnecting: false });
          throw err;
        }
      },

      connectAuto: async () => {
        set({ isConnecting: true });
        try {
          const { data } = await nodesAPI.connectAuto();
          set({
            isConnected: true,
            isConnecting: false,
            connectedNodeId: data.node.address,
            activeTransport: data.profile.type,
            activeConfig: data,
          });
        } catch (err) {
          set({ isConnecting: false });
          throw err;
        }
      },

      disconnect: () => {
        set({
          isConnected: false,
          connectedNodeId: null,
          activeTransport: null,
          activeConfig: null,
          bytesIn: 0,
          bytesOut: 0,
        });
      },

      fetchUsage: async () => {
        const { data } = await usageAPI.get();
        set({ usage: data });
      },
    }),
    {
      name: "vpn-store",
      partialize: (state) => ({
        accessToken: state.accessToken,
        refreshToken: state.refreshToken,
        isAuthenticated: state.isAuthenticated,
        user: state.user,
      }),
    }
  )
);
