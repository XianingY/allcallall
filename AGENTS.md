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

  `.npmrc` sets `legacy-peer-deps=true`: `@testing-library/react-hooks@8`
  declares a `react@^16||^17` peer while the app is on React 18.2. This is the
  standard Expo 51 workaround and keeps both `npm ci` and local installs green.

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
- CI runs three workflows: `ci.yml`, `backend-ci.yml`,
  `platform-ci.yml`. Keep all three green before merging. Web and mobile
  checks (including the web `test:coverage` gate, coverage artifact upload
  and `npm audit`) live in `ci.yml`; the retired `frontend-ci.yml` was merged
  into it to stop running web/mobile twice on every push and PR.
- Push over SSH.
- Never commit `.env`, `.omo`, `.workbuddy`, or `output/`.

## Docs entry points

- `docs/README.md` — maintained documentation set.
- `INDEX.md` — cross-repo index covering this repo and `allcallall-agent-runtime`.
- `docs/reference/AGENTS.md` — compatibility pointer kept for old links; this
  root file is the single source of truth.
