-- 消息撤回（对齐微信「撤回」语义）。
-- 与软删除分列存储：撤回受时限约束、由发送者主动发起、对所有参会人生效并留下墓碑提示；
-- 软删除是记录作废，两者的权限模型与产品语义不同，合并成一列会让审计无法区分。
ALTER TABLE messages ADD COLUMN recalled_at TIMESTAMP NULL DEFAULT NULL;
ALTER TABLE messages ADD COLUMN recalled_by BIGINT UNSIGNED NULL DEFAULT NULL;

-- recalled_at 用于列表/详情读路径过滤正文，需要索引；
-- recalled_by 用于「谁撤回了哪些消息」的合规审计查询。
CREATE INDEX idx_messages_recalled_at ON messages (recalled_at);
CREATE INDEX idx_messages_recalled_by ON messages (recalled_by);
