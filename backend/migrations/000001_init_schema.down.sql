-- 000001_init_schema.down.sql
--
-- Drops the initial schema. It used to be an empty file, which meant
-- migrating down to version 0 removed the schema_migrations row and left
-- all 73 tables behind. Re-running up from there hits "table already"
-- exists" on the first statement and marks the database dirty, which then
-- needs a manual `migrate force` to clear.
--
-- Foreign-key checks are disabled so the drops do not have to be ordered
-- by dependency; IF EXISTS keeps it safe if a later migration renamed or
-- dropped one of these tables.

SET FOREIGN_KEY_CHECKS = 0;

DROP TABLE IF EXISTS `event_outbox`;
DROP TABLE IF EXISTS `tool_approvals`;
DROP TABLE IF EXISTS `tool_policies`;
DROP TABLE IF EXISTS `agent_messages`;
DROP TABLE IF EXISTS `workflow_timers`;
DROP TABLE IF EXISTS `workflow_signals`;
DROP TABLE IF EXISTS `workflow_history_events`;
DROP TABLE IF EXISTS `workflow_tasks`;
DROP TABLE IF EXISTS `workflow_runs`;
DROP TABLE IF EXISTS `rag_chunks`;
DROP TABLE IF EXISTS `rag_source_versions`;
DROP TABLE IF EXISTS `rag_sources`;
DROP TABLE IF EXISTS `rag_source_duplicates`;
DROP TABLE IF EXISTS `rag_source_groups`;
DROP TABLE IF EXISTS `tool_schema_versions`;
DROP TABLE IF EXISTS `agent_prompt_versions`;
DROP TABLE IF EXISTS `agent_context_chunks`;
DROP TABLE IF EXISTS `agent_memories`;
DROP TABLE IF EXISTS `agent_tool_calls`;
DROP TABLE IF EXISTS `agent_steps`;
DROP TABLE IF EXISTS `agent_runs`;
DROP TABLE IF EXISTS `deal_activities`;
DROP TABLE IF EXISTS `deal_contacts`;
DROP TABLE IF EXISTS `deals`;
DROP TABLE IF EXISTS `pipeline_stages`;
DROP TABLE IF EXISTS `pipelines`;
DROP TABLE IF EXISTS `room_settlements`;
DROP TABLE IF EXISTS `recording_exports`;
DROP TABLE IF EXISTS `recording_consents`;
DROP TABLE IF EXISTS `meeting_transcript_segments`;
DROP TABLE IF EXISTS `recording_transcriptions`;
DROP TABLE IF EXISTS `recording_files`;
DROP TABLE IF EXISTS `recording_sessions`;
DROP TABLE IF EXISTS `call_room_events`;
DROP TABLE IF EXISTS `call_room_members`;
DROP TABLE IF EXISTS `call_rooms`;
DROP TABLE IF EXISTS `push_devices`;
DROP TABLE IF EXISTS `organization_audit_events`;
DROP TABLE IF EXISTS `conversation_pins`;
DROP TABLE IF EXISTS `message_reactions`;
DROP TABLE IF EXISTS `attachments`;
DROP TABLE IF EXISTS `chat_events`;
DROP TABLE IF EXISTS `message_reads`;
DROP TABLE IF EXISTS `messages`;
DROP TABLE IF EXISTS `conversation_members`;
DROP TABLE IF EXISTS `conversation_notes`;
DROP TABLE IF EXISTS `conversations`;
DROP TABLE IF EXISTS `organization_policies`;
DROP TABLE IF EXISTS `organization_invites`;
DROP TABLE IF EXISTS `team_members`;
DROP TABLE IF EXISTS `teams`;
DROP TABLE IF EXISTS `organization_members`;
DROP TABLE IF EXISTS `organizations`;
DROP TABLE IF EXISTS `follow_up_tasks`;
DROP TABLE IF EXISTS `call_followups`;
DROP TABLE IF EXISTS `call_transcript_segments`;
DROP TABLE IF EXISTS `contact_profiles`;
DROP TABLE IF EXISTS `invitations`;
DROP TABLE IF EXISTS `deletion_audits`;
DROP TABLE IF EXISTS `billing_webhook_events`;
DROP TABLE IF EXISTS `translation_usage_slices`;
DROP TABLE IF EXISTS `usage_ledgers`;
DROP TABLE IF EXISTS `user_entitlements`;
DROP TABLE IF EXISTS `legal_acceptances`;
DROP TABLE IF EXISTS `abuse_reports`;
DROP TABLE IF EXISTS `user_blocks`;
DROP TABLE IF EXISTS `call_sessions`;
DROP TABLE IF EXISTS `email_send_logs`;
DROP TABLE IF EXISTS `email_verification_codes`;
DROP TABLE IF EXISTS `contacts`;
DROP TABLE IF EXISTS `refresh_sessions`;
DROP TABLE IF EXISTS `sqlite_sequence`;
DROP TABLE IF EXISTS `users`;

SET FOREIGN_KEY_CHECKS = 1;
