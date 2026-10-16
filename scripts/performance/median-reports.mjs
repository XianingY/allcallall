#!/usr/bin/env node
// median-reports.mjs — fold several benchmark runs into one median report.
//
// Nightly noise on shared runners is dominated by scheduling and IO jitter;
// taking the per-metric median of 3 runs keeps the regression gate from
// failing on a single noisy sample. All numeric leaf fields are reduced by
// median (numbers of arrays of numbers); objects are recursed, non-numeric
// values come from the first report.
//
// Usage:
//   node scripts/performance/median-reports.mjs --output median.json a.json b.json c.json

import fs from "node:fs";

function parseArgs(argv) {
  const args = { reports: [] };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--output") {
      args.output = argv[++i];
    } else {
      args.reports.push(a);
    }
  }
  return args;
}

function median(values) {
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  if (sorted.length % 2 === 1) return sorted[mid];
  return (sorted[mid - 1] + sorted[mid]) / 2;
}

function isPlainObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function medianMerge(values) {
  if (values.every((value) => typeof value === "number" && Number.isFinite(value))) {
    return median(values);
  }
  if (values.every(isPlainObject)) {
    const merged = {};
    const keys = new Set(values.flatMap((value) => Object.keys(value)));
    for (const key of keys) {
      merged[key] = medianMerge(values.map((value) => value[key]).filter((value) => value !== undefined));
    }
    return merged;
  }
  if (values.every(Array.isArray)) {
    // Arrays (request ids, trace ids, ...) are run-specific; keep the first.
    return values[0];
  }
  // Strings / null / mixed: keep the first report's value.
  return values[0];
}

const args = parseArgs(process.argv);
if (!args.output || args.reports.length < 2) {
  console.error("[median-reports] usage: --output median.json a.json b.json [c.json ...]");
  process.exit(2);
}

const reports = args.reports.map((path) => {
  try {
    return JSON.parse(fs.readFileSync(path, "utf8"));
  } catch (err) {
    console.error(`[median-reports] failed to parse ${path}: ${err.message}`);
    process.exit(2);
  }
});

const merged = medianMerge(reports);
merged.medianOf = { reports: args.reports, computedAt: new Date().toISOString() };

fs.writeFileSync(args.output, JSON.stringify(merged, null, 2) + "\n");
console.log(`[median-reports] wrote median of ${args.reports.length} reports to ${args.output}`);
