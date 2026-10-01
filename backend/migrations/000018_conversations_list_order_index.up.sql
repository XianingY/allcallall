-- ListConversations filters by organization (plus the member join) and sorts
-- by last_message_at DESC, updated_at DESC. The former organization-only
-- index forced MySQL to sort the whole organization before LIMIT could stop
-- the scan. This composite index is read in reverse for that ordering.
-- Create the replacement before dropping the redundant single-column index.
CREATE INDEX idx_conversations_org_last_message_updated
    ON conversations (organization_id, last_message_at, updated_at, id);
DROP INDEX idx_conversations_organization_id ON conversations;
