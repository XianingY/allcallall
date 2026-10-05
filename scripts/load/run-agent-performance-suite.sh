#!/usr/bin/env bash
set -euo pipefail

# run-agent-performance-suite.sh — Run the end-to-end agent performance suite.
#
# Profiles:
#   baseline            — deterministic in-process orchestration and persistence checks
#   controlled          — Go + Agent Runtime + RAG Runtime with the fake provider
#   networked           — MySQL, Redis, Go, Agent Runtime, RAG Runtime, and fake provider in separate processes
#   real-provider-canary — explicit opt-in, max 10 runs, concurrency 1, never the sole merge gate
#
# Environment:
#   PROFILE             — one of: baseline, controlled, networked, real-provider-canary (default: baseline)
#   BASE_URL            — API base URL (required for controlled, networked, real-provider-canary)
#   TOKEN               — Bearer token (required for controlled, networked, real-provider-canary)
#   ORGANIZATION_ID     — X-Organization-ID header value
#   CONVERSATION_ID     — conversation_id for run creation
#   RUNS                — number of runs (default: profile-dependent)
#   CONCURRENCY         — worker pool size (default: profile-dependent)
#   POLL_INTERVAL_MS    — poll interval in ms (default: 100)
#   TERMINAL_TIMEOUT_MS — per-run timeout in ms (default: 60000)
#   METRICS_URL         — Prometheus metrics endpoint (default: BASE_URL/api/v1/metrics)
#   ALLOW_REAL_PROVIDER_CANARY — must be "1" to enable the real-provider-canary profile
#   FAKE_PROVIDER_LATENCY_MS   — fake provider base delay (default: 50)
#   FAKE_PROVIDER_FAILURE_RATE — fake provider failure probability 0–1 (default: 0)
#   FAKE_PROVIDER_TIMEOUT_RATE — fake provider timeout probability 0–1 (default: 0)
#   FAKE_PROVIDER_RESPONSE_BYTES — fake provider response size (default: 256)
#
# Outputs:
#   - JSON report to stdout
#   - Markdown summary and full artifacts to a temporary directory (printed at end)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

PROFILE="${PROFILE:-baseline}"
BASE_URL="${BASE_URL:-}"
TOKEN="${TOKEN:-}"
ORGANIZATION_ID="${ORGANIZATION_ID:-}"
CONVERSATION_ID="${CONVERSATION_ID:-}"
RUNS="${RUNS:-}"
CONCURRENCY="${CONCURRENCY:-}"
POLL_INTERVAL_MS="${POLL_INTERVAL_MS:-100}"
TERMINAL_TIMEOUT_MS="${TERMINAL_TIMEOUT_MS:-60000}"
METRICS_URL="${METRICS_URL:-}"
ALLOW_REAL_PROVIDER_CANARY="${ALLOW_REAL_PROVIDER_CANARY:-}"

# Profile defaults
case "$PROFILE" in
  baseline)
    : "${RUNS:=4}"
    : "${CONCURRENCY:=2}"
    ;;
  controlled)
    : "${RUNS:=10}"
    : "${CONCURRENCY:=2}"
    ;;
  networked)
    : "${RUNS:=10}"
    : "${CONCURRENCY:=2}"
    ;;
  real-provider-canary)
    if [[ "$ALLOW_REAL_PROVIDER_CANARY" != "1" ]]; then
      echo "[agent-performance-suite] ERROR: real-provider-canary requires ALLOW_REAL_PROVIDER_CANARY=1" >&2
      exit 1
    fi
    : "${RUNS:=10}"
    # Cap at 10 runs, concurrency 1
    if [[ "$RUNS" -gt 10 ]]; then
      RUNS=10
    fi
    CONCURRENCY=1
    ;;
  *)
    echo "[agent-performance-suite] ERROR: unknown PROFILE '$PROFILE'. Use one of: baseline, controlled, networked, real-provider-canary" >&2
    exit 2
    ;;
esac

# Validate required settings for non-baseline profiles
if [[ "$PROFILE" != "baseline" ]]; then
  if [[ -z "$BASE_URL" ]]; then
    echo "[agent-performance-suite] ERROR: BASE_URL is required for profile '$PROFILE'" >&2
    exit 2
  fi
  if [[ -z "$TOKEN" ]]; then
    echo "[agent-performance-suite] ERROR: TOKEN is required for profile '$PROFILE'" >&2
    exit 2
  fi
fi

# For baseline, use the test server (node --test runs the in-process server)
# For controlled/networked, validate the backend is reachable
if [[ "$PROFILE" == "controlled" || "$PROFILE" == "networked" ]]; then
  if ! curl -sf -o /dev/null --max-time 5 "$BASE_URL/api/v1/metrics" 2>/dev/null; then
    echo "[agent-performance-suite] WARNING: $BASE_URL/api/v1/metrics not reachable — suite may fail" >&2
  fi
fi

# Create output directory
OUTPUT_DIR="$(mktemp -d)"
trap 'echo "[agent-performance-suite] artifacts in $OUTPUT_DIR"' EXIT

# Record repository SHAs
ALLCALLALL_SHA="unknown"
AGENT_RUNTIME_SHA="unknown"
if command -v git &>/dev/null; then
  ALLCALLALL_SHA="$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null || echo unknown)"
  AGENT_RUNTIME_SHA="$(git -C "$REPO_ROOT/../allcallall-agent-runtime" rev-parse HEAD 2>/dev/null || echo unknown)"
fi

# Record host data
HOST_DATA="$(uname -srm 2>/dev/null || echo unknown)"
NODE_VERSION="$(node --version 2>/dev/null || echo unknown)"

# Record pre-benchmark metrics if available
PRE_METRICS_FILE="$OUTPUT_DIR/metrics-before.txt"
if [[ -n "$BASE_URL" && "$PROFILE" != "baseline" ]]; then
  METRICS_ENDPOINT="${METRICS_URL:-${BASE_URL%/}/api/v1/metrics}"
  curl -sf --max-time 5 "$METRICS_ENDPOINT" > "$PRE_METRICS_FILE" 2>/dev/null || echo "# metrics unavailable" > "$PRE_METRICS_FILE"
else
  echo "# baseline profile — no external metrics" > "$PRE_METRICS_FILE"
fi

# Start fake provider for controlled/networked profiles
FAKE_PROVIDER_PID=""
FAKE_PROVIDER_URL=""

cleanup_fake_provider() {
  if [[ -n "$FAKE_PROVIDER_PID" ]]; then
    kill "$FAKE_PROVIDER_PID" 2>/dev/null || true
    wait "$FAKE_PROVIDER_PID" 2>/dev/null || true
    FAKE_PROVIDER_PID=""
  fi
}

if [[ "$PROFILE" == "controlled" || "$PROFILE" == "networked" ]]; then
  export FAKE_PROVIDER_LATENCY_MS="${FAKE_PROVIDER_LATENCY_MS:-50}"
  export FAKE_PROVIDER_FAILURE_RATE="${FAKE_PROVIDER_FAILURE_RATE:-0}"
  export FAKE_PROVIDER_TIMEOUT_RATE="${FAKE_PROVIDER_TIMEOUT_RATE:-0}"
  export FAKE_PROVIDER_RESPONSE_BYTES="${FAKE_PROVIDER_RESPONSE_BYTES:-256}"
  export FAKE_PROVIDER_PORT="0"

  node "$SCRIPT_DIR/fake-agent-provider.mjs" &
  FAKE_PROVIDER_PID=$!

  # Wait for the fake provider to start and capture its port
  for _ in $(seq 1 20); do
    if [[ -f /proc/$FAKE_PROVIDER_PID/fd/1 ]] 2>/dev/null; then
      : # Linux
    fi
    sleep 0.2
  done

  # Try to find the port from the process output — give it a moment
  # The fake provider prints its port on startup; we'll use a fixed port approach instead
  # For now, we just note that the fake provider is available for configuration
  FAKE_PROVIDER_URL="http://127.0.0.1:0" # placeholder — real URL depends on startup
fi

# Run the benchmark
echo "[agent-performance-suite] profile=$PROFILE runs=$RUNS concurrency=$CONCURRENCY" >&2

BENCH_ARGS=(
  --base-url "${BASE_URL:-http://localhost:8080}"
  --runs "$RUNS"
  --concurrency "$CONCURRENCY"
  --poll-interval-ms "$POLL_INTERVAL_MS"
  --terminal-timeout-ms "$TERMINAL_TIMEOUT_MS"
)

if [[ -n "$ORGANIZATION_ID" ]]; then
  BENCH_ARGS+=(--organization-id "$ORGANIZATION_ID")
fi
if [[ -n "$CONVERSATION_ID" ]]; then
  BENCH_ARGS+=(--conversation-id "$CONVERSATION_ID")
fi
if [[ -n "$TOKEN" ]]; then
  BENCH_ARGS+=(--token "$TOKEN")
fi

# For baseline profile, run the in-process test instead
if [[ "$PROFILE" == "baseline" ]]; then
  echo "[agent-performance-suite] running baseline profile via test suite..." >&2
  node --test "$SCRIPT_DIR/agent-e2e-bench.test.mjs" 2>&1 | tee "$OUTPUT_DIR/test-output.txt" || {
    echo "[agent-performance-suite] baseline tests FAILED" >&2
    cleanup_fake_provider
    exit 1
  }
  echo "[agent-performance-suite] baseline profile passed" >&2
else
  # Run the benchmark CLI
  echo "[agent-performance-suite] running $PROFILE profile benchmark..." >&2
  node "$SCRIPT_DIR/agent-e2e-bench.mjs" "${BENCH_ARGS[@]}" > "$OUTPUT_DIR/report.json" 2>"$OUTPUT_DIR/bench-stderr.txt" || {
    echo "[agent-performance-suite] benchmark FAILED" >&2
    cat "$OUTPUT_DIR/bench-stderr.txt" >&2
    cleanup_fake_provider
    exit 1
  }
fi

# Record post-benchmark metrics
POST_METRICS_FILE="$OUTPUT_DIR/metrics-after.txt"
if [[ -n "$BASE_URL" && "$PROFILE" != "baseline" ]]; then
  METRICS_ENDPOINT="${METRICS_URL:-${BASE_URL%/}/api/v1/metrics}"
  curl -sf --max-time 5 "$METRICS_ENDPOINT" > "$POST_METRICS_FILE" 2>/dev/null || echo "# metrics unavailable" > "$POST_METRICS_FILE"
else
  echo "# baseline profile — no external metrics" > "$POST_METRICS_FILE"
fi

# Clean up fake provider
cleanup_fake_provider

# Generate Markdown summary
SUMMARY_FILE="$OUTPUT_DIR/summary.md"
{
  echo "# Agent Performance Suite Report"
  echo ""
  echo "| Field | Value |"
  echo "|-------|-------|"
  echo "| Profile | $PROFILE |"
  echo "| Runs | $RUNS |"
  echo "| Concurrency | $CONCURRENCY |"
  echo "| AllCallAll SHA | $ALLCALLALL_SHA |"
  echo "| Agent Runtime SHA | $AGENT_RUNTIME_SHA |"
  echo "| Host | $HOST_DATA |"
  echo "| Node | $NODE_VERSION |"
  echo "| Started | $(date -u +%Y-%m-%dT%H:%M:%SZ) |"
  echo ""

  if [[ "$PROFILE" != "baseline" && -f "$OUTPUT_DIR/report.json" ]]; then
    # Extract key fields from the JSON report
    ACCEPTED="$(node -e "const r=JSON.parse(require('fs').readFileSync('$OUTPUT_DIR/report.json','utf8'));console.log(r.accepted)" 2>/dev/null || echo '?')"
    READY="$(node -e "const r=JSON.parse(require('fs').readFileSync('$OUTPUT_DIR/report.json','utf8'));console.log(r.ready)" 2>/dev/null || echo '?')"
    FAILED="$(node -e "const r=JSON.parse(require('fs').readFileSync('$OUTPUT_DIR/report.json','utf8'));console.log(r.failed)" 2>/dev/null || echo '?')"
    TIMED_OUT="$(node -e "const r=JSON.parse(require('fs').readFileSync('$OUTPUT_DIR/report.json','utf8'));console.log(r.timedOut)" 2>/dev/null || echo '?')"
    E2E_P95="$(node -e "const r=JSON.parse(require('fs').readFileSync('$OUTPUT_DIR/report.json','utf8'));console.log(r.endToEndLatency?.p95 ?? 'N/A')" 2>/dev/null || echo '?')"
    ENQUEUE_P95="$(node -e "const r=JSON.parse(require('fs').readFileSync('$OUTPUT_DIR/report.json','utf8'));console.log(r.enqueueLatency?.p95 ?? 'N/A')" 2>/dev/null || echo '?')"
    QUEUE_P95="$(node -e "const r=JSON.parse(require('fs').readFileSync('$OUTPUT_DIR/report.json','utf8'));console.log(r.queueLatency?.p95 ?? 'N/A')" 2>/dev/null || echo '?')"

    echo "## Results"
    echo ""
    echo "| Metric | Value |"
    echo "|--------|-------|"
    echo "| Accepted | $ACCEPTED |"
    echo "| Ready | $READY |"
    echo "| Failed | $FAILED |"
    echo "| Timed Out | $TIMED_OUT |"
    echo "| Enqueue Latency p95 | ${ENQUEUE_P95} ms |"
    echo "| Queue Latency p95 | ${QUEUE_P95} ms |"
    echo "| End-to-End Latency p95 | ${E2E_P95} ms |"
    echo ""
  else
    echo "## Results"
    echo ""
    echo "Baseline profile: in-process test suite passed."
    echo ""
  fi

  echo "## Artifacts"
  echo ""
  echo "- \`report.json\` — full JSON benchmark report"
  echo "- \`metrics-before.txt\` — Prometheus metrics snapshot before benchmark"
  echo "- \`metrics-after.txt\` — Prometheus metrics snapshot after benchmark"
  echo "- \`summary.md\` — this summary"
} > "$SUMMARY_FILE"

# Print the summary
cat "$SUMMARY_FILE"

echo "" >&2
echo "[agent-performance-suite] complete. Artifacts in $OUTPUT_DIR" >&2
