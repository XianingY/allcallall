#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${MODE:-local}"
AGENT_PROVIDER="${AGENT_PROVIDER:-mock_llm}"
CONVERSATIONS="${CONVERSATIONS:-25}"
BATCH_SIZE="${BATCH_SIZE:-50}"
REPORT_DIR="${REPORT_DIR:-/tmp/allcallall-interview-demo-$(date +%Y%m%d-%H%M%S)}"
GOCACHE="${GOCACHE:-/tmp/allcallall-go-cache}"

mkdir -p "$REPORT_DIR" "$GOCACHE"

run_backend_json() {
  local name="$1"
  shift
  echo ""
  echo "[interview-demo] $name"
  (
    cd "$ROOT_DIR/backend"
    GOCACHE="$GOCACHE" "$@"
  ) | tee "$REPORT_DIR/$name.json"
}

echo "[interview-demo] mode=$MODE provider=$AGENT_PROVIDER report_dir=$REPORT_DIR"

case "$MODE" in
  local)
    run_backend_json agent-eval go run ./cmd/agent-eval -provider "$AGENT_PROVIDER"
    run_backend_json interview-bench go run ./cmd/interview-bench -provider "$AGENT_PROVIDER" -conversations "$CONVERSATIONS" -batch-size "$BATCH_SIZE"
    run_backend_json realtime-replay-bench go run ./cmd/realtime-replay-bench -events "${EVENTS:-2000}" -recipients "${RECIPIENTS:-10}" -replay-window "${REPLAY_WINDOW:-120}" -replay-limit "${REPLAY_LIMIT:-100}"
    run_backend_json chat-ws-replay-bench go run ./cmd/chat-ws-replay-bench -events "${EVENTS:-2000}" -recipients "${RECIPIENTS:-10}" -replay-window "${REPLAY_WINDOW:-120}" -replay-limit "${REPLAY_LIMIT:-100}" -clients "${CLIENTS:-5}"
    ;;
  live)
    echo "[interview-demo] starting MySQL/Redis via scripts/development/start-services.sh"
    "$ROOT_DIR/scripts/development/start-services.sh"
    run_backend_json interview-seed env CONFIG_PATH="${CONFIG_PATH:-./configs/config.yaml}" AGENT_PROVIDER="$AGENT_PROVIDER" go run ./cmd/interview-seed
    cat <<EOF

[interview-demo] live backend follow-up
Start the backend in another terminal:
  cd "$ROOT_DIR/backend" && CONFIG_PATH=./configs/config.yaml go run ./cmd/server/main.go

Then inspect:
  curl -s http://localhost:8080/api/v1/health
  curl -s http://localhost:8080/api/v1/metrics

Authenticated smoke scripts after login/JWT:
  BASE_URL=http://localhost:8080 TOKEN=<jwt> ORGANIZATION_ID=<id> CONVERSATION_ID=<id> ./scripts/load/agent-run-smoke.sh
  WS_URL=ws://localhost:8080/api/v1/chat/ws TOKEN=<jwt> ORGANIZATION_ID=<id> node scripts/load/ws-connections.mjs
EOF
    ;;
  *)
    echo "[interview-demo] unsupported MODE=$MODE (use local or live)" >&2
    exit 2
    ;;
esac

echo ""
echo "[interview-demo] complete: $REPORT_DIR"
