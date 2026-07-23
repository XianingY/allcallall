# AllCallAll 架构与单元测试覆盖分析

> 生成日期：2026-07-23
> 范围：主仓 `AllCallAll`（Go backend / web / mobile / infra）及独立 Python 运行时仓库 `allcallall-agent-runtime`
> 本次工作：架构梳理 + 测试覆盖盘点 + 缺失用例补齐 + CI/CD 修正

---

## 1. 整体架构

AllCallAll 逻辑上是一个系统，但拆成两个独立 git 仓库协同工作：

| 仓库 | 技术栈 | 职责 |
|------|--------|------|
| `AllCallAll`（主仓） | Go (Gin+Gorm+Redis+Pion WebRTC)、React+Vite(web)、Expo RN(mobile)、Electron(desktop) | 平台主后端、前端、移动端、基础设施与编排 |
| `allcallall-agent-runtime`（独立仓） | Python (FastAPI+LangGraph / RAG / sandbox / interview-mcp)，uv workspace | 权威的 Agent/RAG/沙箱运行时，经 HTTP Tool Bridge 与主后端通信 |

主后端通过 HTTP 调用独立运行时；写操作只回传 proposal，由 Go 端审批后执行（写操作 fail-closed）。

### 主后端模块（`backend/internal`）

| 模块 | 职责 |
|------|------|
| `agent` / `runtime` / `mcpplatform` | Agent 编排、LangGraph 工作流、MCP 工具平台与沙箱控制面 |
| `sandbox` / `sandboxsupervisor` | Go 侧沙箱执行与 Python supervisor 契约 |
| `collaboration` / `signaling` / `media` / `presence` | 房间/协作、WebRTC 信令、媒体、在线状态 |
| `commerce` | 计费(RevenueCat webhook)、权益、滥用举报、支持诊断、跟进任务 |
| `auth` / `user` / `contact` / `invitation` | 认证、用户、联系人、邀请 |
| `settlement` | 房间结束结算事件（MQ 发布/消费、幂等落库） |
| `knowledge` / `search` / `translation` / `transcription` | 知识库、ES 检索、实时翻译、转写 |
| `handlers` / `server` | HTTP 路由与中间件（含全局限流、支持网络门禁） |
| `config` / `models` / `database` / `cache` / `ratelimit` / `mq` | 基础设施：配置、领域模型、DB、缓存、限流、消息队列 |
| `apperror` | 统一错误类型与 HTTP 状态码映射 |
| `testutil` | 测试夹具（SQLite、种子数据） |

### 前端 / 移动端

- **web**（`web/src`）：`api/`（HTTP 客户端、各域 API）、`realtime/`（TicketSocket、ChatRealtimeContext、chatEvents 协议）、`lib/`（runtime-config）、`components/`、`pages/`、React 组件树。
- **mobile**（`mobile/src`）：用 `tsx --test` 列举式运行单元测试（实时 reducer、信令、翻译等）。

---

## 2. 测试覆盖盘点（修改前基线）

### 后端（240 个非测试 .go 文件 / 106 个测试文件）

- **已有较完整覆盖**：`agent`(17)、`handlers`(13)、`collaboration`(6)、`runtime`(5)、`mcpplatform`(5)、`commerce`(2，仅 entitlement)、`auth`(4) 等。
- **零测试包**：`apperror`、`invitation`、`models`、`mq`、`interviewbench`。
- **仅 1 个测试文件、覆盖薄弱**：`config`、`contact`、`cache`、`ratelimit`、`signaling`、`settlement`、`storage`、`transcription`、`usergrpc`、`metrics`、`presence`、`media`、`logger`、`database`、`fcm`、`integration`。
- **关键纯逻辑函数长期未被单测**（高价值、零外部依赖）：`apperror` 全部构造函数；`invitation.normalizeLang/randomCode`；`settlement.RoomEndedEvent.Validate/DecodeRoomEndedMessage`；`commerce.parseAppUserID`、`normalizeReportCategory`、`supportRefreshSessionRisk`、`normalizeFollowUpTaskType/Status`、`mustJSON/extractTexts/truncateSentence/peerNameOrEmail`；`config.ApplyDefaults`。

### 前端

- web：10 个测试文件，覆盖 `http`、`mcp`、`runtime-config`、`chatEvents`、`TicketSocket`、`ChatRealtimeContext` 等核心接口；覆盖率门禁阈值已在 `vite.config.ts` 设为回归底线（行 15 / 函数 35 / 分支 55 / 语句 15）。
- mobile：8 个文件，但 `package.json` 的 `test:unit` 为**显式列举**（新增文件不会自动纳入）。

### CI/CD（修改前问题）

| 工作流 | 问题 |
|--------|------|
| `frontend-ci.yml` | web-ci / mobile-ci **只做 typecheck + lint，从不运行测试**（真实缺陷） |
| `backend-ci.yml` | 运行测试但**不生成覆盖率报告、无门禁证据、无 artifact** |
| `ci.yml` | 已覆盖 web/mobile 测试，但 `checkout@v5`，版本落后于 `platform-ci.yml` 的 `@v7` |
| 一致性 | action 版本 `@v5` vs `@v7` 混用 |

---

## 3. 本次补齐的单元测试

全部为**纯函数 / 接口契约**用例，覆盖正常流程、边界条件与异常场景，零外部依赖（不连 DB/Redis/MQ）。

### 新增测试文件（后端）

| 文件 | 覆盖函数 | 场景要点 |
|------|----------|----------|
| `internal/apperror/errors_test.go` | `New`/`Wrap`/`Error`/`Unwrap` 及 7 个构造函数 | 错误信息格式、内部错误 `errors.Is` 透传、各构造函数状态码正确 |
| `internal/invitation/service_test.go` | `normalizeLang`、`randomCode` | 大小写/空格归一化；URL-safe 字符集、长度 24、无碰撞 |
| `internal/settlement/service_pure_test.go` | `RoomEndedEvent.Validate`、`DecodeRoomEndedMessage`、`PublishRoomEnded` | 必填字段/零值校验；JSON 往返 + 校验失败；发布主题/Key/Header 正确；无效事件不入 broker；nil producer 与 broker 错误传播（用 fake `mq.Producer`） |
| `internal/commerce/billing_webhook_test.go` | `parseAppUserID` | `user:123`/`123`/带空格；`0`/`user:0`/非数字/负数均报错 |
| `internal/commerce/block_abuse_service_test.go` | `normalizeReportCategory`、`reportCategoryList`、`ReportCategories` | 大小写归一化；非法类别报错；导出列表与内部 allow-list 一致性不变量 |
| `internal/commerce/support_service_test.go` | `supportRefreshSessionRisk` | 无信号/多活跃会话/单次复用/重复复用/近 24h 复用；**24h 边界**（恰好 24h 前不算"近期"） |
| `internal/commerce/followup_service_test.go` | 类型/状态归一化、`mustJSON`、`extractTexts`、`truncateSentence`、`peerNameOrEmail` | 非法枚举报错；`nil`→`null`、空→`[]`；TranslatedText 优先回退 OriginalText；截断与 `...`；名称/邮箱回退 |
| `internal/config/config_test.go`（扩展） | `DatabaseConfig.ApplyDefaults` | 默认值代入；显式值不被覆盖 |

> 注意：`settlement/service_test.go` 原有的幂等集成测试**未覆盖**，已通过新增 `service_pure_test.go` 追加纯函数用例，避免破坏既有测试。

### 前端（扩展既有文件）

- `web/src/api/http.test.ts`：新增 `apiRequest` 的成功返回/Bearer 头/组织头/禁用 auth 无头/204 与 非JSON 返回 `undefined`/结构化错误映射/无 JSON body 回退 statusText/`retry401=false` 不刷新/刷新失败清空 token；`apiDownload` 的 filename 解析、缺头、401 刷新重试。
- `web/src/realtime/chatEvents.test.go`：扩展 `reduceChatCursor` 边界——等于/低于 cursor 忽略、200 条滑动窗口去重、乱序重放。

### 修正的回归（门禁正确性）

上一轮安全加固将 `SUPPORT_INTERNAL_ONLY` 改为 **fail-closed**（默认仅限内网）。3 个 handler 测试仍断言旧的宽松语义（503/401/200），现因网络门禁先返回 403 而失败。本次：
- 在 `TestCommercialHandlerSupportTokenGuard`、`TestCommercialHandlerSupportRevokesRefreshSessions`、`TestCollaborationHandlerSupportRoomRequiresToken` 中显式 `SUPPORT_INTERNAL_ONLY=false` 以隔离测试 token 门禁。
- 新增 `TestSupportNetworkGuard`：锁定 fail-closed 行为（默认拒绝外网 → 403；loopback 放行并到达 token 门禁；显式 opt-out 放行外网）。

---

## 4. CI/CD 修正

| 文件 | 变更 |
|------|------|
| `frontend-ci.yml` | web-ci 增加 `npm run test:coverage`（vite 阈值即门禁）+ 上传 `web/coverage` artifact；mobile-ci 增加 `npm run test:unit`；`checkout` 升 `@v7` |
| `backend-ci.yml` | `go test` 增加 `-coverprofile=coverage.out -covermode=atomic`；新增 `go vet ./...`、`go tool cover -func` 摘要、上传 `backend-coverage.out` artifact；`checkout` 升 `@v7` |
| `ci.yml` | 全部 `actions/checkout@v5` → `@v7`，统一 action 版本 |

效果：所有语言的单测在 CI 自动运行；覆盖率报告作为 artifact 产出；web 覆盖率阈值、handler 网络门禁作为**门禁**校验；`go vet` 作为静态门禁。

---

## 5. 验证结果

- 后端：`go test ./...` 全绿（含新增 7 文件 + 扩展 1 文件 + 修正 3 处），`go vet ./...` 通过。
- 前端：`npm test` 40 个用例全过；`npm run test:coverage` 通过阈值门禁。
- 工作流 YAML 经 `yaml.safe_load` 校验全部合法。

### 新增/修改测试文件清单
```
backend/internal/apperror/errors_test.go                      (新增)
backend/internal/invitation/service_test.go                  (新增)
backend/internal/settlement/service_pure_test.go            (新增)
backend/internal/commerce/billing_webhook_test.go           (新增)
backend/internal/commerce/block_abuse_service_test.go       (新增)
backend/internal/commerce/support_service_test.go           (新增)
backend/internal/commerce/followup_service_test.go          (新增)
backend/internal/config/config_test.go                      (扩展)
backend/internal/handlers/commercial_handler_test.go        (修正+新增网络门禁测试)
backend/internal/handlers/collaboration_handler_test.go     (修正)
web/src/api/http.test.ts                                     (扩展)
web/src/realtime/chatEvents.test.ts                         (扩展)
```

---

## 6. 后续建议（未在本轮执行）

1. **移动端 `test:unit` 改为 glob**（如 `vitest` 或 `tsx --test src/**/*.test.ts`）以便新增用例自动纳入，避免遗漏。
2. **后端薄弱包补集成测试**：`contact`/`cache`/`ratelimit`/`signaling` 可用 `testutil.OpenSQLite` + miniredis 做轻量集成测试（当前仅 1 个文件或零覆盖）。
3. **页面/组件测试补全**：web 的 `pages/**`、`components/**` 整树覆盖率近 0，是可观的后续工作量（已在 `vite.config.ts` 备注为跟踪项）。
4. **后端覆盖率门禁**：当前仅产出报告未设全局阈值（避免一次性大面积失败）；随单测补全可逐步引入最低行覆盖率门槛。
5. **CI 去重**：`ci.yml` 与 `backend-ci.yml`/`frontend-ci.yml` 在后端/web/mobile 上重复执行，长期可合并为单一编排工作流（删除前需确认分支保护规则未依赖被删任务的 check 名称）。
