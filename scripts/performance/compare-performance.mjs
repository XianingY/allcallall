#!/usr/bin/env node
// compare-performance.mjs — regression gate for archived performance reports.
//
// Compares a current run against a baseline report and fails when a guarded
// metric regresses beyond the threshold. Two report shapes are supported:
//
//   1. interviewbench JSON (cmd/interview-bench): queue_latency.p95_ms,
//      execute_run_latency.p95_ms, total_duration_ms, ready_runs,
//      failed_runs, failed_outbox_events.
//   2. agent-e2e-bench JSON (scripts/load/agent-e2e-bench.mjs):
//      enqueueLatency.p95, queueLatency.p95, runtimeLatency.p95,
//      endToEndLatency.p95, accepted/ready/failed/timedOut.
//
// Usage:
//   node scripts/performance/compare-performance.mjs \
//     --baseline baseline.json --current current.json [--threshold 0.10]
//
// Exit codes: 0 = pass, 1 = regression, 2 = usage/parse error.

import fs from "node:fs";

function parseArgs(argv) {
  const args = {};
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (!a.startsWith("--")) continue;
    const key = a.slice(2);
    const next = argv[i + 1];
    if (next !== undefined && !next.startsWith("--")) {
      args[key] = next;
      i++;
    } else {
      args[key] = "1";
    }
  }
  return args;
}

function readReport(path) {
  if (!path || !fs.existsSync(path)) {
    console.error(`[compare-performance] report not found: ${path ?? "(missing)"}`);
    process.exit(2);
  }
  try {
    return JSON.parse(fs.readFileSync(path, "utf8"));
  } catch (err) {
    console.error(`[compare-performance] failed to parse ${path}: ${err.message}`);
    process.exit(2);
  }
}

// Each metric: label, get(report), direction ("lower" = lower is better,
// "higher" = higher is better, "zero" = must stay at zero).
function metricsFor(report) {
  if (typeof report.queue_latency?.p95_ms === "number") {
    return [
      ["queue_latency.p95_ms", (r) => r.queue_latency?.p95_ms, "lower"],
      ["execute_run_latency.p95_ms", (r) => r.execute_run_latency?.p95_ms, "lower"],
      ["total_duration_ms", (r) => r.total_duration_ms, "lower"],
      ["ready_runs", (r) => r.ready_runs, "higher"],
      ["failed_runs", (r) => r.failed_runs, "zero"],
      ["failed_outbox_events", (r) => r.failed_outbox_events, "zero"],
    ];
  }
  if (typeof report.queueLatency?.p95 === "number") {
    return [
      ["enqueueLatency.p95", (r) => r.enqueueLatency?.p95, "lower"],
      ["queueLatency.p95", (r) => r.queueLatency?.p95, "lower"],
      ["runtimeLatency.p95", (r) => r.runtimeLatency?.p95, "lower"],
      ["endToEndLatency.p95", (r) => r.endToEndLatency?.p95, "lower"],
      ["accepted", (r) => r.accepted, "higher"],
      ["ready", (r) => r.ready, "higher"],
      ["failed", (r) => r.failed, "zero"],
      ["timedOut", (r) => r.timedOut, "zero"],
    ];
  }
  return null;
}

function formatNumber(value) {
  if (value === undefined || value === null) return "n/a";
  if (Number.isInteger(value)) return String(value);
  return value.toFixed(1);
}

const args = parseArgs(process.argv);
const baselinePath = args.baseline;
const currentPath = args.current;
const threshold = Number(args.threshold ?? "0.10");
if (!Number.isFinite(threshold) || threshold < 0) {
  console.error("[compare-performance] --threshold must be a non-negative number");
  process.exit(2);
}
if (!baselinePath || !currentPath) {
  console.error("[compare-performance] usage: --baseline <file> --current <file> [--threshold 0.10]");
  process.exit(2);
}

const baseline = readReport(baselinePath);
const current = readReport(currentPath);
const metrics = metricsFor(baseline);
if (!metrics) {
  console.error("[compare-performance] unrecognized report shape (expected interviewbench or agent-e2e-bench JSON)");
  process.exit(2);
}

const rows = [];
let failed = false;
for (const [label, get, direction] of metrics) {
  const before = get(baseline);
  const after = get(current);
  let status = "ok";
  let note = "";

  if (before === undefined || after === undefined) {
    status = "missing";
    note = "value missing in one report";
  } else if (direction === "zero") {
    if (after > 0) {
      status = "regression";
      note = "must stay 0";
      failed = true;
    }
  } else if (direction === "lower") {
    const limit = before * (1 + threshold);
    if (after > limit) {
      status = "regression";
      note = `> baseline×${(1 + threshold).toFixed(2)} (${formatNumber(limit)})`;
      failed = true;
    }
  } else {
    const limit = before * (1 - threshold);
    if (after < limit) {
      status = "regression";
      note = `< baseline×${(1 - threshold).toFixed(2)} (${formatNumber(limit)})`;
      failed = true;
    }
  }

  rows.push({ label, direction, before, after, status, note });
}

const width = Math.max(...rows.map((row) => row.label.length));
console.log(`[compare-performance] threshold=±${(threshold * 100).toFixed(0)}% baseline=${baselinePath} current=${currentPath}`);
for (const row of rows) {
  const mark = row.status === "ok" ? "✓" : "✗";
  console.log(
    `  ${mark} ${row.label.padEnd(width)} ${String(row.direction).padEnd(6)} ` +
      `baseline=${formatNumber(row.before).padStart(10)} current=${formatNumber(row.after).padStart(10)} ` +
      `${row.note}`
  );
}

if (failed) {
  console.error("[compare-performance] FAIL: performance regression detected");
  process.exit(1);
}
console.log("[compare-performance] PASS: no regression beyond threshold");
