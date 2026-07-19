ALTER TABLE workflow_runs
    ADD COLUMN approval_request_id VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT '',
    ADD COLUMN runtime_request_json LONGTEXT NULL,
    ADD COLUMN execution_lease_token VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT '',
    ADD COLUMN runtime_owner VARCHAR(32) CHARACTER SET ascii NOT NULL DEFAULT 'legacy_go',
    ADD INDEX idx_workflow_runs_approval_request_id (approval_request_id),
    ADD INDEX idx_workflow_runs_execution_lease_token (execution_lease_token),
    ADD INDEX idx_workflow_runs_runtime_owner (runtime_owner);

UPDATE workflow_runs
SET runtime_owner = 'python_langgraph'
WHERE workflow_version LIKE '%langgraph%';

ALTER TABLE agent_runs
    ADD COLUMN approval_request_id VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT '',
    ADD COLUMN runtime_request_json LONGTEXT NULL,
    ADD COLUMN execution_lease_token VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT '',
    ADD COLUMN runtime_owner VARCHAR(32) CHARACTER SET ascii NOT NULL DEFAULT 'legacy_go',
    ADD INDEX idx_agent_runs_approval_request_id (approval_request_id),
    ADD INDEX idx_agent_runs_execution_lease_token (execution_lease_token),
    ADD INDEX idx_agent_runs_runtime_owner (runtime_owner);

UPDATE agent_runs
SET runtime_owner = 'python_langgraph'
WHERE source = 'python_langgraph';

UPDATE workflow_runs SET runtime_request_json = '' WHERE runtime_request_json IS NULL;
UPDATE agent_runs SET runtime_request_json = '' WHERE runtime_request_json IS NULL;

ALTER TABLE workflow_runs MODIFY COLUMN runtime_request_json LONGTEXT NOT NULL;
ALTER TABLE agent_runs MODIFY COLUMN runtime_request_json LONGTEXT NOT NULL;

ALTER TABLE agent_tool_calls
    ADD COLUMN approval_request_id VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT '',
    ADD COLUMN approval_checkpoint_version BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN decision VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN decided_by BIGINT UNSIGNED NULL,
    ADD COLUMN decided_at DATETIME(6) NULL,
    ADD COLUMN mcp_installation_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN mcp_revision_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN mcp_tool_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD INDEX idx_agent_tool_calls_approval_request_id (approval_request_id),
    ADD INDEX idx_agent_tool_calls_approval_checkpoint_version (approval_checkpoint_version),
    ADD INDEX idx_agent_tool_calls_decided_by (decided_by),
    ADD INDEX idx_agent_tool_calls_decided_at (decided_at),
    ADD INDEX idx_agent_tool_calls_mcp_installation_id (mcp_installation_id),
    ADD INDEX idx_agent_tool_calls_mcp_revision_id (mcp_revision_id),
    ADD INDEX idx_agent_tool_calls_mcp_tool_id (mcp_tool_id);

ALTER TABLE tool_approvals
    ADD COLUMN approval_request_id VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT '',
    ADD COLUMN approval_checkpoint_version BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN mcp_installation_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN mcp_revision_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD COLUMN mcp_tool_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD INDEX idx_tool_approvals_approval_request_id (approval_request_id),
    ADD INDEX idx_tool_approvals_approval_checkpoint_version (approval_checkpoint_version),
    ADD INDEX idx_tool_approvals_mcp_installation_id (mcp_installation_id),
    ADD INDEX idx_tool_approvals_mcp_revision_id (mcp_revision_id),
    ADD INDEX idx_tool_approvals_mcp_tool_id (mcp_tool_id);
