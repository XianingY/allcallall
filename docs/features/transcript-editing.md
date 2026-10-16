# Transcript Editing

Status: implemented. This page records the shipped behavior and the decisions
the implementation followed.

## Problem

`meeting_transcript_segments` are append-only rows written by transcription
providers. Speakers cannot fix provider mistakes (wrong words, misattributed
speakers), so the transcript used for agent context, citations, and export is
only as good as the provider. The Web feature matrix currently marks
transcript editing "Not implemented — deferred to avoid versioning/audit model
expansion"; this design is that expansion, kept deliberately minimal.

## Data model

- New append-only table `transcript_segment_revisions`:
  `id, organization_id, segment_id (FK meeting_transcript_segments),
  revision_number (1-based, unique per segment), previous_text, next_text,
  reason (≤512 chars, optional), superseded, actor_user_id, created_at`.
  Hashes are not enough: the audit trail must reconstruct any prior state.
- `meeting_transcript_segments` gains `edited_at *time.Time` (null = provider
  text) and `editor_user_id *uint64`. The `text` column stays the "current"
  value; a single UPDATE plus revision INSERT run in one transaction.
- Transcription re-runs replace segments wholesale (delete + recreate with
  fresh provider rows, new IDs). Before deleting, the ingest appends a
  `superseded` revision (`previous_text` = the manual text, `next_text` =
  empty) for every manually edited row, so history never disappears even
  though the new provider rows have no mapping to the old revision chain.

## Permissions

- Organization owner/admin, or the segment's `speaker_user_id` fixing their
  own words. Regular members cannot edit other speakers.
- Permission is evaluated on the backend on every request; the Web only uses
  it to hide the affordance.

## API

- `PATCH /api/v1/recordings/:recording_id/transcript/segments/:segment_id`
  with `{ text, reason? }` (text required, ≤4096 chars). Identical text is a
  200 no-op. Returns the updated segment plus `revision_number`.
- `GET  /api/v1/recordings/:recording_id/transcript/segments/:segment_id/revisions`
  — newest first, paginated.
- Every accepted edit emits outbox event `transcript.segment.edited`
  (recipients: open transcript viewers) and an `OrganizationAuditEvent`
  with action `transcript.segment.edited`, target
  `meeting_transcript_segment:<id>`, metadata carrying recording id,
  revision number, and SHA-256 of previous/next text (full text lives only
  in the revision table).

## Agent and citation impact

- Agent context continues loading current segment text; citations stay
  attached to `segment_id`, which never changes, so existing citations do
  not break.
- `edited_at` flows into the context manifest: an edited segment lowers
  grounding confidence the same way low provider confidence does, because
  edited text was never spoken verbatim.

## Web UI

- Recordings transcript page: an edit affordance on each segment for
  permitted users, a modal with a before/after diff and optional reason, and
  a revision-history drawer per segment. Editing is disabled while the
  transcription status is not `ready`.

## Tests and rollout

- Backend: permission matrix (admin / speaker / other member / cross-org),
  no-op edit, revision chain integrity, re-run supersession, audit event and
  outbox emission, MySQL migration idempotency.
- Web: contract tests for the two endpoints, diff modal rendering, and
  permission-gated visibility.
- Contract: regenerate OpenAPI types and run `make web-contract-check`.
- Rollout behind no flag: the table is additive and the endpoints are new;
  a rollback drops no data.
