# AllCallAll

[English](#english) | [中文](#中文)

---

## 中文

> 一个基于 WebRTC 的实时音视频通信平台，支持点对点语音通话、联系人管理和在线状态同步。

### ✨ 特性

- 🎤 **实时音视频通话** - 基于 Pion WebRTC 的点对点音频通话
- 👥 **联系人管理** - 添加、搜索和管理通讯录
- 🟢 **在线状态** - 实时显示用户在线状态和最后在线时间
- 🔐 **用户认证** - JWT 令牌认证和会话管理
- 📱 **跨平台** - Android 原生应用支持，iOS 开发中
- 🚀 **高性能** - Redis 缓存、连接池优化、异步 WebSocket 信令
- 🔄 **自动重连** - 网络异常自动重新连接

### 🛠 技术栈

### 后端
- **语言**: Go 1.22+
- **框架**: Gin（HTTP）、Gorilla WebSocket
- **数据库**: MySQL 8.0
- **缓存**: Redis 7.2
- **WebRTC**: Pion v4.0.0
- **认证**: JWT (golang-jwt)

### 移动端
- **框架**: React Native 0.74+
- **开发**: Expo 51.0+
- **语言**: TypeScript
- **UI**: React Navigation
- **WebRTC**: react-native-webrtc

### 基础设施
- **容器化**: Docker & Docker Compose
- **服务代理**: Cloudflare Tunnel

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
git clone https://github.com/XianingY/AllCallAll.git
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
./start.sh

# 验证服务状态
docker-compose -f infra/docker-compose.yml ps
```

### 启动后端服务

```bash
cd backend

# 设置配置文件路径
export CONFIG_PATH=./configs/config.yaml

# 运行后端服务（监听 0.0.0.0:8080）
go run cmd/server/main.go
```

### 启动移动应用

#### 方式 1: USB 连接调试（推荐开发）

```bash
cd mobile

# 构建并安装自定义开发客户端
npm run android

# 在另一个终端启动 Metro 开发服务器
npm run start
```

#### 方式 2: Wi-Fi 无线调试

```bash
cd mobile

# 启动 Metro 服务器（LAN 模式）
npm run start:lan

# 在真机摇一摇菜单中选择 'Change Bundle URL'，输入显示的 LAN 地址
```

#### 方式 3: Cloudflare Tunnel（跨网络）

```bash
cd mobile

# 启动 Tunnel 模式
npm run start:tunnel
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

网络配置由三个部分统一管理：

1. **metro.config.js** - 动态获取本机 LAN IP
2. **src/config/index.ts** - 根据运行平台选择 API 地址
3. **后端配置** - 通过环境变量和 config.yaml 管理

```typescript
// src/config/index.ts
const LAN_IP = "192.168.1.36";  // 开发机 IP
const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;

const API_HOST = isPhysicalAndroid
  ? `http://${LAN_IP}:8080`       // 真机使用 LAN IP
  : Platform.OS === "android"
  ? "http://10.0.2.2:8080"        // 模拟器使用特殊地址
  : "http://localhost:8080";      // 开发机使用本地地址
```

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

**问题**: `AxiosError: Network Error`

**解决方案**:
1. 确认开发机和真机在同一局域网
2. 检查 `src/config/index.ts` 中的 LAN_IP 与开发机 IP 是否一致
3. 运行 `ipconfig getifaddr en0` 检查本机 IP
4. 清除应用数据：`adb shell pm clear com.allcallall.mobile`
5. 重新启动应用

### Metro 编译失败

**问题**: `Unable to resolve module`

**解决方案**:
```bash
# 清理缓存
rm -rf node_modules/.cache /tmp/metro-*
rm -rf .expo

# 重新安装依赖
npm install

# 启动 Metro
npm run start
```

### 后端服务无法启动

**问题**: `failed to connect mysql`

**解决方案**:
```bash
# 确保数据库服务已启动
./start.sh

# 检查 MySQL 连接
mysql -u allcallall -p allcallall_db -h localhost

# 验证 Redis 连接
redis-cli ping
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

- 问题报告: [GitHub Issues](https://github.com/yourusername/allcall/issues)
- 讨论: [GitHub Discussions](https://github.com/yourusername/allcall/discussions)

### 🙏 致谢



---

## English

> Real-time audio/video communication platform built with WebRTC and React Native.

### ✨ Features

- 🎤 **Real-time Audio/Video Calls** - Peer-to-peer audio calls based on Pion WebRTC
- 👥 **Contact Management** - Add, search, and manage contacts
- 🟢 **Online Status** - Real-time user presence and last seen information
- 🔐 **User Authentication** - JWT token authentication and session management
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

#### Mobile
- **Framework**: React Native 0.74+
- **Development**: Expo 51.0+
- **Language**: TypeScript
- **UI**: React Navigation
- **WebRTC**: react-native-webrtc

#### Infrastructure
- **Containerization**: Docker & Docker Compose
- **Service Proxy**: Cloudflare Tunnel

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
git clone https://github.com/XianingY/AllCallAll.git
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

#### Start Database Services

```bash
# Start MySQL and Redis
./start.sh

# Verify service status
docker-compose -f infra/docker-compose.yml ps
```

#### Start Backend Service

```bash
cd backend

# Set configuration file path
export CONFIG_PATH=./configs/config.yaml

# Run backend service (listening on 0.0.0.0:8080)
go run cmd/server/main.go
```

#### Start Mobile Application

##### Method 1: USB Connection Debugging (Recommended for Development)

```bash
cd mobile

# Build and install custom development client
npm run android

# In another terminal, start the Metro development server
npm run start
```

##### Method 2: Wireless Debugging over Wi-Fi

```bash
cd mobile

# Start Metro server (LAN mode)
npm run start:lan

# In the app, shake the device and select 'Change Bundle URL', enter the displayed LAN address
```

##### Method 3: Cloudflare Tunnel (Cross-network)

```bash
cd mobile

# Start Tunnel mode
npm run start:tunnel
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

Network configuration is managed by three components:

1. **metro.config.js** - Dynamically obtains the local LAN IP
2. **src/config/index.ts** - Selects API address based on runtime platform
3. **Backend configuration** - Managed via environment variables and config.yaml

```typescript
// src/config/index.ts
const LAN_IP = "192.168.1.36";  // Development machine IP
const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;

const API_HOST = isPhysicalAndroid
  ? `http://${LAN_IP}:8080`       // Physical device uses LAN IP
  : Platform.OS === "android"
  ? "http://10.0.2.2:8080"        // Emulator uses special address
  : "http://localhost:8080";      // Development machine uses localhost
```

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

**Issue**: `AxiosError: Network Error`

**Solution**:
1. Ensure the development machine and physical device are on the same LAN
2. Check that the LAN_IP in `src/config/index.ts` matches your machine's IP
3. Run `ipconfig getifaddr en0` to check your machine's IP
4. Clear app data: `adb shell pm clear com.allcallall.mobile`
5. Restart the app

#### Metro Compilation Failed

**Issue**: `Unable to resolve module`

**Solution**:
```bash
# Clear cache
rm -rf node_modules/.cache /tmp/metro-*
rm -rf .expo

# Reinstall dependencies
npm install

# Start Metro
npm run start
```

#### Backend Service Cannot Start

**Issue**: `failed to connect mysql`

**Solution**:
```bash
# Ensure database service is running
./start.sh

# Check MySQL connection
mysql -u allcallall -p allcallall_db -h localhost

# Verify Redis connection
redis-cli ping
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

### 🤝 Contributing

1. Fork the project
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

### 📝 License

MIT License - See the [LICENSE](LICENSE) file for details

### 📧 Contact

- Issues: [GitHub Issues](https://github.com/yourusername/allcall/issues)
- Discussions: [GitHub Discussions](https://github.com/yourusername/allcall/discussions)

### 🙏 Acknowledgments

- [Pion WebRTC](https://github.com/pion/webrtc) - WebRTC implementation
- [Expo](https://expo.dev/) - React Native development framework
- [Gin](https://gin-gonic.com/) - Web framework
- All contributors for their support and help

- [Pion WebRTC](https://github.com/pion/webrtc) - WebRTC 实现
- [Expo](https://expo.dev/) - React Native 开发框架
- [Gin](https://gin-gonic.com/) - Web 框架
- 所有贡献者的支持与帮助