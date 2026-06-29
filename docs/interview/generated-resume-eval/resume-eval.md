# AllCallAll Resume Eval Summary

- Generated at: `2026-06-25T06:38:14Z`
- Provider: `rules`
- Recommended resume-safe scope: `current deterministic fixture set + local SQLite functional benchmark`
- Interpretation note: `these metrics validate regression stability and safety boundaries, not open-ended user satisfaction`

## KPI Summary

| Area | Metric | Value |
| --- | --- | --- |
| Regression | planner pass rate | 100.0% |
| Regression | workflow pass rate | 100.0% |
| Regression | task success rate | 100.0% |
| Regression | approval safety rate | 100.0% |
| RAG IR | answerable / negative cases | 32 / 8 |
| RAG IR | Top-K hit rate | 100.0% |
| RAG IR | negative pass rate | 100.0% |
| RAG IR | citation hit rate | 100.0% |
| RAG IR | citation error rate | 57.2% |
| RAG IR | Recall@K | 1.00 |
| RAG IR | Precision@K | 0.43 |
| RAG IR | MRR | 0.97 |
| RAG IR | NDCG@K | 0.98 |
| RAG IR | latency p50 / p95 | 83 ms / 181 ms |
| Benchmark | ready run rate | 100.0% |
| Benchmark | execute-run p95 | 31 ms |
| Benchmark | tool calls per run | 7.0 |
| Benchmark | context chunks per run | 3.0 |

## Resume-Ready Lines

- On the current deterministic fixture set, planner/RAG/workflow regression cases all passed: planner `2/2`, RAG `40/40`, workflow `3/3`.
- RAG retrieval on the current deterministic fixture set covers `32` answerable and `8` negative cases, tracking `Recall@K`, `Precision@K`, `MRR`, Top-K hit rate, negative pass rate, citation error rate, and p50/p95 latency.
- Workflow regression achieved `100.0%` pass rate; `66.7%` of cases triggered approval interception and meeting-transcript coverage was `100.0%` on transcript-required cases.
- A deterministic black-box task eval fixture set now checks natural-language task completion, tool selection, approval safety, and grounding; current task success rate is `100.0%` on `8` cases.
- Local Agent/outbox benchmark completed `25/25` ready runs with `0` failures, `p95=31 ms` execute-run latency, and `7.0` tool calls per run.
