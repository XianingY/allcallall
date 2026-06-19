# Resume Eval

This page is the stable entry point for resume-friendly quantitative evidence for the AllCallAll Agent system. The generated snapshot lives in [`generated-resume-eval/resume-eval.md`](generated-resume-eval/resume-eval.md).

## Regenerate

Run from the repository root:

```bash
make resume-eval
```

Or run the underlying CLI directly:

```bash
cd backend
go run ./cmd/allcallallctl resume-eval \
  -provider rules \
  -out ../docs/interview/generated-resume-eval
```

The command writes:

- `resume-eval.json`: machine-readable KPI summary plus raw eval and benchmark details.
- `resume-eval.md`: human-readable KPI summary for interviews and resume updates.
- `interview-bench.json`: local Agent/outbox benchmark evidence.
- `agent-demo-report.json`: raw planner, RAG, and workflow eval details.
- `agent-eval.json`, `rag-eval.json`, `workflow-eval.json`: per-suite source artifacts.

The command exits non-zero when any planner, RAG, or workflow case fails.

## Measurement Scope

Current resume-eval numbers are intentionally conservative:

- Provider: `rules`
- Planner / RAG / workflow cases: deterministic local fixtures
- Benchmark runtime: temporary SQLite with local worker execution
- Goal: prove quality gates, safety gates, and pipeline stability

This path is suitable for statements such as:

- planner, RAG, and workflow eval all pass under a deterministic regression harness
- meeting transcript context is actually consumed by workflow eval
- write tools are intercepted by approval rather than auto-executed
- the local Agent + outbox path can be benchmarked and reproduced on demand

This path is not a production throughput claim and should not be presented as external LLM quality evidence.

## Resume-Safe Metrics

As of the latest committed snapshot on June 19, 2026:

- Planner eval: `2/2` passed, average estimated prompt tokens `337.5`
- RAG eval: `2/2` passed, average latency `33.0 ms`, citation hit rate `100%`
- Workflow eval: `3/3` passed, approval interception rate `66.7%`, meeting transcript coverage `100%`
- Local Agent/outbox benchmark: `25/25` ready runs, `0` failed runs, execute-run `p95=5 ms`, `7.0` tool calls per run, `3.0` context chunks per run

When writing resume bullets, prefer the generated markdown snapshot plus one supporting raw artifact path instead of copying numbers by hand from multiple documents.

## Recommended Resume Wording

Good examples:

- Built a deterministic eval harness for planner, RAG, and workflow Agent paths; latest local rules-based snapshot passed planner `2/2`, RAG `2/2`, workflow `3/3`.
- Added measurable Agent safety and retrieval checks, including `100%` citation hit rate on the current RAG fixture set and `100%` meeting-transcript coverage on transcript-required workflow cases.
- Benchmarked the local Agent/outbox pipeline at `25/25` ready runs with `0` failures and `5 ms` execute-run p95 on temporary SQLite.

Avoid wording that implies:

- online A/B impact
- production SLA or capacity certification
- externally benchmarked model quality across open-ended prompts
