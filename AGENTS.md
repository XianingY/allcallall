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

Backend:
  cd backend && go build ./...
  cd backend && go test ./internal/...

Web:
  cd web && npm run dev
  cd web && npm run typecheck   # tsc -p tsconfig.app.json --noEmit (bare `tsc --noEmit` is a no-op on the solution tsconfig)
  cd web && npx vitest run

Mobile:
  cd mobile && npm run typecheck   # tsc --noEmit
  cd mobile && npm test            # alias for npm run test:unit

  Test scope: `test:unit` runs an explicit file list on the Node test runner.
  Files that (transitively) import `react-native` cannot be transformed outside
  Metro, so they are intentionally excluded: `src/api/__tests__/`,
  `src/context/signaling/__tests__/` and
  `src/services/translation/OnlineTranslationService.test.ts`. They need a
  Metro/Jest preset to execute. Add new pure-logic tests to the `test:unit`
  list, otherwise they will silently never run.

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
  make verify              # backend tests + web/mobile typecheck + python pytest
  make web-contract-check  # web OpenAPI contract check

## Conventions

- Write clear, descriptive commit messages.
- CI runs four workflows: `ci.yml`, `backend-ci.yml`, `frontend-ci.yml`,
  `platform-ci.yml`. Keep all four green before merging.
- Push over SSH.
- Never commit `.env`, `.omo`, `.workbuddy`, or `output/`.

## Docs entry points

- `docs/README.md` — maintained documentation set.
- `INDEX.md` — cross-repo index covering this repo and `allcallall-agent-runtime`.
- `docs/reference/AGENTS.md` — compatibility pointer kept for old links; this
  root file is the single source of truth.
