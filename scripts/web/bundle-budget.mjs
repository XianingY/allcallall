import path from "node:path";
import { fileURLToPath } from "node:url";

import { parseBundleGraph } from "./bundle-graph.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const distDir = path.join(root, "web/dist");
const budgets = {
  publicJsGzipBytes: 180 * 1024,
  inboxJsGzipBytes: 240 * 1024,
  initialCssGzipBytes: 16 * 1024,
};

const forbiddenPublicPatterns = [
  /vendor-agent-graph-[^/]+\.js$/,
  /vendor-agent-graph-[^/]+\.css$/,
  /vendor-revenuecat-[^/]+\.js$/,
  /vendor-firebase-[^/]+\.js$/,
];

const graph = await parseBundleGraph(distDir);
const total = (assets) => assets.reduce((sum, item) => sum + item.gzipBytes, 0);
const format = (bytes) => `${(bytes / 1024).toFixed(1)} KiB`;

const forbiddenPublicAssets = [...graph.publicJs, ...graph.initialCss].filter((item) =>
  forbiddenPublicPatterns.some((pattern) => pattern.test(item.name)),
);

const failures = [];
if (total(graph.publicJs) > budgets.publicJsGzipBytes) {
  failures.push(`Public entry JS: ${format(total(graph.publicJs))} > ${format(budgets.publicJsGzipBytes)}`);
}
if (total(graph.inboxJs) > budgets.inboxJsGzipBytes) {
  failures.push(`Inbox first-use JS: ${format(total(graph.inboxJs))} > ${format(budgets.inboxJsGzipBytes)}`);
}
if (total(graph.initialCss) > budgets.initialCssGzipBytes) {
  failures.push(`Initial CSS: ${format(total(graph.initialCss))} > ${format(budgets.initialCssGzipBytes)}`);
}
if (forbiddenPublicAssets.length > 0) {
  failures.push(`Forbidden public assets: ${forbiddenPublicAssets.map((item) => item.name).join(", ")}`);
}

const printAssets = (label, assets) => {
  console.log(`${label} (${format(total(assets))} gzip):`);
  assets.forEach((item) => {
    console.log(`- ${item.name}: ${format(item.gzipBytes)}`);
  });
};

console.log("Web bundle budget:");
printAssets("Public entry JS", graph.publicJs);
printAssets("Inbox first-use JS", graph.inboxJs);
printAssets("Initial CSS", graph.initialCss);

if (failures.length > 0) {
  console.error("Bundle budget exceeded:");
  failures.forEach((failure) => console.error(`- ${failure}`));
  process.exit(1);
}

console.log(
  `Bundle budget passed. Public JS <= ${format(budgets.publicJsGzipBytes)}, Inbox JS <= ${format(
    budgets.inboxJsGzipBytes,
  )}, CSS <= ${format(budgets.initialCssGzipBytes)}.`,
);
