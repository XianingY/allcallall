# Backend Deep Dive Talking Points

Use this page as a preparation checklist before backend interviews.

## Architecture

- Go backend with Gin handlers, service-layer business logic, Gorm repositories/models, Redis-assisted realtime infrastructure, and S3-compatible storage abstraction.
- Mobile/Web/Desktop clients share the same `/api/v1` backend surface instead of platform-specific APIs.
- Collaboration data is organization-scoped: conversations, rooms, recordings, notes, messages, and Agent runs all carry organization boundaries.

## Realtime Design

- Chat and collaboration events use WebSocket plus replayable event records.
- Chat events carry explicit `sequence` values while preserving backward-compatible `event_id`.
- `/api/v1/chat/ws` is for collaboration event replay; `/api/v1/ws` and `/api/v1/signaling/*` are WebRTC signaling paths.
- Room state is represented as patchable state, not only page refreshes.
- Meeting events and recording lifecycle events are written into the conversation timeline to keep collaboration context recoverable after reconnect.
- `message.created` is both written as an outbox event and delivered into per-recipient `chat_events`, so Agent-generated system messages enter the same replay path as user messages.
- Per-recipient chat event deduplication uses `message.created:<message_id>:<user_id>`, allowing the outbox handler to safely compensate or retry without duplicating replay events.

Interview angle:

- Discuss how event replay avoids missed messages after mobile backgrounding or weak network reconnects.
- Discuss the tradeoff between a simple WebSocket hub and durable event logs.
- Discuss why realtime fan-out should be idempotent when it is driven by an outbox worker.

## Storage And Cleanup

- Recording storage is abstracted as `local` and `s3` drivers.
- Retention metadata is written at upload time.
- Cleanup workers handle stale refresh sessions and expired recording assets.

Interview angle:

- Explain why object keys are generated server-side.
- Explain soft-delete first, then object deletion, then audit/metrics.

## Auth And Security

- JWT access tokens are short-lived.
- Web refresh sessions are persisted and rotated.
- Refresh reuse is tracked as suspicious behavior.
- Support/internal APIs use dedicated tokens and should never be exposed to regular clients.
- `X-Request-ID` is normalized at the HTTP middleware boundary, returned in error responses, stored on Agent runs, copied into `event_outbox`, and re-injected into async outbox handlers.
- Auth failures use the same JSON error envelope as other APIs: `error`, `code`, `request_id`, and `success=false`.

Interview angle:

- Discuss refresh token replay detection.
- Discuss why organization boundary checks must be repeated in service-layer methods, not only UI routes.
- Discuss why async work needs trace context persisted in durable rows, otherwise HTTP logs and worker logs cannot be correlated after the request ends.

## Agent Backend Design

- Agent execution is persisted as `agent_runs`.
- `POST /agent/runs` creates or returns a `pending` run and enqueues `agent.run.requested`.
- The outbox worker calls `ExecuteRun`, transitions `pending -> running`, and writes `ready` or `failed`.
- Intermediate reasoning stages are stored as `agent_steps`.
- Side effects are stored as `agent_tool_calls`.
- Scoped memories are stored as `agent_memories`.
- Retry safety is provided through `Idempotency-Key`.
- Tool side effects enqueue durable `event_outbox` records.
- Tool execution is still controlled by backend service code.
- `AGENT_PROVIDER` selects the planner: `rules` is deterministic and default, `mock_llm` exercises prompt construction plus structured-output parsing without API keys, and `openai_compatible` calls a configured Chat Completions-compatible endpoint or falls back to `rules` when unavailable.
- `go run ./cmd/interview-bench` provides a database-free proof path: it seeds temporary SQLite data, queues Agent runs, drains outbox events, executes tools, and emits JSON counts, latencies, and counters.

Interview angle:

- Explain why Agent tools need permission checks, idempotency, observability, async execution, and bounded side effects.
- Explain why the first version is deterministic before adding an LLM provider, and how prompt token estimates, latency metrics, and fallback counters make the provider seam observable.

## Outbox Worker Design

- `event_outbox` stores durable domain events with aggregate type, aggregate ID, event name, payload JSON, idempotency key, status, attempts, and error metadata.
- `event_outbox.request_id` preserves the originating request context for async diagnostics.
- The server starts `startOutboxWorker`, which drains pending rows on an interval and processes them through registered handlers.
- `message.created` outbox handling reloads the message, resolves conversation members, writes per-user replay events, and publishes to the WebSocket hub.
- Runtime knobs are `OUTBOX_WORKER_INTERVAL_SEC`, `OUTBOX_WORKER_BATCH_SIZE`, `OUTBOX_WORKER_MAX_ATTEMPTS`, and `OUTBOX_WORKER_RETRY_DELAY_SEC`.
- Metrics distinguish publish, retry, and permanent failure paths.

Interview angle:

- Explain why Agent run creation writes `agent.run.requested`, why message write-back writes `agent.run.completed`, and why both use outbox idempotency keys.
- Explain why the current handler can be a simple observed event while the contract still allows Kafka, Redis Streams, or webhook publishers later.
- Explain how persisting `request_id` turns the outbox from a black box into a supportable async pipeline.

## Realtime Replay Store

- `RealtimeEventStore` owns durable chat/room event persistence for replay.
- It writes `chat_events`, assigns a stable sequence from the persisted row ID, and decodes payloads for reconnect catch-up.
- `Service` still controls membership checks and publishing, while storage mechanics are independently testable.
- `ListRealtimeEventsSince` keeps the API boundary while delegating replay storage to the store.

Interview angle:

- Explain the separation between authorization/business orchestration and durable realtime event storage.
- Explain why `event_id`/`sequence` replay is more reliable than relying only on in-memory WebSocket delivery.

## Service Boundary Refactoring Plan

`backend/internal/collaboration/service.go` is intentionally marked as the next major structural cleanup area. The current safe boundaries are:

- Chat service: conversations, messages, notes, realtime event replay.
- Room service: call rooms, members, media state, room events.
- Recording service: recording sessions, files, exports, retention cleanup.
- Support service: read-only diagnostics for rooms, recordings, users, and failures.

The current code exposes these contracts in `backend/internal/collaboration/boundaries.go`:

- `ChatServiceBoundary`
- `RoomServiceBoundary`
- `RecordingServiceBoundary`
- `SupportServiceBoundary`

The project avoids a big-bang split. The recommended interview explanation is: first add tests and DTO boundaries, then extract one cohesive service at a time.

Completed first extraction:

- Durable realtime replay storage is now isolated in `backend/internal/collaboration/realtime_event_store.go`.
- `backend/internal/collaboration/service.go` delegates create/list mechanics to the store and keeps orchestration responsibilities.
- `realtime_event_store_test.go` covers sequence assignment, `since_id` replay, recipient scoping, and bad payload fallback.

## Next High-Value Engineering Tasks

- Add streaming/tool-call traces behind the existing OpenAI-compatible planner if a live model demo is needed.
- Add load tests for event replay, outbox draining, and Agent run creation.
- Split oversized services such as collaboration into smaller domain services.
- Capture measured p95/p99 baselines in `docs/interview/performance-report.md`.
