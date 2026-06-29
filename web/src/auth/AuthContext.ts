import { createContext, useContext } from "react";

import type * as identity from "@/api/identity";

export type AuthStatus = "loading" | "authenticated" | "anonymous";

export interface AuthContextValue {
  status: AuthStatus;
  user: identity.User | null;
  login(email: string, password: string): Promise<void>;
  register(input: identity.RegisterInput): Promise<void>;
  logout(all?: boolean): Promise<void>;
}

export const AuthContext = createContext<AuthContextValue | null>(null);

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthProvider");
  return value;
}
