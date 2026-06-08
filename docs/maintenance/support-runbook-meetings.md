# Support Runbook: Meetings and Recordings

Use the internal support API with `X-Support-Token`.

All JSON error responses include `error`, `code`, `request_id`, and `success=false`. Use `request_id` to correlate API responses with structured logs.

## Endpoints

- `GET /api/v1/internal/support/users/:userId/summary`
- `DELETE /api/v1/internal/support/users/:userId/sessions/:sessionId`
- `POST /api/v1/internal/support/users/:userId/sessions/revoke-all`
- `GET /api/v1/internal/support/rooms/:roomId`
- `GET /api/v1/internal/support/recordings/:id`

## What to inspect first

### Room issues

Check:

- room status
- latest room events
- member state snapshot
- active/latest recording reference

Typical failure patterns:

- members joined but media never synced
- room ended without a final recording event
- repeated reconnects without a stable `connection_state`

### Session issues

Check the user summary `refresh_sessions` block:

- `active_count`
- `revoked_count`
- `expired_count`
- `invalid_use_count`
- `last_invalid_use_at`
- `risk_level`
- `risk_reasons`
- recent redacted session metadata

The support API intentionally omits refresh token values and token hashes. A non-zero `invalid_use_count` means a rotated, revoked, or expired refresh token was reused and should be treated as a risk signal.

Risk levels:

- `none`: no obvious refresh-session risk signal.
- `low`: many active sessions, but no token reuse signal.
- `medium`: refresh token reuse was detected.
- `high`: repeated or recent refresh token reuse was detected.

Support actions:

- Use `DELETE /api/v1/internal/support/users/:userId/sessions/:sessionId` to revoke one active refresh session.
- Use `POST /api/v1/internal/support/users/:userId/sessions/revoke-all` when the account appears compromised or the user requests forced revocation.
- These actions revoke refresh capability only. Already issued short-lived access tokens can remain valid until they expire.
- Both endpoints are idempotent and return `revoked_sessions`; a value of `0` usually means the target session was already revoked or expired.

### Recording issues

Check:

- `storage_driver`
- `storage_bucket`
- `object_key`
- `etag`
- `retention_until`
- `deleted_at`
- recent `exports` records, including `requested_by`, `status`, `expires_at`, and `download_count`

Typical failure patterns:

- storage write failed before metadata commit
- user attempted cross-organization download
- signed URL generation failed
- export audit failed before redirect/download
- retention worker failed to delete expired objects

## Metrics to watch

- `meeting_join_total`
- `meeting_join_fail_total`
- `meeting_reconnect_total`
- `meeting_reconnect_fail_total`
- `room_media_state_update_total`
- `refresh_session_invalid_use_total`
- `recording_start_total`
- `recording_stop_total`
- `recording_storage_write_fail_total`
- `recording_download_total`
- `recording_download_unauthorized_total`
- `room_event_broadcast_fail_total`
