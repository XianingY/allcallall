# AllCallAll Backup & Restore

Offline-friendly backup tooling for the self-hosted stack described in
`infra/docker-compose.production.yml`. It backs up the two stateful services —
**MySQL** (full instance dump) and **Redis** (RDB snapshot) — into a timestamped
directory under `BACKUP_DIR`, with optional S3 upload and local rotation.

## Files

- `backup.sh` — dump MySQL + snapshot Redis, optionally upload to S3, rotate locally.
- `restore.sh` — restore a chosen backup into the running containers.

## Configuration

Both scripts read the same environment variable names used by
`infra/docker-compose.production.yml`, so you can reuse the stack's `.env`:

| Variable              | Default            | Purpose                                              |
|-----------------------|--------------------|------------------------------------------------------|
| `MYSQL_CONTAINER`     | `mysql`            | Container name running MySQL.                        |
| `MYSQL_ROOT_PASSWORD` | *(required)*       | Root password for `mysqldump` / restore.             |
| `REDIS_CONTAINER`     | `redis`            | Container name running Redis.                        |
| `REDIS_PASSWORD`      | *(empty)*          | Redis `requirepass` (omit if none).                 |
| `BACKUP_DIR`          | `./backups`        | Local root for timestamped backups.                  |
| `BACKUP_RETENTION`    | `7`                | Number of newest local backups to keep (0 = keep all).|
| `S3_BUCKET`           | *(empty)*          | If set **and** `aws` CLI is present, upload here.    |
| `S3_PREFIX`           | `allcallall`       | Key prefix under the bucket.                         |
| `AWS_ENDPOINT`        | *(empty)*          | Custom S3 endpoint (e.g. MinIO / Cloudflare R2).     |

## Usage

```bash
# From the repo root, reusing the production .env for credentials:
set -a; source infra/.env; set +a

# Create a backup (local + S3 if configured):
bash scripts/backup/backup.sh

# List available backups:
bash scripts/backup/restore.sh

# Restore a specific backup (type YES to confirm overwrite):
bash scripts/backup/restore.sh backups/20260729-143000
```

## Cron example

Run nightly at 03:07, with credentials sourced from the env file:

```cron
7 3 * * *  set -a; . /opt/allcallall/infra/.env; set +a; /opt/allcallall/scripts/backup/backup.sh >> /var/log/allcallall-backup.log 2>&1
```

## Notes

- MySQL is dumped with `--all-databases`, so a restore replaces the **entire**
  MySQL instance, not just one schema.
- Redis restore copies `dump.rdb` into the container data dir and restarts the
  container so it reloads the snapshot on startup; connected clients will be
  briefly disconnected.
- If `S3_BUCKET` is set but the `aws` CLI is missing, the script keeps a local
  copy and logs a warning (it never fails silently on the local backup).
