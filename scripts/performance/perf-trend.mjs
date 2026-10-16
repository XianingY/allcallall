#!/usr/bin/env node
// perf-trend.mjs — summarize the local performance evidence archive.
//
// Scans performance-archive/<profile>/*/report.json and prints one line per
// run so regressions and threshold decisions are grounded in a history, not
// a single noisy sample. Handles both report shapes (interviewbench and
// agent-e2e-bench).
//
// Usage:
//   node scripts/performance/perf-trend.mjs [--dir performance-archive] [--profile interview-bench]

import fs from "node:fs";
import path from "node:path";

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

const scriptDir = path.dirname(new URL(import.meta.url).pathname);
const args = parseArgs(process.argv);
const archiveDir = args.dir ?? path.join(scriptDir, "..", "..", "performance-archive");
const profileFilter = args.profile;

if (!fs.existsSync(archiveDir)) {
  console.error(`[perf-trend] archive directory not found: ${archiveDir}`);
  console.error("[perf-trend] archive a run first: scripts/performance/archive-performance-run.sh <report.json> <profile>");
  process.exit(2);
}

function runLabel(dirName) {
  const stamp = dirName.slice(0, 16);
  return `${stamp.slice(0, 8)} ${stamp.slice(9, 11)}:${stamp.slice(11, 13)}:${stamp.slice(13, 15)}`;
}

function collectRuns() {
  const runs = [];
  for (const profile of fs.readdirSync(archiveDir).sort()) {
    const profileDir = path.join(archiveDir, profile);
    if (!fs.statSync(profileDir).isDirectory()) continue;
    if (profileFilter && profile !== profileFilter) continue;
    for (const entry of fs.readdirSync(profileDir).sort()) {
      const reportPath = path.join(profileDir, entry, "report.json");
      if (!fs.existsSync(reportPath)) continue;
      let report;
      try {
        report = JSON.parse(fs.readFileSync(reportPath, "utf8"));
      } catch {
        continue;
      }
      runs.push({ profile, dir: entry, report });
    }
  }
  return runs;
}

function metricsFor(report) {
  if (typeof report.queue_latency?.p95_ms === "number") {
    return {
      queueP95: report.queue_latency?.p95_ms,
      executeP95: report.execute_run_latency?.p95_ms,
      durationMs: report.total_duration_ms,
      throughput: report.ready_runs,
      failures: (report.failed_runs ?? 0) + (report.failed_outbox_events ?? 0),
    };
  }
  return {
    queueP95: report.queueLatency?.p95,
    executeP95: report.runtimeLatency?.p95,
    durationMs: report.endToEndLatency?.p95,
    throughput: report.ready,
    failures: (report.failed ?? 0) + (report.timedOut ?? 0),
  };
}

const runs = collectRuns();
if (runs.length === 0) {
  console.error("[perf-trend] no archived runs found");
  process.exit(2);
}

const columns = [
  ["run", 20],
  ["profile", 18],
  ["queue p95", 12],
  ["exec p95", 12],
  ["duration", 12],
  ["ready", 8],
  ["failures", 10],
];
console.log(columns.map(([name, width]) => name.padEnd(width)).join(""));
for (const run of runs) {
  const m = metricsFor(run.report);
  const cells = [
    runLabel(run.dir),
    run.profile,
    m.queueP95 !== undefined ? `${m.queueP95}ms` : "n/a",
    m.executeP95 !== undefined ? `${m.executeP95}ms` : "n/a",
    m.durationMs !== undefined ? `${m.durationMs}ms` : "n/a",
    m.throughput !== undefined ? String(m.throughput) : "n/a",
    String(m.failures ?? 0),
  ];
  console.log(cells.map((cell, i) => cell.padEnd(columns[i][1])).join(""));
}

console.log(`\n[perf-trend] ${runs.length} archived run(s) under ${archiveDir}`);
console.log("[perf-trend] compare any two: node scripts/performance/compare-performance.mjs --baseline <a>/report.json --current <b>/report.json");
