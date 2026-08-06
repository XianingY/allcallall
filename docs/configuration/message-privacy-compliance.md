# 消息隐私与合规实现

本文档说明 AllCallAll 的消息存储合规设计与配置方式。整体思路对标微信的「**中转而非归档**」模型，并对齐《个人信息保护法》（PIPL）的最小必要、可撤回、可删除等义务。

与 `docs/configuration/security-guidelines.md` 的分工：那份文档覆盖通用基础设施安全（主机、网络、认证），本文只覆盖**消息内容本身的生命周期与合规控制**。

## 设计原则

1. **服务端是中转站，不是档案馆。** 消息正文在服务端只做投递中转，到期物理清空，不做长期归档。
2. **失败即关闭（fail-closed）。** 加密未正确配置时进程直接拒绝启动，绝不静默降级为明文写入。
3. **保留骨架，销毁内容。** 所有销毁动作（TTL 到期、撤回、擦除）都只清空正文/元数据/密文信封，保留消息骨架（ID、时间戳、发送者），使会话时间线保持连贯、已读位点不错乱。
4. **策略单点装配。** API 进程与所有 worker 进程统一调用 `runtime.ApplyPrivacyPolicies`，避免出现「API 加密、worker 明文」这类不一致。
5. **默认关闭，显式开启。** 除搜索索引最小化（隐私优先，默认开启）外，其余能力均需显式配置开启。

## 能力一览

### 1. 留存 TTL（P0）

| 项 | 值 |
| --- | --- |
| 实现 | `backend/internal/collaboration/message_retention.go` |
| 迁移 | `000009_message_retention` |
| 文字消息 | 默认 72 小时后物理清空正文 |
| 媒体消息 | 默认 120 小时后物理清空正文与附件对象 |
| 豁免 | 系统消息、通话事件消息默认不清理 |
| 清理方式 | 后台 worker 按 `MESSAGE_RETENTION_CLEANUP_INTERVAL_MIN` 周期分批执行，单批上限 `MESSAGE_RETENTION_CLEANUP_BATCH_LIMIT` |

```bash
MESSAGE_RETENTION_ENABLED=true
MESSAGE_RETENTION_TEXT_TTL_HOURS=72
MESSAGE_RETENTION_MEDIA_TTL_HOURS=120
MESSAGE_RETENTION_PURGE_SYSTEM=false
MESSAGE_RETENTION_CLEANUP_INTERVAL_MIN=30
MESSAGE_RETENTION_CLEANUP_BATCH_LIMIT=500
```

### 2. 应用层信封加密（P0）

| 项 | 值 |
| --- | --- |
| 实现 | `backend/internal/messagecrypto` |
| 迁移 | `000010_message_encryption` |
| 算法 | 每条消息随机生成 DEK（AES-256-GCM），DEK 由主密钥包裹后随密文存储 |

主密钥必须是 base64 编码。**开启加密但未提供主密钥时，配置加载直接返回错误、进程启动失败**——这是刻意的 fail-closed 设计，防止误配置导致明文落库。

```bash
MESSAGE_ENCRYPTION_ENABLED=true
MESSAGE_ENCRYPTION_MASTER_KEY=<base64-encoded-32-byte-key>
MESSAGE_ENCRYPTION_KEY_ID=k1
```

轮换主密钥时保留旧 `KeyID` 的解密路径，历史消息随 TTL 自然过期后即可下线旧密钥。

### 3. 撤回（P0）

| 项 | 值 |
| --- | --- |
| 实现 | `backend/internal/collaboration/message_recall.go` |
| 迁移 | `000011_message_recall` |
| 接口 | `POST /api/v1/conversations/:id/messages/:messageId/recall` |

撤回会销毁**正文 + metadata + 密文信封 + 附件对象 + 搜索副本**五处，仅保留骨架并写入 `recalled_at` / `recalled_by`。操作幂等：重复撤回不报错、不重复计数。

管理员可通过 `MESSAGE_RECALL_ALLOW_ADMIN_OVERRIDE` 突破时间窗强制撤回，用于合规下架场景，该动作会写审计事件。

```bash
MESSAGE_RECALL_ENABLED=true
MESSAGE_RECALL_WINDOW_MINUTES=2
MESSAGE_RECALL_ALLOW_ADMIN_OVERRIDE=true
```

### 4. 搜索索引最小化（P1）

对应 PIPL 第六条最小必要原则。搜索索引服务通常位于信任边界之外（如独立的 Elasticsearch 集群），不应持有完整正文。

`BuildMessageSearchDocument` 不再写入完整 Body，改为写入**按 rune 截断的摘要** + `body_length` 长度信号，兼顾可检索性与最小化。

```bash
SEARCH_INDEX_ENABLED=true              # 隐私优先，默认开启
SEARCH_INDEX_BODY_SNIPPET_MAX_RUNES=64
```

> 注意：策略的 `Normalized()` 只修正摘要长度下限，**绝不翻转 `Enabled`**，确保配置里的显式禁用不会被悄悄重新打开。

### 5. 被遗忘权 / 一键擦除（P1）

对应 PIPL 第四十七条。实现于 `backend/internal/collaboration/message_erasure.go`，迁移 `000012_message_erasure`。

| 接口 | 权限 |
| --- | --- |
| `POST /api/v1/organizations/:id/users/:userId/messages/erase` | 本人擦除自己；owner/admin 可擦除任意成员 |
| `POST /api/v1/organizations/:id/messages/erase` | 仅 owner/admin |

擦除同样销毁正文、附件、搜索副本，标记 `erased_at` / `erased_by`，分批执行且幂等（`erased_at IS NULL` 作为守卫条件）。权限不足返回 `403`。

### 6. 内容审核 hook（P1）

`ModerationService` 是可插拔接口，默认提供大小写不敏感的 `KeywordModerationService`。审核在 `PublishMessageCreatedFromOutbox` 之后**异步非阻塞**触发（5 秒超时），审核失败只告警、不阻断消息投递——可用性优先于审核完备性。

命中时写入 `message.moderated` 审计事件，并广播 `message.moderation_flagged` 标记。

```bash
CONTENT_MODERATION_ENABLED=true
CONTENT_MODERATION_KEYWORDS=keyword1,keyword2
```

接入外部审核服务时实现 `ModerationService` 接口并在 `runtime.ContentModerationFromConfig` 中替换即可。

### 7. 传输强制 TLS（P2）

`RequireTLS` 中间件挂在 `/api/v1` 路由组上。直接 TLS 连接或反向代理传入 `X-Forwarded-Proto: https` 时放行，否则返回 `403 HTTPS_REQUIRED`。

```bash
SECURITY_REQUIRE_TLS=true
```

生产环境务必开启。在 Nginx/ALB 终止 TLS 的部署形态下，需确保代理正确设置 `X-Forwarded-Proto`。

### 8. 审计日志留存（P2）

组织审计事件的最短留存期由 `AUDIT_LOG_RETENTION_DAYS` 控制，默认 180 天（≥6 个月，满足常见合规要求）。`StartAuditRetentionWorker` 周期性调用 `PurgeExpiredAuditEvents` 分批清理过期事件。

```bash
AUDIT_LOG_RETENTION_DAYS=180
```

### 9. 实名 / 身份核验（P2）

迁移 `000013_identity_binding` 为 `users` 增加 `real_name` / `identity_verified`，为 `organization_policies` 增加 `require_identity_verification`。

组织开启该策略后，未核验用户在 `AcceptOrganizationInvite` 阶段被拒绝，返回 `ErrIdentityVerificationRequired`。核验流程本身对接外部实名服务，仓库内只保留开关与判定。

## 运维检查清单

- [ ] 生产环境 `SECURITY_REQUIRE_TLS=true`，且反向代理正确透传 `X-Forwarded-Proto`
- [ ] `MESSAGE_ENCRYPTION_MASTER_KEY` 来自密钥管理系统，不出现在镜像、compose 文件或代码中
- [ ] 留存 TTL 与业务/法务确认后再调整，不要为了「方便排查」而无限拉长
- [ ] 清理 worker 已启动（检查 `cmd/cleanup-worker` 或 `EMBEDDED_WORKERS=1`）
- [ ] 审计留存不低于监管要求的最短年限
- [ ] 搜索索引集群若在信任边界外，确认 `SEARCH_INDEX_BODY_SNIPPET_MAX_RUNES` 足够小

## 新增迁移注意事项

迁移是双轨的：生产走 `backend/migrations/0000xx_*.sql`（golang-migrate），测试走 `models.AllModels()` 的 AutoMigrate。新增模型时**两处都要加**，并同步：

1. 提升 `backend/internal/runtime/migrations.go` 中的 `currentSchemaVersion`
2. 更新 `backend/internal/runtime/migrations_test.go` 的版本断言与字段清单

当前 `currentSchemaVersion = 13`。

## 相关文档

- [安全指南](./security-guidelines.md) — 通用基础设施与网络安全
- [配置说明](./configuration.md) — 完整配置项参考
- [隐私与账号注销支持材料](../deployment/privacy-and-account-deletion-support.md) — 应用商店合规材料
