# Load Test Results

This document records repeatable interview-oriented load checks. These are not production capacity claims; they are evidence that key backend paths are executable, measurable, and regression-testable.

## Latest Local Suite

- Date: June 9, 2026
- Command: `make interview-load-suite`
- Provider: `rules`
- Environment: local macOS, temporary SQLite/in-process HTTP where applicable
- Report directory: `/tmp/allcallall-interview-suite-20260609-110452`

### Agent Eval

| Metric | Value |
| --- | ---: |
| cases | 2 |
| passed | 2 |
| failed | 0 |
| provider | `rules` |

The eval fixture validates:

- High-priority thread emits `high_priority_thread`.
- Sparse unassigned thread emits `unassigned_conversation` and `insufficient_context`.
- Both cases produce non-empty `summary` and `next_step`.

### Agent / Outbox Benchmark

| Metric | Value |
| --- | ---: |
| conversations | 25 |
| queued_runs | 25 |
| ready_runs | 25 |
| failed_runs | 0 |
| processed_events | 75 |
| pending_outbox_events | 0 |
| failed_outbox_events | 0 |
| agent_steps | 50 |
| agent_tool_calls | 150 |
| system_messages | 25 |
| follow_up_tasks | 25 |
| agent_memories | 25 |
| total_duration_ms | 806 |
| queue_latency_p95_ms | 3 |
| execute_run_latency_p95_ms | 16 |
| outbox_publish_total | 75 |

Interpretation:

- Each run records two steps and six tool calls.
- Each completed run writes one system message, one follow-up task, and one memory row.
- Outbox write amplification is visible and drains cleanly in the local processor.

### Durable Realtime Replay Benchmark

| Metric | Value |
| --- | ---: |
| events | 2000 |
| recipients | 10 |
| target_events | 200 |
| replay_limit | 100 |
| replayed_events | 100 |
| scoped_correctly | true |
| monotonic_ids | true |
| monotonic_sequences | true |
| total_duration_ms | 3132 |
| write_latency_p95_ms | 2 |

Interpretation:

- Replay is recipient-scoped.
- `event_id` and `sequence` stay monotonic.
- Replay limit behavior is deterministic.

### Authenticated Chat WebSocket Replay Benchmark

| Metric | Value |
| --- | ---: |
| events | 2000 |
| recipients | 10 |
| clients | 5 |
| upgrade_success | 5 |
| upgrade_errors | 0 |
| client_errors | 0 |
| expected_per_client | 100 |
| total_replayed | 500 |
| complete_clients | 5 |
| scoped_correctly | true |
| duplicate_events | 0 |
| sequence_mismatch | 0 |
| connect_to_first_latency_p95_ms | 4 |
| connect_to_last_latency_p95_ms | 5 |
| total_duration_ms | 2027 |

Interpretation:

- The benchmark exercises the real `/api/v1/chat/ws` handler path through auth middleware and organization membership resolution.
- It validates replay completeness for multiple clients without a long-running backend.

## Live MySQL / Redis Checklist

Use this checklist when collecting staging-like numbers:

1. Start infra:

```bash
./scripts/development/start-services.sh
```

2. Seed data:

```bash
cd backend
CONFIG_PATH=./configs/config.yaml AGENT_PROVIDER=mock_llm go run ./cmd/interview-seed
```

3. Start server with explicit worker settings:

```bash
CONFIG_PATH=./configs/config.yaml \
OUTBOX_WORKER_INTERVAL_SEC=2 \
OUTBOX_WORKER_BATCH_SIZE=50 \
go run ./cmd/server/main.go
```

4. Run authenticated smoke scripts after login:

```bash
BASE_URL=http://localhost:8080 TOKEN=<jwt> ORGANIZATION_ID=<id> CONVERSATION_ID=<id> ./scripts/load/agent-run-smoke.sh
```

```bash
WS_URL=ws://localhost:8080/api/v1/chat/ws TOKEN=<jwt> ORGANIZATION_ID=<id> CLIENTS=10 node scripts/load/ws-connections.mjs
```

5. Capture:

- `/api/v1/metrics`
- `agent_runs` status counts
- `agent_tool_calls` per run
- `event_outbox` status counts
- `chat_events` latest sequence window

Do not put staging numbers in a resume unless they were measured against MySQL/Redis and the commit SHA is recorded.
