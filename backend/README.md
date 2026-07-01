# AllCallAll Backend

This is the Go backend for AllCallAll: realtime collaboration, meeting rooms, recording storage, recording transcription, AI Agent execution, search, and extractable worker/service boundaries.

## What This Backend Demonstrates

- Gin HTTP APIs with thin handlers and service-layer domain logic.
- Gorm/MySQL persistence with Redis-backed realtime, presence, cache, and rate-limit paths.
- JWT access tokens plus HttpOnly refresh-cookie sessions, rotation, reuse detection, and logout-all.
- Durable WebSocket collaboration replay through `chat_events`.
- WebRTC signaling, meeting room state, recording lifecycle, and local/S3-compatible recording storage.
- Recording-end transcription through `recording.transcription.requested`, `RecordingTranscription`, and `MeetingTranscriptSegment`.
- Agent runs with persisted steps, tool calls, memory, approvals, RAG context chunks, workflow/DAG execution, and Python runtime adapters.
- MySQL outbox with claim/lease/retry/idempotency semantics.
- Optional gRPC User Service, Kafka-compatible settlement pipeline, and Elasticsearch search/read model.

Realtime translation code still exists behind `/api/v1/translation/ws`, but its mobile UI entry points are currently hidden. Recording transcription does not depend on realtime translation.

## Entrypoints

```text
cmd/server          API/signaling server. Embedded workers enabled by default.
cmd/user-service    gRPC User Service for ValidateAccessToken/GetUser.
cmd/agent-worker    Standalone Agent and workflow worker.
cmd/outbox-worker   Collaboration, knowledge, transcription, and optional Kafka bridge worker.
cmd/data-worker     Kafka settlement consumer, writes room_settlements.
cmd/search-worker   Message indexing worker for Elasticsearch.
cmd/cleanup-worker  Refresh-session and recording-retention cleanup.
cmd/mcp-tool-server MCP-compatible stdio tool server for read-only Agent tools.
cmd/beta-seed       Idempotent small-team Beta demo data seed.
```

## Common Commands

```bash
# Run API from repo root
make run-api

# Run API from backend/
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/server

# Run extracted processes
make run-user-service
make run-agent-worker
make run-outbox-worker
make run-data-worker
make run-search-worker
make run-cleanup-worker

# Seed local Beta walkthrough data
make beta-seed

# Test and vet
cd backend && go test ./...
cd backend && go vet ./...
```

`make run-backend` remains as a compatibility alias for `cmd/server`.

## Config Loading

`config.Load()` reads `CONFIG_PATH` YAML, defaulting to `./configs/config.yaml` when the process runs from `backend/`, then applies supported environment overrides.

The API server, Agent Worker, and MCP tool server call `godotenv.Load()` as a local-development convenience. Other workers rely on the environment already being injected by the shell, supervisor, or Docker Compose. Treat `.env` as convenience, not as the single source of runtime configuration.

Core variables:

- `CONFIG_PATH`
- `DB_DSN`
- `REDIS_ADDR`, `REDIS_PASSWORD`
- `JWT_SECRET`
- `MAIL_PASSWORD`
- `WEBRTC_ICE_SERVERS_JSON`
- `AGENT_PROVIDER=rules|mock_llm|openai_compatible`
- `AGENT_PROVIDER_STRICT=true` for Beta/production; prevents a configured real provider from silently falling back to `rules`
- `RAG_RERANK_ENABLED=true`
- `RAG_RERANK_PROVIDER=rules|cross_encoder_compatible`
- `RAG_RERANK_BASE_URL`, `RAG_RERANK_MODEL`, `RAG_RERANK_TIMEOUT_SEC`
- `AGENT_RUNTIME=python_langgraph|legacy_go`; Compose/Beta demo defaults to `python_langgraph`, while bare Go processes without the env still use the legacy in-process runtime. `go` is accepted as a legacy alias. `python_langgraph` routes ReAct runs and supported workflows to the Python LangGraph service while keeping Go-owned data access, tool approval, audit, and write execution.
- `PY_AGENT_RUNTIME_BASE_URL=http://127.0.0.1:8090`
- `PY_RAG_RUNTIME_BASE_URL=http://127.0.0.1:8091`
- `PY_AGENT_RUNTIME_IMAGE`, `PY_RAG_RUNTIME_IMAGE`: Docker image overrides for the external runtime project. Compose defaults to `ghcr.io/xianingy/allcallall-agent-runtime/agent-runtime:v0.2.0` and `ghcr.io/xianingy/allcallall-agent-runtime/rag-runtime:v0.2.0`.
- `PY_AGENT_RUNTIME_TIMEOUT_SEC=60`
- `PY_AGENT_RUNTIME_STRICT=true`
- `AGENT_RUNTIME_TOOL_TOKEN`: bearer token required by the internal read-only tool bridge.
- `PY_AGENT_PROVIDER=rules|openai_compatible`
- `PY_AGENT_OPENAI_BASE_URL`, `PY_AGENT_OPENAI_MODEL`, `PY_AGENT_OPENAI_API_KEY`
- `PY_AGENT_PROMPT_VERSION`, `PY_AGENT_ENABLE_GROUNDING_CHECK`
- `PY_AGENT_ENABLE_AGENTIC_RAG=false`, `PY_AGENT_RAG_MAX_RETRIEVAL_STEPS=3`, `PY_AGENT_RAG_MIN_CONFIDENCE=0.6`
- `PY_AGENT_TOOL_BRIDGE_BASE_URL`, `PY_AGENT_TOOL_BRIDGE_TOKEN`
- `PY_RAG_TOOL_BRIDGE_BASE_URL`, `PY_RAG_TOOL_BRIDGE_TOKEN`
- `PY_RAG_RERANK_PROVIDER=rules|cross_encoder_compatible`, `PY_RAG_TOP_K`, `PY_RAG_MAX_STEPS`, `PY_RAG_MIN_CONFIDENCE`
- `PY_AGENT_HARNESS=allcallall_v1`, `PY_AGENT_LOOP_MAX_STEPS=5`, `PY_AGENT_ENABLE_CRITIC=true`
- `PY_RAG_VECTOR_STORE=none|qdrant`, `PY_RAG_QDRANT_URL`, `PY_RAG_QDRANT_COLLECTION`
- `EMBEDDED_WORKERS=0|1`

Recording and transcription:

- `RECORDING_STORAGE_DRIVER=local|s3`
- `RECORDING_STORAGE_DIR`
- `RECORDING_S3_BUCKET`, `RECORDING_S3_REGION`, `RECORDING_S3_ENDPOINT`
- `RECORDING_S3_ACCESS_KEY_ID`, `RECORDING_S3_SECRET_ACCESS_KEY`
- `RECORDING_S3_FORCE_PATH_STYLE=1`
- `RECORDING_PUBLIC_BASE_URL`
- `TRANSCRIPTION_ENABLED=true`
- `TRANSCRIPTION_PROVIDER=mock|openai_compatible`
- `TRANSCRIPTION_OPENAI_BASE_URL`
- `TRANSCRIPTION_OPENAI_API_KEY`
- `TRANSCRIPTION_OPENAI_MODEL`
- `TRANSCRIPTION_OPENAI_LANGUAGE`
- `TRANSCRIPTION_OPENAI_TIMEOUT_SEC`
- `TRANSCRIPTION_CHUNK_SECONDS`
- `TRANSCRIPTION_MAX_UPLOAD_BYTES`
- `TRANSCRIPTION_FFMPEG_PATH`

Infra extensions:

- `USER_GRPC_ADDR=:9090`
- `USER_SERVICE_GRPC_ADDR=localhost:9090`
- `KAFKA_BROKERS=localhost:9092`
- `KAFKA_SETTLEMENT_TOPIC=allcallall.room.settlements`
- `KAFKA_CONSUMER_GROUP=allcallall-data-worker`
- `ELASTICSEARCH_URL=http://localhost:9200`
- `ELASTICSEARCH_INDEX=allcallall_messages`
- `FCM_SERVICE_ACCOUNT_PATH=/path/firebase-service-account.json`

Beta provider rule: use `AGENT_RUNTIME=python_langgraph`, `PY_AGENT_PROVIDER=openai_compatible`, `PY_AGENT_PROVIDER_STRICT=true`, `TRANSCRIPTION_PROVIDER=openai_compatible`, and real ASR/LLM credentials for product validation. Use `rules`, `mock_llm`, `legacy_go`, and `TRANSCRIPTION_PROVIDER=mock` only for deterministic eval, local development, fallback, or seed-data demos.

## Important API Areas

- Health, readiness, and metrics: `GET /api/v1/health`, `GET /api/v1/ready`, `GET /api/v1/metrics`.
- Auth: `/api/v1/auth/register`, `/login`, `/refresh`, `/logout`, `/logout-all`, `/sessions`.
- Email verification: `/api/v1/email/send-verification-code`, `/api/v1/email/verify-code`.
- Users and contacts: `/api/v1/users/*`, `/api/v1/invitations/*`.
- Collaboration: `/api/v1/organizations/*`, `/conversations/*`, `/chat/ws`, `/search/messages`.
- Meetings: `/api/v1/rooms/*`, `/api/v1/webrtc/config`, `/api/v1/ws`, `/api/v1/signaling/*`.
- Recordings: `/api/v1/rooms/:roomId/recording/start`, `/stop`, `/api/v1/recordings/*`.
- Agent and workflows: `/api/v1/agent/runs`, `/events`, `/events/stream`, `/workflows`, `/approvals`.
- Knowledge base: `/api/v1/knowledge/sources`, `/source-groups`, `/duplicate-candidates`, `/dead-letters`.
- Support/commercial modules: legal, safety, follow-ups, entitlements, RevenueCat webhook, support diagnostics.

See [API Documentation](../docs/api/api-documentation.md) and [Interview API Surface](../docs/interview/api-surface.md).

## Verification

```bash
cd backend && go test ./...
cd backend && go vet ./...
```

For targeted work, prefer the affected package first, for example:

```bash
cd backend && go test ./internal/collaboration ./internal/agent ./internal/runtime ./internal/handlers
```

## Related Docs

- [System Design](../docs/interview/system-design.md)
- [Backend Deep Dive](../docs/interview/backend-deep-dive.md)
- [AI Agent Design](../docs/interview/ai-agent-design.md)
- [Worker Runtime](../docs/interview/worker-runtime.md)
- [Configuration](../docs/configuration/configuration.md)
- [Recording Storage Deployment](../docs/deployment/recording-storage-deployment.md)
