#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_URL="${BASE_URL:-http://localhost:8080}"
WS_URL="${WS_URL:-ws://localhost:8080/api/v1/chat/ws}"
REPORT_DIR="${REPORT_DIR:-/tmp/allcallall-microservice-demo-$(date +%Y%m%d-%H%M%S)}"
CONFIG_PATH="${CONFIG_PATH:-./configs/config.yaml}"
AGENT_PROVIDER="${AGENT_PROVIDER:-mock_llm}"
INTERVIEW_SEED_AGENT_KEY="${INTERVIEW_SEED_AGENT_KEY:-microservice-demo-seed-$(date +%s)}"
MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-rootpass}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-allcallallpass}"
REDIS_PASSWORD="${REDIS_PASSWORD:-redis_secure_password}"
JWT_SECRET="${JWT_SECRET:-microservice-demo-jwt-secret-change-me}"
MAIL_PASSWORD="${MAIL_PASSWORD:-microservice-demo-mail-disabled}"
DB_DSN="${DB_DSN:-allcallall:${MYSQL_PASSWORD}@tcp(127.0.0.1:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local}"
REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
CONCURRENCY="${CONCURRENCY:-2}"
WS_CLIENTS="${WS_CLIENTS:-1}"
WS_DURATION_MS="${WS_DURATION_MS:-1000}"
GOCACHE="${GOCACHE:-/tmp/allcallall-go-cache}"

mkdir -p "$REPORT_DIR" "$GOCACHE"

pids=()
cleanup() {
  for pid in "${pids[@]:-}"; do
    kill "$pid" >/dev/null 2>&1 || true
  done
  for pid in "${pids[@]:-}"; do
    wait "$pid" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT

log() {
  echo "[microservice-demo] $*"
}

json_field() {
  node -e '
const fs = require("node:fs");
const [file, path] = process.argv.slice(1);
const body = JSON.parse(fs.readFileSync(file, "utf8"));
let value = body;
for (const key of path.split(".")) value = value?.[key];
if (value === undefined || value === null) process.exit(2);
process.stdout.write(String(value));
' "$1" "$2"
}

extract_json_field() {
  node -e '
const fs = require("node:fs");
const [file, path] = process.argv.slice(1);
const raw = fs.readFileSync(file, "utf8");
const start = raw.lastIndexOf("{");
const body = JSON.parse(raw.slice(start >= 0 ? start : 0));
let value = body;
for (const key of path.split(".")) value = value?.[key];
if (value === undefined || value === null) process.exit(2);
process.stdout.write(String(value));
' "$1" "$2"
}

wait_for_http() {
  local url="$1"
  local timeout_seconds="${2:-45}"
  local deadline=$(( $(date +%s) + timeout_seconds ))
  while [[ "$(date +%s)" -le "$deadline" ]]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_services() {
  local timeout_seconds="${1:-75}"
  local deadline=$(( $(date +%s) + timeout_seconds ))
  while [[ "$(date +%s)" -le "$deadline" ]]; do
    if (
      cd "$ROOT_DIR"
      MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" MYSQL_PASSWORD="$MYSQL_PASSWORD" REDIS_PASSWORD="$REDIS_PASSWORD" JWT_SECRET="$JWT_SECRET" MAIL_PASSWORD="$MAIL_PASSWORD" \
        docker compose -f infra/docker-compose.yml exec -T mysql mysqladmin ping -h 127.0.0.1 -uroot "-p$MYSQL_ROOT_PASSWORD" --silent
    ) >/dev/null 2>&1 && (
      cd "$ROOT_DIR"
      MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" MYSQL_PASSWORD="$MYSQL_PASSWORD" REDIS_PASSWORD="$REDIS_PASSWORD" JWT_SECRET="$JWT_SECRET" MAIL_PASSWORD="$MAIL_PASSWORD" \
        docker compose -f infra/docker-compose.yml exec -T redis redis-cli -a "$REDIS_PASSWORD" ping
    ) >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

start_go_process() {
  local name="$1"
  shift
  log "starting $name"
  (
    cd "$ROOT_DIR/backend"
    CONFIG_PATH="$CONFIG_PATH" \
    DB_DSN="$DB_DSN" \
    REDIS_ADDR="$REDIS_ADDR" \
    REDIS_PASSWORD="$REDIS_PASSWORD" \
    JWT_SECRET="$JWT_SECRET" \
    MAIL_PASSWORD="$MAIL_PASSWORD" \
    AGENT_PROVIDER="$AGENT_PROVIDER" \
    OUTBOX_WORKER_INTERVAL_SEC="${OUTBOX_WORKER_INTERVAL_SEC:-1}" \
    OUTBOX_WORKER_BATCH_SIZE="${OUTBOX_WORKER_BATCH_SIZE:-50}" \
    GOCACHE="$GOCACHE" \
    "$@"
  ) >"$REPORT_DIR/$name.log" 2>&1 &
  pids+=("$!")
}

log "starting MySQL/Redis"
(
  cd "$ROOT_DIR"
  MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" MYSQL_PASSWORD="$MYSQL_PASSWORD" REDIS_PASSWORD="$REDIS_PASSWORD" JWT_SECRET="$JWT_SECRET" MAIL_PASSWORD="$MAIL_PASSWORD" \
    docker compose -f infra/docker-compose.yml up -d mysql redis
)
wait_for_services 75

if wait_for_http "$BASE_URL/api/v1/health" 2; then
  if [[ "${ALLOW_EXISTING_API:-0}" != "1" ]]; then
    log "API is already running at $BASE_URL; stop it first or set ALLOW_EXISTING_API=1"
    exit 1
  fi
  log "using existing API at $BASE_URL"
else
  start_go_process api env EMBEDDED_WORKERS=0 go run ./cmd/server
  wait_for_http "$BASE_URL/api/v1/health" 75
fi

start_go_process agent-worker env WORKER_ID=microservice-agent-worker go run ./cmd/agent-worker
start_go_process outbox-worker env WORKER_ID=microservice-outbox-worker go run ./cmd/outbox-worker
start_go_process cleanup-worker env WORKER_ID=microservice-cleanup-worker REFRESH_SESSION_CLEANUP_INTERVAL_MIN=1440 RECORDING_CLEANUP_INTERVAL_MIN=1440 go run ./cmd/cleanup-worker

log "seeding demo workspace"
(
  cd "$ROOT_DIR/backend"
  CONFIG_PATH="$CONFIG_PATH" DB_DSN="$DB_DSN" REDIS_ADDR="$REDIS_ADDR" REDIS_PASSWORD="$REDIS_PASSWORD" JWT_SECRET="$JWT_SECRET" MAIL_PASSWORD="$MAIL_PASSWORD" AGENT_PROVIDER="$AGENT_PROVIDER" INTERVIEW_SEED_AGENT_KEY="$INTERVIEW_SEED_AGENT_KEY" GOCACHE="$GOCACHE" \
    go run ./cmd/interview-seed
) | tee "$REPORT_DIR/interview-seed.raw"

organization_id="$(extract_json_field "$REPORT_DIR/interview-seed.raw" organization_id)"
conversation_id="$(extract_json_field "$REPORT_DIR/interview-seed.raw" conversation_id)"
seed_agent_run_id="$(extract_json_field "$REPORT_DIR/interview-seed.raw" agent_run_id)"

log "logging in seeded owner"
login_code="$(curl -sS -o "$REPORT_DIR/login.json" -w '%{http_code}' -X POST "$BASE_URL/api/v1/auth/login" -H "Content-Type: application/json" --data '{"email":"interview.owner@example.com","password":"Interview1234"}')"
if [[ "$login_code" != "200" ]]; then
  log "login failed with HTTP $login_code"
  cat "$REPORT_DIR/login.json" >&2
  exit 1
fi
token="$(json_field "$REPORT_DIR/login.json" access_token)"

log "running Agent smoke through standalone agent-worker"
BASE_URL="$BASE_URL" TOKEN="$token" ORGANIZATION_ID="$organization_id" CONVERSATION_ID="$conversation_id" CONCURRENCY="$CONCURRENCY" POLL_AGENT_RUN=1 AGENT_POLL_TIMEOUT_SECONDS="${AGENT_POLL_TIMEOUT_SECONDS:-45}" \
  "$ROOT_DIR/scripts/load/agent-run-smoke.sh" | tee "$REPORT_DIR/agent-run-smoke.txt"

log "running WebSocket smoke through API replay path"
WS_URL="$WS_URL" TOKEN="$token" ORGANIZATION_ID="$organization_id" CLIENTS="$WS_CLIENTS" DURATION_MS="$WS_DURATION_MS" \
  node "$ROOT_DIR/scripts/load/ws-connections.mjs" | tee "$REPORT_DIR/ws-connections.json"

curl -fsS -H "Authorization: Bearer $token" -H "X-Organization-ID: $organization_id" "$BASE_URL/api/v1/agent/runs/$seed_agent_run_id/events/stream?timeout_ms=5000" \
  | tee "$REPORT_DIR/seed-agent-events.sse" >/dev/null

cat > "$REPORT_DIR/summary.md" <<EOF
# AllCallAll Microservice-Ready Worker Demo

- Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
- API: \`cmd/server\` with \`EMBEDDED_WORKERS=0\`
- Agent worker: \`cmd/agent-worker\`
- Outbox worker: \`cmd/outbox-worker\`
- Cleanup worker: \`cmd/cleanup-worker\`
- Organization ID: \`$organization_id\`
- Conversation ID: \`$conversation_id\`
- Seed Agent Run ID: \`$seed_agent_run_id\`

## Artifacts

- \`api.log\`, \`agent-worker.log\`, \`outbox-worker.log\`, \`cleanup-worker.log\`
- \`interview-seed.raw\`
- \`agent-run-smoke.txt\`
- \`ws-connections.json\`
- \`seed-agent-events.sse\`

## What This Proves

- The API server can run as a modular monolith entrypoint with embedded workers disabled.
- Agent execution can be handled by a standalone worker consuming only \`agent.run.requested\` events.
- Collaboration outbox delivery can be handled by a standalone worker consuming \`agent.run.completed\` and \`message.created\` events.
- Cleanup work can run in a third process without coupling to request handling.
- All processes still share the same MySQL/Redis-backed module boundaries, making this a microservice-ready worker split instead of a big-bang distributed rewrite.
EOF

log "complete: $REPORT_DIR"
