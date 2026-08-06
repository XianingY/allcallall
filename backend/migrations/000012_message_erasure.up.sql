-- 被遗忘权 / 合规擦除（PIPL 第四十七条删除权、组织注销一键清除）。
-- 与撤回分列存储：擦除由 owner/admin 或用户本人针对个人信息行使，范围更宽、强制力更强，
-- 且对所有参会者生效，不区分发送者本人；两者的权限模型与审计语义不同，合并成一列会让审计无法区分。
ALTER TABLE messages ADD COLUMN erased_at TIMESTAMP NULL DEFAULT NULL;
ALTER TABLE messages ADD COLUMN erased_by BIGINT UNSIGNED NULL DEFAULT NULL;

-- erased_at 用于擦除范围查询与读路径过滤正文，需要索引；
-- erased_by 用于「谁擦除了哪些消息」的合规审计查询。
CREATE INDEX idx_messages_erased_at ON messages (erased_at);
CREATE INDEX idx_messages_erased_by ON messages (erased_by);
