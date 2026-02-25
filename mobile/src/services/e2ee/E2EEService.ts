/**
 * E2EE Service - Application-layer End-to-End Encryption
 * 
 * Since react-native-webrtc v124 doesn't support Insertable Streams,
 * we implement E2EE at the application layer:
 * 
 * 1. ECDH key exchange via Data Channel
 * 2. Session key derivation (HKDF with callId as salt)
 * 3. Key fingerprint for manual verification
 * 
 * Note: WebRTC media streams remain encrypted by DTLS-SRTP at transport layer.
 * This E2EE layer protects against SFU eavesdropping (when SFU is added in future).
 */

import * as Keychain from "react-native-keychain";
import { p256 } from "@noble/curves/p256";
import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha256";
import { bytesToHex, hexToBytes, utf8ToBytes } from "@noble/hashes/utils";

const KEYCHAIN_SERVICE_E2EE = "com.allcallall.e2ee";
const E2EE_INFO = utf8ToBytes("AllCallAll E2EE Session Key");
const HEX_PATTERN = /^[0-9a-f]+$/i;

export class E2EEUnsupportedError extends Error {
  constructor(message: string = "Secure random source is unavailable") {
    super(message);
    this.name = "E2EEUnsupportedError";
  }
}

const hasSecureRandom = (): boolean =>
  typeof (globalThis as any)?.crypto?.getRandomValues === "function";

const assertSecureRandomAvailable = (): void => {
  if (!hasSecureRandom()) {
    throw new E2EEUnsupportedError(
      "crypto.getRandomValues is unavailable on this runtime"
    );
  }
};

export const isE2EECryptoSupported = (): boolean =>
  hasSecureRandom();

const normalizeHex = (value: string, label: string): string => {
  const normalized = value.trim().replace(/^0x/i, "").toLowerCase();
  if (
    normalized.length === 0 ||
    normalized.length % 2 !== 0 ||
    !HEX_PATTERN.test(normalized)
  ) {
    throw new Error(`Invalid E2EE ${label} format`);
  }
  return normalized;
};

const parsePrivateKeyHex = (privateKeyHex: string): Uint8Array => {
  const normalized = normalizeHex(privateKeyHex, "private key");
  if (normalized.length !== 64) {
    throw new Error("Invalid E2EE private key length");
  }
  return hexToBytes(normalized);
};

const parsePublicKeyHex = (publicKeyHex: string): Uint8Array => {
  const normalized = normalizeHex(publicKeyHex, "public key");
  if (normalized.length !== 66 && normalized.length !== 130) {
    throw new Error("Invalid E2EE public key length");
  }
  return hexToBytes(normalized);
};

export interface E2EEKeyPair {
  publicKey: string; // Hex-encoded P-256 public key
  privateKey: string; // Hex-encoded P-256 private key (stored securely)
}

export interface E2EESessionKey {
  sessionKey: Uint8Array;
  fingerprint: string; // Hex-encoded SHA-256 of sessionKey
}

/**
 * Generate ECDH key pair using noble-curves (P-256)
 */
export async function generateECDHKeyPair(): Promise<E2EEKeyPair> {
  try {
    assertSecureRandomAvailable();
    const privateKeyBytes = p256.utils.randomPrivateKey();
    const publicKeyBytes = p256.getPublicKey(privateKeyBytes, false);

    return {
      publicKey: bytesToHex(publicKeyBytes),
      privateKey: bytesToHex(privateKeyBytes)
    };
  } catch (error) {
    if (error instanceof E2EEUnsupportedError) {
      throw error;
    }
    console.error("[E2EE] Failed to generate ECDH key pair", error);
    throw new Error("E2EE key generation failed");
  }
}

/**
 * Derive session key from ECDH shared secret
 */
export async function deriveSessionKey(
  myPrivateKeyHex: string,
  peerPublicKeyHex: string,
  callId: string
): Promise<E2EESessionKey> {
  try {
    const myPrivateKey = parsePrivateKeyHex(myPrivateKeyHex);
    const peerPublicKey = parsePublicKeyHex(peerPublicKeyHex);

    const sharedPoint = p256.getSharedSecret(myPrivateKey, peerPublicKey, false);
    const sharedSecret =
      sharedPoint.length >= 33 && sharedPoint[0] === 0x04
        ? sharedPoint.slice(1, 33)
        : sharedPoint.slice(-32);

    if (sharedSecret.length !== 32) {
      throw new Error("Invalid E2EE shared secret length");
    }

    // Derive session key using HKDF-SHA256 with callId as salt.
    const salt = utf8ToBytes(callId);
    const sessionKeyBytes = hkdf(
      sha256,
      sharedSecret,
      salt,
      E2EE_INFO,
      32
    );

    const fingerprint = bytesToHex(sha256(sessionKeyBytes));

    return {
      sessionKey: sessionKeyBytes,
      fingerprint: fingerprint
    };
  } catch (error) {
    if (error instanceof E2EEUnsupportedError) {
      throw error;
    }
    console.error("[E2EE] Failed to derive session key", error);
    throw new Error("E2EE session key derivation failed");
  }
}

/**
 * Store identity key pair in Keychain (for persistent identity)
 */
export async function storeIdentityKeyPair(keyPair: E2EEKeyPair): Promise<void> {
  try {
    await Keychain.setGenericPassword(
      "e2ee_identity",
      JSON.stringify(keyPair),
      {
        service: KEYCHAIN_SERVICE_E2EE,
        accessible: Keychain.ACCESSIBLE.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
        securityLevel: Keychain.SECURITY_LEVEL.SECURE_HARDWARE
      }
    );
  } catch (error) {
    console.error("[E2EE] Failed to store identity key pair", error);
    throw new Error("E2EE key storage failed");
  }
}

/**
 * Load identity key pair from Keychain
 */
export async function loadIdentityKeyPair(): Promise<E2EEKeyPair | null> {
  try {
    const credentials = await Keychain.getGenericPassword({
      service: KEYCHAIN_SERVICE_E2EE
    });

    if (!credentials) {
      return null;
    }

    const keyPair = JSON.parse(credentials.password) as E2EEKeyPair;
    return keyPair;
  } catch (error) {
    console.error("[E2EE] Failed to load identity key pair", error);
    return null;
  }
}

/**
 * Format fingerprint for display (grouped for readability)
 */
export function formatFingerprint(fingerprint: string): string {
  // Format as: XXXX XXXX XXXX XXXX XXXX XXXX XXXX XXXX
  return fingerprint.match(/.{1,4}/g)?.join(" ") || fingerprint;
}
