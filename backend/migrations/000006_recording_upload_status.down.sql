ALTER TABLE recording_files
    DROP COLUMN local_src_path,
    DROP COLUMN upload_status,
    DROP COLUMN upload_attempts,
    DROP COLUMN upload_last_error,
    DROP COLUMN next_retry_at;
