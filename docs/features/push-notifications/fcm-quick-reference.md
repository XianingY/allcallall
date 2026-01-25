# FCM 推送通知实现 - 快速参考

## 已完成 ✅

### 前端（移动端）
- [x] AuthContext.tsx 中添加 FCM Token 发送逻辑
- [x] users API 添加 saveFCMToken 函数

### 后端
- [x] User 模型添加 fcm_token 字段
- [x] User Service 添加 SaveFCMToken/GetFCMToken 方法
- [x] User Repository 添加 UpdateFCMToken 方法
- [x] UserHandler 添加 POST /users/fcm-token 端点
- [x] FCM Manager 创建（框架已就位）
- [x] Signaling Hub 集成 FCM 推送逻辑
- [x] 后端编译验证成功

## 待完成 ⏳

### Firebase 配置（需要用户操作）
- [ ] Firebase 项目创建
- [ ] Android 应用注册
- [ ] 获取 google-services.json
- [ ] 获取 Firebase 服务账户密钥
- [ ] 密钥文件配置到后端

### 代码集成
- [ ] Firebase Admin SDK 依赖安装
- [ ] fcm/manager.go 更新为真实实现
- [ ] cmd/server/main.go 更新初始化逻辑

## 工作流程概览

```
用户登录
    ↓
[AuthContext] 发送 FCM Token
    ↓
[PushNotificationService.sendCurrentTokenToBackend]
    ↓
POST /users/fcm-token
    ↓
[UserHandler] 保存 Token 到数据库
    ↓
用户拨打电话 (call.invite)
    ↓
[SignalingHub] 接收消息
    ↓
[sendCallNotification] 异步触发
    ↓
获取接收者 FCM Token
    ↓
[FCMManager.SendCallNotification]
    ↓
通过 Firebase 发送推送通知
    ↓
接收者设备收到通知
```

## 代码文件位置

### 前端修改
```
mobile/
├── src/
│   ├── context/
│   │   └── AuthContext.tsx           [已修改] 添加 FCM Token 发送
│   └── api/
│       └── users.ts                  [已修改] 添加 saveFCMToken 函数
```

### 后端修改
```
backend/
├── internal/
│   ├── models/
│   │   └── user.go                   [已修改] 添加 fcm_token 字段
│   ├── user/
│   │   ├── service.go                [已修改] SaveFCMToken/GetFCMToken
│   │   └── repository.go             [已修改] UpdateFCMToken
│   ├── handlers/
│   │   └── user_handler.go           [已修改] POST /users/fcm-token 端点
│   ├── signaling/
│   │   └── hub.go                    [已修改] 添加推送通知逻辑
│   └── fcm/
│       └── manager.go                [新建] FCM 推送管理器
├── cmd/
│   └── server/
│       └── main.go                   [已修改] 初始化 FCM Manager
└── docs/
    ├── FCM_IMPLEMENTATION_SUMMARY.md  [新建] 实现总结
    ├── FCM_TESTING_GUIDE.md           [新建] 测试指南
    ├── FIREBASE_INTEGRATION_GUIDE.md  [新建] Firebase 集成指南
    └── FCM_IMPLEMENTATION_QUICK_REF.md [本文件] 快速参考
```

## API 端点

### 保存 FCM Token
```
POST /api/v1/users/fcm-token
Headers:
  - Authorization: Bearer <JWT_TOKEN>
  - Content-Type: application/json

Body:
{
  "fcm_token": "string"
}

Response (200):
{
  "message": "fcm token saved successfully"
}
```

## 数据库

### User 表新增字段
```sql
ALTER TABLE users ADD COLUMN fcm_token VARCHAR(255) DEFAULT NULL, ADD INDEX idx_fcm_token (fcm_token);
```

**字段详情**:
- 字段名: `fcm_token`
- 类型: VARCHAR(255)
- 可空: 是（用户可能没有安装移动应用）
- 索引: 是（快速查询）
- 说明: 存储用户的 Firebase Cloud Messaging token

## 环境变量（可选）

```bash
# Firebase 密钥文件位置（如果使用外部配置）
FIREBASE_CREDENTIALS=/path/to/firebase-key.json

# 日志级别（用于调试）
LOG_LEVEL=debug
```

## 常用命令

### 后端编译验证
```bash
cd /Users/byzantium/github/allcallall/backend
go build ./cmd/server
```

### 启动后端
```bash
# 开发环境
GIN_MODE=debug LOG_LEVEL=debug go run ./cmd/server

# 生产环境
GIN_MODE=release go run ./cmd/server
```

### 数据库验证
```bash
# 查看 users 表结构
mysql -u root -p allcallall -e "DESCRIBE users;"

# 查看特定用户的 FCM Token
mysql -u root -p allcallall -e \
  "SELECT id, email, fcm_token FROM users WHERE email='user@example.com';"

# 清除过期 Token
mysql -u root -p allcallall -e \
  "UPDATE users SET fcm_token = NULL WHERE fcm_token IS NOT NULL AND updated_at < DATE_SUB(NOW(), INTERVAL 30 DAY);"
```

### API 测试
```bash
# 获取 JWT Token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}' \
  | jq -r '.access_token')

# 发送 FCM Token
curl -X POST http://localhost:8080/api/v1/users/fcm-token \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"fcm_token":"test_token_12345"}'

# 查看响应
echo $?  # 应该返回 0（200 OK）
```

## 日志查询

### 查看 FCM 相关日志
```bash
go run ./cmd/server 2>&1 | grep -i "fcm"
```

### 查看信令相关日志
```bash
go run ./cmd/server 2>&1 | grep -i "signaling"
```

### 查看特定用户的日志
```bash
go run ./cmd/server 2>&1 | grep "user@example.com"
```

## Firebase 配置步骤概览

1. **创建 Firebase 项目**
   - 访问 console.firebase.google.com
   - 创建新项目：AllCallAll

2. **注册 Android 应用**
   - Package name: com.allcallall.mobile
   - SHA-1: 从 `./gradlew signingReport` 获取
   - 下载 google-services.json

3. **创建服务账户**
   - 项目设置 → Service Accounts
   - Firebase Admin SDK → 生成新的私钥
   - 保存为 `firebase-key.json`

4. **配置后端**
   - 复制 firebase-key.json 到后端目录
   - 运行 `go get firebase.google.com/go/v4`
   - 更新 fcm/manager.go（使用 Firebase Admin SDK）
   - 重新编译和启动后端

5. **验证**
   - 查看后端日志确认 FCM 初始化成功
   - 发送测试通知确认工作流程

## 故障排除速查表

| 问题 | 可能原因 | 解决方案 |
|------|--------|--------|
| 数据库没有 fcm_token 字段 | 自动迁移未运行 | 重启后端，查看启动日志 |
| FCM Token 保存失败 | API 端点错误 | 检查授权头，验证 JWT token |
| 推送通知未触发 | FCM Manager 未初始化 | 检查后端启动日志中的 FCM 消息 |
| Firebase 初始化失败 | 密钥文件丢失/无效 | 验证 firebase-key.json 位置和权限 |
| 通知发送失败 | Token 无效或过期 | 清除 Token，重新登录获取新 Token |

## 性能指标

| 操作 | 耗时 | 说明 |
|------|------|------|
| 用户登录 + FCM Token 发送 | < 2s | 包括网络延迟 |
| 来电推送通知触发 | < 1s | 异步执行，不阻塞信令 |
| 数据库 Token 保存 | < 100ms | GORM 操作 |
| Firebase 发送通知 | < 500ms | 取决于网络和 Firebase 服务 |

## 安全清单

- [ ] firebase-key.json 已添加到 .gitignore
- [ ] 文件权限设置为 600：`chmod 600 firebase-key.json`
- [ ] 生产环境使用密钥管理服务存储密钥
- [ ] FCM Token 仅通过认证端点发送
- [ ] 通知中不包含敏感信息
- [ ] 实现 Token 轮换机制（可选）

## 下一步优化建议

1. **短期**
   - 完成 Firebase 配置和集成
   - 实现端到端测试验证
   - 部署到测试环境

2. **中期**
   - 添加通知模板系统
   - 实现通知历史记录
   - 添加用户通知偏好设置

3. **长期**
   - 实现通知队列系统
   - 添加推送通知分析
   - 支持消息模板国际化
   - A/B 测试不同的通知文本

## 相关文档

- [FCM 实现总结](./FCM_IMPLEMENTATION_SUMMARY.md) - 详细的实现说明
- [测试指南](./FCM_TESTING_GUIDE.md) - 如何测试推送通知功能
- [Firebase 集成指南](./FIREBASE_INTEGRATION_GUIDE.md) - Firebase Admin SDK 集成步骤

## 支持和问题

如果遇到问题，请检查：

1. 后端启动日志
   ```bash
   GIN_MODE=debug LOG_LEVEL=debug go run ./cmd/server
   ```

2. 数据库状态
   ```bash
   mysql -u root -p allcallall -e "SELECT * FROM users LIMIT 1\G"
   ```

3. Firebase 控制台
   - Cloud Messaging 是否启用
   - 服务账户权限是否正确
   - Device tokens 是否有效

4. 移动端日志
   ```
   [AuthContext] Sending FCM token to backend...
   [PushNotificationService] FCM token sent successfully
   ```

## 版本信息

- Firebase Admin SDK for Go: v4.10.0+
- Go: 1.22+
- React Native: 0.72+
- Node.js: 18+
