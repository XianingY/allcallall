# Backend Architecture

The Go backend is a modular product service built with Gin, Gorm, MySQL, Redis,
WebSocket/WebRTC infrastructure, and durable workers.

## Responsibilities

The backend owns authentication, tenant boundaries, collaboration data,
meetings, recordings, transcripts, approvals, audit, and write execution. These
responsibilities stay in Go even when Agent orchestration or retrieval runs in
Python.

## Package Boundaries

| Area | Packages |
| --- | --- |
| HTTP and process assembly | `internal/server`, `internal/handlers`, `internal/runtime`, `cmd/*` |
| Identity and tenancy | `auth`, `user`, `usergrpc`, `contact`, `invitation` |
| Collaboration | `collaboration`, `chat`, `presence`, `signaling`, `media` |
| Agent platform | `agent`, `mcpplatform`, `knowledge`, `sandbox` |
| Data and infrastructure | `database`, `models`, `storage`, `cache`, `mq`, `search` |
| Policy and operations | `compliance`, `messagecrypto`, `events`, `metrics`, `trace` |

Handlers translate HTTP concerns. Domain services enforce authorization and
business rules. Infrastructure packages provide persistence and external
adapters. Process assembly must not leak back into domain packages.

## Data and Authorization

MySQL is the canonical store. Organization and conversation membership checks
are performed in service methods rather than delegated to clients or search
systems. Elasticsearch, Redis, and Kafka-compatible brokers are optional
supporting infrastructure and do not become sources of authorization truth.

Schema creation for a fresh database uses Gorm models; ordered SQL migrations
advance existing versioned deployments. Read [backend/migrations/README.md](../../backend/migrations/README.md)
before changing schema behavior.

## Realtime and Async Work

Realtime delivery combines live hubs with durable replay rows. Async work uses
the MySQL outbox and independently runnable workers. Events are designed for
at-least-once handling, so consumers enforce idempotency at their write
boundaries.

## Security and Privacy

JWT and refresh-session handling live in the backend. Production deployments
must require TLS for `/api/v1`; probe endpoints intentionally remain available
to kubelet without forwarded-protocol headers. Message retention, encryption,
recall, search minimization, erasure, and moderation are assembled through
`runtime.ApplyPrivacyPolicies` so API and workers share one policy path.

Configuration values are documented in the [configuration reference](../reference/configuration/runtime.md),
and public endpoints are documented in the [API reference](../reference/api/http-api.md).
