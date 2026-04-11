# 生产环境Docker Compose启动和APK构建完整指南

## 🎯 当前环境信息

- **本机局域网IP**: `10.136.17.108`
- **后端API地址**: `http://10.136.17.108:8080`
- **Java版本**: OpenJDK 17.0.15
- **移动端应用ID**: `com.allcallall.mobile`
- **Expo SDK版本**: 51.0.0

---

## 第一部分：使用docker-compose.production.yml启动完整项目环境

### 1. 环境变量准备

生产环境的docker-compose.production.yml需要以下环境变量：

```bash
# 创建.env文件（或在启动时设置环境变量）
export MYSQL_ROOT_PASSWORD="rootpass"
export MYSQL_PASSWORD="allcallallpass"
export REDIS_PASSWORD="redis_secure_password"
export JWT_SECRET="your-secret-key-min-32-chars-required"
export MAIL_PASSWORD="your-qq-email-auth-code"
export WEBRTC_ICE_SERVERS_JSON='[{"urls":["stun:stun.l.google.com:19302"]}]'
```

### 2. 使用Docker Compose启动所有服务

#### 方式A：直接启动（推荐）

```bash
# 1. 进入infra目录
cd /Users/byzantium/github/allcallall/infra

# 2. 设置环境变量
export MYSQL_ROOT_PASSWORD="rootpass"
export MYSQL_PASSWORD="allcallallpass"
export REDIS_PASSWORD="redis_secure_password"
export JWT_SECRET="your-secret-key-min-32-chars-required"
export MAIL_PASSWORD="your-qq-email-auth-code"
export WEBRTC_ICE_SERVERS_JSON='[{"urls":["stun:stun.l.google.com:19302"]}]'

# 3. 启动所有服务（MySQL + Redis + Backend + Nginx）
docker-compose -f docker-compose.production.yml up -d

# 4. 查看服务状态
docker-compose -f docker-compose.production.yml ps
```

#### 方式B：使用.env文件（更清洁）

```bash
# 1. 在infra目录创建.env.production文件
cat > /Users/byzantium/github/allcallall/infra/.env.production << 'EOF'
MYSQL_ROOT_PASSWORD=rootpass
MYSQL_PASSWORD=allcallallpass
REDIS_PASSWORD=redis_secure_password
JWT_SECRET=your-secret-key-min-32-chars-required
MAIL_PASSWORD=your-qq-email-auth-code
WEBRTC_ICE_SERVERS_JSON=[{"urls":["stun:stun.l.google.com:19302"]}]
EOF

# 2. 启动服务
cd /Users/byzantium/github/allcallall/infra
docker-compose -f docker-compose.production.yml --env-file .env.production up -d

# 3. 查看服务状态
docker-compose -f docker-compose.production.yml ps
```

### 3. 验证所有服务是否正常运行

```bash
# 检查MySQL
docker-compose -f docker-compose.production.yml exec mysql mysql -uallcallall -pallcallallpass -e "SELECT 1;"

# 检查Redis
docker-compose -f docker-compose.production.yml exec redis redis-cli -a redis_secure_password ping

# 检查后端服务
curl http://localhost:8080/api/v1/health

# 查看后端日志
docker-compose -f docker-compose.production.yml logs -f backend

# 查看所有容器日志
docker-compose -f docker-compose.production.yml logs -f
```

### 4. Docker Compose服务架构说明

**docker-compose.production.yml** 包含以下服务：

| 服务 | 镜像 | 端口 | 说明 |
|-----|------|------|------|
| MySQL | mysql:8.0 | 3306 | 数据库服务 |
| Redis | redis:7.2 | 6379 | 缓存服务（带密码） |
| Backend | 自定义Dockerfile | 8080 | Go后端服务 |
| Nginx | nginx:latest | 80/443 | 反向代理和静态文件服务 |

**网络**: 所有服务通过 `allcallall_network` 桥接网络相互通信

### 5. 常用命令

```bash
# 停止所有服务（保留数据）
docker-compose -f docker-compose.production.yml stop

# 停止并删除容器（保留数据卷）
docker-compose -f docker-compose.production.yml down

# 重启所有服务
docker-compose -f docker-compose.production.yml restart

# 重启特定服务
docker-compose -f docker-compose.production.yml restart backend

# 查看服务日志
docker-compose -f docker-compose.production.yml logs backend

# 进入容器执行命令
docker-compose -f docker-compose.production.yml exec backend /bin/sh

# 删除所有数据（包括数据卷）
docker-compose -f docker-compose.production.yml down -v
```

---

## 第二部分：为安卓设备构建APK文件

### 前置条件检查

```bash
# 1. 检查Java版本（需要Java 8以上）
java -version
# 输出示例: openjdk version "17.0.15"

# 2. 检查Node.js版本（需要Node.js 18+）
node --version
# 输出示例: v18.0.0 或更高

# 3. 检查npm版本
npm --version
# 输出示例: 8.0.0 或更高
```

### 第一步：检查和更新Android SDK配置

```bash
# 1. 检查ANDROID_SDK_ROOT是否配置
echo $ANDROID_SDK_ROOT

# 2. 如果未配置，需要设置（macOS示例）
# 假设Android SDK位于 ~/Library/Android/sdk
export ANDROID_SDK_ROOT=$HOME/Library/Android/sdk
export PATH=$ANDROID_SDK_ROOT/platform-tools:$PATH

# 3. 将其添加到 ~/.zshrc（永久配置）
cat >> ~/.zshrc << 'EOF'
export ANDROID_SDK_ROOT=$HOME/Library/Android/sdk
export PATH=$ANDROID_SDK_ROOT/platform-tools:$PATH
EOF

source ~/.zshrc
```

> ⚠️ **如果您未安装Android SDK**，需要先安装。由于您在记忆中已禁止安装额外软件，请确保Android SDK已在您的系统中可用。

### 第二步：准备移动端代码

```bash
# 1. 进入移动端目录
cd /Users/byzantium/github/allcallall/mobile

# 2. 安装依赖（如果尚未安装）
npm install

# 3. 清除缓存（可选，用于清除之前的构建缓存）
npm run clean
# 或
rm -rf node_modules/.cache
```

### 第三步：配置API连接地址

在启动 Metro 或构建前设置移动端环境变量，确保开发环境指向当前局域网 IP：

```bash
EXPO_PUBLIC_API_HTTP=http://10.136.17.108:8080 \
EXPO_PUBLIC_API_WS=ws://10.136.17.108:8080 \
npm run start
```

### 第四步：构建APK文件

#### 方式A：使用Expo CLI构建（推荐用于开发/测试）

```bash
# 1. 安装EAS CLI（如果未安装）
npm install -g eas-cli

# 2. 进入移动端目录
cd /Users/byzantium/github/allcallall/mobile

# 3. 构建APK（development版本）
eas build --platform android --profile=development

# 或构建release版本
eas build --platform android --profile=release

# 4. 构建完成后，EAS会提供下载链接
# 下载APK文件到本地设备
```

#### 方式B：使用Gradle构建（本地构建）

```bash
# 1. 进入Android目录
cd /Users/byzantium/github/allcallall/mobile/android

# 2. 构建APK
# Debug版本（推荐用于测试）
./gradlew assembleDebug

# Release版本
./gradlew assembleRelease

# 3. APK文件位置
# Debug: app/build/outputs/apk/debug/app-debug.apk
# Release: app/build/outputs/apk/release/app-release.apk
```

#### 方式C：使用Expo Go快速测试（无需APK）

如果您只是想快速测试，不需要构建APK，可以直接使用Expo Go：

```bash
# 1. 在移动端目录启动Metro开发服务器
cd /Users/byzantium/github/allcallall/mobile
npm run start

# 2. 在Android真机安装Expo Go应用
# 从Google Play商店下载：https://play.google.com/store/apps/details?id=host.exp.exponent

# 3. 扫描显示的QR码或输入URL：exp://10.136.17.108:8081

# 4. 应用会加载并连接到本地后端服务
```

### 第五步：安装APK到安卓设备

```bash
# 1. 启用USB调试
# 设置 > 关于手机 > 连续点击"Build Number"以启用开发者选项
# 开发者选项 > 启用USB调试

# 2. 连接设备到开发机
adb devices
# 应该看到您的设备列表

# 3. 安装APK
adb install -r app/build/outputs/apk/debug/app-debug.apk

# 或使用gradle直接安装
cd /Users/byzantium/github/allcallall/mobile/android
./gradlew installDebug

# 4. 验证安装
adb shell pm list packages | grep allcallall
```

### 第六步：在设备上运行和调试

```bash
# 1. 启动应用
adb shell am start -n com.allcallall.mobile/.MainActivity

# 2. 查看应用日志
adb logcat | grep -i allcallall

# 3. 获取完整的logcat日志
adb logcat > logcat.log

# 4. 设备连接问题排查
# 检查设备是否连接
adb devices -l

# 重启adb服务
adb kill-server
adb start-server

# 检查网络连接
adb shell ping 10.136.17.108
```

---

## 第三部分：配置移动端连接到本地后端

### 1. 获取本机局域网IP

```bash
# 查询IP地址
ifconfig | grep "inet " | grep -v 127.0.0.1

# 输出示例:
# inet 10.136.17.108 netmask 0xffffff00 broadcast 10.136.17.255
```

**您的IP地址**: `10.136.17.108`

### 2. 配置移动端API地址

在启动 Metro 或构建前通过 `EXPO_PUBLIC_*` 指定接口地址：

```bash
EXPO_PUBLIC_API_HTTP=http://10.136.17.108:8080 \
EXPO_PUBLIC_API_WS=ws://10.136.17.108:8080 \
npm run start
```

生产构建示例：

```bash
EXPO_PUBLIC_API_HTTP=https://api.example.com \
EXPO_PUBLIC_API_WS=wss://api.example.com \
EXPO_PUBLIC_FORCE_TLS=1 \
eas build --platform android --profile=release
```

### 3. 验证连接

```bash
# 从Android设备ping开发机
adb shell ping 10.136.17.108

# 从Android设备测试HTTP连接
adb shell curl http://10.136.17.108:8080/ping

# 检查防火墙设置（确保8080端口未被阻止）
# macOS: System Preferences > Security & Privacy > Firewall
```

### 4. 环境变量配置

如果需要根据环境变量切换API地址：

```bash
# 启动开发服务器时指定环境
EXPO_PUBLIC_API_HTTP=http://10.136.17.108:8080 \
EXPO_PUBLIC_API_WS=ws://10.136.17.108:8080 \
npm run start

# 或在 eas.json / 本地 shell 中提供相同的 EXPO_PUBLIC_* 配置
```

---

## 完整启动流程（一步步指南）

### 步骤1：启动Docker Compose环境

```bash
cd /Users/byzantium/github/allcallall/infra

# 设置环境变量
export MYSQL_ROOT_PASSWORD="rootpass"
export MYSQL_PASSWORD="allcallallpass"
export REDIS_PASSWORD="redis_secure_password"
export JWT_SECRET="your-secret-key"
export MAIL_PASSWORD="your-qq-auth-code"
export WEBRTC_ICE_SERVERS_JSON='[{"urls":["stun:stun.l.google.com:19302"]}]'

# 启动所有服务
docker-compose -f docker-compose.production.yml up -d

# 等待所有服务就绪（约30秒）
sleep 30

# 验证服务
docker-compose -f docker-compose.production.yml ps
```

### 步骤2：验证后端服务

```bash
# 测试API健康检查
curl http://localhost:8080/api/v1/health

# 或简单的ping
curl http://localhost:8080/ping
```

### 步骤3：构建APK（选项A - Expo CLI）

```bash
cd /Users/byzantium/github/allcallall/mobile

# 安装EAS CLI
npm install -g eas-cli

# 登录EAS账户
eas login

# 构建APK
eas build --platform android --profile=development

# 等待构建完成并下载APK
```

### 步骤4：构建APK（选项B - Gradle本地构建）

```bash
cd /Users/byzantium/github/allcallall/mobile/android

# 构建Debug APK
./gradlew assembleDebug

# APK位置: app/build/outputs/apk/debug/app-debug.apk
```

### 步骤5：安装APK到设备

```bash
# 确保设备已连接并启用USB调试
adb devices

# 安装APK
adb install -r /Users/byzantium/github/allcallall/mobile/android/app/build/outputs/apk/debug/app-debug.apk

# 验证安装
adb shell pm list packages | grep allcallall
```

### 步骤6：启动应用测试

```bash
# 从Android设备启动应用
adb shell am start -n com.allcallall.mobile/.MainActivity

# 查看日志
adb logcat | grep -i allcallall
```

---

## 🔧 常见问题解决

### 问题1：Docker容器启动失败

```bash
# 查看详细错误日志
docker-compose -f docker-compose.production.yml logs

# 清除所有容器和数据后重新启动
docker-compose -f docker-compose.production.yml down -v
docker-compose -f docker-compose.production.yml up -d
```

### 问题2：后端服务无法启动

```bash
# 检查环境变量是否正确设置
docker-compose -f docker-compose.production.yml exec backend env | grep -E "DB_DSN|REDIS"

# 检查MySQL是否就绪
docker-compose -f docker-compose.production.yml logs mysql

# 手动测试数据库连接
docker-compose -f docker-compose.production.yml exec mysql mysql -uallcallall -pallcallallpass allcallall_db -e "SELECT 1;"
```

### 问题3：Redis连接失败

```bash
# 检查Redis是否运行
docker-compose -f docker-compose.production.yml exec redis redis-cli -a redis_secure_password ping

# 重启Redis服务
docker-compose -f docker-compose.production.yml restart redis
```

### 问题4：APK构建失败

```bash
# 清除gradle缓存
cd /Users/byzantium/github/allcallall/mobile/android
./gradlew clean

# 重新构建
./gradlew assembleDebug

# 查看详细错误
./gradlew assembleDebug --stacktrace
```

### 问题5：安卓设备无法连接后端

```bash
# 验证网络连通性
adb shell ping 10.136.17.108

# 检查防火墙
# macOS: System Preferences > Security & Privacy > Firewall Options

# 验证API地址配置
# 检查 mobile/src/config/index.ts 中的IP地址

# 从设备测试HTTP请求
adb shell curl -v http://10.136.17.108:8080/ping
```

### 问题6：APK安装失败

```bash
# 卸载旧版本
adb uninstall com.allcallall.mobile

# 重新安装
adb install -r app-debug.apk

# 检查设备空间
adb shell df
```

---

## 📋 检查清单

### Docker Compose启动
- [ ] 环境变量已设置
- [ ] 所有四个服务正在运行（MySQL、Redis、Backend、Nginx）
- [ ] 后端服务健康检查通过
- [ ] 可以访问 http://localhost:8080/ping

### APK构建
- [ ] Java 8+ 已安装
- [ ] Node.js 18+ 已安装
- [ ] Android SDK 已配置
- [ ] APK构建完成
- [ ] APK文件大小正常（通常10-50MB）

### 设备准备
- [ ] USB调试已启用
- [ ] 设备已连接到开发机
- [ ] adb devices 显示设备
- [ ] 设备可以ping开发机IP

### 应用运行
- [ ] APK已安装到设备
- [ ] 应用可以启动
- [ ] 应用连接到本地后端
- [ ] 应用可以正常使用各项功能

---

## 📝 环境变量说明

| 变量名 | 说明 | 示例 |
|------|------|------|
| MYSQL_ROOT_PASSWORD | MySQL root用户密码 | rootpass |
| MYSQL_PASSWORD | MySQL应用用户密码 | allcallallpass |
| REDIS_PASSWORD | Redis访问密码 | redis_secure_password |
| JWT_SECRET | JWT签署密钥（最少32字符） | your-secret-key-min-32-chars |
| MAIL_PASSWORD | QQ邮箱授权码 | 16位授权码 |
| WEBRTC_ICE_SERVERS_JSON | ICE服务器配置 | [{"urls":["stun:stun.l.google.com:19302"]}] |

---

## 🔗 相关文档

- [Docker Compose快速启动](../getting-started/docker-startup-guide.md)
- [API文档](../api/api-documentation.md)
- [数据库文档](../api/database.md)

---

## 📋 APK 构建快速参考

### 环境信息
- **应用包名**: `com.allcallall.mobile`
- **Java版本**: OpenJDK 17+

### 使用 Gradle 本地构建（推荐用于测试）

```bash
cd /Users/byzantium/github/allcallall/mobile

# 1. 安装依赖
npm install

# 2. 进入 Android 目录
cd android

# 3. 构建 APK
# Debug版本
./gradlew assembleDebug

# Release版本
./gradlew assembleRelease

# APK位置:
# Debug: app/build/outputs/apk/debug/app-debug.apk
# Release: app/build/outputs/apk/release/app-release.apk
```

### 使用 EAS CLI 构建

```bash
cd /Users/byzantium/github/allcallall/mobile

# 安装 EAS CLI
npm install -g eas-cli

# 登录 EAS
eas login

# 构建 APK
eas build --platform android --profile=development
```

### 安装 APK 到设备

```bash
# 启用 USB 调试
# 设置 > 关于手机 > Build Number (连续点击7次)
# 开发者选项 > USB调试

# 验证设备连接
adb devices

# 安装 APK
adb install -r app-debug.apk

# 或使用 Gradle 安装
cd android && ./gradlew installDebug

# 验证安装
adb shell pm list packages | grep allcallall
```

### 启动应用

```bash
# 启动应用
adb shell am start -n com.allcallall.mobile/.MainActivity

# 查看日志
adb logcat | grep -i allcallall
```

### Gradle 常用命令

```bash
# 构建 Debug APK
./gradlew assembleDebug

# 构建 Release APK
./gradlew assembleRelease

# 清除构建缓存
./gradlew clean

# 安装到连接的设备
./gradlew installDebug

# 查看详细错误日志
./gradlew assembleDebug --stacktrace
```

---

**最后更新**: 2025-01-25
**环境**: macOS + Docker + Android开发环境
