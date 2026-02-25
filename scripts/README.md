# AllCallAll 项目脚本使用指南

此目录包含项目所有的实用脚本，按用途分为三类。

## 📁 目录结构

### 开发脚本 (`development/`)

用于本地开发和测试环境。

#### `start-services.sh`
启动本地开发环境（MySQL + Redis）

```bash
./scripts/development/start-services.sh
```

**功能**:
- 启动 Docker 容器 (MySQL, Redis)
- 等待服务就绪
- 显示服务状态和连接信息

#### `restart-services.sh`
重启所有服务（数据库 + 后端）

```bash
./scripts/development/restart-services.sh
```

**功能**:
- 停止现有的所有服务
- 启动 Docker 容器
- 启动后端服务
- 验证服务健康状态

**注意**: 该脚本会在后台启动后端服务

#### `start-android-debug.sh`
设置 Android 真机调试环境

```bash
./scripts/development/start-android-debug.sh
```

**功能**:
- 配置 ADB 调试
- 设置设备连接
- 启动移动端开发服务器

---

### 部署脚本 (`deployment/`)

用于生产环境部署。

#### `deploy-cloud.sh`
云服务器部署脚本（Nginx/UFW 方式）

```bash
./scripts/deployment/deploy-cloud.sh <server-ip> [domain-name] [work-dir]
```

**功能**:
- 安装 Docker 和 Docker Compose
- 克隆项目代码
- 创建环境配置文件
- 配置 Nginx
- 配置防火墙

**相关脚本**: 如需使用 Cloudflare Tunnel 方式部署，请使用 `infra/deploy-cloudflare-tunnel.sh`

---

### 测试脚本 (`testing/`)

用于功能测试和验证。

#### `test-change-password.sh`
测试修改密码功能

```bash
./scripts/testing/test-change-password.sh
```

**功能**:
- 测试用户登录
- 测试密码修改 API
- 验证修改结果

---

## 🚀 快速开始

### 初次设置

```bash
# 1. 启动数据库和 Redis
./scripts/development/start-services.sh

# 2. 在另一个终端启动后端
cd backend && go run cmd/server/main.go

# 3. 在第三个终端启动移动端开发服务器
cd mobile && npm start
```

### 完整重启

```bash
# 一次性重启所有服务
./scripts/development/restart-services.sh
```

---

## ⚠️ 常见问题

### Docker 启动失败
- 确保 Docker Desktop 正在运行
- 检查磁盘空间是否充足
- 查看日志: `docker-compose logs`

### 后端无法连接数据库
- 确保 MySQL 容器正在运行: `docker ps`
- 检查数据库凭证配置
- 查看后端日志了解详细错误

### 脚本权限不足
```bash
# 添加执行权限
chmod +x scripts/development/*.sh
chmod +x scripts/deployment/*.sh
chmod +x scripts/testing/*.sh
```

---

## 📝 脚本维护

新增脚本时，请：

1. 将脚本放在相应的子目录
2. 添加可执行权限: `chmod +x script.sh`
3. 在此 README 中添加说明
4. 确保脚本有清晰的错误处理和日志输出

---

**最后更新**: 2025-12-16
