# 腾讯全栈 / Agent 工具开发面试问题与参考回答

回答时先讲当前实现，再讲取舍和生产演进。不要用“高并发、分布式、企业级”代替证据。

## 1. 为什么 Go API 不直接同步调用 Python Runtime？

参考回答：创建 run 时，Go 在同一 MySQL 事务里写 `agent_runs` 和
`agent.run.requested` outbox，立即返回可查询的 run。worker 再调用 Python。
这样 HTTP 超时不会丢任务，worker 可以租约重试，run/outbox/request id 可审计。
同步调用更简单，但会把 Provider、checkpoint、Sandbox 冷启动全部叠加到请求延迟，
也很难在客户端断开后继续执行。

追问：outbox 不是消息队列，如何避免多个 worker 重复消费？

答：worker 用数据库状态、lease/attempt 和幂等 handler 协调。即使发生至少一次投递，
run dedupe、execution identity 和外部 idempotency key 仍保证副作用最多一次。生产规模可把
outbox 发布到 Kafka，但事务源仍是 MySQL outbox。

## 2. 50 个并发相同 Idempotency-Key 请求如何只创建一个 run？

参考回答：不能只在 handler 里先查再插，那是 TOCTOU。最终约束必须由数据库唯一索引
`(organization_id,user_id,conversation_id,dedupe_key)` 保证；service 对 duplicate key
读取并返回已有 run。nullable dedupe 允许没有幂等键的普通请求彼此独立。并发测试要用
相同 key 同时发 50 次，并断言 run/outbox 都只有一条。

## 3. Go 事务应该包多大？为什么不把网络调用放进事务？

参考回答：事务只覆盖必须原子提交的本地状态，例如 run + outbox、审批 decision + resume
event、工具结果 + 业务写回。Python、OpenBao、Sandbox、MCP 都是网络 IO，放进事务会长时间
持锁并放大失败。网络调用之后依赖 execution/call id 幂等地落状态，而不是依赖跨服务大事务。

## 4. embedded worker 和独立 worker 的差异是什么？

参考回答：Interview Compose 使用 embedded worker，减少现场进程和配置数量，但 worker
仍通过 outbox、lease 和 repository 合同运行。生产可设 `EMBEDDED_WORKERS=0`，用独立
`agent-worker/outbox-worker` 水平扩容。两种模式共享数据库契约，区别是故障域和资源隔离，
不是两套业务逻辑。

## 5. Go 并发中最容易出问题的点在哪里？

参考回答：一是“查再写”的幂等竞态，必须落到唯一约束；二是 worker lease 续期与超时后
被第二个 worker 接管；三是 subprocess stdout/stderr 与 `Wait()` 顺序，进程退出不代表
stream 已完整校验。`sandboxsupervisor` 先完成 stdout/stderr validation，再发送成功 exit
frame，并用高重复和 race test 验证 missing-newline 竞态。

## 6. 为什么 checkpoint 由 Python 而不是 Go 保存？

参考回答：LangGraph 节点、channel versions、pending writes 和 interrupt 恢复语义属于
Python graph。让 Go 自己序列化会复制 LangGraph 内部协议并产生双主。Go 只保存业务状态和
Python 返回的 checkpoint identity/version，resume 时带 expected version 做协调。

## 7. MySQL Checkpointer 如何保证一致性？

参考回答：`put` 和 `put_writes` 在同一事务，主键包含
`(thread_id, checkpoint_ns, checkpoint_id)`，writes 再包含 task id/path/index。
typed serializer 写入 LONGBLOB，不用 pickle。每个 thread 有原子 version；stale expected
version 返回 409，事务回滚，原 checkpoint 指纹不变。同步/异步 API 共用同一约束。

## 8. 为什么禁止默认 thread_id？

参考回答：默认 thread 会让不同用户或 run 共用 checkpoint namespace，是典型跨租户串读。
当前只允许 `agent:{run_id}` 或 `workflow:{run_id}`，并同时校验 organization、user、
conversation、run scope。thread id 是隔离键之一，不是唯一权限凭据。

## 9. 审批为什么要用 LangGraph interrupt，而不是 Go 暂停一个状态字段？

参考回答：interrupt 把“暂停在哪个节点、完整 tool call、参数摘要、approval_request_id”
放进 checkpoint，Pod 重启后能精确恢复。Go 仍是审批决策权威，保存谁在何时批准了哪个
revision/tool/checkpoint。resume 同时验证 complete decision set 和 expected version，避免
把旧审批应用到新图状态。

## 10. 为什么 Python 不能直接调用 MCP？

参考回答：Python prompt/graph 层最接近不可信模型输出，如果它持有 Vault 或网络权限，
prompt injection 就可能直接变成副作用。Python 只拿授权且固定 revision 的 catalog，执行
回到 Go Gateway。Go 校验组织、审批、capability 和 execution identity，再调 Sandbox。
这让业务权限与模型推理分离。

## 11. MCP readOnlyHint 能直接信任吗？

参考回答：不能。平台把工具风险固化为 `read|write|unknown`，验证过的 read 才可自动执行；
unknown 和 write 都必须审批。第三方 annotation 是输入信号，不是授权事实。管理员发布时
创建不可变 revision，后续 schema/risk 变化必须新 revision。

## 12. Capability JWT 比共享 bearer token 好在哪里？

参考回答：共享 token 一旦泄漏就能调用全部工具。Ed25519 capability 是短期、可离线验证的
最小权限凭证，claims 绑定 org/user/conversation/run、installation revision、tool 集合、
audience 和 expiry。Runner 拒绝跨组织、过期、audience 错误和 revision mismatch。
私钥只在 Go，Runner 只有公钥。

## 13. HTTPS MCP 如何防 SSRF 和 DNS rebinding？

参考回答：生产默认只接受 TLS，解析 hostname 后拒绝 loopback、link-local、私网等地址，
限制 redirect，并把实际连接目标固定到校验过的解析结果；每次 redirect/重解析都重新校验。
Interview 环境是显式例外，只允许 `interview-mcp` 这个精确 DNS 名和本地 CA，拒绝 IP、
wildcard 与 redirect；非 interview 环境配置该开关直接拒绝启动。

## 14. Secret 如何从 OpenBao 到 MCP，而不进入日志和数据库？

参考回答：数据库只存 Vault path。执行时 Go 获取一次性、约 60 秒的 response-wrapping
token；Runner 在 tmpfs 解包，把 header 注入 MCP request。checkpoint、trace、receipt 和日志
不保存 secret value。smoke 会搜索服务日志和 MySQL dump，命中 bearer token 就失败。

## 15. “最多一次副作用”是怎样跨三层实现的？

参考回答：Go run 用 dedupe key；tool call 用 `(run_id,call_id)`；MCP execution 用稳定
execution id；Interview MCP 的 SQLite 对 `idempotency_key` 唯一。任何一层重试都返回已有
结果。不能只靠 HTTP retry policy，也不能只靠外部 MCP，因为本地审计和外部状态都需要
各自的唯一身份。

## 16. 如果 MCP 已成功但 Go 在写 succeeded 前崩溃怎么办？

参考回答：这是经典 ambiguous outcome。Go 重试相同 execution id，Runner/MCP 使用相同
idempotency key 返回原 ticket/receipt，Go 再补写 succeeded。若第三方工具不支持幂等，
平台只能把它标为高风险并通过查询/对账补偿，不能虚假承诺 exactly-once。

## 17. 为什么 Elasticsearch 用 IK，Python grounding 用 jieba？

参考回答：两者解决不同阶段。IK 在 ES analyzer 中负责中文倒排索引和召回；jieba 在 Python
grounding 中把 answer/citation 分词，计算有意义 token/claim coverage。不能把“有 citation”
直接当 grounded，也不能用字符 substring 代替中文语义覆盖。smoke 同时验证 IK `_analyze`、
真实索引命中，以及 jieba 正负例。

## 18. RAG source filter 如何避免越权回退？

参考回答：`source_types` 是硬约束。检索不到 knowledge 时不能悄悄回退到 message/memory，
否则上层以为拿到授权知识，实际泄漏其他来源。Go 查询先带 organization/conversation scope，
Python `filter_chunks` 再防御性过滤；无命中就返回 insufficient，而不是放宽来源。

## 19. 为什么 rules Provider 不是“假 Agent”？

参考回答：它是可替换 provider seam 的确定性实现，用来验证 orchestration、tool schema、
approval、checkpoint、RAG 和故障恢复，不用外部 key。真实 Provider 只替换结构化决策来源，
不改变 side-effect 边界。面试时应说“rules 证明系统语义”，不能说它证明真实模型质量。

## 20. Web 如何避免 Go/Python/前端 DTO 漂移？

参考回答：OpenAPI 是公共 HTTP contract，生成 `@allcallall/api-types`，Web/Mobile 复用；
CI 重新生成后检查 git diff，并运行 handwritten client contract checker。Python 内部模型仍有
独立 typed contract，例如 Go 数值 DB ID 在 shared `ContextChunk` 输入边界显式规范化为
字符串，避免依赖 Pydantic 隐式转换。

## 21. WebSocket 重连为什么要保存 cursor？

参考回答：每次用 `since_id=0` 重连会重复全量 replay，既增加负载又让 UI 重复消费事件。
客户端持久化最近 event id，重连从 cursor 继续；服务端仍按 organization/user scope 过滤并
保证 ID/sequence 单调。cursor 是性能优化，幂等消费仍要处理重复。

## 22. 你会如何观测这个链路？

参考回答：Go 看 HTTP、outbox、approval、execution；Agent 看 run/resume duration、checkpoint
replay/conflict/busy；Runner 看 validate/execute/timeout/unwrap failure；RAG 看 query/grounding。
trace 关联 run id、request id、execution id、checkpoint version 和 revision。Compose 直接检查
`/metrics`；生产接 OTel/Prometheus，并对 conflict、timeout、tenant deny、secret unwrap 告警。

## 23. 为什么 Interview Compose 不把 gVisor 作为前置条件？

参考回答：Docker Desktop 不稳定支持生产 RuntimeClass，强行依赖会让面试演示不可复现。
Compose 验证协议、权限、TLS、secret 和恢复语义；Helm 模板表达 non-root、read-only rootfs、
seccomp、capability drop、NetworkPolicy 和 gVisor。安全设计保留，但不把模板验证冒充运行验证。

## 24. 当前项目最需要诚实承认的不足是什么？

参考回答：rules eval 不代表开放域模型质量；OpenBao 是 dev mode；Interview 私网 trust 是
显式例外；Compose 没证明多副本和真实 gVisor；外部 MCP 是否真正 exactly-once 取决于对方
幂等能力；当前 metrics 是进程内 counter，重启会归零。下一步应在真实 K8s staging 做
多 Pod checkpoint conflict、网络策略、资源耗尽和持久化 Prometheus 验证。

## 25. 如果让你把它部署到生产，前三件事是什么？

参考回答：第一，外部托管 MySQL/Redis/OpenBao，密钥轮换和备份恢复演练；第二，独立 worker
与 Sandbox namespace，上 NetworkPolicy/gVisor、资源配额、镜像扫描和 admission policy；
第三，端到端 OTel、SLO 和故障演练，重点覆盖 ambiguous MCP outcome、checkpoint conflict、
租户拒绝和 secret 泄漏。真实 Provider 只在这些基础边界稳定后逐租户灰度。
