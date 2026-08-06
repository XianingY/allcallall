-- 消息正文应用层信封加密元数据（每条消息独立 DEK，DEK 由主密钥包裹后随行存储）。
-- 为 NULL / 空串时表示该行仍是历史明文，读取路径原样返回，保证灰度上线与回滚安全。
-- Envelope metadata for application-layer message body encryption; NULL means legacy plaintext.
-- 不建索引：该列只随行读写，从不作为查询条件；建索引反而会把密钥材料写进索引页。
ALTER TABLE messages
    ADD COLUMN encryption_metadata VARCHAR(512) NULL DEFAULT NULL;
