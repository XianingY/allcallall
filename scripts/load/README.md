# Load And Smoke Scripts

These scripts are lightweight interview/demo helpers for the backend portfolio. They are not a replacement for a full load-test platform, and they intentionally avoid adding test-only behavior to the application code.

## Scope

Use these scripts to collect evidence for:

- Agent run creation pressure and idempotency behavior.
- Agent run backlog/queue checks through persisted `agent_runs` statuses.
- Outbox drain behavior after Agent runs enqueue `agent.run.requested`, then produce `agent.run.completed` and `message.created`.
- Chat WebSocket connection stability and replay checks for `/api/v1/chat/ws`.
- Durable realtime replay store behavior through local `chat_events` write/replay checks.

For the default deterministic portfolio suite, run:

```bash
make interview-load-suite
```

It writes JSON artifacts plus `summary.md` to `/tmp/allcallall-interview-suite-*`.

For a MySQL/Redis-backed live smoke suite, run:

```bash
make interview-live-suite
```

It starts local Docker MySQL/Redis, seeds deterministic interview data, starts the backend if needed, logs in the seeded owner, runs Agent and WebSocket smoke scripts, and captures `/api/v1/metrics` before and after the smoke load.

Current boundaries:

- Agent execution is asynchronous. `POST /api/v1/agent/runs` returns `202` with a `pending` run; the backend outbox worker consumes `agent.run.requested` and executes the run.
- Outbox drain is handled by the backend worker. This directory does not include a direct outbox processor runner.
- `ws-connections.mjs` opens sockets and counts messages/errors. It does not generate replay events by itself.
- `realtime-replay-bench.sh` generates local durable replay evidence without requiring a running backend, JWT, MySQL, Redis, or WebSocket clients.
- `chat-ws-replay-bench.sh` starts an in-process authenticated Gin/WebSocket server with temporary SQLite and validates the real `/api/v1/chat/ws` replay path.

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

## Core API QPS Benchmark

Use `api-qps-bench.mjs` for live MySQL/Redis + Gin API measurements. It reports request count, QPS, p50/p95/p99 latency, error rate, and status counts as JSON.

The latest recorded local snapshot is documented in `docs/archive/reports/load-test-results.md` under "Core API QPS Benchmark"; do not copy JWT-bearing `login.json` artifacts into the repo.

```bash
BASE_URL=http://localhost:8080 \
TOKEN=<jwt> \
ORGANIZATION_ID=<id> \
CONVERSATION_ID=<id> \
SCENARIO=get_messages \
CONCURRENCY=20 \
DURATION_SECONDS=30 \
node scripts/load/api-qps-bench.mjs
```

Supported scenarios:

- `get_messages`: `GET /api/v1/conversations/:id/messages`
- `post_message`: `POST /api/v1/conversations/:id/messages`
- `post_agent_run`: `POST /api/v1/agent/runs`

For Agent, this script measures create/enqueue QPS only. Continue using `interview-bench` or `agent-run-smoke.sh` when measuring end-to-end Agent execution, outbox drain, tool calls, and ready/failed status.

Run each scenario separately and record commit SHA, machine, database, concurrency, duration, QPS, p95/p99 latency, and error rate before using the numbers in resume material.

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

## Realtime Replay Benchmark

For a stable local proof of replay storage semantics:

```bash
EVENTS=2000 \
RECIPIENTS=10 \
REPLAY_WINDOW=120 \
REPLAY_LIMIT=100 \
./scripts/load/realtime-replay-bench.sh
```

What it validates:

- Durable `chat_events` writes through `RealtimeEventStore`
- Recipient scoping under mixed-recipient event streams
- `since_id` replay semantics
- Replay limit behavior
- Monotonic `event_id` and `sequence`
- Local p50/p95 write and replay latency summaries

This benchmark is intentionally database-free and uses temporary SQLite. It complements, but does not replace, the authenticated WebSocket check below.

## Authenticated WebSocket Replay Benchmark

For a transport-level replay proof without a long-running backend or real account:

```bash
EVENTS=2000 \
RECIPIENTS=10 \
REPLAY_WINDOW=120 \
REPLAY_LIMIT=100 \
CLIENTS=5 \
./scripts/load/chat-ws-replay-bench.sh
```

What it validates:

- Local JWT generation and `auth.Middleware` query-token authentication
- Real `/api/v1/chat/ws?token=...&organization_id=...&since_id=...` upgrade
- Organization membership resolution from temporary SQLite
- Replay through `CollaborationHandler -> ChatHub -> ListRealtimeEventsSince`
- Per-client replay completeness under concurrent clients
- `event_id` and `sequence` monotonicity
- No cross-recipient payload leakage

## WebSocket Replay Check

Authenticated WebSocket replay is still a server-level check because it needs a real JWT and organization membership. Use `realtime-replay-bench.sh` first for deterministic store-level evidence, then validate the WebSocket transport with a running backend.

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


## End-to-End Agent Performance Suite

The agent-e2e-bench module runs reproducible end-to-end benchmarks against the Agent run lifecycle: enqueue → queue → runtime → terminal. It captures per-phase latencies (enqueue, queue, runtime, end-to-end) with p50/p95/p99/max percentiles, Prometheus metric deltas, and repository SHAs for traceability.

### Quick Start

```bash
make agent-performance-suite
```

This runs the `baseline` profile by default, which executes the in-process test suite with a mock server.

### Profiles

| Profile | Description | Runs | Concurrency | Requires |
|---------|-------------|------|-------------|----------|
| `baseline` | Deterministic in-process orchestration and persistence checks | 4 | 2 | None |
| `controlled` | Go backend + Agent Runtime + RAG Runtime with the fake provider; validates backend, Agent Runtime, and RAG Runtime are reachable before running | 10 | 2 | `BASE_URL`, `TOKEN`, running backend + Agent Runtime + RAG Runtime |
| `networked` | Full stack: MySQL, Redis, Go, Agent Runtime, RAG Runtime, and fake provider in separate processes; validates MySQL and Redis connectivity in addition to all controlled-profile dependencies, and records the validated topology in the report | 10 | 2 | `BASE_URL`, `TOKEN`, running MySQL + Redis + backend + Agent Runtime + RAG Runtime |
| `real-provider-canary` | Real provider with strict safety limits; records token ceiling from provider usage data when available | ≤10 | 1 | `ALLOW_REAL_PROVIDER_CANARY=1`, `BASE_URL`, `TOKEN` |

The `real-provider-canary` profile requires `ALLOW_REAL_PROVIDER_CANARY=1` and is capped at 10 runs with concurrency 1. It must never be the sole merge gate.

**What each profile proves:**

- **baseline**: The benchmark module itself works — lifecycle tracking, percentile computation, metric deltas, and token redaction all function correctly against an in-process mock. No external dependencies.
- **controlled**: The Go backend can accept agent run requests, the Agent Runtime and RAG Runtime are reachable, and the fake provider can serve as a deterministic LLM backend. Proves the Go→Agent→RAG→Provider path works end-to-end.
- **networked**: Everything in controlled, plus MySQL and Redis are validated as reachable. Proves the full production-like stack (database, cache, API, runtimes, provider) is connected and functional. The validated topology is recorded in the report.
- **real-provider-canary**: A real LLM provider is used instead of the fake one. Proves the production provider integration works and records token usage for cost monitoring.

### Provider URL Contract

The suite starts `fake-agent-provider.mjs` for the `controlled` and `networked` profiles and sets `AGENT_PROVIDER_URL` to its reachable address (default: `http://127.0.0.1:18465`). The Go backend must be configured to use this provider via its own configuration — the suite does not reconfigure the backend.

To wire the fake provider into the backend, set the backend's provider configuration to match `AGENT_PROVIDER_URL`:

```bash
# Start the backend with the fake provider
AGENT_PROVIDER_URL=http://127.0.0.1:18465 \
CONFIG_PATH=./configs/config.yaml \
go run ./cmd/api
```

The benchmark records the provider URL in the report's `configuration.providerUrl` field for traceability. Override with the `AGENT_PROVIDER_URL` environment variable if the backend uses a different URL than the default.

### Environment Variables

```text
PROFILE                     — baseline | controlled | networked | real-provider-canary
BASE_URL                    — API base URL (e.g. http://localhost:8080)
TOKEN                       — Bearer token (redacted from output)
ORGANIZATION_ID             — X-Organization-ID header value
CONVERSATION_ID             — conversation_id for run creation
RUNS                        — number of runs (default: profile-dependent)
CONCURRENCY                 — worker pool size (default: profile-dependent)
POLL_INTERVAL_MS            — poll interval in ms (default: 100)
TERMINAL_TIMEOUT_MS         — per-run timeout in ms (default: 60000)
METRICS_URL                 — Prometheus metrics endpoint
ALLOW_REAL_PROVIDER_CANARY  — must be "1" for real-provider-canary profile
FAKE_PROVIDER_PORT          — port for the fake provider (default: 18465)
FAKE_PROVIDER_LATENCY_MS    — fake provider base delay (default: 50)
FAKE_PROVIDER_FAILURE_RATE  — fake provider failure probability 0–1 (default: 0)
FAKE_PROVIDER_TIMEOUT_RATE  — fake provider timeout probability 0–1 (default: 0)
FAKE_PROVIDER_RESPONSE_BYTES — fake provider response size (default: 256)
AGENT_PROVIDER_URL          — override the provider URL recorded in the report
MYSQL_HOST                  — MySQL host for networked profile validation (default: 127.0.0.1)
MYSQL_PORT                  — MySQL port for networked profile validation (default: 3306)
REDIS_HOST                  — Redis host for networked profile validation (default: 127.0.0.1)
REDIS_PORT                  — Redis port for networked profile validation (default: 6379)
```

### CLI Flags

The benchmark module also supports CLI flags matching the environment variables:

```text
--base-url / --runs / --concurrency / --organization-id / --conversation-id
--token / --poll-interval-ms / --terminal-timeout-ms / --provider-url
```

### Output

The benchmark emits one JSON document to stdout containing:

- `runId` — stable run identifier for correlation
- `accepted`, `ready`, `failed`, `timedOut` — run counts
- `enqueueLatency`, `queueLatency`, `runtimeLatency`, `endToEndLatency` — per-phase percentile distributions
- `statusCounts` — terminal status distribution
- `requestIds`, `traceIds` — correlation IDs for API enqueue, outbox, Go context, Python nodes, RAG/provider calls, checkpoint writes, and result persistence
- `repositoryShas` — AllCallAll and agent-runtime SHAs
- `tokenCeiling` — aggregate token usage from provider responses (for canary cost monitoring; null when no usage data)
- `metricDeltas` — Prometheus metric deltas (before vs. after)
- `configuration.providerUrl` — the provider URL used/recorded for the run

The suite wrapper also writes a Markdown summary and full artifacts to a temporary directory (printed at end of run).

### Fake Agent Provider

`fake-agent-provider.mjs` exposes an OpenAI-compatible `/v1/chat/completions` endpoint that derives delay, failure, timeout, and response size deterministically from the request sequence number. It listens on a configurable port (default: 18465) and provides a `/health` endpoint for readiness checks. Use it with the `controlled` and `networked` profiles.

### Security

- The benchmark never prints bearer tokens or provider API keys (both `Bearer ...` and `sk-...` patterns are redacted).
- The `real-provider-canary` profile requires explicit opt-in via `ALLOW_REAL_PROVIDER_CANARY=1` and is capped at 10 runs / concurrency 1.
- Do not commit `.env`, `.omo`, `.workbuddy`, `.playwright-mcp/`, `output/`, credentials, or load-test authentication artifacts.

### Repository SHAs

The suite records the AllCallAll worktree SHA (`git rev-parse HEAD`) and the sibling agent-runtime repo SHA (`../allcallall-agent-runtime`). If the sibling repo is not checked out, the runtime SHA is recorded as `"unknown"`.
