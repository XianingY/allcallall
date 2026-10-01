/**
 * E2EE for the web client.
 *
 * All of the cryptography and the key-exchange handshake come from
 * @allcallall/shared - the same code the mobile client runs, which is what
 * makes the two interoperate rather than merely look similar. What this module
 * adds is the web-specific identity key storage.
 *
 * Storage rule (matching the mobile client's web branch): the identity private
 * key lives in sessionStorage only. It is ephemeral - cleared when the tab
 * closes - and is never written to durable storage, because a persisted
 * private key is the thing this feature exists to protect.
 */

import type { E2EEKeyPair } from "@allcallall/shared";

const WEB_E2EE_SESSION_KEY = "e2ee:identity";

const webSessionStorage = (): Storage | null => {
  if (typeof window !== "undefined" && window.sessionStorage) {
    return window.sessionStorage;
  }
  return null;
};

/** Stores the identity key pair in sessionStorage. Never localStorage. */
export async function storeIdentityKeyPair(keyPair: E2EEKeyPair): Promise<void> {
  const storage = webSessionStorage();
  if (!storage) {
    throw new Error("sessionStorage is unavailable; cannot store the E2EE identity key");
  }
  storage.setItem(WEB_E2EE_SESSION_KEY, JSON.stringify(keyPair));
}

/** Loads the identity key pair from sessionStorage, or null. */
export async function loadIdentityKeyPair(): Promise<E2EEKeyPair | null> {
  const storage = webSessionStorage();
  const raw = storage?.getItem(WEB_E2EE_SESSION_KEY);
  return raw ? (JSON.parse(raw) as E2EEKeyPair) : null;
}

/** Clears the identity key pair from sessionStorage. */
export async function clearIdentityKeyPair(): Promise<void> {
  webSessionStorage()?.removeItem(WEB_E2EE_SESSION_KEY);
}

export {
  decryptMessage,
  deriveSessionKey,
  encryptMessage,
  E2EEKeyExchange,
  E2EEUnsupportedError,
  formatFingerprint,
  generateEcdhKeyPair,
  isE2EECryptoSupported,
  type DataChannelLike,
  type E2EEKeyExchangeCallbacks,
  type E2EEKeyPair,
  type E2EESessionKey,
  type KeyExchangeMessage,
  type KeyExchangeRole,
} from "@allcallall/shared";
