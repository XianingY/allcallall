# FAQ

Frequently asked questions for AllCallAll development. If an answer conflicts
with current code, prefer the root `README.md`, `backend/README.md`, and
`docs/README.md`.

## Repository layout

### Why are there two repositories?

AllCallAll is one system split across two git repositories:

- `AllCallAll` (this repo) — Go backend, React web, Expo mobile, Electron
  desktop, infra.
- `allcallall-agent-runtime` — standalone Python Agent + RAG runtime
  (FastAPI + LangGraph), intentionally separated from the Go backend.

They communicate over HTTP (Tool Bridge, shared
`AGENT_RUNTIME_TOOL_BRIDGE_TOKEN`). Write operations are proposal-only in
Python; the Go backend approves and executes them.

### Where did the in-repo Python directories go?

`agent-runtime/`, `rag-runtime/`, `shared/`, `sandbox-runner/`, and
`interview-mcp/` were removed from this repository on 2026-07-22 after being
migrated to `allcallall-agent-runtime`. See `contracts/README.md` for the
authoritative boundary description.

### Where must the runtime repo be checked out?

As a sibling directory: `../allcallall-agent-runtime`. The Makefile targets
(`make run-agent-runtime`, `make interview-up`), docker-compose build
contexts, and CI checkouts all depend on this layout.

## Local development

### How do I start everything locally?

See [Quick Start](./getting-started/quick-start.md) for the full flow. Short
version: `make dev-up` style compose targets in the root `Makefile`, backend
via `cd backend && go run ./cmd/server`, web via `cd web && npm run dev`.

### How do I run the test suites?

- Backend: `cd backend && go build ./... && go test ./...`
- Web: `cd web && npx vitest run` (add `--coverage` for coverage; thresholds
  in `web/vite.config.ts` are a non-regression floor)
- Mobile: `cd mobile && npx tsc --noEmit` (type check; `npm run test:unit`
  runs an explicitly enumerated file list — new tests must be added to that
  script manually)
- Agent runtime (sibling repo): `make lint typecheck test contracts-check
  agent-eval rag-eval`

### Git push fails over HTTPS — what now?

Both repositories must be pushed over SSH
(`git@github.com:XianingY/AllCallAll`,
`git@github.com:XianingY/allcallall-agent-runtime`); HTTPS pushes hit SSL
errors in this environment.

## CI/CD

### Which workflows run on this repo?

- `ci.yml` — main pipeline: backend + integration + web + mobile + desktop + e2e
- `backend-ci.yml` — backend tests, `go vet`, coverage artifact
- `frontend-ci.yml` — web `test:coverage` + mobile `test:unit`
- `platform-ci.yml` — Python runtime jobs (checked out from
  `allcallall-agent-runtime`), sandbox-go, helm, image scans

### Why is ruff pinned in the runtime repo?

`ruff>=0.15,<0.16` is pinned across all runtime pyproject files because a
floating range once pulled a new ruff release whose added rules broke CI with
93 new findings unrelated to the change under test.

## Security / configuration

### Is the support API open by default?

No. `SUPPORT_INTERNAL_ONLY` is fail-closed: unless explicitly set to
`false`/`off`/`0`/`no`, the internal support API only accepts requests from
internal networks (403 `SUPPORT_NETWORK_FORBIDDEN` otherwise).

### Is there global rate limiting?

Yes. `GlobalRateLimit` middleware applies per-client-IP coarse limiting to
all endpoints except `/health` (`GLOBAL_RATE_LIMIT`, default 600 per
`GLOBAL_RATE_WINDOW`, default 1m). It fails open if Redis is unhealthy.

### Where are mobile E2EE keys stored?

In the platform Keychain (native targets). Web targets use ephemeral
`sessionStorage` only. Identity keys are never written to plaintext
`AsyncStorage`; a one-time migration moved legacy copies into the Keychain.

### Where is the full environment variable reference?

- Backend / web / mobile / infra: [Configuration](./configuration/configuration.md)
- Python runtime: `allcallall-agent-runtime/docs/configuration.md`

## Documentation

### Where is the documentation index?

- Cross-repo master index: root [`INDEX.md`](../INDEX.md)
- This repo's maintained doc set: [`docs/README.md`](./README.md)
- Runtime repo index: `allcallall-agent-runtime/INDEX.md`

### Which docs are machine-generated?

`docs/interview/generated-*` directories (and
`docs/generated-ai-agent-portfolio-eval/` in the runtime repo). Regenerate
them with the corresponding `make` targets; do not edit by hand.
