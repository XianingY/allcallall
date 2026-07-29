# AllCallAll 代码审查报告（Code Review）

> 审查日期：2026-07-29 ｜ 范围：AllCallAll 主仓（`/Users/byzantium/github/AllCallAll`）
> 标准：可维护的优秀开源项目（结构 / 质量 / 配置依赖 / 测试 / 文档 / 安全）
> 方法：6 路并行只读 Explore 审计（后端 Go / web+desktop / mobile / 配置+CI+密钥 / 文档 / 测试），逐项带回 `file:line` 与严重度，再由重构 agent 分阶段落地。

---

## 0. 总览（各维度成熟度）

| 维度 | 成熟度 | 主要风险 |
|------|--------|----------|
| 代码结构与组织 | 中 | 后端 god package（mcpplatform 1673 行、agent 包混杂）、handlers 扁平大包耦合 DB 模型；前端巨型 Screen/组件 |
| 代码质量 | 中 | 多处关键路径吞错（`_ = err`）、巨型函数、magic number、fire-forget Promise；桌面/移动缺测试 |
| 配置与依赖管理 | 中 | CI actions 仅 `@vN` 未引脚 SHA、缺 `permissions:`；无 Dependabot/CodeQL/`govulncheck`/`npm audit`；`.env.template` 弱默认口令 |
| 测试覆盖 | 中偏低 | backend-ci **无覆盖率门禁**；interviewbench/models/mq 零测试；desktop 无 test 脚本；mobile 仅显式列举 8 个纯函数 |
| 文档完整性 | 中偏低 | `AGENTS.md`/`CLAUDE.md` 为空（自引用/悬空）；`contracts/` 无随仓 schema；env 变量文档缺项 |
| 安全性 | 中偏高（有亮点） | 移动端 `network_security_config` 明文+硬编码公网 IP（P0）；support token 非恒定时间；sandbox 命令解析可绕过白名单；无 CSP |

**亮点（已做对，勿退步）**：SQL 全程参数化；密码 bcrypt；JWT 要求非空密钥；`.env` 已 gitignore；signaling/chat_hub presence 已 Redis 化；E2EE 私钥已移出 AsyncStorage 入 Keychain；release 流水线已有 Trivy 扫描。

---

## 1. 代码结构与组织

| 严重度 | 位置 | 问题 | 建议 |
|--------|------|------|------|
| P2 | `backend/internal/mcpplatform/service.go:1` | 单文件 1673 行（god package） | 拆 catalog/execution/approval/sandboxclient 子包 |
| P2 | `backend/internal/agent/*` | 工作流引擎/ReAct/持久化/工具执行/恢复混杂 | 拆 engine/persistence/tools |
| P2 | `backend/internal/handlers/*` | 10+ 领域 handler 扁平大包，直接返回 `models.*` | 按域分子包 + 引入 DTO/响应结构 |
| P2 | `backend/internal/server/routes.go` | health/metrics/public/internal 混挂，逐 handler `if deps.X!=nil` | 路由表/注册抽象 |
| P0 | `mobile/src/screens/AgentDemoScreen.tsx:1` | 2308 行，聊天/知识库/工作流/审批混杂 | 拆子组件 + hooks |
| P0 | `mobile/src/screens/ConversationDetailScreen.tsx:1` | 1792 行，实时/笔记/房间多职责 | 拆分 |
| P1 | `mobile/src/screens/MCPPlatformScreen.tsx:1` | 1243 行 | 抽容器/展示组件 |
| P1 | `mobile/src/context/SignalingContext.tsx:1` | 信令+WebRTC+E2EE 耦合（1190 行） | 继续向 `signaling/*` 解耦 |
| P1 | `mobile/src/services` | AudioService×4 冗余实现，选择逻辑乱 | 统一平台门面 |
| P1 | `web/src/pages/agent/AgentLabPanels.tsx:1` | 588 行巨型组件 | 拆 TraceView/ApprovalPanel |
| P2 | `web/src` | Context 与 zustand 混用 | 统一状态管理约定 |

> 巨型文件拆分属高成本重构，已写入 `docs/optimization-roadmap.md` 作为中長期路线，本轮回填其安全/增量部分。

---

## 2. 代码质量

| 严重度 | 位置 | 问题 | 建议 |
|--------|------|------|------|
| P1 | `backend/internal/knowledge/chunk_service.go:26,54` | `_ = markChunkIndexFailed` 吞错，状态不一致 | 记日志 |
| P1 | `backend/internal/invitation/service.go:87` | `_ = Update("status",Expired)` 吞错 | 记日志 |
| P2 | `backend/internal/media/room_engine.go:233,343,352,416` `engine.go:151` | `_ = pc.Close()/RemoveTrack()` 吞清理错 | 记日志 |
| P2 | `backend/internal/sandbox/service.go:260` `kubernetes_runner.go:540,546` | `_ = prepared.Close()/deleteJob` 吞错 | 记日志 |
| P2 | `backend/internal/translation/providers/volc_ast.go:130,262,265,279` | WS 写错 `_ =` 吞掉 | 记日志 |
| P2 | `backend/internal/auth/middleware.go:40` | `_ = token // 仅用于调试` 死代码 | 删除 |
| P2 | `backend/internal/agent/context_retrieval.go:170`(204行) `tool_registry.go:52`(203) `service_react.go:21`(185) `mcpplatform/service.go:497`(175) `commerce/followup_service.go:333`(173) | 过长函数 | 拆分 |
| P2 | `invitation_handler.go:74` `organization_service.go:158` `organization_admin_service.go:338` `recording_service.go:381` `auth/jwt.go:54` | `7*24*time.Hour` magic number 散见 | 提取命名常量 |
| P2 | `web/src/meetings/useMeetingEngine.ts:39` 等 7 处 | `void x.catch(()=>undefined)` 吞异常 | 至少 catch 日志 |
| P2 | `web/src/api/http.ts:36` `MeetingRoomPage.tsx:13` `AgentLabUtils.ts:211` `TicketSocket.ts:29` | `as T`/`JSON.parse` 无结构校验 | 引入 zod |
| P2 | `mobile/src/context/SignalingContext.tsx:444` 等 | `.then()` 无 `.catch` 吞异常 | await+try/catch |
| P2 | `mobile` 多处 | `console.log/error` 裸调用、调试残留 | 统一日志封装 + 环境开关 |

---

## 3. 配置与依赖管理

| 严重度 | 位置 | 问题 | 建议 |
|--------|------|------|------|
| P1 | `.github/workflows/{ci,backend-ci,frontend-ci}.yml` | actions 仅 `@vN` 未引脚 SHA | 引脚 commit SHA（或接受 `@vN` 并加 `permissions:`） |
| P1 | 同上三文件 | 缺顶层 `permissions:` 块 | 显式 `permissions: contents: read` |
| P1 | 全仓 | 无 Dependabot/Renovate | 新增 `.github/dependabot.yml` |
| P2 | CI | 无 `govulncheck` / `npm audit` / CodeQL | 补齐依赖与代码扫描 |
| P2 | `backend/cmd/interview-seed/main.go:93` | 硬编码默认口令 `Interview1234` | 移除默认，强制 env 或随机生成 |
| P2 | `.env.template` | 弱默认口令（`allcallallpass`/`rootpass`/`JWT_SECRET=...12345`）；缺 `TRANSCRIPTION_*`/`WEBRTC_ICE_SERVERS_JSON`/`SANDBOX_*` 样例 | 标注“仅本地”+ 补样例 |
| P2 | `backend/go.mod` `go 1.24.1` | CI `go-version:"1.24.x"` 未锁补丁 | 固定补丁版 |
| P2 | `.gitignore` | `mobile/ios/.xcode.env` 被跟踪 | 忽略 |

> 前端依赖（web/desktop/mobile 均有 lockfile）版本锁定良好；未发现私钥/secret 经 `EXPO_PUBLIC_*`/`VITE_*` 暴露。

---

## 4. 测试覆盖

| 严重度 | 位置 | 问题 | 建议 |
|--------|------|------|------|
| P0 | `.github/workflows/backend-ci.yml` | `go test` 仅出 coverage，**无阈值门禁（不 fail）** | 加覆盖率门禁 |
| P0 | `desktop/package.json` | **无 test 脚本**（仅 node --check） | 加 vitest + 单测 |
| P0 | `mobile/package.json` `test:unit` | 显式列举 8 个纯函数，核心 service 未覆盖 | 改 glob 自动发现 + 扩核心 service |
| P1 | `backend/internal/interviewbench` `models` `mq` | 三包零测试 | 补评分/校验/幂等用例 |
| P1 | `web/` | vitest 仅 ~12 用例，api hooks/auth/calls 无单测 | 补核心域 |
| P1 | `web/tests/e2e` 7 spec；`desktop` 无 e2e | 桌面零冒烟 | 补桌面冒烟 |
| P1 | `agent-runtime` `services/rag-runtime` | 仅 1 测试文件，检索/rerank 单测极薄弱 | 补检索单测 |
| P2 | `agent-runtime` `contracts/schemas` | `contracts-check` 仅生成校验，无行为测试 | 补 schema 解析测试 |

---

## 5. 文档完整性

| 严重度 | 位置 | 问题 | 建议 |
|--------|------|------|------|
| P0 | `AGENTS.md` | 经 symlink 自引用，无真实 agent 指南 | 补真实内容 |
| P0 | `CLAUDE.md` | symlink 指向缺失的 `docs/reference/CLAUDE.md` | 补真实文件 |
| P0 | `contracts/` | 仅 legacy fixtures，无随仓 schema，无法校验 Go↔Python 一致 | 随仓提供或写明外链校验 |
| P1 | `.env.template` vs 代码 | 缺 `TRANSCRIPTION_*`/`WEBRTC_ICE_SERVERS_JSON`/`SANDBOX_*` | 补齐样例 |
| P1 | `openapi.yaml` | 仅 web 侧校验，后端路由无 contract 校验 | 加后端 contract 校验 |
| P2 | `docs/superpowers/plans/2026-06-30-cross-stack-refactoring.md` | 引用已删除路径，易被误读为当前结构 | 加醒目“非当前”横幅 |
| P2 | `README.md` / `docs/configuration/configuration.md` | env 变量清单缺项 | 对齐代码实际变量 |

---

## 6. 安全性

| 严重度 | 位置 | 问题 | 建议 |
|--------|------|------|------|
| P0 | `mobile/android/.../network_security_config.xml:4,6-12` | `cleartextTrafficPermitted=true` + 明文放行硬编码公网 IP | 强制 https，删明文域 |
| P0 | `mobile/android/.../AndroidManifest.xml:23` | `usesCleartextTraffic=true` + `allowBackup=true` | 关明文、按需关备份 |
| P1 | `backend/internal/sandboxsupervisor/server.go:348-350` | `resolveCommand` 含路径分隔符时直返原值，可绕过 PATH 白名单执行任意二进制 | 含分隔符须落 allowlist 否则拒 |
| P1 | `backend/internal/collaboration_support_handler.go:20` | `X-Support-Token` 用 `!=` 比较（时序侧信道） | `subtle.ConstantTimeCompare` |
| P2 | `backend/internal/server/routes.go:60-66` | 内部/support 路由绕过框架级鉴权，仅 handler 自校验 | 组级统一鉴权中间件 |
| P2 | `backend/internal/auth/middleware.go:37` | WS 令牌取 URL query，易经日志泄露 | 改用短时 ticket/子协议头 |
| P2 | `backend/internal/user/password.go:37-39` | 密码策略禁特殊字符，熵低 | 允许特殊字符，按长度+熵 |
| P1 | `desktop/package.json` `electron ^31` | 浮动且偏旧，CVE 风险 | 锁精确版本 + audit |
| P1 | `desktop/main.cjs:51` | 缺 session CSP 与 `setPermissionRequestHandler` | 补充 |
| P2 | `web/index.html` / `desktop/main.cjs` | 无 CSP | `default-src 'self'` |

---

## 7. 优先级与阶段计划（本次执行）

- **阶段 1 · 配置与 CI 卫生**：CI `permissions:` + Dependabot + `govulncheck`/`npm audit`；修 `interview-seed` 硬编码口令、`.env.template` 标注、`.gitignore`、support token 恒定时间、删死代码。
- **阶段 2 · 后端质量与安全**：补关键路径吞错日志、提取 magic 常量、sandbox 命令解析加固。
- **阶段 3 · 前端与移动端安全**：web CSP + eslint max-lines + fire-forget 修复；desktop 锁 electron + test 脚本 + CSP；mobile 关明文传输/备份、收敛权限。
- **阶段 4 · 文档一致性**：补 `AGENTS.md`/`CLAUDE.md` 真实指南、`contracts/` 说明、env 文档对齐。
- **阶段 5 · 测试门禁**：backend-ci 加覆盖率门禁；补充 interviewbench/models/mq 单测；desktop 加 test 脚本。
- **阶段 6（中长期，入 roadmap）**：巨型文件拆分、DTO 边界、zod 校验、状态管理统一、agent-runtime rag-runtime 单测补强。

> 阶段 1–5 在本轮回填并逐阶段提交；阶段 6 已在 `docs/optimization-roadmap.md` 规划，按迭代摊销。
