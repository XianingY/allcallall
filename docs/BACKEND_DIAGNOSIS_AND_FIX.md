# 后端服务诊断和修复报告

## 🚨 问题诊断

### 当前状态
- **后端服务**: 持续重启（Restarting）
- **MySQL服务**: ✅ 正常运行（Up 22 hours）
- **Redis服务**: ✅ 正常运行（Up 22 hours）
- **Nginx服务**: ✅ 正常运行（Up 22 hours）

### 根本原因

后端服务无法启动的原因：

**问题1：MySQL认证失败**
```
Error 1045 (28000): Access denied for user 'allcallall'@'172.19.0.2' (using password: YES)
```
- MySQL容器中设置的密码：`allcallallpass`
- 后端DB_DSN中使用的密码：加密后的密码（VthN8XqT6U3vorMJGbkhKRlFEk4=）
- **结果**: 密码不匹配，认证失败

**问题2：Redis认证失败**
- Redis设置了`--requirepass`（需要认证）
- 后端收到的REDIS_PASSWORD：加密后的密码（k2jEfvZ1r/GDIWTEnuTgMubaBTQ=）
- **结果**: 密码不匹配或格式错误

### 环境变量配置问题

Docker Compose中的环境变量被加密，这导致Backend无法正确连接数据库和缓存。

---

## 🔧 修复方案

### 步骤1：停止当前运行的后端服务

```bash
cd /Users/byzantium/github/allcallall/infra
docker-compose -f docker-compose.production.yml stop backend
```

### 步骤2：删除并重建Backend容器

```bash
# 删除后端容器（保留数据）
docker-compose -f docker-compose.production.yml rm -f backend

# 重新创建Backend容器，使用正确的环境变量
docker-compose -f docker-compose.production.yml up -d backend
```

### 步骤3：设置正确的环境变量

在启动Docker Compose之前，确保环境变量设置正确：

```bash
# 设置环境变量为与MySQL和Redis中实际配置相同的值
export MYSQL_ROOT_PASSWORD="rootpass"
export MYSQL_PASSWORD="allcallallpass"
export REDIS_PASSWORD="redis_secure_password"
export JWT_SECRET="your-secret-key-min-32-characters-required"
export MAIL_PASSWORD="your-qq-email-auth-code"
export WEBRTC_ICE_SERVERS_JSON='[{"urls":["stun:stun.l.google.com:19302"]}]'

# 验证环境变量
env | grep -E "MYSQL|REDIS|JWT|MAIL|WEBRTC"
```

### 步骤4：重启Backend服务

```bash
# 启动backend服务
docker-compose -f docker-compose.production.yml up -d backend

# 等待启动（约10-15秒）
sleep 15

# 检查状态
docker-compose -f docker-compose.production.yml ps backend
```

### 步骤5：验证服务连接

```bash
# 查看Backend日志
docker-compose -f docker-compose.production.yml logs -f backend

# 等待看到类似这样的成功信息：
# 2025-XX-XX INF mysql connection established
# 2025-XX-XX INF connected to redis successfully
# 2025-XX-XX INF http server starting addr=0.0.0.0:8080
```

### 步骤6：测试API端点

```bash
# 测试健康检查
curl http://localhost:8080/ping
# 期望响应: {"message":"pong"}

# 或测试API健康检查
curl http://localhost:8080/api/v1/health
```

---

## 🔍 详细诊断结果

### MySQL连接测试

```bash
# ✅ 通过密码认证测试
docker exec infra-mysql-1 mysql -uallcallall -pallcallallpass allcallall_db -e "SELECT 1;"
# 输出: 1
```

**状态**: ✅ MySQL可以使用标准密码连接成功

### Redis连接测试

```bash
# ❌ Redis认证失败
docker exec infra-redis-1 redis-cli -a redis_secure_password ping
# 输出: Warning... AUTH failed: WRONGPASS invalid username-password pair
```

**状态**: ❌ Redis中的密码配置与后端设置的密码不匹配

### 需要使用的正确密码

| 服务 | 用户 | 正确密码 | Docker Compose变量 |
|-----|------|---------|------------------|
| MySQL | allcallall | allcallallpass | MYSQL_PASSWORD |
| Redis | (default) | redis_secure_password | REDIS_PASSWORD |
| JWT | (N/A) | your-secret-key-min-32-chars | JWT_SECRET |
| Mail | (N/A) | your-qq-auth-code | MAIL_PASSWORD |

---

## ⚠️ 问题原因分析

### 为什么密码被加密？

Docker Compose可能使用了密钥管理功能或环境变量被加密存储，导致实际使用的密码与预期不符。

### 如何避免此问题？

1. **使用.env文件**：创建`.env`文件存储明文密码
2. **验证环境变量**：启动前验证所有必需的环境变量
3. **使用启动脚本**：使用提供的`start-production.sh`脚本，它会自动设置环境变量

---

## 📋 完整修复流程（一步步）

### 方案A：快速修复（推荐）

```bash
#!/bin/bash

cd /Users/byzantium/github/allcallall/infra

# 停止后端服务
docker-compose -f docker-compose.production.yml stop backend

# 设置正确的环境变量
export MYSQL_ROOT_PASSWORD="rootpass"
export MYSQL_PASSWORD="allcallallpass"
export REDIS_PASSWORD="redis_secure_password"
export JWT_SECRET="your-secret-key-min-32-characters-required"
export MAIL_PASSWORD="your-qq-email-auth-code"
export WEBRTC_ICE_SERVERS_JSON='[{"urls":["stun:stun.l.google.com:19302"]}]'

# 删除旧的后端容器
docker-compose -f docker-compose.production.yml rm -f backend

# 重新启动后端服务
docker-compose -f docker-compose.production.yml up -d backend

# 等待启动
sleep 15

# 检查状态
docker-compose -f docker-compose.production.yml ps

# 查看日志确认成功
docker-compose -f docker-compose.production.yml logs backend --tail=20

# 测试API
curl http://localhost:8080/ping
```

### 方案B：使用启动脚本

```bash
cd /Users/byzantium/github/allcallall/infra

# 设置环境变量
export JWT_SECRET="your-secret-key-min-32-chars"
export MAIL_PASSWORD="your-qq-auth-code"

# 运行启动脚本（会自动设置其他变量）
bash start-production.sh
```

---

## ✅ 修复验证检查清单

修复完成后，按以下顺序验证：

### 1. 检查服务状态
```bash
docker-compose -f docker-compose.production.yml ps
# Backend应显示: Up (healthy) 或 Up
```

### 2. 检查后端日志
```bash
docker-compose -f docker-compose.production.yml logs backend --tail=20

# 应该看到：
# 2025-XX-XX INF mysql connection established
# 2025-XX-XX INF connected to redis successfully
# 2025-XX-XX INF http server starting addr=0.0.0.0:8080
```

### 3. 测试MySQL连接
```bash
docker-compose -f docker-compose.production.yml exec backend mysql -uallcallall -pallcallallpass allcallall_db -e "SELECT COUNT(*) FROM information_schema.tables;"
```

### 4. 测试Redis连接
```bash
docker-compose -f docker-compose.production.yml exec backend redis-cli -a redis_secure_password ping
# 期望响应: PONG
```

### 5. 测试API端点
```bash
# 健康检查
curl http://localhost:8080/ping
# 响应: {"message":"pong"}

# API健康检查
curl http://localhost:8080/api/v1/health
# 响应: 应返回200状态码

# 测试用户认证端点
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"password123"}'
# 应返回错误提示（用户不存在）或认证失败，但不应是500错误
```

---

## 📝 关键文件位置

- Docker Compose配置: `/Users/byzantium/github/allcallall/infra/docker-compose.production.yml`
- 启动脚本: `/Users/byzantium/github/allcallall/infra/start-production.sh`
- 后端配置: `/Users/byzantium/github/allcallall/backend/configs/config.production.yaml`
- 后端日志输出: `docker-compose logs backend`

---

## 🆘 如果问题仍未解决

### 检查点1：数据库连接字符串
```bash
# 检查后端中设置的DB_DSN
docker-compose -f docker-compose.production.yml config | grep "DB_DSN"

# 应该显示:
# DB_DSN: allcallall:allcallallpass@tcp(mysql:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local
```

### 检查点2：MySQL用户和密码
```bash
# 重置MySQL用户（如果需要）
docker-compose -f docker-compose.production.yml exec mysql mysql -uroot -prootpass -e "
  ALTER USER 'allcallall'@'%' IDENTIFIED BY 'allcallallpass';
  FLUSH PRIVILEGES;
"
```

### 检查点3：Redis配置
```bash
# 检查Redis密码配置
docker-compose -f docker-compose.production.yml exec redis redis-cli -a redis_secure_password CONFIG GET requirepass
```

### 检查点4：完全重建（最后手段）
```bash
# 删除所有容器和卷（警告：会删除所有数据！）
docker-compose -f docker-compose.production.yml down -v

# 重新启动所有服务
export MYSQL_ROOT_PASSWORD="rootpass"
export MYSQL_PASSWORD="allcallallpass"
export REDIS_PASSWORD="redis_secure_password"
export JWT_SECRET="your-secret-key"
export MAIL_PASSWORD="your-qq-auth-code"

docker-compose -f docker-compose.production.yml up -d
```

---

## 📚 相关文档

- [Docker Compose启动指南](../docs/DOCKER_STARTUP_GUIDE.md)
- [生产环境完整指南](../docs/PRODUCTION_SETUP_AND_APK_BUILD.md)
- [快速参考](../APK_BUILD_QUICK_REFERENCE.md)

---

**诊断时间**: 2025-12-14 23:40 UTC
**IP地址**: 10.136.17.108
**工作目录**: /Users/byzantium/github/allcallall
