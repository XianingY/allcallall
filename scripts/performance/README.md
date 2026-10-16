# Performance Evidence Tooling

This directory holds the regression-gate and evidence tooling for the
performance work:

- `compare-performance.mjs` — compare two reports against a regression
  threshold. Exit 1 on regression; used by the nightly workflow and usable
  locally.
- `median-reports.mjs` — fold several runs (default: 3) into one
  per-metric median report to suppress single-run noise.
- `perf-trend.mjs` — print the history stored in the local archive.
- `archive-performance-run.sh` — copy one report into the local archive.
- `baselines/` — committed gate baselines, recorded **on the same hardware
  class the gate runs on** (GitHub Actions runners for the nightly gate).
  Never hand-edit the numbers; replace the whole file with a fresh
  median-of-3 artifact.

## Local workflow

```bash
# 1. Run any report-producing benchmark, e.g.
cd backend && go run ./cmd/interview-bench -conversations 25 -batch-size 50 > /tmp/bench.json

# 2. Archive it so perf-trend accumulates history
scripts/performance/archive-performance-run.sh /tmp/bench.json interview-bench

# 3. Inspect the trend
node scripts/performance/perf-trend.mjs

# 4. Compare two runs (local, same machine)
node scripts/performance/compare-performance.mjs \
  --baseline performance-archive/interview-bench/<old>/report.json \
  --current performance-archive/interview-bench/<new>/report.json
```

`run-agent-performance-suite.sh` archives automatically when
`PERFORMANCE_ARCHIVE=1` or `PERFORMANCE_ARCHIVE_DIR=<dir>` is set.

The archive (`performance-archive/` at the repo root) is gitignored: absolute
numbers are machine-specific, so local history is not shareable evidence.
The shared evidence channel is the nightly workflow's uploaded artifacts.

## Nightly gate

`.github/workflows/nightly-performance.yml` runs the deterministic
in-process benchmark (`cmd/interview-bench`, rules provider) three times,
takes the per-metric median, and compares it against
`baselines/nightly-interview-bench.json` with a 10% regression threshold.

If the baseline file is absent, the job records a baseline candidate artifact
instead of failing: commit that artifact as the baseline (from a PR), and the
gate activates on the next run. Re-record the baseline only from a dedicated
workflow_dispatch run, and record *why* in the commit message.

Threshold policy: the initial 10% is deliberately permissive on shared
runners. Tighten (or loosen) only with at least a week of nightly evidence,
and document the decision in the performance operations reference.
