# Backend Deep Dive Talking Points

Use this page as a preparation checklist before backend interviews.

## Architecture

- Go backend with Gin handlers, service-layer business logic, Gorm repositories/models, Redis-assisted realtime infrastructure, and S3-compatible storage abstraction.
- Mobile/Web/Desktop clients share the same `/api/v1` backend surface instead of platform-specific APIs.
- Collaboration data is organization-scoped: conversations, rooms, recordings, notes, messages, and Agent runs all carry organization boundaries.

## Realtime Design

- Chat and collaboration events use WebSocket plus replayable event records.
- Chat events carry explicit `sequence` values while preserving backward-compatible `event_id`.
- Room state is represented as patchable state, not only page refreshes.
- Meeting events and recording lifecycle events are written into the conversation timeline to keep collaboration context recoverable after reconnect.

Interview angle:

- Discuss how event replay avoids missed messages after mobile backgrounding or weak network reconnects.
- Discuss the tradeoff between a simple WebSocket hub and durable event logs.

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

Interview angle:

- Discuss refresh token replay detection.
- Discuss why organization boundary checks must be repeated in service-layer methods, not only UI routes.

## Agent Backend Design

- Agent execution is persisted as `agent_runs`.
- Intermediate reasoning stages are stored as `agent_steps`.
- Side effects are stored as `agent_tool_calls`.
- Scoped memories are stored as `agent_memories`.
- Retry safety is provided through `Idempotency-Key`.
- Tool side effects enqueue durable `event_outbox` records.
- Tool execution is still controlled by backend service code.

Interview angle:

- Explain why Agent tools need permission checks, idempotency, observability, and bounded side effects.
- Explain why the first version is deterministic before adding an LLM provider.

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

## Next High-Value Engineering Tasks

- Add a provider interface for Agent planning and keep the current rules planner as the default implementation.
- Add a seed/demo command for interview walkthroughs.
- Add load tests for event replay and Agent run creation.
- Split oversized services such as collaboration into smaller domain services.
- Add architecture diagrams and a short demo script in `docs/interview`.
