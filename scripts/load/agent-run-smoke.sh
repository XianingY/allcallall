#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TOKEN="${TOKEN:-}"
ORGANIZATION_ID="${ORGANIZATION_ID:-}"
CONVERSATION_ID="${CONVERSATION_ID:-}"
CONCURRENCY="${CONCURRENCY:-10}"
POLL_AGENT_RUN="${POLL_AGENT_RUN:-1}"
AGENT_POLL_TIMEOUT_SECONDS="${AGENT_POLL_TIMEOUT_SECONDS:-30}"
AGENT_POLL_INTERVAL_SECONDS="${AGENT_POLL_INTERVAL_SECONDS:-1}"

if [[ -z "$TOKEN" || -z "$ORGANIZATION_ID" || -z "$CONVERSATION_ID" ]]; then
  cat >&2 <<'USAGE'
Usage:
  TOKEN=<jwt> ORGANIZATION_ID=<id> CONVERSATION_ID=<id> ./scripts/load/agent-run-smoke.sh

Optional:
  BASE_URL=http://localhost:8080
  CONCURRENCY=10
  POLL_AGENT_RUN=1
  AGENT_POLL_TIMEOUT_SECONDS=30
  AGENT_POLL_INTERVAL_SECONDS=1

This script creates concurrent Agent runs with distinct Idempotency-Key values.
By default it also polls GET /api/v1/agent/runs/:id until each run is ready/failed.
USAGE
  exit 2
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

json_field() {
  node -e '
const fs = require("node:fs");
const [file, path] = process.argv.slice(1);
const body = JSON.parse(fs.readFileSync(file, "utf8"));
let value = body;
for (const key of path.split(".")) {
  value = value?.[key];
}
if (value === undefined || value === null) {
  process.exit(2);
}
process.stdout.write(String(value));
' "$1" "$2"
}

echo "[agent-run-smoke] base=$BASE_URL org=$ORGANIZATION_ID conversation=$CONVERSATION_ID concurrency=$CONCURRENCY poll=$POLL_AGENT_RUN"

for i in $(seq 1 "$CONCURRENCY"); do
  (
    key="agent-load-$(date +%s)-$i"
    start_seconds="$(date +%s)"
    code="$(curl -sS -o "$tmp_dir/$i.json" -w '%{http_code}' \
      -X POST "$BASE_URL/api/v1/agent/runs" \
      -H "Authorization: Bearer $TOKEN" \
      -H "X-Organization-ID: $ORGANIZATION_ID" \
      -H "Idempotency-Key: $key" \
      -H "Content-Type: application/json" \
      --data "{\"conversation_id\":$CONVERSATION_ID,\"goal\":\"load smoke summarize next step\"}")"
    echo "$code" > "$tmp_dir/$i.status"
    if [[ "$code" != "202" && "$code" != "201" && "$code" != "200" ]]; then
      echo "http_$code" > "$tmp_dir/$i.final"
      exit 0
    fi
    run_id="$(json_field "$tmp_dir/$i.json" "run.id" || true)"
    run_status="$(json_field "$tmp_dir/$i.json" "run.status" || true)"
    echo "$run_id" > "$tmp_dir/$i.run_id"
    echo "$run_status" > "$tmp_dir/$i.initial"
    if [[ -z "$run_id" || "$POLL_AGENT_RUN" != "1" ]]; then
      echo "${run_status:-accepted}" > "$tmp_dir/$i.final"
      exit 0
    fi
    deadline=$((start_seconds + AGENT_POLL_TIMEOUT_SECONDS))
    final_status="$run_status"
    while [[ "$(date +%s)" -le "$deadline" ]]; do
      get_code="$(curl -sS -o "$tmp_dir/$i.poll.json" -w '%{http_code}' \
        -H "Authorization: Bearer $TOKEN" \
        -H "X-Organization-ID: $ORGANIZATION_ID" \
        "$BASE_URL/api/v1/agent/runs/$run_id")"
      if [[ "$get_code" != "200" ]]; then
        final_status="poll_http_$get_code"
        break
      fi
      final_status="$(json_field "$tmp_dir/$i.poll.json" "run.status" || true)"
      if [[ "$final_status" == "ready" || "$final_status" == "failed" ]]; then
        break
      fi
      sleep "$AGENT_POLL_INTERVAL_SECONDS"
    done
    if [[ "$final_status" != "ready" && "$final_status" != "failed" && "$POLL_AGENT_RUN" == "1" ]]; then
      final_status="timeout_${final_status:-unknown}"
    fi
    echo "$final_status" > "$tmp_dir/$i.final"
    echo "$(( $(date +%s) - start_seconds ))" > "$tmp_dir/$i.elapsed_seconds"
  ) &
done

wait

accepted=0
ready=0
failed=0
timeout=0
failure=0
for status_file in "$tmp_dir"/*.status; do
  status="$(cat "$status_file")"
  if [[ "$status" == "202" || "$status" == "201" || "$status" == "200" ]]; then
    accepted=$((accepted + 1))
  else
    failure=$((failure + 1))
  fi
done

for final_file in "$tmp_dir"/*.final; do
  final="$(cat "$final_file")"
  case "$final" in
    ready) ready=$((ready + 1)) ;;
    failed) failed=$((failed + 1)) ;;
    timeout_*) timeout=$((timeout + 1)) ;;
    http_*|poll_http_*) failure=$((failure + 1)) ;;
  esac
done

if compgen -G "$tmp_dir/*.elapsed_seconds" >/dev/null; then
  max_elapsed="$(awk 'max < $1 { max = $1 } END { print max + 0 }' "$tmp_dir"/*.elapsed_seconds)"
else
  max_elapsed=0
fi

echo "[agent-run-smoke] accepted=$accepted ready=$ready failed=$failed timeout=$timeout failure=$failure max_elapsed_seconds=$max_elapsed"
if [[ "$failure" -ne 0 || "$failed" -ne 0 || "$timeout" -ne 0 ]]; then
  echo "[agent-run-smoke] failed responses are in $tmp_dir during execution" >&2
  exit 1
fi
