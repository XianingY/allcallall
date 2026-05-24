# FCM 推送通知集成实现总结

## 概述

已完成两个关键步骤的实现：
1. ✅ 在 AuthContext.tsx 中集成 FCM Token 发送
2. ✅ 在后端实现 FCM Token 保存接口和推送通知逻辑

## 前端实现（移动端）

### 1. AuthContext.tsx 修改

**文件**: `/Users/byzantium/github/allcallall/mobile/src/context/AuthContext.tsx`

在 `login()` 方法中添加了 FCM Token 发送逻辑：

```typescript
import PushNotificationService from "../services/PushNotificationService";

const login = useCallback(
  async (email: string, password: string) => {
    const response = await authApi.login(email, password);
    await persistState(response.access_token, response.user);
    
    // 登录成功后，发送 FCM Token 到后端
    try {
      await PushNotificationService.sendCurrentTokenToBackend(response.access_token);
    } catch (error) {
      console.warn("[AuthContext] Failed to send FCM token:", error);
      // 不中断登录流程，继续进行
    }
  },
  [persistState]
);
```

**关键特性**：
- 登录成功后自动发送 FCM Token 到后端
- 不阻断登录流程，如果 FCM Token 发送失败仍继续登录
- 失败时只保留 `warn`，不打印流程型成功日志

### 2. users API 添加

**文件**: `/Users/byzantium/github/allcallall/mobile/src/api/users.ts`

添加了新的 API 函数：

```typescript
export const saveFCMToken = async (token: string, fcmToken: string) => {
  const api = createApiClient(token);
  await api.post("/users/fcm-token", { fcm_token: fcmToken });
};
```

## 后端实现

### 1. 数据库模型修改

**文件**: `/Users/byzantium/github/allcallall/backend/internal/models/user.go`

添加了 FCM Token 字段到 User 模型：

```go
type User struct {
    // ... 现有字段 ...
    FCMToken     string     `gorm:"size:255;index"` // Firebase Cloud Messaging token
    // ... 其他字段 ...
}
```

**特性**：
- FCM Token 有索引以便快速查询
- 自动迁移将在下次应用启动时添加该列

### 2. 用户服务扩展

**文件**: `/Users/byzantium/github/allcallall/backend/internal/user/service.go`

添加了两个新方法：

```go
// SaveFCMToken 保存或更新用户的 FCM Token
func (s *Service) SaveFCMToken(ctx context.Context, userID uint64, fcmToken string) error {
    if strings.TrimSpace(fcmToken) == "" {
        return errors.New("fcm token cannot be empty")
    }
    return s.repo.UpdateFCMToken(ctx, userID, strings.TrimSpace(fcmToken))
}

// GetFCMToken 获取用户的 FCM Token
func (s *Service) GetFCMToken(ctx context.Context, userID uint64) (string, error) {
    user, err := s.repo.FindByID(ctx, userID)
    if err != nil {
        return "", err
    }
    return user.FCMToken, nil
}
```

### 3. 用户存储库扩展

**文件**: `/Users/byzantium/github/allcallall/backend/internal/user/repository.go`

添加了数据库操作方法：

```go
// UpdateFCMToken 更新用户的 FCM Token
func (r *Repository) UpdateFCMToken(ctx context.Context, userID uint64, fcmToken string) error {
    return r.db.WithContext(ctx).Model(&models.User{}).
        Where("id = ?", userID).
        Update("fcm_token", fcmToken).Error
}
```

### 4. 用户 Handler 新增接口

**文件**: `/Users/byzantium/github/allcallall/backend/internal/handlers/user_handler.go`

添加了新的 HTTP 端点：`POST /users/fcm-token`

```go
type saveFCMTokenRequest struct {
    FCMToken string `json:"fcm_token" binding:"required"`
}

func (h *UserHandler) handleSaveFCMToken(c *gin.Context) {
    claims, err := auth.GetClaimsFromContext(c)
    if err != nil {
        JSONError(c, http.StatusUnauthorized, "unauthorized")
        return
    }

    var req saveFCMTokenRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        JSONError(c, http.StatusBadRequest, err.Error())
        return
    }

    if err := h.users.SaveFCMToken(c.Request.Context(), claims.UserID, req.FCMToken); err != nil {
        h.logger.Error().Err(err).Uint64("user_id", claims.UserID).Msg("save fcm token failed")
        JSONError(c, http.StatusInternalServerError, "failed to save fcm token")
        return
    }

    h.logger.Info().Uint64("user_id", claims.UserID).Str("fcm_token", req.FCMToken[:20]+"...").Msg("fcm token saved")
    JSONSuccess(c, http.StatusOK, gin.H{"message": "fcm token saved successfully"})
}
```

### 5. FCM 推送管理器

**文件**: `/Users/byzantium/github/allcallall/backend/internal/fcm/manager.go`

创建了 FCM 管理器，提供推送通知功能框架：

```go
FCM 管理器现在通过 Firebase Admin SDK 初始化 `messaging.Client`。配置 `FCM_SERVICE_ACCOUNT_PATH` 后会真实发送通知；未配置时会记录 `fcm disabled, skipping ...` 并安全降级。
```

### 6. 信令 Hub 推送通知集成

**文件**: `/Users/byzantium/github/allcallall/backend/internal/signaling/hub.go`

在信令处理中集成了推送通知：

```go
type Hub struct {
    // ... 现有字段 ...
    users        *user.Service
    fcmManager   *fcm.Manager
}

// WithUserService 附加用户服务
func (h *Hub) WithUserService(users *user.Service) { ... }

// WithFCMManager 附加 FCM 管理器
func (h *Hub) WithFCMManager(fcmMgr *fcm.Manager) { ... }

// sendCallNotification 发送来电推送通知
func (h *Hub) sendCallNotification(ctx context.Context, toEmail string, fromEmail string) { ... }
```

在 `handleIncoming()` 中添加了推送通知触发逻辑：

```go
// 如果是 call.invite 消息，需要发送推送通知
if msg.Type == TypeCallInvite {
    h.sendCallNotification(ctx, msg.To, msg.From)
}
```

### 7. 服务器初始化修改

**文件**: `/Users/byzantium/github/allcallall/backend/cmd/server/main.go`

初始化 FCM 管理器并将其附加到信令 Hub：

```go
// 初始化 FCM 管理器
fcmManager := fcm.NewManager(appLogger)
signalingHub.WithUserService(userSvc)
signalingHub.WithFCMManager(fcmManager)
```

## 工作流程

### 用户登录流程

1. 用户在移动端登录
2. AuthContext 调用 `authApi.login()`
3. 登录成功，获得 access token
4. 自动调用 `PushNotificationService.sendCurrentTokenToBackend(token)`
5. 移动端向后端 `POST /users/fcm-token` 发送 FCM Token
6. 后端保存 FCM Token 到数据库

### 来电推送通知流程

1. 用户 A 向用户 B 发起通话（`call.invite` 消息）
2. 信令 Hub 接收消息
3. Hub 检查是否存在 FCM Manager
4. 不阻塞地发送推送通知给用户 B（使用 goroutine）
5. 获取用户 B 的 FCM Token（如果存在）
6. 通过 FCM Manager 发送推送通知；若未配置 `FCM_SERVICE_ACCOUNT_PATH`，则安全跳过

## 下一步待完成

### 短期（移动端）

- [x] AuthContext 中集成 FCM Token 发送
- [x] 为移动端添加 API 函数

### 短期（后端）

- [x] 数据库模型添加 FCM Token 字段
- [x] 用户服务添加 Token 保存/获取方法
- [x] 新增 HTTP 端点 `POST /users/fcm-token`
- [x] 创建 FCM 推送管理器框架
- [x] 在信令 Hub 中集成推送通知逻辑

### 中期（Firebase 配置）

- [ ] 在 Firebase Console 创建项目和应用
- [ ] 下载 `google-services.json` 配置文件
- [ ] 获取 Firebase 服务账户 JSON 密钥
- [ ] 更新 FCM Manager 以使用真实的 Firebase 客户端

### 测试验证

- [ ] 验证登录时 FCM Token 正确发送到后端
- [ ] 验证数据库中 Token 已保存
- [ ] 验证来电时推送通知被发送（需要 Firebase SDK）
- [ ] 验证推送通知在应用后台时能正确显示

## Firebase 配置要求

当获得 Firebase 配置后，更新 `fcm/manager.go`：

```go
import "firebase.google.com/go/v4/messaging"

type Manager struct {
    logger zerolog.Logger
    client *messaging.Client
}

func (m *Manager) SendCallNotification(...) error {
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
            },
        },
    }
    _, err := m.client.Send(ctx, message)
    return err
}
```

## 文件变更汇总

### 新建文件
- `/Users/byzantium/github/allcallall/backend/internal/fcm/manager.go`

### 修改文件

**前端（2 个文件）**:
1. `/Users/byzantium/github/allcallall/mobile/src/context/AuthContext.tsx`
   - 添加 PushNotificationService import
   - 在 login() 中添加 FCM Token 发送逻辑

2. `/Users/byzantium/github/allcallall/mobile/src/api/users.ts`
   - 添加 saveFCMToken() 函数

**后端（7 个文件）**:
1. `/Users/byzantium/github/allcallall/backend/internal/models/user.go`
   - 添加 FCMToken 字段

2. `/Users/byzantium/github/allcallall/backend/internal/user/service.go`
   - 添加 SaveFCMToken() 方法
   - 添加 GetFCMToken() 方法

3. `/Users/byzantium/github/allcallall/backend/internal/user/repository.go`
   - 添加 UpdateFCMToken() 方法

4. `/Users/byzantium/github/allcallall/backend/internal/handlers/user_handler.go`
   - 添加 POST /users/fcm-token 路由
   - 添加 handleSaveFCMToken() 处理方法

5. `/Users/byzantium/github/allcallall/backend/internal/signaling/hub.go`
   - 添加 users 和 fcmManager 字段
   - 添加 WithUserService() 方法
   - 添加 WithFCMManager() 方法
   - 添加 sendCallNotification() 方法
   - 在 handleIncoming() 中添加推送通知触发

6. `/Users/byzantium/github/allcallall/backend/cmd/server/main.go`
   - 添加 fcm 包 import
   - 初始化 FCM Manager
   - 将 user service 和 FCM manager 附加到信令 Hub

## 验证

### 后端编译验证
✅ `go build ./cmd/server` - 成功

### 前端类型检查
- 现有的 TypeScript 错误（与本实现无关）
- AuthContext.tsx - 无新增错误
- users.ts API - 无新增错误
