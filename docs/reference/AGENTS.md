# AllCallAll Agent Guidelines

This file is for coding agents operating in this repo. Prefer small, verifiable changes; follow existing patterns; do not invent new conventions unless the repo is clearly missing one.

## Repo Map
- `backend/`: Go backend (Gin + Gorm/MySQL + Redis + Pion WebRTC)
- `mobile/`: React Native (Expo) app, TypeScript, hybrid signaling (WS + HTTP poll)
- `infra/`: docker-compose and deployment assets
- `scripts/`: dev scripts (DBs, translation models, etc.)

## Commands (Build/Lint/Test)

### Setup & Infra
- Project setup: `make setup` (git submodules + `cd mobile && npm install`)
- Download ML models: `make setup-models`
- Start MySQL/Redis: `./scripts/development/start-services.sh`
- Stop services: `docker-compose -f infra/docker-compose.yml down`
- Clean repo artifacts: `make clean` (removes `mobile/node_modules`, `mobile/.expo`, Android build outputs)
- Android-only clean: `make clean-android`
- Remove downloaded models: `make clean-models`

Infra notes:
- `infra/docker-compose.yml` is the local dev stack (MySQL/Redis/etc.).
- Prefer stopping via `docker-compose -f infra/docker-compose.yml down` over ad-hoc `docker rm`.

### Backend (Go)
- Run server: `make run-backend` (runs `cd backend && go run cmd/server/main.go`)
- Health check (when running locally): `curl http://localhost:8080/health`
- Build: `cd backend && go build ./...`
- Test all: `make test-backend` (or `cd backend && go test ./...`)
- Test a package: `cd backend && go test ./internal/auth`
- Run a single test: `cd backend && go test -v -run '^TestLogin$' ./internal/auth`
- Run tests without cache: `cd backend && go test -count=1 ./...`
- Run with race detector (slower): `cd backend && go test -race ./...`
- Static analysis: `cd backend && go vet ./...`
- Format: `cd backend && gofmt -s -w .`
- Tidy deps (only if imports changed): `cd backend && go mod tidy`

Notes:
- Backend config is typically via `CONFIG_PATH=./configs/config.yaml` when running locally.

### Mobile (React Native / Expo)
- Install deps: `cd mobile && npm install`
- Start Metro (default): `cd mobile && npm run start`
- Start Metro (dev client): `cd mobile && npm run start:dev-client`
- Start Metro (LAN): `cd mobile && npm run start:dev-client:lan`
- Start via Makefile: `make dev-android` / `make dev-ios`
- Recommended debug script (ADB reverse + cache + Metro): `cd mobile && bash scripts/dev-client-debug.sh`

Mobile native build/run:
- Build+install dev client (Android): `cd mobile && npm run android` (Expo prebuild + Gradle)
- Build+install dev client (iOS, macOS only): `cd mobile && npm run ios`

Device networking (common local dev):
- ADB reverse ports: `adb reverse tcp:8080 tcp:8080` and `adb reverse tcp:8081 tcp:8081`
- Verify device forwarding: `adb reverse --list`

Type checks / lint / tests:
- TypeScript: `cd mobile && npx tsc --noEmit`
- Lint: `cd mobile && npm run lint`
- Tests: this repo currently does not define a `test` script in `mobile/package.json`; if/when added, use `cd mobile && npm test`.
- Run Jest via npx (works even without a local dependency): `cd mobile && npx jest`
- Run a single Jest test: `cd mobile && npx jest -t "<TestName>"`
- Run a single Jest file: `cd mobile && npx jest src/path/to/file.test.tsx`

Mobile scripts (see `mobile/scripts/README.md`):
- Env sanity check: `cd mobile && bash scripts/verify-app-env.sh`
- Alarm/audio assets sanity check: `cd mobile && bash scripts/verify-alarm-setup.sh`

Native builds:
- Android debug APK: `make build-android`
- Android release APK: `make build-android-release`
- iOS build (macOS only): `make build-ios`

## Code Style Guidelines

### General
- Match surrounding style; do not do drive-by refactors.
- Do not commit secrets (`.env`, credentials, private keys).
- Prefer repo-local scripts/targets over inventing new ones.

### Go (backend)
- Layout: `cmd/` entrypoints, `internal/` modules; per-module files are typically `service.go`, `repository.go`, and optionally `handlers.go`.
- Layering: handlers (HTTP/WS) should be thin; business logic in services; DB access in repositories.
- Naming: exported `PascalCase`, unexported `camelCase`; constructors use `NewXxx`.
- Imports: 3 groups (stdlib / third-party / internal), separated by blank lines.
- Context: `context.Context` is the first param for service/repository methods.
- Errors: check `err != nil` explicitly; wrap with context using `%w`; prefer package-level `var ErrX = errors.New(...)` for domain errors.
- Logging: use `zerolog` via the repo logger; avoid `fmt.Println` in production code.
- Comments: many backend files use bilingual (Chinese then English) doc comments; preserve that style when editing those files.

Go security/ops:
- Do not log JWTs, email verification codes, or secrets.
- Keep DB transactions scoped; do not pass `*gorm.DB` deep into unrelated layers unless the module already does.

### TypeScript (mobile)
- TS is `strict`; avoid `any`.
- Types: prefer `interface` for object shapes; export types when used across modules.
- Components: functional + hooks; use `useCallback`/`useMemo` when passing callbacks or doing heavy work.
- Imports: React/RN first, then third-party, then internal (`~/` alias maps to `mobile/src/`).
- Errors: wrap async boundaries in `try/catch` and log with a stable prefix (e.g. `[PushNotificationService]`).

Mobile state/layout:
- Prefer colocating feature logic under `mobile/src/services/` and wiring via Context/providers under `mobile/src/context/`.
- Keep networking endpoints and feature flags centralized in `mobile/src/config/`.

Security / real-time rules:
- Secure storage: do not put tokens/keys in AsyncStorage; use Keychain (`react-native-keychain`) for sensitive data.
- E2EE: never log or persist session keys; treat DataChannel message formats as protocol.
- Signaling transport can be WS or polling; do not assume WS is always available.

## Repo-Specific Networking Notes
- WebSocket auth: backend accepts JWT via query `?token=...` (mobile connects as `${WS_URL}?token=...`).
- Mobile env flags (Expo `EXPO_PUBLIC_*`): see `docs/deployment/restricted-network-setup.md`.

Hybrid signaling (WS + poll):
- WebSocket endpoint: `GET /api/v1/ws` (JWT via `?token=`)
- Polling endpoints: `POST /api/v1/signaling/send`, `GET /api/v1/signaling/poll?timeout_ms=...`
- Mobile selection: `SIGNALING_TRANSPORT_MODE` in `mobile/src/config/index.ts`

Common mobile env flags:
- `EXPO_PUBLIC_API_HTTP`, `EXPO_PUBLIC_API_WS`: override API/WS base hosts
- `EXPO_PUBLIC_FORCE_TLS=1`: force `https://` + `wss://`
- `EXPO_PUBLIC_RESTRICTED_NETWORK=1`: prefer `turns:443?transport=tcp` in ICE ordering
- `EXPO_PUBLIC_SIGNALING_TRANSPORT=poll`: force HTTPS long-poll signaling (proxy-friendly)

Push-to-wake (FCM):
- Mobile: `mobile/src/services/PushNotificationService.ts`
- Backend: `backend/internal/fcm/manager.go` (triggered from signaling hub)

## Agent Workflow (Expectations)
- Discovery: use `glob`/`grep` first; avoid guessing file locations.
- Changes: keep diffs minimal; do not mix refactors with bug fixes.
- Verification: run the narrowest relevant check (Go: `go test` for the affected package; Mobile: `npx tsc --noEmit`).
- Git hygiene: do not revert unrelated local changes; do not commit unless explicitly requested.

## Security & Privacy Hard Rules
- Never add or print secrets in logs or comments.
- Mobile: do not store tokens/keys in AsyncStorage; use Keychain.
- E2EE: never persist session keys; never change message formats without updating both ends.

## Cursor / Copilot Rules
- No `.cursor/rules/`, `.cursorrules`, or `.github/copilot-instructions.md` found in this repo at the time of writing.
