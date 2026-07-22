# DEPRECATED — moved to the standalone `allcallall-agent-runtime` repository

This directory is a **stale mirror** of the sandbox runner code that now lives in
the standalone repository
[`allcallall-agent-runtime`](https://github.com/XianingY/allcallall-agent-runtime).

**Do not edit files here.** Changes will not take effect in the deployed runtime,
which is built from the standalone repository:

- The standalone repo is the authoritative source for the sandbox supervisor /
  runner transport under `allcallall-agent-runtime/services/sandbox-runner`
  (referenced by the Go backend's `internal/sandbox` package).
- GitHub Actions CI installs and builds this in-tree copy today
  (`sandbox-runner/Dockerfile`, `sandbox-runner/pyproject.toml`), so the
  directory is intentionally retained until CI is retargeted to the standalone
  repo.

See `contracts/README.md` for cross-repository contract governance.
