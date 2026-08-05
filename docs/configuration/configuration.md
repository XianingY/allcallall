# Configuration

This page is the maintained runtime configuration reference. Backend config is `CONFIG_PATH` YAML plus environment overrides; Web config is runtime `/config.js`; mobile native config uses Expo `EXPO_PUBLIC_*`; Docker Compose can inject all of them.

## Backend Config Loading

`backend/internal/config.Load()` reads:

1. `CONFIG_PATH`, defaulting to `./configs/config.yaml` when the process runs from `backend/`.
2. YAML sections for server, database, Redis, mail, JWT, WebRTC, realtime translation, and logging.
3. Supported environment overrides and runtime-only variables.

The API server, Agent Worker, and MCP tool server call `godotenv.Load()` for local development. Other backend entrypoints do not. Prefer explicit exported variables or Compose env injection for repeatable runs.

## YAML Sections

```yaml
server:
  host: "0.0.0.0"
  port: 8080

database:
  dsn: "allcallall:password@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local"

redis:
  addr: "localhost:6379"
  password: ""

jwt:
  secret: "change-me"
  issuer: "allcallall-backend"

webrtc:
  ice_servers:
    - urls: ["stun:stun.l.google.com:19302"]

translation:
  enabled: false
  provider: "volc_ast"

logging:
  level: "info"
```

Realtime translation remains configurable in YAML, but its mobile UI entry points are currently hidden. Meeting recording transcription is configured separately through `TRANSCRIPTION_*`.

## Core Backend Variables

| Variable | Purpose |
| --- | --- |
| `CONFIG_PATH` | YAML config path. |
| `DB_DSN` | MySQL DSN override. |
| `REDIS_ADDR` | Redis address override. |
| `REDIS_PASSWORD` | Redis password override. |
| `JWT_SECRET` | JWT signing secret override. |
| `MAIL_PASSWORD` | SMTP password override. |
| `WEBRTC_ICE_SERVERS_JSON` | JSON ICE/TURN server list override. |
| `CORS_ALLOWED_ORIGINS` | Comma-separated browser origins. |
| `PUBLIC_WEB_BASE_URL` | Public base URL for legal/account links. |
| `SUPPORT_EMAIL` | Public support contact. |
| `SUPPORT_INTERNAL_ONLY` | Restricts support APIs to private/loopback clients. Production Compose sets this to `true`. |

## Agent And Knowledge

| Variable | Purpose |
| --- | --- |
| `AGENT_PROVIDER` | `rules`, `mock_llm`, or `openai_compatible`; empty defaults to `rules`. |
| `AGENT_PROVIDER_STRICT` | When true, `openai_compatible` must be configured and provider failures are returned instead of falling back to `rules`. Enable this in Beta/production. |
| `AGENT_OPENAI_BASE_URL` | Chat Completions-compatible base URL. |
| `AGENT_OPENAI_MODEL` | Model name for `openai_compatible`. |
| `AGENT_OPENAI_API_KEY` | API key for `openai_compatible`. |
| `AGENT_OPENAI_TIMEOUT_MS` | Optional request timeout. |
| `AGENT_OPENAI_MAX_TOKENS` | Optional response token cap. |
| `RAG_RERANK_ENABLED` | Enables explicit RAG rerank after BM25/vector/RRF retrieval. Defaults to disabled. |
| `RAG_RERANK_PROVIDER` | `rules` for deterministic local rerank or `cross_encoder_compatible` for an HTTP rerank service. |
| `RAG_RERANK_BASE_URL`, `RAG_RERANK_MODEL`, `RAG_RERANK_TIMEOUT_SEC` | HTTP rerank provider settings. The compatible endpoint is called at `/rerank`. |
| `AGENT_RUNTIME` | `go` or `python_langgraph`; defaults to `go`. `python_langgraph` supports `meeting_brief`, `risk_review`, `follow_up_planner`, and `context_qa`. |
| `PY_AGENT_RUNTIME_BASE_URL` | Python Agent Runtime base URL, for example `http://127.0.0.1:8090` locally or `http://agent-runtime:8090` in Compose. |
| `PY_AGENT_RUNTIME_TIMEOUT_SEC` | Python runtime HTTP timeout; defaults to `60`. |
| `PY_AGENT_RUNTIME_STRICT` | When true, Python runtime failures fail the workflow instead of silently falling back to Go. |
| `PY_AGENT_PROVIDER` | Python runtime provider; deterministic `rules` is the default. |
| `PY_AGENT_OPENAI_BASE_URL`, `PY_AGENT_OPENAI_MODEL`, `PY_AGENT_OPENAI_API_KEY` | OpenAI-compatible `/chat/completions` access from the Python runtime. |
| `PY_AGENT_PROMPT_VERSION` | Optional prompt-template override. Empty uses preset defaults such as `meeting_brief_v1`. |
| `PY_AGENT_ENABLE_GROUNDING_CHECK` | Enables Python-side citation grounding checks in trace output; defaults to true. |
| `AGENT_RUNTIME_TOOL_TOKEN` | Go-side bearer token for the internal read-only tool bridge. Leave empty to disable the bridge. |
| `PY_AGENT_TOOL_BRIDGE_BASE_URL`, `PY_AGENT_TOOL_BRIDGE_TOKEN` | Python-side Go tool bridge URL and token. When unset, Python uses only preloaded context. |
| `ROOM_MAX_PARTICIPANTS` | Maximum active/invited members in a meeting room; defaults to `6`. |
| `ROOM_TRICKLE_ICE` | Enables trickle ICE for meeting rooms; defaults to `false`. When enabled, `POST /rooms/:roomId/offer` answers as soon as the local description is set (the response carries `trickle_ice: true`) and the remaining server-side candidates are delivered as `room.ice.candidate` realtime events. Only turn it on once every client applies those events. |
| `ELASTICSEARCH_URL` | Enables Elasticsearch search and chunk indexing. |
| `ELASTICSEARCH_INDEX` | Message index name, default `allcallall_messages`. |
| `ELASTICSEARCH_USERNAME` | Optional basic auth username. |
| `ELASTICSEARCH_PASSWORD` | Optional basic auth password. |

## Worker Runtime

| Variable | Purpose |
| --- | --- |
| `EMBEDDED_WORKERS` | Default enabled. Set `0` to run API without embedded workers. |
| `WORKER_ID` | Stable worker identity for outbox claims. |

## Database Lifecycle

| Variable | Purpose |
| --- | --- |
| `APP_ENV` | `production` and `beta` disable automatic schema migration by default. |
| `DB_AUTO_MIGRATE` | Explicitly enables/disables startup migration. Keep `0` in Beta/production and run `/app/migrate` first. |

Authentication endpoints use Redis-backed IP and hashed-account rate limits when Redis is configured. Login allows 10 attempts per account per 15 minutes; registration and verification-send use stricter hourly limits.
| `OUTBOX_WORKER_INTERVAL_SEC` | Outbox polling interval. |
| `OUTBOX_WORKER_BATCH_SIZE` | Outbox claim batch size. |
| `OUTBOX_WORKER_MAX_ATTEMPTS` | Retry budget before permanent failure. |
| `OUTBOX_WORKER_RETRY_DELAY_SEC` | Retry delay. |
| `OUTBOX_WORKER_LEASE_SEC` | Claim lease duration. |
| `REFRESH_SESSION_CLEANUP_INTERVAL_MIN` | Refresh-session cleanup interval. |
| `REFRESH_SESSION_REVOKED_RETENTION_DAYS` | Revoked-session retention. |
| `RECORDING_CLEANUP_INTERVAL_MIN` | Recording cleanup interval. |

## Recording Storage And Transcription

| Variable | Purpose |
| --- | --- |
| `RECORDING_STORAGE_DRIVER` | `local` or `s3`. |
| `RECORDING_STORAGE_DIR` | Local storage directory. |
| `RECORDING_S3_BUCKET` | S3-compatible bucket. |
| `RECORDING_S3_REGION` | S3 region. |
| `RECORDING_S3_ENDPOINT` | S3-compatible endpoint. |
| `RECORDING_S3_ACCESS_KEY_ID` | Access key. |
| `RECORDING_S3_SECRET_ACCESS_KEY` | Secret key. |
| `RECORDING_S3_FORCE_PATH_STYLE` | `1` for MinIO/path-style services. |
| `RECORDING_PUBLIC_BASE_URL` | Optional public base URL for object serving. |
| `TRANSCRIPTION_ENABLED` | `1`, `true`, or `yes` enables recording-end transcription. |
| `TRANSCRIPTION_PROVIDER` | `mock` or `openai_compatible`; empty defaults to `mock` when transcription is enabled. |
| `TRANSCRIPTION_OPENAI_BASE_URL` | Compatible API base URL; the worker calls `/audio/transcriptions`. |
| `TRANSCRIPTION_OPENAI_API_KEY` | Optional bearer token for the compatible endpoint. |
| `TRANSCRIPTION_OPENAI_MODEL` | Required model name for `openai_compatible`. |
| `TRANSCRIPTION_OPENAI_LANGUAGE` | Optional source-language hint. |
| `TRANSCRIPTION_OPENAI_TIMEOUT_SEC` | Per-request timeout, default 120 seconds. |
| `TRANSCRIPTION_CHUNK_SECONDS` | FFmpeg chunk duration, default 600 seconds. |
| `TRANSCRIPTION_MAX_UPLOAD_BYTES` | Maximum API upload per chunk, default 24 MiB. |
| `TRANSCRIPTION_FFMPEG_PATH` | FFmpeg executable, default `ffmpeg`. |

Long recordings are split into OGG chunks before upload. Local and S3-compatible recording storage are both supported; S3 objects are materialized into a bounded temporary file and removed after processing.

## gRPC, Kafka, FCM

| Variable | Purpose |
| --- | --- |
| `USER_GRPC_ADDR` | `cmd/user-service` listen address, default `:9090`. |
| `USER_SERVICE_GRPC_ADDR` | API auth middleware uses remote User Service when set. |
| `KAFKA_BROKERS` | Comma-separated broker list. |
| `KAFKA_SETTLEMENT_TOPIC` | Settlement topic, default `allcallall.room.settlements`. |
| `KAFKA_CONSUMER_GROUP` | Data worker group, default `allcallall-data-worker`. |
| `FCM_SERVICE_ACCOUNT_PATH` | Enables Firebase Admin SDK push delivery. |
| `REVENUECAT_WEBHOOK_AUTH_TOKEN` | Protects the RevenueCat webhook path. |
| `SUPPORT_API_TOKEN` | Protects internal support endpoints. |

## Web Runtime Variables

The production Web container generates `/config.js` from public environment variables in `infra/web-runtime-config.sh`.

| Variable | Purpose |
| --- | --- |
| `PUBLIC_API_BASE_URL` | Browser REST API base URL; defaults to `/api/v1`. |
| `PUBLIC_WS_BASE_URL` | Browser WebSocket API base URL; empty uses current host and protocol. |
| `FIREBASE_API_KEY` | Firebase Web app public API key. |
| `FIREBASE_AUTH_DOMAIN` | Firebase Web auth domain. |
| `FIREBASE_PROJECT_ID` | Firebase project ID. |
| `FIREBASE_STORAGE_BUCKET` | Optional Firebase storage bucket. |
| `FIREBASE_MESSAGING_SENDER_ID` | Firebase messaging sender ID. |
| `FIREBASE_APP_ID` | Firebase Web app ID. |
| `FIREBASE_VAPID_KEY` | Web Push VAPID public key. |
| `REVENUECAT_PUBLIC_API_KEY` | RevenueCat Web Billing public API key. |

Local Vite Web uses `http://localhost:5173` and proxies `/api` to `http://127.0.0.1:8080`.

## Mobile Native Variables

Only `EXPO_PUBLIC_*` is active for Expo native runtime and build-time config.

| Variable | Purpose |
| --- | --- |
| `EXPO_PUBLIC_API_HTTP` | HTTP API base URL. |
| `EXPO_PUBLIC_API_WS` | WebSocket base URL. |
| `EXPO_PUBLIC_FORCE_TLS` | `1` forces `https` and `wss`. |
| `EXPO_PUBLIC_RESTRICTED_NETWORK` | Prefer restricted-network ICE/signaling behavior. |
| `EXPO_PUBLIC_SIGNALING_TRANSPORT` | `auto` or `poll`. |
| `EXPO_PUBLIC_SIGNALING_SHAPING` | Enables conservative signaling shaping. |
| `EXPO_PUBLIC_E2EE_MODE` | `experimental` enables experimental client E2EE mode. |
| `EXPO_PUBLIC_REVENUECAT_API_KEY` | Android subscription path. |
| `EXPO_PUBLIC_REVENUECAT_OFFERING_ID` | RevenueCat offering. |
| `EXPO_PUBLIC_REVENUECAT_MONTHLY_PRODUCT_ID` | Defaults around `premium_monthly`. |
| `EXPO_PUBLIC_REVENUECAT_YEARLY_PRODUCT_ID` | Defaults around `premium_yearly`. |

`APP_ENV`, `DEV_API`, and `PROD_API_IP` are historical names and must not be documented as active runtime controls.

## Compose Profiles

Default local stack:

```bash
docker compose -f infra/docker-compose.yml up -d mysql redis
```

Interview topology:

```bash
docker compose -f infra/docker-compose.yml \
  --profile microservices \
  --profile interview-infra \
  up api user-service outbox-worker data-worker search-worker kafka elasticsearch
```

## Security Notes

- Never commit `.env`, service account JSON, JWT secrets, SMTP passwords, S3 credentials, or API keys.
- Do not log JWTs, refresh tokens, verification codes, Firebase tokens, session keys, or object-storage credentials.
- Keep authorization in service-layer code even when indexes, workers, or external storage are introduced.
