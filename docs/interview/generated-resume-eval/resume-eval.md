# AllCallAll Resume Eval Summary

- Generated at: `2026-06-19T12:34:50Z`
- Provider: `rules`
- Recommended resume-safe scope: `local deterministic rules + SQLite functional benchmark`

## KPI Summary

| Area | Metric | Value |
| --- | --- | --- |
| Planner | pass rate | 100.0% |
| Planner | avg prompt tokens | 337.5 |
| RAG | pass rate | 100.0% |
| RAG | avg latency | 33.0 ms |
| RAG | citation hit rate | 100.0% |
| Workflow | pass rate | 100.0% |
| Workflow | approval interception rate | 66.7% |
| Workflow | meeting transcript coverage | 100.0% |
| Benchmark | ready run rate | 100.0% |
| Benchmark | execute-run p95 | 5 ms |
| Benchmark | tool calls per run | 7.0 |
| Benchmark | context chunks per run | 3.0 |

## Resume-Ready Lines

- Deterministic planner/RAG/workflow eval all passed: planner `2/2`, RAG `2/2`, workflow `3/3`.
- RAG eval averaged `33.0 ms` per case with `100.0%` citation hit rate across vector and SQL fallback retrieval.
- Workflow eval achieved `100.0%` pass rate; `66.7%` of cases triggered approval interception and meeting-transcript coverage was `100.0%` on transcript-required cases.
- Local Agent/outbox benchmark completed `25/25` ready runs with `0` failures, `p95=5 ms` execute-run latency, and `7.0` tool calls per run.
