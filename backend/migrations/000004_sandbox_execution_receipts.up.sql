CREATE TABLE sandbox_execution_receipts (
    execution_id VARCHAR(96) CHARACTER SET ascii NOT NULL PRIMARY KEY,
    request_digest CHAR(64) CHARACTER SET ascii NOT NULL,
    organization_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    conversation_id BIGINT UNSIGNED NOT NULL,
    run_id BIGINT UNSIGNED NOT NULL,
    run_ref VARCHAR(96) CHARACTER SET ascii NOT NULL,
    tool_call_id VARCHAR(96) CHARACTER SET ascii NOT NULL,
    installation_id BIGINT UNSIGNED NOT NULL,
    revision_id BIGINT UNSIGNED NOT NULL,
    tool_id BIGINT UNSIGNED NOT NULL,
    tool_name VARCHAR(160) NOT NULL,
    source_type VARCHAR(32) NOT NULL,
    status VARCHAR(32) CHARACTER SET ascii NOT NULL,
    job_id VARCHAR(160) NOT NULL DEFAULT '',
    output_json LONGBLOB NULL,
    error_code VARCHAR(64) CHARACTER SET ascii NOT NULL DEFAULT '',
    error_message TEXT NOT NULL,
    timeout_ms BIGINT NOT NULL,
    started_at DATETIME(6) NOT NULL,
    stale_at DATETIME(6) NOT NULL,
    completed_at DATETIME(6) NULL,
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    INDEX idx_sandbox_receipt_request_digest (request_digest),
    INDEX idx_sandbox_receipt_organization (organization_id),
    INDEX idx_sandbox_receipt_user (user_id),
    INDEX idx_sandbox_receipt_run (run_id),
    INDEX idx_sandbox_receipt_installation (installation_id),
    INDEX idx_sandbox_receipt_revision (revision_id),
    INDEX idx_sandbox_receipt_tool (tool_id),
    INDEX idx_sandbox_receipt_status_stale (status, stale_at),
    INDEX idx_sandbox_receipt_completed (completed_at),
    INDEX idx_sandbox_receipt_expires (expires_at)
);

ALTER TABLE mcp_executions
    ADD COLUMN sandbox_request_digest CHAR(64) CHARACTER SET ascii NOT NULL DEFAULT '',
    ADD COLUMN reconcile_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN next_reconcile_at DATETIME(6) NULL;

UPDATE mcp_executions
SET next_reconcile_at = COALESCE(updated_at, CURRENT_TIMESTAMP(6))
WHERE status IN ('queued', 'starting', 'running');

CREATE INDEX idx_mcp_executions_reconcile ON mcp_executions (status, next_reconcile_at, id);
