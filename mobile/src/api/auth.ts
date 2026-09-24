import type { components } from "@allcallall/api-types";

import { createApiClient } from "./client";

type APISchemas = components["schemas"];

// #22：改用 openapi.yaml 生成的共享契约，删除手写请求/响应类型。
// 保留旧导出名，使既有引用无需改动。
export type AuthResponse = APISchemas["AuthResponse"];

export type RegisterPayload = APISchemas["RegisterRequest"];

// spec 里 status 是 string，这里保留客户端原有的窄联合，避免放宽下游的等值判断。
export type RefreshSessionRecord = Omit<
  APISchemas["RefreshSession"],
  "status"
> & {
  status: "active" | "expired" | "revoked";
};

export const register = async (payload: RegisterPayload) => {
  const api = createApiClient();
  const response = await api.post<AuthResponse>("/auth/register", payload, { withCredentials: true });
  return response.data;
};

export const login = async (email: string, password: string) => {
  const api = createApiClient();
  const response = await api.post<AuthResponse>("/auth/login", {
    email,
    password
  }, {
    withCredentials: true
  });
  return response.data;
};

export const refreshSession = async () => {
  const api = createApiClient();
  const response = await api.post<AuthResponse>("/auth/refresh", undefined, { withCredentials: true });
  return response.data;
};

export const logoutSession = async () => {
  const api = createApiClient();
  await api.post("/auth/logout", undefined, { withCredentials: true });
};

export const logoutAllSessions = async (token: string) => {
  const api = createApiClient(token);
  await api.post("/auth/logout-all", undefined, { withCredentials: true });
};

export const listRefreshSessions = async (token: string) => {
  const api = createApiClient(token);
  const response = await api.get<{ sessions: RefreshSessionRecord[] }>("/auth/sessions", {
    withCredentials: true
  });
  return response.data.sessions;
};

export const revokeRefreshSession = async (token: string, sessionId: number) => {
  const api = createApiClient(token);
  await api.delete(`/auth/sessions/${sessionId}`, { withCredentials: true });
};
