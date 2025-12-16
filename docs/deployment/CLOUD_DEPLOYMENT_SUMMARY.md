# 🚀 AllCallAll 云服务器部署方案 - 完整总结

## 📊 部署架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                    互联网用户 (Internet Users)                    │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                   公网 IP: 81.68.168.207
                   域名: api.allcall.com
                            │
        ┌───────────────────┴───────────────────┐
        │                                       │
        ▼                                       ▼
   HTTP:80 ───────────────────────────────► HTTPS:443
   (自动重定向到 HTTPS)                   (安全连接)
        │
        ▼
   ┌──────────────────────────────────────────────┐
   │            Nginx 反向代理 + SSL/TLS          │
   │  • HTTP/HTTPS 协议转换                       │
   │  • 负载均衡                                   │
   │  • 静态文件服务                               │
   │  • 路由转发                                   │
   └──────────────┬───────────────────────────────┘
                  │
        ┌─────────┼─────────┐
        ▼         ▼         ▼
   ┌─────────┐ ┌────────┐ ┌──────────┐
   │ Backend │ │ MySQL  │ │  Redis   │
   │  API    │ │  DB    │ │  Cache   │
   │(8080)   │ │(3306)  │ │ (6379)   │
   └─────────┘ └────────┘ └──────────┘
        │         │           │
        └─────────┴───────────┘
              Docker Compose
              (在 /opt/allcall/infra)

┌──────────────────────────────────────────────────────┐
│         移动应用 (Mobile App - Android/iOS)          │
│  • 通过公网 IP 或域名连接                            │
│  • HTTPS API 调用                                   │
│  • WSS WebSocket 信令连接                           │
│  • WebRTC 点对点视频通话                            │
└──────────────────────────────────────────────────────┘
```

---

## 🎯 部署要点速览

### 后端服务部署
| 组件 | 端口 | 容器网络 | 外网访问 |
|------|------|---------|---------|
| Go API | 8080 | 内部 | 通过 Nginx 代理 |
| MySQL | 3306 | 内部 | ❌ 不开放 |
| Redis | 6379 | 内部 | ❌ 不开放 |
| Nginx | 80/443 | 外部 | ✅ 开放 |

### 移动应用连接配置
```typescript
// 开发环境（本地）
API: http://192.168.31.217:8080
WS: ws://192.168.31.217:8080

// 生产环境（云服务器）
API: https://api.allcall.com
WS: wss://api.allcall.com
```

### 安全策略
- ✅ HTTPS 加密（Let's Encrypt 免费证书）
- ✅ WebSocket 安全连接（WSS）
- ✅ JWT 令牌认证
- ✅ 防火墙端口限制
- ✅ 数据库不对外开放

---

## 📋 完整部署流程

### 第 1 步：云服务器准备（10-15 分钟）

**需要完成的事项**：
- [ ] 云服务器已购买（推荐配置：2+ CPU, 4GB+ RAM, 20GB+ 存储）
- [ ] SSH 密钥已配置
- [ ] 安全组已开放端口 22, 80, 443

**命令**：
```bash
# SSH 连接到服务器
ssh -i /path/to/key.pem ubuntu@81.68.168.207

# 更新系统
sudo apt update && sudo apt upgrade -y
```

---

### 第 2 步：运行自动部署脚本（30-45 分钟）

**一键部署**：
```bash
cd /opt/allcall
bash scripts/deploy-cloud.sh 81.68.168.207 api.allcall.com
```

**脚本自动完成**：
- ✅ 安装 Docker 和 Docker Compose
- ✅ 克隆项目代码
- ✅ 创建环境配置文件
- ✅ 配置 Nginx
- ✅ 配置防火墙

---

### 第 3 步：配置环境变量（5-10 分钟）

**编辑 .env 文件**：
```bash
nano /opt/allcall/.env
```

**必须修改的字段**：
```bash
MYSQL_ROOT_PASSWORD=your_strong_password
MYSQL_PASSWORD=your_strong_password
REDIS_PASSWORD=your_strong_password
JWT_SECRET=your_strong_jwt_secret
MAIL_PASSWORD=your_qq_email_auth_code
```

---

### 第 4 步：启动所有服务（10-15 分钟）

**启动容器**：
```bash
cd /opt/allcall/infra
docker-compose up -d

# 查看状态
docker-compose ps

# 查看日志
docker-compose logs -f backend
```

**等待指标**：
- MySQL 健康检查通过（约 10s）
- Redis 健康检查通过（约 10s）
- 后端服务启动完成（约 20s）

---

### 第 5 步：配置 HTTPS 证书（10-15 分钟）

**前置条件**：
- 域名 DNS 已指向服务器 IP（81.68.168.207）

**申请证书**：
```bash
# 安装 Certbot
sudo apt install -y certbot python3-certbot-nginx

# 申请证书
sudo certbot certonly --standalone -d api.allcall.com

# 启用自动续期
sudo systemctl enable certbot.timer
sudo systemctl start certbot.timer
```

**验证证书**：
```bash
curl https://api.allcall.com/api/v1/health
```

---

### 第 6 步：配置移动应用（5 分钟）

**更新 API 配置文件**：
```typescript
// mobile/src/config/cloud.config.ts
const ENV_CONFIG = {
  production: {
    HTTP: "https://api.allcall.com",
    WS: "wss://api.allcall.com"
  }
};
```

**构建生产版本**：
```bash
cd mobile
eas build --platform android --release
```

---

### 第 7 步：验证和测试（10-15 分钟）

**测试后端 API**：
```bash
# 健康检查
curl https://api.allcall.com/api/v1/health

# 用户接口
curl -X POST https://api.allcall.com/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Test123456","display_name":"Test"}'
```

**测试 WebSocket 连接**：
- 在移动应用中注册账户
- 登录应用
- 验证 WebSocket 连接成功（查看日志）
- 测试发起通话

**端到端测试**：
- 两台设备登录应用
- 一台发起通话，另一台应答
- 验证语音/视频传输正常

---

## 🔑 重要配置清单

### 环境变量（.env）
```bash
✓ MYSQL_ROOT_PASSWORD - 强密码（由脚本生成）
✓ MYSQL_PASSWORD - 强密码（由脚本生成）
✓ REDIS_PASSWORD - 强密码（由脚本生成）
✓ JWT_SECRET - 长随机密钥（由脚本生成）
✓ MAIL_PASSWORD - QQ 邮箱授权码（手动设置）
✓ APP_ENV - production
✓ DOMAIN_NAME - api.allcall.com（改为你的域名）
```

### Nginx 配置
```nginx
✓ HTTP 80 -> 自动重定向 HTTPS 443
✓ HTTPS 443 SSL/TLS
✓ 证书路径：/etc/letsencrypt/live/api.allcall.com/
✓ WebSocket 升级头配置
✓ 反向代理到后端：http://backend:8080
```

### 防火墙规则
```bash
✓ Port 22 (SSH) - 允许管理
✓ Port 80 (HTTP) - 允许（自动重定向）
✓ Port 443 (HTTPS) - 允许
✓ Port 3306, 6379 - 仅内部网络
✓ 其他端口 - 拒绝
```

### DNS 配置
```dns
A 记录: api.allcall.com → 81.68.168.207
A 记录: allcall.com → 81.68.168.207（可选）
CNAME: www.allcall.com → api.allcall.com（可选）
```

---

## 📱 移动应用更新步骤

### 1. 更新配置文件
**编辑 `/mobile/src/config/cloud.config.ts`**：
```typescript
const ENV_CONFIG = {
  production: {
    HTTP: "https://api.allcall.com",  // 改为你的域名
    WS: "wss://api.allcall.com"        // WSS 用于安全 WebSocket
  }
};
```

### 2. 构建生产 APK
```bash
cd /Users/byzantium/github/allcall/mobile

# 使用 EAS Build（推荐）
eas build --platform android --release

# 或本地构建
expo build:android --release-channel production
```

### 3. 分发给用户
- 通过 App Store 或 Google Play 发布
- 或直接分享 APK 文件

---

## 🐛 常见问题和解决方案

### 问题 1：WebSocket 连接失败 (401)
**症状**：应用无法连接到信令服务器

**解决**：
```bash
# 1. 验证 token 有效性
curl -H "Authorization: Bearer $TOKEN" https://api.allcall.com/api/v1/users/me

# 2. 查看后端日志
docker-compose logs backend | grep -i websocket

# 3. 检查应用是否成功登陆
# 在 AuthContext 中查看 token 是否存在
```

### 问题 2：HTTPS 证书错误
**症状**：`curl: (60) SSL certificate problem`

**解决**：
```bash
# 查看证书有效期
openssl x509 -in /etc/letsencrypt/live/api.allcall.com/fullchain.pem -text -noout

# 手动续期
sudo certbot renew --force-renewal

# 重启 Nginx
docker-compose restart nginx
```

### 问题 3：数据库连接失败
**症状**：`Error: connect ECONNREFUSED`

**解决**：
```bash
# 检查 MySQL 容器
docker-compose ps mysql

# 查看 MySQL 日志
docker-compose logs mysql

# 重启 MySQL
docker-compose restart mysql
```

### 问题 4：后端服务无法启动
**症状**：`docker-compose ps` 显示 backend 为 Exited

**解决**：
```bash
# 查看详细错误
docker-compose logs backend --tail=50

# 检查 .env 文件配置
cat /opt/allcall/.env

# 重新构建镜像
docker-compose build --no-cache backend
docker-compose up -d backend
```

---

## 📊 性能优化建议

### 数据库优化
```sql
-- 增加连接数
SET GLOBAL max_connections = 1000;

-- 优化 InnoDB 缓冲池
SET GLOBAL innodb_buffer_pool_size = 2147483648; -- 2GB

-- 添加索引
CREATE INDEX idx_user_email ON users(email);
CREATE INDEX idx_room_code ON rooms(room_code);
```

### Redis 优化
```bash
# 增加最大内存
maxmemory 2gb
maxmemory-policy allkeys-lru

# 启用持久化
save 60 1000  # 60s 内 1000 个更改则保存
```

### Nginx 优化
```nginx
# 增加 worker 连接数
events {
    worker_connections 4096;
}

# 启用 HTTP/2
listen 443 ssl http2;

# 启用压缩
gzip on;
gzip_min_length 10240;
```

---

## 📈 监控和维护

### 日常监控命令
```bash
# 查看容器状态和资源使用
docker stats

# 查看服务器资源
htop

# 查看磁盘使用
df -h
du -sh /opt/allcall

# 查看实时日志
docker-compose logs -f backend --tail=100
```

### 定期维护任务

| 频率 | 任务 | 命令 |
|------|------|------|
| 每日 | 检查日志错误 | `docker-compose logs \| grep ERROR` |
| 每周 | 备份数据库 | `docker-compose exec mysql mysqldump ...` |
| 每月 | 更新依赖 | `docker-compose pull && docker-compose up -d` |
| 每月 | 检查磁盘空间 | `df -h` |
| 每 3 个月 | SSL 证书检查 | `openssl x509 -in ... -text -noout` |

### 数据库备份脚本
```bash
#!/bin/bash
# backup.sh
BACKUP_DIR="/opt/allcall/backups"
mkdir -p "$BACKUP_DIR"
DATE=$(date +%Y%m%d_%H%M%S)

# MySQL 备份
docker-compose exec -T mysql mysqldump -uroot -p$MYSQL_ROOT_PASSWORD allcallall_db > \
  "$BACKUP_DIR/db_backup_$DATE.sql"

# 压缩备份
gzip "$BACKUP_DIR/db_backup_$DATE.sql"

# 删除 7 天前的备份
find "$BACKUP_DIR" -name "db_backup_*.sql.gz" -mtime +7 -delete

echo "✓ 备份完成: $BACKUP_DIR/db_backup_$DATE.sql.gz"
```

---

## 🔒 安全最佳实践

### 1. 定期更新
```bash
# 更新容器镜像
docker-compose pull
docker-compose up -d

# 更新系统
sudo apt update && sudo apt upgrade -y
```

### 2. 访问控制
```bash
# 限制 SSH 访问
sudo ufw allow from 203.0.113.0/24 to any port 22

# 禁用密码认证（使用密钥）
sudo nano /etc/ssh/sshd_config
# PasswordAuthentication no
```

### 3. 日志监控
```bash
# 查看认证失败
docker-compose logs backend | grep "401\|Unauthorized"

# 查看异常请求
docker-compose logs nginx | grep "error"
```

### 4. 备份策略
- 每日备份数据库
- 每周异地备份
- 定期测试恢复流程

---

## 📚 部署文件清单

已创建以下文件供你参考：

| 文件 | 用途 |
|------|------|
| `DEPLOYMENT_GUIDE.md` | 完整部署指南（632 行） |
| `DEPLOYMENT_QUICK_REFERENCE.md` | 快速参考清单（415 行） |
| `scripts/deploy-cloud.sh` | 自动部署脚本（331 行） |
| `scripts/init-cloud-deployment.sh` | 初始化脚本（154 行） |
| `mobile/src/config/cloud.config.ts` | 移动应用配置（92 行） |

---

## 🎓 学习资源

### 推荐阅读
- Docker 官方文档：https://docs.docker.com/
- Docker Compose 指南：https://docs.docker.com/compose/
- Let's Encrypt 指南：https://letsencrypt.org/docs/
- Nginx 反向代理：https://nginx.org/en/docs/
- WebRTC 最佳实践：https://webrtc.org/

### 社区资源
- Docker Hub：https://hub.docker.com/
- Stack Overflow：https://stackoverflow.com/ (tag: docker, kubernetes)
- GitHub Discussions：项目讨论区

---

## ✅ 部署完成检查表

在开始部署前，请确保完成以下准备：

- [ ] 云服务器已购买和配置
- [ ] SSH 访问已设置
- [ ] 域名已购买（可选但推荐）
- [ ] 域名 DNS 已指向服务器 IP
- [ ] 邮箱授权码已获取（QQ 邮箱）
- [ ] 已阅读完整部署指南
- [ ] 已准备好所有密码和密钥

部署后的验证：

- [ ] 后端服务正常运行
- [ ] MySQL 和 Redis 正常运行
- [ ] HTTPS 证书已安装
- [ ] 防火墙规则已配置
- [ ] 移动应用配置已更新
- [ ] API 接口可正常访问
- [ ] WebSocket 连接成功
- [ ] 端到端通话正常

---

## 📞 支持和反馈

如遇到问题：

1. **查看日志**：`docker-compose logs -f backend`
2. **查看部署指南**：`DEPLOYMENT_GUIDE.md`
3. **查看快速参考**：`DEPLOYMENT_QUICK_REFERENCE.md`
4. **GitHub Issues**：提交问题报告
5. **社区讨论**：GitHub Discussions

---

**祝部署成功！** 🎉

**记住**：
- ✅ 所有密码必须强且唯一
- ✅ 定期备份数据库
- ✅ 定期更新依赖
- ✅ 监控日志和性能
- ✅ 保护敏感信息

