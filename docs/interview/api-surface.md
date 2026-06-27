# Interview API Surface

This page lists APIs worth showing in a backend interview. For the full route map, use [API Documentation](../api/api-documentation.md).

## Auth And Sessions

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout-all`
- `GET /api/v1/auth/sessions`
- `DELETE /api/v1/auth/sessions/:sessionID`

Talking points:

- Short-lived access token plus refresh session rotation.
- Refresh reuse detection and logout-all.
- HttpOnly refresh-cookie support for Web.

## Collaboration And Realtime

- `GET /api/v1/conversations`
- `POST /api/v1/conversations`
- `GET /api/v1/conversations/:id`
- `PATCH /api/v1/conversations/:id`
- `GET /api/v1/conversations/:id/messages`
- `POST /api/v1/conversations/:id/messages`
- `GET /api/v1/conversations/:id/notes`
- `POST /api/v1/conversations/:id/notes`
- `GET /api/v1/chat/ws`
- `GET /api/v1/search/messages?q=<keyword>`

Talking points:

- Organization-scoped access control.
- Durable `chat_events` replay with `event_id`, `sequence`, and `since_id`.
- Message search is eventually consistent and membership-filtered after Elasticsearch hits.

## Meetings, Recording, Transcription

- `POST /api/v1/rooms`
- `GET /api/v1/rooms`
- `GET /api/v1/rooms/:roomId/state`
- `POST /api/v1/rooms/:roomId/join`
- `POST /api/v1/rooms/:roomId/leave`
- `POST /api/v1/rooms/:roomId/offer`
- `POST /api/v1/rooms/:roomId/ice`
- `POST /api/v1/rooms/:roomId/media`
- `POST /api/v1/rooms/:roomId/recording/start`
- `POST /api/v1/rooms/:roomId/recording/stop`
- `GET /api/v1/recordings`
- `GET /api/v1/recordings/:id`
- `GET /api/v1/recordings/:id/transcript`
- `POST /api/v1/recordings/:id/transcription/retry`
- `GET /api/v1/recordings/:id/files/:fileId`

Talking points:

- Recording policy is organization-scoped.
- `recording_files` stores object metadata; bytes live in local/S3-compatible storage.
- Stopping a recording can enqueue `recording.transcription.requested`.
- `recording_transcriptions` tracks `pending`, `processing`, `ready`, `failed`, or `skipped`.
- `meeting_transcript_segments` become Agent-retrievable context after transcription succeeds.
- This is independent of realtime translation; translation UI is currently hidden.

## AI Agent And Agent Lab

- `POST /api/v1/agent/runs`
- `GET /api/v1/agent/runs/:id`
- `GET /api/v1/agent/runs/:id/events`
- `GET /api/v1/agent/runs/:id/events/stream`
- `POST /api/v1/agent/workflows`
- `GET /api/v1/agent/workflows`
- `GET /api/v1/agent/workflows/:id`
- `POST /api/v1/agent/workflows/:id/process`
- `GET /api/v1/agent/approvals`
- `POST /api/v1/agent/approvals/:id/decision`

Talking points:

- ReAct-style runs are async: handler creates a pending run and outbox event; worker executes it.
- Workflow runs support DAG-style task execution and human approvals.
- Tool calls are persisted, permission-labeled, and backend-executed.
- `Idempotency-Key` prevents duplicate side effects on retry.
- Provider seam: `rules`, `mock_llm`, `openai_compatible`.
- Agent context includes messages, notes, members, rooms, contact profile, memory, call transcripts, meeting transcripts, and knowledge chunks.

## Knowledge Base

- `POST /api/v1/knowledge/sources`
- `GET /api/v1/knowledge/sources`
- `GET /api/v1/knowledge/sources/:id`
- `POST /api/v1/knowledge/sources/:id/reingest`
- `GET /api/v1/knowledge/source-groups`
- `POST /api/v1/knowledge/source-groups/:id/canonical`
- `GET /api/v1/knowledge/duplicate-candidates`
- `POST /api/v1/knowledge/duplicate-candidates/:id/decision`
- `GET /api/v1/knowledge/dead-letters`
- `POST /api/v1/knowledge/dead-letters/:id/retry`

Talking points:

- Source import supports versioning and duplicate/canonical management.
- Ingest/chunk-index work is outbox-driven.
- Elasticsearch can back chunk indexing/search when configured.

## Internal Boundaries

gRPC:

- `allcallall.user.v1.UserService/ValidateAccessToken`
- `allcallall.user.v1.UserService/GetUser`

Workers:

- `cmd/user-service`
- `cmd/agent-worker`
- `cmd/outbox-worker`
- `cmd/data-worker`
- `cmd/search-worker`
- `cmd/cleanup-worker`

Talking points:

- gRPC is a narrow synchronous boundary for auth/user lookup.
- Kafka is used for bursty room-settlement side effects.
- Elasticsearch is an eventually consistent read model.
- Outbox workers own async side effects including recording transcription.
