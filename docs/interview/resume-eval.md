# Resume Eval

This page is the stable entry point for resume-friendly quantitative evidence for the AllCallAll Agent system. The generated snapshot lives in [`generated-resume-eval/resume-eval.md`](generated-resume-eval/resume-eval.md).

For user-experience-oriented scoring that is intentionally kept separate from reproducible KPI artifacts, see [`agent-ux-eval.md`](agent-ux-eval.md).

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
- `task-eval.json`: reproducible black-box task-eval artifact for current natural-language task fixtures.
- `task-eval.md`: task-level Markdown summary.
- `interview-bench.json`: local Agent/outbox benchmark evidence.
- `agent-demo-report.json`: raw planner, RAG, and workflow eval details.
- `agent-eval.json`, `rag-eval.json`, `workflow-eval.json`: per-suite source artifacts.

The command exits non-zero when any planner, RAG, or workflow case fails.

## Measurement Scope

Current resume-eval numbers are intentionally scoped:

- Provider: `rules`
- Planner / RAG / workflow / task cases: deterministic local fixtures
- Benchmark runtime: temporary SQLite with local worker execution
- Goal: prove regression stability, safety gates, and pipeline stability

This path is suitable for statements such as:

- planner, RAG, workflow, and task-eval cases pass under a deterministic regression harness
- meeting transcript context is actually consumed by workflow eval
- write tools are intercepted by approval rather than auto-executed
- black-box task fixtures can verify natural-language task completion, tool choice, and grounding
- the local Agent + outbox path can be benchmarked and reproduced on demand

This path is not a production throughput claim and should not be presented as external LLM quality evidence.

## Resume-Safe Metrics

As of the latest committed snapshot on June 19, 2026:

- Planner eval: current fixture set `2/2` passed, average estimated prompt tokens `337.5`
- RAG eval: current fixture set `2/2` passed, `Recall@K=1.00`, `Precision@K=0.75`, `MRR=1.00`, citation hit rate `100%`
- Workflow eval: current fixture set `3/3` passed, approval interception rate `66.7%`, meeting transcript coverage `100%`
- Black-box task eval: current fixture set `8/8` passed, used for task completion / tool intent / approval safety / grounding checks
- Local Agent/outbox benchmark: `25/25` ready runs, `0` failed runs, latest embedded benchmark execute-run `p95=12 ms`, `7.0` tool calls per run, `3.0` context chunks per run

When writing resume bullets, prefer the generated markdown snapshot plus one supporting raw artifact path instead of copying numbers by hand from multiple documents.

## Recommended Resume Wording

Good examples:

- Built a deterministic regression harness for planner, RAG, workflow, and task-level Agent paths; current local fixture sets are reproducible and used for regression checks.
- Added measurable retrieval and safety checks, including RAG citation / IR metrics, meeting-transcript grounding, and approval interception for write tools.
- Added a black-box task-eval layer for natural-language task completion and kept manual UX scoring separate from reproducible benchmark artifacts.
- Benchmarked the local Agent/outbox pipeline at `25/25` ready runs with `0` failures and low-latency execute-run p95 on temporary SQLite.

Avoid wording that implies:

- online A/B impact
- production SLA or capacity certification
- externally benchmarked model quality across open-ended prompts
