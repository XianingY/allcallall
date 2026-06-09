#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BASE_URL="${BASE_URL:-http://localhost:8080}"
WS_URL="${WS_URL:-ws://localhost:8080/api/v1/chat/ws}"
REPORT_DIR="${REPORT_DIR:-/tmp/allcallall-interview-live-suite-$(date +%Y%m%d-%H%M%S)}"
CONFIG_PATH="${CONFIG_PATH:-./configs/config.yaml}"
AGENT_PROVIDER="${AGENT_PROVIDER:-mock_llm}"
INTERVIEW_SEED_AGENT_KEY="${INTERVIEW_SEED_AGENT_KEY:-interview-live-suite-$(date +%s)}"
MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-rootpass}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-allcallallpass}"
REDIS_PASSWORD="${REDIS_PASSWORD:-redis_secure_password}"
JWT_SECRET="${JWT_SECRET:-interview-live-suite-jwt-secret-change-me}"
MAIL_PASSWORD="${MAIL_PASSWORD:-interview-live-suite-mail-disabled}"
DB_DSN="${DB_DSN:-allcallall:${MYSQL_PASSWORD}@tcp(127.0.0.1:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local}"
REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
CONCURRENCY="${CONCURRENCY:-5}"
WS_CLIENTS="${WS_CLIENTS:-3}"
WS_DURATION_MS="${WS_DURATION_MS:-3000}"
GOCACHE="${GOCACHE:-/tmp/allcallall-go-cache}"
OUTBOX_WORKER_INTERVAL_SEC="${OUTBOX_WORKER_INTERVAL_SEC:-1}"
OUTBOX_WORKER_BATCH_SIZE="${OUTBOX_WORKER_BATCH_SIZE:-50}"

mkdir -p "$REPORT_DIR" "$GOCACHE"

backend_pid=""

cleanup() {
  if [[ -n "$backend_pid" ]]; then
    kill "$backend_pid" >/dev/null 2>&1 || true
    wait "$backend_pid" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

log() {
  echo "[interview-live-suite] $*"
}

run_json() {
  local name="$1"
  shift
  log "running $name"
  "$@" | tee "$REPORT_DIR/$name.json"
}

extract_json_field() {
  node -e '
const fs = require("node:fs");
const [file, path] = process.argv.slice(1);
const raw = fs.readFileSync(file, "utf8");
const start = raw.lastIndexOf("{");
if (start < 0) process.exit(2);
let body;
try {
  body = JSON.parse(raw.slice(start));
} catch (error) {
  const first = raw.indexOf("{");
  if (first < 0) process.exit(2);
  body = JSON.parse(raw.slice(first));
}
let value = body;
for (const key of path.split(".")) value = value?.[key];
if (value === undefined || value === null) process.exit(2);
process.stdout.write(String(value));
' "$1" "$2"
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
  local timeout_seconds="${1:-60}"
  local deadline=$(( $(date +%s) + timeout_seconds ))
  local mysql_ready=0
  local redis_ready=0

  while [[ "$(date +%s)" -le "$deadline" ]]; do
    if [[ "$mysql_ready" == "0" ]] && (
      cd "$ROOT_DIR"
      MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
      MYSQL_PASSWORD="$MYSQL_PASSWORD" \
      REDIS_PASSWORD="$REDIS_PASSWORD" \
      JWT_SECRET="$JWT_SECRET" \
      MAIL_PASSWORD="$MAIL_PASSWORD" \
      docker compose -f infra/docker-compose.yml exec -T mysql \
        mysqladmin ping -h 127.0.0.1 -uroot "-p$MYSQL_ROOT_PASSWORD" --silent
    ) >/dev/null 2>&1; then
      mysql_ready=1
    fi

    if [[ "$redis_ready" == "0" ]] && (
      cd "$ROOT_DIR"
      MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
      MYSQL_PASSWORD="$MYSQL_PASSWORD" \
      REDIS_PASSWORD="$REDIS_PASSWORD" \
      JWT_SECRET="$JWT_SECRET" \
      MAIL_PASSWORD="$MAIL_PASSWORD" \
      docker compose -f infra/docker-compose.yml exec -T redis \
        redis-cli -a "$REDIS_PASSWORD" ping
    ) >/dev/null 2>&1; then
      redis_ready=1
    fi

    if [[ "$mysql_ready" == "1" && "$redis_ready" == "1" ]]; then
      return 0
    fi
    sleep 1
  done

  log "MySQL ready=$mysql_ready Redis ready=$redis_ready"
  return 1
}

start_services() {
  if [[ "${SKIP_START_SERVICES:-0}" == "1" ]]; then
    log "SKIP_START_SERVICES=1; assuming MySQL/Redis are already running"
    return
  fi
  log "starting MySQL/Redis with docker compose"
  (
    cd "$ROOT_DIR"
	    MYSQL_ROOT_PASSWORD="$MYSQL_ROOT_PASSWORD" \
	    MYSQL_PASSWORD="$MYSQL_PASSWORD" \
	    REDIS_PASSWORD="$REDIS_PASSWORD" \
	    JWT_SECRET="$JWT_SECRET" \
	    MAIL_PASSWORD="$MAIL_PASSWORD" \
	    docker compose -f infra/docker-compose.yml up -d mysql redis
	  )
	  log "waiting for MySQL and Redis"
	  wait_for_services 75
	}

start_backend() {
  if wait_for_http "$BASE_URL/api/v1/health" 2; then
    log "backend already healthy at $BASE_URL"
    return
  fi
  if [[ "${SKIP_START_BACKEND:-0}" == "1" ]]; then
    log "SKIP_START_BACKEND=1; waiting for existing backend"
    wait_for_http "$BASE_URL/api/v1/health" 45
    return
  fi
  log "starting backend server"
  (
    cd "$ROOT_DIR/backend"
    CONFIG_PATH="$CONFIG_PATH" \
    DB_DSN="$DB_DSN" \
    REDIS_ADDR="$REDIS_ADDR" \
    REDIS_PASSWORD="$REDIS_PASSWORD" \
    JWT_SECRET="$JWT_SECRET" \
	    MAIL_PASSWORD="$MAIL_PASSWORD" \
	    AGENT_PROVIDER="$AGENT_PROVIDER" \
	    OUTBOX_WORKER_INTERVAL_SEC="$OUTBOX_WORKER_INTERVAL_SEC" \
	    OUTBOX_WORKER_BATCH_SIZE="$OUTBOX_WORKER_BATCH_SIZE" \
	    GOCACHE="$GOCACHE" \
	    go run ./cmd/server
  ) >"$REPORT_DIR/backend.log" 2>&1 &
  backend_pid="$!"
  if ! wait_for_http "$BASE_URL/api/v1/health" 60; then
    log "backend did not become healthy; tailing log"
    tail -120 "$REPORT_DIR/backend.log" >&2 || true
    exit 1
  fi
}

start_services

log "seeding deterministic interview data"
(
  cd "$ROOT_DIR/backend"
  CONFIG_PATH="$CONFIG_PATH" \
  DB_DSN="$DB_DSN" \
  REDIS_ADDR="$REDIS_ADDR" \
  REDIS_PASSWORD="$REDIS_PASSWORD" \
  JWT_SECRET="$JWT_SECRET" \
	  MAIL_PASSWORD="$MAIL_PASSWORD" \
	  AGENT_PROVIDER="$AGENT_PROVIDER" \
	  INTERVIEW_SEED_AGENT_KEY="$INTERVIEW_SEED_AGENT_KEY" \
	  GOCACHE="$GOCACHE" \
	  go run ./cmd/interview-seed
) | tee "$REPORT_DIR/interview-seed.raw"

organization_id="$(extract_json_field "$REPORT_DIR/interview-seed.raw" organization_id)"
conversation_id="$(extract_json_field "$REPORT_DIR/interview-seed.raw" conversation_id)"
room_id="$(extract_json_field "$REPORT_DIR/interview-seed.raw" room_id)"
seed_agent_run_id="$(extract_json_field "$REPORT_DIR/interview-seed.raw" agent_run_id)"
context_chunks="$(extract_json_field "$REPORT_DIR/interview-seed.raw" context_chunks)"

start_backend

log "capturing metrics before smoke"
curl -fsS "$BASE_URL/api/v1/metrics" > "$REPORT_DIR/metrics-before.prom"

log "logging in interview owner"
login_code="$(curl -sS -o "$REPORT_DIR/login.json" -w '%{http_code}' \
  -X POST "$BASE_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  --data '{"email":"interview.owner@example.com","password":"Interview1234"}')"
if [[ "$login_code" != "200" ]]; then
  log "login failed with HTTP $login_code"
  cat "$REPORT_DIR/login.json" >&2
  exit 1
fi
token="$(json_field "$REPORT_DIR/login.json" access_token)"

log "fetching seeded agent trace"
curl -fsS \
  -H "Authorization: Bearer $token" \
  -H "X-Organization-ID: $organization_id" \
  "$BASE_URL/api/v1/agent/runs/$seed_agent_run_id" \
  | tee "$REPORT_DIR/seed-agent-run.json" >/dev/null

log "fetching seeded agent event timeline"
curl -fsS \
  -H "Authorization: Bearer $token" \
  -H "X-Organization-ID: $organization_id" \
  "$BASE_URL/api/v1/agent/runs/$seed_agent_run_id/events" \
  | tee "$REPORT_DIR/seed-agent-events.json" >/dev/null

log "fetching seeded agent SSE event stream"
curl -fsS -N --max-time 10 \
  -H "Authorization: Bearer $token" \
  -H "X-Organization-ID: $organization_id" \
  "$BASE_URL/api/v1/agent/runs/$seed_agent_run_id/events/stream?timeout_ms=5000" \
  | tee "$REPORT_DIR/seed-agent-events.sse" >/dev/null
if ! grep -q "event:run_ready" "$REPORT_DIR/seed-agent-events.sse"; then
  log "agent SSE stream did not include run_ready"
  cat "$REPORT_DIR/seed-agent-events.sse" >&2
  exit 1
fi

BASE_URL="$BASE_URL" \
TOKEN="$token" \
ORGANIZATION_ID="$organization_id" \
CONVERSATION_ID="$conversation_id" \
CONCURRENCY="$CONCURRENCY" \
POLL_AGENT_RUN=1 \
AGENT_POLL_TIMEOUT_SECONDS="${AGENT_POLL_TIMEOUT_SECONDS:-45}" \
"$ROOT_DIR/scripts/load/agent-run-smoke.sh" | tee "$REPORT_DIR/agent-run-smoke.txt"

WS_URL="$WS_URL" \
TOKEN="$token" \
ORGANIZATION_ID="$organization_id" \
CLIENTS="$WS_CLIENTS" \
DURATION_MS="$WS_DURATION_MS" \
node "$ROOT_DIR/scripts/load/ws-connections.mjs" | tee "$REPORT_DIR/ws-connections.json"

log "capturing metrics after smoke"
curl -fsS "$BASE_URL/api/v1/metrics" > "$REPORT_DIR/metrics-after.prom"

cat > "$REPORT_DIR/summary.md" <<EOF
# AllCallAll Interview Live Suite

- Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
- Base URL: \`$BASE_URL\`
- WebSocket URL: \`$WS_URL\`
- Agent provider: \`$AGENT_PROVIDER\`
- Organization ID: \`$organization_id\`
- Conversation ID: \`$conversation_id\`
- Room ID: \`$room_id\`
- Seed Agent Run ID: \`$seed_agent_run_id\`
- Seed Agent Key: \`$INTERVIEW_SEED_AGENT_KEY\`
- Indexed context chunks: \`$context_chunks\`
- Agent smoke concurrency: \`$CONCURRENCY\`
- WebSocket clients: \`$WS_CLIENTS\`
- WebSocket duration ms: \`$WS_DURATION_MS\`

## Artifacts

- \`interview-seed.raw\`: seed output with demo IDs.
- \`login.json\`: authenticated login response for the seeded owner.
- \`seed-agent-run.json\`: persisted Agent run with steps, tool calls, and trace.
- \`seed-agent-events.json\`: Agent run event timeline for run/step/tool streaming demos.
- \`seed-agent-events.sse\`: Server-Sent Events stream for the same Agent run.
- \`agent-run-smoke.txt\`: concurrent live Agent run creation and polling summary.
- \`ws-connections.json\`: live chat WebSocket connection smoke.
- \`metrics-before.prom\` and \`metrics-after.prom\`: Prometheus-style metric snapshots.
- \`backend.log\`: backend log when this script started the server itself.

## What This Proves

- MySQL-backed seed and migrations work against the local Docker stack.
- Redis-backed backend startup succeeds.
- Auth login returns a real JWT for the seeded user.
- Agent APIs execute through the live HTTP server and outbox worker.
- Agent event streaming emits run/step/tool lifecycle events over SSE.
- Chat WebSocket accepts authenticated organization-scoped clients.
- Metrics can be captured before and after load smoke.
EOF

log "complete: $REPORT_DIR"
