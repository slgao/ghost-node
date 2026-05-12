import axios, { type AxiosInstance } from "axios";

const BASE_URL = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

let _client: AxiosInstance | null = null;

function getClient(): AxiosInstance {
  if (_client) return _client;

  _client = axios.create({
    baseURL: `${BASE_URL}/api/v1`,
    headers: { "Content-Type": "application/json" },
    timeout: 15000,
  });

  // Attach JWT on every request
  _client.interceptors.request.use((config) => {
    const token = localStorage.getItem("access_token");
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  });

  // Auto-refresh on 401
  _client.interceptors.response.use(
    (r) => r,
    async (error) => {
      const original = error.config;
      if (error.response?.status === 401 && !original._retry) {
        original._retry = true;
        try {
          const refreshToken = localStorage.getItem("refresh_token");
          if (!refreshToken) throw new Error("no refresh token");
          const { data } = await axios.post(`${BASE_URL}/api/v1/auth/refresh`, {
            refresh_token: refreshToken,
          });
          localStorage.setItem("access_token", data.tokens.access_token);
          localStorage.setItem("refresh_token", data.tokens.refresh_token);
          original.headers.Authorization = `Bearer ${data.tokens.access_token}`;
          return _client!(original);
        } catch {
          localStorage.clear();
          window.location.href = "/login";
        }
      }
      return Promise.reject(error);
    }
  );

  return _client;
}

// ─── Auth ──────────────────────────────────────────────────────────────────

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_at: string;
}

export interface User {
  id: string;
  email: string;
  role: string;
  is_active: boolean;
  created_at: string;
}

export const authAPI = {
  register: (email: string, password: string) =>
    getClient().post<{ user: User; tokens: TokenPair }>("/auth/register", { email, password }),

  login: (email: string, password: string) =>
    getClient().post<{ tokens: TokenPair }>("/auth/login", { email, password }),

  refresh: (refreshToken: string) =>
    getClient().post<{ tokens: TokenPair }>("/auth/refresh", { refresh_token: refreshToken }),

  logout: (refreshToken: string) =>
    getClient().post("/auth/logout", { refresh_token: refreshToken }),

  me: () => getClient().get<{ user_id: string; email: string; role: string }>("/auth/me"),
};

// ─── Nodes ─────────────────────────────────────────────────────────────────

export interface Node {
  id: string;
  name: string;
  address: string;
  region: string;
  country: string;
  status: "online" | "offline" | "draining";
  cpu_usage: number;
  mem_usage: number;
  active_conns: number;
  transport_profiles: TransportProfile[];
}

export interface TransportProfile {
  id: string;
  type: "xray" | "hysteria2" | "wireguard";
  port: number;
  config: Record<string, unknown>;
  priority: number;
}

export interface ConnectResult {
  profile: TransportProfile;
  vless_uri: string;
  node: { id: string; name: string; address: string; region: string };
}

export interface SubscriptionResult {
  node: string;
  region: string;
  uris: string[];
  clash_url: string;
  singbox_url: string;
  v2rayn_url: string;
}

export const nodesAPI = {
  list: () => getClient().get<{ nodes: Node[] }>("/nodes"),
  get: (id: string) => getClient().get<{ node: Node }>(`/nodes/${id}`),
  connect: (id: string) => getClient().get<ConnectResult>(`/nodes/${id}/connect`),
  connectAuto: () => getClient().get<ConnectResult>("/nodes/connect"),
  subscription: (id: string) =>
    getClient().get<SubscriptionResult>(`/nodes/${id}/subscription?format=all`),
};

// ─── Profile & Devices ─────────────────────────────────────────────────────

export interface Device {
  id: string;
  name: string;
  type: "desktop" | "mobile" | "router";
  last_seen_at: string | null;
}

export const profileAPI = {
  get: () => getClient().get<{ user: User }>("/profile"),
  changePassword: (oldPassword: string, newPassword: string) =>
    getClient().put("/profile/password", { old_password: oldPassword, new_password: newPassword }),
};

export const devicesAPI = {
  list: () => getClient().get<{ devices: Device[] }>("/devices"),
  add: (name: string, type: Device["type"], publicKey: string) =>
    getClient().post<{ device: Device }>("/devices", { name, type, public_key: publicKey }),
  remove: (id: string) => getClient().delete(`/devices/${id}`),
};

// ─── Usage ─────────────────────────────────────────────────────────────────

export interface DailyUsage {
  date: string;
  bytes_in: number;
  bytes_out: number;
}

export interface UsageSummary {
  quota: {
    plan: string;
    quota_bytes: number;
    used_bytes: number;
    remaining_bytes: number;
    used_percent: number;
    expires_at: string | null;
  };
  period: {
    days: number;
    bytes_in: number;
    bytes_out: number;
    total: number;
  };
  daily: DailyUsage[];
}

export const usageAPI = {
  get: () => getClient().get<UsageSummary>("/usage"),
};
