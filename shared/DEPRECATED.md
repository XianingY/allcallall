# DEPRECATED — moved to the standalone `allcallall-agent-runtime` repository

This directory is a **stale mirror** of the shared Python models/utilities that
now live in the standalone repository
[`allcallall-agent-runtime`](https://github.com/XianingY/allcallall-agent-runtime).

**Do not edit files here.** The authoritative source is
`allcallall-agent-runtime/packages/shared` (a workspace package consumed by the
agent and RAG runtimes via `uv` workspace sources).

Edits to this in-tree copy do not affect the deployed runtimes, which build from
the standalone repository. It is retained only as a historical mirror. See
`contracts/README.md` for cross-repository contract governance.
