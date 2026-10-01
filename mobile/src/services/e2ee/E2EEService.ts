/**
 * E2EE for the mobile client.
 *
 * The cryptography itself lives in @allcallall/shared, alongside the web
 * client's - two implementations of one wire format agree only until someone
 * edits one of them, and the failure mode is a call that produces garbage.
 * What stays here is what is genuinely platform-specific: where the identity
 * private key is stored.
 *
 * Storage rules (unchanged):
 *   - native  -> Keychain, hardware-backed where available
 *   - web     -> ephemeral sessionStorage only; never durable plaintext
 *   - a one-time migration rescues a legacy plaintext copy left in
 *     AsyncStorage by an older build, moves it, then deletes it
 */

import AsyncStorage from "@react-native-async-storage/async-storage";
import { Platform } from "react-native";

import {
  deriveSessionKey,
  E2EEUnsupportedError,
  formatFingerprint,
  generateEcdhKeyPair,
  isE2EECryptoSupported,
  type E2EEKeyPair,
  type E2EESessionKey,
} from "@allcallall/shared";

// Re-exported under the names this module has always used, so callers are
// unaffected by the implementation moving.
export { deriveSessionKey, E2EEUnsupportedError, formatFingerprint, isE2EECryptoSupported };
export type { E2EEKeyPair, E2EESessionKey };

export async function generateECDHKeyPair(): Promise<E2EEKeyPair> {
  return generateEcdhKeyPair();
}

const KEYCHAIN_SERVICE_E2EE = "com.allcallall.e2ee";

const LEGACY_E2EE_ASYNC_KEY = `secure:${KEYCHAIN_SERVICE_E2EE}`;
const WEB_E2EE_SESSION_KEY = "e2ee:identity";

const e2eeWebSessionStorage = (): Storage | null => {
  if (typeof window !== "undefined" && window.sessionStorage) {
    return window.sessionStorage;
  }
  return null;
};

async function storeIdentityKeyPairNative(keyPair: E2EEKeyPair): Promise<void> {
  const Keychain = require("react-native-keychain");
  await Keychain.setGenericPassword("e2ee_identity", JSON.stringify(keyPair), {
    service: KEYCHAIN_SERVICE_E2EE,
    accessible: Keychain.ACCESSIBLE.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
    securityLevel: Keychain.SECURITY_LEVEL.SECURE_HARDWARE,
  });
}

async function loadIdentityKeyPairNative(): Promise<E2EEKeyPair | null> {
  const Keychain = require("react-native-keychain");
  const credentials = await Keychain.getGenericPassword({
    service: KEYCHAIN_SERVICE_E2EE,
  });
  if (!credentials) {
    return null;
  }
  return JSON.parse(credentials.password) as E2EEKeyPair;
}

async function storeIdentityKeyPairWeb(keyPair: E2EEKeyPair): Promise<void> {
  e2eeWebSessionStorage()?.setItem(WEB_E2EE_SESSION_KEY, JSON.stringify(keyPair));
}

async function loadIdentityKeyPairWeb(): Promise<E2EEKeyPair | null> {
  const raw = e2eeWebSessionStorage()?.getItem(WEB_E2EE_SESSION_KEY);
  return raw ? (JSON.parse(raw) as E2EEKeyPair) : null;
}

/**
 * One-time migration: rescue a legacy plaintext identity key from AsyncStorage
 * and move it into the secure store. Returns the migrated key pair, or null if
 * no legacy copy existed.
 */
async function migrateLegacyAsyncStorageKeyPair(): Promise<E2EEKeyPair | null> {
  try {
    const raw = await AsyncStorage.getItem(LEGACY_E2EE_ASYNC_KEY);
    if (!raw) {
      return null;
    }
    const parsed = JSON.parse(raw) as { password?: string };
    const keyPair = parsed?.password ? (JSON.parse(parsed.password) as E2EEKeyPair) : null;
    if (keyPair) {
      if (Platform.OS === "web") {
        await storeIdentityKeyPairWeb(keyPair);
      } else {
        await storeIdentityKeyPairNative(keyPair);
      }
    }
    await AsyncStorage.removeItem(LEGACY_E2EE_ASYNC_KEY);
    return keyPair;
  } catch {
    return null;
  }
}

/**
 * Store identity key pair. Native -> Keychain; web -> ephemeral sessionStorage
 * (never AsyncStorage).
 */
export async function storeIdentityKeyPair(keyPair: E2EEKeyPair): Promise<void> {
  try {
    if (Platform.OS === "web") {
      await storeIdentityKeyPairWeb(keyPair);
      return;
    }
    await storeIdentityKeyPairNative(keyPair);
  } catch (error) {
    console.error("[E2EE] Failed to store identity key pair", error);
    throw new Error("E2EE key storage failed");
  }
}

/**
 * Load identity key pair. Tries the secure store first, then performs a
 * one-time migration from any legacy plaintext AsyncStorage copy.
 */
export async function loadIdentityKeyPair(): Promise<E2EEKeyPair | null> {
  try {
    const current = Platform.OS === "web" ? await loadIdentityKeyPairWeb() : await loadIdentityKeyPairNative();
    if (current) {
      return current;
    }
    return await migrateLegacyAsyncStorageKeyPair();
  } catch (error) {
    console.error("[E2EE] Failed to load identity key pair", error);
    return null;
  }
}
