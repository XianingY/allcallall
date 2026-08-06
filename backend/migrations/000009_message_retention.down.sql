DROP INDEX idx_attachments_purged_at ON attachments;
DROP INDEX idx_attachments_retention_until ON attachments;
ALTER TABLE attachments
    DROP COLUMN purged_at,
    DROP COLUMN retention_until;

DROP INDEX idx_messages_purged_at ON messages;
DROP INDEX idx_messages_retention_until ON messages;
ALTER TABLE messages
    DROP COLUMN purged_at,
    DROP COLUMN retention_until;
