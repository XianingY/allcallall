-- 消息分页热查询的复合索引：ListMessagePage 按
-- organization_id + conversation_id 过滤、按 id 游标分页/排序。
-- chat_events 已有同款复合索引（idx_chat_event_recipient），messages 一直缺。
CREATE INDEX idx_messages_org_conversation_id ON messages (organization_id, conversation_id, id);
