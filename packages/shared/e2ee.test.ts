import assert from "node:assert/strict";
import { webcrypto } from "node:crypto";
import { describe, it } from "node:test";

import {
  decryptMessage,
  deriveSessionKey,
  encryptMessage,
  formatFingerprint,
  generateEcdhKeyPair,
  isE2EECryptoSupported,
} from "./e2ee";

// The acceptance bar for this module is "the two clients can read each
// other's messages". Implementing both sides with the same helper would make
// that true by construction but would not prove the wire format is standard -
// two halves of a private format agree with each other and with nothing else.
//
// So the cross-implementation tests below decrypt with the platform's own
// WebCrypto, which shares no code with @noble. If those pass, any stack with
// standard AES-GCM can interoperate, not just ours.

const decode = (bytes: Uint8Array) => new TextDecoder().decode(bytes);

async function platformDecrypt(sessionKey: Uint8Array, payload: string): Promise<string> {
  const [version, ivHex, ciphertextHex] = payload.split(".");
  assert.equal(version, "v1");
  const iv = Uint8Array.from(ivHex.match(/.{2}/g)!.map((pair) => parseInt(pair, 16)));
  const ciphertext = Uint8Array.from(ciphertextHex.match(/.{2}/g)!.map((pair) => parseInt(pair, 16)));
  const key = await webcrypto.subtle.importKey("raw", sessionKey, "AES-GCM", false, ["decrypt"]);
  const plaintext = await webcrypto.subtle.decrypt({ name: "AES-GCM", iv }, key, ciphertext);
  return decode(new Uint8Array(plaintext));
}

async function platformEncrypt(sessionKey: Uint8Array, plaintext: string): Promise<string> {
  const iv = webcrypto.getRandomValues(new Uint8Array(12));
  const key = await webcrypto.subtle.importKey("raw", sessionKey, "AES-GCM", false, ["encrypt"]);
  const ciphertext = await webcrypto.subtle.encrypt({ name: "AES-GCM", iv }, key, new TextEncoder().encode(plaintext));
  const hex = (bytes: Uint8Array) => [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
  return `v1.${hex(iv)}.${hex(new Uint8Array(ciphertext))}`;
}

describe("key agreement", () => {
  it("reports support when a CSPRNG is present", () => {
    assert.equal(isE2EECryptoSupported(), true);
  });

  it("emits an uncompressed P-256 public key, as the mobile client always has", () => {
    const { publicKey } = generateEcdhKeyPair();
    // 0x04 prefix plus x and y: 130 hex characters.
    assert.equal(publicKey.length, 130);
    assert.equal(publicKey.slice(0, 2), "04");
  });

  it("derives the same session key on both sides", () => {
    const alice = generateEcdhKeyPair();
    const bob = generateEcdhKeyPair();

    const aliceView = deriveSessionKey(alice.privateKey, bob.publicKey, "call-abc");
    const bobView = deriveSessionKey(bob.privateKey, alice.publicKey, "call-abc");

    assert.deepEqual(aliceView.sessionKey, bobView.sessionKey);
    assert.equal(aliceView.fingerprint, bobView.fingerprint);
    assert.equal(aliceView.sessionKey.length, 32);
  });

  it("keys differ per call, so a recorded payload cannot be replayed into another", () => {
    const alice = generateEcdhKeyPair();
    const bob = generateEcdhKeyPair();

    const first = deriveSessionKey(alice.privateKey, bob.publicKey, "call-1");
    const second = deriveSessionKey(alice.privateKey, bob.publicKey, "call-2");

    assert.notDeepEqual(first.sessionKey, second.sessionKey);
    assert.notEqual(first.fingerprint, second.fingerprint);
  });

  it("rejects a missing call id rather than deriving an unsalted key", () => {
    const alice = generateEcdhKeyPair();
    const bob = generateEcdhKeyPair();
    assert.throws(() => deriveSessionKey(alice.privateKey, bob.publicKey, ""), /call id/);
  });

  it("rejects malformed keys", () => {
    const alice = generateEcdhKeyPair();
    assert.throws(() => deriveSessionKey("zz", alice.publicKey, "call"), /private key/);
    assert.throws(() => deriveSessionKey(alice.privateKey, "abcd", "call"), /public key/);
  });
});

describe("message encryption", () => {
  const sessionKey = deriveSessionKey(generateEcdhKeyPair().privateKey, generateEcdhKeyPair().publicKey, "c").sessionKey;

  it("round-trips", () => {
    const plaintext = "hello 你好 — with punctuation, emoji 🎧 and \"quotes\"";
    assert.equal(decryptMessage(sessionKey, encryptMessage(sessionKey, plaintext)), plaintext);
  });

  it("uses a fresh IV per message", () => {
    const first = encryptMessage(sessionKey, "same plaintext");
    const second = encryptMessage(sessionKey, "same plaintext");
    assert.notEqual(first, second);
    assert.notEqual(first.split(".")[1], second.split(".")[1]);
  });

  it("tags the payload with its version", () => {
    assert.ok(encryptMessage(sessionKey, "x").startsWith("v1."));
  });

  it("rejects a tampered ciphertext instead of returning garbage", () => {
    const payload = encryptMessage(sessionKey, "transfer 100");
    const [version, iv, ciphertext] = payload.split(".");
    // Flip one byte of the ciphertext.
    const flipped = ciphertext.slice(0, -2) + (ciphertext.slice(-2) === "00" ? "01" : "00");
    assert.throws(() => decryptMessage(sessionKey, `${version}.${iv}.${flipped}`));
  });

  it("rejects a payload encrypted under a different session key", () => {
    const other = deriveSessionKey(generateEcdhKeyPair().privateKey, generateEcdhKeyPair().publicKey, "c").sessionKey;
    const payload = encryptMessage(other, "secret");
    assert.throws(() => decryptMessage(sessionKey, payload));
  });

  it("rejects malformed payloads", () => {
    assert.throws(() => decryptMessage(sessionKey, "not-a-payload"), /format/);
    assert.throws(() => decryptMessage(sessionKey, "v2.00ff.00ff"), /version/);
    assert.throws(() => decryptMessage(sessionKey, "v1.zz.00ff"), /iv format/);
    // IV must be valid here, or the IV check fires first and this would pass
    // for the wrong reason: 12 bytes of IV, one byte of ciphertext.
    assert.throws(() => decryptMessage(sessionKey, "v1.000102030405060708090a0b.00"), /ciphertext/);
  });

  it("rejects a session key of the wrong size", () => {
    assert.throws(() => encryptMessage(new Uint8Array(16), "x"), /32 bytes/);
    assert.throws(() => decryptMessage(new Uint8Array(16), "v1.00ff.00ff"), /32 bytes/);
  });
});

// The actual acceptance bar: two different implementations, one message.
describe("interoperability with an independent implementation", () => {
  it("a message from our implementation decrypts under platform WebCrypto", async () => {
    const alice = generateEcdhKeyPair();
    const bob = generateEcdhKeyPair();
    const aliceKey = deriveSessionKey(alice.privateKey, bob.publicKey, "call-x").sessionKey;

    const payload = encryptMessage(aliceKey, "cross-implementation payload");

    // Bob derives his own key - not aliceKey - and the platform decrypts.
    const bobKey = deriveSessionKey(bob.privateKey, alice.publicKey, "call-x").sessionKey;
    assert.equal(await platformDecrypt(bobKey, payload), "cross-implementation payload");
  });

  it("a message from platform WebCrypto decrypts under ours", async () => {
    const alice = generateEcdhKeyPair();
    const bob = generateEcdhKeyPair();
    const bobKey = deriveSessionKey(bob.privateKey, alice.publicKey, "call-y").sessionKey;

    const payload = await platformEncrypt(bobKey, "from the platform side");

    const aliceKey = deriveSessionKey(alice.privateKey, bob.publicKey, "call-y").sessionKey;
    assert.equal(decryptMessage(aliceKey, payload), "from the platform side");
  });

  it("both sides can talk in both directions", async () => {
    const web = generateEcdhKeyPair();
    const mobile = generateEcdhKeyPair();
    const webKey = deriveSessionKey(web.privateKey, mobile.publicKey, "call-bidi").sessionKey;
    const mobileKey = deriveSessionKey(mobile.privateKey, web.publicKey, "call-bidi").sessionKey;

    // web -> mobile
    assert.equal(decryptMessage(mobileKey, encryptMessage(webKey, "ping")), "ping");
    // mobile -> web
    assert.equal(decryptMessage(webKey, encryptMessage(mobileKey, "pong")), "pong");
    // and via the platform implementation on the other leg
    assert.equal(await platformDecrypt(mobileKey, encryptMessage(webKey, "ping-2")), "ping-2");
    assert.equal(decryptMessage(webKey, await platformEncrypt(mobileKey, "pong-2")), "pong-2");
  });

  it("both sides show the same fingerprint to compare out of band", () => {
    const web = generateEcdhKeyPair();
    const mobile = generateEcdhKeyPair();
    const webKey = deriveSessionKey(web.privateKey, mobile.publicKey, "call-fp");
    const mobileKey = deriveSessionKey(mobile.privateKey, web.publicKey, "call-fp");

    assert.equal(formatFingerprint(webKey.fingerprint), formatFingerprint(mobileKey.fingerprint));
    assert.ok(formatFingerprint(webKey.fingerprint).includes(" "));
  });
});
