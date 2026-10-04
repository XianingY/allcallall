# AllCallAll API Documentation

This is the maintained human-readable API map for the current codebase. Routes are registered from `backend/internal/server/routes.go` and `backend/internal/handlers/*`.

## Conventions

- Base URL: `http://localhost:8080` locally.
- API prefix: `/api/v1`.
- Protected endpoints use `Authorization: Bearer <access_token>`.
- Collaboration endpoints use organization membership and often `X-Organization-ID`.
- Error responses keep the legacy `error` field and add `code` plus `request_id` on hardened paths.
- `/api/v1/chat/ws` is durable collaboration replay. `/api/v1/ws` and `/api/v1/signaling/*` are 1:1 signaling.

## Health

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/health` | Service health. |
| `GET` | `/api/v1/ready` | Readiness check for MySQL, Redis, and required provider configuration. |
| `GET` | `/api/v1/metrics` | Prometheus-style counters when metrics are wired. |

## Auth, Email, Users

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/v1/auth/register` | Register with verified email. |
| `POST` | `/api/v1/auth/login` | Login and issue access/refresh credentials. |
| `POST` | `/api/v1/auth/refresh` | Rotate refresh session and issue a new access token. |
| `POST` | `/api/v1/auth/logout` | Revoke current refresh session. |
| `POST` | `/api/v1/auth/logout-all` | Revoke all refresh sessions for current user. |
| `GET` | `/api/v1/auth/sessions` | List current user's refresh sessions. |
| `DELETE` | `/api/v1/auth/sessions/:sessionID` | Revoke one owned refresh session. |
| `POST` | `/api/v1/auth/password-reset/send` | Send password-reset verification. |
| `POST` | `/api/v1/auth/password-reset/confirm` | Reset password with verification. |
| `POST` | `/api/v1/email/send-verification-code` | Send purpose-scoped code. |
| `POST` | `/api/v1/email/verify-code` | Verify purpose-scoped code. |
| `GET` | `/api/v1/users/me` | Current user profile. |
| `GET` | `/api/v1/users/search` | User search with block/rate-limit checks. |
| `GET` | `/api/v1/users/presence` | Presence lookup. |
| `POST` | `/api/v1/users/change-password` | Change password. |
| `POST` | `/api/v1/users/fcm-token` | Register push token. |

## Push Devices

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/push/devices` | List current user's registered browser/mobile push devices. |
| `POST` | `/api/v1/push/devices` | Register or refresh a push device token. |
| `DELETE` | `/api/v1/push/devices/:deviceId` | Remove a push device registration. |

## Contacts And Invitations

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/users/contacts` | List contacts. |
| `POST` | `/api/v1/users/contacts` | Add contact. |
| `DELETE` | `/api/v1/users/contacts/:id` | Remove contact. |
| `GET` | `/api/v1/users/contacts/:id/profile` | Read business contact profile. |
| `PUT` | `/api/v1/users/contacts/:id/profile` | Upsert business contact profile. |
| `GET` | `/api/v1/invitations/:code` | Read invitation details. |
| `POST` | `/api/v1/invitations` | Create invitation. |
| `POST` | `/api/v1/invitations/:code/accept` | Accept invitation. |

## Organizations And Conversations

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/v1/organizations` | Create workspace. |
| `GET` | `/api/v1/organizations` | List user's workspaces. |
| `POST` | `/api/v1/organizations/:id/switch` | Switch active workspace. |
| `POST` | `/api/v1/organizations/:id/invites` | Invite workspace member. |
| `POST` | `/api/v1/organizations/invites/:code/accept` | Accept workspace invite. |
| `GET` | `/api/v1/organizations/:id/policy` | Read recording policy. |
| `PUT` | `/api/v1/organizations/:id/policy` | Update recording policy. |
| `GET` | `/api/v1/conversations` | List collaboration threads. |
| `POST` | `/api/v1/conversations` | Create direct/channel/meeting thread. |
| `GET` | `/api/v1/conversations/:id` | Read conversation workspace detail. |
| `PATCH` | `/api/v1/conversations/:id` | Update status, priority, assignee, contact binding. |
| `GET` | `/api/v1/conversations/:id/messages` | Page messages. |
| `POST` | `/api/v1/conversations/:id/messages` | Send text/system/call-event message. |
| `POST` | `/api/v1/conversations/:id/read` | Mark conversation read. |
| `GET` | `/api/v1/conversations/:id/notes` | List internal notes. |
| `POST` | `/api/v1/conversations/:id/notes` | Create internal note. |
| `POST` | `/api/v1/conversations/:id/rooms` | Create meeting room from thread. |
| `GET` | `/api/v1/chat/ws` | Durable collaboration WebSocket with replay. |
| `GET` | `/api/v1/search/messages?q=...` | Message search; ES-backed when configured. |

Conversation detail includes Agent context metadata, including meeting transcription status/count when available.

## Meetings, Signaling, Recording

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/webrtc/config` | Return ICE/TURN config. |
| `POST` | `/api/v1/realtime/tickets` | Issue a short-lived browser WebSocket ticket for `chat` or `signaling`. |
| `GET` | `/api/v1/ws` | 1:1 signaling WebSocket. |
| `POST` | `/api/v1/signaling/send` | Polling signaling send fallback. |
| `GET` | `/api/v1/signaling/poll` | Polling signaling receive fallback. |
| `GET` | `/api/v1/translation/ws` | Realtime translation compatibility endpoint; UI currently hidden. |
| `POST` | `/api/v1/rooms` | Create meeting room. |
| `GET` | `/api/v1/rooms` | List rooms. |
| `POST` | `/api/v1/rooms/:roomId/join` | Join room. |
| `POST` | `/api/v1/rooms/:roomId/leave` | Leave room. |
| `POST` | `/api/v1/rooms/:roomId/offer` | Submit WebRTC offer. |
| `POST` | `/api/v1/rooms/:roomId/ice` | Submit ICE candidate. |
| `POST` | `/api/v1/rooms/:roomId/media` | Patch audio/video/connection state. |
| `GET` | `/api/v1/rooms/:roomId/state` | Read room snapshot. |
| `POST` | `/api/v1/rooms/:roomId/recording/start` | Start recording if policy allows. |
| `POST` | `/api/v1/rooms/:roomId/recording/stop` | Stop recording and persist artifacts. |
| `GET` | `/api/v1/recordings` | List accessible recordings. |
| `GET` | `/api/v1/recordings/:id` | Read recording detail and transcription status. |
| `GET` | `/api/v1/recordings/:id/transcript` | Page through meeting transcript segments with `after_id` and `limit`. |
| `POST` | `/api/v1/recordings/:id/transcription/retry` | Owner/admin retry for a failed transcription. |
| `GET` | `/api/v1/recordings/:id/files/:fileId` | Download local file or redirect to signed URL. |

When transcription is enabled, recording stop enqueues `recording.transcription.requested`. The outbox worker writes `meeting_transcript_segments`; failures do not roll back recording persistence.

## AI Agent, Workflow, Knowledge

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/v1/agent/runs` | Create auditable ReAct-style conversation Agent run. |
| `GET` | `/api/v1/agent/runs/:id` | Read run, steps, tool calls, trace. |
| `GET` | `/api/v1/agent/runs/:id/events` | Poll persisted run event timeline. |
| `GET` | `/api/v1/agent/runs/:id/events/stream` | SSE stream backed by persisted rows. |
| `POST` | `/api/v1/agent/runs/:id/submit-tool-outputs` | Submit tool outputs when required. |
| `POST` | `/api/v1/agent/workflows` | Create workflow/DAG Agent run. |
| `GET` | `/api/v1/agent/workflows` | List workflow runs. |
| `GET` | `/api/v1/agent/workflows/:id` | Read workflow run graph/state. |
| `POST` | `/api/v1/agent/workflows/:id/process` | Process workflow run. |
| `GET` | `/api/v1/agent/approvals` | List human approvals. |
| `POST` | `/api/v1/agent/approvals/:id/decision` | Approve or reject tool action. |
| `POST` | `/api/v1/knowledge/sources` | Create/import knowledge source. |
| `GET` | `/api/v1/knowledge/sources` | List knowledge sources. |
| `GET` | `/api/v1/knowledge/sources/:id` | Read source detail. |
| `POST` | `/api/v1/knowledge/sources/:id/reingest` | Re-run source ingestion. |
| `GET` | `/api/v1/knowledge/source-groups` | List duplicate/version groups. |
| `GET` | `/api/v1/knowledge/source-groups/:id` | Read source group. |
| `POST` | `/api/v1/knowledge/source-groups/:id/canonical` | Set canonical version. |
| `GET` | `/api/v1/knowledge/duplicate-candidates` | List duplicate candidates. |
| `POST` | `/api/v1/knowledge/duplicate-candidates/:id/decision` | Accept/reject duplicate candidate. |
| `GET` | `/api/v1/knowledge/dead-letters` | List failed ingestion jobs. |
| `POST` | `/api/v1/knowledge/dead-letters/:id/retry` | Retry failed ingestion. |

The Agent loads messages, notes, memories, contact profile, call follow-ups, 1:1 transcript segments, and meeting recording transcript segments. Retrieved references distinguish `meeting_transcript` from older `call_transcript` sources.

## CRM-Lite, Follow-Ups, Commercial Support

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/pipelines` | List pipelines. |
| `GET` | `/api/v1/deals` | List deals. |
| `POST` | `/api/v1/deals` | Create deal. |
| `GET` | `/api/v1/deals/:id` | Read deal. |
| `PATCH` | `/api/v1/deals/:id` | Update deal. |
| `POST` | `/api/v1/deals/:id/contacts` | Attach contact. |
| `GET` | `/api/v1/deals/:id/activities` | List deal activities. |
| `GET` | `/api/v1/calls/history` | 1:1 call history. |
| `GET` | `/api/v1/calls/:callId/followup` | Read follow-up summary. |
| `POST` | `/api/v1/calls/:callId/followup/generate` | Generate follow-up. |
| `POST` | `/api/v1/calls/:callId/followup/regenerate` | Regenerate follow-up. |
| `GET` | `/api/v1/follow-ups` | List tasks. |
| `POST` | `/api/v1/follow-ups` | Create task. |
| `PATCH` | `/api/v1/follow-ups/:taskId` | Update task. |
| `GET` | `/api/v1/entitlements/me` | Entitlement snapshot. |
| `GET` | `/api/v1/usage/me` | Usage/quota snapshot. |
| `POST` | `/api/v1/billing/revenuecat/webhook` | RevenueCat webhook. |

## Safety, Legal, Support

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/legal/terms` | Public terms page. |
| `GET` | `/legal/privacy` | Public privacy page. |
| `GET` | `/legal/delete-account` | Public deletion page. |
| `GET` | `/invite/:code` | Public invite landing page. |
| `GET` | `/api/v1/legal/current` | Current legal versions. |
| `POST` | `/api/v1/legal/accept` | Record acceptance. |
| `POST` | `/api/v1/users/blocks` | Block user. |
| `GET` | `/api/v1/users/blocks` | List blocks. |
| `DELETE` | `/api/v1/users/blocks/:blockedUserId` | Remove block. |
| `POST` | `/api/v1/users/reports` | Create report. |
| `POST` | `/api/v1/users/me/deletion` | Request account deletion. |
| `GET` | `/api/v1/internal/support/reports` | Support report list. |
| `GET` | `/api/v1/internal/support/users/:userId/summary` | Support user summary. |
| `POST` | `/api/v1/internal/support/users/:userId/sessions/revoke-all` | Revoke all sessions. |
| `DELETE` | `/api/v1/internal/support/users/:userId/sessions/:sessionId` | Revoke one session. |
| `GET` | `/api/v1/internal/support/calls/:callId` | Support call detail. |
| `GET` | `/api/v1/internal/support/rooms/:roomId` | Support room detail. |
| `GET` | `/api/v1/internal/support/recordings/:id` | Support recording detail. |

## Worker Boundaries

- `cmd/user-service`: gRPC token validation and user lookup.
- `cmd/agent-worker`: `agent.run.requested` and workflow processing.
- `cmd/outbox-worker`: collaboration events, knowledge ingest/chunk index, recording transcription, optional Kafka bridge.
- `cmd/data-worker`: Kafka settlement consumer.
- `cmd/search-worker`: Elasticsearch indexing from outbox events.
- `cmd/cleanup-worker`: refresh sessions and recording retention.
