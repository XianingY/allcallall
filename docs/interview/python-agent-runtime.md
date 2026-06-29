# Python Agent Runtime

This page is the interview-facing explanation for the Python Agent Runtime.

## What Changed

AllCallAll keeps the Go backend as the business source of truth, but extracts selected Agent orchestration into `agent-runtime/`, a Python FastAPI + LangGraph service.

The Python runtime currently supports:

- `meeting_brief`
- `risk_review`
- `follow_up_planner`
- `context_qa`

The runtime exposes:

- `GET /health`
- `GET /v1/workflows`
- `POST /v1/workflows/{preset}/run`
- `POST /v1/workflows/meeting-brief/run` for compatibility

## Boundary

Go owns:

- authentication and organization membership
- conversation, meeting, transcript, and knowledge data
- tool schema and permission policy
- read-tool authorization
- human approvals and audit history
- all final writes and side effects

Python owns:

- workflow registry
- LangGraph DAG orchestration
- bounded ReAct role loops
- retrieval/rerank orchestration over Go-supplied context chunks
- prompt/provider adapter logic
- versioned prompt templates such as `meeting_brief_v1`, `risk_review_v1`, `follow_up_planner_v1`, and `context_qa_v1`
- citation grounding checks
- structured trace events
- citation and proposal generation
- Python-side task eval

Python can call Go read-only tools through the internal bridge:

- Go: `AGENT_RUNTIME_TOOL_TOKEN`
- Python: `PY_AGENT_TOOL_BRIDGE_BASE_URL`, `PY_AGENT_TOOL_BRIDGE_TOKEN`

Write tools are never executed by Python. Python returns proposals such as `write_conversation_message`, `create_follow_up_task`, and `upsert_agent_memory`; Go converts them into pending approvals.

## Interview Explanation

The split is not “Python replaces Go.” It is a deliberate Agent runtime boundary:

- Go is better for the stable product backend: auth, collaboration data, transactions, workers, permissions, and audit.
- Python is better for fast AI iteration: LangGraph, provider adapters, prompt experiments, structured output handling, and eval scripts.
- The Agent cannot bypass product safety boundaries because the Python service has no direct database write path.
- Read tools can run automatically after Go authorizes them; write tools always require approval.

## Eval

Run:

```bash
make python-agent-eval
make rerank-eval
make ai-portfolio-eval
```

Output:

- `agent-runtime/evals/reports/python-agent-eval.json`
- `agent-runtime/evals/reports/python-agent-eval.md`
- `docs/interview/generated-rerank-eval/rerank-eval.json`
- `docs/interview/generated-ai-portfolio-eval/ai-portfolio-eval.md`

Current deterministic fixture scope:

- task success
- citation grounding
- tool intent match
- approval safety
- unsupported-claim guarding
- prompt schema / prompt version presence
- grounding-check trace presence

`agent-runtime/app/llamaindex_adapter.py` is an eval-only adapter for comparing a LlamaIndex retrieval baseline on fixture documents. It does not replace the Go knowledge store, because production retrieval still needs organization isolation, source metadata, approval boundaries, and auditability.

These are regression and demonstration metrics, not open-domain model-quality claims.

## Resume-Safe Wording

> Extracted a Python FastAPI + LangGraph Agent Runtime for selected workflow presets while keeping Go as the business source of truth for auth, data, tool authorization, approvals, audit, and writes; supported Workflow DAG + bounded ReAct role loops, versioned prompts, retrieval/rerank trace, citation grounding, approval-only write proposals, and deterministic task-level eval.
