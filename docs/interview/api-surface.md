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
- `sequence` makes ack/replay semantics explicit while preserving existing `event_id`.

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

- `run`: status, source, summary, action items, next step, risk flags
- `steps`: explainable intermediate stages
- `tool_calls`: backend-controlled side effects

Talking points:

- Provider seam: rules now, OpenAI-compatible later.
- Tool calling is persisted and permission-guarded.
- Memory is scoped to organization/user/conversation.
- Outbox event is written for durable async delivery.
