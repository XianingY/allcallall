# AllCallAll

[English](#english) | [中文](#中文)

---

## 中文

> 一个基于 WebRTC 的实时音视频通信平台，支持点对点语音通话、联系人管理和在线状态同步。

### ✨ 特性

- 🎤 **实时音视频通话** - 基于 Pion WebRTC 的点对点音频通话
- 🌍 **实时翻译** - 离线本地 AI 模型，语音识别 + 文本翻译 + 语音合成 (Whisper + Opus-MT + VITS)
- 👥 **联系人管理** - 添加、搜索和管理通讯录
- 🟢 **在线状态** - 实时显示用户在线状态和最后在线时间
- 🔐 **用户认证** - JWT 令牌认证和会话管理
- 📱 **跨平台** - Android 原生应用支持，iOS 开发中
- 🚀 **高性能** - Redis 缓存、连接池优化、异步 WebSocket 信令
- 🔄 **自动重连** - 网络异常自动重新连接
- 🔒 **隐私保护** - 完全离线翻译，数据不上传云端

### 🛠 技术栈

#### 后端
- **语言**: Go 1.22+
- **框架**: Gin（HTTP）、Gorilla WebSocket
- **数据库**: MySQL 8.0
- **缓存**: Redis 7.2
- **WebRTC**: Pion v4.0.0
- **认证**: JWT (golang-jwt)
- **邮件**: SMTP (QQ邮箱 smtp.qq.com:587)

#### 移动端
- **框架**: React Native 0.74+
- **开发**: Expo 51.0+（Expo Development Client）
- **语言**: TypeScript
- **UI**: React Navigation
- **WebRTC**: react-native-webrtc 124.0.0
- **HTTP**: Axios
- **状态管理**: React Context API
- **离线翻译**: Whisper (ASR) + Opus-MT (翻译) + VITS (TTS)
- **模型量化**: INT8 (70% 大小减少)
- **原生集成**: Android JNI + C++

#### 基础设施
- **容器化**: Docker & Docker Compose
- **构建**: Metro Bundler、Expo CLI
- **调试**: ADB (Android Debug Bridge)
- **服务代理**: Cloudflare Tunnel（可选）

### 🚀 快速开始

### 前置要求

- **开发机**: macOS / Linux
- **Node.js**: 18.0 或更新
- **Go**: 1.22 或更新
- **Docker**: 20.10+（可选，用于数据库）
- **Android SDK**: API 级别 31+ （真机调试）
- **ADB**: Android Debug Bridge

### 安装依赖

```bash
# 克隆项目
git clone git@github.com:1victorloui1/AllCallAll.git
cd AllCallAll

# 安装后端依赖
cd backend
go mod download
cd ..

# 安装移动端依赖
cd mobile
npm install
cd ..
```

### 启动数据库服务

```bash
# 启动 MySQL 和 Redis
./scripts/development/start-services.sh

# 验证服务状态
docker-compose -f infra/docker-compose.yml ps
```

### 启动后端服务

#### 前置步骤：配置邮件服务（QQ邮箱）

```bash
cd backend

# 1. 复制环境变量示例文件
cp .env.example .env

# 2. 编辑 .env，填入 QQ 邮箱授权码
# MAIL_PASSWORD=xxxx xxxx xxxx xxxx  (登录你自己的实际授权码)

# 3. 验证后端配置文件中的邮件设置
cat configs/config.yaml | grep -A5 mail:
# 应该显示：
#   host: smtp.qq.com
#   port: 587
#   username: xxxx@qq.com
```

#### 启动后端服务

```bash
cd backend

# 设置配置文件路径
export CONFIG_PATH=./configs/config.yaml

# 运行后端服务（监听 0.0.0.0:8080）
go run cmd/server/main.go

# 验证后端是否运行
curl http://localhost:8080/health
```

### WebRTC/TURN 配置（真机语音必需）

1. 在服务器上准备 TURN（示例，端口 3478）：  
   ```bash
   docker run -d --network host --name coturn instrumentisto/coturn \
     -a -f -v -n --log-file=stdout \
     --realm=allcallall --user=allcallall:strongpassword \
     --external-ip=$(curl -s ifconfig.me) \
     --min-port=49152 --max-port=49200
   ```
2. 把 TURN/STUN 列表下发给后端（移动端会自动拉取）：  
   ```bash
export WEBRTC_ICE_SERVERS_JSON='[
  {"urls":["stun:stun.l.google.com:19302"]},
  {"urls":["turn:81.68.168.207:3478?transport=udp","turn:81.68.168.207:3478?transport=tcp"],"username":"allcallall","credential":"strongpassword"}
]'
```
3. 重启后端。APK 不需要重新打包，登陆后客户端会自动从 `/api/v1/webrtc/config` 读取最新 ICE/TURN 配置。

### 🌐 生产环境（当前部署：81.68.168.207）

- API: `http://81.68.168.207/api/v1`  
- WS: `ws://81.68.168.207/api/v1/ws`

#### 云服务器运维

```bash
# 进入部署目录
cd ~/workspace/AllCallAll-deploy-server/infra

# 首次或重新构建启动（确保 .env 填好）
docker compose -f docker-compose.production.yml up -d --build

# 常规重启后端/nginx
docker compose -f docker-compose.production.yml up -d

# 关闭所有服务
docker compose -f docker-compose.production.yml down
```

#### TURN 服务 (coturn)

```bash
# 首次启动 TURN（监听 3478，端口范围 49152-49200）
docker run -d --network host --name coturn instrumentisto/coturn \
  -a -f -v -n --log-file=stdout \
  --realm=allcallall --user=allcallall:strongpassword \
  --external-ip=$(curl -s ifconfig.me) \
  --min-port=49152 --max-port=49200

# 之后启动/停止
docker start coturn
docker stop coturn
```

#### 数据清理（删除所有账号数据）

```bash
docker exec -it infra-mysql-1 mysql -uroot -p"$MYSQL_ROOT_PASSWORD" allcallall_db \
  -e "SET FOREIGN_KEY_CHECKS=0;
      TRUNCATE TABLE contacts;
      TRUNCATE TABLE email_verification_codes;
      TRUNCATE TABLE email_send_logs;
      TRUNCATE TABLE users;
      SET FOREIGN_KEY_CHECKS=1;"
```

### 📦 移动端 Release APK 构建（本地）

```bash
cd mobile
npm install
cd android
./gradlew assembleRelease
cp app/build/outputs/apk/release/app-release.apk ../AllCallAll.apk
# 可选：安装到真机
adb install -r ../AllCallAll.apk
```

### 启动移动应用

#### 推荐方式：Expo Development Client + ADB 反向转发（最稳定）

```bash
cd mobile

# 使用自动化脚本启动（推荐）
bash scripts/dev-client-debug.sh

# 或手动步骤：
# 1. 配置 ADB 反向转发
adb reverse tcp:8080 tcp:8080
adb reverse tcp:8081 tcp:8081

# 2. 启动 Metro 开发服务器
npm run start:dev-client

# 3. 在真机上扫描二维码或输入 Metro 显示的 URL
```

#### 方式 2: Wi-Fi 无线调试（LAN 模式 - 可选）

```bash
cd mobile

# 启动 Metro 服务器（LAN 模式）
npm run start:dev-client:lan

# 或使用传统 Expo Go
npm run start:lan

# 在真机摇一摇菜单中选择 'Change Bundle URL'，输入显示的 LAN 地址
```

#### 方式 3: 构建自定义开发客户端 APK

```bash
cd mobile

# 首次或需要更新客户端时运行
npm run android

# 这会构建并安装 Expo Development Client
```

### 📁 目录结构

```
allcall/
├── backend/                    # Go 后端服务
│   ├── cmd/
│   │   └── server/             # 应用入口点
│   ├── internal/
│   │   ├── auth/               # 认证和 JWT
│   │   ├── user/               # 用户管理
│   │   ├── contact/            # 联系人管理
│   │   ├── signaling/          # WebRTC 信令
│   │   ├── media/              # Pion WebRTC 媒体引擎
│   │   ├── presence/           # 在线状态管理
│   │   ├── models/             # 数据模型
│   │   ├── handlers/           # HTTP 处理器
│   │   ├── database/           # 数据库连接
│   │   └── cache/              # Redis 缓存
│   ├── configs/                # 配置文件
│   └── Dockerfile              # Docker 镜像
│
├── mobile/                     # React Native 移动应用
│   ├── src/
│   │   ├── screens/            # 应用页面
│   │   ├── components/         # UI 组件
│   │   ├── context/            # 状态管理（Auth、Signaling）
│   │   ├── navigation/         # 路由配置
│   │   ├── config/             # 应用配置
│   │   └── utils/              # 工具函数
│   ├── android/                # Android 原生代码
│   ├── metro.config.js         # Metro 打包器配置
│   ├── app.json                # Expo 配置
│   └── package.json
│
├── infra/                      # 基础设施配置
│   ├── docker-compose.yml      # 本地开发环境
│   ├── docker-compose.production.yml  # 生产环境
│   ├── cloudflared-config.yml  # Cloudflare Tunnel 配置
│   └── deploy.sh               # 云服务器部署脚本
│
└── start.sh                    # 快速启动脚本
```

### 🔧 开发调试

### Metro 开发服务器

Metro 会自动检测本机 LAN IP 并动态绑定。查看启动日志获取 URL：

```bash
npm run start

# 输出示例：
# 📱 Metro开发服务器配置：
#    LAN IP: 192.168.1.36
#    Metro URL: http://192.168.1.36:8081
#    API URL: http://192.168.1.36:8080
#    ✅ 支持USB连接和Wi-Fi连接两种模式
```

### 网络配置

当前配置使用 **ADB 反向转发方案**（推荐）：

```bash
# 自动配置（使用脚本）
bash scripts/dev-client-debug.sh

# 手动配置
adb reverse tcp:8080 tcp:8080  # 后端 API 服务
adb reverse tcp:8081 tcp:8081  # Metro 开发服务器
```

前端应用配置（自动使用 localhost）：

```typescript
// src/config/index.ts
const API_HOST = "http://localhost:8080";  // 通过 ADB 转发
const WS_HOST = "ws://localhost:8080";      // WebSocket 也通过转发
```

**为什么使用 ADB 反向转发？**
- ✅ 比直接使用 LAN IP 更稳定可靠
- ✅ 与本地开发环境一致
- ✅ 支持多设备同时调试
- ✅ 网络更稳定，延迟更低

**可选：LAN 模式（Wi-Fi 调试）**
- 开发机 IP：192.168.31.217
- 使用场景：需要无线自由移动的开发测试

### 常用开发命令

```bash
cd mobile

# 启动 Metro 开发服务器
npm run start

# LAN 模式启动（Wi-Fi 调试）
npm run start:lan

# Tunnel 模式（跨网络）
npm run start:tunnel

# 构建自定义开发客户端
npm run android

# 代码检查
npm run lint
```

### 调试真机应用

```bash
# 查看设备日志
adb logcat

# 清除应用数据并重启
adb shell pm clear com.allcallall.mobile
adb shell am start -n com.allcallall.mobile/.MainActivity

# 配置 ADB 反向端口转发
adb reverse tcp:8080 tcp:8080
adb reverse tcp:8081 tcp:8081
```

### 📡 API 端点

### 认证

```
POST   /api/v1/auth/register     - 用户注册
POST   /api/v1/auth/login        - 用户登录
```

### 用户

```
GET    /api/v1/users/contacts    - 获取联系人列表
GET    /api/v1/users/presence    - 获取用户在线状态
GET    /api/v1/users/search      - 搜索用户
```

### 信令

```
GET    /api/v1/ws                - WebSocket 连接
```

### 🐛 常见问题

### 真机无法连接到开发服务器

**问题**: `AxiosError: Network Error` 或 `Network timeout`

**解决方案**:

1. **检查 ADB 反向转发配置**
   ```bash
   adb reverse --list
   # 应该显示：
   # tcp:8080 tcp:8080
   # tcp:8081 tcp:8081
   
   # 如果缺少，重新配置
   adb reverse tcp:8080 tcp:8080
   adb reverse tcp:8081 tcp:8081
   ```

2. **验证后端服务是否运行**
   ```bash
   curl http://localhost:8080/health
   ```

3. **检查前端配置**
   ```bash
   cat mobile/src/config/index.ts
   # 应该显示 API_HOST = "http://localhost:8080"
   ```

4. **清除应用数据并重新启动**
   ```bash
   adb shell pm clear com.allcallall.mobile
   # 在真机上重新扫描 Metro 二维码
   ```

5. **运行完整启动脚本**
   ```bash
   bash mobile/scripts/dev-client-debug.sh
   ```

### Metro 编译失败或虚拟入口点 404 错误

**问题**: `Unable to resolve module ./.expo/.virtual-metro-entry` 或编译失败

**解决方案**:

1. **使用自动化脚本**（推荐 - 自动处理虚拟入口点）
   ```bash
   bash mobile/scripts/dev-client-debug.sh
   ```

2. **手动清理和重启**
   ```bash
   # 清理缓存
   rm -rf mobile/node_modules/.cache /tmp/metro-*
   
   # 保护虚拟入口点文件（不要删除）
   ls -la mobile/.expo/.virtual-metro-entry.js
   
   # 如果虚拟入口点文件丢失，重建它
   mkdir -p mobile/.expo
   cat > mobile/.expo/.virtual-metro-entry.js << 'EOF'
import { registerRootComponent } from 'expo';
import App from '../App';
registerRootComponent(App);
EOF
   
   # 重新安装依赖和启动
   cd mobile
   npm install
   npm run start:dev-client
   ```

⚠️ **关键提示**: `.virtual-metro-entry.js` 是 Metro 必需的虚拟入口点文件，**绝不能删除**。运行脚本会自动保护它。

### 后端服务无法启动或邮件无法发送

**问题 1**: `failed to connect mysql`

**解决方案**:
```bash
# 确保数据库服务已启动
./scripts/development/start-services.sh

# 检查 MySQL 连接
mysql -u allcallall -p allcallall_db -h localhost

# 验证 Redis 连接
redis-cli ping
```

**问题 2**: 邮件无法发送或验证码无法接收

**解决方案**:
```bash
# 检查 QQ 邮箱 SMTP 配置
cat backend/configs/config.yaml | grep -A5 mail:

# 验证环境变量
echo $MAIL_PASSWORD

# 测试邮件发送端点
curl -X POST http://localhost:8080/api/v1/email/send-verification-code \
  -H "Content-Type: application/json" \
  -d '{"email":"test.user@example.com"}'

# 预期响应: {"message":"verification code sent successfully"}
```

### 📚 开发指南

### 代码风格

- **Go**: 遵循 [Effective Go](https://golang.org/doc/effective_go)
- **TypeScript**: ESLint 配置规范
- **Kotlin**: Android 官方风格指南

### 分支策略

- `main` - 稳定发布版本
- `develop` - 开发分支
- `feature/*` - 功能分支
- `bugfix/*` - 修复分支

### 🤝 贡献指南

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

### 📝 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

### 📧 联系方式

- 问题报告: [GitHub Issues](https://github.com/1victorloui1/AllCallAll/issues)
- 讨论: [GitHub Discussions](https://github.com/1victorloui1/AllCallAll/discussions)

### 🙏 致谢



---

## English

> Real-time audio/video communication platform built with WebRTC and React Native.

### ✨ Features

- 🎤 **Real-time Audio/Video Calls** - Peer-to-peer audio calls based on Pion WebRTC
- 👥 **Contact Management** - Add, search, and manage contacts
- 🟢 **Online Status** - Real-time user presence and last seen information
- 🔐 **User Authentication** - JWT token authentication and session management via email
- 📧 **Email Verification** - Secure user registration with QQ SMTP email verification
- 📱 **Cross-Platform** - Native Android support, iOS in development
- 🚀 **High Performance** - Redis caching, connection pooling, async WebSocket signaling
- 🔄 **Auto Reconnection** - Automatic reconnection on network failure

### 🛠 Technology Stack

#### Backend
- **Language**: Go 1.22+
- **Framework**: Gin (HTTP), Gorilla WebSocket
- **Database**: MySQL 8.0
- **Cache**: Redis 7.2
- **WebRTC**: Pion v4.0.0
- **Authentication**: JWT (golang-jwt)
- **Email**: SMTP (QQ Mail smtp.qq.com:587)

#### Mobile
- **Framework**: React Native 0.74+
- **Development**: Expo 51.0+ (Expo Development Client)
- **Language**: TypeScript
- **UI**: React Navigation
- **WebRTC**: react-native-webrtc 124.0.0
- **HTTP**: Axios
- **State Management**: React Context API

#### Infrastructure
- **Containerization**: Docker & Docker Compose
- **Build**: Metro Bundler, Expo CLI
- **Debug**: ADB (Android Debug Bridge)
- **Service Proxy**: Cloudflare Tunnel (Optional)

### 🚀 Getting Started

#### Prerequisites

- **Development Machine**: macOS / Linux
- **Node.js**: 18.0 or newer
- **Go**: 1.22 or newer
- **Docker**: 20.10+ (optional, for databases)
- **Android SDK**: API level 31+ (physical device debugging)
- **ADB**: Android Debug Bridge

#### Install Dependencies

```bash
# Clone the repository
git clone git@github.com:1victorloui1/AllCallAll.git
cd AllCallAll

# Install backend dependencies
cd backend
go mod download
cd ..

# Install mobile dependencies
cd mobile
npm install
cd ..
```

### 🌐 Production Deployment (current server: 81.68.168.207)

- API: `http://81.68.168.207/api/v1`  
- WS: `ws://81.68.168.207/api/v1/ws`

#### Cloud server ops

```bash
cd ~/workspace/AllCallAll-deploy-server/infra

# First time or rebuild/start (ensure .env is filled)
docker compose -f docker-compose.production.yml up -d --build

# Regular restart
docker compose -f docker-compose.production.yml up -d

# Stop all services
docker compose -f docker-compose.production.yml down
```

#### TURN (coturn)

```bash
# First start (ports: 3478, relay 49152-49200)
docker run -d --network host --name coturn instrumentisto/coturn \
  -a -f -v -n --log-file=stdout \
  --realm=allcallall --user=allcallall:strongpassword \
  --external-ip=$(curl -s ifconfig.me) \
  --min-port=49152 --max-port=49200

# Afterwards
docker start coturn
docker stop coturn
```

#### Data reset (wipe all accounts)

```bash
docker exec -it infra-mysql-1 mysql -uroot -p"$MYSQL_ROOT_PASSWORD" allcallall_db \
  -e "SET FOREIGN_KEY_CHECKS=0;
      TRUNCATE TABLE contacts;
      TRUNCATE TABLE email_verification_codes;
      TRUNCATE TABLE email_send_logs;
      TRUNCATE TABLE users;
      SET FOREIGN_KEY_CHECKS=1;"
```

#### Start Database Services

```bash
# Start MySQL and Redis
./scripts/development/start-services.sh

# Verify service status
docker-compose -f infra/docker-compose.yml ps
```

#### Start Backend Service

**Prerequisite: Configure email service (QQ Mail)**

```bash
cd backend

# 1. Copy environment variable example file
cp .env.example .env

# 2. Edit .env and fill in QQ Mail authorization code
# MAIL_PASSWORD=xxxx xxxx xxxx xxxx  (enter your actual authorization code)

# 3. Verify backend mail config in config.yaml
cat configs/config.yaml | grep -A5 mail:
# Should show:
#   host: smtp.qq.com
#   port: 587
#   username: xxxx@qq.com

# 4. Set configuration file path
export CONFIG_PATH=./configs/config.yaml

# 5. Run backend service (listening on 0.0.0.0:8080)
go run cmd/server/main.go

# 6. Verify backend is running
curl http://localhost:8080/health
```

#### Start Mobile Application

**Recommended: Expo Development Client + ADB Reverse Forwarding (Most Stable)**

```bash
cd mobile

# Using automated script (Recommended)
bash scripts/dev-client-debug.sh

# Or manual steps:
# 1. Configure ADB reverse port forwarding
adb reverse tcp:8080 tcp:8080
adb reverse tcp:8081 tcp:8081

# 2. Start Metro development server
npm run start:dev-client

# 3. Scan QR code on physical device or enter the URL shown by Metro
```

##### Method 2: Wireless Debugging over Wi-Fi (LAN Mode - Optional)

```bash
cd mobile

# Start Metro server (LAN mode)
npm run start:dev-client:lan

# Or use traditional Expo Go
npm run start:lan

# In the app, shake the device and select 'Change Bundle URL', enter the displayed LAN address
```

##### Method 3: Build Custom Development Client APK

```bash
cd mobile

# Run when first installing or updating the client
npm run android

# This builds and installs the Expo Development Client
```

### 📦 Build Release APK (local)

```bash
cd mobile
npm install
cd android
./gradlew assembleRelease
cp app/build/outputs/apk/release/app-release.apk ../AllCallAll.apk
# Optional: install to device
adb install -r ../AllCallAll.apk
```

### 📁 Directory Structure

```
allcall/
├── backend/                    # Go backend service
│   ├── cmd/
│   │   └── server/             # Application entry point
│   ├── internal/
│   │   ├── auth/               # Authentication and JWT
│   │   ├── user/               # User management
│   │   ├── contact/            # Contact management
│   │   ├── signaling/          # WebRTC signaling
│   │   ├── media/              # Pion WebRTC media engine
│   │   ├── presence/           # Online status management
│   │   ├── models/             # Data models
│   │   ├── handlers/           # HTTP handlers
│   │   ├── database/           # Database connection
│   │   └── cache/              # Redis cache
│   ├── configs/                # Configuration files
│   └── Dockerfile              # Docker image
│
├── mobile/                     # React Native mobile application
│   ├── src/
│   │   ├── screens/            # Application pages
│   │   ├── components/         # UI components
│   │   ├── context/            # State management (Auth, Signaling)
│   │   ├── navigation/         # Routing configuration
│   │   ├── config/             # Application configuration
│   │   └── utils/              # Utility functions
│   ├── android/                # Android native code
│   ├── metro.config.js         # Metro bundler configuration
│   ├── app.json                # Expo configuration
│   └── package.json
│
├── infra/                      # Infrastructure configuration
│   ├── docker-compose.yml      # Local development environment
│   ├── docker-compose.production.yml  # Production environment
│   ├── cloudflared-config.yml  # Cloudflare Tunnel configuration
│   └── deploy.sh               # Cloud server deployment script
│
└── start.sh                    # Quick start script
```

### 🔧 Development & Debugging

#### Metro Development Server

Metro automatically detects the local LAN IP and binds dynamically. Check the startup log for the URL:

```bash
npm run start

# Sample output:
# 📱 Metro Development Server Configuration:
#    LAN IP: 192.168.1.36
#    Metro URL: http://192.168.1.36:8081
#    API URL: http://192.168.1.36:8080
#    ✅ Supports both USB and Wi-Fi connection modes
```

#### Network Configuration

Current configuration uses **ADB Reverse Forwarding** (Recommended):

```bash
# Automatic configuration (using script)
bash scripts/dev-client-debug.sh

# Manual configuration
adb reverse tcp:8080 tcp:8080  # Backend API service
adb reverse tcp:8081 tcp:8081  # Metro development server
```

Frontend app configuration (automatically uses localhost):

```typescript
// src/config/index.ts
const API_HOST = "http://localhost:8080";  // Forwarded by ADB
const WS_HOST = "ws://localhost:8080";      // WebSocket also forwarded
```

**Why use ADB reverse forwarding?**
- ✅ More stable and reliable than direct LAN IP
- ✅ Consistent with local development environment
- ✅ Supports debugging multiple devices simultaneously
- ✅ More stable network, lower latency

**Optional: LAN Mode (Wi-Fi Debugging)**
- Development Machine IP: 192.168.31.217
- Use Case: Development testing that requires wireless mobility

#### Common Development Commands

```bash
cd mobile

# Start Metro development server
npm run start

# Start in LAN mode (Wi-Fi debugging)
npm run start:lan

# Tunnel mode (cross-network)
npm run start:tunnel

# Build custom development client
npm run android

# Code linting
npm run lint
```

#### Debug Physical Device

```bash
# View device logs
adb logcat

# Clear app data and restart
adb shell pm clear com.allcallall.mobile
adb shell am start -n com.allcallall.mobile/.MainActivity

# Configure ADB reverse port forwarding
adb reverse tcp:8080 tcp:8080
adb reverse tcp:8081 tcp:8081
```

### 📡 API Endpoints

#### Authentication

```
POST   /api/v1/auth/register     - User registration
POST   /api/v1/auth/login        - User login
```

#### Users

```
GET    /api/v1/users/contacts    - Get contacts list
GET    /api/v1/users/presence    - Get user online status
GET    /api/v1/users/search      - Search users
```

#### Signaling

```
GET    /api/v1/ws                - WebSocket connection
```

### 🐛 Troubleshooting

#### Physical Device Cannot Connect to Development Server

**Issue**: `AxiosError: Network Error` or `Network timeout`

**Solution**:

1. **Check ADB reverse port forwarding configuration**
   ```bash
   adb reverse --list
   # Should show:
   # tcp:8080 tcp:8080
   # tcp:8081 tcp:8081
   
   # If missing, reconfigure
   adb reverse tcp:8080 tcp:8080
   adb reverse tcp:8081 tcp:8081
   ```

2. **Verify backend service is running**
   ```bash
   curl http://localhost:8080/health
   ```

3. **Check frontend configuration**
   ```bash
   cat mobile/src/config/index.ts
   # Should show API_HOST = "http://localhost:8080"
   ```

4. **Clear app data and restart**
   ```bash
   adb shell pm clear com.allcallall.mobile
   # Scan Metro QR code again on physical device
   ```

5. **Run complete startup script**
   ```bash
   bash mobile/scripts/dev-client-debug.sh
   ```

#### Metro Compilation Failed or Virtual Entry Point 404 Error

**Issue**: `Unable to resolve module ./.expo/.virtual-metro-entry` or compilation failed

**Solution**:

1. **Use automated script** (Recommended - automatically handles virtual entry point)
   ```bash
   bash mobile/scripts/dev-client-debug.sh
   ```

2. **Manual cleanup and restart**
   ```bash
   # Clear cache
   rm -rf mobile/node_modules/.cache /tmp/metro-*
   
   # Protect virtual entry point file (do NOT delete)
   ls -la mobile/.expo/.virtual-metro-entry.js
   
   # If virtual entry point file is missing, recreate it
   mkdir -p mobile/.expo
   cat > mobile/.expo/.virtual-metro-entry.js << 'EOF'
import { registerRootComponent } from 'expo';
import App from '../App';
registerRootComponent(App);
EOF
   
   # Reinstall dependencies and restart
   cd mobile
   npm install
   npm run start:dev-client
   ```

⚠️ **Important**: `.virtual-metro-entry.js` is a required Metro virtual entry point file. **Never delete it**. Running the script will automatically protect it.

#### Backend Service Cannot Start or Email Cannot Be Sent

**Issue 1**: `failed to connect mysql`

**Solution**:
```bash
# Ensure database service is running
./scripts/development/start-services.sh

# Check MySQL connection
mysql -u allcallall -p allcallall_db -h localhost

# Verify Redis connection
redis-cli ping
```

**Issue 2**: Email cannot be sent or verification code not received

**Solution**:
```bash
# Check QQ Mail SMTP config
cat backend/configs/config.yaml | grep -A5 mail:

# Verify environment variable
echo $MAIL_PASSWORD

# Test email sending endpoint
curl -X POST http://localhost:8080/api/v1/email/send-verification-code \
  -H "Content-Type: application/json" \
  -d '{"email":"test.user@example.com"}'

# Expected response: {"message":"verification code sent successfully"}
```

### 📚 Development Guide

#### Code Style

- **Go**: Follow [Effective Go](https://golang.org/doc/effective_go)
- **TypeScript**: ESLint configuration standards
- **Kotlin**: Android official style guide

#### Branch Strategy

- `main` - Stable release version
- `develop` - Development branch
- `feature/*` - Feature branches
- `bugfix/*` - Fix branches

### 🌍 Offline Translation Feature

The offline translation feature enables **real-time speech translation** using local AI models, ensuring complete privacy and zero operational costs.

#### Key Technologies
- **Whisper-small** (INT8): Speech Recognition
- **Opus-MT-en-zh** (INT8): Text Translation
- **VITS**: Text-to-Speech Synthesis
- **Total Size**: ~264MB (INT8 quantized)
- **Performance**: <500ms latency, >88% accuracy

#### Documentation
- **[Offline Translation Guide](docs/offline-translation/README.md)** - Complete documentation
- **[Implementation Roadmap](docs/offline-translation/IMPLEMENTATION_ROADMAP.md)** - 5-week development plan
- **[Model Download Guide](docs/offline-translation/setup/MODEL_DOWNLOAD_GUIDE.md)** - Setup instructions

### 🤝 Contributing

1. Fork the project
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

### 📝 License

MIT License - See the [LICENSE](LICENSE) file for details

### 📧 Contact

- Issues: [GitHub Issues](https://github.com/1victorloui1/AllCallAll/issues)
- Discussions: [GitHub Discussions](https://github.com/1victorloui1/AllCallAll/discussions)

### 🙏 Acknowledgments

- [Pion WebRTC](https://github.com/pion/webrtc) - WebRTC implementation
- [Expo](https://expo.dev/) - React Native development framework
- [Gin](https://gin-gonic.com/) - Web framework
- All contributors for their support and help

- [Pion WebRTC](https://github.com/pion/webrtc) - WebRTC 实现
- [Expo](https://expo.dev/) - React Native 开发框架
- [Gin](https://gin-gonic.com/) - Web 框架
- 所有贡献者的支持与帮助
