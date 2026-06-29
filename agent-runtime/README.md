# AllCallAll Agent Runtime

Python FastAPI + LangGraph runtime for AllCallAll Agent workflows.

The Go backend remains the source of truth for users, organizations, conversations, transcript data, tool permissions, approvals, audit logs, and write execution. This service owns AI orchestration: workflow registry, LangGraph DAG execution, bounded ReAct role loops, LLM/provider adapters, structured traces, citations, write-tool proposals, and Python-side task eval.

Supported presets:

- `meeting_brief`
- `risk_review`
- `follow_up_planner` (`follow_up` is accepted by the Go adapter as an alias)
- `context_qa`

## Run Locally

```bash
cd agent-runtime
python -m venv .venv
. .venv/bin/activate
pip install -e ".[dev]"
uvicorn app.main:app --reload --port 8090
```

Use it from the Go backend with:

```bash
AGENT_RUNTIME=python_langgraph
PY_AGENT_RUNTIME_BASE_URL=http://127.0.0.1:8090
PY_AGENT_RUNTIME_STRICT=true
```

## Configuration

- `PY_AGENT_PROVIDER=rules|openai_compatible`: deterministic local provider or real OpenAI-compatible chat provider.
- `PY_AGENT_OPENAI_BASE_URL`, `PY_AGENT_OPENAI_API_KEY`, `PY_AGENT_OPENAI_MODEL`: OpenAI-compatible `/chat/completions` configuration.
- `PY_AGENT_PROVIDER_STRICT=true`: when using `openai_compatible`, missing config or provider errors return a failed workflow response instead of silently falling back.
- `PY_AGENT_TOOL_BRIDGE_BASE_URL`: Go backend base URL for read-only tool execution, for example `http://backend:8080`.
- `PY_AGENT_TOOL_BRIDGE_TOKEN`: shared bearer token matching Go `AGENT_RUNTIME_TOOL_TOKEN`.
- `PY_AGENT_ENABLE_AGENTIC_RAG=false`: enable bounded Agentic RAG retrieval planning and refinement.
- `PY_AGENT_RAG_MAX_RETRIEVAL_STEPS=3`: hard cap for Agentic RAG read-tool retrieval attempts.
- `PY_AGENT_RAG_MIN_CONFIDENCE=0.6`: confidence threshold for stopping retrieval refinement.

If the tool bridge is not configured, the runtime still uses context preloaded by Go. This keeps deterministic local evals independent from a running backend.

## Eval

```bash
cd agent-runtime
python -m app.eval_runner --out evals/reports
```

Outputs:

- `evals/reports/python-agent-eval.json`
- `evals/reports/python-agent-eval.md`

The eval fixtures are deterministic regression cases for task completion, citation grounding, tool intent, approval safety, Agentic RAG refinement, citation coverage, iteration caps, and unsupported-claim guarding. They are not open-domain model-quality claims.
