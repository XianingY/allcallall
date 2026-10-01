# AGENTS.md — AllCallAll Contributor & Coding-Agent Guide

AllCallAll is a realtime collaboration + AI Agent platform. This monorepo holds
the Go backend, the React web app, the Expo mobile app, and the Electron desktop
shell. The Python Agent/RAG runtime lives in a separate repository
(`../allcallall-agent-runtime`), checked out as a sibling directory and built in
CI (`platform-ci.yml`).

## Repository layout

- `backend/` — Go (Gin) API + embedded/extracted workers. Source of truth for product data.
- `web/` — React + Vite production web client.
- `mobile/` — Expo native Android/iOS app.
- `desktop/` — Electron wrapper around the web app.
- `contracts/` — legacy JSON fixtures only; authoritative schemas live in the sibling Python repo.
- `docs/` — maintained documentation set; entry point `docs/README.md` and `INDEX.md`.

## Build & test commands

**Dependency installs always run from the repository root.** This is an npm
workspaces repo and the root `package-lock.json` is the single source of
truth. `web/` and `mobile/` used to carry their own lockfiles; they were
removed because nothing consumed them (CI runs `npm ci` at the root) and they
had drifted out of sync with the root one — a workspace lock is worse than no
lock when it disagrees.

Backend:
  cd backend && go build ./...
  cd backend && go test ./internal/...

Web:
  cd web && npm run dev
  cd web && npm run typecheck   # tsc -p tsconfig.app.json --noEmit (bare `tsc --noEmit` is a no-op on the solution tsconfig)
  cd web && npx vitest run

Mobile:
  cd mobile && npm run typecheck   # tsc --noEmit
  cd mobile && npm test            # runs test:runners (coverage gate) + test:unit + test:jest
  cd mobile && npm run test:jest   # jest-expo: the React-Native-dependent suites

  Two runners, by design:
  - `test:unit` runs an explicit file list on the Node test runner (tsx --test).
    Use it for pure-logic tests that do NOT import react-native.
  - `test:jest` runs jest with the `jest-expo` preset. It covers the three
    suites that transitively import react-native and cannot be transformed
    outside Metro: `src/api/__tests__/signaling.test.ts`,
    `src/context/signaling/__tests__/useWebRTC.test.ts` and
    `src/services/translation/OnlineTranslationService.test.ts`. The scope is
    pinned via `jest.testMatch` in package.json. jest needs the
    `jest-shims/expo-virtual-env.js` shim, mapped from the Metro-only virtual
    module `expo/virtual/env` (babel-preset-expo rewrites `process.env.EXPO_PUBLIC_*`
    into a named import from it).
  Add new pure-logic tests to the `test:unit` list; add new RN-dependent tests
  under one of the jest `testMatch` globs. An unlisted test silently never runs.

  npm's normal peer resolution is intentionally kept enabled. Mobile hook tests
  use `@testing-library/react-native@13` (React 18-compatible), and
  `react-test-renderer` is pinned to `18.2.0` to match the app's React version.

Desktop:
  cd desktop && npm run dev

Root Makefile (`make verify` runs backend tests + `cd web && npm run typecheck` + mobile tsc +
Python pytest; the Python steps are skipped with a notice when the sibling
`allcallall-agent-runtime` checkout is absent). The web project uses a solution tsconfig, so a bare `tsc --noEmit` is a
no-op there — always use `cd web && npm run typecheck`):
  make fmt                 # gofmt -w on backend/
  make lint                # go vet + check-unbounded-find (backend) + npm run lint (web)
  make test                # backend + web + mobile test suites
  make test-backend        # cd backend && go test ./...
  make verify              # backend tests + web/mobile typecheck (+ python when the
                           #   sibling repo is present). Quick, but NOT complete:
                           #   no lint, no web/mobile test suites.
  make verify-full         # what "ready to merge" means: build + vet + the pagination
                           #   gate + every test suite + lint on all three clients +
                           #   python pytest. Fails if the sibling repo is missing
                           #   instead of silently skipping it.
  make web-contract-check  # web OpenAPI contract check

## Architecture boundaries

- Go backend (`backend/`) owns users, organizations, conversations, meetings,
  transcripts, permissions, approvals, audit logs, and write execution.
- Python runtime (`../allcallall-agent-runtime`) owns agent orchestration,
  LangGraph workflows, RAG, rerank, grounding, traces, citations, tool
  proposals, and evaluation.
- `contracts/` in this repo holds legacy fixtures only; authoritative schemas
  are generated and checked in the sibling repo via `make contracts-check`.

## Security defaults

- Production MUST use HTTPS; never serve the API over plain HTTP. Set
  `SECURITY_REQUIRE_TLS=true` so the API rejects plaintext `/api/v1` traffic.
  Health, readiness, status and metrics endpoints are registered on a route
  group that does **not** apply this middleware, because kubelet probes talk
  to the Pod IP without an `X-Forwarded-Proto` header.
- Message privacy policies (retention TTL, envelope encryption, recall, search
  minimization, erasure, moderation) are assembled in
  `backend/internal/runtime/privacy.go`. Any new process must call
  `ApplyPrivacyPolicies` so policy stays consistent across API and workers.
- All secrets and keys come from environment variables; never hardcode
  credentials or tokens. `configs/config.yaml` supports `${VAR}` (required,
  fails startup when unset) and `${VAR:-default}` (optional).
- Never commit `.env`, `.omo`, `.workbuddy`, or `output/`.

## Conventions

- Write clear, descriptive commit messages.
- CI's merge-blocking workflows are `ci.yml` and `platform-ci.yml`. Keep both
  green before merging. `ci.yml` runs backend, mobile, web (including the web
  `test:coverage` gate, coverage artifact upload and `npm audit`), the
  authenticated Beta Web E2E suite, and desktop checks. `platform-ci.yml` runs
  the Python agent/RAG runtimes, MySQL checkpoint idempotency contract, sandbox
  control plane, OpenAPI contract drift, Helm/Kubernetes schema validation, and
  image build+scan. The retired `frontend-ci.yml` and `backend-ci.yml` were
  merged into these to stop running suites twice on every push and PR.
- Push over SSH.
- Never commit `.env`, `.omo`, `.workbuddy`, or `output/`.

## Docs entry points

- `docs/README.md` — maintained documentation set.
- `INDEX.md` — cross-repo index covering this repo and `allcallall-agent-runtime`.
- `CLAUDE.md` and `docs/reference/AGENTS.md` — compatibility pointers kept for
  old links and tooling conventions. **This root file is the single source of
  truth**; anything the pointers say that conflicts with it is out of date.
