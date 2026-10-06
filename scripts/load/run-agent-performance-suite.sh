#!/usr/bin/env bash
set -euo pipefail

# run-agent-performance-suite.sh — Run the end-to-end agent performance suite.
#
# Profiles:
#   baseline            — deterministic in-process orchestration and persistence checks
#   controlled          — Go + Agent Runtime + RAG Runtime with the fake provider;
#                         validates that the backend, Agent Runtime, and RAG Runtime
#                         endpoints are reachable before running
#   networked           — MySQL, Redis, Go, Agent Runtime, RAG Runtime, and fake provider
#                         in separate processes; validates MySQL and Redis connectivity
#                         in addition to all controlled-profile dependencies, and records
#                         the validated topology in the report
#   real-provider-canary — explicit opt-in, max 10 runs, concurrency 1, never the sole merge gate
#
# Provider URL contract:
#   The suite starts the fake-agent-provider for controlled and networked profiles and
#   sets AGENT_PROVIDER_URL to its reachable address. The Go backend must be configured
#   to use this provider via its own configuration (e.g. AGENT_PROVIDER_URL env var or
#   config.yaml agent_provider_url). The suite passes --provider-url to the benchmark
#   so the URL is recorded in the report, but does not reconfigure the backend itself.
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
#   FAKE_PROVIDER_PORT  — port for the fake provider (default: 18465)
#   FAKE_PROVIDER_LATENCY_MS   — fake provider base delay (default: 50)
#   FAKE_PROVIDER_FAILURE_RATE — fake provider failure probability 0–1 (default: 0)
#   FAKE_PROVIDER_TIMEOUT_RATE — fake provider timeout probability 0–1 (default: 0)
#   FAKE_PROVIDER_RESPONSE_BYTES — fake provider response size (default: 256)
#   AGENT_PROVIDER_URL  — override the provider URL recorded in the report (default: fake provider URL)
#   MYSQL_HOST          — MySQL host for networked profile validation (default: 127.0.0.1)
#   MYSQL_PORT          — MySQL port for networked profile validation (default: 3306)
#   REDIS_HOST          — Redis host for networked profile validation (default: 127.0.0.1)
#   REDIS_PORT          — Redis port for networked profile validation (default: 6379)
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
FAKE_PROVIDER_PORT="${FAKE_PROVIDER_PORT:-18465}"
AGENT_PROVIDER_URL="${AGENT_PROVIDER_URL:-}"
MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-3306}"
REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
REDIS_PORT="${REDIS_PORT:-6379}"

# Redact bearer tokens and provider keys from output
redact() {
  sed -E 's/Bearer [^ ]+/Bearer [REDACTED]/g; s/sk-[A-Za-z0-9_-]+/sk-[REDACTED]/g'
}

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

# ---------------------------------------------------------------------------
# Dependency validation
# ---------------------------------------------------------------------------

TOPOLOGY=""

# controlled: validate backend + Agent Runtime + RAG Runtime reachability
if [[ "$PROFILE" == "controlled" || "$PROFILE" == "networked" ]]; then
  if ! curl -sf -o /dev/null --max-time 5 "$BASE_URL/api/v1/metrics" 2>/dev/null; then
    echo "[agent-performance-suite] ERROR: $BASE_URL/api/v1/metrics not reachable — controlled profile requires a running backend" >&2
    exit 1
  fi
  TOPOLOGY+="backend=$BASE_URL"

  # Check Agent Runtime reachability (common default: port 8000)
  AGENT_RUNTIME_URL="${AGENT_RUNTIME_URL:-http://127.0.0.1:8000}"
  if curl -sf -o /dev/null --max-time 3 "$AGENT_RUNTIME_URL/health" 2>/dev/null; then
    TOPOLOGY+=" agent_runtime=$AGENT_RUNTIME_URL"
  else
    echo "[agent-performance-suite] WARNING: Agent Runtime at $AGENT_RUNTIME_URL not reachable — controlled profile may fail" >&2
    TOPOLOGY+=" agent_runtime=unreachable"
  fi

  # Check RAG Runtime reachability (common default: port 8001)
  RAG_RUNTIME_URL="${RAG_RUNTIME_URL:-http://127.0.0.1:8001}"
  if curl -sf -o /dev/null --max-time 3 "$RAG_RUNTIME_URL/health" 2>/dev/null; then
    TOPOLOGY+=" rag_runtime=$RAG_RUNTIME_URL"
  else
    echo "[agent-performance-suite] WARNING: RAG Runtime at $RAG_RUNTIME_URL not reachable — controlled profile may fail" >&2
    TOPOLOGY+=" rag_runtime=unreachable"
  fi
fi

# networked: additionally validate MySQL and Redis
if [[ "$PROFILE" == "networked" ]]; then
  # Validate MySQL connectivity
  if command -v mysql &>/dev/null; then
    if mysqladmin ping -h "$MYSQL_HOST" -P "$MYSQL_PORT" --silent 2>/dev/null; then
      TOPOLOGY+=" mysql=$MYSQL_HOST:$MYSQL_PORT"
    else
      echo "[agent-performance-suite] ERROR: MySQL at $MYSQL_HOST:$MYSQL_PORT not reachable — networked profile requires MySQL" >&2
      exit 1
    fi
  else
    # Fallback: try a TCP connection
    if command -v nc &>/dev/null && nc -z -w 3 "$MYSQL_HOST" "$MYSQL_PORT" 2>/dev/null; then
      TOPOLOGY+=" mysql=$MYSQL_HOST:$MYSQL_PORT"
    else
      echo "[agent-performance-suite] ERROR: Cannot validate MySQL at $MYSQL_HOST:$MYSQL_PORT (no mysql or nc client) — networked profile requires MySQL" >&2
      exit 1
    fi
  fi

  # Validate Redis connectivity
  if command -v redis-cli &>/dev/null; then
    if redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ping 2>/dev/null | grep -q PONG; then
      TOPOLOGY+=" redis=$REDIS_HOST:$REDIS_PORT"
    else
      echo "[agent-performance-suite] ERROR: Redis at $REDIS_HOST:$REDIS_PORT not reachable — networked profile requires Redis" >&2
      exit 1
    fi
  else
    # Fallback: try a TCP connection
    if command -v nc &>/dev/null && nc -z -w 3 "$REDIS_HOST" "$REDIS_PORT" 2>/dev/null; then
      TOPOLOGY+=" redis=$REDIS_HOST:$REDIS_PORT"
    else
      echo "[agent-performance-suite] ERROR: Cannot validate Redis at $REDIS_HOST:$REDIS_PORT (no redis-cli or nc client) — networked profile requires Redis" >&2
      exit 1
    fi
  fi
fi

# ---------------------------------------------------------------------------
# Output directory and SHAs
# ---------------------------------------------------------------------------

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

# ---------------------------------------------------------------------------
# Fake provider lifecycle
# ---------------------------------------------------------------------------

FAKE_PROVIDER_PID=""
FAKE_PROVIDER_URL=""

cleanup_fake_provider() {
  if [[ -n "$FAKE_PROVIDER_PID" ]]; then
    kill "$FAKE_PROVIDER_PID" 2>/dev/null || true
    wait "$FAKE_PROVIDER_PID" 2>/dev/null || true
    FAKE_PROVIDER_PID=""
    FAKE_PROVIDER_URL=""
  fi
}

start_fake_provider() {
  export FAKE_PROVIDER_LATENCY_MS="${FAKE_PROVIDER_LATENCY_MS:-50}"
  export FAKE_PROVIDER_FAILURE_RATE="${FAKE_PROVIDER_FAILURE_RATE:-0}"
  export FAKE_PROVIDER_TIMEOUT_RATE="${FAKE_PROVIDER_TIMEOUT_RATE:-0}"
  export FAKE_PROVIDER_RESPONSE_BYTES="${FAKE_PROVIDER_RESPONSE_BYTES:-256}"
  export FAKE_PROVIDER_PORT

  node "$SCRIPT_DIR/fake-agent-provider.mjs" &
  FAKE_PROVIDER_PID=$!
  FAKE_PROVIDER_URL="http://127.0.0.1:${FAKE_PROVIDER_PORT}"

  # Wait for /health readiness (up to 10 seconds)
  local retries=0
  local max_retries=50
  while [[ $retries -lt $max_retries ]]; do
    if curl -sf -o /dev/null --max-time 1 "$FAKE_PROVIDER_URL/health" 2>/dev/null; then
      echo "[agent-performance-suite] fake provider ready at $FAKE_PROVIDER_URL" >&2
      return 0
    fi
    retries=$((retries + 1))
    sleep 0.2
  done

  echo "[agent-performance-suite] ERROR: fake provider at $FAKE_PROVIDER_URL failed to become ready" >&2
  cleanup_fake_provider
  exit 1
}

# Start fake provider for controlled/networked profiles
if [[ "$PROFILE" == "controlled" || "$PROFILE" == "networked" ]]; then
  start_fake_provider
fi

# Set the provider URL for the report
PROVIDER_URL_FOR_REPORT="${AGENT_PROVIDER_URL:-$FAKE_PROVIDER_URL}"

# ---------------------------------------------------------------------------
# Run the benchmark
# ---------------------------------------------------------------------------

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
if [[ -n "$PROVIDER_URL_FOR_REPORT" ]]; then
  BENCH_ARGS+=(--provider-url "$PROVIDER_URL_FOR_REPORT")
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
    cat "$OUTPUT_DIR/bench-stderr.txt" | redact >&2
    cleanup_fake_provider
    exit 1
  }
  # Redact stderr output in the artifact
  if [[ -f "$OUTPUT_DIR/bench-stderr.txt" ]]; then
    redact < "$OUTPUT_DIR/bench-stderr.txt" > "$OUTPUT_DIR/bench-stderr-redacted.txt" 2>/dev/null || true
  fi
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

# ---------------------------------------------------------------------------
# Generate Markdown summary
# ---------------------------------------------------------------------------

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
  if [[ -n "$PROVIDER_URL_FOR_REPORT" ]]; then
    echo "| Provider URL | $PROVIDER_URL_FOR_REPORT |"
  fi
  if [[ -n "$TOPOLOGY" ]]; then
    echo "| Validated Topology | $TOPOLOGY |"
  fi
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
    TOKEN_CEILING="$(node -e "const r=JSON.parse(require('fs').readFileSync('$OUTPUT_DIR/report.json','utf8'));console.log(r.tokenCeiling ? JSON.stringify(r.tokenCeiling) : 'N/A')" 2>/dev/null || echo 'N/A')"

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
    echo "| Token Ceiling | $TOKEN_CEILING |"
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
