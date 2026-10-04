import assert from "node:assert/strict";
import { readFileSync, realpathSync } from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const scriptsDir = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.dirname(scriptsDir);
const requireFromRoot = createRequire(path.join(repositoryRoot, "package.json"));

const expectedPatches = {
  braces: "3.0.4-patched.0",
  "node-forge": "1.4.1-patched.0",
};

test("the workspace resolves vendored security patches", () => {
  for (const [packageName, expectedVersion] of Object.entries(expectedPatches)) {
    const installedManifestPath = requireFromRoot.resolve(`${packageName}/package.json`);
    const installedManifest = JSON.parse(readFileSync(installedManifestPath, "utf8"));
    const installedRoot = realpathSync(path.dirname(installedManifestPath));
    const vendoredRoot = realpathSync(path.join(repositoryRoot, "patches", packageName));
    const vendoredManifest = JSON.parse(
      readFileSync(path.join(vendoredRoot, "package.json"), "utf8"),
    );

    assert.equal(installedManifest.version, expectedVersion);
    assert.equal(installedRoot, vendoredRoot);
    assert.equal(vendoredManifest.devDependencies, undefined);
    assert.equal(vendoredManifest.scripts, undefined);
  }
});

test("braces rejects input beyond the supported nesting depth", () => {
  const braces = requireFromRoot("braces");
  const nestedInput = "{".repeat(257) + "value" + "}".repeat(257);

  assert.throws(
    () => braces.parse(nestedInput),
    /Input brace nesting depth \(257\), exceeds max depth \(256\)/,
  );
});

test("node-forge rejects extra DigestAlgorithm children", () => {
  const forge = requireFromRoot("node-forge");
  const { asn1 } = forge;
  const digest = "d".repeat(32);
  const algorithm = asn1.create(asn1.Class.UNIVERSAL, asn1.Type.SEQUENCE, true, [
    asn1.create(
      asn1.Class.UNIVERSAL,
      asn1.Type.OID,
      false,
      asn1.oidToDer(forge.oids.sha256).getBytes(),
    ),
    asn1.create(asn1.Class.UNIVERSAL, asn1.Type.NULL, false, ""),
    asn1.create(asn1.Class.UNIVERSAL, asn1.Type.NULL, false, ""),
  ]);
  const digestInfo = asn1.toDer(
    asn1.create(asn1.Class.UNIVERSAL, asn1.Type.SEQUENCE, true, [
      algorithm,
      asn1.create(asn1.Class.UNIVERSAL, asn1.Type.OCTETSTRING, false, digest),
    ]),
  ).getBytes();
  const modulusBytes = 128;
  const encodedDigestInfo =
    "\x00\x01" +
    "\xff".repeat(modulusBytes - digestInfo.length - 3) +
    "\x00" +
    digestInfo;
  const publicKey = forge.pki.setRsaPublicKey(
    new forge.jsbn.BigInteger("ff".repeat(modulusBytes), 16),
    new forge.jsbn.BigInteger("1", 16),
  );

  assert.throws(
    () => publicKey.verify(digest, encodedDigestInfo),
    /does not contain a valid RSASSA-PKCS1-v1_5 DigestInfo value/,
  );
});
