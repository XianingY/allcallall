# Beta Smoke Checklist

This checklist is the maintained Beta v1 product-readiness path for a 3-6 person team. It validates the practical loop:

`organization -> invite -> chat -> meeting -> recording -> transcription -> Agent meeting brief -> citation -> approval write-back`.

## Scope

In scope:

- One API node with embedded or standalone workers.
- One media node and rooms capped at 6 participants.
- Primary browser client in `web/`.
- Local or S3-compatible recording storage.
- OpenAI-compatible ASR for real Beta transcription.
- OpenAI-compatible Agent provider with strict mode for real Beta recap.

Out of scope for this checklist:

- Public SaaS launch readiness.
- SSO/SCIM, complex enterprise RBAC, K8s, multi-media-node failover.
- Transcript editing and audit-versioning.
- RevenueCat and Web Push commercial/operational validation.

## Required Beta Provider Settings

Real Beta validation should not use mock transcription or rules fallback:

```bash
TRANSCRIPTION_ENABLED=true
TRANSCRIPTION_PROVIDER=openai_compatible
TRANSCRIPTION_OPENAI_BASE_URL=https://api.example.com/v1
TRANSCRIPTION_OPENAI_MODEL=whisper-1
TRANSCRIPTION_OPENAI_API_KEY=...
TRANSCRIPTION_CHUNK_SECONDS=600
TRANSCRIPTION_MAX_UPLOAD_BYTES=25165824
TRANSCRIPTION_FFMPEG_PATH=ffmpeg

AGENT_PROVIDER=openai_compatible
AGENT_PROVIDER_STRICT=true
AGENT_OPENAI_BASE_URL=https://api.example.com/v1
AGENT_OPENAI_MODEL=...
AGENT_OPENAI_API_KEY=...
```

Use `TRANSCRIPTION_PROVIDER=mock` and `AGENT_PROVIDER=rules` only for deterministic local demos, eval, and seed-data walkthroughs.

## Optional Local Seed

Seed a deterministic local workspace:

```bash
make beta-seed
```

The seed creates:

- Owner and member login accounts.
- One organization and one team.
- One conversation with sample messages, a pinned message, a reaction, and an internal note.
- One ended meeting linked to the conversation.
- One recording session with ready transcript segments.
- A pending invite and organization audit events.

The seed does not create real audio bytes. It is useful for UI and Agent grounding walkthroughs, not for ASR quality validation.

## Product Smoke

1. Auth and organization:
   - Login as owner.
   - Confirm the Beta organization appears.
   - Open organization console tabs: Overview, Members, Invites, Teams, Policies, Audit.
   - Invite a new member, resend invite, revoke invite.
   - Change a member role as owner.
   - Confirm last owner cannot be downgraded or removed.

2. Member access:
   - Login as member in a second browser context.
   - Confirm member can view organization members and teams.
   - Confirm member cannot modify invites, teams, roles, or policies.

3. Chat:
   - Open the seeded conversation or create a new one.
   - Send a message with optimistic UI.
   - Reply to a message.
   - Add and remove a reaction.
   - Edit own text message.
   - Soft-delete/revoke own message.
   - Pin and unpin an important message.
   - Upload and download an allowed attachment through authenticated backend routes.
   - Refresh the page and confirm messages replay without duplicates.

4. Realtime:
   - Keep two browser contexts open in the same conversation.
   - Confirm created/updated/deleted/reaction/pin events appear in the other context.
   - Confirm typing state appears and expires without being persisted in history.
   - Disconnect/reconnect one context and confirm missed durable events are replayed.

5. Meeting:
   - Start a meeting from the conversation context panel.
   - Join from owner and member contexts.
   - Confirm participant count stays within the 6-person room limit.
   - Start and stop recording.
   - Confirm the recording remains saved even if transcription later fails.

6. Transcription:
   - Confirm recording status moves through pending/processing to ready.
   - Open the recording transcript page.
   - Confirm segments show speaker/track, time range, language/confidence when present, and transcript text.
   - If transcription fails, confirm owner/admin can retry and member cannot.

7. Agent meeting brief:
   - From the conversation context panel, click "生成会议复盘" only after transcript status is ready.
   - Confirm `meeting_brief` refuses to run when no ready transcript exists.
   - Confirm output contains summary, risks, action items, citations, and pending write tools.
   - Confirm citations include `meeting_transcript` and link to the transcript timeline with segment/time parameters.

8. Approval:
   - Confirm read tools execute automatically.
   - Confirm write tools such as message write-back, follow-up creation, and memory write enter approval.
   - Approve one request and verify the side effect.
   - Reject one request and verify no side effect is committed.

## Deployable Checks

```bash
cd backend && go test ./internal/collaboration ./internal/handlers ./internal/server
cd backend && go test ./cmd/chat-ws-replay-bench ./cmd/beta-seed
cd web && npm run typecheck && npm run lint && npm run test && npm run build
docker compose -f infra/docker-compose.production.yml config
```

Run broader backend checks before a release candidate:

```bash
cd backend && go test -count=1 ./...
cd backend && go vet ./...
```

Web Push and Billing pages can remain visible in Beta, but they require Firebase/RevenueCat runtime config and are not core acceptance gates for this product loop.
