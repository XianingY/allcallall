# Microservice Evolution Plan

AllCallAll is intentionally positioned as a modular monolith that can evolve into microservices. The current goal is not to split every domain into a separate service. The goal is to make the boundaries executable, observable, and safe to extract later.

## Current Position

The main API remains `backend/cmd/server`. It owns HTTP routing, authentication middleware, request validation, and synchronous business APIs.

Domain boundaries are kept inside `backend/internal`:

- `auth`: login, JWT, refresh sessions, logout-all, session cleanup.
- `collaboration`: organizations, conversations, rooms, messages, recordings, notes, realtime replay records.
- `agent`: Agent runs, steps, tool calls, memories, RAG-lite context chunks.
- `events`: MySQL-backed outbox, worker claim/lease, retry, idempotency.
- `storage`: local and S3-compatible recording storage.
- `metrics` / `trace`: counters, request IDs, lightweight spans, optional OTLP export.

This is the Stage 1 modular monolith.

## Stage 2: Multi-Process Workers

The first extraction is worker processes, not CRUD services:

- `backend/cmd/agent-worker`: consumes `agent.run.requested` and executes Agent planning/tool calls.
- `backend/cmd/outbox-worker`: consumes `agent.run.completed` and `message.created` for collaboration delivery and replay logs.
- `backend/cmd/cleanup-worker`: runs refresh-session cleanup and recording retention cleanup.

These workers still share MySQL and Redis with the API. That is deliberate. It avoids distributed transactions while proving independent process boundaries, worker leases, retries, and failure isolation.

## Stage 3: Event Bus Migration

The current bridge is `event_outbox`:

1. API writes business rows and outbox rows in the same transaction.
2. Workers claim pending rows by event type.
3. Workers mark rows as published, retry, or failed.
4. Request IDs are persisted so API requests can be correlated with async worker execution.

Current and future migration path:

- Current: MySQL `event_outbox`.
- Current executable bridge: `settlement.room.ended` outbox events can be published to Kafka-compatible brokers by `cmd/outbox-worker` when `KAFKA_BROKERS` is configured.
- Current executable consumer: `cmd/data-worker` consumes `allcallall.room.settlements` and writes idempotent `room_settlements`.
- Later: expand Kafka topics for analytics, notification fan-out, or Agent work queues after measuring traffic.
- Final: Agent service, realtime gateway, and recording service subscribe to event streams and own their own persistence if needed.

## Stage 4: Narrow Synchronous Service Split

The first synchronous service split is the User Service:

- Contract: `backend/proto/user/v1/user.proto`.
- Server: `backend/cmd/user-service`.
- Client: `backend/internal/usergrpc`.
- Runtime switch: `USER_SERVICE_GRPC_ADDR`.

When configured, the API/signaling gateway validates access tokens by calling `UserService/ValidateAccessToken` over gRPC instead of parsing tokens locally. This is intentionally narrow: it proves request-time service decomposition without forcing every domain into a separate database or duplicating organization/business logic.

## Stage 5: Search Read Model

Message search is now an extractable read side:

- Writes: `message.created` also enqueues `search.message.index_requested`.
- Worker: `backend/cmd/search-worker`.
- Index: Elasticsearch via `ELASTICSEARCH_URL`.
- API: `GET /api/v1/search/messages?q=...`.

The search API filters ES hits through conversation membership checks, so MySQL remains the source of truth for authorization.

## Why Not Split Everything First?

Do not split every CRUD module first. `organization-service`, `billing-service`, and generic profile services have tight request-time consistency and authorization coupling. Splitting them early would add network calls, distributed transactions, and operational load without creating a stronger backend interview story.

The current project therefore uses two safer extraction styles:

- A narrow synchronous split: `cmd/user-service` over gRPC for token/user lookup.
- Asynchronous splits: Agent runs, outbox fan-out, Kafka settlement, search indexing, and cleanup workers.

These boundaries are easier to verify, scale, and explain than splitting the entire domain model prematurely.

## Interview Talking Point

The project demonstrates a pragmatic migration path:

> I kept the request path as a modular monolith for transaction safety and local demo simplicity. Then I extracted naturally asynchronous workloads into standalone worker processes using the outbox pattern. This gives independent scaling and failure recovery today, while keeping a clean path to Redis Streams or Kafka later.

Updated version after the gRPC/Kafka/ES additions:

> I kept the external API as a modular monolith, then extracted one narrow synchronous boundary through gRPC User Service and two read/async infrastructure paths through Kafka settlement and Elasticsearch search. This shows how to evolve a monolith by pressure point instead of doing a risky big-bang rewrite.

## Demo Commands

Run the all-in-one API:

```bash
make run-api
```

Run workers independently:

```bash
make run-agent-worker
make run-outbox-worker
make run-cleanup-worker
cd backend && CONFIG_PATH=./configs/config.yaml go run ./cmd/user-service
cd backend && CONFIG_PATH=./configs/config.yaml go run ./cmd/data-worker
cd backend && CONFIG_PATH=./configs/config.yaml go run ./cmd/search-worker
```

Run the multi-process demo:

```bash
make interview-microservice-demo
```

Run the Docker Compose profile:

```bash
docker compose -f infra/docker-compose.yml --profile microservices up api agent-worker outbox-worker cleanup-worker
```

Run optional infrastructure for Kafka and Elasticsearch:

```bash
docker compose -f infra/docker-compose.yml --profile interview-infra up
```

## Latest Local Evidence

Measured locally on June 9, 2026 with Docker MySQL/Redis and four Go processes:

```bash
CONCURRENCY=2 WS_CLIENTS=1 WS_DURATION_MS=1000 make interview-microservice-demo
```

Report directory:

```text
/tmp/allcallall-microservice-demo-20260609-132638
```

Observed evidence:

- API log: `embedded workers disabled; expecting standalone workers`.
- Agent worker log: processed `agent.run.requested` events and executed Agent runs.
- Outbox worker log: processed `message.created` events and wrote realtime replay records.
- Cleanup worker log: started independently from the API process.
- Agent smoke: `accepted=2 ready=2 failed=0`.
- WebSocket smoke: `opened=1 errors=0 messages=24`.
