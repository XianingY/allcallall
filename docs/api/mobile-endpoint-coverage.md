# Mobile 手写 API 层 vs `openapi.yaml` 端点覆盖清单

> 生成日期：2026-09-23 · 任务 #22 的前置产物
> **2026-09-25 更新：第 2 节列出的 5 个缺口域已全部补入 spec 并完成迁移，详见第 6 节。**
> 数据来源：`docs/api/openapi.yaml`（顶层 path 项）与 `mobile/src/api/*.ts` 中的端点字符串字面量。
> 归一化规则：mobile 的模板插值 `${foo}` 等价映射为 OpenAPI 的 `{foo}`。

## 1. 总量

| 指标 | 2026-09-23 | 2026-09-25 |
|---|---|---|
| `openapi.yaml` 顶层 path | 48 | **101** |
| ✅ 已被 spec 覆盖 | 38 | **全部（第 2 节 5 个域已补入）** |
| ❌ spec 缺失 | 53 | **0** |
| ⚠️ spec 中有但 mobile api 层未调用 | 10 | 10 |

## 2. ❌ spec 缺失的 53 个端点（按域分组）

这些域在 `openapi.yaml` 中**整块缺失**，是"直接删除手写层必然编译失败"的根因：

| 域 | 缺失端点数 | 端点 |
|---|---|---|
| **collaboration**（会话/房间/录音/商机） | 25 | `/conversations`, `/conversations/{id}`, `/conversations/{id}/messages`, `/conversations/{id}/notes`, `/conversations/{id}/read`, `/conversations/{id}/rooms`, `/deals`, `/deals/{id}`, `/deals/{id}/activities`, `/deals/{id}/contacts`, `/organizations/{id}/policy`, `/pipelines`, `/recordings`, `/recordings/{id}`, `/recordings/{id}/transcript`, `/recordings/{id}/transcription/retry`, `/rooms`, `/rooms/{id}/ice`, `/rooms/{id}/join`, `/rooms/{id}/leave`, `/rooms/{id}/media`, `/rooms/{id}/offer`, `/rooms/{id}/recording/start`, `/rooms/{id}/recording/stop`, `/rooms/{id}/state` |
| **knowledge**（知识库/RAG 治理） | 11 | `/knowledge/sources`, `/knowledge/sources/{id}`, `/knowledge/sources/{id}/reingest`, `/knowledge/source-groups`, `/knowledge/source-groups/{id}`, `/knowledge/source-groups/{id}/canonical`, `/knowledge/dead-letters`, `/knowledge/dead-letters/{id}/retry`, `/knowledge/duplicate-candidates`, `/knowledge/duplicate-candidates/{id}/decision` |
| **users**（通讯录/邀请/在线状态） | 9 | `/invitations`, `/invitations/{code}`, `/invitations/{code}/accept`, `/users/contacts`, `/users/contacts/{id}`, `/users/contacts/{id}/profile`, `/users/fcm-token`, `/users/presence`, `/users/search` |
| **commercial**（通话/跟进） | 6 | `/calls/history`, `/calls/{id}/followup`, `/calls/{id}/followup/generate`, `/calls/{id}/followup/regenerate`, `/follow-ups`, `/follow-ups/{id}` |
| **signaling**（信令轮询通道） | 2 | `/signaling/poll`, `/signaling/send` |
| **mcpPlatform**（动态 action 段） | 1 | `/agent/mcp/installations/{id}/{action}`（spec 中拆成了 `activate`/`publish`/`validate` 三个具体 path） |

## 3. ✅ spec 已覆盖的 38 个端点（可迁移）

| 模块 | 端点 |
|---|---|
| `auth.ts` | `/auth/login`, `/auth/logout`, `/auth/logout-all`, `/auth/refresh`, `/auth/register`, `/auth/sessions`, `/auth/sessions/{sessionId}` |
| `agent.ts` | `/agent/runs`, `/agent/runs/{runId}`, `/agent/runs/{runId}/events`, `/agent/workflows`, `/agent/workflows/{workflowId}`, `/agent/workflows/{workflowId}/process`, `/agent/approvals`, `/agent/approvals/{approvalId}/decision` |
| `mcpPlatform.ts` | `/agent/mcp/installations`, `/agent/mcp/installations/{id}`, `/agent/mcp/installations/{id}/secrets`, `/agent/mcp/installations/{id}/tools`, `/agent/skills`, `/agent/skills/{skillId}` |
| `commercial.ts` | `/auth/password-reset/send`, `/auth/password-reset/confirm`, `/entitlements/me`, `/legal/current`, `/legal/accept`, `/usage/me`, `/users/blocks`, `/users/blocks/{blockedUserId}`, `/users/me/deletion`, `/users/reports` |
| `email.ts` | `/email/send-verification-code`, `/email/verify-code` |
| `users.ts` | `/users/me`, `/users/change-password` |
| `collaboration.ts` | `/organizations`, `/organizations/{organizationId}/switch` |
| `webrtc.ts` | `/webrtc/config` |

> 其中 `agent.ts` 与 `mcpPlatform.ts` **已经**在 `import from "@allcallall/api-types"`，即共享契约的接入方式已有先例。

## 4. ⚠️ spec 中有、mobile api 层未调用的 10 个

`/agent/mcp/executions/{executionId}`、`/agent/mcp/installations/{id}/activate`、`/agent/mcp/installations/{id}/publish`、`/agent/mcp/installations/{id}/validate`、`/agent/runs/{runId}/submit-tool-outputs`、`/organizations/invites/{code}/accept`、`/organizations/{id}/admin/summary`、`/push/devices`、`/push/devices/{deviceId}`、`/realtime/tickets`

（多为 web 端专用或 push 能力未接入 RN，暂不影响 mobile 迁移。）

## 4.5 web 侧现状（同一契约，进度领先于 mobile）

| 指标 | web | mobile |
|---|---|---|
| api 模块数 | 14 | 12 |
| 已接入 `@allcallall/api-types` 的模块 | **8**（identity / knowledge / collaboration / agent / mcp / platform / meetings / realtime） | **2**（agent / mcpPlatform） |
| 引用端点字面量 | 112 | 91 |
| 与 spec 字面量完全匹配 | 30 | 38 |

要点：

1. **web 的端点模块已基本全部接入共享契约**，未接入的只剩 `http.ts` / `pagination.ts` / `query.ts` 这三个基础设施文件（不是端点模块）。
2. web 的"缺失 82"里有相当部分是**误报**：`/agent/runs/${id}`、`/auth/sessions/${id}`、`/agent/approvals/${id}/decision` 只是参数命名与 spec 的 `{runId}` / `{sessionId}` / `{approvalId}` 不一致，语义上其实已覆盖；另外 `mcp.test.ts` 里的 `/agent/skills/3`、`/api/v1/...` 属于测试夹具，不是真实缺口。真实缺口集中在 `/conversations/*`（attachments / pins / reactions / typing）、`/meetings`、`/knowledge`、`/realtime`。
3. 因此 **#22 的剩余工作量主要在 mobile 侧**——web 已覆盖的表面基本完成迁移。

### 由此调整的落地优先级

1. mobile：把已覆盖端点迁到共享契约（`auth`、`email`、`users` 的 `/users/me` 与 `/users/change-password`、`commercial` 的 legal/entitlements/usage、`collaboration` 的 organizations）。`webrtc.ts` 已完成。
2. 统一 spec 的路径参数命名（`{id}` vs `{runId}` 等），消除跨端字面量漂移。
3. spec 补全（后续）：以 `backend/internal/handlers` 的路由定义为事实来源，逐域补齐 collaboration → knowledge → meetings/rooms → commercial → users/signaling。

## 5. 决策：选 B（分阶段），先迁移已覆盖端点

**选择：B — 仅针对已覆盖的端点生成/接入共享 client，暂时保留手写模块并标注待迁移。**

理由：

1. **A 的成本与风险都在 spec 侧，而不是 client 侧。** 要补全 53 个端点，必须为 collaboration / knowledge / rooms / commercial / users 五块核心域手写 OpenAPI schema（含 request/response 结构）。若照 mobile 客户端的现有字段去反推写 spec，等于把"手写层的假设"固化成契约——这正是 #22 要消灭的**契约漂移**本身，只会让漂移从 client 转移到 spec。
2. **正确做法是以后端 handler 为事实来源生成 spec**，但那是一项独立且远大于 client 迁移的工作量，不应阻塞本次可立即拿到的收益。
3. **B 是 A 的必要前置。** 共享 client 与类型接线一旦就位，后续扩 spec 就退化为"一个域一个域机械替换"，每个域都能用 typecheck + build 单独验证。
4. **B 零回归风险**：未覆盖域保留原手写实现，行为完全不变。

### 落地顺序（每步单独验证）

1. 已覆盖且尚未接共享契约的模块：`email.ts`、`webrtc.ts`、`users.ts`（`/users/me`、`/users/change-password`）、`auth.ts`、`commercial.ts` 的 legal/entitlements/usage 部分、`collaboration.ts` 的 organizations 部分。
2. 对 spec 缺失域加显式 `TODO(#22)` 标注，说明"待 spec 补全后迁移"，避免后来者误以为已对齐。
3. spec 补全（后续）：以 `backend/internal/handlers` 的路由定义为事实来源，逐域补齐 collaboration → knowledge → rooms/recordings → commercial → users/signaling。
4. 每补齐一个域，删除对应手写模块并全量走 `npm run typecheck` + `npm run lint` + 测试。

## 6. 2026-09-25：第 2 节 5 个缺口域已补齐并迁移

`openapi.yaml` 顶层 path 由 48 增至 **101**，第 2 节的 collaboration / knowledge / users /
commercial / signaling 五个域全部纳入 spec，对应 mobile 客户端改为引用
`@allcallall/api-types` 生成的共享契约，`TODO(#22)` 标注已移除。

### 做法

1. **补 spec**：新增约 53 个 path 与所需 schema（`Invitation`、`PresenceRecord`、
   `CallFollowup`、`SourceGroupDetail`、`Pagination`、`RoomListItem`、`RoomEvent`、
   `MeetingSummary`、`ConversationFollowup`、`SignalMessage` 及各请求体 schema）。
   发现多数**实体 schema 早已存在**（`Conversation` / `Room` / `Recording` / `Deal` /
   `Pipeline` / `CallHistory` / `KnowledgeSource` …），缺的只是 path，故工作量主要在 path。
2. **重新生成**：`cd web && npm run generate:api`（openapi-typescript → `schema.d.ts`）。
3. **迁移客户端**：`knowledge.ts` / `users.ts` / `commercial.ts` / `collaboration.ts`
   的记录类型改为生成类型别名；`signalingPoll.ts` 的**线上报文**用 `operations` 类型，
   `SignalMessage` 仍保留为 `./signaling` 的应用内领域类型（被 SignalingContext 与
   signalingTransports 共用），仅在收发边界转换。
4. **以 typecheck 为纠错器**迭代：迁移暴露了 spec 里若干过松的定义——`ConversationDetail`
   的 `latest_recording` / `meeting_summary` 原是 `additionalProperties: true` 自由对象、
   `Room.events` 是自由对象数组、且缺 `latest_room` / `latest_meeting`。已收紧为
   `$ref` 并补齐字段（`RoomEvent` 等），而不是在 client 侧放宽。

### 验证

mobile `typecheck` 0 · `lint` 0 error / 4 warning（预存基线）· `test:unit` 33/33 ·
`test:jest` 13/13；web `contract:check` "OpenAPI contract is in sync" · `typecheck` 0。

### 遗留

- 本次 schema 是**以 mobile 客户端现有字段为依据**补写的（保证 client 可编译、行为不变），
  并非以 `backend/internal/handlers` 为事实来源生成。第 5 节第 1 条的顾虑依然成立：
  后续应以后端 handler 复核这些 schema，消除把 client 假设固化进契约的风险。
- `mcpPlatform` 的动态 action 段（`/agent/mcp/installations/{id}/{action}`）在 spec 中
  仍拆为 `activate` / `publish` / `validate` 三个具体 path，属有意设计，非缺口。
