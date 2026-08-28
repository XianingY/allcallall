# AllCallAll API — Kubernetes 多可用区部署

面向生产的 K8s 部署清单，**跨可用区（AZ）容灾**为核心设计目标。所有资源归属
`allcallall` namespace。

## 资源清单

| 文件 | 作用 |
| --- | --- |
| `namespace.yaml` | Namespace + ResourceQuota + 关键负载 PriorityClass |
| `configmap.yaml` | 非敏感运行时配置（env-driven 开关、部署元数据） |
| `secret.yaml` | 敏感凭据模板（**生产务必用 SealedSecret/ExternalSecrets/Vault**） |
| `deployment.yaml` | 多副本 Deployment，跨 AZ 打散 + 安全基线 |
| `service.yaml` | ClusterIP Service + PodDisruptionBudget（minAvailable=2） |
| `hpa.yaml` | 基于 CPU / 内存 / QPS 的 HPA（min 3 / max 20） |

## 多 AZ 容灾设计

1. **拓扑打散**：`topologySpreadConstraints` 按 `topology.kubernetes.io/zone`
   硬约束（`DoNotSchedule`）保证副本均匀分布在多个 zone，`maxSkew=1`。
2. **反亲和**：`podAntiAffinity` 尽量不让多个副本调度到同一节点。
3. **可用性底线**：HPA `minReplicas=3` + PDB `minAvailable=2`，单 zone
   故障（1/3 副本丢失）仍维持 2 个可用副本，不触发服务降级。
4. **探针区分**：`readinessProbe → /ready`（依赖 mysql/redis 探针，失败仅摘流量），
   `livenessProbe → /health`（仅进程存活，失败才重启）。
5. **优雅终止**：`preStop sleep 10` + `terminationGracePeriodSeconds=30` 保证在途请求完成。

## 部署步骤

```bash
# 1. 注入真实 Secret（使用你们的安全流程，勿提交明文）
kubectl apply -f secret.yaml   # 仅模板，CI 中替换为加密来源

# 2. 依次应用
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f hpa.yaml

# 3. 验证跨 AZ 分布
kubectl -n allcallall get pods -o wide \
  -l app.kubernetes.io/name=allcallall-api \
  --field-selector=status.phase=Running \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.nodeName}{"\n"}{end}'
```

## 外部依赖（MySQL / Redis）

生产环境建议使用托管服务（如云厂商 RDS 多 AZ + Redis Cluster），其高可用由
云平台保障。开发/自托管高可用拓扑见 [`../ha`](../ha)（Group Replication + ProxySQL、
Redis Sentinel）。无论哪种，后端通过 `DB_HOST` / `REDIS_ADDR` 环境变量接入，部署
清单无需改动。

## 前置依赖

- 集群节点需打 `topology.kubernetes.io/zone` 标签（云厂商托管集群默认具备）。
- HPA 自定义指标需部署 [Prometheus Adapter](https://github.com/kubernetes-sigs/prometheus-adapter)，
  将 `http_requests_total`（来自 `/api/v1/metrics`）暴露为 `http_requests_per_second`。
- 对外暴露经 Ingress / Gateway，并前置 WAF（见 [`../waf`](../waf)）。
