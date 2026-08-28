# AllCallAll 自动化备份

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

```bash
tar -xzf allcallall-<ts>.tar.gz -C /tmp/restore
mysql < /tmp/restore/mysql.sql          # 逻辑恢复
redis-cli --pipe < /tmp/restore/redis.rdb  # 或停服后替换 dump.rdb 重启
```
