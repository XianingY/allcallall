/**
 * End-to-end encryption primitives, shared by web and mobile.
 *
 * Why this lives here rather than in each client: "the two clients can read
 * each other's messages" is a property that has to hold across two codebases.
 * Two independent implementations of the same wire format agree only until one
 * of them is edited, and the failure is silent - you find out when a call
 * produces garbage. Sharing the implementation makes interoperability
 * structural instead of coincidental, and testable in one place.
 *
 * The key agreement below is byte-for-byte the format the mobile client
 * already used, so existing peers keep working:
 *   - P-256, public key hex-encoded *uncompressed* (65 bytes / 130 hex chars)
 *   - private key hex, 32 bytes
 *   - ECDH shared point, x coordinate used as the shared secret
 *   - HKDF-SHA256, salt = callId (UTF-8), info = E2EE_INFO, 32-byte output
 *   - fingerprint = SHA-256 of the session key, hex
 *
 * Message encryption is new: mobile previously negotiated a session key and
 * then had nothing to do with it. AES-256-GCM with a fresh 96-bit IV per
 * message, serialised as `v1.<iv hex>.<ciphertext hex>` - versioned so the
 * format can change without breaking peers, and hex rather than base64 so it
 * survives the JSON-over-DataChannel hop unchanged.
 *
 * Not audited. This is a correct use of standard primitives, not a substitute
 * for a review before it is relied upon for anything that matters.
 */

import { gcm } from "@noble/ciphers/aes";
import { p256 } from "@noble/curves/p256";
import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha256";
import { bytesToHex, hexToBytes, utf8ToBytes } from "@noble/hashes/utils";

const E2EE_INFO = utf8ToBytes("AllCallAll E2EE Session Key");
const HEX_PATTERN = /^[0-9a-f]+$/i;

/** Current payload version. Bump when the format changes. */
export const E2EE_PAYLOAD_VERSION = "v1";
const IV_BYTES = 12;
const AES_GCM_TAG_BYTES = 16;

export class E2EEUnsupportedError extends Error {
  constructor(message = "Secure random source is unavailable") {
    super(message);
    this.name = "E2EEUnsupportedError";
  }
}

const hasSecureRandom = (): boolean =>
  typeof (globalThis as { crypto?: { getRandomValues?: unknown } })?.crypto?.getRandomValues === "function";

export const isE2EECryptoSupported = (): boolean => hasSecureRandom();

const assertSecureRandomAvailable = (): void => {
  if (!hasSecureRandom()) {
    throw new E2EEUnsupportedError("crypto.getRandomValues is unavailable on this runtime");
  }
};

const normalizeHex = (value: string, label: string): string => {
  const normalized = value.trim().replace(/^0x/i, "").toLowerCase();
  if (normalized.length === 0 || normalized.length % 2 !== 0 || !HEX_PATTERN.test(normalized)) {
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
  /** Hex-encoded P-256 public key (uncompressed). */
  publicKey: string;
  /** Hex-encoded P-256 private key. Store securely; never in durable plaintext. */
  privateKey: string;
}

export interface E2EESessionKey {
  sessionKey: Uint8Array;
  /** Hex-encoded SHA-256 of the session key, for out-of-band comparison. */
  fingerprint: string;
}

export function generateEcdhKeyPair(): E2EEKeyPair {
  try {
    assertSecureRandomAvailable();
    const privateKeyBytes = p256.utils.randomPrivateKey();
    const publicKeyBytes = p256.getPublicKey(privateKeyBytes, false);
    return {
      publicKey: bytesToHex(publicKeyBytes),
      privateKey: bytesToHex(privateKeyBytes),
    };
  } catch (error) {
    if (error instanceof E2EEUnsupportedError) {
      throw error;
    }
    throw new Error("E2EE key generation failed");
  }
}

/**
 * Derives the session key both sides must arrive at independently.
 *
 * callId is the HKDF salt, not decoration: without it the same key pair
 * produces the same session key for every call, so a recorded payload could be
 * replayed into a later one.
 */
export function deriveSessionKey(
  myPrivateKeyHex: string,
  peerPublicKeyHex: string,
  callId: string,
): E2EESessionKey {
  // Caller mistakes keep their own message. The mobile client wrapped all of
  // these into one generic "derivation failed", which makes a bad key
  // indistinguishable from an unsupported runtime - the check that throws is
  // the only thing that knows what was wrong, so it should say so.
  if (typeof callId !== "string" || callId.length === 0) {
    throw new Error("E2EE session key derivation requires a call id");
  }

  const myPrivateKey = parsePrivateKeyHex(myPrivateKeyHex);
  const peerPublicKey = parsePublicKeyHex(peerPublicKeyHex);

  const sharedPoint = p256.getSharedSecret(myPrivateKey, peerPublicKey, false);
  // Use the x coordinate: the point is 0x04 || x || y, so drop the prefix.
  const sharedSecret =
    sharedPoint.length >= 33 && sharedPoint[0] === 0x04 ? sharedPoint.slice(1, 33) : sharedPoint.slice(-32);

  if (sharedSecret.length !== 32) {
    throw new Error("Invalid E2EE shared secret length");
  }

  const sessionKeyBytes = hkdf(sha256, sharedSecret, utf8ToBytes(callId), E2EE_INFO, 32);
  return { sessionKey: sessionKeyBytes, fingerprint: bytesToHex(sha256(sessionKeyBytes)) };
}

/**
 * Encrypts a UTF-8 string with the session key.
 *
 * A fresh random IV per message is required, not an optimisation: GCM loses
 * confidentiality and integrity if one key is used with the same IV twice.
 */
export function encryptMessage(sessionKey: Uint8Array, plaintext: string): string {
  assertSessionKey(sessionKey);
  const iv = randomBytes(IV_BYTES);
  const ciphertext = gcm(sessionKey, iv).encrypt(utf8ToBytes(plaintext));
  return `${E2EE_PAYLOAD_VERSION}.${bytesToHex(iv)}.${bytesToHex(ciphertext)}`;
}

/**
 * Decrypts a payload produced by encryptMessage.
 *
 * Throws on any tampering: GCM authenticates the ciphertext, so a modified
 * byte fails here rather than decrypting to garbage the caller might display.
 */
export function decryptMessage(sessionKey: Uint8Array, payload: string): string {
  assertSessionKey(sessionKey);

  const parts = payload.split(".");
  if (parts.length !== 3) {
    throw new Error("Invalid E2EE payload format");
  }
  const [version, ivHex, ciphertextHex] = parts;
  if (version !== E2EE_PAYLOAD_VERSION) {
    throw new Error(`Unsupported E2EE payload version: ${version}`);
  }

  const iv = hexToBytes(normalizeHex(ivHex, "iv"));
  if (iv.length !== IV_BYTES) {
    throw new Error("Invalid E2EE iv length");
  }
  const ciphertext = hexToBytes(normalizeHex(ciphertextHex, "ciphertext"));
  if (ciphertext.length < AES_GCM_TAG_BYTES) {
    throw new Error("Invalid E2EE ciphertext length");
  }

  return bytesToUtf8(gcm(sessionKey, iv).decrypt(ciphertext));
}

function assertSessionKey(sessionKey: Uint8Array): void {
  if (!(sessionKey instanceof Uint8Array) || sessionKey.length !== 32) {
    throw new Error("E2EE session key must be 32 bytes");
  }
}

/** Random bytes from the platform CSPRNG, via crypto.getRandomValues. */
function randomBytes(length: number): Uint8Array {
  assertSecureRandomAvailable();
  const bytes = new Uint8Array(length);
  globalThis.crypto.getRandomValues(bytes);
  return bytes;
}

function bytesToUtf8(bytes: Uint8Array): string {
  if (typeof TextDecoder !== "undefined") {
    return new TextDecoder().decode(bytes);
  }
  // Hermes (React Native) has no TextDecoder in older versions.
  let result = "";
  for (const byte of bytes) {
    result += String.fromCharCode(byte);
  }
  return decodeURIComponent(escape(result));
}

/** Groups a fingerprint into blocks of four for reading aloud. */
export function formatFingerprint(fingerprint: string): string {
  return fingerprint.match(/.{1,4}/g)?.join(" ") || fingerprint;
}
