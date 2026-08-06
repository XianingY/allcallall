-- 消息留存期限（对齐 PIPL 第十九条「最短必要期限」与微信「文字 72h / 媒体 120h」服务端留存模型）
-- Message retention windows: server-side bodies are purged once retention_until elapses.
ALTER TABLE messages
    ADD COLUMN retention_until TIMESTAMP NULL DEFAULT NULL,
    ADD COLUMN purged_at TIMESTAMP NULL DEFAULT NULL;

CREATE INDEX idx_messages_retention_until ON messages (retention_until);
CREATE INDEX idx_messages_purged_at ON messages (purged_at);

ALTER TABLE attachments
    ADD COLUMN retention_until TIMESTAMP NULL DEFAULT NULL,
    ADD COLUMN purged_at TIMESTAMP NULL DEFAULT NULL;

CREATE INDEX idx_attachments_retention_until ON attachments (retention_until);
CREATE INDEX idx_attachments_purged_at ON attachments (purged_at);

-- 历史数据回填：存量用户消息按创建时间 + 默认窗口给出留存终点。
-- 系统消息 / 通话事件属于会话运营记录，保持 NULL（不参与自动清理）。
-- Backfill existing user-generated rows; operational rows stay NULL (exempt).
UPDATE messages
SET retention_until = DATE_ADD(created_at, INTERVAL 72 HOUR)
WHERE retention_until IS NULL
  AND deleted_at IS NULL
  AND type = 'text';

UPDATE attachments
SET retention_until = DATE_ADD(created_at, INTERVAL 120 HOUR)
WHERE retention_until IS NULL;
