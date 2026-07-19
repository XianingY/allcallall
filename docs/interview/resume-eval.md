# Resume Eval

This is a deterministic regression artifact, not a production KPI report. The current live
acceptance path is documented in [Interview Chain](interview-chain.md) and must pass
`make interview-smoke && make interview-chaos` before quoting the chain as runnable.

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

The `Task eval runtime: legacy_go` field in the generated report is intentional: this combined
CLI still uses the legacy Go task fixture harness. Python LangGraph quality is reported separately
by `make python-agent-eval` (`9/9` current fixtures) and Python RAG by `make python-rag-eval`.

This path is suitable for statements such as:

- planner, RAG, workflow, and task-eval cases pass under a deterministic regression harness
- meeting transcript context is actually consumed by workflow eval
- write tools are intercepted by approval rather than auto-executed
- black-box task fixtures can verify natural-language task completion, tool choice, and grounding
- the local Agent + outbox path can be benchmarked and reproduced on demand

This path is not a production throughput claim and should not be presented as external LLM quality evidence.

## Resume-Safe Metrics

As of the latest local snapshot after the expanded RAG fixture update:

- Planner eval: current fixture set `2/2` passed, average estimated prompt tokens `337.5`
- RAG eval: current fixture set `40/40` passed, with `32` answerable cases, `8` negative/no-answer cases, distractor documents, `Recall@K=1.00`, `Precision@K≈0.43`, `MRR≈0.97`, negative pass rate `100%`, and citation error rate tracked separately
- Workflow eval: current fixture set `3/3` passed, approval interception rate `66.7%`, meeting transcript coverage `100%`
- Black-box task eval: current fixture set `8/8` passed, used for task completion / tool intent / approval safety / grounding checks
- Local Agent/outbox benchmark: `25/25` ready runs, `0` failed runs, latest embedded benchmark records execute-run p95, `7.0` tool calls per run, and `3.0` context chunks per run
- Historical live local API QPS snapshot: retained for context only; it was measured before the current Compose chain and is not a current acceptance or capacity claim.

When writing resume bullets, prefer the generated markdown snapshot plus one supporting raw artifact path instead of copying numbers by hand from multiple documents.

## Recommended Resume Wording

Good examples:

- Built a deterministic regression harness for planner, RAG, workflow, and task-level Agent paths; current local fixture sets are reproducible and used for regression checks.
- Added measurable retrieval and safety checks, including RAG citation / IR metrics, no-answer negative cases, distractor-document precision checks, meeting-transcript grounding, and approval interception for write tools.
- Added a black-box task-eval layer for natural-language task completion and kept manual UX scoring separate from reproducible benchmark artifacts.
- Benchmarked the local Agent/outbox pipeline at `25/25` ready runs with `0` failures and low-latency execute-run p95 on temporary SQLite.
- Measured core API local QPS on MySQL/Redis at 20 concurrency for 60 seconds, covering read, write + outbox enqueue, and Agent run create/enqueue paths; present these as local benchmark evidence, not production SLA.

Avoid wording that implies:

- online A/B impact
- production SLA or capacity certification
- externally benchmarked model quality across open-ended prompts
