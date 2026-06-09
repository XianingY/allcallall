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

Expected count for a successful v1 run is six tool calls: three read-only and three side-effect tools.

## WebSocket Replay Misses Events

Likely causes:

- Client did not send `since_id`.
- JWT user is not an organization member.
- Replay limit was lower than backlog size.
- Events were never written to `chat_events`.

Checks:

```sql
SELECT id, sequence, event, recipient_user_id, created_at
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

## CI Mobile / Web Job Fails

Known checks:

- `cd mobile && npm run test:unit`
- `cd mobile && npx tsc --noEmit`
- `cd mobile && npm run lint`
- `cd mobile && npx expo export --platform web`
- `cd mobile && npm run web:smoke`

Common causes:

- Node version mismatch. GitHub CI uses Node 24.
- Web export imports a native-only dependency without a platform adapter.
- A route or link helper changed without updating unit tests.

Local reproduction:

```bash
cd mobile
npm run test:unit
npx tsc --noEmit
npm run lint
npx expo export --platform web
npm run web:smoke
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
