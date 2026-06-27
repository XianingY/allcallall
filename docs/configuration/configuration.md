# Configuration

This page is the maintained runtime configuration reference. Backend config is `CONFIG_PATH` YAML plus environment overrides; mobile/Web config is Expo `EXPO_PUBLIC_*`; Docker Compose can inject both.

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

## Agent And Knowledge

| Variable | Purpose |
| --- | --- |
| `AGENT_PROVIDER` | `rules`, `mock_llm`, or `openai_compatible`; empty defaults to `rules`. |
| `AGENT_OPENAI_BASE_URL` | Chat Completions-compatible base URL. |
| `AGENT_OPENAI_MODEL` | Model name for `openai_compatible`. |
| `AGENT_OPENAI_API_KEY` | API key for `openai_compatible`. |
| `AGENT_OPENAI_TIMEOUT_MS` | Optional request timeout. |
| `AGENT_OPENAI_MAX_TOKENS` | Optional response token cap. |
| `ELASTICSEARCH_URL` | Enables Elasticsearch search and chunk indexing. |
| `ELASTICSEARCH_INDEX` | Message index name, default `allcallall_messages`. |
| `ELASTICSEARCH_USERNAME` | Optional basic auth username. |
| `ELASTICSEARCH_PASSWORD` | Optional basic auth password. |

## Worker Runtime

| Variable | Purpose |
| --- | --- |
| `EMBEDDED_WORKERS` | Default enabled. Set `0` to run API without embedded workers. |
| `WORKER_ID` | Stable worker identity for outbox claims. |
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

## Mobile / Web Variables

Only `EXPO_PUBLIC_*` is active for Expo mobile/Web runtime and build-time config.

| Variable | Purpose |
| --- | --- |
| `EXPO_PUBLIC_API_HTTP` | HTTP API base URL. |
| `EXPO_PUBLIC_API_WS` | WebSocket base URL. |
| `EXPO_PUBLIC_FORCE_TLS` | `1` forces `https` and `wss`. |
| `EXPO_PUBLIC_RESTRICTED_NETWORK` | Prefer restricted-network ICE/signaling behavior. |
| `EXPO_PUBLIC_SIGNALING_TRANSPORT` | `auto` or `poll`. |
| `EXPO_PUBLIC_SIGNALING_SHAPING` | Enables conservative signaling shaping. |
| `EXPO_PUBLIC_E2EE_MODE` | `experimental` enables experimental client E2EE mode. |
| `EXPO_PUBLIC_REVENUECAT_API_KEY` | Android subscription demo path. |
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
