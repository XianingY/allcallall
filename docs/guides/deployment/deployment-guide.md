# Deployment Guide

企业 Agent 平台的 Kubernetes、gVisor、NetworkPolicy、外部 MySQL/Redis/OpenBao 和 feature flag 发布流程见 [Agent 平台 Kubernetes 部署](./agent-platform-kubernetes.md)。

This guide describes the local and staging deployment shapes. Production Agent platform deployments use the Kubernetes chart and operational controls documented above.

## Supported Topologies

### 1. Local API With Embedded Workers

```bash
./scripts/development/start-services.sh

cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/server
```

Use this for normal local development. The API process starts embedded workers by default.

### 2. API Plus Standalone Workers

```bash
EMBEDDED_WORKERS=0 make run-api
make run-agent-worker
make run-outbox-worker
make run-cleanup-worker
```

Use this to show the modular-monolith-to-worker split without requiring Kafka or Elasticsearch.

### 3. Interview Infra Profile

```bash
docker compose -f infra/docker-compose.yml \
  --profile microservices \
  --profile interview-infra \
  up api user-service outbox-worker data-worker search-worker kafka elasticsearch
```

This topology demonstrates gRPC auth validation, Kafka-compatible settlement, Elasticsearch search, and standalone workers. Kubernetes is intentionally not implemented.

### 4. Single-node Beta Stack

The maintained Beta shape serves the independent `web/dist` bundle and API through TLS Nginx, runs a one-shot schema migration, persists MySQL/Redis/recordings, and exposes Coturn:

```bash
docker compose --env-file .env -f infra/docker-compose.production.yml config
docker compose --env-file .env -f infra/docker-compose.production.yml up -d --build
curl --cacert infra/ssl/fullchain.pem https://localhost/api/v1/ready
```

Provide `infra/ssl/fullchain.pem` and `infra/ssl/privkey.pem`. Set `TURN_HOST`, `TURN_REALM`, `TURN_USERNAME`, and `TURN_PASSWORD` to public values reachable by clients. This topology is intentionally limited to one media node and rooms of at most six participants.

## Runtime Components

| Component | Entrypoint | Responsibility |
| --- | --- | --- |
| API server | `backend/cmd/server` | HTTP APIs, WebSocket endpoints, signaling, embedded workers by default. |
| User Service | `backend/cmd/user-service` | gRPC token validation and user lookup. |
| Agent Worker | `backend/cmd/agent-worker` | Agent run and workflow processing. |
| Outbox Worker | `backend/cmd/outbox-worker` | Collaboration delivery, knowledge ingest/index events, transcription jobs, optional Kafka bridge. |
| Data Worker | `backend/cmd/data-worker` | Kafka settlement consumer, writes `room_settlements`. |
| Search Worker | `backend/cmd/search-worker` | Indexes messages into Elasticsearch. |
| Cleanup Worker | `backend/cmd/cleanup-worker` | Refresh-session and recording retention cleanup. |

## Required Services

- MySQL 8-compatible database.
- Redis 7-compatible cache/realtime dependency.

Optional services:

- S3-compatible object storage for recordings.
- Firebase Admin SDK service account for FCM.
- Kafka-compatible broker for settlement demo.
- Elasticsearch for search and chunk indexing.

## Environment Checklist

Core:

```bash
CONFIG_PATH=/app/configs/config.yaml
DB_DSN='allcallall:${MYSQL_PASSWORD}@tcp(mysql:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local'
REDIS_ADDR=redis:6379
REDIS_PASSWORD=...
JWT_SECRET=...
MAIL_PASSWORD=...
CORS_ALLOWED_ORIGINS=https://app.example.com
```

Workers:

```bash
EMBEDDED_WORKERS=0
WORKER_ID=outbox-worker-1
OUTBOX_WORKER_INTERVAL_SEC=1
OUTBOX_WORKER_BATCH_SIZE=50
```

Recording and transcription:

```bash
RECORDING_STORAGE_DRIVER=local
RECORDING_STORAGE_DIR=/tmp/allcallall-recordings
TRANSCRIPTION_ENABLED=true
TRANSCRIPTION_PROVIDER=openai_compatible
TRANSCRIPTION_OPENAI_BASE_URL=https://api.example.com/v1
TRANSCRIPTION_OPENAI_MODEL=whisper-1
TRANSCRIPTION_OPENAI_API_KEY=...
TRANSCRIPTION_CHUNK_SECONDS=600
TRANSCRIPTION_MAX_UPLOAD_BYTES=25165824
```

S3-compatible recording storage:

```bash
RECORDING_STORAGE_DRIVER=s3
RECORDING_S3_BUCKET=allcallall-recordings
RECORDING_S3_REGION=us-east-1
RECORDING_S3_ENDPOINT=https://s3.example.com
RECORDING_S3_ACCESS_KEY_ID=...
RECORDING_S3_SECRET_ACCESS_KEY=...
RECORDING_S3_FORCE_PATH_STYLE=1
```

gRPC/Kafka/Elasticsearch:

```bash
USER_GRPC_ADDR=:9090
USER_SERVICE_GRPC_ADDR=user-service:9090
KAFKA_BROKERS=kafka:9092
KAFKA_SETTLEMENT_TOPIC=allcallall.room.settlements
KAFKA_CONSUMER_GROUP=allcallall-data-worker
ELASTICSEARCH_URL=http://elasticsearch:9200
ELASTICSEARCH_INDEX=allcallall_messages
```

The maintained local Elasticsearch image is built from
`infra/elasticsearch/Dockerfile` and checksum-pins the IK plugin version to the
Elasticsearch version. Text fields use `ik_max_word` at index time and
`ik_smart` at query time. Reindex indices created by older images before
rolling out this mapping; startup fails closed when an existing mapping does
not declare the expected analyzers.

Web runtime config:

```bash
PUBLIC_API_BASE_URL=/api/v1
PUBLIC_WS_BASE_URL=
FIREBASE_API_KEY=...
FIREBASE_AUTH_DOMAIN=...
FIREBASE_PROJECT_ID=...
FIREBASE_MESSAGING_SENDER_ID=...
FIREBASE_APP_ID=...
FIREBASE_VAPID_KEY=...
REVENUECAT_PUBLIC_API_KEY=...
```

## Deployment Checklist

- Build succeeds: `cd backend && go build ./...`.
- Tests pass: `cd backend && go test ./...`.
- Vet passes: `cd backend && go vet ./...`.
- Required secrets are injected by runtime environment, not committed.
- `X-Request-ID` is generated or propagated by the edge/proxy.
- Public CORS origins are explicit.
- `/api/v1/chat/ws`, `/api/v1/ws`, and `/api/v1/signaling/*` have proxy timeouts suitable for realtime use.
- Recording download paths are always authorized through the backend.
- FCM, S3, Kafka, Elasticsearch, SMTP, and JWT secrets are never logged.
- Optional Kafka/Elasticsearch demos are either verified or clearly marked disabled.
- `docker compose ... run --rm migrate` succeeds before the API is started; `DB_AUTO_MIGRATE=0` in Beta/production.
- `/api/v1/health` reports process liveness and `/api/v1/ready` reports MySQL/Redis readiness.
- `AGENT_PROVIDER_STRICT=true` and a real transcription provider are configured; mock/rules output is not presented as Beta output.
- Support endpoints require both `SUPPORT_API_TOKEN` and an internal source network; expose them through a VPN/admin network, not the public edge.
- Web access tokens remain in memory; browser session recovery relies on the Secure, HttpOnly refresh cookie.

For the product-level 3-6 person validation path, use the [Beta Smoke Checklist](../development/beta-smoke.md).

## Web / Android / Desktop Checks

```bash
cd web && npm run typecheck && npm run lint && npm test && npm run build && npx playwright test
cd mobile && npm run test:unit && npx tsc --noEmit && npm run lint
cd desktop && npm run check && npm run build
```

Manual smoke:

- Login/register.
- Open Meetings.
- Join and leave a room.
- Start/stop a recording.
- Check recording card status: processing, ready, failed, or no transcript.
- Run an Agent on the conversation and verify meeting transcript context is available when transcription succeeded.

Local seed-data walkthrough:

```bash
make beta-seed
```

This creates a small owner/member workspace with sample chat, organization admin data, an ended meeting, and ready transcript segments. It is for UI and Agent grounding walkthroughs; it does not replace real ASR, real browser media, or TURN/TLS validation.

## Scope Boundaries

- Kubernetes is out of scope.
- The Beta supports OpenAI-compatible recording transcription and local/S3 recording reads. Real provider smoke tests require externally supplied credentials.
- Web billing and Web Push are implemented in the primary Web app, but require RevenueCat/Firebase public config plus backend FCM service-account config.

## Backup And Restore

MySQL:

```bash
docker compose --env-file .env -f infra/docker-compose.production.yml exec -T mysql \
  mysqldump -uallcallall -p"$MYSQL_PASSWORD" --single-transaction allcallall_db > allcallall.sql
cat allcallall.sql | docker compose --env-file .env -f infra/docker-compose.production.yml exec -T mysql \
  mysql -uallcallall -p"$MYSQL_PASSWORD" allcallall_db
```

Local recordings are stored in the `recording_data` volume. Stop the backend before archiving or restoring that volume so files and database metadata remain consistent. For S3, use bucket versioning and the object-store provider's replication/backup policy. Redis contains transient realtime/cache state; persist its volume for operational continuity, but restore MySQL and recordings as the authoritative data pair.

## Security And Observability

The API and TLS edge emit CSP, HSTS, `X-Content-Type-Options`, `Referrer-Policy`, and frame-deny headers. Knowledge URL ingestion validates every redirect target to block redirects into loopback, private, link-local, or multicast networks. Experimental E2EE remains disabled because the current React Native WebRTC runtime does not encrypt media frames with the exchanged application key.

`GET /api/v1/metrics` includes:

- ASR status, provider failure class, audio seconds/files, segment count, and processing duration sum/count.
- Agent run status, provider failures, run duration sum/count, and approval wait sum/count.
- Outbox publish/retry/failure counters and the current `outbox_backlog` gauge sampled by each worker poll.

Request IDs propagate into outbox events and trace spans. Recording, transcription, and Agent records retain their domain IDs so an incident can be followed across logs and persisted run history.

## CI And Protected E2E

Default CI runs backend tests, `go vet`, targeted `go test -race`, Web type/lint/unit/build/Playwright, mobile native checks, desktop checks, and MySQL/Redis/MinIO/Elasticsearch dependency checks. A protected Web smoke can run against a deployed Beta only when these repository secrets are configured:

```text
BETA_E2E_BASE_URL
BETA_E2E_USER_ONE / BETA_E2E_PASSWORD_ONE
BETA_E2E_USER_TWO / BETA_E2E_PASSWORD_TWO
BETA_E2E_ROOM_ID
```

Real ASR and end-to-end recording quality remain protected-environment tests because they require provider credentials, browser media permissions, and routable TURN/TLS endpoints.
