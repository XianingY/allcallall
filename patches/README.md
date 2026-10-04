# Vendored security patches

This directory temporarily vendors patched versions of two npm dependencies
whose upstream releases are still vulnerable:

- `braces@3.0.4-patched.0` fixes the deep nesting stack exhaustion described
  in GHSA-vfj7-8cjw-p6xm / CVE-2026-93687.
- `node-forge@1.4.1-patched.0` fixes the nested `DigestAlgorithm` signature
  forgery described in GHSA-86w9-cpqp-85rv / CVE-2026-85393.

Both packages are only used as build/tooling dependencies in this workspace,
but they are pinned here so the high-severity audit gate remains a real
blocker instead of being bypassed. Once upstream publishes fixed releases,
remove these directories and the matching `overrides` entries.

The root `package.json` declares each package as a local `file:` development
dependency and uses npm's `$dependency` override references so every transitive
consumer resolves to the same vendored copy. The vendored manifests deliberately
omit upstream development dependencies and lifecycle scripts; only runtime
metadata is retained so installing the workspace does not pull in either
project's historical build toolchain.

Run `npm run deps:patch-check` after installation to verify the resolved package
paths, patched versions, and both security regression cases. CI runs this check
before enforcing `npm audit --audit-level=high`.
