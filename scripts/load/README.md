# Load And Smoke Scripts

These scripts are lightweight interview/demo helpers for the backend portfolio. They are not a replacement for a full load-test platform, and they intentionally avoid adding test-only behavior to the application code.

## Scope

Use these scripts to collect evidence for:

- Agent run creation pressure and idempotency behavior.
- Agent run backlog/queue checks through persisted `agent_runs` statuses.
- Outbox drain behavior after Agent runs enqueue `agent.run.requested`, then produce `agent.run.completed` and `message.created`.
- Chat WebSocket connection stability and replay checks for `/api/v1/chat/ws`.

Current boundaries:

- Agent execution is asynchronous. `POST /api/v1/agent/runs` returns `202` with a `pending` run; the backend outbox worker consumes `agent.run.requested` and executes the run.
- Outbox drain is handled by the backend worker. This directory does not include a direct outbox processor runner.
- `ws-connections.mjs` opens sockets and counts messages/errors. It does not generate replay events by itself.

## Prerequisites

- Start MySQL/Redis and the backend server first.
- Use `CONFIG_PATH=./configs/config.yaml` when running the backend locally.
- Use a JWT for a user that belongs to the target organization/conversation.
- Use `X-Organization-ID` for Agent HTTP requests and `organization_id` query params for chat WebSocket connections.
- Optional: seed deterministic interview data with `cd backend && CONFIG_PATH=./configs/config.yaml go run ./cmd/interview-seed`.

Useful server knobs while measuring outbox drain:

```bash
OUTBOX_WORKER_INTERVAL_SEC=5
OUTBOX_WORKER_BATCH_SIZE=50
OUTBOX_WORKER_MAX_ATTEMPTS=3
OUTBOX_WORKER_RETRY_DELAY_SEC=10
```

## Agent Run Smoke

```bash
BASE_URL=http://localhost:8080 \
TOKEN=<jwt> \
ORGANIZATION_ID=<id> \
CONVERSATION_ID=<id> \
CONCURRENCY=10 \
POLL_AGENT_RUN=1 \
AGENT_POLL_TIMEOUT_SECONDS=30 \
./scripts/load/agent-run-smoke.sh
```

What it validates:

- `POST /api/v1/agent/runs`
- Idempotency-key handling per request
- Agent queue behavior: new runs should move from `pending` to `running` to `ready`
- `GET /api/v1/agent/runs/:id` polling until `ready` or `failed`
- Agent write amplification after worker execution: run, steps, tool calls, memory, outbox, message, follow-up task
- Current Agent run backlog check: `agent_runs` should not retain stuck `pending` or `running` rows after successful worker drain

Useful Agent smoke variables:

- `POLL_AGENT_RUN=1`: default; poll each created run until terminal status.
- `POLL_AGENT_RUN=0`: only measure create/enqueue behavior.
- `AGENT_POLL_TIMEOUT_SECONDS=30`: per-run timeout.
- `AGENT_POLL_INTERVAL_SECONDS=1`: poll interval.

Expected script output:

```text
[agent-run-smoke] accepted=10 ready=10 failed=0 timeout=0 failure=0 max_elapsed_seconds=5
```

What to record before and after:

```bash
curl -s http://localhost:8080/api/v1/metrics
```

```sql
SELECT status, COUNT(*) AS count
FROM agent_runs
GROUP BY status;

SELECT run_id, COUNT(*) AS tool_calls
FROM agent_tool_calls
GROUP BY run_id
ORDER BY run_id DESC
LIMIT 20;

SELECT status, COUNT(*) AS count, MIN(created_at) AS oldest_created_at
FROM event_outbox
GROUP BY status;
```

Expected counters to inspect:

- `agent_run_queued_total`
- `agent_run_started_total`
- `agent_run_total`
- `agent_run_failed_total`
- `agent_tool_call_total`
- `agent_memory_write_total`
- `outbox_publish_total`
- `outbox_publish_retry_total`
- `outbox_publish_failed_total`

Idempotency check:

- The script intentionally uses a distinct `Idempotency-Key` per request to create concurrent runs.
- To prove retry safety, repeat a single manual request with the same `Idempotency-Key` and confirm the run/result is reused without duplicate side effects.

## Outbox Drain Check

The Agent smoke script is the easiest way to enqueue outbox rows because run creation writes `agent.run.requested`, and worker execution later writes `agent.run.completed` plus `message.created`.

Suggested flow:

1. Start the backend with explicit `OUTBOX_WORKER_*` settings.
2. Capture `/api/v1/metrics` and `event_outbox` status counts.
3. Run `agent-run-smoke.sh`.
4. Poll `event_outbox` until requested/completed/message rows move from `pending` to published, or until retry/failure status appears.
5. Capture `outbox_publish_total`, `outbox_publish_retry_total`, and `outbox_publish_failed_total` deltas.

Do not claim retry/failure results unless you forced the handler to fail in a controlled dev setup. The default registered handlers execute `agent.run.requested` and observe `agent.run.completed` / `message.created`.

## WebSocket Connection Smoke

```bash
WS_URL=ws://localhost:8080/api/v1/chat/ws \
TOKEN=<jwt> \
ORGANIZATION_ID=<id> \
CLIENTS=10 \
DURATION_MS=10000 \
node scripts/load/ws-connections.mjs
```

What it validates:

- WebSocket connection acceptance
- Authenticated organization-scoped chat WebSocket path
- Basic connection stability
- Message/error counters

The script connects to:

```text
ws://localhost:8080/api/v1/chat/ws?token=<jwt>&organization_id=<id>
```

Note: this script uses the global `WebSocket` runtime. Use Node 22+ or adapt it to a `ws` dependency if your local Node runtime does not expose global WebSocket.

## WebSocket Replay Check

Replay is a separate manual check because the connection script does not create chat events.

Suggested flow:

1. Find the latest event ID for the organization.

```sql
SELECT id, sequence, event, created_at
FROM chat_events
WHERE organization_id = <organization_id>
ORDER BY id DESC
LIMIT 20;
```

2. Start one or more WebSocket clients with `ws-connections.mjs`.
3. Generate chat or Agent events, for example by posting a message or running `agent-run-smoke.sh`.
4. Reconnect with a lower `since_id`.

```bash
WS_URL='ws://localhost:8080/api/v1/chat/ws?since_id=<last_seen_event_id>' \
TOKEN=<jwt> \
ORGANIZATION_ID=<id> \
CLIENTS=1 \
DURATION_MS=5000 \
node scripts/load/ws-connections.mjs
```

5. Verify replayed payloads have increasing `event_id` and `sequence`.

Current backend behavior to account for:

- Replay uses durable `chat_events` where `id > since_id`.
- The current backlog lookup limit is 100 events per reconnect.
- Chat replay uses `/api/v1/chat/ws`; do not confuse it with `/api/v1/ws`, which is the WebRTC signaling WebSocket.

## Fill-In Result Template

```text
Date:
Commit:
Environment:
Scenario:
Command:
Concurrency:
Duration:
Metrics before:
Metrics after:
agent_runs by status before/after:
event_outbox by status before/after:
latest chat_events before/after:
Success count:
Failure count:
p95 latency:
Error rate:
Replay count:
Outbox publish/retry/failure delta:
Notes:
```
