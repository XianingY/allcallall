# AllCallAll 合规与生产配置（Phase 1）

本目录提供上线前必须开启的合规开关模板与说明。

## 1. 隐私合规 7 件套（默认全关 → 上线前逐项开启）

| 控制 | 开关 | 说明 |
|------|------|------|
| 服务端留存 TTL | `MESSAGE_RETENTION_ENABLED` | 文字 72h / 媒体 120h 物理清空 |
| 信封加密 | `MESSAGE_ENCRYPTION_ENABLED` | AES-256-GCM，主密钥经 KMS 或环境变量注入 |
| 撤回 | `MESSAGE_RECALL_ENABLED` | 销毁正文+元数据+信封+附件+搜索副本 |
| 搜索最小化 | `SEARCH_INDEX_ENABLED` | 只存 rune 截断摘要 |
| 内容审核 | `CONTENT_MODERATION_ENABLED` | 异步非阻塞 hook |
| 被遗忘权 | `MESSAGE_ERASURE_ENABLED` | 用户/组织级擦除端点 |
| 实名核验+P2 | `REALNAME_VERIFICATION_ENABLED` | 入组实名拦截 |

统一装配入口：`internal/runtime.ApplyPrivacyPolicies`。

## 2. KMS 主密钥

`internal/kms` 提供 `MasterKeyProvider` 抽象：
- `StaticProvider`：从 `MESSAGE_ENCRYPTION_MASTER_KEY` 读取（默认）。
- `RotatingProvider` + `CloudKMSAdapter`：对接 AWS/GCP/阿里云 KMS，TTL 缓存自动轮转。
上线建议：禁用静态密钥，改用 `KMS_PROVIDER=cloud` 注入云 KMS 解密函数。

## 3. AI 生成内容标识

`internal/compliance.Watermark` 为 AI 文本追加可见标签（如「由人工智能生成」）
与防篡改机器标记（HMAC-SHA256 签名）。`AIGC_LABELING_ENABLED=true` 启用。
`Detect` / `Verify` 用于下游识别与完整性校验。

## 4. ICP 与生成式 AI 备案

- `internal/compliance.ValidateICPFormat` / `ICPRegistry`：ICP 号格式与授权集合校验。
- `internal/compliance.GenerativeAIServiceFiling`：算法备案元数据结构，序列化为
  JSON 供监管提交与年检查阅。
- `AssessPosture()`：实时评估 11 项合规控制就绪度，输出 readiness 百分比与 GA 判定。

## 5. 等保 2.0

建议定级 **三级**（承载个人信息且对外提供 AI 服务）。测评对接见
`docs/` 安全与合规文档；本仓库已具备：RBAC 租户隔离、全参数化 SQL、0 硬编码密钥、
信封加密、审计留存（180 天 worker）、统一租户中间件（Phase 0）。
