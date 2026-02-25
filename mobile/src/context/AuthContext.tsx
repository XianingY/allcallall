import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState
} from "react";
import * as Keychain from "react-native-keychain";

import * as authApi from "../api/auth";
import { User } from "../api/users";
import PushNotificationService from "../services/PushNotificationService";

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

  const authPrompt = useMemo<Keychain.AuthenticationPrompt>(
    () => ({
      title: "Unlock AllCallAll",
      cancel: "Cancel"
    }),
    []
  );

  const bootstrap = useCallback(async () => {
    try {
      // 从安全存储中读取 token 和 user 数据
      // Read token and user data from secure storage
      const credentials = await Keychain.getGenericPassword({
        service: KEYCHAIN_SERVICE,
        authenticationPrompt: authPrompt
      });

      if (!credentials) {
        setState((current) => ({ ...current, loading: false }));
        return;
      }

      let parsed: { token: string; user: User };
      try {
        parsed = JSON.parse(credentials.password) as { token: string; user: User };
      } catch {
        await Keychain.resetGenericPassword({ service: KEYCHAIN_SERVICE });
        setState((current) => ({ ...current, loading: false }));
        return;
      }

      setState({
        token: parsed.token,
        user: parsed.user,
        loading: false
      });
    } catch (error) {
      console.warn("Failed to load auth state from secure storage", error);
      setState((current) => ({ ...current, loading: false }));
    }
  }, [authPrompt]);

  useEffect(() => {
    bootstrap();
  }, [bootstrap]);

  const persistState = useCallback(async (token: string, user: User) => {
    setState({ token, user, loading: false });

    // 存储到安全存储（支持生物识别）
    // Store to secure storage (with biometric protection)
    const secret = JSON.stringify({ token, user });
    const baseOptions: Keychain.SetOptions = {
      service: KEYCHAIN_SERVICE,
      accessible: Keychain.ACCESSIBLE.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
      securityLevel: Keychain.SECURITY_LEVEL.SECURE_HARDWARE
    };

    try {
      const biometryType = await Keychain.getSupportedBiometryType();
      if (biometryType) {
        await Keychain.setGenericPassword("user_session", secret, {
          ...baseOptions,
          accessControl: Keychain.ACCESS_CONTROL.BIOMETRY_ANY_OR_DEVICE_PASSCODE,
          storage: Keychain.STORAGE_TYPE.AES_GCM
        });
        return;
      }
    } catch (error) {
      console.warn(
        "[AuthContext] Failed to enable biometric keychain storage; falling back",
        error
      );
    }

    await Keychain.setGenericPassword("user_session", secret, baseOptions);
  }, []);

  const clearState = useCallback(async () => {
    setState({ token: null, user: null, loading: false });
    await Keychain.resetGenericPassword({ service: KEYCHAIN_SERVICE });
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
