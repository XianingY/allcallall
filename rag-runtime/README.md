# AllCallAll RAG Runtime

Python FastAPI runtime for Agent retrieval orchestration, rerank, grounding checks, and deterministic RAG eval.

The service does not directly access AllCallAll business databases. In production it calls the Go backend internal retrieval bridge with `PY_RAG_TOOL_BRIDGE_BASE_URL` and `PY_RAG_TOOL_BRIDGE_TOKEN`; in eval/tests it can operate on inline fixture chunks.

## Run Locally

```bash
cd rag-runtime
python -m venv .venv
. .venv/bin/activate
pip install -e ".[dev]"
uvicorn app.main:app --reload --port 8091
```

## API

- `GET /health`
- `POST /v1/retrieval/query`
- `POST /v1/retrieval/rerank`
- `POST /v1/retrieval/agentic`
- `POST /v1/grounding/check`

## Eval

```bash
python -m app.eval_runner --out evals/reports
```

The eval is deterministic fixture evidence for retrieval refinement, rerank ordering, grounding, and insufficient-context behavior. It is not an open-domain RAG quality claim.
