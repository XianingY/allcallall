DROP INDEX idx_mcp_executions_reconcile ON mcp_executions;
ALTER TABLE mcp_executions
    DROP COLUMN next_reconcile_at,
    DROP COLUMN reconcile_attempts,
    DROP COLUMN sandbox_request_digest;
DROP TABLE IF EXISTS sandbox_execution_receipts;
