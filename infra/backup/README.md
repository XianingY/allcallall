# AllCallAll 自动化备份

## 三个脚本，按部署形态选一个

| 部署形态 | 脚本 | 调度方式 |
|---|---|---|
| **Kubernetes**（Helm chart） | `infra/helm/allcallall/scripts/backup.sh` | CronJob，随 chart 安装即生效（`values.yaml` 的 `backup` 段） |
| **Docker Compose** | `scripts/backup/backup.sh`（通过 `docker exec` 取容器内数据） | 需自行挂 cron / systemd timer |
| **裸机 / 同机部署** | 本目录的 `backup.sh`（直连 `MYSQL_HOST`，含 Redis RDB 拷贝） | systemd timer（见下） |

Kubernetes 版刻意**不做 Redis 备份**：Redis Pod 有独立文件系统，从备份容器里拷贝 RDB 永远拿到空文件——看起来成功、恢复时什么都没有。Redis 的持久化由 StatefulSet 的持久卷 + AOF 负责，用卷快照恢复。

下列说明针对**本目录的裸机版脚本**。

`backup.sh` 执行：

1. **MySQL** — `mysqldump --single-transaction`（一致快照，不锁表），按 gzip 压缩。
2. **Redis** — 拷贝在线 RDB 快照文件。
3. **归档** — 打包为 `allcallall-<UTC时间戳>.tar.gz` 至 `BACKUP_DIR`。
4. **轮转** — 删除超过 `RETAIN_DAYS`（默认 30）天的旧归档。
5. **异地** — 若设置 `OFFSITE_CMD`，将归档复制至远端（rclone/scp），失败仅告警不阻断。

## 调度（systemd timer 示例）

```ini
# /etc/systemd/system/allcallall-backup.timer
[Unit]
Description=AllCallAll hourly backup

[Timer]
OnCalendar=hourly
Persistent=true

[Install]
WantedBy=timers.target
```

```ini
# /etc/systemd/system/allcallall-backup.service
[Service]
Type=oneshot
User=backup
EnvironmentFile=/etc/allcallall/backup.env
ExecStart=/usr/local/bin/backup.sh
```

`/etc/allcallall/backup.env` 中设置 `MYSQL_*`、`REDIS_*`、`BACKUP_DIR`、
`OFFSITE_CMD="rclone copy %s remote:allcallall-backups"`。

## 恢复演练（务必定期执行）

没有验证过恢复的备份不算备份。至少每季度做一次完整演练：

```bash
tar -xzf allcallall-<ts>.tar.gz -C /tmp/restore
mysql < /tmp/restore/mysql.sql          # 逻辑恢复
redis-cli --pipe < /tmp/restore/redis.rdb  # 或停服后替换 dump.rdb 重启
```

## 验收：怎么确认备份真的有效

1. **作业成功 ≠ 备份有效。** 因此脚本内置了下限校验：归档小于
   `MIN_DUMP_BYTES`（默认 1024 字节）就直接失败退出。`mysqldump` 权限不足、
   连错实例都可能产出"成功但为空"的归档，这是最容易静默失效的场景。
2. **看归档体积是否随业务增长。** 长期不变通常是 dump 内容不对。
3. **抽查归档内容**：`tar -tzf allcallall-<ts>.tar.gz` 应含 `mysql.sql.gz`。
4. **定期恢复演练**（上一节），这是唯一能真正证明可恢复的手段。

Kubernetes 部署可直接看 Job 状态：

```bash
kubectl -n <ns> get jobs -l app.kubernetes.io/component=backup
kubectl -n <ns> logs job/<job-name>        # 日志含 "archive written: ... (N bytes)"
```
