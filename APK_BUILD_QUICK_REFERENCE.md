# APK构建和Docker启动快速参考

## 🎯 您的网络信息
```
本机IP地址: 10.136.17.108
后端服务地址: http://10.136.17.108:8080
WebSocket地址: ws://10.136.17.108:8080
```

---

## 第一步：启动完整的Docker Compose环境

### 使用启动脚本（最简单）
```bash
# 设置必需的环境变量
export JWT_SECRET="your-secret-key-min-32-chars"
export MAIL_PASSWORD="your-qq-email-auth-code"

# 运行启动脚本
cd /Users/byzantium/github/allcallall/infra
bash start-production.sh
```

### 或手动启动
```bash
cd /Users/byzantium/github/allcallall/infra

# 设置所有环境变量
export MYSQL_ROOT_PASSWORD="rootpass"
export MYSQL_PASSWORD="allcallallpass"
export REDIS_PASSWORD="redis_secure_password"
export JWT_SECRET="your-secret-key"
export MAIL_PASSWORD="your-qq-auth-code"
export WEBRTC_ICE_SERVERS_JSON='[{"urls":["stun:stun.l.google.com:19302"]}]'

# 启动服务
docker-compose -f docker-compose.production.yml up -d

# 验证
docker-compose -f docker-compose.production.yml ps
```

### 验证服务
```bash
# 测试后端API
curl http://localhost:8080/ping
# 响应: {"message":"pong"}

# 查看日志
docker-compose -f docker-compose.production.yml logs -f backend
```

---

## 第二步：配置移动端API地址

编辑 `mobile/src/config/index.ts`：

```typescript
// 确保development环境使用您的IP
const getApiConfig = () => {
  switch (APP_ENV) {
    case 'development':
      return {
        HTTP: "http://10.136.17.108:8080",
        WS: "ws://10.136.17.108:8080"
      };
    // ... 其他配置
  }
};
```

---

## 第三步：构建APK

### 选项A：使用Gradle本地构建（推荐用于测试）

```bash
cd /Users/byzantium/github/allcallall/mobile

# 1. 安装依赖
npm install

# 2. 清除缓存（可选）
npm run clean

# 3. 进入Android目录
cd android

# 4. 构建APK
# Debug版本（用于开发测试）
./gradlew assembleDebug

# Release版本（用于生产发布）
./gradlew assembleRelease

# APK位置:
# Debug: app/build/outputs/apk/debug/app-debug.apk
# Release: app/build/outputs/apk/release/app-release.apk
```

### 选项B：使用EAS CLI构建

```bash
cd /Users/byzantium/github/allcallall/mobile

# 1. 安装EAS CLI
npm install -g eas-cli

# 2. 登录EAS
eas login

# 3. 构建APK
eas build --platform android --profile=development

# 4. 下载构建完成后的APK
```

### 选项C：快速测试（无需APK，使用Expo Go）

```bash
cd /Users/byzantium/github/allcallall/mobile

# 启动Metro开发服务器
npm run start

# 在Android设备上：
# 1. 安装Expo Go应用（Google Play商店）
# 2. 扫描终端显示的QR码
# 3. 或输入: exp://10.136.17.108:8081
```

---

## 第四步：安装APK到设备

### 前置条件
```bash
# 启用USB调试
# 设置 > 关于手机 > Build Number (连续点击7次)
# 开发者选项 > USB调试

# 验证设备连接
adb devices
# 应该看到您的设备列表
```

### 安装APK
```bash
# 方式1：使用adb直接安装
adb install -r app-debug.apk

# 方式2：使用gradle安装
cd /Users/byzantium/github/allcallall/mobile/android
./gradlew installDebug

# 验证安装
adb shell pm list packages | grep allcallall
```

### 启动应用
```bash
# 方式1：从设备启动
adb shell am start -n com.allcallall.mobile/.MainActivity

# 方式2：在设备上手动点击应用图标

# 查看日志
adb logcat | grep -i allcallall
```

---

## 完整的一键启动流程

### 第1个终端：启动Docker环境
```bash
export JWT_SECRET="your-secret-key"
export MAIL_PASSWORD="your-qq-auth-code"
cd /Users/byzantium/github/allcallall/infra
bash start-production.sh
```

### 第2个终端：构建APK
```bash
cd /Users/byzantium/github/allcallall/mobile/android
./gradlew assembleDebug

# APK位置: app/build/outputs/apk/debug/app-debug.apk
```

### 第3个终端：安装到设备
```bash
adb install -r /Users/byzantium/github/allcallall/mobile/android/app/build/outputs/apk/debug/app-debug.apk
adb shell am start -n com.allcallall.mobile/.MainActivity
```

---

## 🔧 常见命令速查

### Docker Compose命令
```bash
# 查看所有服务状态
docker-compose -f docker-compose.production.yml ps

# 查看日志
docker-compose -f docker-compose.production.yml logs -f

# 停止服务
docker-compose -f docker-compose.production.yml stop

# 重启服务
docker-compose -f docker-compose.production.yml restart

# 删除所有（清理）
docker-compose -f docker-compose.production.yml down -v
```

### ADB命令
```bash
# 查看连接的设备
adb devices

# 查看日志
adb logcat

# 特定应用的日志
adb logcat | grep allcallall

# 清空日志
adb logcat -c

# 安装APK
adb install -r app.apk

# 卸载应用
adb uninstall com.allcallall.mobile

# 启动应用
adb shell am start -n com.allcallall.mobile/.MainActivity

# 停止应用
adb shell am force-stop com.allcallall.mobile

# 测试网络连接
adb shell ping 10.136.17.108

# 测试HTTP请求
adb shell curl http://10.136.17.108:8080/ping
```

### Gradle命令
```bash
# 构建Debug APK
./gradlew assembleDebug

# 构建Release APK
./gradlew assembleRelease

# 清除构建缓存
./gradlew clean

# 安装Debug APK到连接的设备
./gradlew installDebug

# 卸载Debug APK
./gradlew uninstallDebug

# 查看详细错误日志
./gradlew assembleDebug --stacktrace
```

---

## ⚠️ 故障排除

### Docker启动失败
```bash
# 检查Docker服务
docker info

# 检查详细日志
docker-compose -f docker-compose.production.yml logs

# 清除并重新启动
docker-compose -f docker-compose.production.yml down -v
docker-compose -f docker-compose.production.yml up -d
```

### APK构建失败
```bash
# 清除缓存
cd mobile/android
./gradlew clean

# 重新构建（显示详细日志）
./gradlew assembleDebug --stacktrace

# 检查Java版本（需要Java 8+）
java -version
```

### 设备无法连接
```bash
# 重启adb服务
adb kill-server
adb start-server

# 测试网络连接
adb shell ping 10.136.17.108

# 检查防火墙设置（macOS）
# System Preferences > Security & Privacy > Firewall
```

### 应用无法连接后端
```bash
# 验证后端运行
curl http://localhost:8080/ping

# 检查API地址配置（应为10.136.17.108）
# 编辑 mobile/src/config/index.ts

# 从设备测试
adb shell curl http://10.136.17.108:8080/ping
```

---

## 📋 检查清单

- [ ] Docker已安装并运行
- [ ] Docker Compose服务已启动（MySQL、Redis、Backend、Nginx）
- [ ] 后端API可访问 (http://localhost:8080/ping)
- [ ] 移动端API地址已配置为 `10.136.17.108:8080`
- [ ] Java 8+ 已安装
- [ ] Node.js 18+ 已安装
- [ ] Android SDK 已配置
- [ ] USB调试已启用
- [ ] 设备已连接（adb devices显示）
- [ ] APK已构建成功
- [ ] APK已安装到设备
- [ ] 应用可以启动
- [ ] 应用可以连接到后端服务

---

## 📚 详细文档

- [Docker生产环境完整指南](./docs/PRODUCTION_SETUP_AND_APK_BUILD.md)
- [Docker快速启动指南](./docs/DOCKER_STARTUP_GUIDE.md)
- [API文档](./docs/API_DOCUMENTATION.md)
- [数据库文档](./docs/DATABASE.md)

---

**环境信息**
- IP地址: 10.136.17.108
- Java版本: 17.0.15
- 操作系统: macOS
- 应用包名: com.allcallall.mobile

**更新时间**: 2025-12-14
