# Workers and Processes

The backend can run as one API process with embedded workers or as separate
processes that share the same domain and runtime packages.

## Entrypoints

| Entrypoint | Responsibility |
| --- | --- |
| `backend/cmd/server` | HTTP API and signaling; embedded workers are enabled by default |
| `backend/cmd/user-service` | gRPC access-token validation and user lookup |
| `backend/cmd/agent-worker` | Agent-run and workflow events |
| `backend/cmd/outbox-worker` | Collaboration, knowledge, transcription, and settlement bridge events |
| `backend/cmd/data-worker` | Kafka-compatible settlement consumption and durable writes |
| `backend/cmd/search-worker` | Elasticsearch indexing from outbox events |
| `backend/cmd/cleanup-worker` | Refresh-session and recording-retention cleanup |

## Shared Assembly

Shared runtime functions open the database, apply migrations, configure
tracing, choose recording and transcription adapters, create search/indexing
services, register outbox handlers, and start cleanup loops. Standalone workers
use event filters so one process does not claim another process's work.

## Delivery Semantics

- Claims use a lease so abandoned work can be recovered.
- Handler failures increment attempts and become retryable or terminal based on
  configured limits.
- Agent runs have their own attempt and lease state.
- Settlement writes are idempotent by source event and room/user identity.
- Search and knowledge indexing are eventually consistent.
- Recording metadata is removed only after backing-object deletion succeeds.

## Operating Modes

Embedded mode is the simplest local setup. Extracted mode sets
`EMBEDDED_WORKERS=0` on the API and launches the required worker commands.
Exact environment variables and defaults belong in the
[configuration reference](../reference/configuration/runtime.md); deployment
examples belong in the [deployment guide](../guides/deployment/deployment-guide.md).
