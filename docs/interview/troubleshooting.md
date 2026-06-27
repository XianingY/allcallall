# Troubleshooting Guide

Use this guide to debug the portfolio demo paths before an interview or while explaining production-readiness tradeoffs.

## Agent Run Stuck In `pending`

Likely causes:

- Backend outbox worker is not running.
- `OUTBOX_WORKER_INTERVAL_SEC` is too high for the demo window.
- `event_outbox` has pending rows but the worker cannot publish them.

Checks:

```sql
SELECT id, event, status, attempts, last_error, created_at, updated_at
FROM event_outbox
ORDER BY id DESC
LIMIT 20;
```

```sql
SELECT id, status, attempts, lease_until, error_message, created_at, updated_at
FROM agent_runs
ORDER BY id DESC
LIMIT 20;
```

Metrics to inspect:

- `agent_run_queued_total`
- `agent_run_started_total`
- `outbox_publish_total`
- `outbox_publish_retry_total`
- `outbox_publish_failed_total`

## Agent Run Fails

Likely causes:

- Planner provider is unavailable.
- Conversation membership check failed.
- Tool side effect failed inside a transaction.

Checks:

```sql
SELECT id, source, status, error_message, request_id, attempts
FROM agent_runs
ORDER BY id DESC
LIMIT 20;
```

```sql
SELECT run_id, tool_name, status, error_message, input_json, output_json
FROM agent_tool_calls
WHERE run_id = <run_id>
ORDER BY id ASC;
```

Demo-safe fallback:

```bash
AGENT_PROVIDER=rules make agent-eval
```

## Duplicate Agent Side Effects

Expected safeguards:

- API-level `Idempotency-Key` reuses an existing run.
- Outbox rows use stable idempotency keys such as `agent.run.requested:<run_id>`.
- Completed runs return the persisted result instead of re-executing tools.

Checks:

```sql
SELECT id, idempotency_key, status, conversation_id
FROM agent_runs
WHERE idempotency_key = '<key>';
```

```sql
SELECT tool_name, COUNT(*) AS count
FROM agent_tool_calls
WHERE run_id = <run_id>
GROUP BY tool_name;
```

Expected count for a successful current rules run is seven tool calls: structured context reads, RAG-lite context retrieval, and mutating side-effect tools.

## WebSocket Replay Misses Events

Likely causes:

- Client did not send `since_id`.
- JWT user is not an organization member.
- Replay limit was lower than backlog size.
- Events were never written to `chat_events`.

Checks:

```sql
SELECT id, sequence, event, user_id, created_at
FROM chat_events
WHERE organization_id = <organization_id>
ORDER BY id DESC
LIMIT 50;
```

Local deterministic checks:

```bash
make realtime-replay-bench
make chat-ws-replay-bench
```

## Recording Download Fails

Likely causes:

- User is outside the organization boundary.
- Organization policy disallows export.
- Object storage key or local file is missing.
- Signed URL generation failed.

Checks:

```sql
SELECT id, organization_id, room_id, status
FROM recording_sessions
ORDER BY id DESC
LIMIT 20;
```

```sql
SELECT id, session_id, storage_driver, storage_bucket, object_key, retention_until, deleted_at
FROM recording_files
ORDER BY id DESC
LIMIT 20;
```

Relevant error codes:

- `RECORDING_DOWNLOAD_UNAUTHORIZED`
- `RECORDING_DOWNLOAD_NOT_FOUND`
- `RECORDING_STORAGE_WRITE_FAILED`

## gRPC User Service Fails

Likely causes:

- `cmd/user-service` is not running.
- `USER_SERVICE_GRPC_ADDR` points to the wrong address.
- User Service and API use different JWT config.

Checks:

```bash
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/user-service
```

In another terminal:

```bash
cd backend
USER_SERVICE_GRPC_ADDR=127.0.0.1:9090 \
CONFIG_PATH=./configs/config.yaml \
go run ./cmd/server/main.go
```

Then exercise login and a protected endpoint. If protected endpoints fail only in gRPC mode, compare JWT issuer/secret and inspect User Service logs.

## Kafka Settlement Pipeline Fails

Likely causes:

- `KAFKA_BROKERS` is unset or unreachable.
- Outbox worker is not processing `settlement.room.ended`.
- Data worker is not running.
- Duplicate source events are being rejected correctly and mistaken for missing writes.

Checks:

```sql
SELECT id, event, status, attempts, last_error
FROM event_outbox
WHERE event = 'settlement.room.ended'
ORDER BY id DESC
LIMIT 20;
```

```sql
SELECT room_id, user_id, source_event_id, status, duration_seconds, created_at
FROM room_settlements
ORDER BY id DESC
LIMIT 20;
```

## Elasticsearch Search Fails

Likely causes:

- `ELASTICSEARCH_URL` is unset, so the memory/noop search path is being used.
- Search worker is not running.
- Index events are pending or failed in `event_outbox`.
- The authenticated user is not a member of the conversation containing the hit.

Checks:

```sql
SELECT id, event, status, attempts, last_error
FROM event_outbox
WHERE event = 'search.message.index_requested'
ORDER BY id DESC
LIMIT 20;
```

Then query:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  -H "X-Organization-ID: $ORGANIZATION_ID" \
  "http://localhost:8080/api/v1/search/messages?q=test"
```

## CI Mobile / Web Job Fails

Known checks:

- `cd mobile && npm run test:unit`
- `cd mobile && npx tsc --noEmit`
- `cd mobile && npm run lint`
- `cd web && npm run typecheck`
- `cd web && npm run lint`
- `cd web && npm test`
- `cd web && npm run build`
- `cd web && npx playwright test`

Common causes:

- Node version mismatch. GitHub CI uses Node 24.
- A Web route, generated API type, or link helper changed without updating tests.
- A native-only dependency leaked from `mobile/` into the independent Web app.
- Browser-only SDK config is missing from `web/public/config.js` or mocked runtime config.

Local reproduction:

```bash
cd mobile
npm run test:unit
npx tsc --noEmit
npm run lint
cd ../web
npm run typecheck
npm run lint
npm test
npm run build
npx playwright test
```

## Interview Demo Fails

Use the deterministic path first:

```bash
make interview-demo
```

If that fails, run each piece separately:

```bash
make agent-eval
make interview-bench
make realtime-replay-bench
make chat-ws-replay-bench
```

Only use the live path after Docker services are healthy:

```bash
make interview-demo-live
```

If live mode fails, check MySQL/Redis first:

```bash
docker compose -f infra/docker-compose.yml ps
```

For optional infrastructure demos, check the active profiles:

```bash
docker compose -f infra/docker-compose.yml --profile microservices --profile interview-infra ps
```
