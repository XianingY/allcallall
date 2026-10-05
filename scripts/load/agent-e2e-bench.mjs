#!/usr/bin/env node

/**
 * agent-e2e-bench.mjs — Reproducible end-to-end Agent performance benchmark.
 *
 * Exports runAgentBenchmark(options) → Promise<AgentBenchmarkReport>.
 * Also usable as a CLI that emits one JSON document to stdout.
 *
 * Never prints bearer tokens or provider credentials.
 */

import { execSync } from "node:child_process";
import { fileURLToPath } from "node:url";

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

function percentile(sorted, p) {
  if (sorted.length === 0) return { p50: 0, p95: 0, p99: 0, max: 0, count: 0 };
  const at = (frac) => sorted[Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * frac) - 1))];
  return {
    p50: at(0.5),
    p95: at(0.95),
    p99: at(0.99),
    max: sorted[sorted.length - 1],
    count: sorted.length,
  };
}

function latencyDist(samples) {
  return percentile([...samples].sort((a, b) => a - b));
}

/** Redact bearer tokens from a string — used before any stdout write. */
function redact(s) {
  return s.replace(/Bearer\s+\S+/gi, "Bearer [REDACTED]");
}

/** Read a git SHA safely; returns "unknown" on failure. */
function gitSha(cwd) {
  try {
    return execSync("git rev-parse HEAD", { cwd, encoding: "utf-8" }).trim();
  } catch {
    return "unknown";
  }
}

/** Stable run ID derived from timestamp + random suffix. */
function makeRunId() {
  const ts = Date.now().toString(36);
  const rnd = Math.random().toString(36).slice(2, 8);
  return `bench-${ts}-${rnd}`;
}

/** Parse Prometheus text format into a flat map of metric→value. */
function parsePrometheusText(text) {
  const map = {};
  for (const line of text.split("\n")) {
    if (line.startsWith("#") || !line.trim()) continue;
    const spaceIdx = line.lastIndexOf(" ");
    if (spaceIdx < 1) continue;
    const key = line.slice(0, spaceIdx);
    const val = Number(line.slice(spaceIdx + 1));
    if (Number.isFinite(val)) map[key] = val;
  }
  return map;
}

/** Fetch Prometheus metrics and return parsed map. */
async function fetchMetrics(url) {
  if (!url) return null;
  try {
    const res = await fetch(url, { signal: AbortSignal.timeout(5000) });
    if (!res.ok) return null;
    const text = await res.text();
    return parsePrometheusText(text);
  } catch {
    return null;
  }
}

/** Compute delta between two Prometheus metric maps. */
function computeMetricDeltas(before, after) {
  if (!before || !after) return {};
  const deltas = {};
  for (const key of Object.keys(after)) {
    const bv = before[key];
    const av = after[key];
    if (bv !== undefined && av !== undefined && av >= bv) {
      deltas[key] = av - bv;
    } else if (bv === undefined) {
      // New metric appeared — treat full value as delta
      deltas[key] = av;
    }
    // If av < bv (counter reset), skip — not meaningful as a delta
  }
  return deltas;
}

// ---------------------------------------------------------------------------
// Core benchmark logic
// ---------------------------------------------------------------------------

/**
 * Run a single agent run creation + poll-to-terminal cycle.
 * Returns a result object with timing phases and terminal status.
 */
async function executeRun({ baseUrl, token, organizationId, conversationId, runIndex, pollIntervalMs, terminalTimeoutMs }) {
  const idempotencyKey = `bench-${makeRunId()}-${runIndex}`;
  const headers = {
    "Content-Type": "application/json",
    "Idempotency-Key": idempotencyKey,
  };
  if (organizationId) headers["X-Organization-ID"] = String(organizationId);
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const body = {
    conversation_id: Number(conversationId) || 1,
    goal: `benchmark run ${runIndex}`,
  };

  // Phase 1: Enqueue (POST /api/v1/agent/runs)
  const enqueueStart = performance.now();
  let createRes;
  try {
    createRes = await fetch(`${baseUrl}/api/v1/agent/runs`, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(30000),
    });
  } catch (err) {
    const enqueueEnd = performance.now();
    return {
      accepted: false,
      timedOut: false,
      failed: true,
      terminalStatus: "create_error",
      enqueueLatencyMs: enqueueEnd - enqueueStart,
      queueLatencyMs: 0,
      runtimeLatencyMs: 0,
      endToEndLatencyMs: enqueueEnd - enqueueStart,
      runId: null,
      requestId: null,
      traceId: null,
      error: redact(err.message),
    };
  }
  const enqueueEnd = performance.now();
  const enqueueLatencyMs = enqueueEnd - enqueueStart;

  if (!createRes.ok && createRes.status !== 202) {
    return {
      accepted: false,
      timedOut: false,
      failed: true,
      terminalStatus: `http_${createRes.status}`,
      enqueueLatencyMs,
      queueLatencyMs: 0,
      runtimeLatencyMs: 0,
      endToEndLatencyMs: enqueueLatencyMs,
      runId: null,
      requestId: null,
      traceId: null,
      error: redact(`create returned ${createRes.status}`),
    };
  }

  let createBody;
  try {
    createBody = await createRes.json();
  } catch {
    createBody = {};
  }

  const runId = createBody?.run?.id ?? createBody?.id ?? null;
  const requestId = createBody?.run?.request_id ?? createBody?.request_id ?? null;
  const traceId = createBody?.run?.trace_id ?? createBody?.trace_id ?? null;

  // Phase 2: Poll until terminal status (ready/failed) or timeout
  const pollHeaders = { ...headers };
  delete pollHeaders["Content-Type"]; // GET doesn't need it
  delete pollHeaders["Idempotency-Key"];

  const deadline = Date.now() + terminalTimeoutMs;
  let firstRunningSeen = false;
  let queueLatencyMs = 0;
  let runtimeLatencyMs = 0;
  let terminalStatus = "pending";
  let pollCount = 0;

  while (Date.now() < deadline) {
    // Small sleep before first poll to let the server advance state
    await new Promise((r) => setTimeout(r, pollIntervalMs));

    let pollRes;
    try {
      pollRes = await fetch(`${baseUrl}/api/v1/agent/runs/${runId}`, {
        method: "GET",
        headers: pollHeaders,
        signal: AbortSignal.timeout(10000),
      });
    } catch {
      pollCount++;
      continue;
    }

    if (!pollRes.ok) {
      pollCount++;
      continue;
    }

    let pollBody;
    try {
      pollBody = await pollRes.json();
    } catch {
      pollCount++;
      continue;
    }

    const status = pollBody?.run?.status ?? pollBody?.status ?? "unknown";
    pollCount++;

    if (status === "running" && !firstRunningSeen) {
      firstRunningSeen = true;
      queueLatencyMs = performance.now() - enqueueEnd;
    }

    if (status === "ready" || status === "failed") {
      const now = performance.now();
      if (firstRunningSeen) {
        runtimeLatencyMs = now - enqueueEnd - queueLatencyMs;
      } else {
        // Never saw running — treat queue latency as time to terminal
        queueLatencyMs = now - enqueueEnd;
      }
      terminalStatus = status;
      break;
    }

    // Continue polling for pending/running
  }

  const endToEndLatencyMs = performance.now() - enqueueStart;
  const timedOut = !["ready", "failed"].includes(terminalStatus);

  return {
    accepted: true,
    timedOut,
    failed: terminalStatus === "failed" || timedOut,
    terminalStatus,
    enqueueLatencyMs,
    queueLatencyMs,
    runtimeLatencyMs,
    endToEndLatencyMs,
    runId,
    requestId,
    traceId,
    error: timedOut ? `timed out after ${terminalTimeoutMs}ms in status ${terminalStatus}` : "",
  };
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Run the agent benchmark and return a structured report.
 *
 * @param {object} options
 * @param {string}  options.baseUrl           - API base URL (e.g. http://localhost:8080)
 * @param {string}  [options.metricsUrl]      - Prometheus metrics URL (default: baseUrl + /api/v1/metrics)
 * @param {string}  [options.token]           - Bearer token (redacted from output)
 * @param {string|number} [options.organizationId] - X-Organization-ID header value
 * @param {string|number} [options.conversationId] - conversation_id for run creation
 * @param {number}  [options.runs=10]         - Total runs to execute
 * @param {number}  [options.concurrency=1]   - Worker pool size
 * @param {number}  [options.pollIntervalMs=100] - Milliseconds between poll attempts
 * @param {number}  [options.terminalTimeoutMs=60000] - Per-run timeout waiting for terminal status
 * @returns {Promise<AgentBenchmarkReport>}
 */
export async function runAgentBenchmark(options = {}) {
  const {
    baseUrl,
    metricsUrl: metricsUrlOpt,
    token,
    organizationId,
    conversationId,
  } = options;

  const runs = Number(options.runs) || 10;
  const concurrency = Math.max(1, Number(options.concurrency) || 1);
  const pollIntervalMs = Number(options.pollIntervalMs) || 100;
  const terminalTimeoutMs = Number(options.terminalTimeoutMs) || 60000;
  const metricsUrl = metricsUrlOpt || (baseUrl ? `${baseUrl.replace(/\/+$/, "")}/api/v1/metrics` : null);

  const runId = makeRunId();

  // Capture repository SHAs
  const allcallallSha = gitSha(process.cwd());
  let agentRuntimeSha = "unknown";
  try {
    agentRuntimeSha = gitSha("../allcallall-agent-runtime");
  } catch {
    // sibling repo not checked out
  }

  // Capture pre-benchmark metrics
  const metricsBefore = await fetchMetrics(metricsUrl);

  // Execute runs with a bounded worker pool
  const results = [];
  let nextRun = 0;

  async function worker() {
    while (nextRun < runs) {
      const idx = nextRun++;
      const result = await executeRun({
        baseUrl,
        token,
        organizationId,
        conversationId,
        runIndex: idx,
        pollIntervalMs,
        terminalTimeoutMs,
      });
      results.push(result);
    }
  }

  const startedAt = new Date();
  await Promise.all(Array.from({ length: concurrency }, () => worker()));
  const finishedAt = new Date();

  // Capture post-benchmark metrics
  const metricsAfter = await fetchMetrics(metricsUrl);
  const metricDeltas = computeMetricDeltas(metricsBefore, metricsAfter);

  // Aggregate results
  let accepted = 0;
  let ready = 0;
  let failed = 0;
  let timedOut = 0;
  const statusCounts = {};
  const enqueueSamples = [];
  const queueSamples = [];
  const runtimeSamples = [];
  const e2eSamples = [];
  const requestIds = [];
  const traceIds = [];

  for (const r of results) {
    if (r.accepted) accepted++;
    if (r.terminalStatus === "ready") ready++;
    if (r.terminalStatus === "failed") failed++;
    if (r.timedOut) timedOut++;
    statusCounts[r.terminalStatus] = (statusCounts[r.terminalStatus] || 0) + 1;
    enqueueSamples.push(r.enqueueLatencyMs);
    queueSamples.push(r.queueLatencyMs);
    runtimeSamples.push(r.runtimeLatencyMs);
    e2eSamples.push(r.endToEndLatencyMs);
    if (r.requestId) requestIds.push(r.requestId);
    if (r.traceId) traceIds.push(r.traceId);
  }

  const report = {
    runId,
    startedAt: startedAt.toISOString(),
    finishedAt: finishedAt.toISOString(),
    configuration: {
      runs,
      concurrency,
      pollIntervalMs,
      terminalTimeoutMs,
      baseUrl: baseUrl ? baseUrl.replace(/\/+$/, "") : null,
      organizationId: organizationId ? String(organizationId) : null,
    },
    repositoryShas: {
      allcallall: allcallallSha,
      agentRuntime: agentRuntimeSha,
    },
    accepted,
    ready,
    failed,
    timedOut,
    enqueueLatency: latencyDist(enqueueSamples),
    queueLatency: latencyDist(queueSamples),
    runtimeLatency: latencyDist(runtimeSamples),
    endToEndLatency: latencyDist(e2eSamples),
    statusCounts,
    requestIds,
    traceIds,
    metricDeltas,
  };

  return report;
}

// ---------------------------------------------------------------------------
// CLI entry point
// ---------------------------------------------------------------------------

function parseArgs(argv) {
  const args = {};
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith("--")) {
      const key = a.slice(2);
      const val = argv[i + 1];
      if (val && !val.startsWith("--")) {
        args[key] = val;
        i++;
      } else {
        args[key] = "1";
      }
    }
  }
  return args;
}

function envOrArg(args, argKey, envKey, fallback) {
  return args[argKey] ?? process.env[envKey] ?? fallback;
}

async function cli() {
  const args = parseArgs(process.argv);
  const baseUrl = envOrArg(args, "base-url", "BASE_URL");
  const token = envOrArg(args, "token", "TOKEN");
  const organizationId = envOrArg(args, "organization-id", "ORGANIZATION_ID");
  const conversationId = envOrArg(args, "conversation-id", "CONVERSATION_ID");
  const runs = Number(envOrArg(args, "runs", "RUNS", "10"));
  const concurrency = Number(envOrArg(args, "concurrency", "CONCURRENCY", "1"));
  const pollIntervalMs = Number(envOrArg(args, "poll-interval-ms", "POLL_INTERVAL_MS", "100"));
  const terminalTimeoutMs = Number(envOrArg(args, "terminal-timeout-ms", "TERMINAL_TIMEOUT_MS", "60000"));

  if (!baseUrl) {
    console.error("[agent-e2e-bench] --base-url / BASE_URL is required");
    process.exit(2);
  }

  const report = await runAgentBenchmark({
    baseUrl,
    token,
    organizationId,
    conversationId,
    runs,
    concurrency,
    pollIntervalMs,
    terminalTimeoutMs,
  });

  // Emit JSON to stdout — redact any accidentally included tokens
  const json = JSON.stringify(report, null, 2);
  console.log(redact(json));
}

// Run CLI when executed directly, not when imported
const __filename = fileURLToPath(import.meta.url);
const isMain = process.argv[1] && __filename === new URL(`file://${process.argv[1]}`).pathname;
if (isMain) {
  cli().catch((err) => {
    console.error(`[agent-e2e-bench] ${redact(err.message)}`);
    process.exit(2);
  });
}
