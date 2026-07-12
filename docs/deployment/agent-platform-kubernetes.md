# Agent 平台 Kubernetes 部署

本指南对应 `infra/helm/allcallall`。Chart 只部署无状态工作负载，MySQL、Redis 和 OpenBao 必须由外部托管服务或独立运维栈提供。生产集群需要 Kubernetes 1.27 以上、支持 NetworkPolicy 的 CNI、Ingress Controller、Metrics Server，以及名为 `gvisor` 的 RuntimeClass。

## 部署拓扑

Chart 包含 API、Agent worker、Outbox worker、Agent Runtime、RAG Runtime、Sandbox Control Plane、Sandbox Runner、Web、migration Job。每个工作负载使用独立 ServiceAccount，默认不挂载 Kubernetes API token。所有 Deployment 都包含资源 requests/limits、HPA 和 PDB。

Sandbox Control Plane 与 Runner 默认指定 `runtimeClassName: gvisor`，以非 root 用户运行，根文件系统只读，禁用提权，丢弃全部 Linux capabilities，并使用 `RuntimeDefault` seccomp。临时数据和一次性 secret 只进入有容量上限的 memory-backed `emptyDir`；Chart 不创建 `hostPath` 或其他 host mount。

## 前置检查

确认 RuntimeClass 和 NetworkPolicy 能力：

```bash
kubectl get runtimeclass gvisor
kubectl api-resources | grep networkpolicies
kubectl -n kube-system get pods -l k8s-app=kube-dns
```

云厂商的 gVisor RuntimeClass 名称不同或由节点池限定时，在 values 中同时修改 `components.sandboxControlPlane.runtimeClassName` 和 `components.sandboxRunner.runtimeClassName`，并使用 `nodeSelector`、tolerations 将 Pod 调度到对应节点池。Chart 不创建 RuntimeClass，因为 handler 配置属于集群基础设施，而不是应用发布的一部分。

## 外部依赖与 Secret

默认 values 引用 `allcallall-external` Secret，但不会创建它。推荐通过 External Secrets、Secrets Store CSI 或 GitOps 加密 Secret 管理；不要把凭据写入 values 或提交到仓库。Secret 需要包含：

- `mysql-dsn`：Go 使用的 MySQL DSN。
- `checkpoint-mysql-dsn`：Python checkpointer 使用的 `mysql://` DSN。
- `sandbox-receipt-mysql-dsn`：Sandbox Control Plane 仅用于执行回执的 MySQL DSN；生产账号应只拥有 `sandbox_execution_receipts` 的读写权限。
- `mcp-capability-ed25519-private-key`：API 与 Agent worker 签发短期工具 capability 的共享 Ed25519 私钥；使用 base64 编码的 32 字节 seed 或 64 字节私钥。
- `sandbox-capability-ed25519-public-key`：与上面私钥匹配的 base64 编码 32 字节 Ed25519 公钥；Sandbox Control Plane 只注入此公钥，不持有私钥。Chart 当前始终部署 Sandbox，因此即使 rollout 阶段关闭 MCP/Sandbox feature flag，该 key 也必须存在。
- `redis-password`：Redis 密码，可为空但 key 应存在。
- `openbao-token`：Go 控制面访问 OpenBao 的凭据；生产应使用短期 Kubernetes Auth 凭据替代长期 root token。

临时环境可将敏感值先放进 shell 环境，再创建 Secret：

```bash
eval "$(./scripts/development/generate-agent-capability-keypair.sh)"
kubectl create namespace allcallall
kubectl -n allcallall create secret generic allcallall-external \
  --from-literal=mysql-dsn="$ALLCALLALL_MYSQL_DSN" \
  --from-literal=checkpoint-mysql-dsn="$ALLCALLALL_CHECKPOINT_MYSQL_DSN" \
  --from-literal=sandbox-receipt-mysql-dsn="$ALLCALLALL_SANDBOX_RECEIPT_MYSQL_DSN" \
  --from-literal=mcp-capability-ed25519-private-key="$MCP_CAPABILITY_ED25519_PRIVATE_KEY" \
  --from-literal=sandbox-capability-ed25519-public-key="$SANDBOX_CAPABILITY_ED25519_PUBLIC_KEY" \
  --from-literal=redis-password="$ALLCALLALL_REDIS_PASSWORD" \
  --from-literal=openbao-token="$ALLCALLALL_OPENBAO_TOKEN"
```

`generate-agent-capability-keypair.sh` 使用 OpenSSL 生成临时 Ed25519 PEM，再输出应用接受的 matching 32 字节 seed/public key，临时 PEM 会立即删除。不要分别生成两个值。`MCP_PLATFORM_ENABLED=true` 时私钥必填；`SANDBOX_EXECUTION_ENABLED=true` 时 API 与 Agent worker 还会在启动时校验 public key 是否由该私钥派生，缺失或错配均拒绝启动。Helm 默认注入 `APP_ENV=production`，由 migration job 独占 schema 迁移职责。

API/worker 调用 Sandbox Control Plane 的 validate、execute 和 lookup 时，每次签发独立的 30 秒 Ed25519 JWT。该 JWT 使用 `allcallall-sandbox-control-plane` audience，与 Agent 工具 capability audience 隔离，并绑定 method、escaped path、request digest、`iat/nbf/exp/jti`。authorization digest 包含本次实际 `secret_wrap_token`，防止截获 token 后替换 wrapping token；durable receipt 的幂等 digest 则仍排除该短期 token。除 `/health` 外，Control Plane 对缺失、过期、跨操作或摘要不匹配的 JWT 统一返回 401。

OpenBao 中只保存用户 MCP 凭据，数据库只保存 Vault path。Runner 应只接收一次性、60 秒 response-wrapping token，在 `/run/secrets` tmpfs 中解包，并在执行结束后清空；任何日志、trace 或 Kubernetes Event 都不得包含 secret value。

## NetworkPolicy

`networkPolicy.enabled=true` 时，Chart 先对所有平台 Pod 设置 ingress/egress default deny，再仅放行以下链路：

- Ingress Controller 到 Web/API。
- Web、Agent Runtime、RAG Runtime 到 API。
- API/Agent worker 到 Agent/RAG Runtime 和 Sandbox Control Plane。
- Agent Runtime 到 RAG Runtime。
- Sandbox Control Plane 到 Runner 和回执 MySQL；Runner 本身没有数据库凭据或数据库出口。
- DNS 到指定 kube-dns Pod。

外部出口必须在 `external.*.egressCIDRs` 中显式配置。MySQL、Redis、OpenBao、模型 Provider、MCP endpoint 和镜像 Registry 分开维护 CIDR，Runner 不继承 API 的出口。空列表表示禁止该类出口，不表示允许所有地址。

标准 Kubernetes NetworkPolicy 不能稳定按 FQDN 放行。托管服务 IP 会变化时，应使用 Cilium/Calico 的 FQDN policy，并继续保留 Chart 的 default deny；不要用 `0.0.0.0/0` 绕过控制。HTTPS MCP 仍须由 Sandbox 做 TLS、重定向、DNS rebinding 和私网地址校验。

## 安装与验证

从示例生成环境 values，只填写地址、Secret 名称、CIDR、Ingress 和镜像版本：

```bash
cp infra/helm/allcallall/values-production.example.yaml /tmp/allcallall-values.yaml
helm lint infra/helm/allcallall
helm template allcallall infra/helm/allcallall \
  --namespace allcallall \
  -f /tmp/allcallall-values.yaml | kubeconform -strict -summary -ignore-missing-schemas
helm upgrade --install allcallall infra/helm/allcallall \
  --namespace allcallall --create-namespace \
  -f /tmp/allcallall-values.yaml \
  --atomic --timeout 15m
```

Migration Job 是 `pre-install,pre-upgrade` hook，失败时 Helm 不推进新版本。发布后检查：

```bash
kubectl -n allcallall get deploy,job,hpa,pdb,networkpolicy
kubectl -n allcallall get pods -l app.kubernetes.io/component=sandbox-runner \
  -o jsonpath='{range .items[*]}{.metadata.name}{" runtime="}{.spec.runtimeClassName}{"\n"}{end}'
kubectl -n allcallall port-forward svc/allcallall-allcallall-api 8080:8080
curl --fail http://127.0.0.1:8080/api/v1/health
curl --fail http://127.0.0.1:8080/api/v1/metrics
```

## Feature flag 发布顺序

首轮升级先设置三个新能力为 false：

```yaml
features:
  mcpPlatformEnabled: false
  sandboxExecutionEnabled: false
  langgraphMysqlCheckpointEnabled: false
  legacyGoRuntimeEnabled: true
```

完成 additive migration 后依次启用 MySQL checkpoint shadow mode、Sandbox 只读工具、写工具审批、MCP 发布。每一步至少观察 checkpoint 冲突/恢复、Sandbox 冷启动与执行延迟、审批等待、超时、配额拒绝、租户拒绝和 secret unwrap 失败，再进入下一步。legacy Go Runtime 仅处理不含 MCP 的旧 run，并保留一个发布版本作为紧急回退。

紧急回退时先关闭 `SANDBOX_EXECUTION_ENABLED` 和 `MCP_PLATFORM_ENABLED`，阻止新工具执行；checkpoint 数据仍由 MySQL 保留，避免回退过程覆盖 Python 图状态。不要回滚 additive migration。恢复后再逐项重新打开 flag。

### Runtime 权威边界与审批恢复

Go/MySQL 是业务对象、权限、审批决定、审计和工具副作用的权威；Python/LangGraph 是图节点进度和 checkpoint payload 的权威。两者只通过带 `execution_id`、run scope 和 `expected_checkpoint_version` 的版本化请求协调，不能从本层状态反向覆盖另一层。线程名固定为 `agent:{run_id}` 或 `workflow:{run_id}`，禁止缺省 thread。

Go 在第一次调用 Python 前持久化去除短期 capability 的规范化 runtime request。发生“Python 已提交 checkpoint、Go 尚未保存响应”的进程崩溃时，worker 必须用相同的 immutable request 和 `execution_id` 重试，不能重新读取已经变化的会话上下文拼装请求。Python 对相同 execution 返回已提交结果；payload 漂移、scope 不匹配或 checkpoint 版本冲突均返回 `409`，Go 必须 fail closed。

审批恢复顺序固定如下：

1. Python 用真实 LangGraph `interrupt()` checkpoint 确定性的 approval request、tool call ID、参数及摘要，并返回 `requires_action`。
2. Go 保存 checkpoint 元数据、审批记录，以及 MCP installation/revision/tool ID。审批 API 只事务化记录决定；当前审批集合完整后写 outbox，不在 HTTP 请求中执行工具。
3. Agent worker 消费 outbox，携带完整决定集调用 Python resume；Python 校验 scope、approval request 和预期版本后使用 `Command(resume=...)` 推进 checkpoint。
4. Go 通过 checkpoint CAS 后才执行已批准工具。拒绝项不执行；本地业务写和 tool-call 完成状态在同一 MySQL 事务提交。MCP 必须匹配审批时固定的 revision，漂移时拒绝执行。

`runtime_owner` 在 run 创建时即固定且不可变。只有 owner 为 `legacy_go` 的旧 run 可以进入 legacy engine；owner 为 `python_langgraph` 的 run 在重试和恢复期间始终绑定 Python，即使 Runtime 暂时不可用也不能降级，应保留状态并告警，待兼容 Runtime 恢复。

### Sandbox 回执与崩溃恢复

Sandbox Control Plane 以 `execution_id` 为主键，在调用 Runner 前先创建 durable receipt。请求摘要绑定 organization、user、run、installation revision、tool 和参数，但明确排除一次性 `secret_wrap_token`；相同 execution 和摘要只允许一个调用者进入 Runner，摘要漂移返回冲突。Runner 终态必须先写 receipt，再返回给 Go。Go 在重复执行、执行查询和后台扫描中读取 receipt，并以 CAS 将 `mcp_executions` 从 active 状态推进到终态；首次终态与 `mcp.execution.terminal` outbox 在同一业务事务提交。

父 Agent/Workflow 若仍持有旧 worker lease，terminal handler 会把恢复事件延迟到 lease 到期。独立 recovery sweep 同时扫描所有已过期的 running run，按旧 lease token 生成幂等 outbox，覆盖 worker 在发布后续事件前崩溃的通用窗口。API embedded worker 和独立 Agent worker 都必须启用 MCP reconciliation 与 run recovery，且使用同一套 MCP、Sandbox、OpenBao 和 capability 配置。

任意第三方 MCP 副作用与平台 MySQL 不能形成原子事务。如果 Runner 已产生副作用、但 Control Plane 在 receipt 终态落库前崩溃，过期 receipt 会进入 `outcome_unknown`，Go 将其表现为超时并告警，禁止自动重放。只有当 Runner 或下游 MCP 能按 `execution_id` 持久幂等时，才可以把该窗口提升为 exactly-once；平台默认承诺是不产生自动二次副作用。

升级前需要处理旧版“模拟审批”产生的暂停 run。此类 checkpoint 没有真实 interrupt，不能由新版 resume endpoint 恢复；先在旧版本完成或取消，必要任务升级后创建新 run。应用紧急回退时 Go 与 Python 镜像必须成对回退，并保留 additive v3/v4 schema、checkpoint 与 Sandbox receipt 数据；已由新图接管的 run 继续绑定兼容版本直至完成或显式取消。CI 的 `up -> down -> up` 只验证隔离环境中的迁移可逆性，不代表生产应执行破坏性 down migration。

### Checkpoint 一致性边界

MySQL saver 将单次 `graph.invoke` 产生的新 checkpoint 与 pending writes 放入有界内存 buffer，并在 invocation 成功返回后通过一次 MySQL 事务提交。提交前其他连接看不到部分数据；版本冲突或任一 checkpoint/write 插入失败时，namespace version 和所有 payload 一并回滚。单次 invocation 最多缓存 256 个 checkpoint、4096 条 write 和 16 MiB 序列化 payload，超限返回 `checkpoint_transaction_too_large`。

当前保证是 execution 边界原子性，不是每个 LangGraph 节点单独持久化。Pod 在 invocation 中途退出时会从上一次已提交 execution 重新执行本轮节点；Go 工具网关仍须依靠确定性 `(run_id, call_id)` 防止副作用重复。若后续工作流需要长时间节点级恢复，需要固定 LangGraph 版本并在 Pregel superstep 边界增加 checkpoint bundle hook，不能退回 `put` 和 `put_writes` 分别提交。

## 限额、保留与可观测性

默认值与产品约束一致：个人安装 5 个、组织发布安装 20 个、用户并发 2、组织并发 10、单次 30 秒、0.5 CPU、512 MiB、输出 256 KiB。checkpoint 和 tool payload 保留 30 天，组织审计保留 180 天。删除用户或安装时必须先撤销 OpenBao secret，再清理执行 payload 与 checkpoint。

API 的 `/api/v1/metrics` 与 RAG Runtime 的 `/metrics` 带 Prometheus scrape annotation。设置 `observability.otlpEndpoint` 后，Go API/worker 使用各自 `OTEL_SERVICE_NAME` 导出 trace；同时必须在 `observability.otlpEgressCIDRs` 放行 Collector。告警至少覆盖 checkpoint 冲突率、恢复失败、MCP p95、冷启动 p95、审批积压、Sandbox 失败/超时、配额拒绝、跨租户拒绝和 secret unwrap 失败。
