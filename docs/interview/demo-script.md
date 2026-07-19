# AllCallAll 五分钟面试演示脚本

目标不是把所有功能点一遍，而是用一条真实链路回答五个问题：解决什么业务问题、
为什么拆 Go/Python、工具为什么不能直连、故障后如何恢复、哪些能力仍只是生产设计。

## 演示前准备

```bash
make interview-demo
```

命令完成后使用终端打印的账号登录 `http://localhost:3000/agent-tools`。默认 rules
Provider 不依赖外部模型或网络；不要提前手工改数据库或准备 ID。

备用检查：

```bash
make interview-status
make interview-smoke
```

## 0:00-0:45 业务问题

打开“Agent 工具”页，展示 `Interview Support MCP`、organization scope、active
revision 和三个工具。

建议表述：

> 这个项目解决的是企业 Agent 接第三方工具后的权限、审批、恢复和审计问题。
> 普通聊天 Demo 只证明模型会调用函数；这里要证明写操作不能绕过业务权限，进程重启后
> 能从 checkpoint 恢复，而且重复请求不会创建第二个外部工单。

指出 `lookup_policy/get_ticket` 是 read，`create_support_ticket` 是 write。所有工具名
都是 `mcp.{installation_id}.{tool_name}`，revision 和风险由 Go 平台固化。

## 0:45-1:40 架构边界

打开 [主演示链路](interview-chain.md) 的架构图。

建议表述：

> Go/MySQL 是用户、组织、审批、审计和工具执行的权威；Python/LangGraph 是 graph
> 节点和 checkpoint 的权威。Python 只能拿到 Go 授权后的 catalog，执行也必须回到
> Go Tool Gateway，再经过 Sandbox Runner。这样模型层没有 Vault、Sandbox 或 MCP
> 的直连权限。

解释 outbox：创建 run 和写 `agent.run.requested` 在同一业务事务；worker 失败可以重试，
handler 不等待长耗时 Agent。

## 1:40-2:30 只读工具链路

在 `lookup_policy` 行点击“Agent Lab”。页面自动带入 conversation、ReAct 模式和目标，
点击“启动 ReAct”。

展示：

- run/step/tool trace；
- MCP installation、revision 和 execution result；
- 工具结果来源标记为不可信 MCP 数据；
- read 工具不需要审批。

建议表述：

> 默认规则 Provider 不是为了假装智能，而是为了让现场演示可重复。切换真实
> OpenAI-compatible Provider 后仍走同一结构化工具契约。MCP 返回只作为不可信数据，
> 不能改写 system policy 或风险等级。

## 2:30-3:40 写工具、审批和恢复

回到工具页，在 `create_support_ticket` 行进入 Agent Lab，启动 ReAct。展示
`requires_action`、checkpoint id/version、revision、风险和审批原因。

此时在终端运行：

```bash
make interview-chaos
```

脚本会重启 Agent Runtime、批准暂停中的请求并验证恢复。关键成功行：

```text
write chain passed: ... resumed from checkpoint, execution=1, external_tickets=1
```

建议表述：

> LangGraph interrupt 先把 approval identity、参数摘要和 tool identity 写入 MySQL
> checkpoint；Go 再保存审批。批准后 Go 带 expected checkpoint version 调 resume。
> Python 原子推进版本后，Go 才执行外部工具。MCP SQLite 对 idempotency key 唯一，
> Go execution 对 run/call 唯一，所以重试不产生第二次业务副作用。

## 3:40-4:25 中文检索与可观测性

终端运行：

```bash
make interview-smoke
```

说明 smoke 不是只做 health check，它还验证：

- Elasticsearch 的 `ik_smart` 能拆分“腾讯全栈 Agent 工具审批流程”；
- 中文知识源通过公共 API 创建并被 search-worker 索引；
- RAG `source_types=[knowledge]` 不泄漏 message/memory；
- jieba grounding 正例通过、无关“预算已批准”负例拒绝；
- Go、Agent、RAG、Runner 的 metrics endpoint 均包含预期 counter；
- service log 和 MySQL dump 中不存在 MCP bearer token。

## 4:25-5:00 取舍与诚实边界

建议表述：

> Compose 为了现场稳定，使用 OpenBao dev mode、显式 interview 私网 DNS trust 和
> embedded worker；生产设计使用外部 OpenBao、独立 worker、NetworkPolicy 和 gVisor。
> 我没有把 Kubernetes 模板说成已经跑过的商业集群，也没有把 rules fixture 的 100%
> 说成真实模型准确率。当前能证明的是链路、事务、幂等、安全失败和恢复语义。

最后给出最重要的工程取舍：

1. MySQL 同时承载业务事务和 checkpoint，牺牲部分解耦，换来面试规模下可证明的一致性。
2. Go 掌握 side effect，Python 只掌握 graph，减少动态运行时越权面。
3. 默认 rules Provider 保证离线确定性，真实 Provider 必须显式启用并失败关闭。
4. Compose 证明链路；Kubernetes/gVisor 证明生产隔离设计，两者不混为一谈。

## 演示失败时的恢复顺序

```bash
make interview-status
make interview-smoke
docker compose --env-file /tmp/allcallall-interview-${USER}/interview.env \
  -f infra/docker-compose.yml -f infra/docker-compose.interview.yml logs --tail=100
```

优先讲清失败点，不要现场绕过审批或改库。所有阶段提交都已推送，可以按 commit 回退。

## 演示后清理

```bash
make interview-down
```
