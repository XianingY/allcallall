# Worker Runtime

The worker runtime lives in `backend/internal/runtime`. It lets the same codebase run as one API process with embedded workers or as extracted backend processes.

## Entrypoints

| Entrypoint | Role |
| --- | --- |
| `backend/cmd/server` | API/signaling server. Embedded workers enabled by default. |
| `backend/cmd/user-service` | gRPC User Service for access-token validation and user lookup. |
| `backend/cmd/agent-worker` | Executes `agent.run.requested` and workflow events. |
| `backend/cmd/outbox-worker` | Handles collaboration events, knowledge ingest/index events, recording transcription, and optional Kafka settlement bridge. |
| `backend/cmd/data-worker` | Consumes Kafka-compatible settlement topic and writes `room_settlements`. |
| `backend/cmd/search-worker` | Indexes messages into Elasticsearch from outbox events. |
| `backend/cmd/cleanup-worker` | Cleans refresh sessions and expired recording files. |

## Shared Runtime Functions

- `OpenMigratedDB`: opens MySQL and applies AutoMigrate models.
- `ConfigureTraceFromEnv`: optional tracing setup.
- `RecordingStorageFromEnv`: selects local or S3-compatible recording storage.
- `TranscriptionProviderFromEnv`: enables `mock` recording transcription when `TRANSCRIPTION_ENABLED` is set.
- `SearchServiceFromEnv`: selects memory or Elasticsearch message search.
- `ChunkIndexerFromEnv`: wires vector/BM25-style chunk indexing where configured.
- `KafkaProducerFromEnv` / `KafkaConsumerFromEnv`: enables settlement bridge/data worker paths.
- `ConfigureOutboxProcessorFromEnv`: applies batch size, retry, lease, worker ID, and event filters.

## Event Ownership

| Worker | Events / Work | Why It Is Extractable |
| --- | --- | --- |
| API embedded workers | Local outbox handlers and cleanup | Development convenience. |
| `agent-worker` | `agent.run.requested`, workflow requested events | Agent execution is async, retryable, and lease-protected. |
| `outbox-worker` | `agent.run.completed`, `message.created`, `rag.source.ingest_requested`, `rag.chunk.index_requested`, `recording.transcription.requested`, optional `settlement.room.ended` | Collaboration, knowledge, and transcription side effects do not need to block HTTP. |
| `search-worker` | `search.message.index_requested` | Search indexing is eventually consistent. |
| `data-worker` | Kafka topic `allcallall.room.settlements` | Settlement writes can be smoothed independently from meeting disconnect spikes. |
| `cleanup-worker` | refresh sessions, recording retention | Operational cleanup should not block API requests. |

Standalone workers use event filters so they do not claim events owned by another process.

## Runtime Variables

Outbox:

- `EMBEDDED_WORKERS=0`
- `WORKER_ID`
- `OUTBOX_WORKER_INTERVAL_SEC`
- `OUTBOX_WORKER_BATCH_SIZE`
- `OUTBOX_WORKER_MAX_ATTEMPTS`
- `OUTBOX_WORKER_RETRY_DELAY_SEC`
- `OUTBOX_WORKER_LEASE_SEC`

Recording/transcription:

- `RECORDING_STORAGE_DRIVER`
- `RECORDING_STORAGE_DIR`
- `RECORDING_S3_*`
- `TRANSCRIPTION_ENABLED`
- `TRANSCRIPTION_PROVIDER=mock|openai_compatible`
- `TRANSCRIPTION_OPENAI_*`
- `TRANSCRIPTION_CHUNK_SECONDS`
- `TRANSCRIPTION_FFMPEG_PATH`

Infra:

- `USER_GRPC_ADDR`
- `USER_SERVICE_GRPC_ADDR`
- `KAFKA_BROKERS`
- `KAFKA_SETTLEMENT_TOPIC`
- `KAFKA_CONSUMER_GROUP`
- `ELASTICSEARCH_URL`
- `ELASTICSEARCH_INDEX`

Cleanup:

- `REFRESH_SESSION_CLEANUP_INTERVAL_MIN`
- `REFRESH_SESSION_REVOKED_RETENTION_DAYS`
- `RECORDING_CLEANUP_INTERVAL_MIN`

## Failure Semantics

- Claimed outbox rows use `locked_until`; another worker can reclaim after lease expiry.
- Handler errors increment attempts and either retry later or mark the row failed.
- Agent runs use status, attempts, and `lease_until`.
- Recording transcription failures mark the transcription job failed but do not undo recording persistence.
- Settlement Kafka consumption is idempotent through `source_event_id` and `(room_id,user_id)`.
- Search indexing and knowledge indexing retry through outbox.
- Recording cleanup soft-deletes metadata only after backing object deletion succeeds.

## Interview Narrative

The project stays single-repo and shared-schema for local demos, but it has real extraction points:

1. Keep domain logic in services.
2. Move async side effects to outbox workers.
3. Use gRPC for latency-sensitive auth validation.
4. Use Kafka-compatible messaging for bursty settlement writes.
5. Use Elasticsearch as an eventually consistent read model.
6. Split databases only after ownership and traffic justify it.
