DROP INDEX idx_messages_erased_by ON messages;
DROP INDEX idx_messages_erased_at ON messages;

ALTER TABLE messages DROP COLUMN erased_by;
ALTER TABLE messages DROP COLUMN erased_at;
