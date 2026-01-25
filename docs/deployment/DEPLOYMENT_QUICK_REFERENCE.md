# AllCallAll 云部署 - 快速参考指南

## 📋 部署清单

### 前置准备
- [ ] 云服务器已购买（推荐 2+ 核 CPU，4GB+ 内存）
- [ ] SSH 密钥已配置
- [ ] 域名已购买（可选，但推荐）

### 服务器配置（30 分钟）
- [ ] 运行部署脚本：`bash deploy-cloud.sh 81.68.168.207 api.allcall.com`
- [ ] 编辑 `.env` 文件配置密钥
- [ ] 验证 Docker 和 Docker Compose 已安装

### 数据库和缓存（15 分钟）
- [ ] MySQL 容器已启动
- [ ] Redis 容器已启动
- [ ] 数据库初始化脚本已执行

### 后端服务（15 分钟）
- [ ] 后端服务已构建
- [ ] 后端服务已启动
- [ ] 健康检查端点可访问：`curl http://localhost:8080/api/v1/health`

### HTTPS 和 Nginx（20 分钟）
- [ ] SSL 证书已申请（Let's Encrypt）
- [ ] Nginx 配置已更新
- [ ] HTTPS 可正常访问：`curl https://api.allcall.com`
- [ ] HTTP 自动重定向到 HTTPS

### 移动应用配置（10 分钟）
- [ ] 更新 `cloud.config.ts` 中的域名或 IP
- [ ] 更新 API 配置文件指向云服务器
- [ ] 构建生产版 APK/IPA

### 防火墙和安全（10 分钟）
- [ ] UFW 防火墙已启用
- [ ] 允许必要端口（80, 443, 22）
- [ ] 关闭不必要的端口

### 验证和测试（20 分钟）
- [ ] 后端 API 可正常访问
- [ ] WebSocket 连接正常
- [ ] 移动应用可连接到云服务器
- [ ] 语音/视频通话功能正常

---

## 🚀 快速启动命令

### 1. 连接到服务器
```bash
ssh -i your-key.pem ubuntu@81.68.168.207
```

### 2. 运行部署脚本
```bash
cd /opt/allcallall
bash scripts/deploy-cloud.sh 81.68.168.207 api.allcall.com
```

### 3. 配置环境变量
```bash
nano /opt/allcallallall/.env
# 修改以下内容:
# MAIL_PASSWORD=your_qq_auth_code
# 其他密钥会自动生成
```

### 4. 启动所有服务
```bash
cd /opt/allcallallall/infra
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f backend
```

### 5. 申请 SSL 证书（如果使用域名）
```bash
sudo certbot certonly --standalone -d api.allcall.com

# 自动续期
sudo systemctl enable certbot.timer
sudo systemctl start certbot.timer
```

### 6. 验证服务
```bash
# 测试后端
curl http://81.68.168.207:8080/api/v1/health

# 测试 HTTPS
curl https://api.allcall.com/api/v1/health
```

---

## 🔐 关键密码和密钥管理

### 生成强密码
```bash
# 生成随机密钥
openssl rand -base64 32

# 保存到安全位置
echo "JWT_SECRET=your_generated_secret" > ~/.allcall_secrets
chmod 600 ~/.allcall_secrets
```

### 密钥位置
- JWT Secret: `.env` 文件
- MySQL 密码: `.env` 文件 + docker-compose 环境变量
- Redis 密码: `.env` 文件
- 邮箱授权码: `.env` 文件（手动设置）
- SSL 证书: `/etc/letsencrypt/live/api.allcall.com/`

---

## 🌐 网络配置

### 域名 DNS 配置示例
使用 Namecheap 或 GoDaddy 等域名注册商：

| 类型 | 子域名 | 值 | TTL |
|------|--------|-----|-----|
| A | @ | 81.68.168.207 | 3600 |
| A | api | 81.68.168.207 | 3600 |
| CNAME | www | api.allcall.com | 3600 |

### 验证 DNS
```bash
nslookup api.allcall.com
dig api.allcall.com
host api.allcall.com
```

---

## 📱 移动应用配置

### 更新 API 端点

编辑 `mobile/src/config/cloud.config.ts`：

```typescript
const ENV_CONFIG = {
  production: {
    HTTP: "https://api.allcall.com",  // 改为你的域名
    WS: "wss://api.allcall.com"
  }
};
```

### 构建生产版本

```bash
cd mobile

# 构建 APK（Android）
eas build --platform android --release

# 或本地构建
expo build:android

# 分发 APK 给用户
```

---

## 🐛 故障排查

### WebSocket 连接失败

**症状**: `Expected HTTP 101 response but was '401 Unauthorized'`

**排查步骤**:
```bash
# 1. 检查后端日志
docker-compose logs backend | grep -i websocket

# 2. 验证 token 有效性
TOKEN="your_jwt_token"
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/users/me

# 3. 检查中间件
curl -v http://localhost:8080/api/v1/ws?token=$TOKEN

# 4. 查看 Nginx 日志
docker logs your_nginx_container
```

### 后端服务无法启动

**症状**: `docker-compose ps` 显示 backend 状态为 `Exited`

**排查步骤**:
```bash
# 1. 查看详细日志
docker-compose logs backend --tail=50

# 2. 检查数据库连接
docker-compose exec mysql mysql -u allcallall -p allcallall_db -e "SELECT 1"

# 3. 检查 Redis 连接
docker-compose exec redis redis-cli ping

# 4. 查看配置文件
cat /opt/allcallallall/backend/configs/config.yaml
```

### 数据库连接错误

**症状**: `Error: connect ECONNREFUSED 127.0.0.1:3306`

**排查步骤**:
```bash
# 1. 检查 MySQL 容器状态
docker-compose ps mysql

# 2. 进入 MySQL 容器
docker-compose exec mysql bash

# 3. 查看用户权限
mysql -uroot -prootpass -e "SHOW GRANTS FOR 'allcallall'@'%';"

# 4. 重启 MySQL
docker-compose restart mysql
docker-compose logs mysql
```

### HTTPS 证书问题

**症状**: `curl: (60) SSL certificate problem`

**排查步骤**:
```bash
# 1. 查看证书有效期
openssl x509 -in /etc/letsencrypt/live/api.allcall.com/fullchain.pem -text -noout | grep "Not"

# 2. 手动续期
sudo certbot renew --dry-run

# 3. 强制续期
sudo certbot renew --force-renewal

# 4. 重启 Nginx
docker-compose restart nginx
```

### 防火墙阻止连接

**症状**: `Connection refused` 或 `Connection timeout`

**排查步骤**:
```bash
# 1. 查看防火墙规则
sudo ufw status

# 2. 检查端口是否开放
telnet 81.68.168.207 443
telnet 81.68.168.207 80

# 3. 添加缺失的规则
sudo ufw allow 443/tcp
sudo ufw allow 80/tcp

# 4. 重新加载防火墙
sudo ufw reload
```

---

## 📊 监控和日志

### 实时监控日志

```bash
# 后端日志
docker-compose logs -f backend --tail=100

# Nginx 日志
docker-compose logs -f nginx

# 所有服务日志
docker-compose logs -f
```

### 性能监控

```bash
# 查看容器资源使用
docker stats

# 查看服务器资源
htop

# 查看磁盘使用
df -h
du -sh /opt/allcallall
```

### 数据库备份

```bash
# 备份 MySQL
docker-compose exec mysql mysqldump -uroot -prootpass allcallall_db > backup.sql

# 恢复 MySQL
docker-compose exec -T mysql mysql -uroot -prootpass allcallall_db < backup.sql

# 备份 Redis
docker-compose exec redis redis-cli --rdb /data/dump.rdb
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

### 2. 监控日志
```bash
# 查看认证失败
docker-compose logs backend | grep "401\|Unauthorized"

# 查看异常请求
docker-compose logs nginx | grep "error"
```

### 3. 备份重要数据
```bash
# 定时备份脚本
0 2 * * * /opt/allcallallall/scripts/backup.sh

# 创建 backup.sh
#!/bin/bash
BACKUP_DIR="/opt/allcallallall/backups"
DATE=$(date +%Y%m%d_%H%M%S)
docker-compose exec -T mysql mysqldump -uroot -prootpass allcallall_db > "$BACKUP_DIR/backup_$DATE.sql"
```

### 4. 限制访问
```bash
# 只允许特定 IP 访问 SSH
sudo ufw allow from 203.0.113.0/24 to any port 22

# 禁止端口扫描
sudo ufw default deny incoming
```

---

## 📞 获取帮助

### 常用命令速查

```bash
# 查看所有服务状态
docker-compose ps

# 重启单个服务
docker-compose restart backend

# 重启所有服务
docker-compose restart

# 查看服务日志（最后 50 行）
docker-compose logs backend --tail=50

# 进入容器
docker-compose exec backend bash

# 查看环境变量
docker-compose config | grep -A 20 "backend:"

# 停止所有服务
docker-compose down

# 停止并删除数据
docker-compose down -v
```

### 联系信息

- 项目仓库: https://github.com/yourusername/allcall
- 问题报告: GitHub Issues
- 讨论: GitHub Discussions

---

## 📝 部署记录

部署日期: ___________
部署人: ___________
服务器 IP: 81.68.168.207
域名: ___________
备注: ___________

---

**祝部署顺利！** 🎉

