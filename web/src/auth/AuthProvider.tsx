import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import * as identity from "@/api/identity";
import { setAccessToken } from "@/api/http";
import { AuthContext, type AuthStatus } from "@/auth/AuthContext";

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [user, setUser] = useState<identity.User | null>(null);

  useEffect(() => {
    let active = true;
    identity.restoreSession().then((payload) => {
      if (active) { setUser(payload.user); setStatus("authenticated"); }
    }).catch(() => {
      setAccessToken(null);
      if (active) setStatus("anonymous");
    });
    return () => { active = false; };
  }, []);

  const authenticate = useCallback(async (action: Promise<identity.AuthResponse>) => {
    const payload = await action;
    setUser(payload.user);
    setStatus("authenticated");
  }, []);

  const login = useCallback((email: string, password: string) => authenticate(identity.login(email, password)), [authenticate]);
  const register = useCallback((input: identity.RegisterInput) => authenticate(identity.register(input)), [authenticate]);
  const endSession = useCallback(async (all = false) => {
    try { if (all) await identity.logoutAll(); else await identity.logout(); } finally {
      setAccessToken(null); setUser(null); setStatus("anonymous"); queryClient.clear();
    }
  }, [queryClient]);

  const value = useMemo(() => ({ status, user, login, register, logout: endSession }), [status, user, login, register, endSession]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
