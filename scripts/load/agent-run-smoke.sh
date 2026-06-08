#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TOKEN="${TOKEN:-}"
ORGANIZATION_ID="${ORGANIZATION_ID:-}"
CONVERSATION_ID="${CONVERSATION_ID:-}"
CONCURRENCY="${CONCURRENCY:-10}"

if [[ -z "$TOKEN" || -z "$ORGANIZATION_ID" || -z "$CONVERSATION_ID" ]]; then
  cat >&2 <<'USAGE'
Usage:
  TOKEN=<jwt> ORGANIZATION_ID=<id> CONVERSATION_ID=<id> ./scripts/load/agent-run-smoke.sh

Optional:
  BASE_URL=http://localhost:8080
  CONCURRENCY=10

This script creates concurrent Agent runs with distinct Idempotency-Key values.
USAGE
  exit 2
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo "[agent-run-smoke] base=$BASE_URL org=$ORGANIZATION_ID conversation=$CONVERSATION_ID concurrency=$CONCURRENCY"

for i in $(seq 1 "$CONCURRENCY"); do
  (
    key="agent-load-$(date +%s)-$i"
    code="$(curl -sS -o "$tmp_dir/$i.json" -w '%{http_code}' \
      -X POST "$BASE_URL/api/v1/agent/runs" \
      -H "Authorization: Bearer $TOKEN" \
      -H "X-Organization-ID: $ORGANIZATION_ID" \
      -H "Idempotency-Key: $key" \
      -H "Content-Type: application/json" \
      --data "{\"conversation_id\":$CONVERSATION_ID,\"goal\":\"load smoke summarize next step\"}")"
    echo "$code" > "$tmp_dir/$i.status"
  ) &
done

wait

success=0
failure=0
for status_file in "$tmp_dir"/*.status; do
  status="$(cat "$status_file")"
  if [[ "$status" == "201" || "$status" == "200" ]]; then
    success=$((success + 1))
  else
    failure=$((failure + 1))
  fi
done

echo "[agent-run-smoke] success=$success failure=$failure"
if [[ "$failure" -ne 0 ]]; then
  echo "[agent-run-smoke] failed responses are in $tmp_dir during execution" >&2
  exit 1
fi
