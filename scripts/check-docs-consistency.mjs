#!/usr/bin/env node

import {
  existsSync,
  readFileSync,
  readdirSync,
} from "node:fs";
import { dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const DEFAULT_ROOT = resolve(import.meta.dirname, "..");
const DOCUMENTATION_INDEX = "docs/README.md";
const GOVERNANCE_FILES = [
  "LICENSE",
  "CONTRIBUTING.md",
  "SECURITY.md",
  "CODE_OF_CONDUCT.md",
  "SUPPORT.md",
];
const IGNORED_DIRECTORIES = new Set([
  ".git",
  ".venv",
  ".worktrees",
  "build",
  "coverage",
  "dist",
  "node_modules",
  "output",
  "vendor",
]);
const COMPATIBILITY_POINTER_MARKERS = [
  "compatibility page preserves an older link",
  "compatibility pointer",
  "compatibility reference",
  "this pointer exists to keep old links",
];
const ARCHIVE_STATUS_MARKER =
  "archive status:** historical material; not part of the maintained product documentation";

const REFERENCE_PATTERNS = [
  {
    label: "GitHub workflow",
    re: /\.github\/workflows\/([A-Za-z0-9._-]+\.ya?ml)/g,
    resolve: (match) => join(".github", "workflows", match),
  },
  {
    label: "repository file",
    re: /`((?:backend|web|mobile|desktop|infra|deploy|scripts|packages|contracts)\/[A-Za-z0-9._/-]+\.(?:go|py|ts|tsx|js|mjs|cjs|json|ya?ml|sh|sql|md))`/g,
    resolve: (match) => match,
  },
];

function toPosix(path) {
  return path.split(sep).join("/");
}

function lineNumber(source, offset) {
  return source.slice(0, offset).split("\n").length;
}

function isExternalLink(target) {
  return /^(?:[a-z][a-z0-9+.-]*:|\/\/)/i.test(target);
}

function stripLinkDestination(target) {
  const unwrapped = target.trim().replace(/^<|>$/g, "");
  return decodeURIComponent(unwrapped.split(/[?#]/, 1)[0]);
}

function isArchiveOrGenerated(relativePath) {
  const path = toPosix(relativePath);
  return (
    path.startsWith("docs/archive/") ||
    path.startsWith("docs/superpowers/") ||
    /(^|\/)generated-[^/]+\//.test(path)
  );
}

function isCompatibilityPointer(source) {
  const lower = source.toLowerCase();
  return COMPATIBILITY_POINTER_MARKERS.some((marker) => lower.includes(marker));
}

function isArchivedSource(source) {
  return source.toLowerCase().includes(ARCHIVE_STATUS_MARKER);
}

function sourceOutsideFences(source) {
  const lines = source.split("\n");
  const visible = [];
  let fence = null;

  for (const line of lines) {
    const match = line.match(/^ {0,3}(`{3,}|~{3,})(.*)$/);
    if (!match) {
      visible.push(fence ? "" : line);
      continue;
    }

    const marker = match[1];
    if (fence === null) {
      fence = { character: marker[0], length: marker.length };
      visible.push("");
      continue;
    }

    if (
      marker[0] === fence.character &&
      marker.length >= fence.length &&
      match[2].trim() === ""
    ) {
      fence = null;
    }
    visible.push("");
  }

  return visible.join("\n");
}

export function collectMarkdownFiles(root) {
  const files = [];

  function walk(directory) {
    if (!existsSync(directory)) {
      return;
    }

    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      if (entry.isDirectory() && IGNORED_DIRECTORIES.has(entry.name)) {
        continue;
      }

      const absolutePath = join(directory, entry.name);
      if (entry.isDirectory()) {
        walk(absolutePath);
      } else if (entry.isFile() && entry.name.endsWith(".md")) {
        files.push(toPosix(relative(root, absolutePath)));
      }
    }
  }

  walk(root);
  return files.sort();
}

export function checkMarkdownFile(root, relativePath, source) {
  const failures = [];
  const documentDirectory = dirname(join(root, relativePath));
  const lines = source.split("\n");
  const headings = [];
  let fence = null;

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const fenceMatch = line.match(/^ {0,3}(`{3,}|~{3,})(.*)$/);

    if (fenceMatch) {
      const marker = fenceMatch[1];
      const suffix = fenceMatch[2].trim();
      if (fence === null) {
        if (suffix === "") {
          failures.push(`${relativePath}:${index + 1}: opening code fence needs a language tag`);
        }
        fence = { character: marker[0], length: marker.length };
      } else if (
        marker[0] === fence.character &&
        marker.length >= fence.length &&
        suffix === ""
      ) {
        fence = null;
      }
      continue;
    }

    if (fence !== null) {
      continue;
    }

    const headingMatch = line.match(/^(#{1,6})\s+\S/);
    if (headingMatch) {
      headings.push({ level: headingMatch[1].length, line: index + 1 });
    }
  }

  const h1Count = headings.filter(({ level }) => level === 1).length;
  if (h1Count !== 1) {
    failures.push(`${relativePath}: expected exactly one H1, found ${h1Count}`);
  }

  for (let index = 1; index < headings.length; index += 1) {
    const previous = headings[index - 1];
    const current = headings[index];
    if (current.level > previous.level + 1) {
      failures.push(
        `${relativePath}:${current.line}: heading skips from H${previous.level} to H${current.level}`,
      );
    }
  }

  const visibleSource = sourceOutsideFences(source);
  const linkPattern = /!?\[[^\]]*\]\(([^)]+)\)/g;
  let linkMatch;
  while ((linkMatch = linkPattern.exec(visibleSource)) !== null) {
    const rawTarget = linkMatch[1].trim().split(/\s+["']/u, 1)[0];
    if (rawTarget.startsWith("#") || isExternalLink(rawTarget)) {
      continue;
    }

    let target;
    try {
      target = stripLinkDestination(rawTarget);
    } catch {
      failures.push(
        `${relativePath}:${lineNumber(visibleSource, linkMatch.index)}: invalid link target "${rawTarget}"`,
      );
      continue;
    }
    if (target === "") {
      continue;
    }

    const resolvedTarget = resolve(documentDirectory, target);
    if (!existsSync(resolvedTarget)) {
      failures.push(
        `${relativePath}:${lineNumber(visibleSource, linkMatch.index)}: relative link "${rawTarget}" does not resolve`,
      );
    }
  }

  for (const { label, re, resolve: resolveReference } of REFERENCE_PATTERNS) {
    re.lastIndex = 0;
    let match;
    while ((match = re.exec(visibleSource)) !== null) {
      const target = resolveReference(match[1]);
      if (!existsSync(join(root, target))) {
        failures.push(
          `${relativePath}:${lineNumber(visibleSource, match.index)}: references ${label} "${target}", which does not exist`,
        );
      }
    }
  }

  return failures;
}

export function checkDocumentationIndex(root, maintainedFiles, indexSource) {
  const failures = [];
  const indexDirectory = dirname(join(root, DOCUMENTATION_INDEX));
  const indexedFiles = new Set();
  const linkPattern = /!?\[[^\]]*\]\(([^)]+)\)/g;
  let match;

  while ((match = linkPattern.exec(sourceOutsideFences(indexSource))) !== null) {
    const rawTarget = match[1].trim().split(/\s+["']/u, 1)[0];
    if (rawTarget.startsWith("#") || isExternalLink(rawTarget)) {
      continue;
    }
    try {
      const target = stripLinkDestination(rawTarget);
      if (target !== "") {
        indexedFiles.add(toPosix(relative(root, resolve(indexDirectory, target))));
      }
    } catch {
      // Invalid destinations are reported by checkMarkdownFile.
    }
  }

  for (const relativePath of maintainedFiles) {
    const normalizedPath = toPosix(relativePath);
    if (normalizedPath === DOCUMENTATION_INDEX || isArchiveOrGenerated(normalizedPath)) {
      continue;
    }

    const absolutePath = join(root, normalizedPath);
    if (existsSync(absolutePath)) {
      const source = readFileSync(absolutePath, "utf8");
      if (isCompatibilityPointer(source) || isArchivedSource(source)) {
        continue;
      }
    }

    if (!indexedFiles.has(normalizedPath)) {
      failures.push(`${normalizedPath}: maintained document is not linked from ${DOCUMENTATION_INDEX}`);
    }
  }

  return failures;
}

export function checkGovernanceFiles(root, readmeSource) {
  const failures = [];
  const linkedFiles = new Set();
  const linkPattern = /!?\[[^\]]*\]\(([^)]+)\)/g;
  let match;

  while ((match = linkPattern.exec(sourceOutsideFences(readmeSource))) !== null) {
    const rawTarget = match[1].trim().split(/\s+["']/u, 1)[0];
    if (rawTarget.startsWith("#") || isExternalLink(rawTarget)) {
      continue;
    }
    try {
      const target = stripLinkDestination(rawTarget);
      if (target !== "") {
        linkedFiles.add(toPosix(relative(root, resolve(root, target))));
      }
    } catch {
      // Invalid destinations are reported by checkMarkdownFile.
    }
  }

  for (const governanceFile of GOVERNANCE_FILES) {
    if (!existsSync(join(root, governanceFile))) {
      failures.push(`${governanceFile}: required governance file is missing`);
      continue;
    }
    if (!linkedFiles.has(governanceFile)) {
      failures.push(`${governanceFile}: governance file is not linked from README.md`);
    }
  }

  return failures;
}

export function checkDocumentationTree(root) {
  const failures = [];
  const markdownFiles = collectMarkdownFiles(root);

  for (const relativePath of markdownFiles) {
    const source = readFileSync(join(root, relativePath), "utf8");
    if (isArchiveOrGenerated(relativePath) || isArchivedSource(source)) {
      continue;
    }
    failures.push(
      ...checkMarkdownFile(root, relativePath, source),
    );
  }

  const indexPath = join(root, DOCUMENTATION_INDEX);
  if (!existsSync(indexPath)) {
    failures.push(`${DOCUMENTATION_INDEX}: canonical documentation index is missing`);
    return failures;
  }

  const maintainedFiles = markdownFiles.filter((path) => path.startsWith("docs/"));
  failures.push(
    ...checkDocumentationIndex(root, maintainedFiles, readFileSync(indexPath, "utf8")),
  );
  const readmePath = join(root, "README.md");
  if (existsSync(readmePath)) {
    failures.push(...checkGovernanceFiles(root, readFileSync(readmePath, "utf8")));
  }
  return failures;
}

function main() {
  const failures = checkDocumentationTree(DEFAULT_ROOT);
  if (failures.length > 0) {
    for (const failure of failures) {
      console.error(failure);
    }
    console.error(`\ncheck-docs-consistency: ${failures.length} problem(s)`);
    process.exitCode = 1;
    return;
  }

  const checked = collectMarkdownFiles(DEFAULT_ROOT).length;
  console.log(`check-docs-consistency: ok (${checked} Markdown file(s) checked)`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
