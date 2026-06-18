# Recording Storage And Transcription

Meeting recordings use a storage abstraction with two drivers:

- `local`: development and transcription-friendly local files.
- `s3`: S3-compatible object storage for production-like recording assets.

## Storage Variables

| Variable | Purpose |
| --- | --- |
| `RECORDING_STORAGE_DRIVER` | `local` or `s3`. |
| `RECORDING_STORAGE_DIR` | Local storage root. |
| `RECORDING_S3_BUCKET` | S3-compatible bucket. |
| `RECORDING_S3_REGION` | S3 region. |
| `RECORDING_S3_ENDPOINT` | S3-compatible endpoint. |
| `RECORDING_S3_ACCESS_KEY_ID` | Access key. |
| `RECORDING_S3_SECRET_ACCESS_KEY` | Secret key. |
| `RECORDING_S3_FORCE_PATH_STYLE` | `1` for MinIO/path-style services. |
| `RECORDING_PUBLIC_BASE_URL` | Optional public base URL. |
| `RECORDING_CLEANUP_INTERVAL_MIN` | Cleanup interval, default `60`. |

## Runtime Behavior

- The media layer produces recording artifacts.
- The collaboration service persists file metadata through `RecordingStorage`.
- `recording_sessions` stores lifecycle state.
- `recording_files` stores storage driver, bucket, object key, ETag, content type, file size, duration, metadata, retention, and soft delete state.
- Recording bytes are never stored in MySQL.

## Download Behavior

- `local`: backend serves the file directly after resolving it under `RECORDING_STORAGE_DIR`.
- `s3`: backend returns a short-lived signed URL redirect.
- All downloads pass organization/member checks before local serving or signed URL generation.
- Local paths and S3 object keys are normalized and rejected if they escape the configured root/key space.

## Recording-End Transcription

Recording transcription is independent of realtime translation.

```bash
TRANSCRIPTION_ENABLED=true
TRANSCRIPTION_PROVIDER=mock
```

Flow:

1. `POST /api/v1/rooms/:roomId/recording/stop` persists recording artifacts.
2. The service creates or updates `recording_transcriptions` with status `pending`.
3. It enqueues `recording.transcription.requested` in `event_outbox`.
4. The outbox worker calls the configured provider for each audio `recording_file`.
5. Segments are written to `meeting_transcript_segments`.
6. The job becomes `ready`, `failed`, or `skipped`.

Status meanings:

- `pending`: queued after recording stop.
- `processing`: worker claimed the job.
- `ready`: transcript segments were written.
- `failed`: provider/storage error; recording remains saved.
- `skipped`: no conversation binding, no audio file, or no transcript segments.

v1 only supports locally readable recording files. If `OpenLocal=false`, such as S3-only files, transcription is marked failed until Reader/download support is added.

## Agent Integration

The Agent conversation context loader reads `meeting_transcript_segments` for the current conversation, prioritizes the latest recording session, and exposes them as `meeting_transcript` context chunks. This lets the Agent answer questions like "what did we just discuss in the meeting?" after recording transcription completes.

## Retention

Retention is derived from organization policy:

- `recording_storage_days`
- `recording_export_allowed`
- `recording_mode`

Cleanup soft-deletes metadata only after object deletion succeeds. If object deletion fails, `deleted_at` remains empty so the cleanup worker can retry.

Metrics:

- `recording_retention_deleted_total`
- `recording_retention_delete_fail_total`
