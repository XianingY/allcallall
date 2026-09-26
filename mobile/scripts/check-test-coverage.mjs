#!/usr/bin/env node
/**
 * Guard against tests that silently never run.
 *
 * The mobile app has two runners on purpose: test:unit (node:test via tsx) for
 * pure logic, and test:jest (jest-expo) for anything that needs the React
 * Native runtime. That split means a test file can fall through both - it is
 * easy to add src/screens/Foo.test.ts, see `npm test` pass, and never realise
 * the new suite never executed.
 *
 * This script asserts every *.test.ts(x) under src is claimed by at least one
 * runner, and prints the split. Run it from `npm test`.
 */

import { readFileSync } from "node:fs";
import { readdir } from "node:fs/promises";
import { join, relative, resolve, sep } from "node:path";

const rootDir = resolve(import.meta.dirname, "..");
const srcDir = join(rootDir, "src");

const packageJson = JSON.parse(readFileSync(join(rootDir, "package.json"), "utf8"));

/** Walk src and collect every test file, normalised to posix separators. */
async function collectTestFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...(await collectTestFiles(full)));
      continue;
    }
    if (/\.test\.tsx?$/.test(entry.name)) {
      files.push(relative(rootDir, full).split(sep).join("/"));
    }
  }
  return files;
}

/**
 * Translate a micromatch-style glob into a regular expression. Only the
 * constructs used by this project's jest config are handled; anything else
 * throws rather than silently mismatching.
 */
function globToRegExp(pattern) {
  let source = pattern.replace(/^<rootDir>\//, "");
  if (source.includes("{") || source.includes("!")) {
    throw new Error(`unsupported glob syntax in testMatch: ${pattern}`);
  }

  // Placeholders first: expanding "**/" straight to "(?:[^/]+/)*" would leave a
  // literal "*" in the result, which the later "*" rule would then rewrite and
  // corrupt. Swap in sentinels, then substitute once.
  const DIR_WILDCARD = "\u0000dir\u0000";
  const ANY_WILDCARD = "\u0000any\u0000";
  const NAME_WILDCARD = "\u0000name\u0000";
  const CHAR_WILDCARD = "\u0000char\u0000";

  source = source
    .replace(/[.+^${}()|[\]\\]/g, "\\$&")
    .replace(/\*\*\//g, DIR_WILDCARD)
    .replace(/\*\*/g, ANY_WILDCARD)
    .replace(/\*/g, NAME_WILDCARD)
    .replace(/\?/g, CHAR_WILDCARD)
    .split(DIR_WILDCARD).join("(?:[^/]+/)*")
    .split(ANY_WILDCARD).join(".*")
    .split(NAME_WILDCARD).join("[^/]*")
    .split(CHAR_WILDCARD).join("[^/]");

  return new RegExp(`^${source}$`);
}

function matchesAny(patterns, file) {
  return patterns.some((pattern) => globToRegExp(pattern).test(file));
}

const unitFiles = packageJson.scripts["test:unit"]
  .split(/\s+/)
  .filter((token) => token.endsWith(".ts") || token.endsWith(".tsx"))
  .map((file) => file.replace(/^\.\//, ""));

const jestConfig = packageJson.jest ?? {};
const testMatch = jestConfig.testMatch ?? [];
const ignorePatterns = (jestConfig.testPathIgnorePatterns ?? []).filter((p) => p !== "/node_modules/");

const testFiles = (await collectTestFiles(srcDir)).sort();

const orphans = [];
const byUnit = [];
const byJest = [];

for (const file of testFiles) {
  const inUnit = unitFiles.includes(file);
  const inJest = matchesAny(testMatch, file) && !ignorePatterns.some((p) => file.includes(p.replace(/^\.\//, "")));
  if (inUnit) byUnit.push(file);
  if (inJest && !inUnit) byJest.push(file);
  if (!inUnit && !inJest) orphans.push(file);
}

// Should not happen with a broad testMatch, but a file listed for unit and also
// matched by jest would run twice on every `npm test`.
const duplicated = testFiles.filter(
  (file) => unitFiles.includes(file) && matchesAny(testMatch, file) && !ignorePatterns.some((p) => file.includes(p.replace(/^\.\//, ""))),
);

console.log(`[test-coverage] ${testFiles.length} test file(s)`);
console.log(`[test-coverage]   node runner (test:unit): ${byUnit.length}`);
console.log(`[test-coverage]   jest (test:jest):       ${byJest.length}`);

if (orphans.length > 0) {
  console.error("\n[test-coverage] ERROR: these test files are claimed by neither runner:");
  for (const file of orphans) console.error(`  - ${file}`);
  console.error("\nAdd pure-logic tests to the test:unit list, or let jest pick them up.");
  process.exit(1);
}

if (duplicated.length > 0) {
  console.error("\n[test-coverage] ERROR: these files run in BOTH runners:");
  for (const file of duplicated) console.error(`  - ${file}`);
  console.error("\nAdd them to jest testPathIgnorePatterns so they only run once.");
  process.exit(1);
}

console.log("[test-coverage] OK: every test file has exactly one runner");
