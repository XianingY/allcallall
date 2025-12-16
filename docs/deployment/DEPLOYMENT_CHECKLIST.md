# AllCallAll 云部署 - 部署前检查清单

## ✅ 预部署检查 (Pre-Deployment Checklist)

### 云服务器准备

- [ ] **云服务器已购买**
  - 操作系统: Ubuntu 20.04 LTS 或更新
  - CPU: 2+ 核心（推荐 4 核）
  - 内存: 4GB+（推荐 8GB）
  - 存储: 20GB+（推荐 50GB）
  - 公网 IP: 81.68.168.207

- [ ] **SSH 访问已配置**
  - 可以正常 SSH 连接: `ssh -i key.pem ubuntu@81.68.168.207`
  - 已配置公钥认证

- [ ] **安全组规则已开放**
  - Port 22 (SSH) - 允许
  - Port 80 (HTTP) - 允许
  - Port 443 (HTTPS) - 允许

### 域名和 DNS

- [ ] **域名已购买（可选但推荐）**
  - 域名注册商: _______________
  - 域名: _______________
  - 注册邮箱: _______________

- [ ] **DNS 记录已配置**
  - A 记录: api.allcall.com → 81.68.168.207
  - A 记录: allcall.com → 81.68.168.207（可选）
  - CNAME: www.allcall.com → api.allcall.com（可选）
  - DNS 生效时间: 通常需要 24-48 小时

- [ ] **DNS 解析已验证**
  ```bash
  nslookup api.allcall.com
  # 应该显示: 81.68.168.207
  ```

### 邮箱和通知

- [ ] **QQ 邮箱授权码已获取**
  - QQ 邮箱: _______________
  - 授权码: _______________（将填入 .env 文件）
  - 获取方式: QQ邮箱 → 设置 → 账户安全 → SMTP 服务

- [ ] **其他邮箱配置（如需要）**
  - SMTP 服务器: _______________
  - SMTP 端口: _______________
  - 发件人邮箱: _______________
  - 授权码/密码: _______________

### 项目代码

- [ ] **项目代码已提交到 Git**
  ```bash
  cd /Users/byzantium/github/allcall
  git status  # 应该显示 "nothing to commit"
  ```

- [ ] **所有本地修改已保存**
  - 是否有未提交的代码? 
  - 是否有敏感信息被提交? 
  - .env 和密钥文件是否在 .gitignore 中?

---

## 📋 部署执行清单 (Deployment Execution)

### 第 1 步：连接到服务器

- [ ] **SSH 连接成功**
  ```bash
  ssh -i /path/to/key.pem ubuntu@81.68.168.207
  ```

- [ ] **验证服务器信息**
  ```bash
  uname -a
  lsb_release -a
  df -h
  free -h
  ```

### 第 2 步：运行部署脚本

- [ ] **克隆项目代码**
  ```bash
  cd /opt
  sudo mkdir -p /opt/allcall
  cd /opt/allcall
  git clone https://github.com/yourusername/allcall.git .
  ```

- [ ] **运行部署脚本**
  ```bash
  bash scripts/deploy-cloud.sh 81.68.168.207 api.allcall.com
  ```
  预计时间: 30-45 分钟

- [ ] **脚本执行成功**
  - Docker 已安装
  - Docker Compose 已安装
  - .env 文件已创建
  - Nginx 配置已创建
  - 防火墙已配置

### 第 3 步：配置环境变量

- [ ] **编辑 .env 文件**
  ```bash
  nano /opt/allcall/.env
  ```

- [ ] **修改所有密码**
  ```
  MYSQL_ROOT_PASSWORD=your_strong_password_here
  MYSQL_PASSWORD=your_strong_password_here
  REDIS_PASSWORD=your_strong_password_here
  JWT_SECRET=your_long_random_secret_here
  MAIL_PASSWORD=your_qq_email_auth_code
  ```

- [ ] **验证所有必需字段已填写**
  - MYSQL_ROOT_PASSWORD ✓
  - MYSQL_PASSWORD ✓
  - REDIS_PASSWORD ✓
  - JWT_SECRET ✓
  - MAIL_PASSWORD ✓
  - DOMAIN_NAME ✓（改为你的域名）

### 第 4 步：启动服务

- [ ] **启动所有容器**
  ```bash
  cd /opt/allcall/infra
  docker-compose up -d
  ```

- [ ] **等待服务启动**
  ```bash
  docker-compose ps
  # 所有容器应该显示 "Up"，大约需要 30-60 秒
  ```

- [ ] **检查服务健康状态**
  ```bash
  docker-compose logs backend | tail -20
  # 应该显示 "Server listening on :8080"
  ```

### 第 5 步：配置 HTTPS（如果使用域名）

- [ ] **验证 DNS 已解析**
  ```bash
  nslookup api.allcall.com
  # 应该显示: 81.68.168.207
  ```

- [ ] **安装 Certbot**
  ```bash
  sudo apt install -y certbot python3-certbot-nginx
  ```

- [ ] **申请 SSL 证书**
  ```bash
  sudo certbot certonly --standalone -d api.allcall.com
  ```

- [ ] **验证证书已创建**
  ```bash
  ls -la /etc/letsencrypt/live/api.allcall.com/
  # 应该显示: fullchain.pem privkey.pem
  ```

- [ ] **启用自动续期**
  ```bash
  sudo systemctl enable certbot.timer
  sudo systemctl start certbot.timer
  ```

### 第 6 步：验证后端服务

- [ ] **测试 HTTP 连接**
  ```bash
  curl http://81.68.168.207:8080/api/v1/health
  # 应该返回: {"status":"ok"} 或类似响应
  ```

- [ ] **测试 HTTPS 连接（如适用）**
  ```bash
  curl https://api.allcall.com/api/v1/health
  # 应该返回成功响应
  ```

- [ ] **测试用户注册**
  ```bash
  curl -X POST http://81.68.168.207:8080/api/v1/auth/register \
    -H "Content-Type: application/json" \
    -d '{"email":"test@example.com","password":"Test123456","display_name":"Test"}'
  # 应该返回: {"success":true,...}
  ```

### 第 7 步：更新移动应用配置

- [ ] **编辑云配置文件**
  ```bash
  nano /Users/byzantium/github/allcall/mobile/src/config/cloud.config.ts
  ```

- [ ] **更新生产环境端点**
  ```typescript
  production: {
    HTTP: "https://api.allcall.com",  // 改为你的域名或 IP
    WS: "wss://api.allcall.com"
  }
  ```

- [ ] **构建生产版本**
  ```bash
  cd /Users/byzantium/github/allcall/mobile
  eas build --platform android --release
  # 或: expo build:android --release-channel production
  ```

- [ ] **测试应用连接**
  - 在移动设备上安装生产版 APK
  - 打开应用，注册新账户
  - 验证是否能成功登陆
  - 检查日志是否显示 WebSocket 连接成功

---

## 🔐 安全检查 (Security Checks)

### 密码和密钥安全

- [ ] **所有默认密码已更改**
  - MySQL root 密码 ✓
  - MySQL 用户密码 ✓
  - Redis 密码 ✓
  - JWT Secret ✓

- [ ] **密码满足强度要求**
  - 长度 ≥ 16 字符
  - 包含大小写字母
  - 包含数字
  - 包含特殊字符
  ```bash
  # 使用以下命令生成强密码
  openssl rand -base64 32
  ```

- [ ] **.env 文件未提交到 Git**
  ```bash
  grep -n ".env" .gitignore
  # 应该显示 ".env" 在 .gitignore 中
  ```

- [ ] **敏感信息未在日志中显示**
  ```bash
  docker-compose logs backend | grep -i "password\|secret"
  # 不应该显示任何密码
  ```

### 防火墙和网络安全

- [ ] **防火墙规则已配置**
  ```bash
  sudo ufw status
  # 应该显示允许: 22, 80, 443
  ```

- [ ] **数据库端口未对外开放**
  ```bash
  sudo ufw status | grep 3306
  sudo ufw status | grep 6379
  # 应该不显示这些端口对外开放
  ```

- [ ] **SSH 仅允许密钥认证**
  ```bash
  grep "PasswordAuthentication" /etc/ssh/sshd_config
  # 应该显示: PasswordAuthentication no
  ```

### 数据库安全

- [ ] **MySQL 有特定用户（非 root）**
  ```bash
  docker-compose exec mysql mysql -uallcallall -p -e "SELECT USER();"
  # 应该显示: allcallall@%
  ```

- [ ] **数据库备份已计划**
  ```bash
  # 创建备份脚本
  cat > backup.sh << 'EOF'
  #!/bin/bash
  DATE=$(date +%Y%m%d_%H%M%S)
  docker-compose exec -T mysql mysqldump -uroot -p$MYSQL_ROOT_PASSWORD allcallall_db > backup_$DATE.sql
  gzip backup_$DATE.sql
  echo "✓ Backup: backup_$DATE.sql.gz"
  EOF
  chmod +x backup.sh
  ```

---

## 📊 性能和监控检查

### 服务器资源

- [ ] **服务器 CPU 使用率正常**
  ```bash
  top
  # 应该显示 < 50% CPU 使用率
  ```

- [ ] **内存使用率在合理范围**
  ```bash
  free -h
  # Available 内存应该 > 1GB
  ```

- [ ] **磁盘空间充足**
  ```bash
  df -h /opt/allcall
  # 应该显示 > 10GB 可用空间
  ```

### 容器性能

- [ ] **所有容器运行正常**
  ```bash
  docker stats
  # 查看 CPU 和内存使用情况
  ```

- [ ] **数据库性能良好**
  ```bash
  docker-compose exec mysql mysql -uroot -prootpass -e "SHOW VARIABLES LIKE 'max%';"
  ```

### 日志监控

- [ ] **后端日志无严重错误**
  ```bash
  docker-compose logs backend --tail=100 | grep -i "error\|fatal"
  # 不应该显示许多错误
  ```

- [ ] **Nginx 日志正常**
  ```bash
  docker-compose logs nginx | tail -20
  # 应该显示正常的请求日志
  ```

---

## 🧪 端到端测试 (End-to-End Testing)

### 后端 API 测试

- [ ] **用户注册成功**
  ```bash
  curl -X POST https://api.allcall.com/api/v1/auth/register \
    -H "Content-Type: application/json" \
    -d '{"email":"test@example.com","password":"Test123456","display_name":"Test"}'
  ```

- [ ] **用户登陆成功**
  ```bash
  curl -X POST https://api.allcall.com/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"email":"test@example.com","password":"Test123456"}'
  # 应该返回 token
  ```

- [ ] **获取用户信息成功**
  ```bash
  TOKEN="your_token_here"
  curl -H "Authorization: Bearer $TOKEN" https://api.allcall.com/api/v1/users/me
  ```

### 移动应用测试

- [ ] **应用可成功安装**
  - APK 下载无损坏
  - 安装过程顺利
  - 应用启动正常

- [ ] **应用可成功注册**
  - 输入邮箱和密码
  - 接收验证邮件
  - 验证码验证成功
  - 注册完成

- [ ] **应用可成功登陆**
  - 使用已注册账户登陆
  - 获取 JWT token
  - 进入主界面

- [ ] **WebSocket 连接成功**
  - 查看应用日志
  - 应该显示: "WebSocket 连接成功" 或 "Signaling connected"
  - 不应该显示 401 错误

- [ ] **通话功能正常**
  - 两个用户登陆
  - 一个用户发起通话
  - 另一个用户接收通话
  - 通话连接建立
  - 语音/视频传输正常
  - 通话结束正常

---

## 📝 部署记录

```
部署日期: ________________________
部署人: ________________________
服务器 IP: 81.68.168.207
域名: ________________________
部署时长: ________________________

问题和解决方案:
________________________________________________________________
________________________________________________________________

备注:
________________________________________________________________
________________________________________________________________
```

---

## ✅ 部署完成确认

- [ ] 所有前置检查已完成
- [ ] 所有部署步骤已执行
- [ ] 所有安全检查已通过
- [ ] 所有性能检查已通过
- [ ] 所有端到端测试已完成
- [ ] 生产环境已就绪

**部署状态**: ✅ 完成 / ⏳ 进行中 / ❌ 失败

**签字和日期**: ______________________

---

## 📞 如需帮助

1. 查看详细部署指南: `cat DEPLOYMENT_GUIDE.md`
2. 查看快速参考: `cat DEPLOYMENT_QUICK_REFERENCE.md`
3. 查看方案总结: `cat CLOUD_DEPLOYMENT_SUMMARY.md`
4. 查看服务日志: `docker-compose logs -f backend`
5. 提交 GitHub Issue

**祝部署成功！** 🎉

