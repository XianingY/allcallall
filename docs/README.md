# 📚 AllCallAll 项目文档

欢迎来到 AllCallAll 项目的文档中心！本文档按功能模块分类，方便您快速查找所需信息。

## 📂 文档目录

### 🚀 快速开始
- [快速启动指南](./getting-started/quick-start.md) - 最快的项目启动方式
- [Docker 启动指南](./getting-started/docker-startup-guide.md) - Docker 环境配置
- [环境变量配置](./getting-started/unified-env-config.md) - 统一的环境变量设置

### 🌍 部署相关
- [部署指南](./deployment/deployment-guide.md) - 完整部署指南（含架构图和命令速查）
- [部署检查清单](./deployment/deployment-checklist.md) - 部署前检查项目
- [生产环境和APK构建](./deployment/production-setup-and-apk-build.md) - 本地部署、Docker配置和APK构建
- [受限网络配置](./deployment/restricted-network-setup.md) - 企业网络和防火墙配置指南

### 🔌 API 和后端
- [API 文档](./api/api-documentation.md) - API 详细说明
- [数据库文档](./api/database.md) - 数据库结构和操作
- [后端诊断修复](./api/backend-diagnosis-and-fix.md) - 后端常见问题诊断
- [后端连接测试](./api/backend-connection-test-report.md) - 连接测试报告

### 📱 移动端开发
- [移动端文档中心](./mobile/README.md) - 完整的移动端开发文档
- [Alarm 功能增强](./mobile/features/alarm-enhancements-summary.md) - 来电提醒功能
- [音频配置](./mobile/setup/audio-files-setup.md) - 音频文件设置
- [环境配置](./mobile/setup/app-env-usage.md) - 应用环境变量
- [脚本使用指南](../mobile/scripts/README.md) - 移动端脚本和工具

### 🔔 推送通知功能
- [FCM 实现总结](./features/push-notifications/fcm-implementation-summary.md) - 实现概览
- [FCM 测试指南](./features/push-notifications/fcm-testing-guide.md) - 完整测试指南
- [FCM 快速参考](./features/push-notifications/fcm-quick-reference.md) - 命令和API速查
- [Firebase 集成指南](./features/push-notifications/firebase-integration-guide.md) - 详细集成步骤
- [推送通知修复指南](./features/push-notifications/push-notification-fix-guide.md) - 问题诊断和修复

### 🎵 Alarm 功能
- [Alarm 功能总结](./alarm/alarm-only-pr-guide.md) - Alarm 功能PR指南
- [重置状态报告](./alarm/final-reset-status.md) - Git 分支重置记录
- [Revert 状态报告](./alarm/revert-status.md) - 提交回滚记录

### ⚙️ 配置管理
- [系统配置](./configuration/configuration.md) - 项目配置说明
- [安全指南](./configuration/security-guidelines.md) - 项目安全规范

### 📝 开发参考
- [项目概览](./reference/CLAUDE.md) - 项目整体架构和说明
- [AI 助手指南](./reference/AGENTS.md) - Claude AI 辅助开发指南

### 📋 PR 和维护
- [PR 创建指南](./pr/pr-creation-guide.md) - 如何创建PR
- [PR 描述模板](./pr/pr-description-template.md) - PR描述模板
- [文件整理计划](./maintenance/file-organization-plan.md) - 项目结构整理方案
- [清理检查清单](./maintenance/cleanup-checklist.md) - 文件清理检查表
- [整理快速总结](./maintenance/organization-quick-summary.md) - 整理方案快速参考
- [归档总结](./maintenance/archive-summary.md) - 历史文档归档记录

## 🎯 快速导航

### 新开发者
1. 阅读 [README](../README.md) - 项目概述
2. 查看 [快速启动指南](./getting-started/quick-start.md) - 了解最快的启动方式
3. 阅读 [安全指南](./configuration/security-guidelines.md) - 安全规范

### 移动端开发者
1. 阅读 [移动端文档中心](./mobile/README.md) - 完整的移动端开发文档
2. 查看 [音频配置](./mobile/setup/audio-files-setup.md) - 了解音频功能
3. 参考 [推送通知实现](./features/push-notifications/fcm-implementation-summary.md) - 了解推送功能
4. 使用 [脚本工具](../mobile/scripts/README.md) - 运行验证脚本

### DevOps 工程师
1. 查看 [部署指南](./deployment/deployment-guide.md) - 完整部署步骤（含架构图和命令速查）
2. 使用 [部署检查清单](./deployment/deployment-checklist.md)
3. 参考 [生产环境和APK构建](./deployment/production-setup-and-apk-build.md)

### API 开发者
1. 阅读 [API 文档](./api/api-documentation.md)
2. 查看 [数据库文档](./api/database.md)
3. 参考 [后端诊断修复](./api/backend-diagnosis-and-fix.md) - 解决常见问题

## 📖 文档规范

- 所有文档使用 Markdown 格式
- 文档应包含清晰的目录结构
- 代码示例使用代码块标记
- 重要信息使用警告框突出显示

## 🤝 贡献指南

欢迎为项目文档做出贡献！请确保：
1. 文档内容准确、清晰
2. 使用统一的格式和风格
3. 及时更新过期信息

## 📞 联系我们

如有问题或建议，请通过以下方式联系：
- 提交 Issue
- 发起 Pull Request
- 联系项目维护者

---

**最后更新**: 2025-12-16  
**文档版本**: v2.0
