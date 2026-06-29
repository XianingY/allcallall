# Load Test Results

This document records repeatable interview-oriented load checks. These are not production capacity claims; they are evidence that key backend paths are executable, measurable, and regression-testable.

## Latest Local Suite

- Date: June 19, 2026
- Command: `make resume-eval`
- Provider: `rules`
- Environment: local macOS, temporary SQLite/in-process HTTP where applicable
- Report directory: `docs/interview/generated-resume-eval`

### Planner Eval

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

### RAG Eval

| Metric | Value |
| --- | ---: |
| cases | 40 |
| passed | 40 |
| failed | 0 |
| answerable_cases | 32 |
| negative_cases | 8 |
| top_k_hit_rate | 100% |
| negative_pass_rate | 100% |
| citation_hit_rate | 100% |
| citation_error_rate | 57.2% |
| latency_p50_ms | 83 |
| latency_p95_ms | 181 |
| recall_at_k | 1.00 |
| precision_at_k | 0.43 |
| mrr | 0.97 |
| ndcg_at_k | 0.98 |
| vector_case_rate | 62.5% |
| sql_fallback_case_rate | 37.5% |

Interpretation:

- The deterministic fixture proves vector retrieval, SQL fallback retrieval, distractor-document precision, and no-answer behavior remain regression-testable.
- Negative/no-answer cases are tracked separately from answerable Recall/MRR so no-answer fixtures do not inflate or deflate IR metrics.
- Citation error rate is intentionally tracked to avoid presenting citation presence as citation correctness.

### Task Eval

| Metric | Value |
| --- | ---: |
| cases | 8 |
| passed | 8 |
| failed | 0 |
| task_success_rate | 100% |
| tool_intent_match_rate | 100% |
| approval_safety_rate | 100% |
| citation_presence_rate | 100% |
| meeting_grounding_rate | 100% |

Interpretation:

- This layer is a deterministic black-box task fixture set, not a user satisfaction survey.
- It verifies whether natural-language tasks complete, whether the chosen tools are reasonable, and whether approval / grounding behavior remains stable.

### Workflow Eval

| Metric | Value |
| --- | ---: |
| cases | 3 |
| passed | 3 |
| failed | 0 |
| ready_case_rate | 66.7% |
| approval_interception_rate | 66.7% |
| meeting_transcript_coverage | 100% |

Interpretation:

- The workflow harness exercises the fixed DAG, bounded role-level ReAct, approval interception, and transcript-backed meeting recap.
- One case intentionally ends in a policy-driven `failed` status, so pass/fail here reflects expected guardrail behavior rather than only `ready` terminal states.

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
| agent_tool_calls | 175 |
| system_messages | 25 |
| follow_up_tasks | 25 |
| agent_memories | 25 |
| agent_context_chunks | 75 |
| total_duration_ms | 536 |
| queue_latency_p95_ms | 1 |
| execute_run_latency_p95_ms | 12 |
| outbox_publish_total | 75 |

Interpretation:

- Each run records two steps and seven tool calls: recent meetings, members, contact profile, RAG-lite context chunks, message write-back, follow-up task, and scoped memory upsert.
- Notes, messages, and meeting-aware context are indexed into `agent_context_chunks`, then retrieved as Top-K context for the planner.
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
| total_duration_ms | 1400 |
| write_latency_p95_ms | 1 |

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

### Core API QPS Benchmark

Use `scripts/load/api-qps-bench.mjs` against a live local MySQL/Redis + Gin backend before adding QPS numbers to resume material.

Latest measured snapshot:

- Date: June 25, 2026
- Git HEAD during run: `000e153` plus local benchmark/eval working-tree changes
- Command: `node scripts/load/api-qps-bench.mjs`
- Environment: isolated Docker MySQL 8.0 + Redis 7.2, live Gin backend on `:18080`, `mock_llm`, seeded interview organization/conversation
- Report directory: `/tmp/allcallall-qps-20260625-145158`
- Concurrency: `20`
- Duration: `60` seconds per scenario

| Scenario | Endpoint | Requests | QPS | p50 Latency | p95 Latency | p99 Latency | Error Rate | Status Counts |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| `get_messages` | `GET /api/v1/conversations/:id/messages` | 35,286 | 587.99 | 29 ms | 63 ms | 113 ms | 0% | `200: 35286` |
| `post_message` | `POST /api/v1/conversations/:id/messages` | 2,878 | 47.71 | 394 ms | 574 ms | 765 ms | 0% | `201: 2878` |
| `post_agent_run` | `POST /api/v1/agent/runs` | 4,258 | 70.81 | 245 ms | 492 ms | 1138 ms | 0% | `202: 4258` |

Interpretation:

- `get_messages` is the cleanest read-path baseline and reached `587.99` QPS with p95 `63 ms`.
- `post_message` includes synchronous DB write and outbox enqueue pressure, so it is materially slower than the read path.
- `post_agent_run` measures run creation and queue enqueue only. It should not be described as Agent end-to-end throughput.
- These numbers are local benchmark evidence only. Use the report directory, concurrency, duration, and environment notes whenever quoting them.

Scenarios to run separately:

| Scenario | Endpoint | Scope |
| --- | --- | --- |
| `get_messages` | `GET /api/v1/conversations/:id/messages` | Read-path latency/QPS |
| `post_message` | `POST /api/v1/conversations/:id/messages` | Synchronous write + outbox enqueue pressure |
| `post_agent_run` | `POST /api/v1/agent/runs` | Agent run create/enqueue QPS only |

Required output fields:

| Metric | Meaning |
| --- | --- |
| `requests` | Completed requests during the measured window |
| `qps` | Completed requests divided by measured duration |
| `latency_ms.p50/p95/p99` | Client-observed request latency |
| `error_rate` | Non-2xx or timeout ratio |
| `status_counts` | HTTP status distribution |

Do not use `post_agent_run` QPS as Agent end-to-end throughput. Agent completion, tool calls, and outbox drain are measured by `interview-bench` or `agent-run-smoke.sh`.

## Live MySQL / Redis Checklist

Use the automated live suite when collecting staging-like local numbers:

```bash
make interview-live-suite
```

The suite writes a report to `/tmp/allcallall-interview-live-suite-*` and captures seed IDs, login response, Agent trace, Agent polling events, Agent SSE events, Agent smoke, WebSocket smoke, and metrics snapshots.

### Latest Live MySQL / Redis Suite

- Date: June 9, 2026
- Command: `CONCURRENCY=2 WS_CLIENTS=1 WS_DURATION_MS=1000 make interview-live-suite`
- Provider: `mock_llm`
- Environment: local Docker MySQL 8.0 + Redis 7.2, live Gin backend, live JWT auth, live `/api/v1/chat/ws`
- Report directory: `/tmp/allcallall-interview-live-suite-20260609-124501`

| Check | Result |
| --- | ---: |
| MySQL seed / migration | pass |
| Redis-backed backend startup | pass |
| Auth login | pass |
| Seeded Agent run trace fetch | pass |
| Seeded Agent run events fetch | pass |
| Seeded Agent SSE stream fetch | pass |
| Seeded Agent tool calls | 7 |
| Indexed context chunks | 20 |
| Agent smoke accepted | 2 |
| Agent smoke ready | 2 |
| Agent smoke failed | 0 |
| Agent smoke max elapsed seconds | 1 |
| WebSocket clients opened | 1 |
| WebSocket errors | 0 |
| WebSocket messages observed | 20 |
| Metrics snapshots captured | before + after |

Interpretation:

- This suite exercises the live MySQL schema instead of SQLite-only tests.
- It verifies that the Agent HTTP path, outbox worker, auth middleware, and WebSocket transport run together in one local stack.
- It caught and fixed two MySQL-only issues before this result: a reserved `agent_memories.key` query and a duplicate legal acceptance index tag.

Manual checklist:

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

## Optional gRPC / Kafka / Elasticsearch Evidence

These paths exist in the codebase but should only be reported with measured results from an environment where the optional services are actually running.

| Scenario | Current Evidence Status | Command / Signal |
| --- | --- | --- |
| gRPC User Service validation | TBD measured numbers | `cmd/user-service` plus API with `USER_SERVICE_GRPC_ADDR` |
| Kafka room settlement | TBD measured numbers | `event_outbox` -> broker -> `cmd/data-worker` -> `room_settlements` |
| Elasticsearch message search | TBD measured numbers | `search.message.index_requested` -> `cmd/search-worker` -> `/api/v1/search/messages` |

Recommended next measurement run:

```bash
docker compose -f infra/docker-compose.yml --profile microservices --profile interview-infra up
```

Capture commit SHA, topic/index names, request count, p95 latency, error rate, and database/index row counts before adding numbers to resume material.
