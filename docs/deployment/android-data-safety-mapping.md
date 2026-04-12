# Android Data Safety Mapping

本文件用于 Google Play Data Safety 表单准备，和当前仓库实现保持一致。

## Collected Data

### Personal info
- Email address
  - 用途：账号注册、登录、联系人识别、密码重置、支持排查
  - 是否必需：是
  - 是否共享：否

### App activity
- In-app actions
  - 用途：最近通话、订阅 entitlement、翻译额度、黑名单/举报处理
  - 是否必需：是
  - 是否共享：否

### App info and performance
- Crash logs
  - 当前仓库尚未接入 Crashlytics 生产上报；正式发版前需要补齐
  - 是否共享：否

### Device or other IDs
- FCM token
  - 用途：来电推送唤醒
  - 是否必需：否
  - 是否共享：否

## Not Collected For Long-term Storage
- 原始通话音频
- 长期保存的实时翻译文本归档
- 媒体会话密钥

## Security
- 鉴权 token 使用 Keychain 存储，不落 AsyncStorage
- 翻译额度与 entitlement 以服务端为准
- 账号删除后清理联系人、通话历史、FCM token、entitlement、usage，并只保留非 PII 删除审计摘要

## Release Checklist
- 在 Play Console 中核对本文件与实际行为一致
- 若后续接入 Analytics/Crashlytics，需同步更新本文件
