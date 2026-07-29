# Cross-Repository Contract Governance

This directory and the standalone
[`allcallall-agent-runtime`](https://github.com/XianingY/allcallall-agent-runtime)
repository together define the API boundary between the **Go backend**
(product source of truth) and the **Python Agent/RAG runtimes**.

## Source of truth

The authoritative contract artifacts live in the standalone repository:

- `allcallall-agent-runtime/contracts/schemas/*.schema.json` — JSON Schemas for
  the HTTP API boundary (agent run request/response, RAG retrieval request/response).
- `allcallall-agent-runtime/contracts/fixtures/*.json` — golden request/response
  fixtures used by the runtime's own contract checks (`make contracts-check`).
- `allcallall-agent-runtime/contracts/README.md` — runtime-side contract notes.

This main repository's `contracts/fixtures/` contains **legacy sample payloads**
(`agent_run_request.json`, `agent_run_response.json`, `rag_retrieval_request.json`,
`rag_retrieval_response.json`). They are kept for reference only and are **not**
referenced by the Go build or tests; treat the standalone repo as canonical.

## Governance rule

1. Any change to the Go ↔ Python runtime HTTP API boundary MUST update the JSON
   Schemas in `allcallall-agent-runtime/contracts/schemas/` first.
2. The Go backend MUST stay wire-compatible with those schemas; the Python
   runtimes validate outbound/inbound payloads against them (`make contracts-check`).
3. The in-tree `agent-runtime/`, `rag-runtime/`, `shared/`, `sandbox-runner/`,
   and `interview-mcp/` Python directories were **removed** from this main
   repository (they were stale mirrors; each carried a `DEPRECATED.md`). The
   deployed runtime is built exclusively from the standalone repository. The
   Python runtimes are checked out as a sibling `allcallall-agent-runtime/`
   during local development (`make run-agent-runtime`, `make interview-up`) and
   in CI (`platform-ci.yml`).

## Why two repos

Python owns agent orchestration, LangGraph workflows, bounded ReAct loops,
prompt/provider adapters, Agentic RAG, rerank, grounding, traces, citations,
tool proposals, and deterministic eval. Go owns users, organizations,
conversations, meetings, transcripts, permissions, approvals, audit logs, and
write execution. Splitting the runtime into its own repository keeps the Python
release cadence independent of the Go monorepo and avoids a large, rarely-built
Python subtree in the main repository.

## Note for contributors (this repository)

- This repository's `contracts/` directory contains **only legacy fixtures**
  (`agent_run_request.json`, `agent_run_response.json`, `rag_retrieval_request.json`,
  `rag_retrieval_response.json`). They are reference samples, not the build/test source
  of truth, and are not consumed by the Go build.
- The **authoritative** JSON Schemas and golden fixtures are generated and validated in
  the standalone sibling repository `allcallall-agent-runtime` (checked out at
  `../allcallall-agent-runtime`) via `make contracts-check`.
- Go ↔ Python wire-contract consistency is guarded by that repository's CI
  (`platform-ci.yml`). Any change to the runtime HTTP API boundary must update the
  schemas there first; the Go backend must stay compatible. Do not treat the in-tree
  `contracts/` fixtures as canonical.
