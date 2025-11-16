# 原生构建指南 - 完整音视频通话功能

本指南说明如何从 Expo 切换到原生构建，以实现完整的 WebRTC 音视频通话功能。

## 为什么需要原生构建？

`react-native-webrtc` 是一个原生模块，**不支持在 Expo Go 中运行**。要使用完整的音视频通话功能，必须构建原生应用。

## 前置要求

### Android 开发环境

1. **安装 Android SDK**
   ```bash
   brew install android-sdk
   ```

2. **设置环境变量**
   ```bash
   export ANDROID_HOME=~/Library/Android/sdk
   export PATH=$PATH:$ANDROID_HOME/tools:$ANDROID_HOME/platform-tools
   ```

3. **安装构建工具**
   - Android SDK Platform 34
   - Android Build Tools 34.0.0
   - Android Emulator

### iOS 开发环境（Mac 用户）

```bash
brew install cocoapods
```

## 构建原生应用

### Android

```bash
cd /Users/byzantium/github/allcall/mobile

# 构建 APK（开发版）
npx expo run:android --device

# 或构建到 emulator
npx expo run:android
```

**首次构建时间较长（10-15 分钟），后续增量构建更快。**

### iOS

```bash
cd /Users/byzantium/github/allcall/mobile

# 构建到真机
npx expo run:ios --device

# 或构建到模拟器
npx expo run:ios
```

## 应用配置说明

`app.json` 中已配置所有必要权限：

```json
{
  "android": {
    "permissions": [
      "INTERNET",
      "RECORD_AUDIO",
      "MODIFY_AUDIO_SETTINGS",
      "CAMERA",
      "BLUETOOTH",
      "BLUETOOTH_ADMIN",
      "BLUETOOTH_CONNECT"
    ]
  },
  "ios": {
    "infoPlist": {
      "NSMicrophoneUsageDescription": "Allow AllCallAll to access the microphone for voice calls"
    }
  }
}
```

## 完整通话流程

### 1. 启动后端服务
```bash
cd /Users/byzantium/github/allcall/backend
go run ./cmd/server
```

服务器会输出：
```
2025-11-15T14:39:54+08:00 INF http server starting addr=0.0.0.0:8080
```

### 2. 构建并运行移动应用
```bash
cd /Users/byzantium/github/allcall/mobile
npx expo run:android
```

### 3. 在应用中发起通话

**步骤：**
1. 用两个不同账号登录应用（A 用户和 B 用户）
2. A 用户在联系人列表中找到 B 用户，点击"呼叫"按钮
3. 系统请求麦克风权限 → **用户同意**
4. 应用开始建立 WebRTC 连接：
   - 获取本地媒体流（音频）
   - 创建 PeerConnection
   - 生成 SDP Offer
   - 通过信令服务器发送邀请
5. B 用户收到来电提示
6. B 用户点击"接听"
7. 系统建立音频连接，双向通话开始

## 关键日志输出

### 成功的通话建立流程：

```
[ensureAudioPermission] Requesting permissions: ["android.permission.RECORD_AUDIO", ...]
[ensureAudioPermission] Permission result: {...}
[ensureAudioPermission] All permissions granted: true

[startCall] Requesting media stream...
[startCall] webrtcMediaDevices: available
[startCall] Requesting getUserMedia with audio only...
[startCall] Media stream obtained: 1 tracks
[startCall] Track obtained - Kind: audio Enabled: true

[startCall] Creating peer connection...
[startCall] Adding track: audio
[startCall] Creating offer...
[startCall] Offer created, SDP length: 1234
[startCall] Setting local description...
[startCall] Local description set
[startCall] Sending call.invite message...
[sendMessage] Message sent successfully

[SignalingContext] Received message: call.invite.ack
[SignalingContext] Session created successfully with callId: xxx
```

### 故障排查：

| 错误信息 | 原因 | 解决方案 |
|---------|------|---------|
| `WebRTC mediaDevices not available` | 未使用原生构建 | 使用 `npx expo run:android` |
| `getUserMedia failed` | 麦克风权限被拒 | 在系统设置中授予权限 |
| `Failed to get media stream` | 麦克风被占用 | 关闭其他占用麦克风的应用 |
| `setRemoteDescription failed` | SDP 解析错误 | 检查后端信令服务 |

## 性能优化

### ICE Servers（已配置）
- 使用 Google STUN 服务器集群
- 支持自定义 TURN 服务器（可选）

### 媒体配置
- 仅音频流（无视频）→ 降低带宽占用
- 16kHz 采样率 → 优化通话质量

## 故障排除

### 如果通话无法建立

1. **检查网络连接**
   ```bash
   adb shell ping 8.8.8.8
   ```

2. **检查后端是否运行**
   ```bash
   curl http://localhost:8080/healthz
   ```

3. **查看详细日志**
   - 使用 Android Studio Logcat
   - 或 `adb logcat | grep -i webrtc`

4. **重启应用**
   ```bash
   adb uninstall com.allcallall.mobile
   npx expo run:android
   ```

## 下一步

- [ ] 在 Android 真机上测试
- [ ] 在 iOS 真机上测试
- [ ] 添加视频通话功能
- [ ] 部署到生产环境

---

**最后更新：2025-11-15**
