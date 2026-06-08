# Support Runbook: Meetings and Recordings

Use the internal support API with `X-Support-Token`.

## Endpoints

- `GET /api/v1/internal/support/users/:userId/summary`
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
- recent redacted session metadata

The support API intentionally omits refresh token values and token hashes. A non-zero `invalid_use_count` means a rotated, revoked, or expired refresh token was reused and should be treated as a risk signal.

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
- `recording_start_total`
- `recording_stop_total`
- `recording_storage_write_fail_total`
- `recording_download_total`
- `recording_download_unauthorized_total`
- `room_event_broadcast_fail_total`
