# CLAUDE.md — Guide for Claude & coding agents

AllCallAll is a realtime collaboration + AI Agent platform. This repo = Go
backend + React web + Expo mobile + Electron desktop. The Python Agent/RAG
runtime is a sibling repo `../allcallall-agent-runtime` (built in CI via
`platform-ci.yml`).

## Architecture boundaries

- Go backend (`backend/`) owns users, orgs, conversations, meetings,
  transcripts, permissions, approvals, audit logs, and write execution.
- Python runtime (`../allcallall-agent-runtime`) owns agent orchestration,
  LangGraph workflows, RAG, rerank, grounding, traces, citations, tool
  proposals, and eval.
- `contracts/` in this repo holds legacy fixtures only; authoritative schemas
  are generated/checked in the sibling repo via `make contracts-check`.

## Build & test

  Backend:  cd backend && go build ./... && go test ./internal/...
  Web:      cd web && npm run dev ; npx vitest run
  Mobile:   cd mobile && npx tsc --noEmit ; npm run test:unit
  Desktop:  cd desktop && npm run dev
  Root:     make fmt                 (gofmt -w on backend/)
            make lint                (go vet backend + npm run lint web)
            make test-backend        (backend tests)
            make verify              (backend + web/mobile tsc + python pytest)
            make web-contract-check  (web OpenAPI contract check)

There is no `make typecheck` target at the repo root; use the per-module `tsc`
commands above.

## Security defaults

- Production deployments MUST use HTTPS; never serve the API over plain HTTP in prod.
  Set `SECURITY_REQUIRE_TLS=true` so the API rejects plaintext `/api/v1` traffic.
- Message privacy policies (retention TTL, envelope encryption, recall, search
  minimization, erasure, moderation) are assembled in
  `backend/internal/runtime/privacy.go`. Any new process must call
  `ApplyPrivacyPolicies` so policy stays consistent across API and workers.
- All secrets/keys come from environment variables; never hardcode credentials or tokens.
- Do not commit `.env`, `.omo`, `.workbuddy`, or `output/`.
- Keep `ci.yml`, `backend-ci.yml`, `frontend-ci.yml`, `platform-ci.yml` green. Push over SSH.

## Docs

Start at `docs/README.md` and `INDEX.md`. This file is the canonical guide for
coding agents; `docs/reference/AGENTS.md` is a compatibility pointer.
