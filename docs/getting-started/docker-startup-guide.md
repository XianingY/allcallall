# Docker Compose 完整启动指南

## 🖥️ 当前环境信息

### 网络配置
- **本机局域网IP**: `<YOUR_LAN_IP>`
- **后端API端口**: `8080`
- **后端WebSocket端口**: `8080`
- **移动端开发服务器端口**: `8081`

> ⚠️ **重要**: 确保安卓真机和开发机在同一局域网中

## 🚀 快速启动（推荐）

### 方式1：使用启动脚本（最简单）

```bash
# 进入项目根目录
cd /Users/byzantium/github/allcallall

# 运行启动脚本（仅启动MySQL和Redis）
bash scripts/development/start-services.sh
```

**脚本会自动执行**：
- ✅ 启动MySQL 8.0数据库
- ✅ 启动Redis 7.2缓存
- ✅ 等待服务健康检查
- ✅ 显示服务状态

### 方式2：手动启动（更灵活）

```bash
# 进入infra目录
cd infra

# 启动所有服务（MySQL、Redis、Backend可选）
docker compose up -d

# 或仅启动数据库服务
docker compose up -d mysql redis
```

## 📦 Docker Compose 配置详解

### MySQL配置
```yaml
Database: allcallall_db
Username: allcallall
Password: ${MYSQL_PASSWORD}
Port: 3306
```

### Redis配置
```yaml
Port: 6379
Database: 0
```

### Backend配置（可选）
- 自动等待MySQL和Redis就绪
- 自动挂载配置文件
- 自动设置环境变量

## ⚙️ 后端服务启动

启动数据库后，需要在**新的终端窗口**中启动Go后端服务：

### 环境变量配置

```bash
# 必需的环境变量
export MAIL_PASSWORD="你的QQ邮箱授权码"
export DB_DSN="allcallall:${MYSQL_PASSWORD}@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local"
export REDIS_ADDR="localhost:6379"
```

### 启动后端服务

```bash
# 方式1：设置环境变量后启动
cd /Users/byzantium/github/allcallall/backend
export MAIL_PASSWORD="你的QQ邮箱授权码"
export DB_DSN="allcallall:${MYSQL_PASSWORD}@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local"
export REDIS_ADDR="localhost:6379"
go run cmd/server/main.go

# 方式2：一行命令启动
cd /Users/byzantium/github/allcallall/backend && \
export MAIL_PASSWORD="你的QQ邮箱授权码" && \
export DB_DSN="allcallall:${MYSQL_PASSWORD}@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local" && \
export REDIS_ADDR="localhost:6379" && \
go run cmd/server/main.go
```

### 验证后端服务

```bash
# 测试健康检查端点
curl http://localhost:8080/api/v1/health
# 期望响应: {"status":"ok"}
```

## 📱 移动端开发服务器启动

在**第三个终端窗口**中启动移动端开发服务器：

```bash
cd /Users/byzantium/github/allcallall/mobile

# 启动Expo开发服务器
npm run start

# 或使用
npm start
```

### 连接真机设备

1. 在安卓真机上安装 **Expo Go** 应用
2. 确保手机和开发机在同一WiFi网络中
3. 方式一：扫描终端显示的QR码
4. 方式二：手动输入 `exp://<YOUR_LAN_IP>:8081`

## 🔍 Docker Compose 常用命令

### 查看服务状态
```bash
cd infra
docker compose ps
```

### 查看服务日志
```bash
# 查看所有日志
docker compose logs -f

# 查看MySQL日志
docker compose logs -f mysql

# 查看Redis日志
docker compose logs -f redis

# 查看Backend日志
docker compose logs -f backend
```

### 停止服务
```bash
# 停止所有服务（保留数据）
docker compose stop

# 停止并删除容器（保留数据卷）
docker compose down

# 停止并删除所有数据
docker compose down -v
```

### 重启服务
```bash
# 重启单个服务
docker compose restart mysql

# 重启所有服务
docker compose restart
```

## 📋 完整启动流程（一步步指南）

### 终端1：启动数据库服务
```bash
cd /Users/byzantium/github/allcallall
bash scripts/development/start-services.sh
```
等待输出：
```
📊 Service Status:
...
mysql          ...     Up 10 seconds (health: healthy)
redis          ...     Up 10 seconds (health: healthy)
```

### 终端2：启动后端服务
```bash
cd /Users/byzantium/github/allcallall/backend
export MAIL_PASSWORD="你的QQ邮箱授权码"
export DB_DSN="allcallall:${MYSQL_PASSWORD}@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local"
export REDIS_ADDR="localhost:6379"
go run cmd/server/main.go
```
等待输出：
```
2025-XX-XX INF mysql connection established
2025-XX-XX INF connected to redis successfully
2025-XX-XX INF http server starting addr=0.0.0.0:8080
```

### 终端3：启动移动端开发服务器
```bash
cd /Users/byzantium/github/allcallall/mobile
npm run start
```
等待输出：
```
› Metro waiting on exp://<YOUR_LAN_IP>:8081
› Scan the QR code above with Expo Go
```

## 🔐 移动端配置信息

### 已配置的API地址
- **开发环境**: `http://<YOUR_LAN_IP>:8080`
- **WebSocket**: `ws://<YOUR_LAN_IP>:8080`
- **API基础路径**: `/api/v1`

### 配置文件位置
- `mobile/src/config/index.ts` - 主配置文件

### 环境变量检测
移动应用启动时会自动检测环境变量 `APP_ENV`：
- `development` - 使用本地开发配置
- `staging` - 使用测试环境配置
- `production` - 使用生产环境配置

## ⚠️ 常见问题

### 问题1：Docker容器无法启动
```bash
# 检查Docker是否运行
docker info

# 查看容器日志
cd infra
docker compose logs mysql

# 重新构建并启动
docker compose down -v
docker compose up -d
```

### 问题2：手机无法连接后端服务
**检查清单**：
- ✅ 后端服务是否正常运行：`curl http://localhost:8080/api/v1/health`
- ✅ 手机是否连接正确的WiFi网络
- ✅ 防火墙是否阻止8080端口
- ✅ `mobile/src/config/index.ts` 中的IP地址是否正确为 `<YOUR_LAN_IP>`

### 问题3：MySQL连接错误
```bash
# 检查MySQL是否就绪
docker compose exec mysql mysql -uallcallall -p"${MYSQL_PASSWORD}" -e "SELECT 1;"

# 等待健康检查完成（可能需要30秒）
sleep 30
docker compose ps
```

### 问题4：Redis连接错误
```bash
# 检查Redis是否就绪
docker compose exec redis redis-cli ping
# 期望响应: PONG
```

## 📝 环境变量说明

| 变量名 | 默认值 | 说明 |
|------|------|------|
| MAIL_PASSWORD | - | QQ邮箱授权码（必需） |
| DB_DSN | 见下表 | 数据库连接字符串 |
| REDIS_ADDR | localhost:6379 | Redis连接地址 |
| APP_ENV | development | 应用环境（development/staging/production） |

## 🔗 服务端口映射

| 服务 | 容器端口 | 主机端口 | 说明 |
|-----|---------|---------|------|
| MySQL | 3306 | 3306 | 数据库 |
| Redis | 6379 | 6379 | 缓存 |
| Backend | 8080 | 8080 | HTTP API + WebSocket |
| Metro | 8081 | 8081 | 移动端开发服务器 |

## ✅ 启动检查清单

- [ ] Docker服务正常运行
- [ ] MySQL和Redis容器健康
- [ ] 后端服务正常启动（端口8080可访问）
- [ ] 移动端开发服务器正常启动（端口8081可访问）
- [ ] 手机Expo Go应用已安装
- [ ] 手机和开发机在同一WiFi网络
- [ ] `mobile/src/config/index.ts` 中的IP地址为 `<YOUR_LAN_IP>`
- [ ] 可以在手机上扫描QR码或输入URL连接到开发服务器

---

**提示**: 如需更改局域网IP地址，请编辑 `mobile/src/config/index.ts` 文件中的 `development` 环境配置。
