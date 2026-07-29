#!/usr/bin/env bash
#
# backup.sh — offline-friendly backup for the self-hosted AllCallAll stack.
#
# Dumps the whole MySQL instance (mysqldump --all-databases) and snapshots the
# Redis dataset (BGSAVE -> copy the RDB file). Output is written to a timestamped
# directory under BACKUP_DIR. If S3 credentials/bucket are provided AND the aws
# CLI is available, the files are also uploaded; otherwise they are kept locally
# with simple count-based rotation.
#
# All connection details are injected through the same environment variable names
# used by infra/docker-compose.production.yml, so the script can be sourced from
# the same .env file the stack uses.
#
set -euo pipefail

# ---- configuration (env-injected, matching docker-compose.production.yml) ----
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql}"
MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-}"
MYSQL_DATABASE="${MYSQL_DATABASE:-allcallall_db}"

REDIS_CONTAINER="${REDIS_CONTAINER:-redis}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"

BACKUP_DIR="${BACKUP_DIR:-./backups}"
BACKUP_RETENTION="${BACKUP_RETENTION:-7}"

# Optional S3 upload. Leave S3_BUCKET empty to keep backups local only.
S3_BUCKET="${S3_BUCKET:-}"
S3_PREFIX="${S3_PREFIX:-allcallall}"
AWS_ENDPOINT="${AWS_ENDPOINT:-}"

# ---- helpers ----
log() { printf '[backup] %s\n' "$*"; }
die() { printf '[backup][ERROR] %s\n' "$*" >&2; exit 1; }

[ -n "${MYSQL_ROOT_PASSWORD:-}" ] || die "MYSQL_ROOT_PASSWORD is not set (it is required for mysqldump)."

mysql_auth=(-uroot -p"${MYSQL_ROOT_PASSWORD}")
redis_cli_auth=()
[ -n "${REDIS_PASSWORD:-}" ] && redis_cli_auth=(-a "${REDIS_PASSWORD}")

# ---- prepare ----
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
DEST="${BACKUP_DIR}/${TIMESTAMP}"
mkdir -p "${DEST}"

SQL_FILE="${DEST}/mysql.sql.gz"
RDB_FILE="${DEST}/redis.rdb"

if ! command -v docker >/dev/null 2>&1; then
  die "docker is required to reach the mysql/redis containers."
fi

# ---- MySQL ----
log "Dumping all MySQL databases -> ${SQL_FILE}"
docker exec "${MYSQL_CONTAINER}" \
  mysqldump "${mysql_auth[@]}" --all-databases --single-transaction --routines --events --quick \
  | gzip > "${SQL_FILE}"
log "MySQL dump complete ($(du -h "${SQL_FILE}" | cut -f1))."

# ---- Redis ----
log "Triggering Redis BGSAVE on ${REDIS_CONTAINER}"
docker exec "${REDIS_CONTAINER}" redis-cli "${redis_cli_auth[@]}" BGSAVE >/dev/null

# Wait for the background save to finish.
for _ in $(seq 1 30); do
  in_progress="$(docker exec "${REDIS_CONTAINER}" redis-cli "${redis_cli_auth[@]}" INFO persistence 2>/dev/null \
    | awk -F: '/^rdb_bgsave_in_progress:/{gsub(/\r/,"",$2); print $2}')"
  [ "${in_progress:-0}" = "0" ] && break
  sleep 1
done

last_status="$(docker exec "${REDIS_CONTAINER}" redis-cli "${redis_cli_auth[@]}" INFO persistence 2>/dev/null \
  | awk -F: '/^rdb_last_bgsave_status:/{gsub(/\r/,"",$2); print $2}')"
[ "${last_status:-ok}" = "ok" ] || die "Redis BGSAVE reported status '${last_status}'."

log "Copying Redis RDB -> ${RDB_FILE}"
docker cp "${REDIS_CONTAINER}:/data/dump.rdb" "${RDB_FILE}" >/dev/null
log "Redis snapshot complete ($(du -h "${RDB_FILE}" | cut -f1))."

# ---- optional S3 upload ----
if [ -n "${S3_BUCKET}" ]; then
  if command -v aws >/dev/null 2>&1; then
    log "Uploading backup to s3://${S3_BUCKET}/${S3_PREFIX}/${TIMESTAMP}/"
    endpoint_opt=()
    [ -n "${AWS_ENDPOINT}" ] && endpoint_opt=(--endpoint-url "${AWS_ENDPOINT}")
    aws s3 cp "${SQL_FILE}" "s3://${S3_BUCKET}/${S3_PREFIX}/${TIMESTAMP}/mysql.sql.gz" "${endpoint_opt[@]}"
    aws s3 cp "${RDB_FILE}" "s3://${S3_BUCKET}/${S3_PREFIX}/${TIMESTAMP}/redis.rdb" "${endpoint_opt[@]}"
  else
    log "S3_BUCKET is set but the aws CLI is not installed; keeping local copy only."
  fi
else
  log "S3_BUCKET not set; keeping local copy only."
fi

# ---- local rotation ----
if [ "${BACKUP_RETENTION}" -gt 0 ]; then
  log "Rotating local backups, keeping the newest ${BACKUP_RETENTION}."
  ls -1 "${BACKUP_DIR}" | sort -r | tail -n +"$((BACKUP_RETENTION + 1))" | while read -r old; do
    rm -rf "${BACKUP_DIR}/${old}"
    log "Removed old backup ${old}."
  done
fi

log "Backup finished: ${DEST}"
