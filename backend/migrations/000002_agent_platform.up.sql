ALTER TABLE agent_runs
    ADD COLUMN dedupe_key VARCHAR(128) NULL,
    ADD COLUMN checkpoint_id VARCHAR(160) NOT NULL DEFAULT '',
    ADD COLUMN checkpoint_version BIGINT UNSIGNED NOT NULL DEFAULT 0;

UPDATE agent_runs AS target
JOIN (
    SELECT organization_id, user_id, conversation_id, idempotency_key, MIN(id) AS keep_id
    FROM agent_runs
    WHERE idempotency_key <> ''
    GROUP BY organization_id, user_id, conversation_id, idempotency_key
) AS source ON source.keep_id = target.id
SET target.dedupe_key = target.idempotency_key;

CREATE UNIQUE INDEX idx_agent_run_dedupe
    ON agent_runs (organization_id, user_id, conversation_id, dedupe_key);
CREATE INDEX idx_agent_runs_checkpoint_id ON agent_runs (checkpoint_id);

ALTER TABLE workflow_runs
    ADD COLUMN dedupe_key VARCHAR(128) NULL,
    ADD COLUMN checkpoint_id VARCHAR(160) NOT NULL DEFAULT '',
    ADD COLUMN checkpoint_version BIGINT UNSIGNED NOT NULL DEFAULT 0;

UPDATE workflow_runs AS target
JOIN (
    SELECT organization_id, user_id, conversation_id, idempotency_key, MIN(id) AS keep_id
    FROM workflow_runs
    WHERE idempotency_key <> ''
    GROUP BY organization_id, user_id, conversation_id, idempotency_key
) AS source ON source.keep_id = target.id
SET target.dedupe_key = target.idempotency_key;

CREATE UNIQUE INDEX idx_workflow_run_dedupe
    ON workflow_runs (organization_id, user_id, conversation_id, dedupe_key);
CREATE INDEX idx_workflow_runs_checkpoint_id ON workflow_runs (checkpoint_id);

UPDATE agent_tool_calls
SET call_id = CONCAT('legacy:', id)
WHERE call_id IS NULL OR call_id = '';
ALTER TABLE agent_tool_calls MODIFY COLUMN call_id VARCHAR(96) NOT NULL;
CREATE UNIQUE INDEX idx_agent_tool_call_run ON agent_tool_calls (run_id, call_id);

CREATE TABLE mcp_installations (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    organization_id BIGINT UNSIGNED NOT NULL,
    owner_user_id BIGINT UNSIGNED NOT NULL,
    scope VARCHAR(32) NOT NULL,
    display_name VARCHAR(160) NOT NULL,
    source_type VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    active_revision_id BIGINT UNSIGNED NULL,
    vault_path VARCHAR(500) NOT NULL DEFAULT '',
    last_error TEXT NOT NULL,
    published_by BIGINT UNSIGNED NULL,
    published_at DATETIME(6) NULL,
    deleted_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    INDEX idx_mcp_installations_org (organization_id),
    INDEX idx_mcp_installations_owner (owner_user_id),
    INDEX idx_mcp_installations_scope (scope),
    INDEX idx_mcp_installations_status (status),
    INDEX idx_mcp_installations_deleted (deleted_at)
);

CREATE TABLE mcp_installation_revisions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    installation_id BIGINT UNSIGNED NOT NULL,
    revision INT NOT NULL,
    transport VARCHAR(32) NOT NULL,
    image_ref VARCHAR(500) NOT NULL DEFAULT '',
    image_digest VARCHAR(160) NOT NULL DEFAULT '',
    endpoint_url VARCHAR(1000) NOT NULL DEFAULT '',
    command_json LONGTEXT NOT NULL,
    args_json LONGTEXT NOT NULL,
    config_json LONGTEXT NOT NULL,
    network_allowlist_json LONGTEXT NOT NULL,
    scan_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    scan_report_json LONGTEXT NOT NULL,
    created_by BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY idx_mcp_installation_revision (installation_id, revision),
    INDEX idx_mcp_revision_digest (image_digest),
    INDEX idx_mcp_revision_scan_status (scan_status)
);

CREATE TABLE mcp_tools (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    installation_id BIGINT UNSIGNED NOT NULL,
    revision_id BIGINT UNSIGNED NOT NULL,
    namespaced_name VARCHAR(255) CHARACTER SET ascii NOT NULL,
    original_name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL,
    input_schema_json LONGTEXT NOT NULL,
    output_schema_json LONGTEXT NOT NULL,
    risk VARCHAR(32) NOT NULL DEFAULT 'unknown',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    schema_version VARCHAR(64) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY idx_mcp_tool_revision_name (revision_id, namespaced_name),
    INDEX idx_mcp_tools_installation (installation_id),
    INDEX idx_mcp_tools_revision (revision_id),
    INDEX idx_mcp_tools_risk (risk)
);

CREATE TABLE mcp_executions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    execution_id VARCHAR(96) CHARACTER SET ascii NOT NULL,
    run_ref VARCHAR(96) CHARACTER SET ascii NOT NULL,
    organization_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    agent_run_id BIGINT UNSIGNED NULL,
    workflow_run_id BIGINT UNSIGNED NULL,
    installation_id BIGINT UNSIGNED NOT NULL,
    revision_id BIGINT UNSIGNED NOT NULL,
    tool_id BIGINT UNSIGNED NOT NULL,
    tool_call_id VARCHAR(96) CHARACTER SET ascii NOT NULL,
    status VARCHAR(32) NOT NULL,
    input_json LONGTEXT NOT NULL,
    output_json LONGTEXT NOT NULL,
    sandbox_job_id VARCHAR(160) NOT NULL DEFAULT '',
    attempts INT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL,
    started_at DATETIME(6) NULL,
    completed_at DATETIME(6) NULL,
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY idx_mcp_executions_execution_id (execution_id),
    UNIQUE KEY idx_mcp_execution_call (run_ref, tool_call_id),
    INDEX idx_mcp_executions_org (organization_id),
    INDEX idx_mcp_executions_user (user_id),
    INDEX idx_mcp_executions_status (status),
    INDEX idx_mcp_executions_expires (expires_at),
    INDEX idx_mcp_executions_tool_call (tool_call_id)
);

CREATE TABLE agent_skills (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    organization_id BIGINT UNSIGNED NOT NULL,
    owner_user_id BIGINT UNSIGNED NOT NULL,
    scope VARCHAR(32) NOT NULL,
    name VARCHAR(160) NOT NULL,
    description TEXT NOT NULL,
    instructions LONGTEXT NOT NULL,
    status VARCHAR(32) NOT NULL,
    version INT NOT NULL DEFAULT 1,
    published_by BIGINT UNSIGNED NULL,
    published_at DATETIME(6) NULL,
    deleted_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    INDEX idx_agent_skills_org (organization_id),
    INDEX idx_agent_skills_owner (owner_user_id),
    INDEX idx_agent_skills_scope (scope),
    INDEX idx_agent_skills_status (status)
);

CREATE TABLE agent_skill_tools (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    skill_id BIGINT UNSIGNED NOT NULL,
    tool_id BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE KEY idx_agent_skill_tool (skill_id, tool_id),
    INDEX idx_agent_skill_tools_tool (tool_id)
);

CREATE TABLE langgraph_checkpoints (
    thread_id VARCHAR(160) CHARACTER SET ascii NOT NULL,
    checkpoint_ns VARCHAR(160) CHARACTER SET ascii NOT NULL,
    checkpoint_id VARCHAR(160) CHARACTER SET ascii NOT NULL,
    parent_checkpoint_id VARCHAR(160) CHARACTER SET ascii NOT NULL DEFAULT '',
    execution_id VARCHAR(96) CHARACTER SET ascii NOT NULL DEFAULT '',
    workflow_run_id BIGINT UNSIGNED NULL,
    agent_run_id BIGINT UNSIGNED NULL,
    version BIGINT UNSIGNED NOT NULL,
    checkpoint_type VARCHAR(64) CHARACTER SET ascii NOT NULL,
    checkpoint_blob LONGBLOB NOT NULL,
    metadata_type VARCHAR(64) CHARACTER SET ascii NOT NULL,
    metadata_blob LONGBLOB NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (thread_id, checkpoint_ns, checkpoint_id),
    INDEX idx_langgraph_checkpoint_parent (parent_checkpoint_id),
    INDEX idx_langgraph_checkpoint_execution (execution_id),
    INDEX idx_langgraph_checkpoint_version (thread_id, checkpoint_ns, version),
    INDEX idx_langgraph_checkpoint_created (created_at)
);

CREATE TABLE langgraph_checkpoint_threads (
    thread_id VARCHAR(160) CHARACTER SET ascii NOT NULL,
    checkpoint_ns VARCHAR(160) CHARACTER SET ascii NOT NULL,
    current_version BIGINT UNSIGNED NOT NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (thread_id, checkpoint_ns)
);

CREATE TABLE langgraph_checkpoint_writes (
    thread_id VARCHAR(160) CHARACTER SET ascii NOT NULL,
    checkpoint_ns VARCHAR(160) CHARACTER SET ascii NOT NULL,
    checkpoint_id VARCHAR(160) CHARACTER SET ascii NOT NULL,
    task_id VARCHAR(160) CHARACTER SET ascii NOT NULL,
    task_path VARCHAR(500) CHARACTER SET ascii NOT NULL,
    write_index INT NOT NULL,
    channel VARCHAR(160) CHARACTER SET ascii NOT NULL,
    value_type VARCHAR(64) CHARACTER SET ascii NOT NULL,
    value_blob LONGBLOB NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (thread_id, checkpoint_ns, checkpoint_id, task_id, task_path, write_index),
    INDEX idx_langgraph_writes_created (created_at)
);
