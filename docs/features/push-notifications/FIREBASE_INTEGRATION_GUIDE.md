# Firebase Admin SDK 集成指南

## 概述

本指南说明如何在 AllCallAll 后端集成 Firebase Cloud Messaging (FCM) 来发送真实的推送通知。

## 前置条件

- Firebase 项目（需创建）
- Android 应用已注册到 Firebase
- google-services.json 配置文件
- Firebase 服务账户密钥 JSON 文件

## 步骤 1：Firebase 项目设置

### 1.1 创建 Firebase 项目

1. 访问 [Firebase Console](https://console.firebase.google.com/)
2. 点击 "Add project"
3. 项目名称：`AllCallAll`
4. 创建项目

### 1.2 添加 Android 应用

1. 在 Firebase 项目中，点击 "Add app"
2. 选择 "Android"
3. 填写应用信息：
   - Android package name: `com.allcallall.mobile`
   - App nickname: `AllCallAll Mobile` (可选)
   - SHA-1 certificate fingerprint (需要)

#### 获取 SHA-1 证书指纹

**对于调试签名**:
```bash
cd /Users/byzantium/github/allcallall/mobile/android
./gradlew signingReport
```

在输出中查找 SHA-1 值，格式如：
```
Variant: debug
Config: debug
Store: ~/.android/keystore
Alias: AndroidDebugKey
MD5: XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX
SHA1: XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX
SHA-256: XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX:XX
```

4. 将 SHA-1 值粘贴到 Firebase 控制台
5. 点击 "Register app"
6. 下载 `google-services.json`
7. 将文件放在 `mobile/android/app/google-services.json`

### 1.3 启用 Cloud Messaging

1. 在 Firebase 项目中，进入 "Cloud Messaging" 标签
2. 确保已启用 FCM 服务
3. 记下 "Sender ID" (在发送测试通知时需要)

## 步骤 2：获取服务账户密钥

### 2.1 创建服务账户

1. 在 Firebase 控制台，点击项目设置图标 (⚙️)
2. 进入 "Service accounts"
3. 切换到 "Firebase Admin SDK" 标签
4. 点击 "Generate new private key"
5. 确认对话框
6. 自动下载 JSON 密钥文件（保管好这个文件，它包含敏感信息）

### 2.2 保存密钥文件

```bash
# 将下载的 JSON 文件保存到项目中（不提交到版本控制）
cp ~/Downloads/allcallall-xxxxx-firebase-adminsdk-xxxxx.json \
  /Users/byzantium/github/allcallall/backend/firebase-key.json

# 添加到 .gitignore
echo "firebase-key.json" >> /Users/byzantium/github/allcallall/backend/.gitignore
```

## 步骤 3：Go 依赖配置

### 3.1 添加 Firebase Admin SDK

在后端目录中，添加 Firebase 依赖：

```bash
cd /Users/byzantium/github/allcallall/backend
go get firebase.google.com/go/v4
go get firebase.google.com/go/v4/messaging
```

### 3.2 更新 go.mod

运行 `go mod tidy` 确保依赖正确：

```bash
go mod tidy
go mod download
```

## 步骤 4：后端代码集成

### 4.1 修改 fcm/manager.go

替换现有的 manager.go 文件内容：

```go
package fcm

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/rs/zerolog"
	"google.golang.org/api/option"
)

// Manager handles Firebase Cloud Messaging operations
type Manager struct {
	logger zerolog.Logger
	client *messaging.Client
}

// NewManager creates a new FCM manager
func NewManager(logger zerolog.Logger) (*Manager, error) {
	ctx := context.Background()
	
	// 从环境变量或配置文件加载 Firebase 密钥
	credentialsFile := "firebase-key.json"
	opt := option.WithCredentialsFile(credentialsFile)
	
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, fmt.Errorf("initialize firebase app: %w", err)
	}
	
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialize messaging client: %w", err)
	}
	
	return &Manager{
		logger: logger.With().Str("component", "fcm_manager").Logger(),
		client: client,
	}, nil
}

// SendCallNotification sends an incoming call notification
func (m *Manager) SendCallNotification(ctx context.Context, fcmToken string, fromEmail string, displayName string, callID string) error {
	if fcmToken == "" {
		m.logger.Debug().Str("from", fromEmail).Msg("fcm token is empty, skipping notification")
		return nil
	}

	if m.client == nil {
		m.logger.Error().Msg("firebase messaging client not initialized")
		return fmt.Errorf("firebase client not available")
	}

	message := &messaging.Message{
		Token: fcmToken,
		Notification: &messaging.Notification{
			Title: "AllCallAll - 来电",
			Body:  fmt.Sprintf("%s 正在呼叫您", displayName),
		},
		Data: map[string]string{
			"type":       "incoming_call",
			"call_id":    callID,
			"from_email": fromEmail,
			"from_name":  displayName,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				Sound: "default",
				Title: "AllCallAll - 来电",
				Body:  fmt.Sprintf("%s 正在呼叫您", displayName),
			},
		},
	}

	response, err := m.client.Send(ctx, message)
	if err != nil {
		m.logger.Error().Err(err).
			Str("to_token", fcmToken[:20]+"...").
			Str("from_email", fromEmail).
			Msg("failed to send call notification")
		return err
	}

	m.logger.Info().
		Str("to_token", fcmToken[:20]+"...").
		Str("from_email", fromEmail).
		Str("message_id", response).
		Msg("call notification sent successfully")

	return nil
}

// SendMissedCallNotification sends a missed call notification
func (m *Manager) SendMissedCallNotification(ctx context.Context, fcmToken string, fromEmail string, displayName string) error {
	if fcmToken == "" {
		m.logger.Debug().Str("from", fromEmail).Msg("fcm token is empty, skipping notification")
		return nil
	}

	if m.client == nil {
		m.logger.Error().Msg("firebase messaging client not initialized")
		return fmt.Errorf("firebase client not available")
	}

	message := &messaging.Message{
		Token: fcmToken,
		Notification: &messaging.Notification{
			Title: "AllCallAll - 未接来电",
			Body:  fmt.Sprintf("您有来自 %s 的未接来电", displayName),
		},
		Data: map[string]string{
			"type":       "missed_call",
			"from_email": fromEmail,
			"from_name":  displayName,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Notification: &messaging.AndroidNotification{
				Sound: "default",
				Title: "AllCallAll - 未接来电",
				Body:  fmt.Sprintf("您有来自 %s 的未接来电", displayName),
			},
		},
	}

	response, err := m.client.Send(ctx, message)
	if err != nil {
		m.logger.Error().Err(err).
			Str("to_token", fcmToken[:20]+"...").
			Str("from_email", fromEmail).
			Msg("failed to send missed call notification")
		return err
	}

	m.logger.Info().
		Str("to_token", fcmToken[:20]+"...").
		Str("from_email", fromEmail).
		Str("message_id", response).
		Msg("missed call notification sent successfully")

	return nil
}
```

### 4.2 修改 cmd/server/main.go

更新 FCM Manager 初始化：

```go
// 初始化 FCM 管理器
// Initialize FCM manager
fcmManager, err := fcm.NewManager(appLogger)
if err != nil {
	appLogger.Warn().Err(err).Msg("failed to initialize FCM manager, push notifications will be disabled")
	fcmManager = nil // 如果初始化失败，禁用推送通知
} else {
	signalingHub.WithUserService(userSvc)
	signalingHub.WithFCMManager(fcmManager)
}
```

**注意**: 如果 Firebase 初始化失败，应用仍会继续运行，但推送通知功能会被禁用。

### 4.3 环境变量配置 (可选)

如果想通过环境变量指定密钥文件位置，修改 fcm/manager.go：

```go
// 从环境变量或默认位置加载 Firebase 密钥
credentialsFile := os.Getenv("FIREBASE_CREDENTIALS")
if credentialsFile == "" {
	credentialsFile = "firebase-key.json"
}

opt := option.WithCredentialsFile(credentialsFile)
```

启动时：
```bash
FIREBASE_CREDENTIALS=/path/to/firebase-key.json go run ./cmd/server
```

## 步骤 5：生产环境部署

### 5.1 使用 Docker 部署

在 `Dockerfile` 中，确保 Firebase 密钥被正确复制：

```dockerfile
# 添加到 Dockerfile
COPY firebase-key.json /app/firebase-key.json
RUN chmod 600 /app/firebase-key.json
```

或使用挂载卷：

```bash
docker run -v /path/to/firebase-key.json:/app/firebase-key.json ...
```

### 5.2 Kubernetes 部署

使用 Secret 管理 Firebase 密钥：

```bash
# 创建 Secret
kubectl create secret generic firebase-key \
  --from-file=firebase-key.json=/path/to/firebase-key.json

# 在 Deployment 中挂载
volumeMounts:
  - name: firebase-key
    mountPath: /app/firebase-key.json
    subPath: firebase-key.json
    readOnly: true

volumes:
  - name: firebase-key
    secret:
      secretName: firebase-key
```

## 步骤 6：测试推送通知

### 6.1 使用 Firebase Console 测试

1. 在 Firebase Console，进入 Cloud Messaging
2. 点击 "Send your first message"
3. 创建通知
4. 粘贴一个有效的 FCM Token
5. 点击 "Send test message"

### 6.2 使用 curl 测试（通过后端）

```bash
# 首先登录获取有效的 JWT token
TOKEN=$(curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}' \
  | jq -r '.access_token')

# 发送 FCM Token
curl -X POST http://localhost:8080/api/v1/users/fcm-token \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"fcm_token":"test_token_xxxxx"}'
```

## 故障排除

### 问题 1：Firebase 初始化失败

**错误**: `failed to initialize FCM manager`

**解决**:
1. 检查 `firebase-key.json` 是否存在且有效
2. 确保文件路径正确（从后端根目录相对）
3. 检查文件权限：`ls -la firebase-key.json`
4. 验证 JSON 格式：`jq . firebase-key.json`

### 问题 2：无权限发送消息

**错误**: `permission denied: ...`

**解决**:
1. 在 Firebase Console 检查项目 ID
2. 验证服务账户有 "Editor" 或相应角色
3. 尝试重新生成服务账户密钥

### 问题 3：FCM Token 无效

**错误**: `invalid registration token`

**解决**:
1. 确保 Token 是从 Google Play Services 获取的
2. Token 应该是完整的字符串，不包含空格或换行符
3. 检查 Token 是否已过期（Token 可能在应用卸载/重装后变化）

## 性能建议

### 1. 批量发送

如果需要发送大量通知，使用批量 API：

```go
// 发送多个通知
resp, err := m.client.SendAll(ctx, []*messaging.Message{
	message1,
	message2,
	message3,
})
```

### 2. 异步处理

保持现有的 goroutine 异步发送实现，避免阻塞信令处理。

### 3. 监控和日志

启用详细日志以监控推送通知状态：

```bash
LOG_LEVEL=debug go run ./cmd/server 2>&1 | grep fcm
```

## 安全考虑

### 1. 保护 Firebase 密钥

- ✅ 添加 `firebase-key.json` 到 `.gitignore`
- ✅ 限制文件权限：`chmod 600 firebase-key.json`
- ✅ 在生产环境使用密钥管理服务（如 AWS Secrets Manager、Vault）
- ❌ 不要在代码中硬编码密钥

### 2. Token 安全

- 仅通过认证的 API 端点 (`POST /users/fcm-token`) 发送 Token
- 定期刷新 Token（Firebase SDK 会自动处理）
- 删除用户时清除其 FCM Token

### 3. 通知内容

- 不在通知中包含敏感信息
- 通知标题和正文应该是安全的（已在代码中实现）

## 后续优化

### 1. Token 管理

实现 Token 轮换机制：
- 追踪 Token 年龄
- 定期更新过期 Token
- 清除无效 Token

### 2. 通知队列

实现消息队列以处理大量通知：
- 使用 Redis 作为队列
- 后台 Worker 处理发送
- 重试机制处理失败

### 3. 通知模板

创建通知模板系统：
- 国际化支持
- 可配置的通知文本
- A/B 测试支持

## 参考资源

- [Firebase Admin SDK for Go](https://pkg.go.dev/firebase.google.com/go/v4)
- [Firebase Cloud Messaging Documentation](https://firebase.google.com/docs/cloud-messaging)
- [Android Firebase Integration](https://firebase.google.com/docs/android/setup)
