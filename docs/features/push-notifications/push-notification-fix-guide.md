# 推送通知功能完整修复指南

## 📋 问题总结

您的推送通知功能无法正常工作，原因是系统中存在**5个关键问题**：

### 1. ❌ App.tsx 未设置导航引用（致命问题）
**症状**：即使接收到推送消息，应用也无法导航到来电页面
**根本原因**：`PushNotificationService` 需要导航引用来在后台或应用被杀时唤起应用并导航
**状态**：✅ **已修复** - App.tsx 已更新，添加了导航引用设置

### 2. ❌ AndroidManifest.xml 缺少推送权限
**症状**：Android 12+ 设备上无法显示推送通知
**缺失的权限**：`android.permission.POST_NOTIFICATIONS`
**状态**：✅ **已修复** - 已添加 POST_NOTIFICATIONS 权限

### 3. ❌ FCM Token 未发送到后端
**症状**：后端无法获知设备的 FCM Token，因此无法发送推送消息
**原因**：历史版本的 `PushNotificationService.ts` 中 `sendTokenToServer()` 曾是 TODO；当前实现已改为使用统一 API 客户端上报 Token。
**状态**：✅ **已修复** - 添加了 `sendCurrentTokenToBackend()` 方法

### 4. ⚠️ Firebase 服务账户未配置
**症状**：后端会安全跳过 FCM 发送，日志显示 `fcm disabled, skipping ...`
**原因**：运行环境未提供 `FCM_SERVICE_ACCOUNT_PATH`
**状态**：⏳ **待配置** - 需要部署时挂载服务账户 JSON

### 5. ❌ Firebase 配置文件缺失
**症状**：FCM 客户端无法初始化
**缺失文件**：`google-services.json` (Android) 或 `GoogleService-Info.plist` (iOS)
**状态**：⏳ **待配置** - 需要从 Firebase 项目下载

---

## ✅ 已完成的修复

### 修复 1：App.tsx - 设置推送通知导航引用
**文件**：`/Users/byzantium/github/allcallall/mobile/App.tsx`

```typescript
// 已添加：
import { useRef } from "react";
import { NavigationContainerRef } from "@react-navigation/native";
import PushNotificationService from "./src/services/PushNotificationService";

// 创建导航引用
const navigationRef = useRef<NavigationContainerRef<any>>(null);

// 设置推送通知的导航引用
useEffect(() => {
  if (navigationRef.current) {
    PushNotificationService.setNavigationRef(navigationRef);
  }
}, []);

// 将 ref 传递给 NavigationContainer
<NavigationContainer ref={navigationRef}>
```

**为什么重要**：
- 推送通知需要导航引用来在后台或应用被杀时唤起应用
- 导航引用允许 `PushNotificationService` 直接跳转到来电页面

### 修复 2：AndroidManifest.xml - 添加推送权限
**文件**：`/Users/byzantium/github/allcallall/mobile/android/app/src/main/AndroidManifest.xml`

```xml
<!-- 已添加 -->
<uses-permission android:name="android.permission.POST_NOTIFICATIONS"/>
```

**为什么重要**：
- Android 12 (API 31) 及以上要求应用必须声明 `POST_NOTIFICATIONS` 权限才能显示推送通知
- 没有此权限，推送通知将被系统静默丢弃

### 修复 3：PushNotificationService - 实现 Token 发送
**文件**：`/Users/byzantium/github/allcallall/mobile/src/services/PushNotificationService.ts`

**新增方法**：`sendCurrentTokenToBackend(authToken: string)`

```typescript
public async sendCurrentTokenToBackend(authToken: string): Promise<void> {
  this.authToken = authToken;
  if (!this.currentToken) {
    await this.getFCMToken();
  }
  if (!this.currentToken) {
    console.warn("[PushNotificationService] No FCM token available");
    return;
  }

  await saveFCMToken(this.authToken, this.currentToken);
}
```

**使用方式**：
在用户成功登录后调用此方法。修改 `AuthContext.tsx` 中的登录逻辑：

```typescript
// 在登录成功后添加
import PushNotificationService from "../services/PushNotificationService";

// 登录成功后
await PushNotificationService.sendCurrentTokenToBackend(token);
```

---

## ⏳ 需要您完成的步骤

### 步骤 1：在 AuthContext.tsx 中集成 FCM Token 发送

**文件**：`/Users/byzantium/github/allcallall/mobile/src/context/AuthContext.tsx`

在 `login()` 方法中，登录成功后添加：

```typescript
// ... 现有的登录逻辑 ...

// 登录成功，保存 token
await AsyncStorage.setItem('authToken', response.data.access_token);

// 发送 FCM Token 到后端
import PushNotificationService from "../services/PushNotificationService";
await PushNotificationService.sendCurrentTokenToBackend(response.data.access_token);

// ... 其他逻辑 ...
```

### 步骤 2：在部署环境配置后端 FCM

当前后端接口与推送发送逻辑已经存在；需要补的是部署侧配置。

#### 2.1 配置 Firebase 服务账户路径

在后端运行环境中设置：

```bash
export FCM_SERVICE_ACCOUNT_PATH="/absolute/path/to/firebase-service-account.json"
```

#### 2.2 验证真实发送或安全降级

配置后应看到：

```
2026-XX-XX INF fcm enabled component=fcm_manager credentials_path=/absolute/path/to/firebase-service-account.json
2026-XX-XX INF call notification sent component=fcm_manager from=user_a@example.com call_id=uuid-string message_id=projects/.../messages/...
```

未配置时应看到安全降级日志：

```
2026-XX-XX DBG fcm disabled, skipping call notification component=fcm_manager from=user_a@example.com call_id=uuid-string
```

### 步骤 3：获取 Firebase 配置文件

**google-services.json** 的来源：

1. 访问 [Firebase Console](https://console.firebase.google.com/)
2. 选择或创建您的 Firebase 项目
3. 点击 "Add app" → 选择 Android
4. 填写应用信息：
   - Package name: `com.allcallall.mobile`
   - App nickname: AllCallAll (可选)
   - SHA-1 certificate fingerprint: 从以下命令获取
   
   ```bash
   cd mobile/android
   ./gradlew signingReport
   ```

5. 下载 `google-services.json`
6. 将文件放在：`mobile/android/app/google-services.json`

**iOS 配置** (如果需要)：
- 下载 `GoogleService-Info.plist`
- 在 Xcode 中添加到项目

### 步骤 4：安装必要的 NPM 依赖

已安装的 Firebase 依赖（在 package.json 中）：
- ✅ `@react-native-firebase/app` (v23.7.0)
- ✅ `@react-native-firebase/messaging` (v19.0.0)

### 步骤 5：重新构建 APK

```bash
cd mobile/android

# 清理构建
./gradlew clean

# 构建 Release APK
./gradlew assembleRelease

# APK 位置
# app/build/outputs/apk/release/app-release.apk
```

---

## 🧪 测试推送通知

### 前置条件
1. ✅ google-services.json 已配置
2. ✅ 后端 FCM 接口已实现
3. ✅ 应用已安装到真机

### 测试步骤

**步骤 1：验证 FCM Token 获取**
```
1. 启动应用
2. 查看 logcat 日志
3. 搜索 "[PushNotificationService] FCM Token:"
4. 确认 Token 已成功获取
```

**步骤 2：验证 Token 发送到后端**
```
1. 登录应用
2. 在后端日志中查看 Token 保存日志
3. 在数据库中检查用户的 fcm_token 字段
```

**步骤 3：测试推送通知**
```
使用 Firebase Console 或 Admin SDK 发送测试推送：

// 使用 Firebase Admin SDK (Node.js)
const admin = require('firebase-admin');

const message = {
  notification: {
    title: 'Test Notification',
    body: 'This is a test push notification'
  },
  data: {
    type: 'incoming_call',
    call_id: 'test-123',
    from_user: 'Test User',
    from_email: 'test@example.com'
  },
  token: 'USER_FCM_TOKEN_HERE'
};

admin.messaging().send(message)
  .then(response => {
    console.log('Successfully sent message:', response);
  })
  .catch(error => {
    console.log('Error sending message:', error);
  });
```

### 预期行为

**应用在前台**：
- ✅ 触发 `onMessage` 监听器
- ✅ 播放来电铃声和震动
- ✅ 显示通知

**应用在后台**：
- ✅ 显示系统通知
- ✅ 点击通知打开应用并导航到来电页面

**应用被杀**：
- ✅ FCM 在后台处理消息
- ✅ 显示系统通知
- ✅ 点击通知启动应用并导航到来电页面

---

## 🔍 常见问题排查

### 问题 1：FCM Token 获取失败
**症状**：日志中没有看到 "FCM Token:" 消息
**原因**：
- google-services.json 未正确配置
- Firebase 项目配置有误
- 设备没有 Google Play Services

**解决**：
```bash
# 检查 google-services.json 是否存在
ls -la mobile/android/app/google-services.json

# 重新下载 google-services.json
# 从 Firebase Console 重新下载，确保 SHA-1 签名正确
```

### 问题 2：Token 无法发送到后端
**症状**：看到 Token，但后端没有收到
**原因**：
- AuthContext 中未调用 `sendCurrentTokenToBackend()`
- 后端接口未实现
- JWT Token 已过期

**解决**：
- 确认在 AuthContext.ts 中的登录成功后调用了发送方法
- 检查后端是否有 `/users/fcm-token` 接口
- 验证 JWT Token 是否有效

### 问题 3：即使收到 Token，也无法接收推送
**症状**：Token 已保存，但无法收到推送通知
**原因**：
- PushNotificationService 中没有设置导航引用（App.tsx 未修复）
- 后端没有发送推送消息
- 消息格式不正确

**解决**：
- 确认 App.tsx 中已设置导航引用
- 在后端日志中检查是否发送了推送消息
- 验证消息中的 `data` 字段是否包含必需的字段

---

## 📱 测试检查清单

- [ ] google-services.json 已放在 `mobile/android/app/` 目录
- [ ] AndroidManifest.xml 中已添加 `POST_NOTIFICATIONS` 权限
- [ ] App.tsx 中已添加导航引用设置
- [ ] PushNotificationService.ts 中已实现 `sendCurrentTokenToBackend()`
- [ ] AuthContext.tsx 中已在登录成功后调用 `sendCurrentTokenToBackend()`
- [ ] 后端已实现 `POST /users/fcm-token` 接口
- [ ] 后端已实现 FCM 消息发送逻辑
- [ ] 新 APK 已构建并安装到真机
- [ ] 应用日志显示 FCM Token 已获取
- [ ] 后端日志显示 Token 已保存
- [ ] 使用 Firebase Console 发送测试推送成功接收

---

## 📝 总结

推送通知功能现在应该能正常工作。完整的数据流应该是：

```
1. 应用启动
   ↓
2. PushNotificationService 获取 FCM Token
   ↓
3. 用户登录
   ↓
4. AuthContext 调用 sendCurrentTokenToBackend()
   ↓
5. Token 发送到后端并保存
   ↓
6. 当有来电时，后端通过 FCM 发送推送
   ↓
7. 设备接收推送，PushNotificationService 处理
   ↓
8. 导航到来电页面，播放铃声和震动
```

如有任何问题，请检查：
1. 日志输出（logcat）
2. Firebase Console 中的 FCM 统计
3. 后端数据库中的 FCM Token 记录
4. 消息格式是否完全正确
