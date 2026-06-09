# Worker Runtime

The worker runtime lives in `backend/internal/runtime`. It keeps the API server and standalone worker commands on the same implementation path.

## Entrypoints

- `backend/cmd/server`: API server. Uses embedded workers by default for simple local development.
- `backend/cmd/agent-worker`: standalone Agent worker.
- `backend/cmd/outbox-worker`: standalone collaboration outbox worker.
- `backend/cmd/cleanup-worker`: standalone retention/session cleanup worker.

## Shared Runtime Functions

- `AutoMigrate`: one model migration list shared by API and workers.
- `OpenMigratedDB`: opens MySQL and applies migrations.
- `ConfigureTraceFromEnv`: installs optional OTLP span export when `OTEL_EXPORTER_OTLP_ENDPOINT` is set.
- `RegisterAgentOutboxHandlers`: registers `agent.run.requested`.
- `RegisterCollaborationOutboxHandlers`: registers `agent.run.completed` and `message.created`.
- `ConfigureOutboxProcessorFromEnv`: applies batch size, retry, lease, worker ID, and event filters.
- `StartAgentWorker`, `StartCollaborationOutboxWorker`, `StartCleanupWorker`: start long-running loops.

## Event Ownership

| Worker | Events / Work | Why It Is Extractable |
| --- | --- | --- |
| API embedded worker | all outbox events, cleanup | Local dev convenience |
| `agent-worker` | `agent.run.requested` | Agent execution is async, retryable, and lease-protected |
| `outbox-worker` | `agent.run.completed`, `message.created` | Realtime replay delivery is idempotent and event-driven |
| `cleanup-worker` | refresh sessions, recording retention | Operational cleanup should not block API requests |

The `events.Processor` supports event filters so standalone workers do not claim events that belong to another worker process.

## Environment Variables

- `EMBEDDED_WORKERS=0`: disable API-embedded workers and expect standalone workers.
- `WORKER_ID`: stable worker identity for outbox claims.
- `OUTBOX_WORKER_INTERVAL_SEC`: polling interval.
- `OUTBOX_WORKER_BATCH_SIZE`: claim batch size.
- `OUTBOX_WORKER_MAX_ATTEMPTS`: retry budget before permanent failure.
- `OUTBOX_WORKER_RETRY_DELAY_SEC`: retry backoff.
- `OUTBOX_WORKER_LEASE_SEC`: lease duration for claimed events.
- `REFRESH_SESSION_CLEANUP_INTERVAL_MIN`: refresh-session cleanup interval.
- `REFRESH_SESSION_REVOKED_RETENTION_DAYS`: revoked session retention.
- `RECORDING_CLEANUP_INTERVAL_MIN`: recording retention cleanup interval.

## Failure Semantics

- If a worker crashes after claiming an outbox event, `locked_until` eventually expires and another worker can reclaim it.
- If a handler returns an error, the processor increments attempts and either retries later or marks the event failed.
- If Agent execution times out, the run is marked `failed` with the lease cleared, so it can be retried under the attempt budget.
- If recording object deletion fails, metadata remains undeleted so a later cleanup pass can retry.

## Microservice Readiness

This runtime is intentionally still single-repo and shared-database. That is the transition state:

1. Prove process boundaries with independent workers.
2. Keep transactional writes inside MySQL while using outbox rows as the event contract.
3. Replace the outbox polling implementation with Redis Streams/Kafka only after the event schema and failure semantics are stable.

That makes the project credible as a modular monolith today and microservice-ready tomorrow.
