# DEPRECATED — moved to the standalone `allcallall-agent-runtime` repository

This directory is a **stale mirror** of the RAG Runtime code that now lives in
the standalone repository
[`allcallall-agent-runtime`](https://github.com/XianingY/allcallall-agent-runtime).

**Do not edit files here.** Changes will not take effect in the deployed runtime,
which is built from the standalone repository:

- The standalone repo is the authoritative source:
  `allcallall-agent-runtime/services/rag-runtime`.
- Container images build from the standalone repo (see
  `infra/docker-compose.agent-runtime.local.yml`, whose `build.context` is
  `../../allcallall-agent-runtime`).
- GitHub Actions CI installs and builds this in-tree copy today
  (`rag-runtime/Dockerfile`, `rag-runtime/pyproject.toml`), so the directory is
  intentionally retained until CI is retargeted to the standalone repo.
- The authoritative HTTP API contract (JSON Schemas) lives in
  `allcallall-agent-runtime/contracts/`.

## Local development

Run the runtime from the standalone repo instead:

```bash
cd ../allcallall-agent-runtime
make install-dev
make run-rag-runtime
```

See `contracts/README.md` for cross-repository contract governance.
