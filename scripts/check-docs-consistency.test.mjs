import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, test } from "node:test";

import {
  checkDocumentationTree,
  checkMarkdownFile,
} from "./check-docs-consistency.mjs";

const temporaryRoots = [];

afterEach(async () => {
  await Promise.all(
    temporaryRoots.splice(0).map((root) => rm(root, { recursive: true, force: true })),
  );
});

async function createRepository(files) {
  const root = await mkdtemp(join(tmpdir(), "allcallall-docs-"));
  temporaryRoots.push(root);

  for (const [relativePath, source] of Object.entries(files)) {
    const path = join(root, relativePath);
    await mkdir(dirname(path), { recursive: true });
    await writeFile(path, source, "utf8");
  }

  return root;
}

test("reports a broken relative Markdown link", async () => {
  const root = await createRepository({
    "docs/README.md": "# Documentation\n\n- [Guide](guide.md)\n",
    "docs/guide.md": "# Guide\n\n[Missing](missing.md)\n",
  });

  const failures = checkDocumentationTree(root);

  assert.ok(failures.some((failure) => failure.includes("missing.md")));
});

test("reports a maintained document missing from the documentation index", async () => {
  const root = await createRepository({
    "docs/README.md": "# Documentation\n",
    "docs/guides/orphan.md": "# Orphan\n",
  });

  const failures = checkDocumentationTree(root);

  assert.ok(failures.some((failure) => failure.includes("docs/guides/orphan.md")));
});

test("does not require archived documents in the maintained index", async () => {
  const root = await createRepository({
    "docs/README.md": "# Documentation\n",
    "docs/archive/report.md": "# Historical report\n",
  });

  assert.deepEqual(checkDocumentationTree(root), []);
});

test("reports an opening code fence without a language tag", async () => {
  const root = await createRepository({});

  const failures = checkMarkdownFile(
    root,
    "docs/guide.md",
    "# Guide\n\n```\nmake test\n```\n",
  );

  assert.ok(failures.some((failure) => failure.includes("language tag")));
});

test("reports multiple H1 headings and skipped heading levels", async () => {
  const root = await createRepository({});

  const failures = checkMarkdownFile(
    root,
    "docs/guide.md",
    "# Guide\n\n### Details\n\n# Duplicate\n",
  );

  assert.ok(failures.some((failure) => failure.includes("exactly one H1")));
  assert.ok(failures.some((failure) => failure.includes("skips from H1 to H3")));
});

test("reports a compatibility pointer whose target is missing", async () => {
  const root = await createRepository({});

  const failures = checkMarkdownFile(
    root,
    "INDEX.md",
    "# Documentation Index\n\nThis compatibility page preserves an older link.\n\n[Documentation](docs/README.md)\n",
  );

  assert.ok(failures.some((failure) => failure.includes("docs/README.md")));
});

test("reports missing or unlinked governance files", async () => {
  const root = await createRepository({
    "README.md": "# Project\n\n[Contributing](CONTRIBUTING.md)\n",
    "docs/README.md": "# Documentation\n",
    "CONTRIBUTING.md": "# Contributing\n",
    "SECURITY.md": "# Security\n",
  });

  const failures = checkDocumentationTree(root);

  for (const governanceFile of [
    "LICENSE",
    "SECURITY.md",
    "CODE_OF_CONDUCT.md",
    "SUPPORT.md",
  ]) {
    assert.ok(
      failures.some((failure) => failure.includes(governanceFile)),
      `expected a governance failure for ${governanceFile}`,
    );
  }
});
