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
  cd web && npx vitest run

Mobile:
  cd mobile && npx tsc --noEmit
  cd mobile && npm run test:unit

Desktop:
  cd desktop && npm run dev

Root Makefile (there is NO `make lint` or `make typecheck` target; use the
per-module commands above):
  make test-backend        # cd backend && go test ./...
  make verify              # backend tests + web/mobile tsc + python pytest
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
