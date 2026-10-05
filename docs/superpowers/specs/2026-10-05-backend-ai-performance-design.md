# Backend and AI Performance Design

**Date:** 2026-10-05

**Status:** Approved design; awaiting implementation plan

**Repositories:** `AllCallAll` and sibling `allcallall-agent-runtime`
**Scope:** Go API and workers, Python Agent Runtime, Python RAG Runtime, MySQL/Redis integration, and Kubernetes scaling signals

## Summary

AllCallAll already has several sound performance foundations: a transactional
MySQL outbox with `SKIP LOCKED`, idempotent Agent execution leases, bounded Go
asynchronous work, reusable database pools, Prometheus and OTLP integration,
and deterministic Agent evaluations. The main scaling risk is not a single
slow function. It is the interaction between polling, serial event processing,
unbounded or mismatched concurrency, repeated context retrieval, layered
retries, and incomplete operational signals across the Go-to-Python execution
path.

This design improves the existing architecture incrementally. It keeps MySQL as
the durable product store and retains the current Go/Python ownership boundary.
The first delivery establishes trustworthy end-to-end measurements. Later
deliveries make outbox processing continuously draining and bounded, eliminate
known query amplification, introduce admission control and cooperative
cancellation in Python, reuse network clients across requests, reduce duplicate
retrieval and checkpoint state, and scale workloads from queue pressure rather
than CPU alone.

A dedicated event-streaming platform is deliberately deferred. It becomes an
option only if measured traffic exceeds the optimized MySQL outbox's sustainable
capacity or requires ordering and replay properties that the current store
cannot provide economically.

## Goals

1. Make queue wait, execution time, external-call time, database pressure, and
   payload growth visible across the complete Agent path.
2. Remove avoidable polling delay and head-of-line blocking from Go outbox
   processing while preserving idempotency and per-aggregate ordering.
3. Bound Python Agent concurrency and reject overload predictably instead of
   allowing timed-out work to consume the service indefinitely.
4. Eliminate the known Workflow result N+1 query pattern and reduce Agent
   context database round trips and serialization volume.
5. Reuse HTTP connections to the model provider, Go Tool Bridge, RAG Runtime,
   and Qdrant across requests.
6. Establish one deadline and retry budget for each Agent execution.
7. Remove duplicate retrieval and repeated RAG processing while preserving
   grounding, citation, approval, and safety behavior.
8. Reduce checkpoint write volume to the minimum state required for correct
   recovery.
9. Scale workers and runtimes from workload pressure signals in addition to
   CPU utilization.
10. Keep each change independently testable, observable, reversible, and
    practical for a public open-source deployment.

## Non-goals

- Replacing MySQL, Redis, Gin, FastAPI, LangGraph, or the current provider
  abstraction in the first implementation cycle.
- Moving authoritative product writes, permissions, approvals, or audit data
  from Go into Python.
- Introducing Kafka, NATS, Redis Streams, Celery, or another queue before the
  optimized outbox has been measured under representative load.
- Making independent Agent roles parallel before their state, tool, cost, and
  ordering boundaries are proven safe.
- Treating deterministic rules-engine benchmarks as evidence of real model or
  production RAG capacity.
- Adding caches without an explicit owner, invalidation policy, memory bound,
  and measured benefit.
- Trading correctness, citation quality, approval enforcement, isolation, or
  recoverability for lower latency.

## System Boundary

The existing ownership model remains authoritative:

- Go owns product data, permissions, conversations, meetings, transcripts,
  approvals, audit logs, durable event delivery, and write execution.
- Python Agent Runtime owns workflow orchestration, model calls, role routing,
  tool proposals, trace construction, and checkpoint-driven recovery.
- Python RAG Runtime owns retrieval planning, candidate processing, reranking,
  evidence construction, and grounding support.
- MySQL remains the source of truth for product and checkpoint persistence.
- Redis remains an acceleration and coordination dependency, not the owner of
  irreplaceable product state.

The target request path is:

```text
API request
  -> bounded Go context assembly
  -> durable outbox enqueue
  -> continuously draining workload-specific worker
  -> bounded Python admission queue
  -> deadline-aware Agent graph
  -> one owned retrieval path
  -> bounded checkpoint persistence
  -> idempotent Go result/write execution
```

## Current Baseline

### Revisions inspected

- `AllCallAll`: `df05b01b`
- `allcallall-agent-runtime`: `f8cd660`

Both repositories were inspected on their clean `main` branches. The existing
untracked `.playwright-mcp/` directory in `AllCallAll` is unrelated and is not
part of this design.

### Deterministic local benchmark

The following current-revision command completed successfully:

```bash
cd backend
go run ./cmd/interview-bench -conversations 25 -batch-size 50
```

Observed result on 2026-10-05:

| Metric | Result |
| --- | ---: |
| Conversations | 25 |
| Ready / failed runs | 25 / 0 |
| Processed events | 75 |
| Total duration | 392 ms |
| Queue latency p95 | 2 ms |
| Execute-run latency p95 | 9 ms |

This benchmark uses the deterministic rules provider and local test storage. It
is useful as a regression check for Go orchestration and persistence behavior,
but it does not exercise networked MySQL, the Python services, an external LLM,
Qdrant, multi-hop retrieval, or production concurrency.

Historical load snapshots are retained only as context. They must not be used
as current capacity claims because they measured different revisions and, for
Agent creation, measured enqueue rather than end-to-end completion.

## Current Bottlenecks

### Outbox polling and serial execution

The Go outbox worker defaults to a 30-second interval. Each tick claims one
batch and processes the claimed events serially. A handler error returns from
the batch early, so unrelated later events wait for another attempt. Slow Agent
or workflow work can therefore delay short events when they share a processor.

With the default batch size of 100, the polling cadence alone provides a
nominal claim rate of about 3.3 events per second per worker before handler time
is included. A two-minute lease can also become too short for events near the
end of a slow serial batch. Execution fencing protects correctness, but stale
claims still waste work and produce contention.

The claim query has individual indexes but no composite index aligned with its
status, event, availability, lock, and ordering predicates. Each final state is
also updated separately, and every processing cycle performs a backlog count.

### Agent context amplification

Go context construction performs approximately 9 to 11 sequential database
queries for conversation data, notes, messages, rooms, memories, members,
follow-ups, transcripts, recording transcription, and contact profile. Context
chunk refresh and retrieval add more work. Contact profile lookup is repeated
while recording context tool calls.

The resulting context is serialized into the run's request JSON, transferred to
Python, and may later enter checkpoint state. This couples latency and storage
growth to conversation history unless every collection has a deliberate bound.

### Workflow result N+1 queries

Listing up to 50 workflow runs loads the runs first and then builds each result
with separate tasks, messages, approvals, history, signals, and timers queries.
The worst case is approximately 301 SQL statements for one list request.

Individual Agent run materialization also performs separate steps and tool-call
queries. These patterns increase database round trips and reduce predictability
as page size grows.

### Database logging and configuration drift

GORM is configured at `Info` level unconditionally, causing every SQL statement
to be logged in production. This adds formatting, allocation, and log I/O to
database-heavy paths.

The checked-in database configuration uses `conn_max_lifetime_minutes`, while
the Go configuration field expects `conn_max_lifetime`. The intended 30-minute
setting is therefore not reliably applied and the code falls back to its
default.

The Go MySQL and Redis pools have explicit size defaults, but those defaults are
not currently derived from pod count, worker concurrency, database capacity, or
per-request query demand.

### Go-to-Python deadline mismatch

Go reuses a single HTTP client for Agent Runtime calls, which is a good base.
The current client uses one total timeout and buffers the complete response up
to 4 MiB. It has no separate connect, response-header, or idle-body budgets and
no client-side concurrency limiter.

The Go client timeout, Python request timeout, Go execution lease, checkpoint
lease, and retry delays do not form one coherent deadline hierarchy. A lower
layer may continue work after an upper layer has already abandoned the request.

### Python thread saturation and incomplete cancellation

Agent routes are synchronous and execute through Starlette's thread handling.
The harness then submits blocking LangGraph work to a second fixed
`ThreadPoolExecutor` with 16 workers. A timeout from `future.result()` ends the
wait but cannot stop the running graph invocation. Timed-out work may continue
to hold an executor thread, checkpoint connection, model request, and retrieval
request.

There is no explicit admission queue bound, queue-wait timeout, inflight gauge,
or overload response. A burst of slow or timed-out work can therefore exhaust
the executor while incoming HTTP requests continue to accumulate.

The checkpoint pool defaults to four connections per saver, which is not
coordinated with the 16 graph workers. This creates an implicit second queue
inside persistence.

### Short-lived outbound clients

The Agent provider, Tool Bridge, and RAG client can be constructed during each
run or node execution. RAG's Go bridge creates and closes an `httpx.Client` per
query, while the Qdrant adapter uses one-shot module calls. These patterns lose
cross-request TCP/TLS reuse and make connection pressure difficult to budget.

### Serial and duplicate AI work

The default graph keeps role routing and early termination disabled. Its normal
path executes search, memory, synthesis, and risk analysis serially. The role
router records potential parallel groups but does not currently execute them in
parallel.

Retrieval can occur in Go before the Agent call, in Agent retrieval nodes, and
again through RAG Runtime calling back into Go. Search and risk roles may also
make overlapping tool requests. Agentic retrieval and role loops each allow
multiple sequential iterations.

Within RAG processing, candidate filtering, tokenization, ordering, and
reranking may be repeated between iterative retrieval and final evidence-pack
construction. Multi-layer retries use blocking backoff and may multiply across
Go, Agent Runtime, RAG Runtime, provider, and tool calls.

### Checkpoint state growth

The MySQL checkpointer is bounded and transactional, but asynchronous methods
delegate synchronous PyMySQL work to threads. A graph execution buffers its
checkpoint writes before commit, with an explicit 16 MiB transaction limit.
The existence of a dedicated oversized-checkpoint error and legacy fallback
shows that state growth is already an operational concern.

Graph state currently includes context, role results, traces, retrieval
artifacts, and other data that may be serialized repeatedly. Not all of this is
required to resume execution correctly.

### Metrics and autoscaling gaps

Go exposes standard HTTP duration histograms and runtime metrics, plus a second
custom counter store. It does not yet expose the queue, database-pool, external
dependency, and payload-size metrics required to locate Agent bottlenecks.

The Python services expose process-local custom counters only. They have no
histograms or gauges for queue wait, inflight work, node duration, checkpoint
pool pressure, checkpoint bytes, provider latency, token usage, or RAG stages.

Kubernetes HPAs use CPU utilization alone. CPU does not reveal a blocked model
request, exhausted executor, checkpoint connection wait, or growing outbox.
The Helm runtime template also annotates RAG Runtime for Prometheus scraping but
not Agent Runtime.

## Design Principles

1. **Measure before tuning.** Optimization claims require comparable end-to-end
   evidence from the same workload and revision.
2. **Bound every queue.** HTTP admission, outbox claims, worker pools, model
   calls, retrieval, and database access all need explicit capacity and overload
   behavior.
3. **One owner per retry.** Retries are budgeted at the layer that can classify
   the failure and remain within the caller's deadline.
4. **One owner per retrieval.** A run may combine multiple sources, but it must
   not repeat the same retrieval path merely because data crossed a service
   boundary.
5. **Preserve ordering narrowly.** Ordering is enforced for the aggregate that
   requires it, not by serializing unrelated work.
6. **Persist recovery state, not observability state.** Large traces and
   evidence belong in dedicated storage when they are not needed for resume.
7. **Reject overload explicitly.** A fast, observable rejection is safer than
   hidden queue growth and eventual timeout.
8. **Scale from pressure.** Queue age and saturation are better leading signals
   than CPU for network-bound AI workloads.
9. **Optimize incrementally.** Every phase has a feature flag, compatibility
   path, or direct rollback strategy.

## Target Service-Level Indicators

These are initial engineering targets, not current production claims. The P0
baseline may refine the numeric values before they become release gates.

| Area | Initial target |
| --- | --- |
| Agent enqueue API | p95 below 250 ms, error rate below 0.5% at target load |
| Outbox queue wait | p95 below 2 s during steady load |
| Outbox recovery | drain at least 2x the measured peak ingress rate |
| Oldest pending event | below 5 s during normal operation |
| Workflow list query count | no more than 8 SQL statements for 50 runs |
| Agent context query count | no more than 5 database round trips before optional retrieval |
| Python admission queue | bounded; no unbounded growth |
| Timed-out Agent work | no continued untracked execution beyond the cancellation grace period |
| Checkpoint pool | p95 acquisition wait below 50 ms under target load |
| Checkpoint payload | explicit warning and rejection thresholds, with p95 tracked by workflow |
| Availability under overload | controlled 429/503 responses; no process-wide executor starvation |
| Correctness | no regression in grounding, citations, approval safety, idempotency, or recovery evals |

Model completion latency depends on provider and model choice. The project will
therefore report queue wait, orchestration overhead, provider time, retrieval
time, and end-to-end time separately instead of publishing one misleading
aggregate target.

## Performance Measurement Architecture

### Shared correlation

Every Agent execution carries a stable run ID and trace/correlation ID across:

- API enqueue;
- outbox event and worker claim;
- Go context loading;
- Agent Runtime HTTP request;
- LangGraph nodes;
- RAG Runtime requests;
- provider and Tool Bridge calls;
- checkpoint transactions;
- result persistence and write execution.

Metrics use bounded labels such as service, operation, workflow preset, event
class, result, and dependency. User IDs, organization IDs, conversation IDs,
run IDs, free-form tool names, prompts, and exception strings must not become
Prometheus labels.

### Go metrics

Add histograms and gauges for:

- outbox queue age, claim size, batch duration, event duration, retry delay,
  lease extension, lease conflict, and backlog by bounded event class;
- worker inflight, configured concurrency, rejected submissions, and queue
  depth;
- context assembly total time, query count, selected record count, serialized
  bytes, and token estimate;
- Workflow and Agent result materialization query count and duration;
- Agent Runtime request connect, header, body, and total duration;
- MySQL `sql.DB` open, in-use, idle, wait count, wait duration, and connection
  close statistics;
- Redis pool active, idle, wait, timeout, and error statistics where exposed by
  the selected client.

Standard Prometheus metrics should be the long-term source of truth. Existing
custom counters remain temporarily for dashboard compatibility and are removed
only after their consumers migrate.

### Python metrics

Replace or extend counter-only registries with standard Prometheus-compatible
histograms and gauges for:

- admission inflight, queue depth, queue wait, queue timeout, rejection, and
  cancellation outcome;
- workflow and node duration by bounded node category;
- provider request duration, TTFT where streaming supports it, attempts,
  timeout, status class, input tokens, and output tokens;
- Tool Bridge and RAG client pool usage, request duration, attempts, and
  failures;
- checkpoint connection acquisition, transaction duration, write count,
  serialized bytes, and oversized rejection;
- RAG bridge, Qdrant, filtering, reranking, evidence construction, candidate
  count, and evidence count;
- early-termination decisions, selected roles, loop iterations, and retrieval
  reuse.

Agent Runtime and RAG Runtime must both be scraped in Kubernetes.

### Load profiles

The performance suite has four distinct profiles:

1. **Deterministic orchestration:** no external model; detects application and
   persistence regressions.
2. **Controlled fake-provider E2E:** configurable latency, failure, timeout,
   token size, and response size; validates overload and retry behavior.
3. **Networked integration:** MySQL, Redis, Go, Agent Runtime, RAG Runtime, and
   a deterministic provider in separate processes or containers.
4. **Real-provider canary:** small, cost-bounded sample for realistic provider
   latency and token behavior; never used as the sole merge gate.

Each report records commit SHAs, hardware or pod resources, replica counts,
configuration, dataset size, concurrency, duration, latency percentiles,
throughput, error classes, token usage, database pressure, and queue depth.

## Go Data-Path Design

### Continuously draining outbox

The worker runs a drain loop rather than processing only one batch per long
polling interval:

```text
wait for wake-up or short idle poll
  -> claim bounded batch
  -> dispatch through bounded workload pool
  -> persist individual outcomes
  -> immediately claim next batch while work exists
  -> return to idle wait when empty
```

Enqueue operations may optionally emit an in-process wake-up hint. Correctness
must never depend on the hint because another process may enqueue the event; a
short fallback poll remains required.

### Workload isolation

Events are assigned to a small, documented set of workload classes, for
example:

- Agent and workflow execution;
- short product and notification events;
- search and indexing;
- transcription and media processing.

Each class has an independent concurrency budget. A slow Agent request cannot
consume every worker slot needed by short events. Configuration remains simple
enough for an open-source single-node deployment, with safe defaults and a
single-worker compatibility mode.

### Ordering and idempotency

Events for the same aggregate or run are dispatched through the same ordering
key when their handlers require ordering. Different keys may run concurrently.
Existing idempotency, execution leases, checkpoint versions, and result fencing
remain the correctness boundary.

A handler failure is recorded for that event and does not prevent unrelated
events from completing. Batch processing returns an aggregate error only for
observability and process health, not as a reason to abandon unprocessed rows.

### Lease behavior

The claimed work window must be short enough that all claimed events can begin
within their lease, or the processor must extend leases for queued and active
work. Lease duration is derived from execution timeout plus a bounded grace
period, not from batch size alone.

Shutdown stops new claims, drains active work within the termination budget,
and leaves unfinished leases to expire safely.

### Outbox indexing and writes

Add and verify a composite index aligned with the final MySQL claim query. The
exact column order is chosen from `EXPLAIN ANALYZE` against representative
status and event distributions rather than assumed from source inspection.

Final-state writes may be batched when rows share the same target state and
metadata. Correctness and per-event error visibility take priority over reducing
the number of updates.

Backlog counts used for metrics must not add a full count query to every hot
processing iteration. They may be sampled, maintained from transitions, or
collected by a separate low-frequency observer.

### Batched result loading

Workflow list materialization loads each child table once for all selected run
IDs and groups rows in memory. Stable ordering must match current response
semantics. The design target is one query for runs and one per child collection,
independent of page size.

Agent run result loading may use a small fixed number of queries or joins where
cardinality cannot multiply rows unexpectedly.

### Bounded context assembly

Context inputs receive explicit independent limits for messages, notes,
memories, follow-ups, transcript segments, and generated chunks. Selection is
performed in the database with useful ordering and indexes instead of loading
unbounded history and trimming afterward.

Queries that share the same key and lifetime are merged or executed
concurrently only when doing so reduces latency without exceeding the database
connection budget. Repeated contact-profile lookup is removed.

The Go-to-Python contract carries a context manifest containing selected counts,
estimated tokens, truncation indicators, and source identifiers. This makes
context loss explicit and measurable.

### Database and logging configuration

GORM log level becomes environment-configurable and defaults to warning or
error in production. Slow-query logging remains available with duration and
sampling controls.

The connection lifetime configuration key is corrected with compatibility for
the previous spelling during one deprecation window. Startup logs the effective
pool settings without exposing credentials.

Pool defaults are documented as per-process values. Deployment guidance checks
that:

```text
sum(max open connections across pods)
  <= database connection budget after operational reserve
```

## Python Runtime Design

### Admission control

Each Agent Runtime process owns one explicit admission controller with:

- maximum active graph executions;
- maximum queued executions;
- maximum queue-wait duration;
- per-organization and optionally per-user fairness limits;
- overload responses with `Retry-After`;
- metrics for active, queued, rejected, cancelled, and timed-out executions.

The graph concurrency default is derived from CPU, memory, provider limits, and
checkpoint pool capacity. It is not silently fixed at 16.

The HTTP layer performs one thread handoff for blocking graph work, not nested
uncoordinated scheduling. A future async graph implementation may remove the
handoff, but an async route alone does not make blocking provider or database
calls non-blocking.

### Deadline propagation and cancellation

Go computes an absolute execution deadline and sends it to Agent Runtime. Python
derives every node, provider, tool, retrieval, and checkpoint timeout from the
remaining budget.

Graph state carries a lightweight cancellation/deadline token. Nodes check it
before starting expensive work and between bounded loop iterations. Client
disconnect, Go cancellation, deadline expiration, and shutdown request
cancellation through the same mechanism.

Because a Python thread cannot be safely killed, cooperative cancellation is
the first implementation. Calls that cannot be interrupted are tracked until
they finish and do not free an admission slot prematurely. If provider libraries
or arbitrary tools remain non-cancellable, a later isolated process execution
mode may enforce hard termination.

### Shared outbound clients

FastAPI lifespan constructs and closes process-wide, thread-safe clients for:

- model providers;
- Go Tool Bridge;
- RAG Runtime;
- Qdrant and other retrieval backends.

Clients receive explicit connection-pool limits, connect timeout, read timeout,
write timeout, pool-acquisition timeout, keep-alive expiry, and DNS/TLS reuse.
The harness and graph nodes receive these clients through dependency injection.

Authentication and tenant context remain request-scoped even when the transport
client is process-scoped.

### Retry budget

Each execution has a maximum attempt and time budget. A layer retries only
failures that it can classify as transient and only when enough deadline remains
for the backoff and another attempt.

The default ownership is:

- transport clients retry safe connection failures and selected idempotent
  status codes;
- graph nodes decide whether a semantically different tool or retrieval attempt
  is useful;
- Go outbox retries the durable overall execution after the Python request has
  failed definitively.

Nested layers communicate attempt metadata so one request cannot accidentally
receive the full retry count at every boundary. Backoff uses bounded exponential
jitter and releases scarce concurrency resources where practical.

### Checkpoint capacity and state

Checkpoint connection capacity is configured alongside admission concurrency.
Acquisition has a timeout and emits wait metrics rather than blocking invisibly.

Checkpoint state is classified as:

- required to resume correctly;
- useful for debugging but externally storable;
- reproducible and therefore unnecessary to persist;
- request-scoped dependency that must be re-injected.

Large evidence payloads, complete traces, provider responses, and repeated
context copies move out of the checkpoint when they are not required for
resume. Checkpoint serialization records byte size before database work and
rejects oversized state with an actionable classification.

Transaction batching remains, but the implementation avoids holding a database
connection while waiting on a provider or tool.

### Write-tool queue ownership

The Python in-memory write-tool queue is not a production durability boundary.
Approved product writes should ultimately be represented in the Go durable
outbox, where multi-replica claiming, leases, retries, idempotency, and dead
letters already exist.

Until migration is complete, the Python queue remains explicitly
single-process/development-only or is disabled in multi-replica production.

## Retrieval and Graph Optimization

### Single retrieval owner

Every workflow declares its retrieval mode:

- `go_context`: bounded product context supplied by Go is sufficient;
- `rag_runtime`: RAG Runtime owns iterative retrieval and evidence construction;
- `hybrid`: both are required, with source IDs and query fingerprints used to
  prevent duplicate retrieval.

Python does not call RAG merely because the service is configured. RAG does not
call back into Go for a query whose bounded source set is already present.

### Per-run retrieval cache

A bounded, request-scoped cache keys normalized query, source scope, retrieval
policy, and corpus version. It may reuse:

- normalized query tokens;
- candidate filtering results;
- source fetches;
- reranker features and scores;
- evidence pack fragments.

It must not cross tenants or survive beyond the run unless a separate
tenant-safe cache design is approved.

### Candidate reduction

Cheap filtering and deduplication happen before expensive reranking. Candidate
and evidence limits are explicit. Final evidence construction reuses existing
scores instead of reranking the same candidate set without a changed query or
policy.

### Routing and early termination

Role routing and early termination are enabled behind independent feature
flags. Rollout is gated by deterministic evaluations and replayed production
traces with sensitive content removed.

Early termination requires sufficient evidence, citation coverage, goal
coverage, and no unresolved safety or approval condition. Role routing must
retain the risk role for workflows where policy requires it, even if the router
predicts that the role is unnecessary.

### Controlled parallelism

Parallel roles are a later optimization. They may execute concurrently only
when:

- neither depends on the other's output;
- they do not mutate the same checkpoint fields or product state;
- their combined provider and token budget is bounded;
- deterministic merge ordering is defined;
- cancellation and partial failure behavior are tested.

Parallelism is not enabled merely because `parallel_groups` contains multiple
roles.

## Deployment and Autoscaling

CPU-based HPA remains as a safety signal. Additional scaling signals are:

| Component | Primary pressure signals |
| --- | --- |
| Outbox Worker | pending count, oldest pending age, claim-to-complete rate |
| Agent Worker | active executions, execution queue age, lease contention |
| Agent Runtime | admission queue depth, queue wait, inflight / capacity |
| RAG Runtime | inflight queries, queue wait, retrieval stage duration |
| API | request rate, latency, CPU, connection count |

Custom-metric scaling must use stabilization windows and conservative scale-down
so a temporary provider slowdown does not create replica oscillation. Maximum
replica counts are validated against MySQL, Redis, provider, and Qdrant
connection and rate budgets.

The Python container initially remains one Uvicorn process per pod. Kubernetes
replicas provide process isolation and simpler memory accounting. Multiple
Uvicorn workers may be evaluated later, but only with adjusted checkpoint,
HTTP, and database pool budgets because every process creates its own pools.

## Delivery Phases

### Phase 0: Measurement and safety rails

- Add Go and Python queue, pool, dependency, payload, and stage metrics.
- Scrape both Python runtimes in Helm.
- Add deterministic-provider, networked E2E, burst, sustained-load, timeout,
  cancellation, and recovery profiles.
- Record the first reproducible performance report.
- Define dashboards and alerts for backlog age, saturation, timeout leakage,
  checkpoint growth, and dependency latency.

No throughput optimization is accepted before this phase can show whether it
helped and whether it shifted pressure elsewhere.

### Phase 1: Low-risk Go improvements

- Correct database lifetime configuration with compatibility handling.
- Make GORM production log level configurable.
- Batch Workflow result loading and remove repeated profile lookup.
- Add bounded context selection and payload measurements.
- Add the outbox composite index after `EXPLAIN ANALYZE` verification.
- Change the outbox to continuous drain with per-event failure isolation.

### Phase 2: Bounded concurrency and network reuse

- Introduce outbox workload pools and per-aggregate ordering.
- Add lease extension or bounded claim-window behavior.
- Introduce Python admission control and checkpoint-pool coordination.
- Use lifespan-managed HTTP clients and explicit transport limits.
- Align deadlines and implement cooperative cancellation.
- Establish one retry budget across the execution.

### Phase 3: AI work reduction

- Add retrieval ownership and per-run reuse.
- Remove repeated tokenization and reranking.
- Slim checkpoint state and externalize non-resume trace payloads.
- Roll out role routing and early termination behind evaluation gates.
- Evaluate safe independent-role parallelism only after the serial path meets
  correctness and observability requirements.

### Phase 4: Pressure-based scaling and architecture decision

- Add queue and saturation metrics to autoscaling.
- Tune replica and pool budgets from measured capacity.
- Consolidate approved write execution onto the Go durable outbox.
- Run sustained and failure-injection tests at expected peak and 2x peak.
- Evaluate a dedicated event stream only if the optimized outbox fails the
  documented capacity or operational requirements.

## Validation Strategy

### Correctness gates

All existing merge-blocking checks remain required. Performance changes also
add focused tests for:

- no duplicate or reordered effects under concurrent outbox processing;
- one event failure not preventing unrelated batch completion;
- lease expiration, renewal, shutdown, and re-claim behavior;
- Workflow list response equivalence after batched loading;
- context truncation indicators and deterministic selection order;
- admission queue bounds, fairness, overload response, and cancellation;
- shared-client startup, shutdown, authentication isolation, and pool limits;
- retry-budget propagation and prevention of nested retry multiplication;
- checkpoint resume compatibility after state slimming;
- retrieval reuse without cross-tenant or stale-corpus leakage;
- routing and early-termination evaluation thresholds.

### Performance gates

Each optimization records before/after evidence from the same test profile.
Reports include p50, p95, p99, throughput, error rate, queue wait, resource use,
database queries, database wait, network attempts, token use, and checkpoint
bytes.

A change fails performance review when it improves average latency but causes
unbounded queue growth, materially worsens p99, increases duplicate external
calls, reduces correctness metrics, or hides overload behind timeouts.

### Required repository verification

For `AllCallAll`:

```bash
make verify-full
```

Focused backend changes additionally run relevant package tests, migration
tests, outbox concurrency tests, and the deterministic Agent benchmark.

For `allcallall-agent-runtime`:

```bash
make verify
```

Focused Python changes additionally run Agent and RAG evaluation suites,
checkpoint integration tests, and the new concurrency/load profiles.

## Rollout and Rollback

- New outbox concurrency starts at one worker per class, then increases through
  configuration after metrics are stable.
- Workload routing, early termination, retrieval ownership, and state slimming
  use independent flags or compatibility modes.
- Schema changes are additive first. Old code must tolerate the new index and
  metadata during rolling deployment.
- New checkpoint state remains readable by the previous runtime during the
  compatibility window, or deployment is blocked until active old checkpoints
  drain.
- Admission limits launch conservatively and expose rejection metrics before
  HPA begins using them.
- Every rollout defines a rollback signal: duplicate effects, lease conflicts,
  queue-age growth, provider error growth, correctness regression, checkpoint
  incompatibility, or database saturation.

## Open-Source Operational Requirements

- Safe defaults must work on a single-node development deployment without
  requiring a message broker or custom metrics adapter.
- Advanced concurrency, custom autoscaling, and external storage options are
  documented as optional production profiles.
- Environment variables have explicit units, defaults, allowed ranges, and
  interaction notes.
- Performance reports include reproducible commands and do not present local
  synthetic results as universal capacity claims.
- New dependencies require a clear maintenance benefit and must not duplicate
  functionality already available in the selected standard libraries.

## Recommended Implementation Sequence

The implementation plan should divide the work into independently reviewable
changes in this order:

1. Cross-service metrics and load-test foundation.
2. Runtime scrape, GORM logging, and database configuration corrections.
3. Workflow N+1 and Agent context query reduction.
4. Continuous outbox drain and per-event error isolation.
5. Workload pools, ordering keys, lease behavior, and claim indexing.
6. Python admission control and checkpoint capacity coordination.
7. Shared clients, deadline propagation, cancellation, and retry budget.
8. Retrieval ownership, RAG reuse, and checkpoint state reduction.
9. Evaluation-gated routing and early termination.
10. Pressure-based autoscaling and final queue-architecture decision.

This sequence produces measurable improvements early while delaying the most
behavior-sensitive graph and infrastructure changes until the system can
observe and safely roll them back.
