import assert from "node:assert/strict";
import test from "node:test";

import {
  canBindInstallationToSkill,
  canManageScopedResource,
  isPublicHTTPSEndpoint,
  splitNonEmptyLines,
  toolExecutionPolicy,
  validateInstallationDraft,
  validateSecretDrafts,
  type MCPInstallationDraft,
} from "./mcpPlatformUtils";

const draft = (
  input: Partial<MCPInstallationDraft>,
): MCPInstallationDraft => ({
  displayName: "Calendar tools",
  scope: "personal",
  sourceType: "https",
  transport: "streamable_http",
  imageRef: "",
  endpointURL: "https://mcp.example.com/v1",
  commandLines: "",
  argumentLines: "",
  allowlistLines: "",
  ...input,
});

test("accepts a public HTTPS MCP endpoint", () => {
  const result = validateInstallationDraft(draft({}));
  assert.deepEqual(result.errors, {});
  assert.equal(result.value?.endpoint_url, "https://mcp.example.com/v1");
  assert.equal(result.value?.transport, "streamable_http");
});

test("rejects insecure, credentialed, and private HTTPS endpoints", () => {
  assert.equal(isPublicHTTPSEndpoint("http://mcp.example.com"), false);
  assert.equal(isPublicHTTPSEndpoint("https://user:pass@mcp.example.com"), false);
  assert.equal(isPublicHTTPSEndpoint("https://127.0.0.1/mcp"), false);
  assert.equal(isPublicHTTPSEndpoint("https://192.168.1.8/mcp"), false);
  assert.equal(isPublicHTTPSEndpoint("https://service.internal/mcp"), false);
  assert.equal(isPublicHTTPSEndpoint("https://[fc00::1]/mcp"), false);
  assert.equal(isPublicHTTPSEndpoint("https://fcloud.example/mcp"), true);
});

test("requires an OCI image pinned to a sha256 digest", () => {
  const invalid = validateInstallationDraft(
    draft({ sourceType: "oci", imageRef: "ghcr.io/acme/mcp:latest" }),
  );
  assert.match(invalid.errors.imageRef, /sha256/);

  const digest = "a".repeat(64);
  const valid = validateInstallationDraft(
    draft({
      sourceType: "oci",
      imageRef: `ghcr.io/acme/mcp@sha256:${digest}`,
      commandLines: "python\n-m\nserver",
    }),
  );
  assert.deepEqual(valid.errors, {});
  assert.deepEqual(valid.value?.command, ["python", "-m", "server"]);
  assert.equal(valid.value?.transport, "stdio");
});

test("secret mapper rejects duplicate keys and keeps values verbatim", () => {
  const duplicate = validateSecretDrafts([
    { key: "API_TOKEN", value: "one" },
    { key: "API_TOKEN", value: "two" },
  ]);
  assert.match(duplicate.errors["1"], /重复/);

  const valid = validateSecretDrafts([
    { key: "API_TOKEN", value: "  token with spaces  " },
  ]);
  assert.deepEqual(valid.value, { API_TOKEN: "  token with spaces  " });
});

test("line parser preserves structured argv boundaries", () => {
  assert.deepEqual(splitNonEmptyLines(" alpha \n\n--flag=value\n beta "), [
    "alpha",
    "--flag=value",
    "beta",
  ]);
});

test("scope permissions keep organization resources admin-only", () => {
  assert.equal(canManageScopedResource("personal", false), true);
  assert.equal(canManageScopedResource("organization", false), false);
  assert.equal(canManageScopedResource("organization", true), true);
});

test("organization Skills only accept organization installation tools", () => {
  assert.equal(canBindInstallationToSkill("organization", "personal"), false);
  assert.equal(canBindInstallationToSkill("organization", "organization"), true);
  assert.equal(canBindInstallationToSkill("personal", "organization"), true);
});

test("write and unknown tools explain their approval requirement", () => {
  assert.match(toolExecutionPolicy("read"), /直接执行/);
  assert.match(toolExecutionPolicy("write"), /人工审批/);
  assert.match(toolExecutionPolicy("unknown"), /人工审批/);
});
