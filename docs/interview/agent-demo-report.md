# Agent Demo Eval Report

This page is the stable entry point for the AllCallAll Agent demo eval. The generated snapshot lives in [`generated-agent-report/agent-demo-report.md`](generated-agent-report/agent-demo-report.md).

For resume-friendly KPI rollups that combine eval quality and local benchmark data, use [`resume-eval.md`](resume-eval.md).

## Regenerate

Run from the repository root:

```bash
make agent-demo-report
```

Or run the underlying CLI directly:

```bash
cd backend
go run ./cmd/allcallallctl eval \
  -provider rules \
  -out ../docs/interview/generated-agent-report
```

The CLI writes:

- `agent-eval.json`: deterministic planner cases.
- `rag-eval.json`: RAG retrieval, citation, vector, and SQL fallback cases.
- `workflow-eval.json`: fixed DAG multi-agent workflow cases.
- `agent-demo-report.json`: combined machine-readable report.
- `agent-demo-report.md`: combined human-readable report.

The command exits non-zero when any case fails, so the same path can be wired into CI later.

## Latest Snapshot

Current generated status:

| Suite | Cases | Passed | Failed |
| --- | ---: | ---: | ---: |
| Planner | 2 | 2 | 0 |
| RAG | 2 | 2 | 0 |
| Workflow | 3 | 3 | 0 |

What this proves:

- The rules planner can produce non-empty summaries, next steps, risks, and action items for seeded conversation fixtures.
- RAG retrieval returns cited chunks in both vector and SQL fallback modes.
- The workflow engine can build the fixed DAG, run bounded ReAct loops inside selected role tasks, pause for approvals, execute approved tools, and fail early when policy denies a write tool.

## Interview Talking Points

- The eval is deterministic and does not require external model credentials.
- RAG eval uses a local in-memory vector index so vector behavior is testable without Elasticsearch.
- Workflow eval runs against a temporary SQLite database and still exercises real service code, bounded role ReAct, task state, approvals, and tool execution.
- The generated Markdown is for humans; the JSON files are for automated regression checks.
