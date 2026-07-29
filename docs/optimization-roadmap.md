# 优化改造路线图（Optimization Roadmap）

本项目审查遗留项的可执行改造方案。编号与审查报告对应：
- **P1#8** 媒体 RoomEngine 外置（录制上传已落地 / 房间状态 Redis 外置仅设计）
- **P2** 架构与技术债：#15 巨型文件拆分、#16 mobile `any` 清理、#17 i18n 覆盖、#18 web/mobile SDK 统一、#24 样板收敛

所有改造遵循「最小安全增量 + 可回滚」原则，分批融入迭代，不一次性大重构。

---

## P1#8 媒体 RoomEngine 外置

### 8.1 录制上传对象存储（已落地，代码增量）
- 现状：`backend/internal/media/room_engine.go:30,94` 房间状态纯内存、录制落本地盘，多副本下不可水平扩展、文件随 Pod 销毁丢失。
- 增量：`StopRecording` 后若配置了 S3（复用既有 `storage.RecordingStorage`，`RECORDING_STORAGE_DRIVER=s3`），在后台 goroutine 将本地录制文件异步 `SaveFile` 到对象存储；失败仅 `logger.Warn`，**不阻断会话结束**。
- 未配置 S3 时行为完全不变（回退本地盘）。对象键取 `recordings/<roomID>/<相对baseDir路径>`。
- 验证：`cd backend && go build ./...` 通过；`go test ./internal/media/... ./internal/storage/... ./internal/collaboration/...` 通过。

### 8.2 房间状态 Redis 外置（仅设计，本次不实现代码）
- **现状**：`RoomEngine.rooms`（`room_engine.go:30`）为进程内 `map`，信令/媒体节点多副本时房间状态无法跨节点共享，需依赖 sticky LB 才能正确路由。
- **目标**：房间状态（参与者、Track、录制句柄引用）外置到 Redis，使媒体节点可水平扩展、Pod 重启不丢状态。
- **分步方案**：
  1. 引入 feature flag（如 `MEDIA_ROOM_STATE_REDIS=1`），默认关闭，保持内存态为 fallback。
  2. 新增 `RoomStateStore` 接口：`Load/Save/Delete(room)` + 订阅房间事件；Redis 实现用 Hash 存房间元数据，Track/PC 句柄等不可序列化的对象仍留本地，仅把「路由所需」的轻量状态（roomID→nodeID、参与者列表）外置。
  3. 节点注册自身 `nodeID` 到 Redis；信令层按 `roomID` 查 `nodeID` 做定向转发，解除 sticky LB 强约束。
  4. 房间变更通过 Redis Pub/Sub 或短 TTL 广播，处理并发加锁/版本号。
- **风险**：WebRTC `PeerConnection` 无法序列化，纯外置不可行，必须「状态外置 + 媒体流本地」混合；Redis 不可用需自动降级到内存态并告警。
- **回滚**：关闭 feature flag 即回退内存态，无迁移数据需要清理。
- **建议排期**：放在录制上传之后，单独迭代，先 solo 节点验证再上多副本。

---

## P2#15 巨型文件拆分

| 文件 | 规模 | 拆分方向与子域边界 |
|---|---|---|
| `backend/internal/mcpplatform/service.go` | 1673 行 | 按能力拆：MCP 安装/授权/调用/计费/审计；每子域一个 `*.go`（如 `install.go`、`billing.go`），`service.go` 仅留编排与接口聚合。 |
| `backend/internal/handlers/commercial_handler.go` | 1117 行 | 拆为订阅/订单/权益/Webhook 处理；HTTP 绑定与领域逻辑分离到 `internal/commercial/`。 |
| `mobile/src/screens/AgentDemoScreen.tsx` | 2308 行 | 拆为容器 + 子组件：对话列表、输入条、工具调用卡片、Agent 消息气泡、设置面板。 |
| `mobile/src/screens/ConversationDetailScreen.tsx` | 1759 行（实测 1792） | 拆为头部、消息流、草稿工具栏、附件选择器、成员面板。 |
| `mobile/src/context/SignalingContext.tsx` | 1190 行 | 信令连接管理 / 房间状态 / 媒体协商 拆分；连接逻辑下沉到 `lib/signaling`。 |

- 目标：单文件 < 600 行，子域边界清晰、可单测。
- 步骤：先抽纯函数与 hooks（不改行为），再移文件，最后删死代码；每步带测试。
- 风险：跨组件状态耦合，需先凝固接口再拆；用 `react-refresh` 校验不破坏热更。
- 回滚：按文件分批合并，单批出问题 revert 该文件即可。
- 排期：每迭代拆 1–2 个文件，约 3–4 迭代完成。

---

## P2#16 mobile `any` 清理

- 现状：`tsconfig` 未收紧 `strict`；`SignalingContext.tsx` ~29 处 `any`、`useWebRTC.ts` ~16 处 `any`，复用 `@allcallall/api-types`。
- 目标：开启 `strict` + `noImplicitAny` + `noUncheckedIndexedAccess`；关键路径 `any` 清零。
- 步骤：
  1. 复制当前 `tsconfig` 为 `tsconfig.strict.json` 逐步开选项，CI 并行跑，不阻塞主构建。
  2. 优先补类型清单：`SignalingContext.tsx`（信令消息联合类型）、`useWebRTC.ts`（PC/Track 事件回调类型），全部来自 `packages/api-types`（含 `schema.d.ts`、`index.ts`）。
  3. 用 `api-types` 中的 DTO 替换手写 `any`；无法推导处用 `unknown` + 类型守卫。
  4. 全量开启后替换主 `tsconfig` 并删除影子配置。
- 风险：`strict` 暴露大量隐性错误；用类型守卫避免大面积改逻辑。
- 回滚：保留 `tsconfig`（非 strict）直至全绿，分步切换。
- 排期：2–3 迭代，按文件优先级推进。

---

## P2#17 i18n 覆盖

- 现状：`web/src/i18n.ts` ~5 个 key、`mobile/src/i18n/locales/en.json` ~10 个 key，页面硬编码文案多，未形成规范。
- key 规范：`<domain>.<component>.<meaning>`（如 `call.control.mute`）；禁止中文做 key；缺失 key 在 dev 抛告警。
- 逐页覆盖路线：
  1. 先覆盖高频页：通话/房间、登录、设置、订阅（web 与 mobile 同套 key 命名）。
  2. 抽 `useTranslation()` 封装，页面只取 key，文案集中到 `en.json`/中文包。
  3. 移动端补齐 `en.json` 其余页 key，web 端补齐 `i18n.ts` 缺失域。
  4. 增加 i18n 覆盖率检查（脚本扫描硬编码中文字串）。
- 风险：key 重名/遗漏；用 lint 规则约束。
- 回滚：文案集中化纯增量，不影响功能。
- 排期：每迭代覆盖 2–3 页，约 3 迭代。

---

## P2#18 SDK 统一

- 现状：web 用 `web/src/api/http.ts`（`apiRequest` 含 401→`/auth/refresh`→retry，单一 `refreshPromise` 防抖）；mobile 用 `mobile/src/api/client.ts` 各自封装，刷新/重试逻辑不一致。
- 目标：抽 OpenAPI 生成 client + 公共 `fetch` 封装，mobile 对齐 web 的 `401→refresh→retry`。
- 步骤：
  1. 以 `docs/api/openapi.yaml` 为准，CI 生成 TS client（web/mobile 共用 `packages/api-types`）。
  2. 抽 `packages/api-client`：导出 `apiRequest` 与 `refresh` 单例（复用 web 现有 `refreshPromise` 防抖模式）。
  3. mobile `client.ts` 改为调用公共封装，删除本地重复刷新逻辑；web `http.ts` 下沉到公共包。
  4. 统一错误类型 `APIError`，401 统一走 refresh 后 `retry401:false` 重试一次。
- 风险：生成 client 与手写类型差异；先并行双实现，比对通过后切换。
- 回滚：mobile 保留旧 `client.ts` 分支，可切回。
- 排期：2 迭代，先发包再迁移。

---

## P2#24 样板收敛

- `@node_span` 装饰器：将性能/链路追踪装饰器统一收口到 `packages/telemetry`（或 backend 对应包），禁止业务文件内联打点；登记使用约定与示例。
- `lib/format.ts` 统一：web 与 mobile 各有一份日期/时长/数字格式化，收敛到 `packages/format`，两端复用同一实现，消除重复与偏差。
- `paramUint` 绑定器：路由参数 `string→uint` 解析（如 `mobile/src/screens/SessionsScreen.tsx` 等处）抽为共享 `parseUint`/`paramUint`，统一校验与默认值，替换散落 `Number()`/`parseInt`。
- signaling 粘性 LB 约束文档登记：在 `docs/deployment/meeting-room-state-protocol.md` 或本文件登记「多副本下房间状态依赖 sticky LB」的约束与后续 Redis 外置（#8.2）的解除条件。
- 风险：跨端收敛需同步发版；用 semver 包管理。
- 回滚：每收敛项独立发包，可单点回退。
- 排期：穿插在各迭代，作为「顺手收敛」项。
