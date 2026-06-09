#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EVENTS="${EVENTS:-2000}"
RECIPIENTS="${RECIPIENTS:-10}"
REPLAY_WINDOW="${REPLAY_WINDOW:-120}"
REPLAY_LIMIT="${REPLAY_LIMIT:-100}"
GOCACHE="${GOCACHE:-/tmp/allcallall-go-cache}"

mkdir -p "$GOCACHE"
cd "$ROOT_DIR/backend"
GOCACHE="$GOCACHE" go run ./cmd/realtime-replay-bench \
  -events "$EVENTS" \
  -recipients "$RECIPIENTS" \
  -replay-window "$REPLAY_WINDOW" \
  -replay-limit "$REPLAY_LIMIT"
