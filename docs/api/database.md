# AllCallAll 数据库文档

## 📋 概述

AllCallAll 使用 MySQL 8.0 作为主数据库，通过 GORM 进行 ORM 映射。数据库支持自动迁移，会在应用启动时自动创建表结构。

## 🗄️ 数据库信息

- **数据库类型**: MySQL 8.0+
- **ORM 框架**: GORM v1.25.8
- **字符集**: UTF8MB4（支持 Emoji）
- **时区**: 本地时区
- **连接池**: 支持连接池配置

## 📊 数据库连接配置

### DSN 格式

```
[username[:password]@][protocol[(address)]]/dbname[?param1=value1&...&paramN=valueN]
```

### 示例配置

```yaml
database:
  dsn: "allcallall:allcallallpass@tcp(localhost:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local"
  max_open_conns: 25
  max_idle_conns: 10
  conn_max_lifetime_minutes: 30
```

## 📋 数据表结构

### 1. users (用户表)

存储所有用户的基本信息。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | BIGINT | PRIMARY KEY, AUTO_INCREMENT | 用户唯一标识 |
| email | VARCHAR(255) | UNIQUE, NOT NULL | 邮箱地址（登录凭证） |
| password_hash | VARCHAR(255) | NOT NULL | 密码哈希值 |
| display_name | VARCHAR(100) | NOT NULL | 显示名称 |
| avatar_url | VARCHAR(500) | NULL | 头像 URL |
| status | VARCHAR(20) | DEFAULT 'active' | 账户状态: active, inactive, banned |
| email_verified | BOOLEAN | DEFAULT false | 邮箱是否已验证 |
| created_at | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP | 更新时间 |
| last_seen_at | TIMESTAMP | NULL | 最后活跃时间 |

**索引**:
- `PRIMARY KEY (id)`
- `UNIQUE INDEX idx_users_email (email)`
- `INDEX idx_users_status (status)`
- `INDEX idx_users_created_at (created_at)`

**示例数据**:
```sql
INSERT INTO users (email, password_hash, display_name, email_verified)
VALUES ('user@example.com', '$2a$10$...', '张三', true);
```

### 2. contacts (联系人表)

存储用户之间的联系人关系（好友关系）。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | BIGINT | PRIMARY KEY, AUTO_INCREMENT | 联系人关系唯一标识 |
| user_id | BIGINT | NOT NULL, FOREIGN KEY | 用户 ID |
| contact_user_id | BIGINT | NOT NULL, FOREIGN KEY | 联系人用户 ID |
| status | VARCHAR(20) | DEFAULT 'pending' | 关系状态: pending, accepted, blocked |
| created_at | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | 创建时间 |
| updated_at | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP | 更新时间 |

**索引**:
- `PRIMARY KEY (id)`
- `UNIQUE INDEX idx_contacts_user_contact (user_id, contact_user_id)`
- `INDEX idx_contacts_contact_user_id (contact_user_id)`
- `INDEX idx_contacts_status (status)`

**关系说明**:
- 每个好友关系在表中存储一条记录
- `user_id` 和 `contact_user_id` 的组合是唯一的
- 支持双向好友（需要两条记录）

**示例数据**:
```sql
INSERT INTO contacts (user_id, contact_user_id, status)
VALUES (1, 2, 'accepted');
```

### 3. email_verification_codes (邮箱验证码表)

存储发送的邮箱验证码。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | BIGINT | PRIMARY KEY, AUTO_INCREMENT | 记录唯一标识 |
| email | VARCHAR(255) | NOT NULL | 邮箱地址 |
| code | VARCHAR(10) | NOT NULL | 验证码 |
| purpose | VARCHAR(20) | NOT NULL | 验证码用途: register, reset_password |
| used | BOOLEAN | DEFAULT false | 是否已使用 |
| expires_at | TIMESTAMP | NOT NULL | 过期时间 |
| created_at | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | 创建时间 |

**索引**:
- `PRIMARY KEY (id)`
- `INDEX idx_email_codes_email (email)`
- `INDEX idx_email_codes_expires_at (expires_at)`
- `INDEX idx_email_codes_purpose (purpose)`

**验证码生命周期**:
- 注册验证码: 10 分钟
- 找回密码验证码: 10 分钟

**示例数据**:
```sql
INSERT INTO email_verification_codes (email, code, purpose, expires_at)
VALUES ('user@example.com', '123456', 'register', DATE_ADD(NOW(), INTERVAL 10 MINUTE));
```

### 4. email_send_logs (邮件发送日志表)

记录所有邮件发送历史。

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| id | BIGINT | PRIMARY KEY, AUTO_INCREMENT | 记录唯一标识 |
| email | VARCHAR(255) | NOT NULL | 收件人邮箱 |
| subject | VARCHAR(200) | NOT NULL | 邮件主题 |
| template | VARCHAR(50) | NOT NULL | 使用的模板 |
| status | VARCHAR(20) | NOT NULL | 发送状态: success, failed |
| error_message | TEXT | NULL | 错误信息（如果失败） |
| sent_at | TIMESTAMP | DEFAULT CURRENT_TIMESTAMP | 发送时间 |

**索引**:
- `PRIMARY KEY (id)`
- `INDEX idx_email_logs_email (email)`
- `INDEX idx_email_logs_sent_at (sent_at)`
- `INDEX idx_email_logs_status (status)`

**用途**:
- 邮件发送审计
- 问题排查
- 发送频率统计

**示例数据**:
```sql
INSERT INTO email_send_logs (email, subject, template, status)
VALUES ('user@example.com', '邮箱验证', 'verification', 'success');
```

## 🔗 表关系

```
users (1) -----> (N) contacts
users (1) -----> (N) email_verification_codes
users (1) -----> (N) email_send_logs
```

### 关系详细说明

1. **用户与联系人** (多对多)
   - 通过 `contacts` 表实现
   - 需要在应用层保证数据一致性

2. **用户与邮箱验证码** (一对多)
   - 一个用户可以有多条验证码记录
   - 验证码使用后不应删除，保留作为审计

3. **用户与邮件日志** (一对多)
   - 记录用户接收的所有邮件
   - 用于问题追踪

## 📝 数据库操作示例

### 创建用户

```sql
-- 1. 插入用户信息
INSERT INTO users (email, password_hash, display_name, email_verified)
VALUES ('user@example.com', '$2a$10$...', '张三', false);

-- 2. 获取用户 ID
SELECT id FROM users WHERE email = 'user@example.com';
```

### 添加联系人

```sql
-- 双向好友关系
INSERT INTO contacts (user_id, contact_user_id, status)
VALUES (1, 2, 'accepted'), (2, 1, 'accepted');
```

### 发送邮箱验证码

```sql
-- 1. 插入验证码记录
INSERT INTO email_verification_codes (email, code, purpose, expires_at)
VALUES ('user@example.com', '123456', 'register', DATE_ADD(NOW(), INTERVAL 10 MINUTE));

-- 2. 记录邮件发送
INSERT INTO email_send_logs (email, subject, template, status)
VALUES ('user@example.com', '邮箱验证', 'verification', 'success');
```

### 验证邮箱

```sql
-- 1. 验证验证码
UPDATE email_verification_codes
SET used = true
WHERE email = 'user@example.com' AND code = '123456' AND used = false;

-- 2. 更新用户邮箱验证状态
UPDATE users
SET email_verified = true
WHERE email = 'user@example.com';
```

## 🔍 常用查询

### 获取用户信息

```sql
-- 根据邮箱获取用户（登录时使用）
SELECT id, email, password_hash, display_name, email_verified, status
FROM users
WHERE email = 'user@example.com';
```

### 搜索用户

```sql
-- 根据邮箱或昵称搜索
SELECT id, email, display_name, avatar_url
FROM users
WHERE email LIKE '%keyword%'
   OR display_name LIKE '%keyword%'
   AND status = 'active'
LIMIT 20;
```

### 获取联系人列表

```sql
-- 获取用户的所有联系人
SELECT u.id, u.email, u.display_name, u.avatar_url, u.last_seen_at
FROM contacts c
JOIN users u ON c.contact_user_id = u.id
WHERE c.user_id = 1
  AND c.status = 'accepted'
ORDER BY u.display_name;
```

### 获取在线用户

```sql
-- 获取最近 5 分钟内有活动的用户
SELECT id, email, display_name, last_seen_at
FROM users
WHERE last_seen_at >= DATE_SUB(NOW(), INTERVAL 5 MINUTE)
  AND status = 'active';
```

### 清理过期验证码

```sql
-- 删除过期的未使用验证码
DELETE FROM email_verification_codes
WHERE expires_at < NOW() AND used = false;
```

## 📈 性能优化

### 索引优化

1. **主键索引**: 所有表都有自增主键
2. **唯一索引**: 邮箱等唯一字段
3. **复合索引**: 常用查询条件的组合
4. **时间索引**: 按时间范围的查询

### 查询优化

```sql
-- 使用 LIMIT 限制结果集
SELECT * FROM users WHERE status = 'active' LIMIT 100;

-- 使用索引字段进行 WHERE
SELECT * FROM contacts WHERE user_id = 1 AND status = 'accepted';

-- 避免 SELECT *
SELECT id, email, display_name FROM users WHERE id = 1;
```

### 分页查询

```sql
-- 第一页
SELECT * FROM users ORDER BY created_at DESC LIMIT 20 OFFSET 0;

-- 第二页
SELECT * FROM users ORDER BY created_at DESC LIMIT 20 OFFSET 20;
```

### 慢查询优化

1. 避免在 WHERE 子句中使用函数
2. 使用 EXPLAIN 分析查询计划
3. 适当添加索引
4. 避免返回大量数据

## 🔒 数据安全

### 密码存储

使用 bcrypt 进行密码哈希：

```go
// Go 代码示例
hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
```

**安全等级**:
- Cost: 10（可调整）
- 盐值自动生成
- 不可逆加密

### 敏感数据

- **密码**: 仅存储哈希值，不存储明文
- **JWT Secret**: 存储在环境变量或配置中心
- **邮箱**: 可能包含敏感信息，注意日志脱敏

### 数据脱敏

```go
// 日志中的敏感信息脱敏
email := "user@example.com"
maskedEmail := strings.Replace(email, "@", "*@*", 1) // user*@*example.com
```

## 🧹 数据清理

### 定期清理任务

```sql
-- 清理过期的验证码（建议每日执行）
DELETE FROM email_verification_codes
WHERE expires_at < NOW() - INTERVAL 1 DAY;

-- 清理旧的邮件日志（建议每月执行）
DELETE FROM email_send_logs
WHERE sent_at < NOW() - INTERVAL 3 MONTH;

-- 清理已使用的注册验证码（建议每周执行）
DELETE FROM email_verification_codes
WHERE used = true AND created_at < NOW() - INTERVAL 1 WEEK;
```

### 备份策略

```bash
# 完整备份
mysqldump -u allcallall -p allcallall_db > backup_$(date +%Y%m%d).sql

# 仅结构备份
mysqldump -u allcallall -p --no-data allcallall_db > schema_backup.sql

# 仅数据备份
mysqldump -u allcallall -p --no-create-info allcallall_db > data_backup.sql
```

## 🔧 数据库迁移

### 自动迁移

应用启动时自动执行：

```go
// backend/cmd/server/main.go
db.AutoMigrate(
    &models.User{},
    &models.Contact{},
    &models.EmailVerificationCode{},
    &models.EmailSendLog{},
)
```

### 手动迁移

```bash
# 安装 golang-migrate
go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# 创建迁移文件
migrate create -ext sql -dir migrations -seq init_users_table

# 执行迁移
migrate -path migrations -database "mysql://allcallall:pass@tcp(localhost:3306)/allcallall_db" up
```

## 📊 监控指标

### 关键指标

1. **连接数**: 当前活跃连接数
2. **查询性能**: 平均查询时间
3. **慢查询**: 慢查询数量和频率
4. **锁等待**: 表锁和行锁等待时间
5. **缓存命中率**: 查询缓存命中率

### 监控命令

```sql
-- 查看当前连接数
SHOW STATUS LIKE 'Threads_connected';

-- 查看慢查询
SHOW STATUS LIKE 'Slow_queries';

-- 查看缓存命中率
SHOW STATUS LIKE 'Qcache_hit_rate%';

-- 查看表锁状态
SHOW STATUS LIKE 'Table_locks%';
```

## 🚨 常见问题

### 1. 连接数过多

**问题**: `Too many connections`

**解决方案**:
- 增加 MySQL 最大连接数
- 优化应用连接池配置
- 及时释放数据库连接

```sql
-- 查看当前连接数
SHOW STATUS LIKE 'Threads_connected';

-- 查看最大连接数
SHOW VARIABLES LIKE 'max_connections';
```

### 2. 死锁

**问题**: 多个事务相互等待导致死锁

**解决方案**:
- 调整事务隔离级别
- 优化事务中的操作顺序
- 缩短事务执行时间

### 3. 慢查询

**问题**: 查询响应时间过长

**解决方案**:
- 添加适当的索引
- 优化查询语句
- 使用 EXPLAIN 分析执行计划

```sql
-- 启用慢查询日志
SET GLOBAL slow_query_log = 'ON';
SET GLOBAL long_query_time = 2;
```

### 4. 主从同步延迟

**问题**: 从库数据落后于主库

**解决方案**:
- 检查网络延迟
- 优化主库写入速度
- 调整同步参数

## 📚 相关资源

### 官方文档
- [MySQL 8.0 文档](https://dev.mysql.com/doc/refman/8.0/en/)
- [GORM 文档](https://gorm.io/docs/)

### 工具推荐
- **数据库客户端**: MySQL Workbench, DBeaver, phpMyAdmin
- **监控工具**: Prometheus + Grafana, Percona Monitoring
- **备份工具**: mysqldump, XtraBackup

## 📝 相关文档

- [API 文档](./API_DOCUMENTATION.md)
- [配置说明](./CONFIGURATION.md)
- [部署指南](./DEPLOYMENT_GUIDE.md)
- [安全指南](../configuration/SECURITY_GUIDELINES.md)
