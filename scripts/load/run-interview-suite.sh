#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
AGENT_PROVIDER="${AGENT_PROVIDER:-rules}"
CONVERSATIONS="${CONVERSATIONS:-25}"
BATCH_SIZE="${BATCH_SIZE:-50}"
EVENTS="${EVENTS:-2000}"
RECIPIENTS="${RECIPIENTS:-10}"
REPLAY_WINDOW="${REPLAY_WINDOW:-120}"
REPLAY_LIMIT="${REPLAY_LIMIT:-100}"
CLIENTS="${CLIENTS:-5}"
REPORT_DIR="${REPORT_DIR:-/tmp/allcallall-interview-suite-$(date +%Y%m%d-%H%M%S)}"
GOCACHE="${GOCACHE:-/tmp/allcallall-go-cache}"

mkdir -p "$REPORT_DIR" "$GOCACHE"

run_backend_json() {
  local name="$1"
  shift
  echo "[interview-suite] running $name"
  (
    cd "$ROOT_DIR/backend"
    GOCACHE="$GOCACHE" "$@"
  ) | tee "$REPORT_DIR/$name.json"
}

run_backend_json agent-eval go run ./cmd/agent-eval -provider "$AGENT_PROVIDER"
run_backend_json interview-bench go run ./cmd/interview-bench -provider "$AGENT_PROVIDER" -conversations "$CONVERSATIONS" -batch-size "$BATCH_SIZE"
run_backend_json realtime-replay-bench go run ./cmd/realtime-replay-bench -events "$EVENTS" -recipients "$RECIPIENTS" -replay-window "$REPLAY_WINDOW" -replay-limit "$REPLAY_LIMIT"
run_backend_json chat-ws-replay-bench go run ./cmd/chat-ws-replay-bench -events "$EVENTS" -recipients "$RECIPIENTS" -replay-window "$REPLAY_WINDOW" -replay-limit "$REPLAY_LIMIT" -clients "$CLIENTS"

cat > "$REPORT_DIR/summary.md" <<EOF
# AllCallAll Interview Load Suite

- Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
- Agent provider: \`$AGENT_PROVIDER\`
- Conversations: \`$CONVERSATIONS\`
- Batch size: \`$BATCH_SIZE\`
- Replay events: \`$EVENTS\`
- Recipients: \`$RECIPIENTS\`
- Replay window: \`$REPLAY_WINDOW\`
- Replay limit: \`$REPLAY_LIMIT\`
- WebSocket clients: \`$CLIENTS\`

## Artifacts

- \`agent-eval.json\`
- \`interview-bench.json\`
- \`realtime-replay-bench.json\`
- \`chat-ws-replay-bench.json\`

## Notes

This suite is deterministic and local by default. It uses temporary SQLite or in-process HTTP/WebSocket servers, so it is safe for interview demos and CI-style checks. Use \`scripts/load/agent-run-smoke.sh\` and \`scripts/load/ws-connections.mjs\` against a running MySQL/Redis backend for live environment evidence.
EOF

echo "[interview-suite] complete: $REPORT_DIR"
