# Resume Bullets

Use these as raw material. Pick 2-3 bullets and tune them for the target role.

## Backend / Realtime

- Built an AI-powered realtime collaboration backend in Go with Gin, Gorm/MySQL, Redis, WebSocket replay, WebRTC room signaling, and S3-compatible recording storage.
- Designed organization-scoped collaboration threads with replayable realtime events, explicit event sequence numbers, room state patches, recording lifecycle events, and conversation timeline recovery.
- Added recording-end meeting transcription with provider abstraction, mock ASR provider, transcription job state, transcript segment persistence, and Agent retrieval over meeting recording content.
- Delivered `message.created` outbox fan-out into per-recipient WebSocket replay records with dedup keys, so Agent-generated system messages and user messages share the same reconnect path.
- Implemented refresh session hardening with HttpOnly refresh cookies, token rotation, suspicious refresh reuse tracking, logout-all, and support-side session inspection.

## AI Agent Engineering

- Implemented an asynchronous explainable AI Agent run queue with persisted pending/running/ready/failed states, outbox-triggered worker execution, intermediate steps, tool-call records, scoped memory, idempotency keys, and backend-controlled tool execution.
- Added Agent execution recovery with attempt counters and `lease_until`, allowing transient planner failures to retry and stale `running` runs to recover after worker crashes.
- Added `AGENT_PROVIDER` selection with deterministic rules, mock structured-output, and configurable OpenAI-compatible planners, preserving stable tests while supporting LLM-backed planning with fallback metrics.
- Integrated Agent tools for writing collaboration messages, creating follow-up tasks, upserting memory, and persisting outbox events for durable async delivery.
- Extended Agent context retrieval to include meeting transcript segments separately from 1:1 call transcript segments, improving source attribution for meeting summaries and follow-up reasoning.

## Reliability / Distributed Systems

- Added durable realtime event replay and explicit event sequence fields to reduce missed updates after WebSocket reconnects.
- Introduced an event outbox model and worker with idempotent enqueue semantics, request-id propagation, worker claim/lease fields, retry limits, configurable batch/interval controls, and publish/retry/failure metrics.
- Extracted a gRPC User Service boundary for internal access-token validation, enabling the connection-heavy signaling/API gateway to scale independently from user-center IO workloads.
- Added a Kafka-compatible room settlement pipeline: room-ended events are written to outbox, bridged to Kafka, and consumed by a Data Worker into idempotent `room_settlements`.
- Added an Elasticsearch-backed message search read model with async outbox indexing and post-search service-layer permission filtering.
- Propagated `X-Request-ID` through HTTP error responses, Agent runs, outbox rows, and async worker handlers for traceable backend diagnostics.
- Built recording lifecycle infrastructure with local/S3-compatible storage drivers, retention metadata, cleanup worker, download authorization, and support diagnostics.
- Decoupled meeting transcription from realtime translation, allowing recording-end transcription even when realtime translation UI is hidden.

## Performance Evidence

- Captured local Agent/outbox benchmark evidence: 25 queued runs, 25 ready runs, 0 failures, 75 processed outbox events, 175 tool calls, 50 RAG context chunks, and 26 ms execute-run p95 on temporary SQLite.
- Captured realtime replay benchmark evidence: 2000 persisted events, 100 replayed events, scoped replay correctness, monotonic IDs/sequences, and 3 ms write p95 on temporary SQLite.
- Captured authenticated WebSocket replay evidence: 5 clients, 500 total replayed events, 0 upgrade/client errors, 0 duplicates, and 9 ms connect-to-last p95 against the real `/api/v1/chat/ws` path.

## Interview Short Pitch

AllCallAll is a Go backend engineering portfolio project that combines realtime collaboration, WebRTC meeting infrastructure, durable event replay, recording storage/transcription, secure session management, and an explainable AI Agent pipeline. The project demonstrates backend system design rather than product launch or UI polish.
