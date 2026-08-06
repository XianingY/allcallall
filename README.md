# AllCallAll

AllCallAll is a full-stack engineering portfolio project for an **enterprise realtime collaboration and AI-assisted management system**. It is built around React/Vite, Go, Gin, Gorm/MySQL, Redis, WebSocket/WebRTC, recording storage, asynchronous workers, Elasticsearch search, and AI Agent workflows.

The strongest project story is not a commercial app launch. The repo demonstrates how an enterprise collaboration product can evolve from a modular monolith into a maintainable full-stack system with generated API contracts, an admin dashboard, realtime collaboration, performance gates, extracted worker/service boundaries, a primary Web app, native Expo mobile app, and Electron shell.

## Current Positioning

- **Realtime collaboration**: organizations, conversations, messages, notes, durable WebSocket replay, room state, WebRTC signaling, and recording sessions.
- **Enterprise admin console**: organization overview, member/invite/team/policy/audit management, admin summary API, Redis-backed dashboard cache, and reproducible dashboard/message-list benchmarks.
- **AI Agent system**: ReAct-style single-agent runs, workflow/DAG-style multi-agent tasks, tool calling, approvals, persisted traces, memory, and conversation-aware context retrieval. Beta/demo Compose defaults to a Python FastAPI + LangGraph Agent Runtime while Go remains the source of truth for data, permissions, approvals, audit, and writes.
- **Meeting recording transcription**: recording stop can enqueue `recording.transcription.requested`; the provider abstraction supports mock and OpenAI-compatible ASR paths and stores `MeetingTranscriptSegment` rows for Agent retrieval.
- **Hybrid retrieval**: conversation context plus external knowledge sources, BM25/vector search where Elasticsearch is configured, Go-layer reranking/chunk assembly, and an optional Python RAG Runtime for Agentic retrieval orchestration, rerank, evidence packs, and grounding checks.
- **Async reliability**: MySQL outbox, claim/lease workers, retries, idempotency keys, and Prometheus-style counters.
- **Extractable runtime**: API server can run embedded workers locally, or split into User Service, Agent Worker, Outbox Worker, Data Worker, Search Worker, and Cleanup Worker.
- **Primary Web surface**: `web/` is the main browser client, covering auth, organizations, collaboration, chat, calls, meetings, recordings, transcripts, Agent Lab, knowledge, approvals, billing settings, Web Push, and responsive layouts.
- **Supporting native/desktop surfaces**: `mobile/` remains the Expo Android/iOS client; `desktop/` is a thin Electron shell over the Web client.
- **Beta v1 scope**: the practical product loop is a 3-6 person team flow: create organization, invite members, chat, start a meeting, record, transcribe, generate an Agent meeting brief, inspect citations, and approve write-back tools.

Realtime translation code is still present for compatibility, but the mobile UI entry points are currently hidden. Meeting transcription is now independent of the realtime translation switch.

## Repository Map

```text
backend/     Go backend: API, auth, collaboration, Agent adapters, search, storage, workers
web/         Primary React + Vite + TypeScript Web application
mobile/      Expo React Native app for native Android/iOS
desktop/     Electron shell wrapping the Web client
infra/       Docker Compose local stack and optional interview infra profiles
scripts/     Development, smoke, seed, and benchmark scripts
docs/        Current docs, interview docs, deployment notes, and selected references
```

The Python Agent/RAG runtime source has been split into
[`XianingY/allcallall-agent-runtime`](https://github.com/XianingY/allcallall-agent-runtime).
AllCallAll integrates it over HTTP and Docker images; Go remains the source of truth for product data, permissions, approvals, audit, and write execution.

## Fast Start

Start MySQL and Redis:

```bash
./scripts/development/start-services.sh
```

Run the backend API:

```bash
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/server
```

Health check:

```bash
curl http://localhost:8080/api/v1/health
```

Run the Web app:

```bash
cd web
npm install
npm run dev
```

Vite serves the app at `http://localhost:5173` and proxies `/api` and WebSocket traffic to `http://127.0.0.1:8080`.

Run the desktop shell:

```bash
cd desktop
npm install
ALLCALLALL_WEB_URL=http://localhost:5173 npm run build
```

Android development still uses Expo/React Native and `EXPO_PUBLIC_*` configuration. `APP_ENV` is historical and is not an active mobile runtime control.

## Backend Commands

```bash
# API server, embedded workers enabled by default
make run-api

# Extracted service/process boundaries
make run-user-service
make run-agent-worker
make run-outbox-worker
make run-data-worker
make run-search-worker
make run-cleanup-worker
make run-agent-runtime
make run-rag-runtime

# Deterministic interview/demo harnesses
make beta-seed
make interview-demo
make interview-smoke
make interview-chaos
make interview-down
make interview-microservice-demo
make agent-eval
make rag-eval
make rerank-eval
make python-agent-eval
make python-rag-eval
make ai-agent-jd-eval
make ai-portfolio-eval
make dashboard-bench
make realtime-replay-bench
make chat-ws-replay-bench
make web-contract-check
make web-performance-check
```

`make run-backend` is retained as a compatibility alias for the API server.

## Runtime Configuration

The main API server, Agent Worker, and MCP tool server call `godotenv.Load()` as a development convenience, then load `CONFIG_PATH` YAML and environment overrides. In production or Docker Compose, inject variables explicitly.

Common backend variables:

- `CONFIG_PATH`: backend YAML config path.
- `DB_DSN`: MySQL DSN override.
- `REDIS_ADDR`, `REDIS_PASSWORD`: Redis connection.
- `JWT_SECRET`: JWT signing secret.
- `MAIL_PASSWORD`: SMTP credential override.
- `AGENT_PROVIDER=rules|mock_llm|openai_compatible`: Agent planner/provider selection.
- `AGENT_PROVIDER_STRICT=true`: required for Beta/production when using a real provider; disables silent fallback to `rules`.
- `RAG_RERANK_ENABLED=true`, `RAG_RERANK_PROVIDER=rules|cross_encoder_compatible`: optional post-retrieval rerank over BM25/vector/RRF candidates.
- `AGENT_RUNTIME=python_langgraph|legacy_go`: Agent orchestration runtime. Compose/Beta demo defaults to `python_langgraph`; bare Go processes without this env still use the legacy in-process runtime. `go` is accepted as a legacy alias.
- `PY_AGENT_RUNTIME_BASE_URL`: Python LangGraph runtime URL, defaulting to `http://127.0.0.1:8090` locally and `http://agent-runtime:8090` in Compose.
- `PY_RAG_RUNTIME_BASE_URL`: Python RAG Runtime URL, defaulting to `http://rag-runtime:8091` in Compose when Agentic RAG calls are enabled.
- `PY_AGENT_RUNTIME_IMAGE`, `PY_RAG_RUNTIME_IMAGE`: Docker image overrides. Compose defaults to `ghcr.io/xianingy/allcallall-agent-runtime/{agent-runtime,rag-runtime}:v0.2.0`.
- `PY_AGENT_RUNTIME_TIMEOUT_SEC`, `PY_AGENT_RUNTIME_STRICT`: timeout and strict failure behavior for the Python runtime.
- `AGENT_RUNTIME_TOOL_TOKEN`: shared token that protects the Go read-only tool bridge for Python runtime calls.
- `PY_AGENT_PROVIDER=rules|openai_compatible`: Python runtime provider selection.
- `PY_AGENT_OPENAI_BASE_URL`, `PY_AGENT_OPENAI_MODEL`, `PY_AGENT_OPENAI_API_KEY`: Python runtime OpenAI-compatible chat provider settings.
- `PY_AGENT_PROMPT_VERSION`, `PY_AGENT_ENABLE_GROUNDING_CHECK`: Python prompt-template and grounding-check controls.
- `PY_AGENT_ENABLE_AGENTIC_RAG=false`, `PY_AGENT_RAG_MAX_RETRIEVAL_STEPS=3`, `PY_AGENT_RAG_MIN_CONFIDENCE=0.6`: bounded Agentic RAG planning/refinement controls.
- `PY_AGENT_TOOL_BRIDGE_BASE_URL`, `PY_AGENT_TOOL_BRIDGE_TOKEN`: Python runtime access to Go-owned read-only tools.
- `PY_RAG_TOOL_BRIDGE_BASE_URL`, `PY_RAG_TOOL_BRIDGE_TOKEN`: Python RAG Runtime access to the Go internal authorized retrieval bridge.
- `PY_RAG_RERANK_PROVIDER=rules|cross_encoder_compatible`, `PY_RAG_TOP_K`, `PY_RAG_MAX_STEPS`, `PY_RAG_MIN_CONFIDENCE`: Python RAG Runtime retrieval and rerank controls.
- `PY_AGENT_HARNESS=allcallall_v1`, `PY_AGENT_LOOP_MAX_STEPS=5`, `PY_AGENT_ENABLE_CRITIC=true`: Python Agent Runtime Harness and bounded-loop controls.
- `PY_RAG_VECTOR_STORE=none|qdrant`, `PY_RAG_QDRANT_URL`, `PY_RAG_QDRANT_COLLECTION`: optional Qdrant adapter for vector/payload retrieval experiments; Go remains the production data boundary.
- `EMBEDDED_WORKERS=0|1`: controls API-process workers.
- `FCM_SERVICE_ACCOUNT_PATH`: enables Firebase Admin SDK push delivery.
- `RECORDING_STORAGE_DRIVER=local|s3`: recording storage driver.
- `RECORDING_STORAGE_DIR`: local recording storage root.
- `RECORDING_S3_*`: S3-compatible recording storage config.
- `TRANSCRIPTION_ENABLED=true`: enables recording transcription jobs.
- `TRANSCRIPTION_PROVIDER=mock|openai_compatible`: recording transcription provider.
- `TRANSCRIPTION_OPENAI_BASE_URL`, `TRANSCRIPTION_OPENAI_MODEL`, `TRANSCRIPTION_OPENAI_API_KEY`: OpenAI-compatible ASR settings for Beta.
- `USER_SERVICE_GRPC_ADDR`: enables API auth validation through User Service.
- `KAFKA_BROKERS`, `KAFKA_SETTLEMENT_TOPIC`: enables settlement event publishing/consumption.
- `ELASTICSEARCH_URL`, `ELASTICSEARCH_INDEX`: enables Elasticsearch message/search indexing.

Privacy / compliance variables (see [Privacy & Message Compliance](#privacy--message-compliance)):

- `MESSAGE_RETENTION_ENABLED`, `MESSAGE_RETENTION_TEXT_TTL_HOURS` (default `72`), `MESSAGE_RETENTION_MEDIA_TTL_HOURS` (default `120`), `MESSAGE_RETENTION_PURGE_SYSTEM`, `MESSAGE_RETENTION_CLEANUP_INTERVAL_MIN` (default `30`), `MESSAGE_RETENTION_CLEANUP_BATCH_LIMIT` (default `500`).
- `MESSAGE_ENCRYPTION_ENABLED`, `MESSAGE_ENCRYPTION_MASTER_KEY` (base64; **required** when encryption is enabled — startup fails otherwise), `MESSAGE_ENCRYPTION_KEY_ID`.
- `MESSAGE_RECALL_ENABLED`, `MESSAGE_RECALL_WINDOW_MINUTES`, `MESSAGE_RECALL_ALLOW_ADMIN_OVERRIDE`.
- `SEARCH_INDEX_ENABLED`, `SEARCH_INDEX_BODY_SNIPPET_MAX_RUNES` (default `64`).
- `CONTENT_MODERATION_ENABLED`, `CONTENT_MODERATION_KEYWORDS` (comma-separated).
- `SECURITY_REQUIRE_TLS`: rejects plaintext `/api/v1` traffic with `403 HTTPS_REQUIRED`; trusts `X-Forwarded-Proto: https` behind a terminating proxy.
- `AUDIT_LOG_RETENTION_DAYS` (default `180`): retention floor for organization audit events.

Web runtime public variables are injected through `/config.js` in production:

- `PUBLIC_API_BASE_URL`
- `PUBLIC_WS_BASE_URL`
- `FIREBASE_API_KEY`, `FIREBASE_AUTH_DOMAIN`, `FIREBASE_PROJECT_ID`, `FIREBASE_STORAGE_BUCKET`, `FIREBASE_MESSAGING_SENDER_ID`, `FIREBASE_APP_ID`, `FIREBASE_VAPID_KEY`
- `REVENUECAT_PUBLIC_API_KEY`

Mobile native public variables:

- `EXPO_PUBLIC_API_HTTP`
- `EXPO_PUBLIC_API_WS`
- `EXPO_PUBLIC_FORCE_TLS`
- `EXPO_PUBLIC_RESTRICTED_NETWORK`
- `EXPO_PUBLIC_SIGNALING_TRANSPORT`
- `EXPO_PUBLIC_SIGNALING_SHAPING`
- `EXPO_PUBLIC_E2EE_MODE=experimental`
- `EXPO_PUBLIC_REVENUECAT_API_KEY` and related RevenueCat variables for Android subscription paths.

## Optional Microservice / Infra Demo

Run local infrastructure for the interview-oriented extracted topology:

```bash
docker compose -f infra/docker-compose.yml \
  --profile microservices \
  --profile interview-infra \
  up api user-service outbox-worker data-worker search-worker kafka elasticsearch
```

Important variables for that topology:

```bash
USER_GRPC_ADDR=:9090
USER_SERVICE_GRPC_ADDR=user-service:9090
KAFKA_BROKERS=kafka:9092
KAFKA_SETTLEMENT_TOPIC=allcallall.room.settlements
ELASTICSEARCH_URL=http://elasticsearch:9200
ELASTICSEARCH_INDEX=allcallall_messages
EMBEDDED_WORKERS=0
```

## Beta Demo Seed

For local Beta walkthroughs, seed a small team workspace after MySQL is running:

```bash
make beta-seed
```

The command creates deterministic owner/member accounts, an organization, a team, a conversation, sample messages, a completed meeting, ready transcript segments, and audit events. It prints login credentials and direct Web routes. The seeded transcript is metadata and text-only; it is useful for UI and Agent grounding demos, not for real ASR/download validation.

## Privacy & Message Compliance

Message handling follows a WeChat-style "transit, not archive" model, aligned with PIPL obligations. All policies are assembled in one place (`runtime.ApplyPrivacyPolicies`) so the API server and every worker process share identical behavior. Everything is config-gated and off by default unless noted.

| Capability | Where | Behavior |
| --- | --- | --- |
| Retention TTL | `internal/collaboration/message_retention.go` | Text bodies are physically cleared after 72h, media after 120h. Message skeletons (ids, timestamps, sender) survive so threads stay coherent. System and call-event messages are exempt by default. |
| Envelope encryption | `internal/messagecrypto` | Each message gets a random AES-256-GCM DEK, wrapped by the master key and stored alongside the ciphertext. Fail-closed: a missing or invalid master key aborts startup rather than silently writing plaintext. |
| Recall | `internal/collaboration/message_recall.go` | `POST /api/v1/conversations/:id/messages/:messageId/recall`. Destroys body, metadata, envelope, attachment objects, and the search copy; keeps the skeleton. Idempotent. Admins can force-recall past the window for takedown requests. |
| Search index minimization | `internal/collaboration/search_policy.go` | Search documents carry a rune-truncated snippet plus a `body_length` signal instead of the full body, so an index service outside the trust boundary never holds complete content. |
| Right to erasure | `internal/collaboration/message_erasure.go` | `POST /api/v1/organizations/:id/users/:userId/messages/erase` (self, or owner/admin for anyone) and `POST /api/v1/organizations/:id/messages/erase` (owner/admin only). Batched, idempotent, marks `erased_at`/`erased_by`. |
| Content moderation | `internal/collaboration/message_moderation.go` | Pluggable `ModerationService` invoked asynchronously after publish, so moderation never blocks delivery. Hits write an audit event and broadcast a flag. A keyword implementation ships by default. |
| Transport TLS | `internal/server/middleware.go` | `RequireTLS` rejects plaintext `/api/v1` traffic. |
| Audit retention | `internal/collaboration/audit_retention.go` | A worker purges organization audit events older than `AUDIT_LOG_RETENTION_DAYS` in batches. |
| Identity verification | `models.User.IdentityVerified` | When an organization sets `RequireIdentityVerification`, unverified users are rejected at invite acceptance with `ErrIdentityVerificationRequired`. |

Related migrations: `000009_message_retention` through `000013_identity_binding`. Bump `currentSchemaVersion` in `internal/runtime/migrations.go` and the assertion in `migrations_test.go` whenever a migration is added.

## Documentation

Start with:

- [Cross-Repo Master Index](INDEX.md) — covers this repo and `allcallall-agent-runtime`
- [Documentation Index](docs/README.md)
- [FAQ](docs/faq.md)
- [Backend README](backend/README.md)
- [Quick Start](docs/getting-started/quick-start.md)
- [Configuration](docs/configuration/configuration.md)
- [Interview README](docs/interview/README.md)
- [Interview Chain](docs/interview/interview-chain.md)
- [Five-Minute Demo Script](docs/interview/demo-script.md)
- [Tencent Interview Questions](docs/interview/tencent-interview-questions.md)
- [Tencent Full-Stack JD Fit](docs/interview/tencent-fullstack-jd-fit.md)
- [System Design](docs/interview/system-design.md)
- [AI Agent Design](docs/interview/ai-agent-design.md)
- [API Surface](docs/interview/api-surface.md)

## Quality Gates

Backend:

```bash
cd backend && go test ./... && go vet ./...
```

Web:

```bash
cd web && npm run typecheck && npm run lint && npm test && npm run build
cd web && npm run contract:check && npm run bundle:budget
```

Mobile:

```bash
cd mobile && npx tsc --noEmit && npm run lint
```

Desktop:

```bash
cd desktop && npm run check && npm run build
```

Use the narrowest relevant check for small changes, then broaden when shared behavior or generated surfaces are touched.

## Code Style & Formatting

The codebase follows language-native formatting, and CI expects these to be clean before merge.

- **Go (backend/)**: run `gofmt -w .` to format, then `go vet ./...` and `go test ./...`. The repo does not require `protoc`; gRPC bindings are hand-written from `backend/proto/user/v1/user.proto`.
- **Web (web/)**: `npm run lint` (ESLint) and `npm run typecheck` (`tsc --noEmit`) are the style/type gates, and `npm test` runs the Vitest suite.
- **Mobile (mobile/)** and **Desktop (desktop/)**: `npx tsc --noEmit` plus `npm run lint` / `npm run check`.

Convenience wrappers live in the Makefile:

```bash
make fmt    # gofmt -w on backend/
make lint   # go vet (backend) + npm run lint (web)
```

## Scope Boundaries

- Kubernetes is intentionally not part of the current implementation.
- Transcript editing, large-meeting SFU scale-out, and Kubernetes are future enhancements.
- Kafka and Elasticsearch adapters exist, but live smoke tests require the optional Compose profiles.
- Production Web billing and Web Push require RevenueCat/Firebase public runtime config plus backend FCM service-account config.
- The `.proto` file is the source contract; current Go gRPC binding is hand-written because `protoc` is not required by CI.

## License

This repository is used as a personal engineering portfolio project. Confirm licensing requirements before commercial reuse.
