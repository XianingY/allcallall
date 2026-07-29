#!/usr/bin/env bash
#
# restore.sh — restore a previously created AllCallAll backup.
#
# Usage:
#   ./restore.sh <backup_dir>      Restore MySQL + Redis from that backup.
#   ./restore.sh                   List available backups under BACKUP_DIR and exit.
#
# The target database/redis containers must already be running (this script does
# not recreate them). MySQL is restored with --all-databases (the dump is a full
# instance dump). Redis is restored by copying the RDB into the container data
# dir and restarting the container so it reloads the snapshot on startup.
#
# Environment variables mirror docker-compose.production.yml:
#   MYSQL_CONTAINER (default mysql)   REDIS_CONTAINER (default redis)
#   MYSQL_ROOT_PASSWORD               REDIS_PASSWORD
#   BACKUP_DIR (default ./backups)
#
set -euo pipefail

MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql}"
MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-}"

REDIS_CONTAINER="${REDIS_CONTAINER:-redis}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"

BACKUP_DIR="${BACKUP_DIR:-./backups}"

log() { printf '[restore] %s\n' "$*"; }
die() { printf '[restore][ERROR] %s\n' "$*" >&2; exit 1; }

[ -n "${MYSQL_ROOT_PASSWORD:-}" ] || die "MYSQL_ROOT_PASSWORD is not set (required to restore)."

mysql_auth=(-uroot -p"${MYSQL_ROOT_PASSWORD}")
redis_cli_auth=()
[ -n "${REDIS_PASSWORD:-}" ] && redis_cli_auth=(-a "${REDIS_PASSWORD}")

if ! command -v docker >/dev/null 2>&1; then
  die "docker is required to reach the mysql/redis containers."
fi

# No argument -> list backups.
if [ $# -eq 0 ]; then
  log "Available backups under ${BACKUP_DIR}:"
  backups="$(ls -1 "${BACKUP_DIR}" 2>/dev/null | sort -r || true)"
  if [ -z "${backups}" ]; then
    log "  (none found)"
  else
    printf '  %s\n' "${backups}"
  fi
  log "Run: $0 <backup_dir>"
  exit 0
fi

SRC="$1"
SQL_FILE="${SRC}/mysql.sql.gz"
RDB_FILE="${SRC}/redis.rdb"

[ -d "${SRC}" ] || die "Backup directory not found: ${SRC}"
[ -f "${SQL_FILE}" ] || die "MySQL dump not found in ${SRC} (expected mysql.sql.gz)."
[ -f "${RDB_FILE}" ] || die "Redis RDB not found in ${SRC} (expected redis.rdb)."

printf 'This will OVERWRITE the MySQL instance and Redis data in container %s / %s from:\n  %s\n' \
  "${MYSQL_CONTAINER}" "${REDIS_CONTAINER}" "${SRC}"
printf 'Type YES to continue: '
read -r confirm
[ "${confirm}" = "YES" ] || die "Aborted by user."

# ---- MySQL ----
log "Restoring MySQL from ${SQL_FILE}"
if command -v pv >/dev/null 2>&1; then
  pv "${SQL_FILE}" | gunzip | docker exec -i "${MYSQL_CONTAINER}" mysql "${mysql_auth[@]}"
else
  gunzip -c "${SQL_FILE}" | docker exec -i "${MYSQL_CONTAINER}" mysql "${mysql_auth[@]}"
fi
log "MySQL restore complete."

# ---- Redis ----
log "Restoring Redis from ${RDB_FILE}"
docker cp "${RDB_FILE}" "${REDIS_CONTAINER}:/data/dump.rdb" >/dev/null
log "Restarting ${REDIS_CONTAINER} to reload the RDB snapshot."
docker restart "${REDIS_CONTAINER}" >/dev/null
# Give Redis a moment to come back and confirm it loaded.
sleep 3
docker exec "${REDIS_CONTAINER}" redis-cli "${redis_cli_auth[@]}" ping >/dev/null \
  || die "Redis did not respond after restart."
log "Redis restore complete."

log "Restore finished."
