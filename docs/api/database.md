# Database Model

This document describes the schema at the domain level. The authoritative migration list is `backend/internal/runtime/migrations.go`; model definitions live in `backend/internal/models`.

## Storage Overview

- Primary store: MySQL 8-compatible database.
- ORM: Gorm AutoMigrate for local/demo development.
- Cache/realtime support: Redis.
- Async backbone: `event_outbox`.
- Search/read model: Elasticsearch when `ELASTICSEARCH_URL` is configured.
- Recording bytes: local or S3-compatible object storage; MySQL stores metadata only.

## Identity And Security

| Table | Purpose |
| --- | --- |
| `users` | Accounts, email login, password hash, FCM token, status, soft deletion. |
| `refresh_sessions` | Refresh-cookie sessions, token hash, rotation, invalid reuse tracking, revoke state. |
| `email_verification_codes` | Purpose-scoped codes for register/password reset/deletion flows. |
| `email_send_logs` | Email send audit trail. |
| `push_devices` | Device token metadata for push-to-wake. |

Refresh tokens are stored as hashes. Service code consumes verification state instead of trusting client-side form flow.

## Contacts, Safety, Legal

| Table | Purpose |
| --- | --- |
| `contacts` | User-to-user relationship. |
| `contact_profiles` | Business metadata: company, role, timezone, language defaults, notes, relationship status. |
| `invitations` | Business contact invite links. |
| `user_blocks` | Block relationships used by search, contact, invite, and call paths. |
| `abuse_reports` | Structured user reports. |
| `legal_acceptances` | Terms/privacy acceptance records. |
| `deletion_audits` | Non-PII account deletion audit summary. |

## Organization And Collaboration

| Table | Purpose |
| --- | --- |
| `organizations` | Workspace/tenant root. |
| `organization_members` | User membership and role. |
| `teams`, `team_members` | Optional team grouping. |
| `organization_invites` | Workspace/team invite flow. |
| `organization_policies` | Recording mode, retention, export permission. |
| `conversations` | Collaboration thread with type, status, priority, assignee, contact binding. |
| `conversation_members` | Membership, read/mute state. |
| `conversation_notes` | Internal handoff notes. |
| `messages` | Text/system/call-event messages. |
| `message_reads` | Per-user read receipts. |
| `attachments` | Attachment metadata placeholder. |
| `chat_events` | Durable per-recipient realtime events for WebSocket replay. |

`organization_id` is the main tenant boundary for conversations, rooms, recordings, Agent runs, search, support diagnostics, and CRM-lite records.

## Meetings, Recordings, Transcription

| Table | Purpose |
| --- | --- |
| `call_rooms` | Meeting room metadata, optional conversation binding, status, creator. |
| `call_room_members` | Participant state, host flag, join/leave, audio/video, connection state. |
| `call_room_events` | Room lifecycle, media-state, recording, and ended events. |
| `recording_sessions` | Recording lifecycle for a room. |
| `recording_files` | Storage driver, bucket, object key, ETag, retention, soft delete, content metadata. |
| `recording_transcriptions` | One transcription job per recording session. Status: `pending`, `processing`, `ready`, `failed`, `skipped`. |
| `meeting_transcript_segments` | Recording transcript segments by conversation, room, recording, file, speaker/track, time range, provider. |
| `recording_consents` | Participant consent tracking hook. |
| `recording_exports` | Download/export audit metadata. |
| `room_settlements` | Kafka/data-worker demo output for room settlement records. |

`meeting_transcript_segments` intentionally stays separate from `call_transcript_segments`; the former is meeting recording transcription, the latter is older 1:1 call subtitle/follow-up material.

## AI Agent, Workflow, Knowledge

| Table | Purpose |
| --- | --- |
| `agent_runs` | ReAct-style Agent request, status, provider, idempotency key, result. |
| `agent_steps` | Explainable execution stages. |
| `agent_tool_calls` | Tool audit log with input/output/status. |
| `agent_memories` | Scoped memory by organization/user/conversation/key. |
| `agent_context_chunks` | Conversation RAG chunks from messages, notes, memories, and transcripts. |
| `agent_prompt_versions` | Prompt registry/version metadata. |
| `tool_schema_versions` | Tool schema version metadata. |
| `workflow_runs` | Workflow/DAG Agent run state. |
| `workflow_tasks` | Workflow task graph nodes. |
| `workflow_history_events` | Workflow event history. |
| `workflow_signals`, `workflow_timers` | Workflow coordination state. |
| `agent_messages` | Agent Lab message records. |
| `tool_policies` | Tool policy metadata. |
| `tool_approvals` | Human-in-the-loop approval records. |
| `rag_source_groups` | Exact/near-duplicate source grouping. |
| `rag_source_duplicates` | Duplicate candidate decisions. |
| `rag_sources` | Imported text/URL/file source metadata. |
| `rag_source_versions` | Version/canonical source state. |
| `rag_chunks` | Knowledge chunks and vector/search metadata. |

The Agent context loader now includes up to 80 `meeting_transcript_segments` for the current conversation and prioritizes the latest recording session.

## Calls, Follow-Ups, Usage, Billing

| Table | Purpose |
| --- | --- |
| `call_sessions` | 1:1 call lifecycle. |
| `call_transcript_segments` | Final 1:1 transcript/translation segments, not raw audio. |
| `call_followups` | Rules-based follow-up cards. |
| `follow_up_tasks` | Callback/send-message/schedule-next-call tasks. |
| `translation_usage_slices` | Idempotent 30-second usage slices. |
| `usage_ledgers` | Usage/quota ledger. |
| `user_entitlements` | Entitlement state. |
| `billing_webhook_events` | Idempotent webhook audit events. |

Billing and realtime translation remain supporting modules, not the main portfolio narrative.

## CRM-Lite

| Table | Purpose |
| --- | --- |
| `pipelines` | Organization-level pipeline. |
| `pipeline_stages` | Stages such as new, qualified, meeting scheduled, won, lost. |
| `deals` | Lightweight deal record. |
| `deal_contacts` | Deal-contact association. |
| `deal_activities` | Activity stream from calls, meetings, recordings, and follow-ups. |

## Durable Outbox

| Table | Purpose |
| --- | --- |
| `event_outbox` | Transactional event outbox with status, attempts, retry time, lease, request ID, last error. |

Important events include:

- `agent.run.requested`
- `workflow.run.requested`
- `agent.run.completed`
- `message.created`
- `search.message.index_requested`
- `rag.source.ingest_requested`
- `rag.chunk.index_requested`
- `settlement.room.ended`
- `recording.transcription.requested`

## Useful Inspection Queries

```sql
SELECT status, COUNT(*) AS count, MIN(created_at) AS oldest_created_at
FROM event_outbox
GROUP BY status;

SELECT id, organization_id, user_id, sequence, event, created_at
FROM chat_events
ORDER BY id DESC
LIMIT 20;

SELECT status, provider, COUNT(*) AS count
FROM recording_transcriptions
GROUP BY status, provider;

SELECT recording_session_id, recording_file_id, track_key, start_ms, end_ms, LEFT(text, 120) AS preview
FROM meeting_transcript_segments
ORDER BY id DESC
LIMIT 20;

SELECT status, source, COUNT(*) AS count
FROM agent_runs
GROUP BY status, source;
```

## Migration Notes

The repo currently uses Gorm AutoMigrate for development and portfolio demos. For production, the next step would be explicit versioned migrations. Until then, update `backend/internal/runtime/migrations.go` whenever a model must be materialized in local/dev databases.
