#!/usr/bin/env bash
#
# AllCallAll — MySQL logical backup for the Kubernetes deployment.
#
# Executed by the backup CronJob (templates/backup-cronjob.yaml). Connection
# details come from the external MySQL secret, so no credential lives in the
# chart or in this script.
#
# Redis is deliberately not dumped here. In Kubernetes the Redis pod has its own
# filesystem, so an RDB copy from inside the backup container always comes up
# empty - a backup that looks fine and restores nothing. Redis durability is the
# Redis StatefulSet's job (persistent volume + AOF); recover it from a volume
# snapshot instead.
#
# Env overrides:
#   MYSQL_DSN                            full DSN (preferred)
#   MYSQL_HOST/PORT/USER/PASSWORD        alternative when no DSN is set
#   BACKUP_DIR                           default /var/backups/allcallall
#   RETAIN_DAYS                          default 30
#   MIN_DUMP_BYTES                       fail if the archive is smaller (default 1024)
#   OFFSITE_CMD                          optional, e.g. "rclone copy %s remote:allcallall-backups"
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/allcallall}"
RETAIN_DAYS="${RETAIN_DAYS:-30}"
MIN_DUMP_BYTES="${MIN_DUMP_BYTES:-1024}"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
LOG_PREFIX="[backup ${TS}]"

log() { echo "${LOG_PREFIX} $*"; }
fail() { echo "${LOG_PREFIX} ERROR: $*" >&2; exit 1; }

[[ -n "${MYSQL_DSN:-}" || -n "${MYSQL_HOST:-}" ]] \
  || fail "set MYSQL_DSN or MYSQL_HOST (injected from the external MySQL secret)"

mkdir -p "${BACKUP_DIR}"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

dump_mysql() {
  if [[ -n "${MYSQL_DSN:-}" ]]; then
    mysqldump --single-transaction --routines --triggers --all-databases \
      --result-file="${TMP}/mysql.sql" "${MYSQL_DSN}" || fail "mysqldump failed"
  else
    mysqldump --single-transaction --routines --triggers --all-databases \
      --result-file="${TMP}/mysql.sql" \
      -h"${MYSQL_HOST:-127.0.0.1}" -P"${MYSQL_PORT:-3306}" \
      -u"${MYSQL_USER:-root}" -p"${MYSQL_PASSWORD:-}" || fail "mysqldump failed"
  fi
  gzip -f "${TMP}/mysql.sql"
  log "mysql dump complete ($(du -h "${TMP}/mysql.sql.gz" | cut -f1))"
}

archive() {
  local out="${BACKUP_DIR}/allcallall-${TS}.tar.gz"
  tar -czf "${out}" -C "${TMP}" .

  # A zero-length or tiny archive means mysqldump produced nothing. Failing
  # loudly here is the whole point: an empty backup that reports success is
  # worse than no backup at all, because nobody finds out until restore time.
  local size
  size="$(stat -c %s "${out}" 2>/dev/null || stat -f %z "${out}")"
  if [[ "${size}" -lt "${MIN_DUMP_BYTES}" ]]; then
    fail "archive is ${size} bytes, below MIN_DUMP_BYTES=${MIN_DUMP_BYTES}"
  fi
  log "archive written: ${out} (${size} bytes)"
  ARCHIVE="${out}"
}

rotate() {
  find "${BACKUP_DIR}" -name 'allcallall-*.tar.gz' -mtime "+${RETAIN_DAYS}" -delete 2>/dev/null || true
  log "rotated archives older than ${RETAIN_DAYS} days"
}

ship_offsite() {
  [[ -z "${OFFSITE_CMD:-}" ]] && return 0
  # shellcheck disable=SC2059
  local cmd
  cmd="$(printf "${OFFSITE_CMD}" "${ARCHIVE}")"
  log "shipping offsite: ${cmd}"
  # Non-fatal on purpose: the local copy is the source of truth, and a flaky
  # object store must not turn a successful backup into a failed job.
  eval "${cmd}" || log "WARN: offsite ship failed (non-fatal)"
}

main() {
  log "starting backup"
  dump_mysql
  archive
  rotate
  ship_offsite
  log "backup finished"
}

main "$@"
