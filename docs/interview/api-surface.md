# Interview API Surface

This document lists the API areas worth showing in a backend interview. It intentionally focuses on backend engineering depth rather than product breadth.

## Auth And Sessions

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout-all`
- `GET /api/v1/auth/sessions`
- `DELETE /api/v1/auth/sessions/:id`

Talking points:

- Short-lived access token plus refresh session rotation.
- Refresh reuse detection.
- Logout-all for account recovery and device risk control.

## Collaboration Threads

- `GET /api/v1/conversations`
- `POST /api/v1/conversations`
- `GET /api/v1/conversations/:id`
- `PATCH /api/v1/conversations/:id`
- `GET /api/v1/conversations/:id/messages`
- `POST /api/v1/conversations/:id/messages`
- `GET /api/v1/conversations/:id/notes`
- `POST /api/v1/conversations/:id/notes`

Talking points:

- Organization-scoped access control.
- Conversation state, priority, assignee, notes, and system event messages.
- Local patch events instead of full-page reloads.

## Realtime

Collaboration event stream:

- `GET /api/v1/chat/ws`

Realtime payload fields:

- `event_id`
- `sequence`
- `event`
- `organization_id`
- `payload`
- `created_at`

Replay behavior:

- The client reconnects with `since_id`.
- The backend reads durable `chat_events` where `id > since_id`.
- `sequence` makes ack/replay semantics explicit while preserving existing `event_id`. Today it mirrors the persisted event ID, which keeps replay ordering durable and easy to reason about.

WebRTC signaling:

- `GET /api/v1/ws`
- `POST /api/v1/signaling/send`
- `GET /api/v1/signaling/poll`

Talking points:

- Chat replay and WebRTC signaling are separate realtime concerns.
- Polling signaling is a proxy-friendly fallback when WebSocket signaling is not reliable.

## Meetings And Rooms

- `GET /api/v1/rooms`
- `POST /api/v1/rooms`
- `GET /api/v1/rooms/:roomId/state`
- `POST /api/v1/rooms/:roomId/join`
- `POST /api/v1/rooms/:roomId/leave`
- `POST /api/v1/rooms/:roomId/offer`
- `POST /api/v1/rooms/:roomId/ice`
- `POST /api/v1/rooms/:roomId/media`

Talking points:

- Room state model.
- Member media state synchronization.
- WebRTC offer/ICE path.
- Meeting events are tied back into conversation threads.

## Recordings

- `POST /api/v1/rooms/:roomId/recording/start`
- `POST /api/v1/rooms/:roomId/recording/stop`
- `GET /api/v1/recordings`
- `GET /api/v1/recordings/:id`
- `GET /api/v1/recordings/:id/files/:fileId`

Talking points:

- Local and S3-compatible recording storage.
- Retention metadata and cleanup worker.
- Download authorization and organization boundary checks.

## AI Agent

- `POST /api/v1/agent/runs`
- `GET /api/v1/agent/runs/:id`

Required headers:

- `Authorization: Bearer <access_token>`
- `X-Organization-ID: <organization_id>`
- Optional `Idempotency-Key: <stable_retry_key>`

Response shape:

- `run`: status, source, idempotency key, summary, action items, next step, risk flags
- `steps`: explainable intermediate stages
- `tool_calls`: backend-controlled read-only context calls and mutating side effects

Talking points:

- `POST /agent/runs` returns `202 Accepted` with a `pending` run, then `agent.run.requested` is drained by the outbox worker.
- Provider seam: `AGENT_PROVIDER=rules` for deterministic demos; `AGENT_PROVIDER=mock_llm` for prompt + structured-output parsing demos; `AGENT_PROVIDER=openai_compatible` is wired as an unavailable provider that falls back to `rules` during service execution.
- Tool calling is persisted and permission-guarded.
- Memory is scoped to organization/user/conversation.
- Repeating a request with the same `Idempotency-Key` returns the existing run result instead of duplicating tool side effects.
- Outbox events `agent.run.requested`, `agent.run.completed`, and `message.created` give a durable async delivery path that can later be swapped for Kafka or Redis Streams.
