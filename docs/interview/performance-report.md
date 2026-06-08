# Performance Report Template

This report is a living document for the backend interview portfolio story. Fill in numbers after running smoke/load checks against a local or staging environment. Do not present placeholder values as measured results.

## Goals

- Validate the Agent run queue path, including idempotency, persisted run status, outbox enqueue/drain, tool-call write amplification, and stuck `pending`/`running` backlog.
- Validate outbox worker drain rate, retry behavior, and failure visibility.
- Validate WebSocket replay behavior under concurrent clients and reconnects with `since_id`.
- Validate meeting event throughput and database write pressure.
- Validate recording download authorization and storage latency.

Portfolio focus:

- AI Agent reliability: `agent_runs`, `agent_steps`, `agent_tool_calls`, `agent_memories`, and `event_outbox` are the primary tables to discuss.
- Realtime reliability: `/api/v1/chat/ws` replays durable `chat_events` by `since_id`; `/api/v1/ws` and `/api/v1/signaling/*` are separate WebRTC signaling paths.
- Operational visibility: `/api/v1/metrics` exposes in-memory counters in Prometheus text format for demo and interview inspection.

## Test Environment

- Backend: Go 1.24, Gin, Gorm
- Database: MySQL 8.0 or local Docker MySQL
- Redis: Redis 7.2
- Storage: local or S3-compatible MinIO
- Machine:
- Commit SHA:
- Dataset/seed:
- `AGENT_PROVIDER`:
- `OUTBOX_WORKER_INTERVAL_SEC`:
- `OUTBOX_WORKER_BATCH_SIZE`:
- `OUTBOX_WORKER_MAX_ATTEMPTS`:
- `OUTBOX_WORKER_RETRY_DELAY_SEC`:

## Load Scripts

Scripts live in `scripts/load/`.

- `agent-run-smoke.sh`: concurrent Agent run creation against one conversation.
- `ws-connections.mjs`: WebSocket connection smoke/load template for `/api/v1/chat/ws`.

Current script boundaries:

- There is no standalone Agent queue worker script. The server outbox worker consumes `agent.run.requested`, transitions `agent_runs` from `pending` to `running`, and marks the run `ready` or `failed`.
- There is no standalone outbox drain script. Exercise outbox drain by creating Agent runs, then observe `agent.run.requested`, `agent.run.completed`, `message.created`, `event_outbox` status counts, and `outbox_publish_*` metrics while the server worker runs.
- There is no standalone replay generator. Validate replay manually by generating chat/Agent events, reconnecting to `/api/v1/chat/ws?organization_id=<id>&since_id=<last_seen_event_id>`, and checking replayed `event_id`/`sequence` ordering.

## Metrics To Capture

Backend `/api/v1/metrics`:

- `agent_run_queued_total`
- `agent_run_started_total`
- `agent_run_total`
- `agent_run_failed_total`
- `agent_tool_call_total`
- `agent_memory_write_total`
- `outbox_publish_total`
- `outbox_publish_retry_total`
- `outbox_publish_failed_total`
- `chat_realtime_delivery_fail_total`
- `meeting_join_total`
- `recording_download_total`
- `recording_storage_write_fail_total`

Database checks:

```sql
-- Agent run queue/backlog proxy.
SELECT status, COUNT(*) AS count
FROM agent_runs
GROUP BY status;

-- Agent write amplification per run.
SELECT run_id, COUNT(*) AS tool_calls
FROM agent_tool_calls
GROUP BY run_id
ORDER BY run_id DESC
LIMIT 20;

-- Outbox drain status.
SELECT status, COUNT(*) AS count, MIN(created_at) AS oldest_created_at
FROM event_outbox
GROUP BY status;

-- Realtime replay window.
SELECT id, sequence, event, created_at
FROM chat_events
WHERE organization_id = <organization_id>
ORDER BY id DESC
LIMIT 20;
```

System metrics:

- p50/p95/p99 API latency
- MySQL CPU, slow queries, active connections
- Redis ops/sec and memory
- WebSocket connected clients
- Error rate

## Baseline Results

| Scenario | Concurrency | Duration | p95 Latency | Error Rate | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| Agent run creation | TBD | TBD | TBD | TBD | New idempotency key per request; expect `202 pending` |
| Agent idempotency replay | TBD | TBD | TBD | TBD | Same key should not duplicate tool side effects |
| Agent run backlog | TBD | TBD | TBD | TBD | Count `pending`/`running`/`failed` rows before and after worker drain |
| Outbox drain | TBD | TBD | TBD | TBD | `agent.run.requested`, `agent.run.completed`, `message.created` batch size/retry settings |
| WebSocket connections | TBD | TBD | TBD | TBD | Authenticated `/api/v1/chat/ws` |
| WebSocket replay | TBD | TBD | TBD | TBD | `since_id` replay, backlog limit 100 |
| Meeting event replay | TBD | TBD | TBD | TBD | Room events written into conversation event stream |
| Recording download | TBD | TBD | TBD | TBD | local vs S3 |

## Fill-In Run Template

Copy one block per run.

```text
Date:
Commit:
Environment:
Dataset:
Operator:

Scenario:
Command:
Concurrency:
Duration:
Input IDs:
  organization_id:
  conversation_id:
  room_id:
  since_id:

Before:
  /api/v1/metrics snapshot:
  agent_runs by status:
  event_outbox by status:
  latest chat_events:

After:
  /api/v1/metrics snapshot:
  agent_runs by status:
  event_outbox by status:
  latest chat_events:

Results:
  success_count:
  failure_count:
  p50_latency_ms:
  p95_latency_ms:
  p99_latency_ms:
  error_rate:
  outbox_published_delta:
  outbox_retry_delta:
  outbox_failed_delta:
  replayed_event_count:
  duplicate_side_effects_detected:

Notes:
```

## Scenario Checklists

Agent run creation and queue/backlog:

- Run `agent-run-smoke.sh` with distinct idempotency keys.
- Capture `agent_run_queued_total`, `agent_run_started_total`, `agent_run_total`, `agent_run_failed_total`, `agent_tool_call_total`, and `agent_memory_write_total` before and after.
- Query `agent_runs` by status. The steady-state target is no stuck `pending` or `running` rows after the outbox worker drains requested runs.
- Query `agent_tool_calls` per run to explain write amplification.
- Repeat one request with the same `Idempotency-Key`; expected result is the same run result without duplicate message, follow-up task, memory, or outbox side effects.
- Replay worker execution against the same run; expected result is the persisted ready run without another six tool calls.

Outbox drain:

- Start the server with explicit `OUTBOX_WORKER_*` values.
- Create Agent runs until `event_outbox` has new `agent.run.requested` rows.
- Confirm the worker publishes requested rows and creates corresponding `agent.run.completed` plus `message.created` events during tool execution.
- Measure time from row creation to `published_at` and compare it with worker interval and batch size.
- Capture `outbox_publish_total`, `outbox_publish_retry_total`, and `outbox_publish_failed_total`.
- For failure testing, use a controlled local/dev setup only; do not claim retry/failure results unless the handler path was actually forced to fail.

WebSocket replay:

- Connect with `TOKEN`, `ORGANIZATION_ID`, and optionally `WS_URL=ws://localhost:8080/api/v1/chat/ws`.
- Generate events while one or more clients are disconnected.
- Reconnect with `since_id=<last_seen_event_id>`.
- Verify replayed messages include monotonically increasing `event_id` and `sequence`.
- Record whether replay count is capped by the current backlog limit of 100.

## Bottlenecks To Discuss

- MySQL is currently the durable realtime replay store; high event throughput may require Redis Streams or Kafka later.
- Agent execution is asynchronous but still writes multiple rows per completed run: run state transition, steps, tool calls, memory, outbox, message, follow-up task.
- Outbox throughput is controlled by `OUTBOX_WORKER_INTERVAL_SEC`, `OUTBOX_WORKER_BATCH_SIZE`, retry delay, and handler latency.
- WebRTC media relay and recording are separate bottleneck domains from HTTP API latency.

## Optimization Ideas

- Move Agent execution from the current outbox-worker handler to a dedicated queue/worker pool if planner latency or tool execution becomes too expensive for the general outbox processor.
- Batch realtime event writes per recipient group.
- Replace the lightweight outbox handler with Redis Streams/Kafka publishing if event volume grows or cross-service consumers are introduced.
- Add Redis Streams for high-volume room events.
- Add SQL indexes for hottest replay/query paths.
- Split collaboration service into chat, room, recording, and support services.
