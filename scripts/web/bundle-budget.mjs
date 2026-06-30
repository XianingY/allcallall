import { readdirSync, statSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const assetsDir = path.join(root, "web/dist/assets");
const maxJSBytes = Number(process.env.WEB_BUNDLE_MAX_JS_BYTES || 820 * 1024);
const maxCSSBytes = Number(process.env.WEB_BUNDLE_MAX_CSS_BYTES || 140 * 1024);

const assets = readdirSync(assetsDir)
  .filter((name) => name.endsWith(".js") || name.endsWith(".css"))
  .map((name) => {
    const file = path.join(assetsDir, name);
    return { name, size: statSync(file).size, type: name.endsWith(".js") ? "js" : "css" };
  })
  .sort((left, right) => right.size - left.size);

const failures = assets.filter((asset) => asset.size > (asset.type === "js" ? maxJSBytes : maxCSSBytes));
const format = (bytes) => `${(bytes / 1024).toFixed(1)} KiB`;

console.log("Largest web bundles:");
assets.slice(0, 10).forEach((asset) => {
  console.log(`- ${asset.name}: ${format(asset.size)}`);
});

if (failures.length) {
  console.error("Bundle budget exceeded:");
  failures.forEach((asset) => {
    const limit = asset.type === "js" ? maxJSBytes : maxCSSBytes;
    console.error(`- ${asset.name}: ${format(asset.size)} > ${format(limit)}`);
  });
  process.exit(1);
}

console.log(`Bundle budget passed. JS <= ${format(maxJSBytes)}, CSS <= ${format(maxCSSBytes)}.`);
