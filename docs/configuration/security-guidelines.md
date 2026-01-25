# AllCallAll 安全指南

## 📋 概述

本文档提供 AllCallAll 项目的安全最佳实践，确保应用在开发和生产环境中的安全性。

## 🔐 认证与授权

### 1. JWT Token 安全

#### Token 生成
- 使用强随机算法生成 JWT secret（建议 32+ 字符）
- 设置合理的 Token 过期时间
  - 访问 Token: 15-60 分钟
  - 刷新 Token: 7 天

#### Token 存储
- **客户端**: 使用安全的存储方式（iOS Keychain, Android Keystore）
- **避免**: localStorage（XSS 风险）
- **推荐**: HTTP-only Cookie 或安全存储

#### Token 传输
```http
# 开发环境
Authorization: Bearer <token>

# 生产环境（必须使用 HTTPS）
Authorization: Bearer <token>
```

### 2. 密码安全

#### 密码策略
- 最小长度: 8 字符
- 建议: 包含大小写字母、数字和特殊字符
- 禁止: 常见弱密码（123456, password 等）

#### 密码存储
使用 bcrypt 进行哈希存储：

```go
// Go 代码示例
import "golang.org/x/crypto/bcrypt"

hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
```

**安全特性**:
- 自动盐值生成
- 可配置成本因子（Cost）
- 计算密集型，抵御暴力破解

#### 密码验证
```go
// 验证密码
err := bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
if err != nil {
    // 密码错误
}
```

### 3. 访问控制

#### 角色定义
- **普通用户**: 基本功能访问权限
- **管理员**: 系统管理权限（暂未实现）

#### 权限检查
所有受保护的 API 都需要通过中间件验证：

```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if token == "" {
            c.JSON(401, gin.H{"error": "未授权"})
            c.Abort()
            return
        }

        // 验证 Token
        claims, err := ValidateToken(token)
        if err != nil {
            c.JSON(401, gin.H{"error": "无效的 Token"})
            c.Abort()
            return
        }

        // 将用户信息存储到上下文
        c.Set("user_id", claims.UserID)
        c.Set("user_email", claims.Email)
        c.Next()
    }
}
```

## 🛡️ 数据保护

### 1. 敏感数据加密

#### 数据库加密
- **密码**: 使用 bcrypt 哈希存储
- **JWT Secret**: 存储在环境变量或密钥管理服务
- **邮箱密码**: 使用环境变量或密钥管理服务

#### 传输加密
- **API**: 生产环境必须使用 HTTPS
- **WebSocket**: 生产环境必须使用 WSS
- **邮件**: 使用 STARTTLS 或 TLS

### 2. 数据脱敏

#### 日志脱敏
```go
// 敏感信息脱敏示例
func maskEmail(email string) string {
    parts := strings.Split(email, "@")
    if len(parts) != 2 {
        return email
    }
    username := parts[0]
    domain := parts[1]

    if len(username) > 2 {
        username = username[:2] + "***"
    }

    return username + "@" + domain
}
```

#### 日志配置
```yaml
logging:
  level: "info"
  # 避免记录敏感信息
```

### 3. 输入验证

#### 参数验证
所有 user 输入都需要验证：

```go
type registerRequest struct {
    Email       string `json:"email" binding:"required,email"`
    Password    string `json:"password" binding:"required,min=8"`
    DisplayName string `json:"display_name" binding:"required,min=2,max=100"`
}
```

#### SQL 注入防护
使用 GORM 的参数化查询：

```go
// ✅ 安全
db.Where("email = ?", email).First(&user)

// ❌ 不安全（不要这样做）
db.Raw("SELECT * FROM users WHERE email = '" + email + "'")
```

#### XSS 防护
- 对输出内容进行转义
- 使用 Content Security Policy (CSP)
- 避免使用 `innerHTML`

## 🔒 通信安全

### 1. HTTPS/WSS 配置

#### 证书管理
- 使用 Let's Encrypt 或商业 CA 签发的证书
- 定期更新证书（建议自动续期）
- 启用 HSTS

#### Nginx 配置示例
```nginx
server {
    listen 443 ssl http2;
    server_name allcallall.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    # 安全配置
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # HSTS
    add_header Strict-Transport-Security "max-age=31536000" always;

    # CSP
    add_header Content-Security-Policy "default-src 'self'" always;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 2. CORS 配置

#### 限制跨域访问
```go
func CorsMiddleware() gin.HandlerFunc {
    return cors.New(cors.Config{
        AllowOrigins:     []string{"https://allcallall.example.com"},
        AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
        AllowHeaders:     []string{"Origin", "Authorization", "Content-Type"},
        AllowCredentials: true,
        MaxAge:           12 * time.Hour,
    })
}
```

## 🚨 安全威胁防护

### 1. 暴力破解防护

#### 登录限制
```go
// 实现登录失败次数限制
func RateLimitLogin(email string) bool {
    // 检查失败次数
    failures, _ := redis.Get(ctx, "login_fail:"+email).Int()
    if failures >= 5 {
        return false // 锁定账户
    }
    return true
}
```

#### 验证码防护
- 邮箱验证码: 6 位数字
- 有效期: 10 分钟
- 验证后自动失效

### 2. WebRTC 安全

#### ICE 服务器安全
```yaml
webrtc:
  ice_servers:
    # 使用可信的 STUN 服务器
    - urls:
        - "stun:stun.l.google.com:19302"

    # 生产环境使用 TURN 服务器
    - urls: "turn:turn.example.com:3478"
      username: "${TURN_USERNAME}"
      credential: "${TURN_PASSWORD}"
```

#### DTLS 加密
WebRTC 媒体流自动使用 DTLS 加密，确保通信安全。

### 3. WebSocket 安全

#### 认证检查
```go
func (h *Hub) Handle(c *gin.Context) {
    // 1. 检查 Token
    token := c.Query("token")
    if token == "" {
        c.JSON(401, gin.H{"error": "缺少认证 Token"})
        return
    }

    // 2. 验证 Token
    claims, err := ValidateToken(token)
    if err != nil {
        c.JSON(401, gin.H{"error": "无效的 Token"})
        return
    }

    // 3. 升级为 WebSocket
    conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
    if err != nil {
        return
    }

    // 4. 创建客户端
    client := &Client{
        hub:    h,
        conn:   conn,
        userID: claims.UserID,
        email:  claims.Email,
    }
}
```

## 🔍 安全监控

### 1. 日志记录

#### 关键事件日志
- 用户登录/登出
- 密码修改
- 权限变更
- 异常访问

#### 日志格式
```go
logger.Info().
    Str("event", "user_login").
    Str("email", userEmail).
    Str("ip", clientIP).
    Int("user_id", userID).
    Msg("用户登录")
```

### 2. 异常检测

#### 监控指标
- 登录失败率
- API 错误率
- 异常 IP 访问
- 大量请求来源

#### 告警设置
```yaml
# 监控配置示例
alerts:
  - name: "高登录失败率"
    condition: "login_failures > 100 in 5m"
    action: "send_email"

  - name: "异常 IP 访问"
    condition: "requests_from_same_ip > 1000 in 1m"
    action: "block_ip"
```

## 🔐 部署安全

### 1. 环境安全

#### 生产环境清单
- [ ] 使用 HTTPS/WSS
- [ ] 修改默认密码
- [ ] 配置防火墙规则
- [ ] 禁用不必要的服务
- [ ] 定期更新依赖
- [ ] 启用日志审计
- [ ] 配置备份策略
- [ ] 设置监控告警

### 2. 服务器安全

#### 防火墙配置
```bash
# 只允许必要端口
ufw allow 22    # SSH
ufw allow 80    # HTTP
ufw allow 443   # HTTPS
ufw enable
```

#### SSH 安全
```bash
# 禁用密码登录
PasswordAuthentication no

# 使用密钥登录
PubkeyAuthentication yes

# 禁用 root 登录
PermitRootLogin no
```

### 3. 数据库安全

#### MySQL 安全
```sql
-- 删除测试数据库
DROP DATABASE IF EXISTS test;

-- 删除匿名用户
DELETE FROM mysql.user WHERE User='';

-- 限制远程访问
DELETE FROM mysql.user WHERE Host NOT IN ('localhost', '127.0.0.1', '::1');

-- 刷新权限
FLUSH PRIVILEGES;
```

#### Redis 安全
```conf
# 设置密码
requirepass ${REDIS_PASSWORD}

# 禁用危险命令
rename-command FLUSHDB ""
rename-command FLUSHALL ""
rename-command KEYS ""

# 绑定内网 IP
bind 127.0.0.1 10.0.0.0/8
```

## 🧪 安全测试

### 1. 渗透测试

#### 测试范围
- [ ] 认证绕过
- [ ] 权限提升
- [ ] SQL 注入
- [ ] XSS 攻击
- [ ] CSRF 攻击
- [ ] 文件上传漏洞
- [ ] 敏感信息泄露

#### 测试工具
- **OWASP ZAP**: Web 应用安全扫描
- **Burp Suite**: 渗透测试平台
- **Nmap**: 端口扫描
- **SQLMap**: SQL 注入检测

### 2. 代码审计

#### 安全审计清单
- [ ] 输入验证
- [ ] 输出编码
- [ ] 认证机制
- [ ] 会话管理
- [ ] 错误处理
- [ ] 日志记录

## 🚨 安全威胁防护

### 1. 安全事件分类

#### 事件级别
- **P0 - 严重**: 数据泄露、系统入侵
- **P1 - 高危**: 权限绕过、服务中断
- **P2 - 中危**: 信息泄露、拒绝服务
- **P3 - 低危**: 安全配置错误

#### 响应流程
```
1. 检测和识别
   ↓
2. 事件分类和评估
   ↓
3. 遏制和隔离
   ↓
4. 根除和恢复
   ↓
5. 总结和预防
```

### 2. 数据泄露应对

#### 立即行动
1. 确认泄露范围
2. 隔离受影响系统
3. 保留证据日志
4. 通知相关方

#### 后续行动
1. 修复漏洞
2. 更新安全策略
3. 用户通知
4. 合规报告

## 📚 安全资源

### 推荐阅读
- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [Web 安全攻防战](https://book.douban.com/subject/25944135/)
- [白帽子讲 Web 安全](https://book.douban.com/subject/10593425/)

### 安全工具
- **静态分析**: GoSec, SonarQube
- **依赖扫描**: Dependabot, Snyk
- **容器安全**: Trivy, Clair

## ✅ 安全检查清单

### 开发阶段
- [ ] 使用参数化查询
- [ ] 验证所有输入
- [ ] 加密敏感数据
- [ ] 安全的错误处理
- [ ] 安全的日志记录

### 测试阶段
- [ ] 安全功能测试
- [ ] 渗透测试
- [ ] 代码审计
- [ ] 依赖漏洞扫描

### 部署阶段
- [ ] HTTPS/WSS 配置
- [ ] 防火墙规则
- [ ] 安全监控
- [ ] 备份策略
- [ ] 访问控制

### 运维阶段
- [ ] 定期安全更新
- [ ] 日志审计
- [ ] 漏洞扫描
- [ ] 安全培训
- [ ] 应急响应演练

## 📝 相关文档

- [API 文档](../api/api-documentation.md)
- [配置说明](./configuration.md)
- [数据库文档](../api/database.md)
- [部署指南](../deployment/deployment-guide.md)

---

**注意**: 安全是一个持续的过程，需要定期评估和改进。请根据实际情况调整安全策略。
