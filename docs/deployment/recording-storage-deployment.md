# Recording Storage Deployment

Meeting recordings now use a storage abstraction with two drivers:

- `local`: development fallback
- `s3`: production S3-compatible object storage

## Environment variables

- `RECORDING_STORAGE_DRIVER=local|s3`
- `RECORDING_STORAGE_DIR`
- `RECORDING_S3_BUCKET`
- `RECORDING_S3_REGION`
- `RECORDING_S3_ENDPOINT`
- `RECORDING_S3_ACCESS_KEY_ID`
- `RECORDING_S3_SECRET_ACCESS_KEY`
- `RECORDING_S3_FORCE_PATH_STYLE=1`
- `RECORDING_PUBLIC_BASE_URL` (optional)
- `RECORDING_CLEANUP_INTERVAL_MIN` (optional, default `60`)

## Runtime behavior

- Recording artifacts are first produced by the media layer.
- The collaboration service persists them through `RecordingStorage`.
- `recording_files` stores:
  - `storage_driver`
  - `storage_bucket`
  - `object_key`
  - `etag`
  - `retention_until`
  - `deleted_at`

## Download behavior

- `local` driver: the backend serves the file directly after resolving it under `RECORDING_STORAGE_DIR`.
- `s3` driver: the backend returns a short-lived signed URL redirect.
- Recording downloads always go through backend organization/member checks before local serving or signed URL generation.
- Local object keys and local download paths are rejected if they escape the configured storage root.

## Retention

Retention is derived from organization policy:

- `recording_storage_days`
- `recording_export_allowed`
- `recording_mode`

`retention_until` is written at upload time. Cleanup jobs soft-delete expired metadata only after the backing object delete succeeds. If object deletion fails, `deleted_at` remains empty so the worker can retry later. Dedicated audit rows are not implemented yet; use structured logs and retention metrics for the current runbook.

## Cleanup worker

The backend now starts a built-in recording cleanup worker on boot.

- It runs immediately once at startup.
- It then repeats every `RECORDING_CLEANUP_INTERVAL_MIN`.
- It marks expired `recording_files` with `deleted_at`.
- It removes the backing object through the configured storage driver.
- Storage delete failure stops the current cleanup run and leaves the file metadata active for retry.

Relevant metrics:

- `recording_retention_deleted_total`
- `recording_retention_delete_fail_total`
