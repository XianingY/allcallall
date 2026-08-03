-- 录制文件上传状态机：支持 persist 阶段 S3 上传失败后交由后台 Worker 重试，
-- 保证录制文件最终一定落到对象存储，同时用单一 object_key 消除原先的双写冗余副本。
ALTER TABLE recording_files
    ADD COLUMN local_src_path VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN upload_status VARCHAR(32) NOT NULL DEFAULT 'done',
    ADD COLUMN upload_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN upload_last_error TEXT,
    ADD COLUMN next_retry_at DATETIME(6) NULL;

CREATE INDEX idx_recording_files_upload_status ON recording_files (upload_status);
