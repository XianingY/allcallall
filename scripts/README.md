# AllCallAll 项目脚本使用指南

此目录包含项目所有的实用脚本，按用途分为三类。

## 📁 目录结构

### 开发脚本 (`development/`)

用于本地开发和测试环境。

#### `start-services.sh`
启动本地开发环境（MySQL + Redis）

```bash
./scripts/development/start-services.sh
```

**功能**:
- 启动 Docker 容器 (MySQL, Redis)
- 等待服务就绪
- 显示服务状态和连接信息

#### `restart-services.sh`
重启所有服务（数据库 + 后端）

```bash
./scripts/development/restart-services.sh
```

**功能**:
- 停止现有的所有服务
- 启动 Docker 容器
- 启动后端服务
- 验证服务健康状态

**注意**: 该脚本会在后台启动后端服务

#### `generate-agent-capability-keypair.sh`
生成 Agent capability 使用的 Ed25519 密钥对（应用接受 raw 32 字节 RFC 8032 seed 与 public key）。

```bash
./scripts/development/generate-agent-capability-keypair.sh
```

**说明**:
- `umask 077`，私钥临时文件在脚本退出时自动删除
- 输出可直接写入环境变量供本地调试使用

#### `start-android-debug.sh`
设置 Android 真机调试环境

```bash
./scripts/development/start-android-debug.sh
```

**功能**:
- 配置 ADB 调试
- 设置设备连接
- 启动移动端开发服务器

---

### 部署脚本 (`deployment/`)

用于生产环境部署。

#### `deploy-cloud.sh`
云服务器部署脚本（Nginx/UFW 方式）

```bash
./scripts/deployment/deploy-cloud.sh <server-ip> [domain-name] [work-dir]
```

**功能**:
- 安装 Docker 和 Docker Compose
- 克隆项目代码
- 创建环境配置文件
- 配置 Nginx
- 配置防火墙

**相关脚本**: 如需使用 Cloudflare Tunnel 方式部署，请使用 `infra/deploy-cloudflare-tunnel.sh`

---

### 测试脚本 (`testing/`)

用于功能测试和验证。

#### `test-change-password.sh`
测试修改密码功能

```bash
./scripts/testing/test-change-password.sh
```

**功能**:
- 测试用户登录
- 测试密码修改 API
- 验证修改结果

---

### 备份与恢复 (`backup/`)

#### `backup.sh`
全量备份自托管栈：MySQL 全实例 dump + Redis RDB 快照，输出到 `BACKUP_DIR` 下按时间戳命名的目录。

```bash
./scripts/backup/backup.sh
```

**功能**:
- `mysqldump --all-databases` 导出全实例
- Redis `BGSAVE` 后复制 RDB 文件
- 若提供 S3 凭据且存在 `aws` CLI 则上传，否则本地按数量轮转
- 连接信息复用 `infra/docker-compose.production.yml` 的同名环境变量

#### `restore.sh`
从已有备份恢复 MySQL 与 Redis。

```bash
./scripts/backup/restore.sh                 # 列出可用备份
./scripts/backup/restore.sh <backup_dir>    # 恢复指定备份
```

**前置条件**: 目标 MySQL / Redis 容器必须已在运行，脚本不会重建容器。

---

### 压测脚本 (`load/`)

#### `agent-run-smoke.sh`
Agent run 冒烟压测：并发发起 agent run 并检查是否全部落库。

```bash
BASE_URL=http://localhost:8080 TOKEN=... ORGANIZATION_ID=... CONVERSATION_ID=... \
  CONCURRENCY=10 ./scripts/load/agent-run-smoke.sh
```

#### `api-qps-bench.mjs`
HTTP API QPS 基准测试。必填环境变量：`BASE_URL` / `TOKEN` / `ORGANIZATION_ID` / `CONVERSATION_ID`。

```bash
BASE_URL=http://localhost:8080 TOKEN=... ORGANIZATION_ID=... CONVERSATION_ID=... \
  node scripts/load/api-qps-bench.mjs
```

#### `ws-connections.mjs`
WebSocket 长连接容量测试：建立 `CLIENTS` 个连接并保持 `DURATION_MS` 毫秒。

```bash
WS_URL=ws://localhost:8080/api/v1/chat/ws TOKEN=... ORGANIZATION_ID=... \
  CLIENTS=10 DURATION_MS=10000 node scripts/load/ws-connections.mjs
```

#### `chat-ws-replay-bench.sh` / `realtime-replay-bench.sh`
Chat / Realtime 事件回放压测，验证断线重连时的事件补发链路。

```bash
EVENTS=2000 RECIPIENTS=10 REPLAY_WINDOW=120 REPLAY_LIMIT=100 \
  ./scripts/load/chat-ws-replay-bench.sh
```

#### `run-interview-suite.sh`
面试/演示用的一键回归套件（种子数据 + 事件回放）。

```bash
AGENT_PROVIDER=rules CONVERSATIONS=25 EVENTS=2000 ./scripts/load/run-interview-suite.sh
```

#### `run-interview-live-suite.sh`
面向已启动栈的在线套件，同时压测 HTTP 与 WebSocket。

```bash
BASE_URL=http://localhost:8080 WS_URL=ws://localhost:8080/api/v1/chat/ws \
  ./scripts/load/run-interview-live-suite.sh
```

---

### Web 质量门禁 (`web/`)

#### `openapi-contract-check.mjs`
校验 `docs/api/openapi.yaml` 与生成的 `packages/api-types/schema.d.ts` 是否同步。

```bash
cd web && npm run contract:check
```

#### `bundle-budget.mjs`
校验前端产物体积预算（默认 JS ≤ 820 KB、CSS ≤ 140 KB，可用 `WEB_BUNDLE_MAX_JS_BYTES` / `WEB_BUNDLE_MAX_CSS_BYTES` 覆盖）。

```bash
cd web && npm run build && npm run bundle:budget
```

---

### 安全脚本 (`secret-scan.sh`)

提交前的三层密钥扫描，**任一层命中即阻断提交**：

1. 自引用环境变量默认值（变量名暗示是密钥、默认值又指回自身）—— GitGuardian 对这类模式会误报，本地先拦住
2. 高信号 provider 正则（AWS / GitHub / OpenAI / Slack / 私钥）
3. `gitleaks`（如已安装）

```bash
./scripts/secret-scan.sh
```

---

### 演示与面试栈

#### `interview-stack.sh`
一键拉起完整面试栈（MySQL / Redis / Elasticsearch / migration / MCP / sandbox / RAG / agent-runtime / backend / web）。

```bash
./scripts/interview-stack.sh
```

#### `interview-demo.sh`
本地模式的一键演示，生成会话种子数据并产出报告目录。

```bash
MODE=local AGENT_PROVIDER=mock_llm ./scripts/interview-demo.sh
```

#### `interview-microservice-demo.sh`
面向已运行栈的微服务拆分演示（HTTP + WebSocket）。

```bash
BASE_URL=http://localhost:8080 ./scripts/interview-microservice-demo.sh
```

---

### 一次性工具脚本

#### `ai_agent_jd_eval.py`
按 JD 维度评估 Agent 能力的离线打分脚本（读取 JSON 输入，产出评估报告）。

```bash
python3 scripts/ai_agent_jd_eval.py --help
```

#### `split_agent_service.py`
按函数名把 `backend/internal/agent/service.go` 拆分为多个文件的重构辅助脚本（一次性工具，非日常使用）。

```bash
python3 scripts/split_agent_service.py
```

## 🚀 快速开始

### 初次设置

```bash
# 1. 启动数据库和 Redis
./scripts/development/start-services.sh

# 2. 在另一个终端启动后端
cd backend && go run cmd/server/main.go

# 3. 在第三个终端启动移动端开发服务器
cd mobile && npm start
```

### 完整重启

```bash
# 一次性重启所有服务
./scripts/development/restart-services.sh
```

---

## ⚠️ 常见问题

### Docker 启动失败
- 确保 Docker Desktop 正在运行
- 检查磁盘空间是否充足
- 查看日志: `docker compose logs`

### 后端无法连接数据库
- 确保 MySQL 容器正在运行: `docker ps`
- 检查数据库凭证配置
- 查看后端日志了解详细错误

### 脚本权限不足
```bash
# 添加执行权限
chmod +x scripts/development/*.sh
chmod +x scripts/deployment/*.sh
chmod +x scripts/testing/*.sh
```

---

## 📝 脚本维护

新增脚本时，请：

1. 将脚本放在相应的子目录
2. 添加可执行权限: `chmod +x script.sh`
3. 在此 README 中添加说明
4. 确保脚本有清晰的错误处理和日志输出

---

**最后更新**: 2026-09-21
