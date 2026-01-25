# AllCallAll 配置说明

## 📋 概述

AllCallAll 使用 YAML 格式的配置文件来管理所有系统参数。配置文件位于 `backend/configs/config.yaml`。

## 📁 配置文件位置

- **开发环境**: `backend/configs/config.yaml`
- **生产环境**: `backend/configs/config.production.yaml`
- **环境变量**: 可通过环境变量覆盖配置项（如 `${MAIL_PASSWORD}`）

## ⚙️ 配置项详解

### 1. 服务器配置 (server)

```yaml
server:
  host: "0.0.0.0"              # 监听地址
  port: 8080                   # 监听端口
  read_timeout_seconds: 10     # 读取超时时间（秒）
  write_timeout_seconds: 15    # 写入超时时间（秒）
  idle_timeout_seconds: 60     # 空闲超时时间（秒）
```

**说明**:
- `host`: 设置为 `"0.0.0.0"` 可监听所有网络接口
- `port`: HTTP 服务端口，默认为 8080
- `read_timeout`: 读取请求体的超时时间
- `write_timeout`: 响应写入的超时时间
- `idle_timeout`: 连接空闲超时时间

**生产环境建议**:
```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout_seconds: 30
  write_timeout_seconds: 30
  idle_timeout_seconds: 120
```

### 2. 数据库配置 (database)

```yaml
database:
  dsn: "allcallall:allcallallpass@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local"
  max_open_conns: 25           # 最大打开连接数
  max_idle_conns: 10           # 最大空闲连接数
  conn_max_lifetime_minutes: 30 # 连接最大生存时间（分钟）
```

**DSN 格式**:
```
[username[:password]@][protocol[(address)]]/dbname[?param1=value1&...&paramN=valueN]
```

**参数说明**:
- `parseTime=true`: 自动解析时间字段
- `charset=utf8mb4`: 使用 UTF-8 编码，支持 Emoji
- `loc=Local`: 使用本地时区

**生产环境示例**:
```yaml
database:
  dsn: "prod_user:${DB_PASSWORD}@tcp(db.example.com:3306)/allcallall_prod?parseTime=true&charset=utf8mb4&loc=Local"
  max_open_conns: 100
  max_idle_conns: 20
  conn_max_lifetime_minutes: 60
```

**性能调优建议**:
- **高并发场景**: 增加 `max_open_conns` 到 100-200
- **内存优化**: 适当降低 `max_idle_conns`
- **连接池**: 设置合理的 `conn_max_lifetime` 避免连接泄漏

### 3. Redis 配置 (redis)

```yaml
redis:
  addr: "localhost:6379"       # Redis 地址
  username: ""                 # 用户名（可选）
  password: ""                 # 密码（可选）
  db: 0                        # 数据库编号
```

**用途**:
- 会话存储
- 用户在线状态管理
- 缓存加速
- WebSocket 连接管理

**生产环境示例**:
```yaml
redis:
  addr: "redis.example.com:6379"
  username: "allcallall"
  password: "${REDIS_PASSWORD}"
  db: 0
```

### 4. 邮件配置 (mail)

```yaml
mail:
  host: "smtp.qq.com"          # SMTP 服务器
  port: 587                    # SMTP 端口（587 为 STARTTLS）
  username: "1569297330@qq.com" # 发送者邮箱
  password: "${MAIL_PASSWORD}"  # 邮箱授权码（从环境变量读取）
  from: "1569297330@qq.com"    # 发件人地址
  from_name: "AllCallAll"      # 发件人名称
  max_retries: 3               # 最大重试次数
  retry_delay_seconds: 5       # 重试间隔（秒）
```

**支持的邮箱服务商**:

#### QQ 邮箱
```yaml
mail:
  host: "smtp.qq.com"
  port: 587
  username: "your_email@qq.com"
  password: "${QQ_MAIL_AUTH_CODE}"  # QQ 邮箱授权码
```

#### 163 邮箱
```yaml
mail:
  host: "smtp.163.com"
  port: 587
  username: "your_email@163.com"
  password: "${163_MAIL_AUTH_CODE}"
```

#### Gmail
```yaml
mail:
  host: "smtp.gmail.com"
  port: 587
  username: "your_email@gmail.com"
  password: "${GMAIL_APP_PASSWORD}"
```

**获取邮箱授权码**:
- **QQ 邮箱**: 设置 → 账户 → 开启 "SMTP/IMAP服务" → 获取授权码
- **163 邮箱**: 设置 → POP3/SMTP/IMAP → 开启服务 → 获取授权码
- **Gmail**: 账户 → 安全性 → 两步验证 → 应用专用密码

### 5. JWT 配置 (jwt)

```yaml
jwt:
  secret: "please_change_me"                    # 签名密钥（必须修改）
  issuer: "allcallall-backend"                  # 签发者
  access_token_ttl_minutes: 60                  # 访问令牌有效期（分钟）
  refresh_token_ttl_hours: 168                  # 刷新令牌有效期（小时）
```

**安全建议**:
- `secret`: 使用强随机字符串（建议 32+ 字符）
- `access_token_ttl`: 开发环境可设置较长，生产环境建议 15-60 分钟
- `refresh_token_ttl`: 建议 7 天（168 小时）

**生成强密钥**:
```bash
# 使用 openssl 生成
openssl rand -base64 32

# 使用 go 生成
go run -c 'import "crypto/rand"; b := make([]byte, 32); rand.Read(b); println(string(b))'
```

**生产环境示例**:
```yaml
jwt:
  secret: "${JWT_SECRET}"  # 从环境变量读取
  issuer: "allcallall-backend"
  access_token_ttl_minutes: 30
  refresh_token_ttl_hours: 168
```

### 6. WebRTC 配置 (webrtc)

```yaml
webrtc:
  ice_servers:
    - urls:
        - "stun:stun.l.google.com:19302"
```

**ICE 服务器说明**:
ICE（Interactive Connectivity Establishment）服务器用于 NAT 穿透，帮助建立 WebRTC 连接。

**STUN 服务器**（免费）:
```yaml
webrtc:
  ice_servers:
    - urls:
        - "stun:stun.l.google.com:19302"
        - "stun:stun1.l.google.com:19302"
        - "stun:stun2.l.google.com:19302"
```

**TUN 服务器**（付费，更可靠）:
```yaml
webrtc:
  ice_servers:
    - urls: "turn:turn.example.com:3478"
      username: "turn_user"
      credential: "turn_password"
```

**综合配置**:
```yaml
webrtc:
  ice_servers:
    # STUN 服务器
    - urls:
        - "stun:stun.l.google.com:19302"
        - "stun:stun1.l.google.com:19302"

    # TUN 服务器（生产环境推荐）
    - urls: "turn:turn.example.com:3478"
      username: "${TURN_USERNAME}"
      credential: "${TURN_PASSWORD}"
```

**常用 STUN 服务器**:
- `stun:stun.l.google.com:19302` - Google
- `stun:stun1.l.google.com:19302` - Google
- `stun:stun.ekiga.net` - Ekiga
- `stun:stun.ideasip.com` - Ideas
- `stun:stun.rixtelecom.se` - Rixtelecom
- `stun:stun.schlund.de` - Schlund

### 8. FCM 配置 (fcm) - 可选

> ⚠️ **功能状态**: 即将推出 - 待 Firebase Admin SDK 集成

```yaml
fcm:
  service_account_path: "${FCM_SERVICE_ACCOUNT_PATH}"
  enabled: true
```

**参数说明**:
- `service_account_path`: Firebase 服务账号 JSON 文件绝对路径
- `enabled`: 是否启用 FCM (未配置 service_account_path 时自动禁用)

**环境变量**:
```bash
export FCM_SERVICE_ACCOUNT_PATH="/opt/allcall/secrets/firebase-key.json"
```

**当前状态**:
后端 FCM 模块 (`backend/internal/fcm/manager.go`) 已就位，日志将显示:
```
call notification would be sent (Firebase SDK not yet configured)
```

### 9. 信令配置 (signaling)

```yaml
signaling:
  ws_ping_interval: 30s       # WebSocket ping 间隔
  ws_pong_wait: 60s           # 等待 pong 响应时间
  poll_timeout_ms: 25000      # HTTP long-poll 超时 (毫秒)
```

**后端信令端点**:
| 端点 | 方法 | 说明 |
|-----|------|------|
| `/api/v1/ws?token=...` | GET | WebSocket 连接 (主通道) |
| `/api/v1/signaling/send` | POST | 发送信令消息 (备用) |
| `/api/v1/signaling/poll?timeout_ms=...` | GET | 轮询信令消息 (备用) |

> 📌 HTTP Poll 端点需要 JWT 认证 (`Authorization: Bearer <token>`)

### 10. 日志配置 (logging)

```yaml
logging:
  level: "info"  # debug | info | warn | error
```

**日志级别**:
- `debug`: 详细的调试信息
- `info`: 一般信息（推荐生产环境使用）
- `warn`: 警告信息
- `error`: 错误信息

**生产环境建议**:
```yaml
logging:
  level: "info"
```

**开发环境**:
```yaml
logging:
  level: "debug"
```

## 🔧 环境变量覆盖

### 后端环境变量

```bash
# 数据库
export DB_DSN="user:pass@tcp(host:3306)/db"
export MYSQL_ROOT_PASSWORD="..."
export MYSQL_PASSWORD="..."

# Redis
export REDIS_ADDR="localhost:6379"
export REDIS_PASSWORD="..."

# 认证
export JWT_SECRET="..."
export MAIL_PASSWORD="..."

# WebRTC
export TURN_USERNAME="..."
export TURN_PASSWORD="..."
export WEBRTC_ICE_SERVERS_JSON='[...]'

# FCM 推送 (新增)
export FCM_SERVICE_ACCOUNT_PATH="/path/to/firebase-key.json"

# WebSocket (新增)
export WS_PING_INTERVAL="30s"
export WS_PONG_WAIT="60s"
```

### 移动端环境变量 (EXPO_PUBLIC_*)

```bash
# API 配置
EXPO_PUBLIC_API_HTTP="https://api.example.com"
EXPO_PUBLIC_API_WS="wss://api.example.com"
EXPO_PUBLIC_FORCE_TLS="1"

# 受限网络配置
EXPO_PUBLIC_RESTRICTED_NETWORK="1"
EXPO_PUBLIC_SIGNALING_TRANSPORT="auto"  # auto | poll
EXPO_PUBLIC_SIGNALING_SHAPING="1"
```

### 移动端环境变量完整参考

| 变量名 | 默认值 | 说明 |
|-------|-------|------|
| `EXPO_PUBLIC_API_HTTP` | (config) | API 基础地址 |
| `EXPO_PUBLIC_API_WS` | (config) | WebSocket 基础地址 |
| `EXPO_PUBLIC_FORCE_TLS` | `0` | `1` = 强制 HTTPS/WSS |
| `EXPO_PUBLIC_RESTRICTED_NETWORK` | `0` | `1` = 优先 TURNS on 443 |
| `EXPO_PUBLIC_SIGNALING_TRANSPORT` | `auto` | `auto` 或 `poll` |
| `EXPO_PUBLIC_SIGNALING_SHAPING` | `0` | `1` = 启用 WS keepalive |

## 📦 多环境配置

### 开发环境 (config.yaml)
```yaml
# 使用本地服务，调试友好
server:
  port: 8080
  read_timeout_seconds: 10
  write_timeout_seconds: 15

database:
  dsn: "allcallall:allcallallpass@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local"

redis:
  addr: "localhost:6379"

mail:
  password: "${MAIL_PASSWORD}"

jwt:
  secret: "dev_secret_change_in_production"
  access_token_ttl_minutes: 1440  # 24 小时

logging:
  level: "debug"
```

### 生产环境 (config.production.yaml)
```yaml
# 生产优化配置
server:
  port: 8080
  read_timeout_seconds: 30
  write_timeout_seconds: 30
  idle_timeout_seconds: 120

database:
  dsn: "prod_user:${DB_PASSWORD}@tcp(prod-db:3306)/allcallall_prod?parseTime=true&charset=utf8mb4&loc=Local"
  max_open_conns: 100
  max_idle_conns: 20
  conn_max_lifetime_minutes: 60

redis:
  addr: "prod-redis:6379"
  password: "${REDIS_PASSWORD}"

mail:
  password: "${MAIL_PASSWORD}"

jwt:
  secret: "${JWT_SECRET}"
  access_token_ttl_minutes: 30

webrtc:
  ice_servers:
    - urls: "turn:turn.example.com:3478"
      username: "${TURN_USERNAME}"
      credential: "${TURN_PASSWORD}"

logging:
  level: "info"
```

## 🚀 启动时指定配置

```bash
# 使用默认配置
go run cmd/server/main.go

# 指定配置文件
CONFIG_PATH=./configs/config.production.yaml go run cmd/server/main.go

# 或使用环境变量
export CONFIG_PATH=./configs/config.production.yaml
go run cmd/server/main.go
```

## ✅ 配置验证

启动时会自动验证配置：

1. **必填项检查**: 确保所有必需字段都已配置
2. **格式验证**: 验证邮箱格式、URL 格式等
3. **连接测试**: 测试数据库、Redis 连接
4. **默认值**: 为可选字段应用默认值

## 🔍 常见配置问题

### 1. 数据库连接失败
```
Error: mysql: connection refused
```
**解决方案**:
- 检查 MySQL 服务是否启动
- 验证 DSN 中的主机、端口、用户名、密码
- 确认数据库存在

### 2. Redis 连接失败
```
Error: redis: connection refused
```
**解决方案**:
- 检查 Redis 服务是否启动
- 验证 `redis.addr` 配置
- 确认密码正确（如果设置了密码）

### 3. 邮件发送失败
```
Error: SMTP: authentication failed
```
**解决方案**:
- 确认使用的是授权码而非邮箱密码
- 检查 SMTP 配置（host、port）
- 确认已开启 SMTP 服务

### 4. JWT Token 无效
```
Error: invalid token
```
**解决方案**:
- 检查 `jwt.secret` 是否设置
- 确认 Token 未过期
- 验证 Token 格式正确

### 5. WebRTC 连接失败
```
Error: ICE connection failed
```
**解决方案**:
- 检查 ICE 服务器配置
- 确认防火墙允许 UDP 流量
- 考虑使用 TUN 服务器提高连接率

## 📊 性能调优

### 高并发场景
```yaml
database:
  max_open_conns: 200      # 增加连接池大小
  max_idle_conns: 50
  conn_max_lifetime_minutes: 30

server:
  read_timeout_seconds: 30
  write_timeout_seconds: 30

redis:
  addr: "redis-cluster:6379"  # 使用 Redis 集群
```

### 低内存场景
```yaml
database:
  max_open_conns: 10       # 减少连接数
  max_idle_conns: 5
  conn_max_lifetime_minutes: 15

logging:
  level: "warn"            # 减少日志输出
```

## 🔐 安全配置

### 生产环境安全清单
- [ ] 修改默认密码
- [ ] 使用强 JWT secret
- [ ] 启用 TLS/HTTPS
- [ ] 配置防火墙规则
- [ ] 限制数据库访问权限
- [ ] 使用专用邮箱账户
- [ ] 配置 CORS 策略
- [ ] 启用日志审计
- [ ] 定期更新依赖
- [ ] 配置监控和告警

## 📝 配置模板

完整的配置模板请参考 `backend/configs/config.yaml`。

## 🔗 相关文档

- [API 文档](./API_DOCUMENTATION.md)
- [数据库文档](./DATABASE.md)
- [部署指南](./DEPLOYMENT_GUIDE.md)
- [安全指南](./SECURITY_GUIDELINES.md)
