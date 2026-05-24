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
  const response = await api.post<AuthResponse>("/auth/register", payload);
  return response.data;
};

export const login = async (email: string, password: string) => {
  const api = createApiClient();
  const response = await api.post<AuthResponse>("/auth/login", {
    email,
    password
  });
  return response.data;
};
