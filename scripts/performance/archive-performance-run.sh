#!/usr/bin/env bash
set -euo pipefail

# archive-performance-run.sh — copy a load/benchmark JSON report into the
# unified performance evidence archive.
#
# Archive layout:
#   <archive-dir>/<profile>/<UTC-timestamp>-<git-sha>/report.json
#   <archive-dir>/<profile>/<UTC-timestamp>-<git-sha>/meta.json
#
# The archive is intentionally local (gitignored): absolute numbers are
# machine-dependent, so the shared evidence channel is the nightly workflow's
# uploaded artifacts plus the committed baseline files under
# scripts/performance/baselines/. The archive is the single predictable place
# local runs accumulate for perf-trend.
#
# Usage:
#   scripts/performance/archive-performance-run.sh <report.json> <profile> [archive-dir]

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <report.json> <profile> [archive-dir]" >&2
  exit 2
fi

REPORT="$1"
PROFILE="$2"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ARCHIVE_DIR="${3:-${PERFORMANCE_ARCHIVE_DIR:-$REPO_ROOT/performance-archive}}"

if [[ ! -f "$REPORT" ]]; then
  echo "[archive-performance] report not found: $REPORT" >&2
  exit 2
fi

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
if GIT_SHA="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null)"; then
  RUN_DIR="$ARCHIVE_DIR/$PROFILE/${STAMP}-${GIT_SHA}"
else
  RUN_DIR="$ARCHIVE_DIR/$PROFILE/${STAMP}-nogit"
fi
if [[ -d "$RUN_DIR" ]]; then
  SUFFIX=1
  while [[ -d "${RUN_DIR}-${SUFFIX}" ]]; do
    SUFFIX=$((SUFFIX + 1))
  done
  RUN_DIR="${RUN_DIR}-${SUFFIX}"
fi

mkdir -p "$RUN_DIR"
cp "$REPORT" "$RUN_DIR/report.json"

{
  echo "{"
  echo "  \"profile\": \"$PROFILE\","
  echo "  \"archivedAt\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
  echo "  \"sourceReport\": \"$REPORT\","
  echo "  \"repositorySha\": \"$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo unknown)\","
  echo "  \"host\": \"$(hostname 2>/dev/null || echo unknown)\""
  echo "}"
} > "$RUN_DIR/meta.json"

echo "[archive-performance] archived $PROFILE report to $RUN_DIR"
