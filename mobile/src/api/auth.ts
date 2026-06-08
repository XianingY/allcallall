import { createApiClient } from "./client";

export interface AuthResponse {
  user: {
    id: number;
    email: string;
    display_name: string;
  };
  access_token: string;
}

export interface RegisterPayload {
  email: string;
  password: string;
  display_name: string;
  accept_current_legal: boolean;
}

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
