import { readdirSync, statSync } from "node:fs";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { gzip } from "node:zlib";

export async function gzipSize(filePath) {
  const input = await readFile(filePath);
  return await new Promise((resolve, reject) => {
    gzip(input, (error, result) => (error ? reject(error) : resolve(result.length)));
  });
}

async function asset(distDir, name) {
  const file = path.join(distDir, name);
  return {
    name,
    file,
    bytes: statSync(file).size,
    gzipBytes: await gzipSize(file),
  };
}

function unique(values) {
  return [...new Set(values)];
}

function htmlAttribute(html, tagName, attribute, rel) {
  const pattern = new RegExp(
    `<${tagName}[^>]*${rel ? `rel="${rel}"[^>]*` : ""}${attribute}="/assets/([^"]+)"`,
    "g",
  );
  return [...html.matchAll(pattern)].map((match) => match[1]);
}

async function staticImports(assetsDir, assetNames, name) {
  const source = await readFile(path.join(assetsDir, name), "utf8");
  const imports = [...source.matchAll(/["']\.\/([^"']+\.js)["']/g)].map((match) => match[1]);
  return unique(imports.filter((imported) => assetNames.has(imported)));
}

async function importClosure(assetsDir, assetNames, rootName) {
  const pending = [rootName];
  const visited = new Set();

  while (pending.length > 0) {
    const current = pending.pop();
    if (visited.has(current)) continue;
    visited.add(current);

    for (const imported of await staticImports(assetsDir, assetNames, current)) {
      if (!visited.has(imported)) pending.push(imported);
    }
  }

  return [...visited].sort();
}

export async function parseBundleGraph(distDir) {
  const html = await readFile(path.join(distDir, "index.html"), "utf8");
  const assetsDir = path.join(distDir, "assets");
  const names = readdirSync(assetsDir).filter((name) => /\.(?:js|css)$/.test(name));
  const assetNames = new Set(names.filter((name) => name.endsWith(".js")));

  const entry = htmlAttribute(html, "script", "src")[0];
  const modulePreloads = htmlAttribute(html, "link", "href", "modulepreload");
  const stylesheets = htmlAttribute(html, "link", "href", "stylesheet");

  const publicJsNames = unique([entry, ...modulePreloads.filter((name) => name.endsWith(".js"))]);
  const initialCssNames = unique(stylesheets.filter((name) => name.endsWith(".css")));

  const inboxRoots = names.filter((name) => /^InboxPage-[^/]+\.js$/.test(name));
  if (inboxRoots.length !== 1) {
    throw new Error(`Expected exactly one InboxPage chunk, found ${inboxRoots.length}`);
  }

  const publicJs = await Promise.all(publicJsNames.map((name) => asset(assetsDir, name)));
  const initialCss = await Promise.all(initialCssNames.map((name) => asset(assetsDir, name)));
  const inboxNames = await importClosure(assetsDir, assetNames, inboxRoots[0]);
  const inboxJs = await Promise.all(inboxNames.map((name) => asset(assetsDir, name)));

  return { publicJs, inboxJs, initialCss };
}
