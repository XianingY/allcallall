-- latestConversationFollowup（会话详情页每次执行）查询
-- WHERE conversation_id = ? AND type = 'call_event' ORDER BY created_at DESC LIMIT 1。
-- 现有 conversation_id 单列索引需要扫描整个会话再过滤排序；
-- call_event 只占会话消息的极少数，复合索引可直达最新一条。
CREATE INDEX idx_messages_conversation_type_created
    ON messages (conversation_id, type, created_at);
