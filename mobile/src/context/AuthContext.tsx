import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState
} from "react";
import { Platform } from "react-native";
import AsyncStorage from "@react-native-async-storage/async-storage";

import * as authApi from "../api/auth";
import { acceptInvitation, User } from "../api/users";
import secureStorage from "../platform/secureStorage";
import AnalyticsService from "../services/AnalyticsService";
import BillingService from "../services/BillingService";
import PushNotificationService from "../services/PushNotificationService";
import { PENDING_INVITATION_CODE_STORAGE_KEY } from "../constants/invitations";

const KEYCHAIN_SERVICE = "com.allcallall.auth";

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
    displayName: string,
    acceptCurrentLegal: boolean
  ) => Promise<void>;
  logout: () => Promise<void>;
  logoutAll: () => Promise<void>;
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

  const authPromptTitle = useMemo(() => "Unlock AllCallAll", []);

  const persistState = useCallback(async (token: string, user: User) => {
    setState({ token, user, loading: false });
    PushNotificationService.setAuthToken(token);
    await BillingService.initialize(`user:${user.id}`);

    // Web access tokens remain in React memory. Session recovery uses the
    // HttpOnly refresh cookie issued by the backend.
    if (Platform.OS === "web") {
      return;
    }

    // 存储到安全存储（支持生物识别）
    // Store to secure storage (with biometric protection)
    const secret = JSON.stringify({ token, user });
    try {
      const biometricAvailable = await secureStorage.supportsBiometricProtection();
      await secureStorage.save(KEYCHAIN_SERVICE, "user_session", secret, {
        requireBiometric: biometricAvailable,
        promptTitle: authPromptTitle
      });
      return;
    } catch (error) {
      console.warn(
        "[AuthContext] Failed to enable biometric keychain storage; falling back",
        error
      );
    }

    await secureStorage.save(KEYCHAIN_SERVICE, "user_session", secret);
  }, [authPromptTitle]);

  const bootstrap = useCallback(async () => {
    try {
      if (Platform.OS === "web") {
        // Remove sessions written by older builds before attempting cookie refresh.
        await secureStorage.clear(KEYCHAIN_SERVICE);
        try {
          const refreshed = await authApi.refreshSession();
          await persistState(refreshed.access_token, refreshed.user);
        } catch {
          setState((current) => ({ ...current, loading: false }));
        }
        return;
      }

      // 从安全存储中读取 token 和 user 数据
      // Read token and user data from secure storage
      const credentials = await secureStorage.load(KEYCHAIN_SERVICE, authPromptTitle);

      if (!credentials) {
        setState((current) => ({ ...current, loading: false }));
        return;
      }

      let parsed: { token: string; user: User };
      try {
        parsed = JSON.parse(credentials.password) as { token: string; user: User };
      } catch {
        await secureStorage.clear(KEYCHAIN_SERVICE);
        setState((current) => ({ ...current, loading: false }));
        return;
      }

      setState({
        token: parsed.token,
        user: parsed.user,
        loading: false
      });
      PushNotificationService.setAuthToken(parsed.token);
      await BillingService.initialize(`user:${parsed.user.id}`);
    } catch (error) {
      console.warn("Failed to load auth state from secure storage", error);
      setState((current) => ({ ...current, loading: false }));
    }
  }, [authPromptTitle, persistState]);

  useEffect(() => {
    bootstrap();
  }, [bootstrap]);

  const flushPendingInvitation = useCallback(async (accessToken: string) => {
    try {
      const code = await AsyncStorage.getItem(PENDING_INVITATION_CODE_STORAGE_KEY);
      if (!code) {
        return;
      }
      await acceptInvitation(accessToken, code);
      AnalyticsService.track("invite_accepted");
      await AsyncStorage.removeItem(PENDING_INVITATION_CODE_STORAGE_KEY);
    } catch (error) {
      console.warn("[AuthContext] Failed to accept pending invitation:", error);
    }
  }, []);

  const clearLocalState = useCallback(async () => {
    setState({ token: null, user: null, loading: false });
    PushNotificationService.setAuthToken(null);
    await BillingService.logout();
    await secureStorage.clear(KEYCHAIN_SERVICE);
  }, []);

  const clearState = useCallback(async () => {
    await clearLocalState();
    if (Platform.OS === "web") {
      try {
        await authApi.logoutSession();
      } catch {
        // Local logout must not be blocked by a network failure.
      }
    }
  }, [clearLocalState]);

  const login = useCallback(
    async (email: string, password: string) => {
      const response = await authApi.login(email, password);
      await persistState(response.access_token, response.user);
      await flushPendingInvitation(response.access_token);

      try {
        await PushNotificationService.sendCurrentTokenToBackend(response.access_token);
      } catch (error) {
        console.warn("[AuthContext] Failed to send FCM token:", error);
      }
    },
    [flushPendingInvitation, persistState]
  );

  const register = useCallback(
    async (
      email: string,
      password: string,
      displayName: string,
      acceptCurrentLegal: boolean
    ) => {
      const response = await authApi.register({
        email,
        password,
        display_name: displayName,
        accept_current_legal: acceptCurrentLegal
      });
      await persistState(response.access_token, response.user);
      AnalyticsService.track("signup_completed");
      await flushPendingInvitation(response.access_token);
      try {
        await PushNotificationService.sendCurrentTokenToBackend(response.access_token);
      } catch (error) {
        console.warn("[AuthContext] Failed to send FCM token after registration:", error);
      }
    },
    [flushPendingInvitation, persistState]
  );

  const logout = useCallback(async () => {
    await clearState();
  }, [clearState]);

  const logoutAll = useCallback(async () => {
    const currentToken = state.token;
    if (currentToken) {
      await authApi.logoutAllSessions(currentToken);
    }
    await clearLocalState();
  }, [clearLocalState, state.token]);

  const value = useMemo<AuthContextValue>(
    () => ({
      ...state,
      login,
      register,
      logout,
      logoutAll
    }),
    [state, login, register, logout, logoutAll]
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
