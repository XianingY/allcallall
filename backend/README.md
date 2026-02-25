# AllCallAll Backend

Go 后端服务，提供实时信令、用户管理和数据存储支持。

## 📚 文档导航

- **[API 文档](../docs/api/api-documentation.md)** - 接口定义和使用说明
- **[数据库文档](../docs/api/database.md)** - 数据库结构设计
- **[快速启动](../docs/getting-started/quick-start.md)** - 开发环境搭建
- **[诊断指南](../docs/api/backend-diagnosis-and-fix.md)** - 常见问题排查

## 🛠️ 技术栈

- **语言**: Go 1.22+
- **Web 框架**: Gin
- **WebRTC**: Pion
- **数据库**: MySQL 8.0 (Gorm)
- **缓存**: Redis 7.2
- **认证**: JWT

## 🚀 常用命令

```bash
# 运行服务
go run cmd/server/main.go

# 运行测试
go test ./...

# 代码格式化
gofmt -s -w .
```
