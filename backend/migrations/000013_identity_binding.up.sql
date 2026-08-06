-- 实名绑定（金融/监管合规「谁在操作」可追溯要求）。
-- 用户表增加实名与身份核验标记；组织策略增加「加入需身份核验」开关。
-- 与既有录制/留存策略分列，因其约束的是成员准入而非内容处理。
ALTER TABLE users ADD COLUMN real_name VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN identity_verified TINYINT(1) NOT NULL DEFAULT 0;

ALTER TABLE organization_policies ADD COLUMN require_identity_verification TINYINT(1) NOT NULL DEFAULT 0;
