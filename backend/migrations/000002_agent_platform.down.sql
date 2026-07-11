DROP TABLE IF EXISTS langgraph_checkpoint_writes;
DROP TABLE IF EXISTS langgraph_checkpoint_threads;
DROP TABLE IF EXISTS langgraph_checkpoints;
DROP TABLE IF EXISTS agent_skill_tools;
DROP TABLE IF EXISTS agent_skills;
DROP TABLE IF EXISTS mcp_executions;
DROP TABLE IF EXISTS mcp_tools;
DROP TABLE IF EXISTS mcp_installation_revisions;
DROP TABLE IF EXISTS mcp_installations;

DROP INDEX idx_agent_tool_call_run ON agent_tool_calls;
ALTER TABLE agent_tool_calls MODIFY COLUMN call_id VARCHAR(64) NULL;

DROP INDEX idx_workflow_runs_checkpoint_id ON workflow_runs;
DROP INDEX idx_workflow_run_dedupe ON workflow_runs;
ALTER TABLE workflow_runs
    DROP COLUMN checkpoint_version,
    DROP COLUMN checkpoint_id,
    DROP COLUMN dedupe_key;

DROP INDEX idx_agent_runs_checkpoint_id ON agent_runs;
DROP INDEX idx_agent_run_dedupe ON agent_runs;
ALTER TABLE agent_runs
    DROP COLUMN checkpoint_version,
    DROP COLUMN checkpoint_id,
    DROP COLUMN dedupe_key;
