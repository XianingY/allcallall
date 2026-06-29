# AllCallAll

AllCallAll is a backend engineering portfolio project for an **AI-powered realtime collaboration system**. It is built around Go, Gin, Gorm/MySQL, Redis, WebSocket/WebRTC, recording storage, asynchronous workers, Elasticsearch search, and AI Agent workflows.

The strongest project story is not a commercial app launch. The repo demonstrates how a realtime collaboration backend can evolve from a modular monolith into extracted worker/service boundaries while keeping a primary Web app, native Expo mobile app, and Electron shell available.

## Current Positioning

- **Realtime collaboration**: organizations, conversations, messages, notes, durable WebSocket replay, room state, WebRTC signaling, and recording sessions.
- **AI Agent system**: ReAct-style single-agent runs, workflow/DAG-style multi-agent tasks, tool calling, approvals, persisted traces, memory, and conversation-aware context retrieval. Workflow presets can optionally run in a Python FastAPI + LangGraph runtime while Go remains the source of truth for data, permissions, approvals, audit, and writes.
- **Meeting recording transcription**: recording stop can enqueue `recording.transcription.requested`; the provider abstraction supports mock and OpenAI-compatible ASR paths and stores `MeetingTranscriptSegment` rows for Agent retrieval.
- **Hybrid retrieval**: conversation context plus external knowledge sources, BM25/vector search where Elasticsearch is configured, and Go-layer reranking/chunk assembly.
- **Async reliability**: MySQL outbox, claim/lease workers, retries, idempotency keys, and Prometheus-style counters.
- **Extractable runtime**: API server can run embedded workers locally, or split into User Service, Agent Worker, Outbox Worker, Data Worker, Search Worker, and Cleanup Worker.
- **Primary Web surface**: `web/` is the main browser client, covering auth, organizations, collaboration, chat, calls, meetings, recordings, transcripts, Agent Lab, knowledge, approvals, billing settings, Web Push, and responsive layouts.
- **Supporting native/desktop surfaces**: `mobile/` remains the Expo Android/iOS client; `desktop/` is a thin Electron shell over the Web client.
- **Beta v1 scope**: the practical product loop is a 3-6 person team flow: create organization, invite members, chat, start a meeting, record, transcribe, generate an Agent meeting brief, inspect citations, and approve write-back tools.

Realtime translation code is still present for compatibility, but the mobile UI entry points are currently hidden. Meeting transcription is now independent of the realtime translation switch.

## Repository Map

```text
backend/     Go backend: API, auth, collaboration, Agent, search, storage, workers
agent-runtime/ Python FastAPI + LangGraph runtime, provider adapters, tool bridge client, and evals
web/         Primary React + Vite + TypeScript Web application
mobile/      Expo React Native app for native Android/iOS
desktop/     Electron shell wrapping the Web client
infra/       Docker Compose local stack and optional interview infra profiles
scripts/     Development, smoke, seed, and benchmark scripts
docs/        Current docs, interview docs, deployment notes, and selected references
```

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

# Deterministic interview/demo harnesses
make beta-seed
make interview-demo
make interview-microservice-demo
make agent-eval
make rag-eval
make rerank-eval
make python-agent-eval
make ai-portfolio-eval
make realtime-replay-bench
make chat-ws-replay-bench
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
- `AGENT_RUNTIME=go|python_langgraph`: workflow orchestration runtime. Default is `go`; `python_langgraph` supports `meeting_brief`, `risk_review`, `follow_up_planner`, and `context_qa`.
- `PY_AGENT_RUNTIME_BASE_URL`: Python LangGraph runtime URL, defaulting to `http://127.0.0.1:8090` locally and `http://agent-runtime:8090` in Compose.
- `PY_AGENT_RUNTIME_TIMEOUT_SEC`, `PY_AGENT_RUNTIME_STRICT`: timeout and strict failure behavior for the Python runtime.
- `AGENT_RUNTIME_TOOL_TOKEN`: shared token that protects the Go read-only tool bridge for Python runtime calls.
- `PY_AGENT_PROVIDER=rules|openai_compatible`: Python runtime provider selection.
- `PY_AGENT_OPENAI_BASE_URL`, `PY_AGENT_OPENAI_MODEL`, `PY_AGENT_OPENAI_API_KEY`: Python runtime OpenAI-compatible chat provider settings.
- `PY_AGENT_PROMPT_VERSION`, `PY_AGENT_ENABLE_GROUNDING_CHECK`: Python prompt-template and grounding-check controls.
- `PY_AGENT_ENABLE_AGENTIC_RAG=false`, `PY_AGENT_RAG_MAX_RETRIEVAL_STEPS=3`, `PY_AGENT_RAG_MIN_CONFIDENCE=0.6`: bounded Agentic RAG planning/refinement controls.
- `PY_AGENT_TOOL_BRIDGE_BASE_URL`, `PY_AGENT_TOOL_BRIDGE_TOKEN`: Python runtime access to Go-owned read-only tools.
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

## Documentation

Start with:

- [Documentation Index](docs/README.md)
- [Backend README](backend/README.md)
- [Quick Start](docs/getting-started/quick-start.md)
- [Configuration](docs/configuration/configuration.md)
- [Interview README](docs/interview/README.md)
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

## Scope Boundaries

- Kubernetes is intentionally not part of the current implementation.
- Transcript editing, large-meeting SFU scale-out, and Kubernetes are future enhancements.
- Kafka and Elasticsearch adapters exist, but live smoke tests require the optional Compose profiles.
- Production Web billing and Web Push require RevenueCat/Firebase public runtime config plus backend FCM service-account config.
- The `.proto` file is the source contract; current Go gRPC binding is hand-written because `protoc` is not required by CI.

## License

This repository is used as a personal engineering portfolio project. Confirm licensing requirements before commercial reuse.
