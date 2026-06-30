import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const webDir = path.join(root, "web");
const openapi = path.join(root, "docs/api/openapi.yaml");
const generated = path.join(webDir, "src/api/schema.d.ts");
const bin = path.join(webDir, "node_modules/.bin", process.platform === "win32" ? "openapi-typescript.cmd" : "openapi-typescript");
const tempDir = mkdtempSync(path.join(tmpdir(), "allcallall-openapi-"));
const tempFile = path.join(tempDir, "schema.d.ts");

try {
  execFileSync(bin, [openapi, "-o", tempFile], { cwd: webDir, stdio: "pipe" });
  const expected = readFileSync(tempFile, "utf8").replace(/\r\n/g, "\n");
  const current = readFileSync(generated, "utf8").replace(/\r\n/g, "\n");
  if (expected !== current) {
    console.error("OpenAPI generated types are out of sync.");
    console.error("Run: cd web && npm run generate:api");
    process.exit(1);
  }
  console.log("OpenAPI contract is in sync.");
} finally {
  rmSync(tempDir, { recursive: true, force: true });
}
