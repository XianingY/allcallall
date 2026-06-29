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
- Traceability: `X-Request-ID` is normalized at ingress and persisted on Agent runs plus outbox rows, so API responses, worker logs, and database rows can be correlated.

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

- `go run ./cmd/interview-bench`: local SQLite benchmark for Agent run creation, outbox drain, tool-call side effects, and metric counters. This is the fastest interview-safe evidence command because it does not require MySQL, Redis, or external model credentials.
- `go run ./cmd/realtime-replay-bench`: local SQLite benchmark for durable realtime event writes, recipient-scoped `since_id` replay, replay limits, and sequence monotonicity.
- `scripts/load/realtime-replay-bench.sh`: shell wrapper around `cmd/realtime-replay-bench` for load-script consistency.
- `go run ./cmd/chat-ws-replay-bench`: in-process authenticated Gin/WebSocket replay benchmark for `/api/v1/chat/ws`.
- `scripts/load/chat-ws-replay-bench.sh`: shell wrapper around `cmd/chat-ws-replay-bench`.
- `scripts/load/api-qps-bench.mjs`: live MySQL/Redis + Gin API benchmark for `GET messages`, `POST message`, and Agent run create/enqueue QPS.
- `make interview-microservice-demo`: multi-process worker demo with API, Agent worker, outbox worker, and cleanup worker.
- `cmd/user-service`: gRPC User Service for internal token validation and user lookup demos.
- `cmd/data-worker`: Kafka consumer for `allcallall.room.settlements`.
- `cmd/search-worker`: Elasticsearch indexing worker for `search.message.index_requested`.
- `agent-run-smoke.sh`: concurrent Agent run creation against one conversation, with optional polling until each run reaches `ready` or `failed`.
- `ws-connections.mjs`: WebSocket connection smoke/load template for `/api/v1/chat/ws`.

Current script boundaries:

- `interview-bench` is a local functional benchmark, not a production load test. Use it to prove the Agent/outbox pipeline and capture baseline write amplification; use staging/MySQL scripts for concurrency and infrastructure numbers.
- `realtime-replay-bench` is a local functional benchmark for the durable replay store. It proves `chat_events` scope/sequence/replay behavior without a running backend or JWT.
- `chat-ws-replay-bench` is an in-process authenticated transport benchmark. It starts a local Gin/WebSocket server, generates a local JWT, resolves organization membership from SQLite, and validates the real `/api/v1/chat/ws` replay path without external services.
- Standalone workers exist for the portfolio demo. Use `EMBEDDED_WORKERS=0` plus `cmd/agent-worker`, `cmd/outbox-worker`, and `cmd/cleanup-worker` to prove process isolation while keeping the same database contracts.
- Kafka and Elasticsearch checks require optional infrastructure. Use the Compose `interview-infra` profile or local equivalents, then run `cmd/data-worker` and `cmd/search-worker` with `KAFKA_BROKERS` and `ELASTICSEARCH_URL` configured.

## Metrics To Capture

Backend `/api/v1/metrics`:

- `agent_run_queued_total`
- `agent_run_started_total`
- `agent_run_total`
- `agent_run_failed_total`
- `agent_planner_latency_ms_total`
- `agent_planner_token_estimate_total`
- `agent_planner_fallback_total`
- `agent_tool_call_total`
- `agent_memory_write_total`
- `outbox_publish_total`
- `outbox_publish_retry_total`
- `outbox_publish_failed_total`
- `chat_realtime_delivery_fail_total`
- `meeting_join_total`
- `recording_download_total`
- `recording_storage_write_fail_total`
- `room_settlement_published_total` if enabled in the current build
- `search_message_index_requested_total` if enabled in the current build

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

-- Trace correlation from API request to async worker row.
SELECT id, request_id, status, event_name, attempts, last_error
FROM event_outbox
WHERE request_id = '<request_id>'
ORDER BY id DESC;

-- Realtime replay window.
SELECT id, sequence, event, created_at
FROM chat_events
WHERE organization_id = <organization_id>
ORDER BY id DESC
LIMIT 20;

-- Kafka settlement worker output.
SELECT room_id, user_id, source_event_id, status, duration_seconds, created_at
FROM room_settlements
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
| Local Agent/outbox benchmark | 1 process | 536 ms | 12 ms execute-run | 0% | June 19, 2026 deterministic `rules` run, temporary SQLite |
| Live GET messages API | 20 clients | 60 s | 63 ms | 0% | June 25, 2026 isolated Docker MySQL/Redis, `mock_llm`, 35,286 requests, 587.99 QPS |
| Live POST message API | 20 clients | 60 s | 574 ms | 0% | June 25, 2026 isolated Docker MySQL/Redis, `mock_llm`, 2,878 requests, 47.71 QPS |
| Live Agent run create/enqueue API | 20 clients | 60 s | 492 ms | 0% | June 25, 2026 isolated Docker MySQL/Redis, `mock_llm`, 4,258 requests, 70.81 QPS; not Agent end-to-end throughput |
| Agent run creation | TBD | TBD | TBD | TBD | New idempotency key per request; expect `202 pending` |
| Agent idempotency replay | TBD | TBD | TBD | TBD | Same key should not duplicate tool side effects |
| Agent run backlog | TBD | TBD | TBD | TBD | Count `pending`/`running`/`failed` rows before and after worker drain |
| Outbox drain | TBD | TBD | TBD | TBD | `agent.run.requested`, `agent.run.completed`, `message.created` batch size/retry settings |
| Local realtime replay benchmark | 1 process | 3815 ms | 3 ms write | 0% | commit `955e593`, temporary SQLite |
| Authenticated chat WebSocket replay | 5 clients | 2740 ms | 9 ms connect-to-last | 0% | commit `955e593`, in-process Gin/WebSocket |
| gRPC User Service validation | TBD | TBD | TBD | TBD | `cmd/server` with `USER_SERVICE_GRPC_ADDR` -> `cmd/user-service` |
| Kafka room settlement | TBD | TBD | TBD | TBD | outbox -> Kafka -> `cmd/data-worker` -> `room_settlements` |
| Elasticsearch message search | TBD | TBD | TBD | TBD | message -> outbox -> `cmd/search-worker` -> ES -> membership-filtered API |
| WebSocket connections | TBD | TBD | TBD | TBD | Authenticated `/api/v1/chat/ws` |
| WebSocket replay | TBD | TBD | TBD | TBD | `since_id` replay, backlog limit 100 |
| Meeting event replay | TBD | TBD | TBD | TBD | Room events written into conversation event stream |
| Recording download | TBD | TBD | TBD | TBD | local vs S3 |

## Latest Local Agent Benchmark Snapshot

Measured locally on June 19, 2026 (Asia/Shanghai) with temporary SQLite. Treat this as a functional benchmark and interview demo baseline, not a production load-test result.

Command:

```bash
go run ./cmd/interview-bench -conversations 25 -batch-size 50
```

Result summary:

| Metric | Value |
| --- | ---: |
| conversations | 25 |
| queued_runs | 25 |
| ready_runs | 25 |
| failed_runs | 0 |
| processed_events | 75 |
| pending_outbox_events | 0 |
| failed_outbox_events | 0 |
| agent_tool_calls | 175 |
| agent_context_chunks | 75 |
| total_duration_ms | 536 |
| queue_latency_p95_ms | 1 |
| execute_run_latency_p95_ms | 12 |
| outbox_publish_total | 75 |

Notes:

- Each completed run records seven auditable tool calls: three structured context tools, one RAG-lite context retrieval tool, and three mutating side-effect tools.
- The RAG-lite path indexes notes, messages, and meeting-aware context into `agent_context_chunks` and retrieves bounded Top-K snippets before planning.
- Mutating tool orchestration is now isolated in `backend/internal/agent/tool_executor.go`, so this benchmark covers the extracted executor boundary as well as the async run queue and outbox path.

## Latest Deterministic Resume Eval Snapshot

Measured locally on June 19, 2026 (Asia/Shanghai) through `make resume-eval`. This snapshot combines deterministic regression, RAG IR metrics, task-level black-box checks, and pipeline benchmark data under one `rules` path.

| Metric | Value |
| --- | ---: |
| planner_pass_rate | 100% |
| planner_avg_prompt_tokens | 337.5 |
| rag_pass_rate | 100% |
| rag_avg_latency_ms | 26.0 |
| rag_citation_hit_rate | 100% |
| rag_recall_at_k | 1.00 |
| rag_precision_at_k | 0.75 |
| rag_mrr | 1.00 |
| rag_ndcg_at_k | 1.00 |
| workflow_pass_rate | 100% |
| workflow_approval_interception_rate | 66.7% |
| workflow_meeting_transcript_coverage | 100% |
| task_eval_cases | 8 |
| task_success_rate | 100% |
| task_approval_safety_rate | 100% |
| benchmark_ready_run_rate | 100% |
| benchmark_execute_run_p95_ms | 12 |
| benchmark_tool_calls_per_run | 7.0 |
| benchmark_context_chunks_per_run | 3.0 |

Use this section for resume and interview claims about deterministic regression, retrieval quality on the current fixture set, and safety gates. Use the benchmark-only section above for pipeline and latency claims. Use [Agent UX Eval](agent-ux-eval.md) for manual pilot scoring.

## Latest Local Realtime Replay Snapshot

Measured locally on June 9, 2026 (Asia/Shanghai) at commit `955e593` with temporary SQLite. Treat this as durable replay-store evidence, not an authenticated WebSocket transport result.

Command:

```bash
make realtime-replay-bench
```

Result summary:

| Metric | Value |
| --- | ---: |
| events | 2000 |
| recipients | 10 |
| total_events_written | 2000 |
| target_events | 200 |
| replay_since_id | 791 |
| replayed_events | 100 |
| expected_replayed | 100 |
| scoped_correctly | true |
| monotonic_ids | true |
| monotonic_sequences | true |
| sequence_mismatch | 0 |
| total_duration_ms | 3815 |
| write_latency_p95_ms | 3 |
| replay_latency_p95_ms | 0 |

## Latest Authenticated WebSocket Replay Snapshot

Measured locally on June 9, 2026 (Asia/Shanghai) at commit `955e593` with temporary SQLite, local JWT generation, in-process Gin router, and real `/api/v1/chat/ws` WebSocket upgrade.

Command:

```bash
make chat-ws-replay-bench
```

Result summary:

| Metric | Value |
| --- | ---: |
| events | 2000 |
| recipients | 10 |
| clients | 5 |
| target_events | 200 |
| replay_since_id | 791 |
| expected_per_client | 100 |
| upgrade_success | 5 |
| upgrade_errors | 0 |
| client_errors | 0 |
| total_replayed | 500 |
| complete_clients | 5 |
| scoped_correctly | true |
| monotonic_ids | true |
| monotonic_sequences | true |
| duplicate_events | 0 |
| sequence_mismatch | 0 |
| connect_to_first_p95_ms | 8 |
| connect_to_last_p95_ms | 9 |
| total_duration_ms | 2740 |

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

- Run `go run ./cmd/interview-bench -conversations 25 -batch-size 50` first. Capture `queued_runs`, `ready_runs`, `processed_events`, `agent_tool_calls`, `pending_outbox_events`, `queue_latency`, `execute_run_latency`, and `counters`.
- Repeat with `-provider=mock_llm` to demonstrate prompt construction and structured-output parsing without external credentials.
- Repeat with `-provider=openai_compatible` to demonstrate unavailable-provider fallback into `rules` and `agent_planner_fallback_total`.
- Run `agent-run-smoke.sh` with distinct idempotency keys.
- Keep `POLL_AGENT_RUN=1` when measuring end-to-end queue drain latency. Use `POLL_AGENT_RUN=0` only when measuring create/enqueue latency.
- Capture `agent_run_queued_total`, `agent_run_started_total`, `agent_run_total`, `agent_run_failed_total`, `agent_tool_call_total`, and `agent_memory_write_total` before and after.
- Capture `agent_planner_latency_ms_total`, `agent_planner_token_estimate_total`, and `agent_planner_fallback_total` when comparing `rules`, `mock_llm`, and unavailable `openai_compatible` fallback behavior.
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

gRPC User Service:

- Start `cmd/user-service` with the same `CONFIG_PATH` and JWT secret as the API.
- Start `cmd/server` with `USER_SERVICE_GRPC_ADDR=127.0.0.1:<port>`.
- Run login/refresh/protected endpoint smoke.
- Capture user-service request logs and API latency. Do not claim a gRPC-vs-local latency improvement unless both paths were measured on the same machine and commit.

Kafka room settlement:

- Start Kafka-compatible infrastructure, for example Redpanda from the Compose interview profile.
- Configure `KAFKA_BROKERS` and run the API/outbox worker so `settlement.room.ended` events are bridged to the settlement topic.
- Run `cmd/data-worker` and end one or more rooms.
- Query `room_settlements`; expected behavior is one idempotent settlement row per room participant.
- Re-deliver the same source event if testing idempotency; expected behavior is no duplicate settlement side effect.

Elasticsearch message search:

- Start Elasticsearch and configure `ELASTICSEARCH_URL`.
- Run `cmd/search-worker`.
- Create messages in a conversation and wait for `search.message.index_requested` outbox events to drain.
- Query `GET /api/v1/search/messages?q=<keyword>`.
- Verify results are filtered by conversation membership and do not leak cross-organization messages.

WebSocket replay:

- Connect with `TOKEN`, `ORGANIZATION_ID`, and optionally `WS_URL=ws://localhost:8080/api/v1/chat/ws`.
- Generate events while one or more clients are disconnected.
- Reconnect with `since_id=<last_seen_event_id>`.
- Verify replayed messages include monotonically increasing `event_id` and `sequence`.
- Record whether replay count is capped by the current backlog limit of 100.

## Bottlenecks To Discuss

- MySQL is currently the durable realtime replay store; high event throughput may require Redis Streams or additional Kafka topics later.
- Agent execution is asynchronous but still writes multiple rows per completed run: run state transition, steps, tool calls, memory, outbox, message, follow-up task.
- Outbox throughput is controlled by `OUTBOX_WORKER_INTERVAL_SEC`, `OUTBOX_WORKER_BATCH_SIZE`, retry delay, and handler latency.
- Kafka settlement and Elasticsearch search are optional infrastructure paths, so the local demo must clearly state whether those services were actually enabled.
- WebRTC media relay and recording are separate bottleneck domains from HTTP API latency.

## Optimization Ideas

- Move high-volume Agent execution from the current outbox contract to a dedicated Kafka/Redis Streams queue if planner latency or tool execution becomes too expensive for the general outbox processor.
- Batch realtime event writes per recipient group.
- Expand Kafka topics beyond room settlement after measuring event volume and consumer needs.
- Add Redis Streams for high-volume low-retention room state patches if MySQL replay becomes too expensive.
- Add SQL indexes for hottest replay/query paths.
- Split collaboration service into chat, room, recording, and support services.
