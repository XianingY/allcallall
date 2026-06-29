# Quick Start

This is the shortest current path to run the backend API and Web surface locally.

## Prerequisites

- Go 1.24+
- Node.js compatible with the Vite Web app and Expo native toolchain.
- Docker / Docker Compose
- MySQL and Redis through `infra/docker-compose.yml`

## 1. Start MySQL And Redis

```bash
./scripts/development/start-services.sh
```

Verify:

```bash
docker compose -f infra/docker-compose.yml ps
```

## 2. Run Backend API

```bash
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/server
```

Health check:

```bash
curl http://localhost:8080/api/v1/health
```

Notes:

- Embedded workers are enabled by default.
- Set `EMBEDDED_WORKERS=0` if you want to run standalone workers.
- The API server can load a local `.env` as a convenience, but explicit shell/Docker env injection is the deployable path.

## 3. Optional Recording Transcription

The current transcription path runs after recording stop, not during realtime translation.

```bash
cd backend
TRANSCRIPTION_ENABLED=true \
TRANSCRIPTION_PROVIDER=mock \
RECORDING_STORAGE_DRIVER=local \
RECORDING_STORAGE_DIR=/tmp/allcallall-recordings \
CONFIG_PATH=./configs/config.yaml \
go run ./cmd/server
```

When a room recording is stopped, the service creates a `recording.transcription.requested` outbox event. The worker writes `meeting_transcript_segments` that the Agent can later retrieve as `meeting_transcript` context.

Use `TRANSCRIPTION_PROVIDER=mock` only for local deterministic development. For a Beta validation run, configure an OpenAI-compatible ASR provider instead:

```bash
TRANSCRIPTION_ENABLED=true
TRANSCRIPTION_PROVIDER=openai_compatible
TRANSCRIPTION_OPENAI_BASE_URL=https://api.example.com/v1
TRANSCRIPTION_OPENAI_MODEL=whisper-1
TRANSCRIPTION_OPENAI_API_KEY=...
AGENT_PROVIDER=openai_compatible
AGENT_PROVIDER_STRICT=true
```

## 4. Optional Beta Seed

After MySQL is running, seed a small-team walkthrough workspace:

```bash
make beta-seed
```

The command creates owner/member accounts, an organization, a team, a conversation, sample chat messages, a completed meeting, ready transcript segments, and direct Web routes. This is seed text for UI and Agent grounding demos; it does not create real audio bytes or prove ASR quality.

## 5. Run Web Workspace

```bash
cd web
npm install
npm run dev
```

The Vite dev server listens on `http://localhost:5173` and proxies `/api` plus WebSocket traffic to the backend at `http://127.0.0.1:8080`.

Useful Web routes:

- `/meetings`
- `/meetings/:roomId/preflight`
- `/meetings/:roomId`
- `/agent-lab`
- `/knowledge`
- `/conversations/:conversationId`

## 6. Run Android Development Client

```bash
adb reverse tcp:8080 tcp:8080
adb reverse tcp:8081 tcp:8081

cd mobile
npm run start:dev-client
```

Mobile native runtime config uses `EXPO_PUBLIC_*` only. Browser runtime config belongs to the `web/` app and production `/config.js`. `APP_ENV` is historical.

## 7. Optional Extracted Processes

```bash
make run-user-service
make run-agent-worker
make run-outbox-worker
make run-data-worker
make run-search-worker
make run-cleanup-worker
```

Useful variables:

```bash
USER_GRPC_ADDR=:9090
USER_SERVICE_GRPC_ADDR=localhost:9090
KAFKA_BROKERS=localhost:9092
KAFKA_SETTLEMENT_TOPIC=allcallall.room.settlements
ELASTICSEARCH_URL=http://localhost:9200
ELASTICSEARCH_INDEX=allcallall_messages
```

## 8. Optional Interview Infra

```bash
docker compose -f infra/docker-compose.yml \
  --profile microservices \
  --profile interview-infra \
  up api user-service outbox-worker data-worker search-worker kafka elasticsearch
```

This starts the portfolio/demo topology: API, gRPC User Service, standalone workers, Kafka-compatible broker, and Elasticsearch.

## Verification

```bash
cd backend && go test ./... && go vet ./...
cd web && npm run typecheck && npm run lint && npm test && npm run build
cd mobile && npm run test:unit && npx tsc --noEmit && npm run lint
cd desktop && npm run check && npm run build
```

Use narrower checks while developing, then broaden before committing shared behavior changes.

## More Docs

- [Configuration](../configuration/configuration.md)
- [Deployment Guide](../deployment/deployment-guide.md)
- [AI Agent Design](../interview/ai-agent-design.md)
- [Worker Runtime](../interview/worker-runtime.md)
