-- chat_events 留存清理按 created_at 过滤（PurgeBefore / StartRealtimeEventRetentionWorker）。
-- 该列目前只存在于复合索引 (organization_id, user_id, created_at) 的尾部，
-- WHERE created_at < ? 无法利用它，清理批次将全表扫描。补独立索引。
CREATE INDEX idx_chat_events_created_at ON chat_events (created_at);
