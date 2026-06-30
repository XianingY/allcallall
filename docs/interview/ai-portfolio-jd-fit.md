# AI Agent JD Fit

This page maps the current AllCallAll implementation to AI application / AI Agent internship requirements. It is intended for interview preparation and resume tuning.

## Capability Mapping

| JD capability | Project evidence | Boundary |
| --- | --- | --- |
| AI application design and iteration | Meeting recap flow from conversation/meeting transcript context to Agent output, citations, approval proposals, and write-back. | Beta product path is small-team collaboration, not public SaaS scale. |
| Intelligent Agent design | Python FastAPI + LangGraph is the Beta/demo Agent orchestration runtime for ReAct plus `meeting_brief`, `risk_review`, `follow_up_planner`, and `context_qa`; Go legacy runtime remains fallback/regression baseline. | Go remains source of truth for auth, data, approvals, audit, and writes. |
| Prompt engineering | Versioned Python prompt templates with `PY_AGENT_PROMPT_VERSION`; structured JSON output for summary/action/risk fields. | Current prompts are deterministic and compact; not a large prompt-optimization study. |
| Knowledge base system | Text/URL/file knowledge ingestion, chunking, dedupe/versioning, conversation-scoped retrieval, citation metadata. | Real production content volume is not claimed. |
| Embedding and vector DB | Elasticsearch dense-vector retrieval, BM25 retrieval, hybrid RRF fusion, SQL fallback for deterministic local paths. | Eval fixtures are local and deterministic. |
| Hybrid retrieval and rerank | Explicit Go `Reranker` abstraction plus Python RAG Runtime for Agentic retrieval, rerank, evidence pack, and grounding checks. | `rules` is used for reproducible evidence; cross-encoder HTTP provider is configurable but not required for CI. |
| Agent framework usage | LangGraph DAG controls workflow; bounded ReAct role loops run inside `searcher` and `risk_analyst`; LangChain Core handles prompt templates/provider adapter. | CrewAI/AutoGen are not in the main path because the product needs strict tool/approval boundaries. |
| LlamaIndex exploration | Eval-only `llamaindex_adapter.py` can compare fixture retrieval against a LlamaIndex baseline. | It does not bypass Go-owned organization isolation or production knowledge models. |
| Eval mindset | Planner/RAG/workflow deterministic regression, rerank quality eval, Python LangGraph task eval, black-box UX task eval. | These are fixture/regression metrics, not open-domain model quality claims. |

## Current Demo Commands

```bash
make agent-demo-report
make rerank-eval
make python-agent-eval
make python-rag-eval
make ai-agent-jd-eval
make ai-portfolio-eval
```

`make rerank-eval` writes:

- `docs/interview/generated-rerank-eval/rerank-eval.json`
- `docs/interview/generated-rerank-eval/rerank-eval.md`

`make ai-portfolio-eval` writes:

- `docs/interview/generated-ai-portfolio-eval/ai-portfolio-eval.json`
- `docs/interview/generated-ai-portfolio-eval/ai-portfolio-eval.md`

`make ai-agent-jd-eval` writes:

- `docs/interview/generated-ai-agent-jd-eval/ai-agent-jd-eval.json`
- `docs/interview/generated-ai-agent-jd-eval/ai-agent-jd-eval.md`

## Interview Framing

Use this framing:

> I did not move all business logic into Python. Go owns product data, permissions, approval, audit, and writes. Python is the AI orchestration layer: `agent-runtime` runs LangGraph/ReAct/prompt/provider logic, and `rag-runtime` runs Agentic retrieval, rerank, evidence pack, grounding, and eval. This gives the project a clear AI full-stack boundary instead of a fragile all-in-one Agent service.

For rerank:

> Retrieval first collects BM25/vector/RRF candidates. A separate rerank layer scores candidate chunks using either deterministic rules or a cross-encoder-compatible HTTP provider. The Agent citation output carries retrieval mode, original ranking metadata, rerank score, rerank reason, and final rank, so the ranking decision is inspectable in Agent Lab.

For eval:

> I avoid presenting tiny fixture pass rates as overall model quality. The project separates deterministic regression, retrieval/rerank quality, and black-box task completion. That is more honest for a student project without a real production RAG dataset.

## Resume-Safe Bullet

- Built an AI Agent collaboration system with Go-owned business/security boundaries, a Python FastAPI + LangGraph Agent Runtime, and a separate Python RAG Runtime; implemented hybrid RAG with BM25/vector/RRF, explicit rerank metadata, versioned prompts, citation grounding, approval-only write proposals, and deterministic planner/RAG/workflow/task eval reports.

## Why Not CrewAI / AutoGen In Main Path

CrewAI and AutoGen are useful for multi-agent experiments, but the current product needs deterministic workflow control, tool authorization, and human approval gates. LangGraph is a better fit because the graph structure makes each node, ReAct loop, citation, and write proposal easier to persist and explain.
