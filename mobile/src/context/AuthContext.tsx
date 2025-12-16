import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState
} from "react";
import AsyncStorage from "@react-native-async-storage/async-storage";

import * as authApi from "../api/auth";
import * as usersApi from "../api/users";
import { User } from "../api/users";
import PushNotificationService from "../services/PushNotificationService";

const STORAGE_KEY = "allcallall.auth";

interface AuthState {
  token: string | null;
  user: User | null;
  loading: boolean;
}

interface AuthContextValue extends AuthState {
  login: (email: string, password: string) => Promise<void>;
  register: (
    email: string,
    password: string,
    displayName: string
  ) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({
  children
}) => {
  const [state, setState] = useState<AuthState>({
    token: null,
    user: null,
    loading: true
  });

  const bootstrap = useCallback(async () => {
    try {
      const stored = await AsyncStorage.getItem(STORAGE_KEY);
      if (!stored) {
        setState((current) => ({ ...current, loading: false }));
        return;
      }
      const parsed = JSON.parse(stored) as { token: string; user: User };
      setState({
        token: parsed.token,
        user: parsed.user,
        loading: false
      });
    } catch (error) {
      console.warn("Failed to load auth state", error);
      setState((current) => ({ ...current, loading: false }));
    }
  }, []);

  useEffect(() => {
    bootstrap();
  }, [bootstrap]);

  const persistState = useCallback(async (token: string, user: User) => {
    setState({ token, user, loading: false });
    await AsyncStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ token, user })
    );
  }, []);

  const clearState = useCallback(async () => {
    setState({ token: null, user: null, loading: false });
    await AsyncStorage.removeItem(STORAGE_KEY);
  }, []);

  const login = useCallback(
    async (email: string, password: string) => {
      const response = await authApi.login(email, password);
      await persistState(response.access_token, response.user);
      
      // 登录成功后，发送 FCM Token 到后端
      // Send FCM Token to backend after successful login
      try {
        console.log("[AuthContext] Sending FCM token to backend...");
        await PushNotificationService.sendCurrentTokenToBackend(response.access_token);
        console.log("[AuthContext] FCM token sent successfully");
      } catch (error) {
        console.warn("[AuthContext] Failed to send FCM token:", error);
        // 不中断登录流程，继续进行
        // Continue with login process even if FCM token send fails
      }
    },
    [persistState]
  );

  const register = useCallback(
    async (email: string, password: string, displayName: string) => {
      const response = await authApi.register({
        email,
        password,
        display_name: displayName
      });
      await persistState(response.access_token, response.user);
    },
    [persistState]
  );

  const logout = useCallback(async () => {
    await clearState();
  }, [clearState]);

  const value = useMemo<AuthContextValue>(
    () => ({
      ...state,
      login,
      register,
      logout
    }),
    [state, login, register, logout]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export const useAuthContext = () => {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuthContext must be used within AuthProvider");
  }
  return ctx;
};
