# AllCallAll 快速启动卡片

## 🎯 当前网络配置
```
本机IP地址: 10.136.17.108
后端服务: http://10.136.17.108:8080
WebSocket: ws://10.136.17.108:8080
```

## ⚡ 三个终端，三个命令

### 终端1：启动数据库（MySQL + Redis）
```bash
cd /Users/byzantium/github/allcallall
./start.sh
```

### 终端2：启动后端服务
```bash
cd /Users/byzantium/github/allcallall/backend && \
export MAIL_PASSWORD="你的QQ授权码" && \
export DB_DSN="allcallall:allcallallpass@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local" && \
export REDIS_ADDR="localhost:6379" && \
go run cmd/server/main.go
```

### 终端3：启动移动端开发服务器
```bash
cd /Users/byzantium/github/allcallall/mobile
npm run start
```

## 📱 在真机上运行
1. 安卓手机安装 **Expo Go** 应用
2. 扫描终端3显示的QR码
3. 或手动输入: `exp://10.136.17.108:8081`

## ✅ 验证服务状态
```bash
# 检查后端服务
curl http://localhost:8080/ping

# 检查数据库
docker exec -i infra-mysql-1 mysql -uallcallall -pallcallallpass -e "SELECT 1;"

# 检查Redis
docker exec -i infra-redis-1 redis-cli ping
```

## 🛑 停止所有服务
```bash
# 关闭Docker容器
cd infra && docker-compose down

# 终止后端服务（Ctrl+C）和移动端服务（Ctrl+C）
```

## 📚 详细文档
- 完整启动指南: `docs/DOCKER_STARTUP_GUIDE.md`
- API文档: `docs/API_DOCUMENTATION.md`
- 数据库文档: `docs/DATABASE.md`

---

**注意**: IP地址 `10.136.17.108` 已自动配置在 `mobile/src/config/index.ts` 中
