DROP INDEX idx_messages_recalled_by ON messages;
DROP INDEX idx_messages_recalled_at ON messages;

ALTER TABLE messages DROP COLUMN recalled_by;
ALTER TABLE messages DROP COLUMN recalled_at;
