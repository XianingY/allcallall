import { webcrypto } from "node:crypto";
import { describe, expect, it } from "vitest";

import { decryptMessage, deriveSessionKey, encryptMessage, generateEcdhKeyPair } from "./index";

// Smoke test for the web bundle: proves the shared implementation is usable
// from this client and interoperates with an independent AES-GCM
// implementation. The full test matrix lives in packages/shared.

describe("web e2ee", () => {
  it("derives the same session key as the peer", () => {
    const web = generateEcdhKeyPair();
    const mobile = generateEcdhKeyPair();
    const webKey = deriveSessionKey(web.privateKey, mobile.publicKey, "call-web").sessionKey;
    const mobileKey = deriveSessionKey(mobile.privateKey, web.publicKey, "call-web").sessionKey;
    expect([...webKey]).toEqual([...mobileKey]);
  });

  it("round-trips through the payload format", () => {
    const key = deriveSessionKey(generateEcdhKeyPair().privateKey, generateEcdhKeyPair().publicKey, "call-web").sessionKey;
    const payload = encryptMessage(key, "web round trip");
    expect(decryptMessage(key, payload)).toBe("web round trip");
  });

  it("interoperates with platform WebCrypto", async () => {
    const key = deriveSessionKey(generateEcdhKeyPair().privateKey, generateEcdhKeyPair().publicKey, "call-x").sessionKey;

    // Shared encrypts, WebCrypto decrypts.
    const encrypted = encryptMessage(key, "one direction");
    const [version, ivHex, ciphertextHex] = encrypted.split(".");
    expect(version).toBe("v1");
    const iv = Uint8Array.from(ivHex.match(/.{2}/g)!.map((pair) => parseInt(pair, 16)));
    const ciphertext = Uint8Array.from(ciphertextHex.match(/.{2}/g)!.map((pair) => parseInt(pair, 16)));
    const cryptoKey = await webcrypto.subtle.importKey("raw", key, "AES-GCM", false, ["decrypt"]);
    const plain = await webcrypto.subtle.decrypt({ name: "AES-GCM", iv }, cryptoKey, ciphertext);
    expect(new TextDecoder().decode(plain)).toBe("one direction");

    // WebCrypto encrypts, shared decrypts.
    const iv2 = webcrypto.getRandomValues(new Uint8Array(12));
    const cryptoKey2 = await webcrypto.subtle.importKey("raw", key, "AES-GCM", false, ["encrypt"]);
    const ciphertext2 = await webcrypto.subtle.encrypt({ name: "AES-GCM", iv: iv2 }, cryptoKey2, new TextEncoder().encode("other direction"));
    const hex = (bytes: Uint8Array) => [...bytes].map((b) => b.toString(16).padStart(2, "0")).join("");
    expect(decryptMessage(key, `v1.${hex(iv2)}.${hex(new Uint8Array(ciphertext2))}`)).toBe("other direction");
  });
});
