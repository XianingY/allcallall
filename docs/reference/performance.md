# Performance Operations Reference

This page is the operations guide for measuring, scaling, and rolling back the
Agent/RAG performance work. It is not a benchmark report. Before promoting a
change, record its profile, environment, and before/after evidence in the
corresponding load artifacts.

## Load profiles

| Profile | Purpose | Runs / concurrency | Merge meaning |
|---|---|---:|---|
| `baseline` | Deterministic in-process orchestration and persistence checks with a mock provider. | 4 / 2 | Proves benchmark correctness and detects lifecycle regressions without external dependencies. |
| `controlled` | Go backend, Agent Runtime, and RAG Runtime with the deterministic fake provider. | 10 / 2 | Proves the Go → Agent Runtime → RAG Runtime → provider path and metric collection. |
| `networked` | The controlled stack with separate MySQL and Redis processes and topology validation. | 10 / 2 | Proves the full production-like dependency graph. |
| `real-provider-canary` | A real provider under strict safety limits. | ≤10 / 1 | Proves provider integration and records token usage. It is never the sole merge gate. |

Run the suite with `make agent-performance-suite`. Set `PROFILE`, `BASE_URL`,
`TOKEN`, `ORGANIZATION_ID`, and `CONVERSATION_ID` as described in
[`scripts/load/README.md`](../../scripts/load/README.md).

## Pressure metrics

### Go backend

The standard Prometheus endpoint is `:9090/metrics`. The business endpoint on
`:8080/api/v1/metrics` continues to expose compatibility counters such as
`outbox_backlog`, `outbox_publish_total`,
`outbox_publish_retry_total`, and `outbox_publish_failed_total`.

| Metric | Type | Interpretation |
|---|---|---|
| `outbox_queue_wait_seconds` | Histogram | Time from event creation to processing claim, by bounded `work_class`. Rising p95 means the outbox is the bottleneck. |
| `outbox_event_duration_seconds` | Histogram | Handler duration by bounded `work_class` and `outcome`. Rising duration with level backlog points at a slow handler or dependency. |
| `outbox_inflight` | Gauge | Events currently being processed. Compare against configured worker concurrency. |
| `outbox_backlog` | Gauge | Compatibility pending-event count sampled by each worker. Useful for coarse alarms, not sub-second queue-age control. |
| `agent_context_query_duration_seconds` | Histogram | Context assembly latency. Growth often indicates unbounded context selection rather than CPU starvation. |
| `agent_context_entries` / `agent_context_chunks` | Histograms | Context selection volume. |
| `agent_context_window_used_tokens` ÷ `agent_context_window_max_tokens` | Gauge ratio | Context saturation. |
| `dependency_request_duration_seconds` | Histogram | Outbound dependency latency by service, operation, and status. |
| `sql_db_connections` | Gauge | Open, in-use, and idle MySQL connections by bounded pool. |
| `sql_db_wait_count_total` | Counter | MySQL pool acquisition waits. |
| `redis_pool_connections` | Gauge | Redis pool connection states. |
| `redis_pool_wait_total` | Counter | Redis pool acquisitions that had to wait. |
| `redis_pool_timeouts_total` | Counter | Redis pool acquisitions that timed out. |

### Agent Runtime

The Agent Runtime exposes standard Prometheus metrics at `/metrics`.

| Metric | Type | Interpretation |
|---|---|---|
| `agent_runtime_admission_active_runs` | Gauge | Workflow runs holding an admission lease. |
| `agent_runtime_admission_queued_runs` | Gauge | Runs waiting for a lease. This is the queue-depth pressure signal. |
| `agent_runtime_admission_queue_wait_seconds` | Histogram | Wait before admission. |
| `agent_runtime_admission_rejected_total` | Counter | Bounded admission rejections (`queue_full`, `timeout`). |
| `agent_runtime_workflow_duration_seconds` | Histogram | End-to-end workflow duration. |
| `agent_runtime_workflow_runs_total` / `agent_runtime_workflow_failures_total` | Counters | Throughput and failures. |
| `agent_runtime_node_duration_seconds` | Histogram | Bounded graph-node latency. |
| `agent_runtime_dependency_request_total` / `_errors_total` / `_duration_seconds` | Metrics | Provider, RAG, and Tool Bridge pressure. |
| `agent_runtime_checkpoint_operation_total` / `_errors_total` | Counters | Checkpoint get/put/list activity and failures. |
| `checkpoint_payload_bytes` | Histogram | Original and projected checkpoint payload sizes. |
| `agent_runtime_retry_total` | Counter | Bounded retry attempts by dependency. |
| `agent_runtime_cancel_requested_total` | Counter | Cooperative cancellation requests by bounded reason. |
| `agent_runtime_cancelled_total` | Counter | Runs that exited cooperatively after cancellation. |
| `agent_runtime_cancel_grace_exceeded_total` | Counter | Runs that did not exit within the cancellation grace period. |
| `agent_runtime_retrieval_reuse_total` | Counter | Retrieval results served from request-scoped reuse. |

### RAG Runtime

Use `rag_runtime_query_total`, `rag_runtime_prepare_total`,
`rag_runtime_rerank_total`, `rag_runtime_agentic_total`,
`rag_runtime_grounding_check_total`, `rag_runtime_go_bridge_errors_total`,
`rag_runtime_go_bridge_pool_timeouts_total`,
`rag_runtime_qdrant_errors_total`,
`rag_runtime_qdrant_pool_timeouts_total`, and
`rag_runtime_qdrant_fallback_total`. Rising Qdrant pool timeouts with level
query volume means the RAG HTTP pool or Qdrant capacity is too small. Rising
fallback volume means availability is being protected by a less capable
retrieval path.

## Autoscaling

Every component keeps a CPU `Resource` metric. Each autoscaling block also has:

```yaml
externalMetrics: []
scaleDownStabilizationWindowSeconds: 300
```

External metrics render only when configured; the chart default remains
CPU-only. External metrics require the Kubernetes custom-metrics API, normally
provided by a Prometheus custom-metrics adapter. This chart does not install
that adapter.

A production pressure configuration should start conservatively:

```yaml
components:
  agentWorker:
    autoscaling:
      externalMetrics:
        - name: allcallall_agent_outbox_oldest_pending_seconds
          targetAverageValue: "2"
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

The two oldest-age metrics are adapter-defined and must be event-class
filtered. Derive `allcallall_agent_outbox_oldest_pending_seconds` from the
oldest pending row for the agent worker event classes
(`agent.run.requested`, `workflow.run.requested`,
`agent.tool.write.requested`, `workflow.tool.write.requested`, and
`mcp.execution.terminal`). Derive
`allcallall_outbox_oldest_pending_seconds` from the oldest pending row for the
outbox worker event classes (`agent.run.completed`, `message.created`,
`rag.source.ingest_requested`, `rag.chunk.index_requested`, and, when enabled,
`recording.transcription.requested` plus `settlement.room.ended`). A SQL-backed
custom-metrics adapter is one way to compute these values; neither is a native
Go counter. Bind each filtered metric only to the worker that owns those
events. Do not bind `allcallall_agent_admission_queue_depth` to the Go agent
worker: it maps to `agent_runtime_admission_queued_runs`, which measures
Python Runtime admission pressure and belongs on `agentRuntime`. Verify all
adapter resources before enabling them:

```bash
kubectl get --raw /apis/custom.metrics.k8s.io/v1beta1 \
  | jq '.resources[] | select(.name | contains("allcallall"))'
```

The 300-second scale-down stabilization window prevents a brief provider
slowdown from creating replica oscillation. Do not reduce it to make a load test
finish faster. If the adapter becomes unavailable, remove the configured
`externalMetrics` entries and return to the CPU fallback; do not remove the CPU
metric.

### Connection-budget validation

Before enabling External metrics, calculate the theoretical worst-case
connection total at each component's `maxReplicas`. A pool is a ceiling, not a
reservation, but the downstream server must tolerate that ceiling plus its
operational and administrative reserve.

| Component | Formula | Chart default at `maxReplicas` |
|---|---|---:|
| API MySQL | `maxReplicas × database.max_open_conns` | 10 × 50 = 500 |
| API Redis | `maxReplicas × redis.pool_size` | 10 × 500 = 5000 |
| API Agent Runtime HTTP | `maxReplicas × runtime.maxConnsPerHost` | 10 × 20 = 200 |
| Agent Worker MySQL | `maxReplicas × database.max_open_conns` | 10 × 50 = 500 |
| Agent Worker Redis | `maxReplicas × redis.pool_size` | 10 × 500 = 5000 |
| Agent Worker Agent Runtime HTTP | `maxReplicas × runtime.maxConnsPerHost` | 10 × 20 = 200 |
| Outbox Worker MySQL | `maxReplicas × database.max_open_conns` | 8 × 50 = 400 |
| Agent Runtime checkpoint MySQL | `maxReplicas × PY_AGENT_CHECKPOINT_MYSQL_POOL_SIZE` | 12 × 4 = 48 |
| Agent Runtime provider/RAG/Tool Bridge HTTP | `maxReplicas × PY_AGENT_HTTP_MAX_CONNECTIONS` | 12 × 20 = 240 |
| RAG Runtime Go Bridge/Qdrant HTTP | `maxReplicas × PY_RAG_HTTP_MAX_CONNECTIONS` | 12 × 20 = 240 |

The outbox worker does not initialize Redis. The Agent Runtime HTTP pool is
shared by provider, RAG, and Tool Bridge requests; 240 is the worst case if all
connections are simultaneously aimed at one provider. The RAG Runtime shares
one pool for Go Bridge and Qdrant, so 240 is the Qdrant worst case if all
traffic is directed there. At the chart maxima, the three Go backend MySQL
pools can total 1400 connections and the API plus Agent Worker Redis pools can
total 10000 connections. Validate the actual provider rate limits, Qdrant
connection limits, MySQL `max_connections`, and Redis `maxclients` before
enabling External metrics. Lower per-pod pool limits when a maximum is outside
the measured server budget.

## Alert thresholds

These are initial thresholds, not universal SLOs. Tune them after collecting at
least one sustained-load baseline in the target environment.

| Signal | Warning | Critical |
|---|---|---|
| Outbox backlog (`outbox_backlog`) | >1000 for 10 minutes | >10000 for 5 minutes |
| Outbox age (adapter metric) | >30 seconds for 5 minutes | >120 seconds for 5 minutes |
| Outbox queue wait p95 | >5 seconds for 10 minutes | >30 seconds for 10 minutes |
| Agent admission queue depth | >4 average for 5 minutes | >16 for 5 minutes or a `queue_full` rejection burst |
| Agent admission queue wait p95 | >2.5 seconds for 10 minutes | >5 seconds for 10 minutes |
| Dependency error ratio | >1% for 10 minutes | >5% for 10 minutes |
| MySQL pool waits | >0.1 waits/second for 10 minutes | >1 wait/second for 10 minutes |
| Redis pool waits | >1 wait/second for 10 minutes | Any pool timeout |
| Checkpoint payload p95 | >1 MiB | >4 MiB or sustained growth over three load runs |
| Checkpoint operation errors | >0 for 5 minutes | Retry/backoff not reducing errors for 15 minutes |
| Context saturation | Used/max ratio >0.9 for 10 minutes | Truncation indications plus latency regression |
| Cancellation grace exceeded | Any increase | Any increase during a rollout |
| Workflow failure ratio | >1% for 10 minutes | >5% for 10 minutes |

The outbox External target of 2 seconds is more aggressive than the 30-second
warning alert: HPA should react before an operator-facing SLO is breached.

## Overload interpretation

- **429 Too Many Requests** means a bounded rate limit or provider quota. It is
  expected under overload; clients must honor `Retry-After` and back off with
  jitter. Sustained 429s mean the offered load, quota, or replica/pool budget is
  wrong, not that retries should be multiplied.
- **503 Service Unavailable** means bounded admission, checkpoint busy, or a
  dependency is unavailable. It protects the system by shedding work and should
  carry `Retry-After` when the downstream protocol provides one.
- **504 Gateway Timeout** means a deadline expired. Do not blindly retry at every
  layer; the Go/Python retry budget and outbox lease decide whether one bounded
  retry is safe.
- **409 CHECKPOINT_VERSION_CONFLICT** means concurrent state ownership or a
  stale resume, not a capacity signal. Investigate lease ownership before
  scaling anything.
- **413 CHECKPOINT_TRANSACTION_TOO_LARGE** means checkpoint payload projection
  or request size is too large. Retrying the same payload cannot succeed.

If admission rejections rise while `agent_runtime_admission_active_runs` stays
at its configured cap, add capacity only after confirming the checkpoint and
provider pools are not already saturated. If outbox age rises while
`outbox_inflight` is low, increase worker concurrency or replicas. If it rises
while inflight is saturated and event duration rises, the handler or a
downstream dependency is the limiting resource.

## Cancellation and shutdown

An HTTP timeout requests cooperative cancellation; it does not immediately
release admission capacity. The lease stays held until the graph future exits, so
a slow provider cannot cause over-admission. `cancellation_grace_seconds` bounds
that wait. Observe:

- `agent_runtime_cancel_requested_total`
- `agent_runtime_cancelled_total`
- `agent_runtime_cancel_grace_exceeded_total`

Reasons are bounded to `client_cancelled`, `deadline_exceeded`, `shutdown`, and
`lease_lost`. Approved writes remain Go-owned and durable: a cancellation or
worker retry can reprocess an outbox event, but its approval-bound idempotency
key prevents a duplicate product write. During rollout, let Kubernetes drain
pods; do not force-kill workers before the cancellation and outbox lease grace
windows have elapsed.

## Checkpoint warnings

Treat these as performance rollbacks, not tuning opportunities:

- `checkpoint_payload_bytes` p95 above 1 MiB or growing run-over-run.
- Checkpoint errors or lock waits rising while request volume is level.
- Frequent `CHECKPOINT_VERSION_CONFLICT` after a deployment.
- A rollout where the prior runtime cannot read newly written checkpoint state.

Large checkpoints increase MySQL CPU, lock time, and replication pressure.
Adding replicas can worsen that bottleneck. Reduce projected state or move
non-resume trace data out of the checkpoint first.

## Rollback signals

Roll back pressure scaling when any of these appear:

- Duplicate approved-write side effects or duplicate durable write events.
- Outbox backlog or oldest-pending age grows while replicas increase.
- Additional outbox replicas stop improving drain rate materially.
- Lease conflicts, checkpoint version conflicts, or checkpoint incompatibility.
- Provider error ratio, database wait, or lock time becomes the limiting resource.
- Admission, cancellation-grace, or correctness regressions in a load profile.
- External metrics become unavailable or the HPA oscillates.

To roll back External metrics, set each affected block's `externalMetrics` back
to `[]`; the CPU `Resource` metric remains. Keep the 300-second scale-down
window unless a measured environment proves a different value is safer.

## Event-stream adoption gate

Do not introduce a new event broker until all of the following are documented
from the optimized outbox:

```text
optimized outbox cannot drain 2x measured peak ingress
oldest pending age breaches the SLO during sustained load
MySQL CPU or lock wait is the measured limiting resource
additional outbox replicas no longer improve drain rate materially
the operational cost of a new broker is documented and accepted
```
