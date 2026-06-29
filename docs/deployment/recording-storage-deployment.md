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
TRANSCRIPTION_PROVIDER=openai_compatible
TRANSCRIPTION_OPENAI_BASE_URL=https://api.example.com/v1
TRANSCRIPTION_OPENAI_MODEL=whisper-1
TRANSCRIPTION_OPENAI_API_KEY=...
TRANSCRIPTION_CHUNK_SECONDS=600
TRANSCRIPTION_MAX_UPLOAD_BYTES=25165824
TRANSCRIPTION_FFMPEG_PATH=ffmpeg
```

`TRANSCRIPTION_PROVIDER=mock` remains available for deterministic local development and tests, but it should not be used as Beta proof of real ASR behavior.

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

Local and S3-compatible recordings can both enter the transcription path. Local files are opened directly; S3 objects are downloaded into a bounded temporary file and removed after processing. Long OGG recordings are split with FFmpeg before upload to the ASR endpoint.

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
