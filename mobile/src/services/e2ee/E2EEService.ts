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

import { NativeModules, Platform } from "react-native";
import * as Keychain from "react-native-keychain";

const KEYCHAIN_SERVICE_E2EE = "com.allcallall.e2ee";

export interface E2EEKeyPair {
  publicKey: string; // Base64-encoded public key
  privateKey: string; // Base64-encoded private key (stored securely)
}

export interface E2EESessionKey {
  sessionKey: Uint8Array;
  fingerprint: string; // Hex-encoded SHA-256 of sessionKey
}

/**
 * Generate ECDH key pair using native crypto
 */
export async function generateECDHKeyPair(): Promise<E2EEKeyPair> {
  try {
    // Use native crypto for ECDH (P-256 curve)
    const keyPair = await crypto.subtle.generateKey(
      {
        name: "ECDH",
        namedCurve: "P-256"
      },
      true,
      ["deriveKey", "deriveBits"]
    );

    const publicKeyJwk = await crypto.subtle.exportKey("jwk", keyPair.publicKey);
    const privateKeyJwk = await crypto.subtle.exportKey("jwk", keyPair.privateKey);

    return {
      publicKey: JSON.stringify(publicKeyJwk),
      privateKey: JSON.stringify(privateKeyJwk)
    };
  } catch (error) {
    console.error("[E2EE] Failed to generate ECDH key pair", error);
    throw new Error("E2EE key generation failed");
  }
}

/**
 * Derive session key from ECDH shared secret
 */
export async function deriveSessionKey(
  myPrivateKeyJwk: string,
  peerPublicKeyJwk: string,
  callId: string
): Promise<E2EESessionKey> {
  try {
    const myPrivateKey = await crypto.subtle.importKey(
      "jwk",
      JSON.parse(myPrivateKeyJwk),
      { name: "ECDH", namedCurve: "P-256" },
      false,
      ["deriveKey", "deriveBits"]
    );

    const peerPublicKey = await crypto.subtle.importKey(
      "jwk",
      JSON.parse(peerPublicKeyJwk),
      { name: "ECDH", namedCurve: "P-256" },
      false,
      []
    );

    // Derive shared secret
    const sharedSecret = await crypto.subtle.deriveBits(
      { name: "ECDH", public: peerPublicKey },
      myPrivateKey,
      256
    );

    // Derive session key using HKDF with callId as salt
    const salt = new TextEncoder().encode(callId);
    const info = new TextEncoder().encode("AllCallAll E2EE Session Key");

    const sessionKeyMaterial = await crypto.subtle.importKey(
      "raw",
      sharedSecret,
      { name: "HKDF" },
      false,
      ["deriveKey"]
    );

    const sessionKey = await crypto.subtle.deriveKey(
      {
        name: "HKDF",
        hash: "SHA-256",
        salt: salt,
        info: info
      },
      sessionKeyMaterial,
      { name: "AES-GCM", length: 256 },
      true,
      ["encrypt", "decrypt"]
    );

    // Export session key for fingerprint computation
    const sessionKeyRaw = await crypto.subtle.exportKey("raw", sessionKey);
    const sessionKeyBytes = new Uint8Array(sessionKeyRaw);

    // Compute fingerprint (SHA-256 hash)
    const fingerprintBuffer = await crypto.subtle.digest("SHA-256", sessionKeyBytes);
    const fingerprintArray = Array.from(new Uint8Array(fingerprintBuffer));
    const fingerprint = fingerprintArray.map(b => b.toString(16).padStart(2, "0")).join("");

    return {
      sessionKey: sessionKeyBytes,
      fingerprint: fingerprint
    };
  } catch (error) {
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
