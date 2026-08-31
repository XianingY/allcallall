#!/usr/bin/env bash
#
# AllCallAll — automated database backup.
#
# Dumps MySQL (logical, per-schema) and snapshots Redis (RDB copy), rotates
# local retention, and optionally ships to an offsite location (rclone/scp).
# Designed to be driven by systemd timer or cron; never blocks on network.
#
# Env overrides:
#   MYSQL_DSN / MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD
#   REDIS_HOST / REDIS_PORT / REDIS_PASSWORD
#   BACKUP_DIR (default /var/backups/allcallall)
#   RETAIN_DAYS (default 30)
#   OFFSITE_CMD (optional, e.g. "rclone copy %s remote:allcallall-backups")
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/allcallall}"
RETAIN_DAYS="${RETAIN_DAYS:-30}"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
LOG_PREFIX="[backup ${TS}]"

log() { echo "${LOG_PREFIX} $*"; }
fail() { echo "${LOG_PREFIX} ERROR: $*" >&2; exit 1; }

mkdir -p "${BACKUP_DIR}"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

backup_mysql() {
  if [[ -n "${MYSQL_DSN:-}" ]]; then
    mysqldump --single-transaction --routines --triggers --all-databases \
      --result-file="${TMP}/mysql.sql" "${MYSQL_DSN}" \
      || mysqldump --single-transaction --routines --triggers --all-databases \
         --result-file="${TMP}/mysql.sql" \
         -h"${MYSQL_HOST:-127.0.0.1}" -P"${MYSQL_PORT:-3306}" \
         -u"${MYSQL_USER:-root}" -p"${MYSQL_PASSWORD:-}" \
         || fail "mysqldump failed"
  else
    mysqldump --single-transaction --routines --triggers --all-databases \
      --result-file="${TMP}/mysql.sql" \
      -h"${MYSQL_HOST:-127.0.0.1}" -P"${MYSQL_PORT:-3306}" \
      -u"${MYSQL_USER:-root}" -p"${MYSQL_PASSWORD:-}" \
      || fail "mysqldump failed"
  fi
  gzip -f "${TMP}/mysql.sql"
  log "mysql dump complete ($(du -h "${TMP}/mysql.sql.gz" | cut -f1))"
}

backup_redis() {
  local rdb
  rdb="$(redis-cli -h "${REDIS_HOST:-127.0.0.1}" -p "${REDIS_PORT:-6379}" \
    ${REDIS_PASSWORD:+-a "${REDIS_PASSWORD}"} --no-auth-warning SAVE \
    && redis-cli -h "${REDIS_HOST:-127.0.0.1}" -p "${REDIS_PORT:-6379}" \
    ${REDIS_PASSWORD:+-a "${REDIS_PASSWORD}"} --no-auth-warning --raw GET '' 2>/dev/null \
    ; true)"
  # Copy the live RDB file instead of relying on SAVE output.
  local dump_path
  dump_path="$(redis-cli -h "${REDIS_HOST:-127.0.0.1}" -p "${REDIS_PORT:-6379}" \
    ${REDIS_PASSWORD:+-a "${REDIS_PASSWORD}"} --no-auth-warning CONFIG GET dir 2>/dev/null \
    | tail -1)/dump.rdb"
  if [[ -f "${dump_path}" ]]; then
    cp "${dump_path}" "${TMP}/redis.rdb"
    log "redis rdb copied from ${dump_path}"
  else
    log "WARN: redis rdb not found at ${dump_path}; skipping redis backup"
  fi
}

rotate() {
  find "${BACKUP_DIR}" -name 'allcallall-*.tar.gz' -mtime "+${RETAIN_DAYS}" -delete 2>/dev/null || true
}

ship_offsite() {
  [[ -z "${OFFSITE_CMD:-}" ]] && return 0
  # shellcheck disable=SC2059
  local cmd; cmd="$(printf "${OFFSITE_CMD}" "${1}")"
  log "shipping offsite: ${cmd}"
  eval "${cmd}" || log "WARN: offsite ship failed (non-fatal)"
}

main() {
  log "starting backup"
  backup_mysql
  backup_redis
  local archive="${BACKUP_DIR}/allcallall-${TS}.tar.gz"
  tar -czf "${archive}" -C "${TMP}" .
  log "archive written: ${archive}"
  rotate
  ship_offsite "${archive}"
  log "backup finished"
}

main "$@"
