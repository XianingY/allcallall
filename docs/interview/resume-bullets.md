# Resume Bullets

Use these as raw material. Pick 2-3 bullets and tune them for the target role.

## Backend / Realtime

- Built an AI-powered realtime collaboration backend in Go with Gin, Gorm/MySQL, Redis, WebSocket replay, WebRTC room signaling, and S3-compatible recording storage.
- Designed organization-scoped collaboration threads with replayable realtime events, explicit event sequence numbers, room state patches, recording lifecycle events, and conversation timeline recovery.
- Implemented refresh session hardening with HttpOnly refresh cookies, token rotation, suspicious refresh reuse tracking, logout-all, and support-side session inspection.

## AI Agent Engineering

- Implemented an explainable AI Agent execution model with persisted runs, intermediate steps, tool-call records, scoped memory, idempotency keys, metrics, and backend-controlled tool execution.
- Added a deterministic rules-based Agent provider and an OpenAI-compatible provider seam, enabling stable tests while preserving a clean path to LLM-backed planning.
- Integrated Agent tools for writing collaboration messages, creating follow-up tasks, upserting memory, and persisting outbox events for durable async delivery.

## Reliability / Distributed Systems

- Added durable realtime event replay and explicit event sequence fields to reduce missed updates after WebSocket reconnects.
- Introduced an event outbox model with idempotent enqueue semantics for reliable async event publishing after database commits.
- Built recording lifecycle infrastructure with local/S3-compatible storage drivers, retention metadata, cleanup worker, download authorization, and support diagnostics.

## Interview Short Pitch

AllCallAll is a Go backend portfolio project that combines realtime collaboration, WebRTC meeting infrastructure, durable event replay, recording storage, secure session management, and an explainable AI Agent pipeline. The project demonstrates backend system design rather than just product UI.
