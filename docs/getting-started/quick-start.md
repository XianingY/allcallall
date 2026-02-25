# AllCallAll 快速启动卡片

## 🎯 当前网络配置
```
本机IP地址: <YOUR_LAN_IP>
后端服务: http://<YOUR_LAN_IP>:8080
WebSocket: ws://<YOUR_LAN_IP>:8080
```

## ⚡ 三个终端，三个命令

### 终端1：启动数据库（MySQL + Redis）
```bash
cd /Users/byzantium/github/allcallall
bash scripts/development/start-services.sh
```

### 终端2：启动后端服务
```bash
cd /Users/byzantium/github/allcallall && \
set -a && source .env && set +a && \
cd backend && \
export MAIL_PASSWORD="${MAIL_PASSWORD:-你的QQ授权码}" && \
export DB_DSN="allcallall:${MYSQL_PASSWORD}@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local" && \
export REDIS_ADDR="localhost:6379" && export REDIS_PASSWORD="${REDIS_PASSWORD}" && \
go run cmd/server/main.go
```

### 终端3：启动移动端开发服务器
```bash
cd /Users/byzantium/github/allcallall/mobile
bash scripts/dev-client-debug.sh
```

## 📱 在真机上运行
1. 安卓手机安装 **Expo Go** 应用
2. 扫描终端3显示的QR码
3. 或手动输入: `exp://<YOUR_LAN_IP>:8081`

## ✅ 验证服务状态
```bash
# 检查后端服务
curl http://localhost:8080/api/v1/health

# 检查容器状态
docker compose -f infra/docker-compose.yml ps
```

## 🛑 停止所有服务
```bash
# 关闭Docker容器
cd /Users/byzantium/github/allcallall/infra && docker compose -f docker-compose.yml down

# 终止后端服务（Ctrl+C）和移动端服务（Ctrl+C）
```

## 📚 详细文档
- 完整启动指南: `docs/getting-started/docker-startup-guide.md`
- API文档: `docs/api/api-documentation.md`
- 数据库文档: `docs/api/database.md`

---

**注意**: 真机开发建议优先使用 `mobile/scripts/dev-client-debug.sh` 自动配置 ADB 反向代理。
