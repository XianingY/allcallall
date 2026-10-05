# Backend and AI Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Go-to-Python Agent path measurably faster and predictably bounded by removing query amplification, continuously draining the durable outbox, enforcing admission and deadline budgets, reusing outbound connections, reducing duplicate retrieval and checkpoint state, and scaling from workload pressure.

**Architecture:** Preserve Go as the owner of product state, permissions, approvals, durable events, and write execution, while Python continues to own Agent orchestration and RAG. Add observability first, then optimize each bounded stage without changing public API contracts. Keep MySQL Outbox as the default durable queue and evaluate a dedicated event stream only after the optimized implementation fails a documented capacity gate.

**Tech Stack:** Go 1.25, Gin, GORM, MySQL, Redis, Prometheus client_golang, OpenTelemetry, Python 3.11, FastAPI, LangGraph, httpx, PyMySQL, pytest, Helm, Kubernetes HPA, Node.js load scripts.

**Spec:** `docs/superpowers/specs/2026-10-05-backend-ai-performance-design.md`

## Global Constraints

- Work in isolated worktrees when implementation begins; create one worktree for `AllCallAll` and one for `../allcallall-agent-runtime`.
- Keep commits repository-local. Neither repository may require an atomic commit in the other.
- Preserve existing HTTP routes, JSON fields, OpenAPI contracts, authorization, approval safety, outbox idempotency, execution leases, and checkpoint resume behavior.
- Keep MySQL as the durable product and checkpoint store during this plan.
- Do not introduce Kafka, NATS, Redis Streams, Celery, or another production queue in these tasks.
- Every queue and concurrency boundary must have a configured maximum and observable overload behavior.
- Prometheus labels must be bounded. Do not use user IDs, organization IDs, conversation IDs, run IDs, prompts, exception strings, arbitrary tool names, or arbitrary event names as labels.
- Preserve strict ordering only where required; unrelated aggregates must remain independently processable.
- Agent enqueue p95 target: `< 250 ms` at the documented target load.
- Outbox queue-wait p95 target: `< 2 s` during steady load.
- Outbox drain target: at least `2x` measured peak ingress without unbounded backlog growth.
- Workflow list target: at most `8` SQL statements when returning 50 runs.
- Agent context target: at most `5` database round trips before optional retrieval.
- Checkpoint pool acquisition p95 target: `< 50 ms` under target load.
- Timed-out Agent work must remain accounted for until it exits and must not free capacity early.
- Performance changes must not regress grounding, citations, approval safety, idempotency, or recovery evaluations.
- Do not commit `.env`, `.omo`, `.workbuddy`, `.playwright-mcp/`, `output/`, credentials, or load-test authentication artifacts.
- Use `apply_patch` for source and documentation edits.
- Follow red-green-refactor for behavior changes and commit after every task.

## Repository and Worktree Setup

At execution time, use the worktree skill before Task 1. Suggested branches:

```bash
cd /Users/byzantium/github/AllCallAll
git worktree add ../AllCallAll-backend-ai-performance -b perf/backend-ai-performance

cd /Users/byzantium/github/allcallall-agent-runtime
git worktree add ../allcallall-agent-runtime-performance -b perf/runtime-performance
```

Record both starting SHAs in the first benchmark report. All paths below are relative to the corresponding repository worktree.

## File Structure

### `AllCallAll`

| Path | Responsibility |
| --- | --- |
| `backend/internal/metrics/performance.go` | Bounded-label queue, pool, dependency, context, and payload metrics |
| `backend/internal/cache/redis.go` | Redis client construction and pool-stat snapshot access |
| `backend/internal/database/mysql.go` | Effective GORM logging and SQL pool configuration |
| `backend/internal/agent/workflow_result_batch.go` | Fixed-query-count Workflow result assembly |
| `backend/internal/agent/context_repository.go` | Five-query base-context loading boundary |
| `backend/internal/agent/context_budget.go` | Context collection, byte, and token budgets |
| `backend/internal/events/processor.go` | Continuous drain and bounded parallel dispatch |
| `backend/internal/events/outbox.go` | Claims, ordered eligibility, lease extension, and state updates |
| `backend/migrations/000021_event_outbox_claim_indexes.*.sql` | Outbox claim and aggregate-order indexes |
| `backend/internal/agent/workflow_external_runtime.go` | Deadline propagation, transport budgets, overload classification |
| `scripts/load/agent-e2e-bench.mjs` | Concurrent enqueue-to-terminal benchmark |
| `scripts/load/fake-agent-provider.mjs` | Deterministic latency and failure provider |
| `infra/helm/allcallall/` | Runtime configuration, scraping, and pressure-based scaling |
| `docs/reference/performance.md` | SLOs, capacity model, benchmarks, and tuning guidance |

### `allcallall-agent-runtime`

| Path | Responsibility |
| --- | --- |
| `services/agent-runtime/allcallall_agent_runtime/metrics.py` | Counters, gauges, and histograms |
| `services/agent-runtime/allcallall_agent_runtime/admission.py` | Bounded execution admission and queue policy |
| `services/agent-runtime/allcallall_agent_runtime/deadline.py` | Absolute deadline, remaining budget, and cancellation token |
| `services/agent-runtime/allcallall_agent_runtime/clients.py` | Process-lifetime provider, Tool Bridge, and RAG clients |
| `services/agent-runtime/allcallall_agent_runtime/checkpoint/payload.py` | Resume-state projection and byte accounting |
| `services/agent-runtime/allcallall_agent_runtime/nodes/retrieval.py` | Retrieval ownership and per-run reuse |
| `services/agent-runtime/allcallall_agent_runtime/nodes/parallel_roles.py` | Bounded branch-local role execution and deterministic merge |
| `services/rag-runtime/allcallall_rag_runtime/clients.py` | Shared Go Bridge and Qdrant HTTP clients |
| `services/rag-runtime/allcallall_rag_runtime/pipeline.py` | Explicit source selection and request-scoped retrieval reuse |
| `services/rag-runtime/allcallall_rag_runtime/retrieval.py` | Single-pass candidate preparation and rerank reuse |

---

### Task 1: Go Performance Metrics Foundation and Runtime Scraping

**Repository:** `AllCallAll`

**Files:**

- Create: `backend/internal/metrics/performance.go`
- Create: `backend/internal/metrics/performance_test.go`
- Modify: `backend/internal/cache/redis.go`
- Create: `backend/internal/cache/redis_test.go`
- Modify: `infra/helm/allcallall/templates/runtime-deployments.yaml:20-31`
- Modify: `infra/helm/allcallall/templates/backend-deployments.yaml`
- Test: `backend/internal/metrics/performance_test.go`

**Interfaces:**

- Produces: `func NewPerformanceCollectors(registerer prometheus.Registerer) *PerformanceCollectors`.
- Produces collector methods `ObserveOutboxQueueWait`, `ObserveOutboxEvent`, `SetOutboxInflight`, `ObserveAgentContext`, `ObserveDependency`, `UpdateSQLDBStats`, and `UpdateRedisPoolStats` with the argument lists exercised in Step 1.
- Produces matching package-level functions backed by the process-default collector, including `func UpdateSQLDBStats(pool string, stats sql.DBStats)` and `func UpdateRedisPoolStats(pool string, stats *redis.PoolStats)`.
- Produces: `func SnapshotRedisPoolStats(client *redis.Client) *redis.PoolStats`
- Produces: `func BoundedLabel(raw string, allowed map[string]struct{}, fallback string) string`

- [ ] **Step 1: Write failing metric registration tests**

```go
func TestPerformanceCollectorsExposeBoundedMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	collectors := NewPerformanceCollectors(registry)

	collectors.ObserveOutboxQueueWait("agent", 250*time.Millisecond)
	collectors.ObserveOutboxEvent("agent", "success", 500*time.Millisecond)
	collectors.SetOutboxInflight("agent", 2)
	collectors.ObserveAgentContext(40*time.Millisecond, 4, 30, 4096, 900)
	collectors.ObserveDependency("agent_runtime", "run", "success", 2*time.Second)
	collectors.UpdateSQLDBStats("primary", sql.DBStats{OpenConnections: 5, InUse: 3, Idle: 2, WaitCount: 7})
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{TotalConns: 5, IdleConns: 2, WaitCount: 7, Timeouts: 1})

	assertMetricExists(t, registry, "outbox_queue_wait_seconds")
	assertMetricExists(t, registry, "agent_context_query_count")
	assertMetricExists(t, registry, "dependency_request_duration_seconds")
	assertMetricExists(t, registry, "sql_db_connections")
	assertMetricExists(t, registry, "redis_pool_connections")
	assertMetricExists(t, registry, "redis_pool_wait_total")
}
```

In `backend/internal/cache/redis_test.go`, construct a client with a test pool and assert `SnapshotRedisPoolStats` returns the client's current `PoolStats()` pointer without starting a goroutine.

- [ ] **Step 2: Run the test and verify failure**

```bash
cd backend && go test ./internal/metrics ./internal/cache -run 'TestPerformanceCollectors|TestSnapshotRedisPoolStats' -count=1
```

Expected: fail because the performance collectors do not exist.

- [ ] **Step 3: Implement bounded-label collectors**

Use injected `prometheus.Registerer` instances for tests and fixed duration buckets:

```go
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120}

var allowedWorkClasses = map[string]struct{}{
	"agent": {}, "collaboration": {}, "search": {}, "transcription": {}, "settlement": {}, "other": {},
}
```

Expose a process-default collector through package functions while retaining constructor injection. Keep the existing custom counter store during dashboard migration.

Export Redis active/idle/total connections, pool hits/misses, wait count, timeouts, and stale connections. Treat `nil` stats as a no-op. Do not create a ticker in `NewRedis`; Task 3 gives the runtime/bootstrap owner responsibility for periodic sampling and cancellation.

- [ ] **Step 4: Expose both Python runtime metric endpoints**

Change the runtime annotation condition to:

```yaml
{{- if $root.Values.observability.prometheus.scrape }}
prometheus.io/scrape: "true"
prometheus.io/port: {{ $runtime.port | quote }}
prometheus.io/path: /metrics
{{- end }}
```

Correct the Go API annotation to port `9090` and path `/metrics` if rendered output still points to the business endpoint.

- [ ] **Step 5: Verify**

```bash
cd backend && go test ./internal/metrics ./internal/cache -count=1
cd .. && helm template allcallall infra/helm/allcallall > /tmp/allcallall-performance-chart.yaml
rg -n "prometheus.io/(scrape|port|path)" /tmp/allcallall-performance-chart.yaml
```

Expected: metric tests pass; Agent and RAG pods are scraped; API metrics use port 9090.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/metrics/performance.go backend/internal/metrics/performance_test.go backend/internal/cache/redis.go backend/internal/cache/redis_test.go infra/helm/allcallall/templates/runtime-deployments.yaml infra/helm/allcallall/templates/backend-deployments.yaml
git commit -m "perf(observability): add backend pressure metrics"
```

---

### Task 2: Reproducible End-to-End Agent Performance Suite

**Repository:** `AllCallAll`

**Files:**

- Create: `scripts/load/agent-e2e-bench.mjs`
- Create: `scripts/load/agent-e2e-bench.test.mjs`
- Create: `scripts/load/fake-agent-provider.mjs`
- Create: `scripts/load/run-agent-performance-suite.sh`
- Modify: `scripts/load/README.md`
- Modify: `Makefile`

**Interfaces:**

- Produces: `runAgentBenchmark(options): Promise<AgentBenchmarkReport>`.
- `AgentBenchmarkReport` contains `accepted`, `ready`, `failed`, `timedOut`, `enqueueLatency`, `queueLatency`, `runtimeLatency`, `endToEndLatency`, `statusCounts`, and `metricDeltas`.
- Each report records both repository SHAs plus the stable run ID, request ID, and trace ID needed to correlate API enqueue, outbox, Go context, Python nodes, RAG/provider calls, checkpoint writes, and result persistence.
- Fake provider consumes `FAKE_PROVIDER_LATENCY_MS`, `FAKE_PROVIDER_FAILURE_RATE`, `FAKE_PROVIDER_TIMEOUT_RATE`, and `FAKE_PROVIDER_RESPONSE_BYTES`.
- The suite exposes four named profiles: `baseline`, `controlled`, `networked`, and `real-provider-canary`.

- [ ] **Step 1: Write failing lifecycle and percentile tests**

```js
test("reports enqueue-to-terminal phases separately", async () => {
  const report = await runAgentBenchmark({
    baseUrl: server.url,
    concurrency: 2,
    runs: 4,
    pollIntervalMs: 5,
    terminalTimeoutMs: 500,
  });

  assert.equal(report.ready, 4);
  assert.equal(report.failed, 0);
  assert.ok(report.enqueueLatency.p95 >= 0);
  assert.ok(report.queueLatency.p95 >= 0);
  assert.ok(report.endToEndLatency.p95 >= report.enqueueLatency.p95);
});
```

- [ ] **Step 2: Run the test and verify failure**

```bash
node --test scripts/load/agent-e2e-bench.test.mjs
```

Expected: fail because the benchmark module does not exist.

- [ ] **Step 3: Implement the benchmark runner**

Use a fixed worker pool, a distinct idempotency key per logical run, and bounded polling. Capture accepted, first-running, and terminal timestamps. Emit one JSON document to stdout and never print bearer tokens.

Support these flags and environment variables:

```text
--runs / RUNS
--concurrency / CONCURRENCY
--base-url / BASE_URL
--organization-id / ORGANIZATION_ID
--conversation-id / CONVERSATION_ID
--token / TOKEN
--poll-interval-ms / POLL_INTERVAL_MS
--terminal-timeout-ms / TERMINAL_TIMEOUT_MS
```

- [ ] **Step 4: Implement the deterministic provider and suite wrapper**

Expose an OpenAI-compatible `/chat/completions` route. Derive delay, failure, timeout, and response size deterministically from request sequence number. The wrapper records repository SHAs, host data, configuration, pre/post metrics, JSON output, and a Markdown summary under a printed temporary directory.

Implement the profiles as follows:

```text
baseline: deterministic in-process orchestration and persistence checks
controlled: Go + Agent Runtime + RAG Runtime with the fake provider
networked: MySQL, Redis, Go, Agent Runtime, RAG Runtime, and fake provider in separate processes or containers
real-provider-canary: explicit opt-in, maximum 10 runs, concurrency 1, token ceiling recorded, never the sole merge gate
```

Require `ALLOW_REAL_PROVIDER_CANARY=1` for the canary profile and redact provider credentials from command output and reports.

Add:

```make
agent-performance-suite:
	./scripts/load/run-agent-performance-suite.sh
```

- [ ] **Step 5: Verify**

```bash
node --test scripts/load/agent-e2e-bench.test.mjs
bash -n scripts/load/run-agent-performance-suite.sh
node --check scripts/load/fake-agent-provider.mjs
```

Expected: all commands exit 0.

- [ ] **Step 6: Commit**

```bash
git add Makefile scripts/load/README.md scripts/load/agent-e2e-bench.mjs scripts/load/agent-e2e-bench.test.mjs scripts/load/fake-agent-provider.mjs scripts/load/run-agent-performance-suite.sh
git commit -m "test(perf): add end-to-end agent benchmark"
```

---

### Task 3: Correct Database Configuration, Logging, and Pool Metrics

**Repository:** `AllCallAll`

**Files:**

- Modify: `backend/internal/config/core.go:56-75`
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/internal/database/mysql.go`
- Modify: `backend/internal/database/mysql_test.go`
- Modify: `backend/internal/runtime/bootstrap.go`
- Modify: `backend/internal/runtime/bootstrap_test.go`
- Modify: `backend/internal/bootstrap/server.go`
- Modify: `backend/internal/bootstrap/server_test.go`
- Modify: `backend/cmd/agent-worker/main.go`
- Modify: `backend/configs/config.yaml`
- Modify: `backend/docs/configuration.md`

**Interfaces:**

- Adds: `DatabaseConfig.LogLevel string` from YAML `log_level` and env `DB_LOG_LEVEL`.
- Adds compatibility input for deprecated YAML `conn_max_lifetime_minutes`.
- Produces: `func ParseGORMLogLevel(raw string, production bool) gormlogger.LogLevel`.
- Consumes: `metrics.UpdateSQLDBStats("primary", sqlDB.Stats())` from Task 1.
- Consumes: `metrics.UpdateRedisPoolStats("primary", cache.SnapshotRedisPoolStats(redisClient))` from Task 1.
- Produces: `func StartSQLPoolMetrics(ctx context.Context, sqlDB *sql.DB, interval time.Duration)` and `func StartRedisPoolMetrics(ctx context.Context, redisClient *redis.Client, interval time.Duration)`; runtime/bootstrap owners supply lifecycle contexts.

- [ ] **Step 1: Write failing configuration tests**

```go
func TestDatabaseConfigAcceptsDeprecatedLifetimeMinutes(t *testing.T) {
	var cfg Config
	err := yaml.Unmarshal([]byte("database:\n  conn_max_lifetime_minutes: 30\n"), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.ApplyDefaults()
	if cfg.Database.ConnMaxLifetime != 30*time.Minute {
		t.Fatalf("lifetime=%s want=30m", cfg.Database.ConnMaxLifetime)
	}
}
```

Also assert that `conn_max_lifetime: 45m` wins when both keys exist and production defaults to `warn`.

Add cancellation tests for both samplers: use a 5 ms interval, cancel the owner context after at least one sample, and assert sampling exits without another observation. The tests must use injected observer callbacks or a test registry rather than global metric state.

- [ ] **Step 2: Run tests and verify failure**

```bash
cd backend && go test ./internal/config ./internal/database -count=1
```

- [ ] **Step 3: Implement compatibility and logger selection**

Add an auxiliary YAML field:

```go
type DatabaseConfig struct {
	DSN                       string        `yaml:"dsn" env:"DB_DSN"`
	MaxOpenConns              int           `yaml:"max_open_conns" env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConns              int           `yaml:"max_idle_conns" env:"DB_MAX_IDLE_CONNS"`
	ConnMaxLifetime           time.Duration `yaml:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME"`
	DeprecatedLifetimeMinutes int           `yaml:"conn_max_lifetime_minutes"`
	ConnMaxIdleTime           time.Duration `yaml:"conn_max_idle_time" env:"DB_CONN_MAX_IDLE_TIME"`
	LogLevel                  string        `yaml:"log_level" env:"DB_LOG_LEVEL"`
}
```

Apply the deprecated value only when `ConnMaxLifetime == 0`. Default to `warn` in production/beta and `info` in development.

Start cancellation-aware 15-second samplers from lifecycle owners. `runtime.OpenDB` creates a private sampler context after obtaining `sqlDB`, starts `StartSQLPoolMetrics`, and cancels it before closing the pool in the returned cleanup. `bootstrap.RunServer` starts `StartRedisPoolMetrics` with its root context after Redis construction. Move the Agent worker's signal context before Redis construction and start the Redis sampler there as well. On each tick, call `sqlDB.Stats()` or `redisClient.PoolStats()` through the Task 1 adapters. The samplers must not be started inside `database.NewMySQL` or `cache.NewRedis`.

- [ ] **Step 4: Correct configuration and documentation**

Use:

```yaml
conn_max_lifetime: 30m
conn_max_idle_time: 5m
log_level: warn
```

Document every pool value as per-process and include the cluster connection-budget equation from the specification.

- [ ] **Step 5: Verify**

```bash
cd backend && go test ./internal/config ./internal/database ./internal/cache ./internal/runtime -count=1
cd backend && go vet ./internal/config ./internal/database ./internal/cache ./internal/runtime
```

- [ ] **Step 6: Commit**

```bash
git add backend/internal/config/core.go backend/internal/config/config.go backend/internal/config/config_test.go backend/internal/database/mysql.go backend/internal/database/mysql_test.go backend/internal/cache/redis.go backend/internal/cache/redis_test.go backend/internal/runtime/bootstrap.go backend/internal/runtime/bootstrap_test.go backend/internal/bootstrap/server.go backend/internal/bootstrap/server_test.go backend/cmd/agent-worker/main.go backend/configs/config.yaml backend/docs/configuration.md
git commit -m "perf(database): correct pool configuration and logging"
```

---

### Task 4: Remove Workflow Result N+1 Queries

**Repository:** `AllCallAll`

**Files:**

- Create: `backend/internal/agent/workflow_result_batch.go`
- Modify: `backend/internal/agent/workflow.go:218-245`
- Modify: `backend/internal/agent/workflow_runtime.go:234-280`
- Modify: `backend/internal/agent/workflow_test.go`

**Interfaces:**

- Produces: `func (s *Service) buildWorkflowResults(ctx context.Context, runs []models.WorkflowRun) ([]WorkflowResult, error)`.
- Produces: `type workflowResultCollections struct { Tasks map[uint64][]models.WorkflowTask; Messages map[uint64][]models.AgentMessage; Approvals map[uint64][]models.ToolApproval; History map[uint64][]models.WorkflowHistoryEvent; Signals map[uint64][]models.WorkflowSignal; Timers map[uint64][]models.WorkflowTimer; Truncated map[uint64]bool }`.
- Produces: `func (s *Service) loadWorkflowResultCollections(ctx context.Context, runIDs []uint64) (workflowResultCollections, error)`.
- Produces: `func projectWorkflowResult(run models.WorkflowRun, collections workflowResultCollections) WorkflowResult`.
- Preserves: `buildWorkflowResult(ctx, run)` for single-run callers.
- Preserves collection ordering, `workflowResultMaxRows`, citations, action items, risk flags, and `Truncated` semantics.

- [ ] **Step 1: Write a failing query-count regression test**

Seed 50 runs with all six child collection types and count GORM query callbacks:

```go
func TestListWorkflowRunsUsesFixedQueryCount(t *testing.T) {
	svc, db := newWorkflowTestService(t)
	seedWorkflowRunsWithChildren(t, db, 50)
	counter := installQueryCounter(db)

	results, err := svc.ListWorkflowRuns(context.Background(), testOrgID, testUserID, WorkflowListFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 50 {
		t.Fatalf("results=%d want=50", len(results))
	}
	if got := counter.Count(); got > 8 {
		t.Fatalf("queries=%d want<=8", got)
	}
}
```

Add an equivalence test comparing one batched result with `buildWorkflowResult` for ordering and truncation.

- [ ] **Step 2: Run tests and verify failure**

```bash
cd backend && go test ./internal/agent -run 'TestListWorkflowRunsUsesFixedQueryCount|TestBuildWorkflowResultsMatchesSingle' -count=1
```

- [ ] **Step 3: Implement batched collection loading**

Collect run IDs, issue one query per child table using `WHERE workflow_run_id IN ?`, order by `workflow_run_id ASC, id DESC`, retain at most `workflowResultMaxRows` per run, reverse retained rows to the current ascending response order, and set truncation independently per run.

```go
func (s *Service) buildWorkflowResults(ctx context.Context, runs []models.WorkflowRun) ([]WorkflowResult, error) {
	if len(runs) == 0 {
		return []WorkflowResult{}, nil
	}
	runIDs := make([]uint64, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.ID)
	}
	collections, err := s.loadWorkflowResultCollections(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	results := make([]WorkflowResult, 0, len(runs))
	for _, run := range runs {
		results = append(results, projectWorkflowResult(run, collections))
	}
	return results, nil
}
```

`projectWorkflowResult` must contain the same citations, action-item, risk-flag, ordering, and truncation projection currently performed by `buildWorkflowResult`; make the single-run function call this shared projector so the equivalence test compares one implementation path rather than duplicated logic.

- [ ] **Step 4: Switch list callers and verify**

Replace the per-run loop in `ListWorkflowRuns`; keep single-run endpoints unchanged.

```bash
cd backend && go test ./internal/agent -run 'WorkflowResult|ListWorkflowRuns' -count=1
cd backend && go test ./internal/agent -count=1
```

Expected: 50-run loading stays at or below 8 SQL statements.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/agent/workflow.go backend/internal/agent/workflow_runtime.go backend/internal/agent/workflow_result_batch.go backend/internal/agent/workflow_test.go
git commit -m "perf(agent): batch workflow result loading"
```

---

### Task 5: Bound and Measure Agent Context Assembly

**Repository:** `AllCallAll`

**Files:**

- Create: `backend/internal/agent/context_budget.go`
- Create: `backend/internal/agent/context_budget_test.go`
- Create: `backend/internal/agent/context_repository.go`
- Create: `backend/internal/agent/context_repository_test.go`
- Modify: `backend/internal/agent/service.go:95-108`
- Modify: `backend/internal/agent/persistence.go:13-200`
- Modify: `backend/internal/agent/runtime_request_snapshot.go`
- Modify: `backend/internal/agent/workflow_external_contract.go`
- Modify: `backend/internal/agent/workflow_external_contract_test.go`
- Modify: `backend/internal/agent/service_test.go`

**Interfaces:**

- Produces: `type ContextBudget struct { Messages, Notes, Memories, Rooms, Followups, CallTranscriptSegments, MeetingTranscriptSegments, Chunks, MaxBytes, MaxEstimatedTokens int }`.
- Produces: `func ContextBudgetFromEnv() ContextBudget` with clamped safe defaults matching current record limits.
- Produces: `type ContextManifest struct { Selected map[string]int; Truncated []string; SerializedBytes int; EstimatedTokens int }`.
- Produces: `type contextRepository struct { db *gorm.DB }` and `func (r contextRepository) LoadBase(ctx context.Context, organizationID, userID, conversationID uint64, budget ContextBudget) (*conversationContext, int, error)`.
- Guarantees: `LoadBase` performs at most five SQL statements: `(1)` conversation plus optional contact profile, `(2)` messages, `(3)` members, `(4)` tagged supporting artifacts, and `(5)` tagged transcript/recording artifacts.
- Adds: `conversationContext.Manifest ContextManifest`.
- Adds: `conversationContext.ContactProfileLookupAttempted bool`.
- Adds optional request field `context_manifest`, preserving compatibility with older Python runtimes.

- [ ] **Step 1: Write failing budget tests**

```go
func TestContextBudgetFromEnvClampsValues(t *testing.T) {
	t.Setenv("AGENT_CONTEXT_MESSAGE_LIMIT", "100000")
	t.Setenv("AGENT_CONTEXT_MAX_BYTES", "1024")
	budget := ContextBudgetFromEnv()
	if budget.Messages != 200 {
		t.Fatalf("messages=%d want=200", budget.Messages)
	}
	if budget.MaxBytes != 64*1024 {
		t.Fatalf("bytes=%d want=%d", budget.MaxBytes, 64*1024)
	}
}
```

Add tests proving deterministic selection order, manifest truncation fields, and that `recordContextToolCalls` uses `conversationCtx.ContactProfile` without a second profile query.

In `context_repository_test.go`, seed every supported context type, install the existing GORM query callback counter, call `LoadBase`, and assert `queryCount <= 5`. Also assert the joined contact-profile state distinguishes `skipped`, `not_found`, and `found`, and that collection limits are applied in SQL rather than after unbounded loading.

- [ ] **Step 2: Run focused tests and verify failure**

```bash
cd backend && go test ./internal/agent -run 'ContextBudget|ContextRepository|ContextToolCallsReuseProfile|ContextManifest' -count=1
```

- [ ] **Step 3: Implement bounded selection and manifest construction**

Apply database limits before loading rows. Preserve deterministic ordering. Estimate tokens with the existing Agent estimation convention and serialize the final runtime request once to calculate actual bytes.

If the request exceeds its byte or token budget, trim in this order while retaining the goal and at least one current-message source when present:

```text
old meeting transcript segments
old call transcript segments
old messages
low-importance memories
old notes
low-score context chunks
```

Record every reduced collection in `ContextManifest.Truncated`.

- [ ] **Step 4: Implement the five-query base-context repository**

Use these fixed statement boundaries:

```text
1. conversations LEFT JOIN contact_profiles for the requested owner/contact
2. messages ordered newest-first with budget.Messages LIMIT
3. conversation_members ordered by id
4. one UNION ALL tagged artifact query for notes, memories, rooms, and followups
5. one UNION ALL tagged artifact query for call transcript segments,
   meeting transcript segments, and the latest recording-transcription status
```

The two tagged queries scan into private row envelopes and decode into typed model slices. Use subqueries against the already-bounded message/call IDs so followups and call transcripts do not add round trips. Keep `refreshConversationContextChunks` and `retrieveConversationContextChunks` outside `LoadBase`; they are the optional retrieval phase excluded from the five-query target.

Return the actual statement count with the context so Task 1 instrumentation records the enforced boundary. `loadConversationContext` must call `contextRepository.LoadBase`, then perform optional chunk refresh/retrieval.

- [ ] **Step 5: Remove duplicate contact-profile lookup**

Project tool-call output from `conversationCtx.ContactProfile`. Add a boolean `ContactProfileLookupAttempted` to distinguish `skipped`, `not_found`, and `found` without querying again.

- [ ] **Step 6: Add query and payload instrumentation**

Wrap context assembly with Task 1 metrics. Count actual database operations through a small internal counter incremented by the loading helpers; do not infer the count from populated collections.

- [ ] **Step 7: Verify**

```bash
cd backend && go test ./internal/agent -run 'Context|RuntimeRequest' -count=1
cd backend && go test ./internal/agent -count=1
```

Expected: base context stays at or below five SQL statements before optional retrieval, selected collections obey limits, manifests report truncation, and profile lookup is not repeated.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/agent/context_budget.go backend/internal/agent/context_budget_test.go backend/internal/agent/context_repository.go backend/internal/agent/context_repository_test.go backend/internal/agent/service.go backend/internal/agent/persistence.go backend/internal/agent/runtime_request_snapshot.go backend/internal/agent/workflow_external_contract.go backend/internal/agent/workflow_external_contract_test.go backend/internal/agent/service_test.go
git commit -m "perf(agent): bound conversation context payloads"
```

---

### Task 6: Continuously Drain Outbox and Isolate Event Failures

**Repository:** `AllCallAll`

**Files:**

- Modify: `backend/internal/events/processor.go`
- Modify: `backend/internal/events/processor_test.go`
- Modify: `backend/internal/events/processor_alert_test.go`
- Modify: `backend/internal/runtime/workers_starters.go:13-31`
- Modify: `backend/internal/runtime/workers_agent_recovery_test.go`

**Interfaces:**

- Produces: `type ProcessBatchResult struct { Claimed, Succeeded, Retried, Dead int; Errors []error }`.
- Produces: `func (p *Processor) ProcessBatch(ctx context.Context) (ProcessBatchResult, error)`.
- Produces: `func (p *Processor) Run(ctx context.Context, idleInterval time.Duration)` that immediately repeats while work is available.
- Preserves: `ProcessOnce(ctx) (int, error)` as a compatibility wrapper until existing callers migrate.

- [ ] **Step 1: Write failing failure-isolation test**

```go
func TestProcessorContinuesAfterOneEventFails(t *testing.T) {
	store, db := newProcessorTestStore(t)
	processor := NewProcessor(store)
	processor.WithRetry(3, time.Minute)
	processor.Register("test.event", func(_ context.Context, row models.EventOutbox) error {
		if row.AggregateID == 1 {
			return errors.New("poison")
		}
		return nil
	})
	seedProcessorEvents(t, store, 1, 2, 3)

	result, err := processor.ProcessBatch(context.Background())
	if err != nil {
		t.Fatalf("batch infrastructure error: %v", err)
	}
	if result.Retried != 1 || result.Succeeded != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertOutboxStatus(t, db, 2, models.EventOutboxStatusPublished)
	assertOutboxStatus(t, db, 3, models.EventOutboxStatusPublished)
}
```

- [ ] **Step 2: Write failing continuous-drain test**

Use batch size two with five events. Run the processor with a one-hour idle interval and assert all five complete without advancing the idle timer three times.

- [ ] **Step 3: Run tests and verify failures**

```bash
cd backend && go test ./internal/events -run 'TestProcessorContinuesAfterOneEventFails|TestProcessorDrainsBeforeIdleWait' -count=1
```

- [ ] **Step 4: Implement typed per-event outcomes**

`processEvent` returns an outcome after its state transition succeeds. Handler failures successfully persisted as retry or dead outcomes do not abort the batch. Claim failures and state-transition database failures remain processor errors.

- [ ] **Step 5: Implement continuous drain**

Use an idle timer rather than a fixed work ticker:

```go
for {
	result, err := p.ProcessBatch(ctx)
	if err != nil {
		p.recordRunFailure(err)
		if !waitForContextOrTimer(ctx, p.errorBackoff) {
			return
		}
		continue
	}
	if result.Claimed > 0 {
		continue
	}
	if !waitForContextOrTimer(ctx, idleInterval) {
		return
	}
}
```

- [ ] **Step 6: Move backlog sampling out of the hot loop**

Sample `CountPendingForEvents` from a separate ticker no more frequently than every 10 seconds by default. Preserve the existing `outbox_backlog` compatibility metric.

- [ ] **Step 7: Verify**

```bash
cd backend && go test ./internal/events ./internal/runtime -count=1
cd backend && go test -race ./internal/events -count=1
```

Expected: poison events no longer block later events, processing drains until empty, and cancellation exits cleanly.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/events/processor.go backend/internal/events/processor_test.go backend/internal/events/processor_alert_test.go backend/internal/runtime/workers_starters.go backend/internal/runtime/workers_agent_recovery_test.go
git commit -m "perf(outbox): continuously drain pending events"
```

---

### Task 7: Add Bounded Outbox Parallelism, Ordering, Leases, and Indexes

**Repository:** `AllCallAll`

**Files:**

- Modify: `backend/internal/events/processor.go`
- Modify: `backend/internal/events/outbox.go`
- Modify: `backend/internal/events/processor_test.go`
- Modify: `backend/internal/events/outbox_test.go`
- Modify: `backend/internal/runtime/workers_registration.go:338-351`
- Modify: `backend/internal/runtime/workers_starters.go`
- Create: `backend/migrations/000021_event_outbox_claim_indexes.up.sql`
- Create: `backend/migrations/000021_event_outbox_claim_indexes.down.sql`
- Create: `scripts/load/outbox-claim-explain.sql`
- Modify: `backend/internal/runtime/migrations_test.go`
- Modify: `infra/helm/allcallall/templates/configmap.yaml`
- Modify: `infra/helm/allcallall/values.yaml`

**Interfaces:**

- Produces: `type ProcessorConfig struct { BatchSize, Concurrency, QueueDepth int; IdleInterval, ErrorBackoff, Lease, LeaseRefresh time.Duration }`.
- Produces: `func (p *Processor) WithConfig(config ProcessorConfig) *Processor`.
- Produces: `func (s *Store) ExtendLease(ctx context.Context, id uint64, workerID string, until time.Time) (bool, error)`.
- Produces: `func (s *Store) MarkPublishedBatch(ctx context.Context, ids []uint64, workerID string) error`.
- Produces: `func (s *Store) MarkRetryBatch(ctx context.Context, ids []uint64, workerID string, cause error, availableAt time.Time) error`.
- Produces: `func (s *Store) MarkDeadBatch(ctx context.Context, ids []uint64, workerID string, cause error) error`.
- Produces: `func OrderingKey(row models.EventOutbox) string` using `aggregate_type:aggregate_id`.
- Adds environment inputs `OUTBOX_WORKER_CONCURRENCY`, `OUTBOX_WORKER_QUEUE_DEPTH`, `OUTBOX_WORKER_IDLE_MS`, `OUTBOX_WORKER_ERROR_BACKOFF_MS`, and `OUTBOX_WORKER_LEASE_REFRESH_SEC`.

- [ ] **Step 1: Write failing bounded-concurrency test**

Block handlers on a channel and assert maximum observed concurrency equals three and never exceeds it:

```go
processor.WithConfig(ProcessorConfig{
	BatchSize: 20,
	Concurrency: 3,
	QueueDepth: 6,
	Lease: time.Minute,
})
```

- [ ] **Step 2: Write failing aggregate-order, lease, and batch-state tests**

Seed interleaved events for two aggregates. Assert each aggregate completes in ascending outbox ID order while the aggregates overlap. Use a handler longer than the initial lease and assert a competing worker cannot reclaim the active row after refresh.

Seed rows sharing a target state and assert `MarkPublishedBatch`, `MarkRetryBatch`, and `MarkDeadBatch` each use one update, clear leases, and preserve the single-row methods' timestamps, attempts, error, and availability semantics. Add a mismatch test where one ID is no longer owned/eligible: the store must return the failed IDs or fall back to individual updates so per-event error visibility is retained.

- [ ] **Step 3: Run tests and verify failure**

```bash
cd backend && go test ./internal/events -run 'Concurrency|AggregateOrder|LeaseRefresh|BatchState' -count=1
```

- [ ] **Step 4: Implement sharded bounded dispatch**

Create `Concurrency` worker queues with total capacity `QueueDepth`. Hash `OrderingKey` to a shard. Each shard processes sequentially; different shards run concurrently.

For ordered Agent/workflow and result-write events, prevent cross-replica reordering by making a candidate ineligible while an earlier pending row exists for the same aggregate:

```sql
AND NOT EXISTS (
  SELECT 1
  FROM event_outbox earlier
  WHERE earlier.aggregate_type = candidate.aggregate_type
    AND earlier.aggregate_id = candidate.aggregate_id
    AND earlier.status = 'pending'
    AND earlier.id < candidate.id
)
```

Leave idempotent indexing events unordered for higher concurrency.

- [ ] **Step 5: Implement lease refresh**

Refresh only rows still owned by the current worker. If ownership is lost, cancel the handler with `context.WithCancelCause` and emit a lease-conflict metric. Stop refresh after completion and during shutdown.

- [ ] **Step 6: Batch compatible final-state writes**

After a dispatch wave completes, group successful IDs together; group retry IDs only when they share the exact `availableAt` and normalized error message; group dead IDs only when they share the exact normalized error message. Use the three batch methods for groups larger than one and retain single-row methods otherwise.

Each batch update must constrain both ID and current worker ownership. When `RowsAffected != len(ids)`, identify mismatched IDs with bounded per-ID updates and emit `outbox_final_state_update_error_total` with `state` restricted to `published`, `retry`, or `dead` for each failed row. Never hide an individual event's persistence failure merely to reduce SQL statements.

- [ ] **Step 7: Validate and add indexes**

`scripts/load/outbox-claim-explain.sql` must contain the representative pending-status/event distribution setup plus the exact claim and aggregate-order queries. Capture `EXPLAIN ANALYZE` before and after the migration in the benchmark report. Keep the column order below only if the post-migration plan uses the intended index without a full table scan; otherwise update the migration and test expectation to the measured order.

The up migration creates:

```sql
CREATE INDEX idx_event_outbox_claim
  ON event_outbox (status, event, available_at, locked_until, id);

CREATE INDEX idx_event_outbox_aggregate_order
  ON event_outbox (aggregate_type, aggregate_id, status, id);
```

The down migration drops those two indexes. Extend migration tests to assert exact column order.

- [ ] **Step 8: Configure safe defaults**

```text
agent-worker concurrency: 2
collaboration outbox concurrency: 8
search worker concurrency: 4
queue depth: 2x concurrency
idle poll: 500 ms
error backoff: 1 s
lease refresh: one third of lease duration
```

Clamp concurrency to `1..64` and queue depth to `concurrency..1024`.

- [ ] **Step 9: Verify**

```bash
cd backend && go test ./internal/events ./internal/runtime -count=1
cd backend && go test -race ./internal/events -count=1
cd backend && go test ./internal/runtime -run Migration -count=1
cd .. && helm template allcallall infra/helm/allcallall > /tmp/allcallall-performance-chart.yaml
rg -n "OUTBOX_WORKER_(CONCURRENCY|QUEUE_DEPTH|IDLE_MS|LEASE_REFRESH_SEC)" /tmp/allcallall-performance-chart.yaml
```

- [ ] **Step 10: Commit**

```bash
git add backend/internal/events/processor.go backend/internal/events/processor_test.go backend/internal/events/outbox.go backend/internal/events/outbox_test.go backend/internal/runtime/workers_registration.go backend/internal/runtime/workers_starters.go backend/internal/runtime/migrations_test.go backend/migrations/000021_event_outbox_claim_indexes.up.sql backend/migrations/000021_event_outbox_claim_indexes.down.sql scripts/load/outbox-claim-explain.sql infra/helm/allcallall/templates/configmap.yaml infra/helm/allcallall/values.yaml
git commit -m "perf(outbox): add bounded parallel processing"
```

---

### Task 8: Align Go Agent Runtime Deadlines and Overload Handling

**Repository:** `AllCallAll`

**Files:**

- Modify: `backend/internal/agent/workflow_external_runtime.go:275-430`
- Modify: `backend/internal/agent/workflow_external_runtime_test.go`
- Modify: `backend/internal/agent/workflow_external_contract.go`
- Modify: `backend/internal/agent/workflow_external_contract_test.go`
- Modify: `backend/internal/agent/lifecycle.go`
- Modify: `backend/internal/agent/workflow.go`
- Modify: `infra/helm/allcallall/templates/configmap.yaml`
- Modify: `infra/helm/allcallall/values.yaml`

**Interfaces:**

- Adds headers `X-AllCallAll-Deadline` as RFC3339Nano UTC and `X-AllCallAll-Attempt` as a positive integer.
- Produces: `type RuntimeTransportConfig struct { ConnectTimeout, ResponseHeaderTimeout, IdleConnTimeout, TotalTimeout time.Duration; MaxIdleConns, MaxIdleConnsPerHost, MaxConnsPerHost int }`.
- Produces: `func NewPythonLangGraphRuntime(baseURL string, client *http.Client) *PythonLangGraphRuntime`.
- Produces: `type RuntimeOverloadedError struct { RetryAfter time.Duration; Body string }` wrapping `ErrWorkflowRuntimeUnavailable`.

- [ ] **Step 1: Write failing deadline-header test**

```go
func TestPythonRuntimePropagatesAbsoluteDeadline(t *testing.T) {
	deadline := time.Now().UTC().Add(3 * time.Second).Truncate(time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := time.Parse(time.RFC3339Nano, r.Header.Get("X-AllCallAll-Deadline"))
		if err != nil || !got.Equal(deadline) {
			t.Fatalf("deadline=%q parsed=%v err=%v", r.Header.Get("X-AllCallAll-Deadline"), got, err)
		}
		writeReadyRuntimeResponse(t, w)
	}))
	defer server.Close()

	runtime := NewPythonLangGraphRuntime(server.URL, server.Client())
	_, err := runtime.RunAgent(ctx, minimalRuntimeRequest())
	if err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Write failing overload-classification test**

Return 429 and 503 with `Retry-After: 2`. Assert `errors.As(err, *RuntimeOverloadedError)` and a two-second retry hint.

- [ ] **Step 3: Run focused tests and verify failure**

```bash
cd backend && go test ./internal/agent -run 'PythonRuntimePropagates|RuntimeOverloaded' -count=1
```

- [ ] **Step 4: Implement explicit transport budgets**

```go
transport := &http.Transport{
	DialContext: (&net.Dialer{Timeout: cfg.ConnectTimeout, KeepAlive: 30 * time.Second}).DialContext,
	ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
	IdleConnTimeout: cfg.IdleConnTimeout,
	MaxIdleConns: cfg.MaxIdleConns,
	MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
	MaxConnsPerHost: cfg.MaxConnsPerHost,
}
```

Do not retry POST requests inside the transport. Durable retry remains owned by the execution and outbox layers.

- [ ] **Step 5: Propagate deadline and classify overload**

Set the deadline header only when the context has a deadline. Parse integer and HTTP-date `Retry-After` values. Treat 429 and capacity-related 503 responses as deferred execution, preserving attempts through the current defer path.

- [ ] **Step 6: Validate duration hierarchy**

```text
connect timeout < response-header timeout < Python request deadline
Python request deadline + cancellation grace < Go execution lease
outbox lease > Go execution lease + persistence grace
```

Fail startup with the conflicting environment variable names when the hierarchy is invalid.

- [ ] **Step 7: Verify**

```bash
cd backend && go test ./internal/agent ./internal/runtime -count=1
cd backend && go test -race ./internal/agent -run 'PythonLangGraphRuntime|RuntimeOverloaded' -count=1
```

- [ ] **Step 8: Commit**

```bash
git add backend/internal/agent/workflow_external_runtime.go backend/internal/agent/workflow_external_runtime_test.go backend/internal/agent/workflow_external_contract.go backend/internal/agent/workflow_external_contract_test.go backend/internal/agent/lifecycle.go backend/internal/agent/workflow.go infra/helm/allcallall/templates/configmap.yaml infra/helm/allcallall/values.yaml
git commit -m "perf(agent): align runtime deadlines and overload handling"
```

---

### Task 9: Add Python Metrics and Bounded Admission Control

**Repository:** `allcallall-agent-runtime`

**Files:**

- Modify: `services/agent-runtime/pyproject.toml`
- Modify: `services/agent-runtime/allcallall_agent_runtime/config.py`
- Replace: `services/agent-runtime/allcallall_agent_runtime/metrics.py`
- Create: `services/agent-runtime/allcallall_agent_runtime/admission.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/api/app.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/api/routes.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/orchestration/harness.py`
- Create: `services/agent-runtime/tests/test_metrics.py`
- Create: `services/agent-runtime/tests/test_admission.py`
- Modify: `services/agent-runtime/tests/test_api.py`
- Modify: `services/agent-runtime/tests/test_timeout_retry.py`
- Modify: `uv.lock`

**Interfaces:**

- Adds configuration `max_active_runs: int = 4`, `max_queued_runs: int = 16`, `max_queue_wait_seconds: float = 5.0`, and `cancellation_grace_seconds: float = 2.0`.
- Produces: `effective_max_active_runs(config: AgentRuntimeConfig) -> int`, bounded by the configured graph limit and the documented checkpoint/provider capacity model.
- Produces: `class AdmissionController` with `acquire(organization_id: int, deadline: float | None) -> AdmissionLease`.
- Produces: `class AdmissionRejected(RuntimeError)` with `reason` and `retry_after_seconds`.
- Produces: `AdmissionLease.close()` and context-manager support.
- Preserves `registry.counter(name, description).inc()` as a compatibility adapter while adding standard histograms and gauges.

- [ ] **Step 1: Add the metrics dependency**

Add `prometheus-client>=0.21,<1` to Agent Runtime dependencies and regenerate `uv.lock` using the repository's documented `uv` workflow.

- [ ] **Step 2: Write failing admission tests**

Use barriers rather than arbitrary sleeps for active-capacity assertions:

```python
def test_admission_rejects_when_active_and_queue_are_full() -> None:
    controller = AdmissionController(max_active=1, max_queued=1, max_queue_wait_seconds=0.05)
    active = controller.acquire(organization_id=1, deadline=None)

    queued = start_acquire(controller, organization_id=2)
    assert queued.waiting.wait(timeout=1)

    with pytest.raises(AdmissionRejected) as exc:
        controller.acquire(organization_id=3, deadline=None)
    assert exc.value.reason == "queue_full"
    assert exc.value.retry_after_seconds >= 1

    active.close()
    queued.join_and_close()
```

Add tests for FIFO order, queue timeout, queued cancellation, idempotent release, organization fairness, and no early capacity release when an HTTP caller times out.

Add configuration tests proving a MySQL checkpoint pool too small for the requested active-run limit either clamps to the documented effective capacity or fails startup with both conflicting setting names; it must never silently create an uncoordinated executor queue.

- [ ] **Step 3: Run tests and verify failure**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_admission.py tests/test_metrics.py -q
```

- [ ] **Step 4: Implement standard metrics**

Define bounded counters, gauges, and histograms for admission, workflow duration, node duration, dependency requests, checkpoint operations, payload bytes, retries, cancellation, and retrieval reuse. Inject a `CollectorRegistry` for tests and render with `generate_latest()`.

- [ ] **Step 5: Implement admission control**

Use `threading.Condition`, an explicit waiter deque, and monotonic deadlines. Allow exactly `max_active_runs` leases and at most `max_queued_runs` waiters. Queued cancellation removes its waiter before returning. `AdmissionLease.close()` is idempotent.

The graph execution owner releases the lease in `finally`; the HTTP timeout path only requests cancellation.

- [ ] **Step 6: Wire application and routes**

Store the controller on `application.state.admission`. Route dependencies retrieve it from `Request.app.state`. Map rejection to:

```python
raise HTTPException(
    status_code=503,
    detail={"code": "runtime_overloaded", "reason": exc.reason},
    headers={"Retry-After": str(exc.retry_after_seconds)},
)
```

Route all workflow entry points through one helper. Replace the fixed global 16-worker executor with an executor sized from `effective_max_active_runs(config)`, owned and closed by application lifespan. Log the configured active limit, effective active limit, queue limit, provider limit, and checkpoint pool size at startup.

- [ ] **Step 7: Verify**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_admission.py tests/test_metrics.py tests/test_api.py tests/test_timeout_retry.py -q
../../.venv/bin/python -m ruff check allcallall_agent_runtime tests
../../.venv/bin/python -m mypy allcallall_agent_runtime
```

- [ ] **Step 8: Commit in the Python repository**

```bash
git add services/agent-runtime/pyproject.toml services/agent-runtime/allcallall_agent_runtime/config.py services/agent-runtime/allcallall_agent_runtime/metrics.py services/agent-runtime/allcallall_agent_runtime/admission.py services/agent-runtime/allcallall_agent_runtime/api/app.py services/agent-runtime/allcallall_agent_runtime/api/routes.py services/agent-runtime/allcallall_agent_runtime/orchestration/harness.py services/agent-runtime/tests/test_metrics.py services/agent-runtime/tests/test_admission.py services/agent-runtime/tests/test_api.py services/agent-runtime/tests/test_timeout_retry.py uv.lock
git commit -m "perf(runtime): add bounded agent admission"
```

---

### Task 10: Propagate Deadlines, Add Cooperative Cancellation, and Bound Retries

**Repository:** `allcallall-agent-runtime`

**Files:**

- Create: `services/agent-runtime/allcallall_agent_runtime/deadline.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/api/routes.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/orchestration/harness.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/retry.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/state.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/dag.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/nodes/context.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/nodes/retrieval.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/nodes/synthesis.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/nodes/check.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/nodes/approval.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/nodes/role_router.py`
- Create: `services/agent-runtime/tests/test_retry_budget.py`
- Modify: `services/agent-runtime/tests/test_timeout_retry.py`
- Modify: `services/agent-runtime/tests/test_resilience.py`

**Interfaces:**

- Produces: `class ExecutionDeadline` with `from_header(value: str | None, default_seconds: float)`, `remaining_seconds()`, `expired()`, `cancel(reason)`, and `raise_if_cancelled()`.
- Produces: `class ExecutionCancelled(RuntimeError)` with a bounded reason code.
- Produces: `class RetryBudget` with `max_attempts`, `deadline`, `attempts_used`, and `next_delay(base, maximum) -> float | None`.
- Adds graph runtime context keys `execution_deadline` and `cancellation_token`; both are request-scoped and excluded from checkpoint serialization.

- [ ] **Step 1: Write failing deadline parsing tests**

Cover valid RFC3339 UTC, malformed input, already expired input, and local maximum clamping. An earlier caller deadline wins; a later caller deadline is capped by the local maximum.

- [ ] **Step 2: Write failing retry-budget tests**

```python
def test_retry_budget_stops_when_backoff_exceeds_remaining_time() -> None:
    clock = FakeClock(now=100.0)
    deadline = ExecutionDeadline(monotonic_deadline=100.4, clock=clock)
    budget = RetryBudget(max_attempts=3, deadline=deadline, jitter=lambda _: 0.0)

    assert budget.next_delay(base=0.1, maximum=1.0) == 0.1
    clock.advance(0.35)
    assert budget.next_delay(base=0.1, maximum=1.0) is None
```

Add a test proving provider and outer workflow retries consume the same budget rather than multiplying attempts.

- [ ] **Step 3: Run tests and verify failure**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_retry_budget.py tests/test_timeout_retry.py -q
```

- [ ] **Step 4: Implement deadline and cancellation objects**

Use `time.monotonic()` internally. Keep absolute UTC only for propagation and logs. Restrict cancellation reasons to `client_cancelled`, `deadline_exceeded`, `shutdown`, and `lease_lost`.

- [ ] **Step 5: Check cancellation around expensive operations**

Call `raise_if_cancelled()` before and after provider, Tool Bridge, RAG, and checkpoint operations and at every bounded loop iteration. Derive downstream timeouts from remaining budget.

- [ ] **Step 6: Share one retry budget**

Extend `with_retry` and `with_retry_async` with optional `budget: RetryBudget | None`. Preserve current keyword behavior when no budget is provided. Retry only classified transient failures and only when another attempt plus backoff fits inside the deadline.

- [ ] **Step 7: Track timed-out work until exit**

On HTTP timeout, request cancellation and return 504. Keep admission capacity and the inflight gauge occupied until the graph future exits. Record separate `cancel_requested`, `cancelled`, and `cancel_grace_exceeded` counters.

- [ ] **Step 8: Verify**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_retry_budget.py tests/test_timeout_retry.py tests/test_resilience.py -q
../../.venv/bin/python -m ruff check allcallall_agent_runtime tests
../../.venv/bin/python -m mypy allcallall_agent_runtime
```

- [ ] **Step 9: Commit in the Python repository**

```bash
git add services/agent-runtime/allcallall_agent_runtime/deadline.py services/agent-runtime/allcallall_agent_runtime/api/routes.py services/agent-runtime/allcallall_agent_runtime/orchestration/harness.py services/agent-runtime/allcallall_agent_runtime/retry.py services/agent-runtime/allcallall_agent_runtime/state.py services/agent-runtime/allcallall_agent_runtime/dag.py services/agent-runtime/allcallall_agent_runtime/nodes/context.py services/agent-runtime/allcallall_agent_runtime/nodes/retrieval.py services/agent-runtime/allcallall_agent_runtime/nodes/synthesis.py services/agent-runtime/allcallall_agent_runtime/nodes/check.py services/agent-runtime/allcallall_agent_runtime/nodes/approval.py services/agent-runtime/allcallall_agent_runtime/nodes/role_router.py services/agent-runtime/tests/test_retry_budget.py services/agent-runtime/tests/test_timeout_retry.py services/agent-runtime/tests/test_resilience.py
git commit -m "perf(runtime): propagate execution deadlines"
```

---

### Task 11: Reuse Python Outbound Connections Across Requests

**Repository:** `allcallall-agent-runtime`

**Files:**

- Create: `services/agent-runtime/allcallall_agent_runtime/clients.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/config.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/api/app.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/orchestration/harness.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/providers/base.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/providers/openai_compatible.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/tool_bridge.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/rag_runtime_client.py`
- Create: `services/agent-runtime/tests/test_clients.py`
- Modify: `services/agent-runtime/tests/test_harness_factory.py`
- Modify: `services/agent-runtime/tests/test_resilience.py`
- Create: `services/rag-runtime/allcallall_rag_runtime/clients.py`
- Modify: `services/rag-runtime/allcallall_rag_runtime/api.py`
- Modify: `services/rag-runtime/allcallall_rag_runtime/pipeline.py`
- Modify: `services/rag-runtime/allcallall_rag_runtime/go_bridge.py`
- Modify: `services/rag-runtime/allcallall_rag_runtime/qdrant_adapter.py`
- Modify: `services/rag-runtime/tests/test_rag_runtime.py`

**Interfaces:**

- Produces Agent: `RuntimeClients(provider: LLMProvider, tool_bridge: GoToolBridgeLayer, rag_runtime: RAGRuntimeClient)` with `close()`.
- Produces Agent: `build_runtime_clients(config: AgentRuntimeConfig, http_client: httpx.Client | None = None) -> RuntimeClients`.
- Produces RAG: `RAGClients(go_bridge: GoRetrievalBridge, qdrant: QdrantAdapter)` with `close()`.
- Produces RAG: `build_rag_clients(config: RAGRuntimeConfig, http_client: httpx.Client | None = None) -> RAGClients`.
- Client constructors accept injected `httpx.Client` and do not close injected clients.
- Adds configuration `http_max_connections`, `http_max_keepalive_connections`, `http_keepalive_expiry_sec`, `http_connect_timeout_sec`, `http_read_timeout_sec`, `http_write_timeout_sec`, and `http_pool_timeout_sec`.

- [ ] **Step 1: Write failing lifecycle tests**

```python
def test_runtime_clients_reuse_one_transport_across_runs() -> None:
    transport = CountingTransport(response_json=valid_provider_response())
    http = httpx.Client(transport=transport)
    clients = build_runtime_clients(test_config(), http_client=http)

    clients.provider.synthesize(request(), ["one"])
    clients.provider.synthesize(request(), ["two"])

    assert transport.client_instances == 1
    assert transport.requests == 2
```

Add shutdown tests proving owned clients close once and injected clients remain caller-owned.

- [ ] **Step 2: Run tests and verify failure**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_clients.py tests/test_harness_factory.py -q
cd ../rag-runtime
../../.venv/bin/python -m pytest tests/test_rag_runtime.py -q
```

- [ ] **Step 3: Implement process-lifetime clients**

```python
limits = httpx.Limits(
    max_connections=config.http_max_connections,
    max_keepalive_connections=config.http_max_keepalive_connections,
    keepalive_expiry=config.http_keepalive_expiry_sec,
)
timeout = httpx.Timeout(
    connect=config.http_connect_timeout_sec,
    read=config.http_read_timeout_sec,
    write=config.http_write_timeout_sec,
    pool=config.http_pool_timeout_sec,
)
```

FastAPI lifespan creates each client bundle, stores it on `app.state`, injects it into the harness or route dependency, and closes it during shutdown.

- [ ] **Step 4: Remove one-shot clients and preserve tenant isolation**

`GoRetrievalBridge.query` and `QdrantAdapter.query` call their injected client. `select_retrieval_chunks` accepts `RAGClients`. Agent retrieval nodes receive the process-owned `RAGRuntimeClient`. Authorization headers and tenant payload fields are rebuilt per request and never stored on the shared transport.

- [ ] **Step 5: Verify**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_clients.py tests/test_harness_factory.py tests/test_tool_layer.py tests/test_resilience.py -q
../../.venv/bin/python -m ruff check allcallall_agent_runtime tests
../../.venv/bin/python -m mypy allcallall_agent_runtime

cd ../rag-runtime
../../.venv/bin/python -m pytest tests/test_rag_runtime.py -q
../../.venv/bin/python -m ruff check allcallall_rag_runtime tests
../../.venv/bin/python -m mypy allcallall_rag_runtime
```

- [ ] **Step 6: Commit in the Python repository**

```bash
git add services/agent-runtime/allcallall_agent_runtime/clients.py services/agent-runtime/allcallall_agent_runtime/config.py services/agent-runtime/allcallall_agent_runtime/api/app.py services/agent-runtime/allcallall_agent_runtime/orchestration/harness.py services/agent-runtime/allcallall_agent_runtime/providers/base.py services/agent-runtime/allcallall_agent_runtime/providers/openai_compatible.py services/agent-runtime/allcallall_agent_runtime/tool_bridge.py services/agent-runtime/allcallall_agent_runtime/rag_runtime_client.py services/agent-runtime/tests/test_clients.py services/agent-runtime/tests/test_harness_factory.py services/agent-runtime/tests/test_resilience.py services/rag-runtime/allcallall_rag_runtime/clients.py services/rag-runtime/allcallall_rag_runtime/api.py services/rag-runtime/allcallall_rag_runtime/pipeline.py services/rag-runtime/allcallall_rag_runtime/go_bridge.py services/rag-runtime/allcallall_rag_runtime/qdrant_adapter.py services/rag-runtime/tests/test_rag_runtime.py
git commit -m "perf(runtime): reuse outbound HTTP connections"
```

---

### Task 12: Remove Duplicate Retrieval and Slim Checkpoint State

**Repository:** `allcallall-agent-runtime`

**Files:**

- Modify: `services/agent-runtime/allcallall_agent_runtime/models/retrieval.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/state.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/nodes/retrieval.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/dag.py`
- Create: `services/agent-runtime/allcallall_agent_runtime/checkpoint/payload.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/checkpoint/mysql.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/checkpoint/sqlite_saver.py`
- Create: `services/agent-runtime/tests/test_checkpoint_payload.py`
- Modify: `services/agent-runtime/tests/test_mysql_checkpoint.py`
- Modify: `services/agent-runtime/tests/test_optimization_modules.py`
- Modify: `services/rag-runtime/allcallall_rag_runtime/models.py`
- Modify: `services/rag-runtime/allcallall_rag_runtime/pipeline.py`
- Modify: `services/rag-runtime/allcallall_rag_runtime/retrieval.py`
- Modify: `services/rag-runtime/tests/test_rag_runtime.py`

**Interfaces:**

- Adds: `RetrievalMode = Literal["go_context", "rag_runtime", "hybrid"]`.
- Adds optional request fields `retrieval_mode`, `context_fingerprint`, and `corpus_version`.
- Produces: `class RunRetrievalCache` keyed by normalized query, source scope, retrieval policy, context fingerprint, and corpus version.
- Produces: `prepare_candidates(query, chunks, source_types) -> PreparedCandidates`.
- Produces: `rerank_prepared(prepared, top_k) -> RerankResponse`.
- Produces: `project_checkpoint_state(state: Mapping[str, Any]) -> dict[str, Any]`.
- Produces: `serialized_checkpoint_size(payload: Mapping[str, Any]) -> int`.

- [ ] **Step 1: Write failing retrieval ownership tests**

```python
def test_go_context_mode_does_not_call_rag_runtime() -> None:
    client = RecordingRAGClient()
    result = retrieve_for_step(request(retrieval_mode="go_context"), plan(), client, RunRetrievalCache())
    assert client.calls == 0
    assert result.chunks == request_context_chunks()

def test_hybrid_mode_reuses_identical_query() -> None:
    client = RecordingRAGClient()
    cache = RunRetrievalCache(max_entries=16)
    first = retrieve_for_step(request(retrieval_mode="hybrid"), plan(), client, cache)
    second = retrieve_for_step(request(retrieval_mode="hybrid"), plan(), client, cache)
    assert first == second
    assert client.calls == 1
```

Add a RAG test proving final evidence construction does not rerank unchanged prepared candidates twice.

- [ ] **Step 2: Write failing checkpoint projection tests**

Create representative state with request-scoped clients, cancellation token, complete trace, large evidence text, role outputs, approvals, and resume identifiers. Assert that projection removes runtime objects, retains resume state, replaces large evidence bodies with references and hashes, remains deserializable, and produces deterministic bytes.

Add a compatibility fixture written by the pre-change serializer and prove the new saver can resume it. If the projected representation cannot be read by the previous runtime, keep the old write shape behind a compatibility flag until active old checkpoints drain; do not silently make rollback unsafe.

- [ ] **Step 3: Run tests and verify failure**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_checkpoint_payload.py tests/test_mysql_checkpoint.py tests/test_optimization_modules.py -q
cd ../rag-runtime
../../.venv/bin/python -m pytest tests/test_rag_runtime.py -q
```

- [ ] **Step 4: Implement retrieval ownership and per-run reuse**

Default absent mode to `hybrid`. In `go_context`, use supplied chunks without calling RAG. In `hybrid`, call RAG only when context sufficiency is below threshold or the plan requires an absent source type. Bound the cache to one execution, 16 entries, and the request's chunk budget.

- [ ] **Step 5: Prepare and rerank candidates once**

Normalize, filter, deduplicate, and tokenize into `PreparedCandidates`. Reuse scores when query, source scope, policy, and corpus version are unchanged. Apply cheap top-N reduction before expensive reranking.

- [ ] **Step 6: Implement checkpoint projection**

Project state immediately before serialization. Keep execution ID, checkpoint version, pending approvals, selected evidence IDs, loop position, and compact role outputs. Exclude clients, cancellation objects, duplicated context, complete provider bodies, and complete trace payloads. Emit original and projected byte histograms while retaining the 16 MiB hard transaction limit.

- [ ] **Step 7: Verify**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_checkpoint_payload.py tests/test_mysql_checkpoint.py tests/test_optimization_modules.py -q
../../.venv/bin/python -m allcallall_agent_runtime.eval_runner --out /tmp/allcallall-agent-eval

cd ../rag-runtime
../../.venv/bin/python -m pytest tests/test_rag_runtime.py -q
../../.venv/bin/python -m allcallall_rag_runtime.eval_runner --out /tmp/allcallall-rag-eval
```

- [ ] **Step 8: Commit in the Python repository**

```bash
git add services/agent-runtime/allcallall_agent_runtime/models/retrieval.py services/agent-runtime/allcallall_agent_runtime/state.py services/agent-runtime/allcallall_agent_runtime/nodes/retrieval.py services/agent-runtime/allcallall_agent_runtime/dag.py services/agent-runtime/allcallall_agent_runtime/checkpoint/payload.py services/agent-runtime/allcallall_agent_runtime/checkpoint/mysql.py services/agent-runtime/allcallall_agent_runtime/checkpoint/sqlite_saver.py services/agent-runtime/tests/test_checkpoint_payload.py services/agent-runtime/tests/test_mysql_checkpoint.py services/agent-runtime/tests/test_optimization_modules.py services/rag-runtime/allcallall_rag_runtime/models.py services/rag-runtime/allcallall_rag_runtime/pipeline.py services/rag-runtime/allcallall_rag_runtime/retrieval.py services/rag-runtime/tests/test_rag_runtime.py
git commit -m "perf(ai): reduce retrieval and checkpoint amplification"
```

---

### Task 13: Evaluation-Gated Routing, Early Termination, and Role Parallelism

**Repository:** `allcallall-agent-runtime`

**Files:**

- Modify: `services/agent-runtime/allcallall_agent_runtime/config.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/dag.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/state.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/metrics.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/eval_runner.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/nodes/role_router.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/nodes/synthesis.py`
- Create: `services/agent-runtime/allcallall_agent_runtime/nodes/parallel_roles.py`
- Create: `services/agent-runtime/tests/test_parallel_roles.py`
- Modify: `services/agent-runtime/tests/test_optimization_modules.py`
- Modify: `services/agent-runtime/tests/test_check_agents.py`
- Modify: `services/agent-runtime/evals/cases.json`

**Interfaces:**

- Adds flags `enable_role_router`, `enable_early_termination`, and `enable_parallel_roles`; all remain `false` by default.
- Produces: `EarlyTerminationThresholds(evidence_sufficiency: float, citation_coverage: float, goal_coverage: float)`.
- Produces: `should_terminate_early(*, evidence_sufficiency: float, citation_coverage: float, goal_coverage: float, required_roles_complete: bool, unresolved_approval: bool, safety_blocked: bool, thresholds: EarlyTerminationThresholds) -> bool`.
- Produces: `class RoleDelta(TypedDict)` with `role`, branch-local `trace_events`, one `role_result`, `citations`, `action_items`, and `risk_flags`.
- Produces: `execute_parallel_roles(state: GraphState, roles: Sequence[str], *, max_parallel: int, token_budget: int) -> list[RoleDelta]`.
- Produces: `merge_role_deltas(deltas: Sequence[RoleDelta], canonical_order: Sequence[str]) -> dict[str, Any]` with deterministic role and citation ordering.
- Adds bounded metrics for selected roles, early-termination decisions, parallel-group duration, cancellation, and partial failure.

- [ ] **Step 1: Write failing routing and early-termination gate tests**

Add cases proving a context-only request may skip memory and risk work, while risk intent, write proposals, unresolved approvals, or safety policy always retain `risk_analyst`. Assert early termination is rejected unless evidence sufficiency, citation coverage, and goal coverage all meet configured thresholds.

```python
def test_risk_policy_prevents_early_termination() -> None:
    thresholds = EarlyTerminationThresholds(0.8, 0.8, 0.8)
    assert should_terminate_early(
        evidence_sufficiency=1.0,
        citation_coverage=1.0,
        goal_coverage=1.0,
        required_roles_complete=True,
        unresolved_approval=True,
        safety_blocked=False,
        thresholds=thresholds,
    ) is False
```

- [ ] **Step 2: Write failing controlled-parallelism tests**

Use blocking fake roles to prove only `searcher` and `memory_agent` overlap, `synthesize` waits for both, and maximum role concurrency is two. Assert branch inputs are immutable snapshots, no branch writes checkpoint state, and merged results always follow canonical role order regardless of completion order.

Add cancellation and partial-failure cases: cancellation stops all branches; one branch failure cancels siblings and returns a classified error without committing a partial checkpoint; token/provider reservations exceeding the fixed group budget force sequential execution.

- [ ] **Step 3: Run focused tests and verify failure**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_parallel_roles.py tests/test_optimization_modules.py -q
```

- [ ] **Step 4: Gate role routing and early termination**

Keep both features disabled unless their independent flags are true. Extend evaluation fixtures with skippable-role and mandatory-risk cases. `should_terminate_early` must require all quality thresholds and reject termination when approval, safety, or required-role work remains.

- [ ] **Step 5: Implement bounded independent-role execution**

Only the `searcher` and `memory_agent` pair is initially eligible. Copy the read-only request/context view into each branch, reserve the group's provider-call and token budget before dispatch, cap concurrency at two, and collect branch-local `RoleDelta` values. Do not pass checkpoint writers or mutable shared result lists into branches.

Merge deltas in `searcher`, `memory_agent`, `synthesize`, `risk_analyst` order. On cancellation or partial failure, cancel outstanding futures, discard all unmerged deltas, release reservations once, and let the existing retry/deadline policy classify the run. Do not enable parallel execution merely because `RoleAllocation.parallel_groups` contains multiple roles.

- [ ] **Step 6: Run the correctness and evaluation gates**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_parallel_roles.py tests/test_optimization_modules.py tests/test_check_agents.py tests/test_meeting_brief.py tests/test_resilience.py -q
../../.venv/bin/python -m allcallall_agent_runtime.eval_runner --out /tmp/allcallall-agent-role-eval
../../.venv/bin/python -m ruff check allcallall_agent_runtime tests
../../.venv/bin/python -m mypy allcallall_agent_runtime
```

Expected: routing and early termination meet existing quality/safety thresholds; the parallel flag remains off by default; enabling it changes latency only for the eligible independent pair and does not change deterministic outputs.

- [ ] **Step 7: Commit in the Python repository**

```bash
git add services/agent-runtime/allcallall_agent_runtime/config.py services/agent-runtime/allcallall_agent_runtime/dag.py services/agent-runtime/allcallall_agent_runtime/state.py services/agent-runtime/allcallall_agent_runtime/metrics.py services/agent-runtime/allcallall_agent_runtime/eval_runner.py services/agent-runtime/allcallall_agent_runtime/nodes/role_router.py services/agent-runtime/allcallall_agent_runtime/nodes/synthesis.py services/agent-runtime/allcallall_agent_runtime/nodes/parallel_roles.py services/agent-runtime/tests/test_parallel_roles.py services/agent-runtime/tests/test_optimization_modules.py services/agent-runtime/tests/test_check_agents.py services/agent-runtime/evals/cases.json
git commit -m "perf(ai): gate role routing and parallel execution"
```

---

### Task 14: Consolidate Write Ownership and Add Pressure-Based Scaling

**Repositories:** `allcallall-agent-runtime`, then `AllCallAll`

**Files in `allcallall-agent-runtime`:**

- Modify: `services/agent-runtime/allcallall_agent_runtime/config.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/api/app.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/api/routes.py`
- Modify: `services/agent-runtime/allcallall_agent_runtime/orchestration/harness.py`
- Modify: `services/agent-runtime/tests/test_tool_queue_integration.py`
- Modify: `services/agent-runtime/tests/test_api.py`

**Files in `AllCallAll`:**

- Modify: `backend/internal/agent/workflow_external_contract.go`
- Modify: `backend/internal/agent/workflow_external_contract_test.go`
- Modify: `backend/internal/agent/external_tool_approval.go`
- Modify: `backend/internal/agent/agent_resume.go`
- Modify: `backend/internal/agent/agent_resume_test.go`
- Modify: `backend/internal/agent/workflow_approval.go`
- Modify: `backend/internal/agent/workflow_test.go`
- Modify: `backend/internal/agent/approved_local_tools.go`
- Modify: `backend/internal/agent/mcp_deferred_execution_test.go`
- Modify: `backend/internal/runtime/workers_registration.go`
- Modify: `infra/helm/allcallall/values.yaml`
- Modify: `infra/helm/allcallall/templates/configmap.yaml`
- Modify: `infra/helm/allcallall/templates/autoscaling.yaml`
- Create: `docs/reference/performance.md`
- Modify: `scripts/load/README.md`

**Interfaces:**

- Python responses continue returning approved write proposals; Python does not execute them asynchronously in production multi-replica mode.
- Go enqueues approved writes with stable outbox idempotency keys.
- Helm values add optional external metrics for backlog age, queue depth, and inflight saturation while retaining CPU fallback.

- [ ] **Step 1: Write failing Python production-safety tests**

Assert startup rejects `PY_AGENT_ENABLE_TOOL_QUEUE=true` with `PY_AGENT_DEPLOYMENT_MODE=multi_replica`, while workflow responses still contain approved proposals for Go execution.

- [ ] **Step 2: Restrict the Python in-memory queue**

Retain it for explicit single-process development and deterministic tests. Report the effective mode in `/ready` and `/v1/capabilities`.

- [ ] **Step 3: Verify and commit Python changes**

```bash
cd services/agent-runtime
../../.venv/bin/python -m pytest tests/test_tool_queue_integration.py tests/test_api.py -q
../../.venv/bin/python -m ruff check allcallall_agent_runtime tests
../../.venv/bin/python -m mypy allcallall_agent_runtime

git add services/agent-runtime/allcallall_agent_runtime/config.py services/agent-runtime/allcallall_agent_runtime/api/app.py services/agent-runtime/allcallall_agent_runtime/api/routes.py services/agent-runtime/allcallall_agent_runtime/orchestration/harness.py services/agent-runtime/tests/test_tool_queue_integration.py services/agent-runtime/tests/test_api.py
git commit -m "fix(runtime): keep durable write execution in Go"
```

- [ ] **Step 4: Write failing Go idempotency tests**

For both Agent and Workflow approval paths, submit the same approved proposal twice with identical execution, checkpoint version, and tool-call ID. Assert one durable resume event and one product write. Submit two distinct proposal IDs and assert both are retained. Re-run the existing MCP deferred-execution tests to prove in-progress executions remain resumable.

- [ ] **Step 5: Route approved writes through Go outbox**

Use the current approval transaction so approval state and write-event enqueue are atomic. Build the idempotency key from execution ID, checkpoint version, and tool-call ID.

- [ ] **Step 6: Add optional pressure-based HPA values**

Extend autoscaling blocks with an empty default:

```yaml
externalMetrics: []
```

Render external metrics only when configured. Provide this production example:

```yaml
components:
  outboxWorker:
    autoscaling:
      externalMetrics:
        - name: allcallall_outbox_oldest_pending_seconds
          targetAverageValue: "2"
  agentRuntime:
    autoscaling:
      externalMetrics:
        - name: allcallall_agent_admission_queue_depth
          targetAverageValue: "4"
```

Keep CPU metrics enabled and document the Prometheus custom metrics adapter requirement.

Add conservative scale-down stabilization windows and maximum replica values. Validate the documented worst-case MySQL, Redis, provider, and Qdrant connection totals at those maxima before enabling external metrics.

- [ ] **Step 7: Write the performance operations reference**

Document the four load profiles, metric names, alert thresholds, pool sizing, overload interpretation, cancellation behavior, checkpoint warnings, rollback signals, and this event-stream adoption gate:

```text
optimized outbox cannot drain 2x measured peak ingress
oldest pending age breaches the SLO during sustained load
MySQL CPU or lock wait is the measured limiting resource
additional outbox replicas no longer improve drain rate materially
the operational cost of a new broker is documented and accepted
```

- [ ] **Step 8: Verify and commit Go/deployment changes**

```bash
cd backend && go test ./internal/agent ./internal/events ./internal/runtime -count=1
cd .. && helm template allcallall infra/helm/allcallall > /tmp/allcallall-performance-chart.yaml
npm run test:docs
npm run docs:check

git add backend/internal/agent/workflow_external_contract.go backend/internal/agent/workflow_external_contract_test.go backend/internal/agent/external_tool_approval.go backend/internal/agent/agent_resume.go backend/internal/agent/agent_resume_test.go backend/internal/agent/workflow_approval.go backend/internal/agent/workflow_test.go backend/internal/agent/approved_local_tools.go backend/internal/agent/mcp_deferred_execution_test.go backend/internal/runtime/workers_registration.go infra/helm/allcallall/values.yaml infra/helm/allcallall/templates/configmap.yaml infra/helm/allcallall/templates/autoscaling.yaml docs/reference/performance.md scripts/load/README.md
git commit -m "perf(platform): scale agent workloads from queue pressure"
```

---

## Final Integration Verification

Run this after all task commits exist in their respective worktrees.

### `AllCallAll`

- [ ] **Run focused race and migration checks**

```bash
cd backend
go test -race ./internal/events ./internal/agent ./internal/database ./internal/runtime -count=1
go test ./internal/runtime -run Migration -count=1
```

- [ ] **Run the complete repository gate**

```bash
cd /Users/byzantium/github/AllCallAll-backend-ai-performance
make verify-full
```

- [ ] **Render deployment configuration**

```bash
helm lint infra/helm/allcallall
helm template allcallall infra/helm/allcallall > /tmp/allcallall-performance-chart.yaml
```

### `allcallall-agent-runtime`

- [ ] **Run the complete runtime gate and image builds**

```bash
cd /Users/byzantium/github/allcallall-agent-runtime-performance
make verify
make docker-build
```

### End-to-end evidence

- [ ] **Run deterministic orchestration regression**

```bash
cd /Users/byzantium/github/AllCallAll-backend-ai-performance/backend
go run ./cmd/interview-bench -conversations 25 -batch-size 50
```

Expected: 25 ready, 0 failed, 0 pending outbox events, with no unexplained regression from the recorded baseline.

- [ ] **Run controlled-provider sustained and burst profiles**

```bash
cd /Users/byzantium/github/AllCallAll-backend-ai-performance
RUNS=200 CONCURRENCY=20 FAKE_PROVIDER_LATENCY_MS=250 make agent-performance-suite
RUNS=200 CONCURRENCY=50 FAKE_PROVIDER_LATENCY_MS=1000 make agent-performance-suite
```

Expected:

- enqueue p95 below 250 ms at target load;
- outbox queue-wait p95 below 2 seconds at steady load;
- bounded 429/503 responses during overload instead of unbounded timeouts;
- complete backlog drain after the burst;
- timed-out work remains accounted for until cancellation or completion;
- no duplicate product writes;
- no checkpoint transaction exceeds the hard limit;
- Workflow list query count stays at or below 8;
- database and HTTP pools avoid sustained saturation.

- [ ] **Compare quality and safety evaluations**

Require no regression in:

```text
task completion
citation grounding
approval safety
tool intent
unsupported-claim detection
iteration caps
checkpoint resume
idempotent write execution
```

- [ ] **Review the event-stream decision gate**

Record sustained ingress, drain rate, oldest-event age, MySQL CPU, lock wait, and scaling efficiency. Keep MySQL Outbox when it meets the target. Open a separate architecture proposal only when every Task 14 gate is satisfied.

## Execution Order

- Tasks 1 through 8 are committed in `AllCallAll`.
- Tasks 9 through 13 and the first half of Task 14 are committed in `allcallall-agent-runtime`.
- The second half of Task 14 returns to `AllCallAll` after Python response behavior is verified.
- Task 1 precedes optimization work because later tasks consume its metrics.
- Task 6 precedes Task 7 because failure isolation and continuous draining simplify bounded dispatch.
- Task 9 precedes Tasks 10 and 11 because admission owns execution lifetime and metrics.
- Task 11 precedes Task 12 because retrieval reuse depends on injected process-lifetime clients.
- Task 12 precedes Task 13 because role decisions consume the deduplicated retrieval and compact checkpoint state.
- Early termination, dynamic routing, role parallelism, and custom HPA metrics remain disabled by default until final evidence is reviewed.
