import AsyncStorage from "@react-native-async-storage/async-storage";
import { Platform } from "react-native";

export interface SecureStorageValue {
  username: string;
  password: string;
}

export interface SecureStorageSaveOptions {
  promptTitle?: string;
  requireBiometric?: boolean;
}

export interface SecureStorageAdapter {
  load(service: string, promptTitle?: string): Promise<SecureStorageValue | null>;
  save(
    service: string,
    username: string,
    password: string,
    options?: SecureStorageSaveOptions
  ): Promise<void>;
  clear(service: string): Promise<void>;
  supportsBiometricProtection(): Promise<boolean>;
}

const webPrefix = "secure:";

const webStorage = () => {
  if (typeof window === "undefined" || !window.sessionStorage) {
    return null;
  }
  return window.sessionStorage;
};

const webAdapter: SecureStorageAdapter = {
  async load(service) {
    const key = `${webPrefix}${service}`;
    const storage = webStorage();
    const raw = storage?.getItem(key) ?? await AsyncStorage.getItem(key);
    if (!raw) {
      return null;
    }
    try {
      const parsed = JSON.parse(raw) as SecureStorageValue;
      if (storage && !storage.getItem(key)) {
        storage.setItem(key, raw);
        await AsyncStorage.removeItem(key);
      }
      return parsed;
    } catch {
      storage?.removeItem(key);
      await AsyncStorage.removeItem(key);
      return null;
    }
  },
  async save(service, username, password) {
    const key = `${webPrefix}${service}`;
    const value = JSON.stringify({ username, password } satisfies SecureStorageValue);
    const storage = webStorage();
    if (storage) {
      storage.setItem(key, value);
      await AsyncStorage.removeItem(key);
      return;
    }
    await AsyncStorage.setItem(key, value);
  },
  async clear(service) {
    const key = `${webPrefix}${service}`;
    webStorage()?.removeItem(key);
    await AsyncStorage.removeItem(key);
  },
  async supportsBiometricProtection() {
    return false;
  },
};

const nativeAdapter: SecureStorageAdapter = {
  async load(service, promptTitle) {
    const Keychain = require("react-native-keychain");
    const credentials = await Keychain.getGenericPassword({
      service,
      authenticationPrompt: promptTitle
        ? {
            title: promptTitle,
            cancel: "Cancel",
          }
        : undefined,
    });
    if (!credentials) {
      return null;
    }
    return {
      username: credentials.username,
      password: credentials.password,
    };
  },
  async save(service, username, password, options) {
    const Keychain = require("react-native-keychain");
    const baseOptions = {
      service,
      accessible: Keychain.ACCESSIBLE.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
      securityLevel: Keychain.SECURITY_LEVEL.SECURE_HARDWARE,
    };
    const biometryType = await Keychain.getSupportedBiometryType();
    if (options?.requireBiometric && biometryType) {
      await Keychain.setGenericPassword(username, password, {
        ...baseOptions,
        accessControl: Keychain.ACCESS_CONTROL.BIOMETRY_ANY_OR_DEVICE_PASSCODE,
        storage: Keychain.STORAGE_TYPE.AES_GCM,
      });
      return;
    }
    await Keychain.setGenericPassword(username, password, baseOptions);
  },
  async clear(service) {
    const Keychain = require("react-native-keychain");
    await Keychain.resetGenericPassword({ service });
  },
  async supportsBiometricProtection() {
    const Keychain = require("react-native-keychain");
    return Boolean(await Keychain.getSupportedBiometryType());
  },
};

const secureStorage: SecureStorageAdapter =
  Platform.OS === "web" ? webAdapter : nativeAdapter;

export default secureStorage;
