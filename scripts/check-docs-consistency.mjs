#!/usr/bin/env node
/**
 * Fails when documentation references files that do not exist.
 *
 * The audit that produced the current plan found the docs making claims that
 * had drifted from the code: AGENTS.md told contributors CI runs
 * `backend-ci.yml`, which was never a file, and README.md described Kubernetes
 * as unimplemented while a full Helm chart sat in infra/. Both cost real time
 * to anyone who believed them.
 *
 * This checks the mechanical part - the referenced file must exist - which is
 * the part that can be checked without judgement. It deliberately does not try
 * to verify prose.
 *
 * Run from the repository root.
 */

import { existsSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");

/** Documents whose references must resolve. */
const DOCS = ["README.md", "AGENTS.md", "CLAUDE.md", "CONTRIBUTING.md", "docs/README.md", "INDEX.md"];

/**
 * Only paths that are unambiguously files are checked. Bare names such as
 * "Dockerfile" or example snippets would produce false positives.
 */
const REFERENCE_PATTERNS = [
  { label: "GitHub workflow", re: /\.github\/workflows\/([A-Za-z0-9._-]+\.ya?ml)/g, resolve: (m) => join(".github", "workflows", m) },
  { label: "repo file", re: /`((?:backend|web|mobile|desktop|infra|scripts|packages)\/[A-Za-z0-9._/-]+\.(?:go|ts|tsx|js|mjs|cjs|json|ya?ml|sh|sql|md))`/g, resolve: (m) => m },
];

let failures = 0;
let checked = 0;

for (const doc of DOCS) {
  const path = join(root, doc);
  if (!existsSync(path)) {
    continue;
  }
  const text = readFileSync(path, "utf8");

  for (const { label, re } of REFERENCE_PATTERNS) {
    // Fresh lastIndex per document: the regex is shared across iterations.
    re.lastIndex = 0;
    let match;
    while ((match = re.exec(text)) !== null) {
      const relative = REFERENCE_PATTERNS.find((p) => p.label === label).resolve(match[1]);
      checked += 1;
      if (!existsSync(join(root, relative))) {
        const line = text.slice(0, match.index).split("\n").length;
        console.error(`${doc}:${line}: references ${label} "${relative}", which does not exist`);
        failures += 1;
      }
    }
  }
}

if (failures > 0) {
  console.error(`\ncheck-docs-consistency: ${failures} stale reference(s) out of ${checked} checked`);
  console.error("Fix the document, or add the missing file - do not leave a pointer to nothing.");
  process.exit(1);
}

console.log(`check-docs-consistency: ok (${checked} reference(s) resolve)`);
