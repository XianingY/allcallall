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

## OpenTelemetry-Lite Observability

- `backend/internal/trace` now provides a lightweight span recorder without adding a full OpenTelemetry SDK dependency.
- `StartSpan` stores `trace_id`, `span_id`, `parent_span_id`, `request_id`, `outbox_id`, attributes, duration, and error status.
- If no recorder is configured, spans are no-ops, so production behavior does not change.
- `OTEL_EXPORTER_OTLP_ENDPOINT` enables an OTLP/HTTP export seam; when set, the server installs a global recorder and posts span payloads to `/v1/traces`.
- `OTEL_SERVICE_NAME` can override the exported service name; otherwise it defaults to `allcallall-backend`.
- The outbox processor records `outbox.process_event`.
- Agent execution records `agent.execute_run`, `agent.planner.plan`, and `agent.tools.execute_side_effects`.
- Tests can inject `MemorySpanRecorder` to assert async trace shape without parsing logs.

Interview angle:

- Explain the difference between correlation IDs and spans: `request_id` tells which request started the work; spans show where time and errors occur across handler, outbox, planner, and tools.
- Explain why this project uses a small internal recorder first: it gives deterministic tests and a clean seam for exporting to an OpenTelemetry Collector without making business code depend on the full SDK.
- Explain how `event_outbox.request_id` and `trace.WithOutboxID` bridge HTTP request context into worker execution.

## Agent Backend Design

- Agent execution is persisted as `agent_runs`.
- `POST /agent/runs` creates or returns a `pending` run and enqueues `agent.run.requested`.
- The outbox worker calls `ExecuteRun`, atomically acquires a run with `attempts` and `lease_until`, transitions it to `running`, and writes `ready` or `failed`.
- Failed runs can be retried while under the run attempt budget, and stale `running` runs can be recovered after the lease expires.
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
- Explain why async Agent execution needs leases and attempts, not only a `pending/running/ready` enum.
- Explain why the first version is deterministic before adding an LLM provider, and how prompt token estimates, latency metrics, and fallback counters make the provider seam observable.

## Outbox Worker Design

- `event_outbox` stores durable domain events with aggregate type, aggregate ID, event name, payload JSON, idempotency key, status, attempts, and error metadata.
- `event_outbox.request_id` preserves the originating request context for async diagnostics.
- The shared worker runtime starts outbox processors that claim pending rows with `locked_by` and `locked_until`, then process them through registered handlers.
- `cmd/server` can run embedded workers for local development, while `cmd/agent-worker` and `cmd/outbox-worker` can run as independent processes with event filters.
- `message.created` outbox handling reloads the message, resolves conversation members, writes per-user replay events, and publishes to the WebSocket hub.
- Runtime knobs are `OUTBOX_WORKER_INTERVAL_SEC`, `OUTBOX_WORKER_BATCH_SIZE`, `OUTBOX_WORKER_MAX_ATTEMPTS`, and `OUTBOX_WORKER_RETRY_DELAY_SEC`.
- Metrics distinguish publish, retry, and permanent failure paths.

Interview angle:

- Explain why Agent run creation writes `agent.run.requested`, why message write-back writes `agent.run.completed`, and why both use outbox idempotency keys.
- Explain why the current handler can be a simple observed event while the contract still allows Kafka, Redis Streams, or webhook publishers later.
- Explain how persisting `request_id` turns the outbox from a black box into a supportable async pipeline.
- Explain how claim/lease avoids duplicate processing across multiple backend replicas while still allowing expired work to be recovered.
- Explain how event filters allow Agent and collaboration workers to scale independently without claiming each other's outbox rows.

## Realtime Replay Store

- `RealtimeEventStore` owns durable chat/room event persistence for replay.
- It writes `chat_events`, assigns a stable sequence from the persisted row ID, and decodes payloads for reconnect catch-up.
- `Service` still controls membership checks and publishing, while storage mechanics are independently testable.
- `ListRealtimeEventsSince` keeps the API boundary while delegating replay storage to the store.
- Tests include a reconnect replay case that creates missed room/message events and verifies `since_id` returns only the target user's missing events in stable order.

Interview angle:

- Explain the separation between authorization/business orchestration and durable realtime event storage.
- Explain why `event_id`/`sequence` replay is more reliable than relying only on in-memory WebSocket delivery.

## Failure Injection Evidence

- Outbox delivery failures are covered by `backend/internal/events/processor_test.go`: the processor retries transient handler errors, then marks the event failed and increments retry/failure metrics.
- Agent planner failures are covered by `backend/internal/agent/service_test.go`: failed runs can be retried, stale running runs can be recovered after lease expiry, and planner timeout now persists a failed terminal state even when the execution context is canceled.
- Recording storage cleanup failures are covered by `backend/internal/collaboration/service_test.go`: object-delete errors leave metadata undeleted so later cleanup can retry safely.
- Realtime reconnect loss is covered by `backend/internal/collaboration/realtime_event_store_test.go`: missed events after a disconnect are replayed by `since_id`, scoped by organization/user, and paginated deterministically.

Interview angle:

- Explain that the project does not only test happy paths; it deliberately injects worker, planner, storage, and realtime failures.
- Explain why timeout/cancellation handling is subtle: if failure persistence uses the canceled request context, the system can leave jobs stuck in `running`.

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

Completed second extraction:

- Chat message list/create and outbox message delivery entrypoints now live in `backend/internal/collaboration/chat_service.go`.
- Realtime fan-out, message dedup delivery, and room/conversation event publishing now live in `backend/internal/collaboration/realtime_delivery.go`.
- This keeps the public `Service` API intact while reducing the main file and creating clear seams for future chat/room service extraction.

Completed third extraction:

- Room lifecycle, meeting join/leave, WebRTC offer/ICE handling, media-state updates, and room state loading now live in `backend/internal/collaboration/room_service.go`.
- Recording start/stop, recording file lookup, download URL generation, export audit, retention cleanup, retention metadata, and storage artifact persistence now live in `backend/internal/collaboration/recording_service.go`.
- Support-side read-only room and recording diagnostics now live in `backend/internal/collaboration/support_service.go`.
- Conversation status/priority/assignee/contact update planning now lives in `backend/internal/collaboration/conversation_update.go`, with table tests covering pure decision logic before DB transactions and realtime patch publication.
- The main collaboration service file is now about 1,800 lines, while keeping existing API contracts and tests green.

Completed handler extraction:

- `backend/internal/handlers/collaboration_handler.go` keeps shared wiring, route registration, DTO parsing helpers, and response helpers.
- Conversation/chat endpoints now live in `backend/internal/handlers/collaboration_conversation_handler.go`.
- Room and WebRTC signaling endpoints now live in `backend/internal/handlers/collaboration_room_handler.go`.
- Recording endpoints now live in `backend/internal/handlers/collaboration_recording_handler.go`.
- Internal support diagnostics now live in `backend/internal/handlers/collaboration_support_handler.go`.

Completed worker runtime extraction:

- Shared migrations now live in `backend/internal/runtime/migrations.go`.
- Shared worker registration and cleanup loops now live in `backend/internal/runtime/workers.go`.
- `backend/cmd/agent-worker`, `backend/cmd/outbox-worker`, and `backend/cmd/cleanup-worker` can run as standalone processes.
- `EMBEDDED_WORKERS=0` lets the API run without internal workers so the multi-process worker split can be demonstrated locally.

Completed Agent extraction:

- Agent mutating tools are coordinated by `backend/internal/agent/tool_executor.go`.
- `executeRulesRun` now focuses on run state, context collection, planner execution, and final run persistence; the side-effect executor owns ordered tool execution and tool metrics for message write-back, follow-up task creation, and memory upsert.
- This is a useful interview seam because future guardrails, tool authorization, timeout budgets, or live model tool traces can be added in one place.

## Next High-Value Engineering Tasks

- Add streaming/tool-call traces behind the existing OpenAI-compatible planner if a live model demo is needed.
- Split oversized services such as collaboration into smaller domain services.
- Capture measured p95/p99 baselines in `docs/interview/performance-report.md`.
