# AllCallAll Agent Runtime

Python FastAPI + LangGraph runtime for AllCallAll Agent workflows.

The Go backend remains the source of truth for users, organizations, conversations, transcript data, tool permissions, approvals, audit logs, and write execution. This service owns AI orchestration: workflow registry, LangGraph DAG execution, bounded ReAct role loops, LLM/provider adapters, structured traces, citations, write-tool proposals, and Python-side task eval.

## Ownership and Durable Resume

Go and Python deliberately own different state:

- Go/MySQL owns product and authorization state, approval decisions, audit records, tool execution, and business side effects.
- Python/LangGraph owns graph-node progress and checkpoint payloads. Go coordinates it through versioned run and resume requests; neither side rewrites the other side's state.

Every run request has an explicit `execution_id` and exactly one scope, `agent_run_id` or `workflow_run_id`. The Go backend stores the normalized initial runtime request without its short-lived tool capability before dispatch. A retry reuses that immutable request and `execution_id`, even if the conversation changes after the first attempt. Python returns the previously committed result for the same execution and rejects a different payload or a stale checkpoint version with `409`. Thread IDs are always `agent:{run_id}` or `workflow:{run_id}`; there is no shared default thread.

Write-tool proposals use a real LangGraph `interrupt()`. Python checkpoints the deterministic `approval_request_id`, exact tool call IDs, arguments, and argument digests, then returns `requires_action`. Go persists that approval set and any resolved MCP installation, revision, and tool IDs. The approval HTTP endpoint only records the decision and enqueues work. The Agent worker then sends all decisions to the matching resume endpoint:

- `POST /v1/agents/react/resume`
- `POST /v1/workflows/{preset}/resume`

Python validates the run scope, approval request, complete decision set, and expected checkpoint version before resuming with `Command(resume=...)`. Only after Python advances the checkpoint does the Go worker execute approved tools. Local business writes and tool-call completion share a database transaction. MCP execution is bound to the installation revision and tool that were resolved before approval; a revision mismatch fails closed.

Runtime ownership is fixed when the run is created. Only runs whose immutable `runtime_owner` is `legacy_go` may use the legacy engine. A `python_langgraph` run must stay on Python through retries and resume, and fails closed when that runtime is unavailable.

### Compatibility and rollback

Checkpoints created by the earlier simulated-approval graph do not contain a LangGraph interrupt and cannot be resumed by this runtime. Before upgrading, finish or cancel those paused runs; restart necessary work as a new run after the deployment. Do not route them through the legacy runtime.

For an application rollback, keep the additive version 3 database schema and checkpoint rows, and roll back the Go and Python images together. Runs already acquired by the new Python graph must remain pinned to the compatible runtime until they finish or are explicitly canceled. The migration `up -> down -> up` check exists to prove isolated-environment reversibility; production rollback should not destructively remove checkpoint or approval metadata.

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
- `PY_AGENT_ENABLE_GROUNDING_CHECK=true`: enable citation grounding; Chinese claims use the pinned jieba tokenizer from `allcallall-shared`.

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

## Checkpoint Contract Tests

The MySQL-backed recovery tests require an isolated database:

```bash
cd agent-runtime
PY_AGENT_TEST_MYSQL_DSN='mysql://user:password@127.0.0.1:3306/allcallall_contract' \
  python -m pytest -q tests/test_mysql_checkpoint.py
PY_AGENT_TEST_MYSQL_DSN='mysql://user:password@127.0.0.1:3306/allcallall_contract' \
  python -m pytest -q tests/test_approval_resume.py -k mysql
```

They cover transactional checkpoint writes, restart recovery, idempotent `execution_id` replay, resume rollback on failure, checkpoint-version conflicts, and exclusion of short-lived tool capabilities from checkpoint payloads.
