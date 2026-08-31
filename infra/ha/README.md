# AllCallAll 高可用部署（MySQL / Redis）

本目录提供可直接运行的 **自托管高可用** 参考实现；生产环境优先选择云厂商托管
（阿里云 RDS / 腾讯云 TencentDB / AWS RDS）或 Kubernetes Operator（mysql-operator / Vitess）。

## MySQL — 3 节点 Group Replication + ProxySQL

- `mysql-ha-compose.yaml`：3 个 MySQL 8.0 节点组成 **单主 Group Replication**，
  同步复制 → 节点故障零数据丢失；ProxySQL 做读写分离与主库自动 failover。
- 首次启动需手动 bootstrap：在 `mysql1` 执行
  `SET GLOBAL group_replication_bootstrap_group=ON; START GROUP_REPLICATION;`
  再在其余节点 `START GROUP_REPLICATION;` 加入集群。
- 生产建议：跨可用区部署 3 节点（如 az-a / az-b / az-c），ProxySQL 前置 2 副本。

## Redis — 3 主 + 3 Sentinel

- `redis-ha-compose.yaml` + `sentinel.conf`：Sentinel 监控主节点，故障时自动
  promote 副本并重配客户端。应用须使用 Sentinel-aware 客户端（或由 Sentinel
  发现主节点后连接）。
- 持久化开启 AOF（`appendonly yes`），密码鉴权。

## 备份

见 `../backup/backup.sh`：逻辑全量 + RDB 拷贝，本地保留 30 天，可选 rclone/scp
异地归档。由 systemd timer 或 cron 每小时触发。

## 安全

- 所有密码经环境变量注入（`MYSQL_ROOT_PASSWORD` / `REDIS_PASSWORD`），不落库到镜像。
- 仅 ProxySQL(6033)/Redis(6379) 暴露端口；MySQL 内部 3306 不对外。
- 生产前叠加：传输层 TLS、网络策略（仅允许应用网段访问数据层）、定期备份演练恢复。
