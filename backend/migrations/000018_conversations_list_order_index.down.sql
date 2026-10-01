-- Restore the former single-column index before removing its replacement so
-- organization-filtered queries never run without index support.
CREATE INDEX idx_conversations_organization_id
    ON conversations (organization_id);
DROP INDEX idx_conversations_org_last_message_updated ON conversations;
