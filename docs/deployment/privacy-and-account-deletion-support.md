# Privacy And Account Deletion Support

本文件作为上线支持 runbook，覆盖隐私、删除账号和用户支持的最小处理流程。

## Public Endpoints
- `GET /legal/terms`
- `GET /legal/privacy`
- `GET /legal/delete-account`
- `GET /api/v1/legal/current`

这些页面由当前 Go 服务直接承载，正式环境必须配置：
- `PUBLIC_WEB_BASE_URL`
- `SUPPORT_EMAIL`

## In-app Entry Points
- 注册页：查看条款与隐私政策
- 设置页：法律文档入口
- 设置页：删除账号入口
- 删除账号页：密码或邮箱验证码二次确认

## Account Deletion Behavior
- 删除时清理：
  - 账号邮箱和显示名称去标识化
  - 联系人关系
  - 通话历史
  - 黑名单关系
  - 举报记录
  - 法律接受记录
  - 翻译 usage ledger
  - 翻译 usage slice
  - entitlement
  - FCM token
- 删除后保留：
  - 不含可逆个人信息的删除审计摘要

## Support Handling
- 举报记录通过 `POST /api/v1/users/reports` 入库
- 支持邮件异步投递到 `SUPPORT_EMAIL`
- 若邮件发送失败：
  - 不回滚举报记录
  - 查看指标 `abuse_report_email_fail_total`
  - 使用内部支持接口排查

## Internal Support API
需要请求头：
- `X-Support-Token: <SUPPORT_API_TOKEN>`

接口：
- `GET /api/v1/internal/support/reports`
- `GET /api/v1/internal/support/users/:userId/summary`
- `GET /api/v1/internal/support/calls/:callId`

## Manual Response Template
- 确认用户邮箱或 user ID
- 确认是否已完成删除或举报提交
- 如为删除问题，检查 `deletion_audits`
- 如为翻译/订阅问题，检查 entitlement、usage、最近通话和 RevenueCat webhook
- 如为举报问题，检查支持 API 和支持邮箱投递状态
