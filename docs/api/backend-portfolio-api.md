# Backend Portfolio API Surface

This Markdown API guide is optimized for backend interviews. It highlights the endpoints that demonstrate realtime systems, WebRTC signaling, session security, recording storage, and AI Agent engineering.

## Common Headers

```http
Authorization: Bearer <access_token>
X-Organization-ID: <organization_id>
Content-Type: application/json
```

For retry-safe Agent calls:

```http
Idempotency-Key: <stable-client-generated-key>
```

## Health And Metrics

### `GET /api/v1/health`

Checks service health.

### `GET /api/v1/metrics`

Returns Prometheus-style counters. Interview-focused counters include:

- `agent_run_queued_total`
- `agent_run_started_total`
- `agent_run_total`
- `agent_run_failed_total`
- `agent_tool_call_total`
- `agent_memory_write_total`
- `outbox_publish_total`
- `outbox_publish_retry_total`
- `outbox_publish_failed_total`
- `chat_realtime_delivery_fail_total`

## Auth And Refresh Sessions

### `POST /api/v1/auth/login`

Returns an access token and, for Web clients, can issue an HttpOnly refresh cookie.

### `POST /api/v1/auth/refresh`

Rotates refresh sessions and rejects suspicious reuse.

### `POST /api/v1/auth/logout-all`

Revokes all refresh sessions for the authenticated user.

### `GET /api/v1/auth/sessions`

Lists current refresh sessions for device/session management.

### `DELETE /api/v1/auth/sessions/:id`

Revokes one refresh session.

## Realtime Collaboration

### `GET /api/v1/chat/ws`

WebSocket endpoint for collaboration events.

Event payload:

```json
{
  "event_id": 123,
  "sequence": 123,
  "event": "message.created",
  "organization_id": 1,
  "payload": {},
  "created_at": "2026-06-09T00:00:00Z"
}
```

Design note: `event_id` remains backward compatible; `sequence` makes replay/ack semantics explicit.

## Conversations

### `GET /api/v1/conversations`

Lists organization-scoped collaboration threads.

### `POST /api/v1/conversations`

Creates a thread.

### `GET /api/v1/conversations/:id`

Returns thread detail, workspace summary, latest meeting, latest recording, latest note, and follow-up summary.

### `PATCH /api/v1/conversations/:id`

Updates collaboration fields such as status, assignee, priority, and contact binding.

### `GET /api/v1/conversations/:id/messages`

Lists thread messages.

### `POST /api/v1/conversations/:id/messages`

Creates a text/system/call-event message.

## Rooms And WebRTC Signaling

### `POST /api/v1/rooms`

Creates a meeting room.

### `GET /api/v1/rooms/:roomId/state`

Returns room state, member status, recording status, and media state.

### `POST /api/v1/rooms/:roomId/join`

Joins a room.

### `POST /api/v1/rooms/:roomId/leave`

Leaves a room.

### `POST /api/v1/rooms/:roomId/offer`

Submits a WebRTC offer and receives an answer.

### `POST /api/v1/rooms/:roomId/ice`

Submits ICE candidates.

### `POST /api/v1/rooms/:roomId/media`

Updates member media state such as audio/video enabled and connection state.

## Recordings

### `POST /api/v1/rooms/:roomId/recording/start`

Starts recording according to organization policy.

### `POST /api/v1/rooms/:roomId/recording/stop`

Stops recording and creates recording files/assets.

### `GET /api/v1/recordings`

Lists recording assets.

### `GET /api/v1/recordings/:id/files/:fileId`

Downloads a recording file after organization and permission checks.

## AI Agent

### `POST /api/v1/agent/runs`

Creates or returns an idempotent Agent run. The endpoint returns `202 Accepted`; execution is asynchronous. A newly created run starts as `pending`, is enqueued as `agent.run.requested`, and is executed by the outbox worker. Clients poll `GET /api/v1/agent/runs/:id` until the run reaches `ready` or `failed`.

Request:

```json
{
  "conversation_id": 100,
  "goal": "summarize current support handoff"
}
```

Response:

```json
{
  "run": {
    "id": 1,
    "source": "rules",
    "status": "pending",
    "goal": "summarize current support handoff",
    "summary": "",
    "action_items": [],
    "next_step": "",
    "risk_flags": []
  },
  "steps": [],
  "tool_calls": []
}
```

When the worker completes, the same run exposes `status=ready`, `steps`, and `tool_calls`.

Current tools:

- `query_recent_meetings`
- `query_conversation_members`
- `query_contact_profile`
- `write_conversation_message`
- `create_follow_up_task`
- `upsert_agent_memory`

Durable events:

- `agent.run.requested`
- `agent.run.completed`
- `message.created`

### `GET /api/v1/agent/runs/:id`

Loads one Agent run with steps and tool calls.

## Demo Seed

```bash
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/interview-seed
```

The seed command creates demo users, organization, conversation, meeting, contact profile, messages, and an idempotent Agent run.
