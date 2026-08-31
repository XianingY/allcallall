# AllCallAll — 全链路压测

基于 [k6](https://k6.io) 的端到端压测脚本，覆盖真实用户关键路径，用于上线前容量
评估、版本回归基准与 SLA 红线校验。

## 覆盖链路

| 阶段 | 接口 | 模拟行为 |
| --- | --- | --- |
| status-probe | `GET /api/v1/status` | 监控页存活（对外 SLA 探针） |
| chat-flow | `POST /api/v1/chat/conversations` | 发起聊天会话 |
| agent-run | `POST /api/v1/agent/runs` | 触发 Agent 运行（最重路径） |
| knowledge-search | `GET /api/v1/knowledge/search` | 知识库检索（RAG 向量召回） |
| usage-dashboard | `GET /api/v1/commerce/usage` | 组织用量看板 |

每个 VU 使用独立身份（`loadtest-<id>`），模拟多租户并发，避免单租户缓存/限速
干扰基线。

## 运行

```bash
# 安装
brew install k6   # 或见 https://k6.io/docs/get-started/installation/

# 最小运行（本地）
k6 run -e BASE_URL=http://localhost:8080 \
       -e AUTH_TOKEN=<dev-token> \
       infra/loadtest/loadtest.js

# 生产容量评估（高并发 + 渐进负载）
k6 run -e BASE_URL=https://api.allcallall.example.com \
       -e AUTH_TOKEN=<test-token> \
       -e USER_COUNT=500 \
       --stage 2m:100,5m:500,2m:500,1m:0 \
       infra/loadtest/loadtest.js
```

## SLA 红线（thresholds）

脚本内置退出阈值，未达标即 `k6` 以非 0 退出，可被 CI 阻断发布：

- `http_req_duration`: p95 < 800ms，p99 < 1500ms
- `http_req_failed`: 传输层错误率 < 1%
- `app_errors`: 业务错误（非 2xx/3xx）< 2%

## 与 SRE 的结合

- 压测期间观察 `/api/v1/metrics`（Prometheus）的 `http_requests_total`、
  `go_goroutines`、`process_resident_memory_bytes`，定位瓶颈。
- 配合 HPA（`infra/k8s/hpa.yaml`）：在 plateau 阶段确认副本能按 CPU/QPS 自动扩容，
  且新副本跨 AZ 打散（`topologySpreadConstraints`）。
- 压测后检查 SLA 状态页历史与各组件 latency，写入 Phase 4 的容量评审记录。

## 注意事项

- `AUTH_TOKEN` 必须是**测试租户**的有效令牌；勿用生产账号。
- 大规模压测前先在边缘 WAF 将压测来源 IP 加入白名单，避免被 `rate-limit` 规则误伤
  （见 `infra/waf`）。
- 向量检索（knowledge-search）依赖 RAG 后端，压测峰值应与 RAG 服务的容量对齐。
