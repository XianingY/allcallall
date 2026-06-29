# AllCallAll AI Portfolio Eval

This report groups the evidence useful for AI Agent / AI application interviews. It separates deterministic regression, retrieval/rerank quality, and black-box task completion so the numbers are not overstated.

## Evidence Layers

| Layer | Result | Scope |
| --- | --- | --- |
| Deterministic regression | Planner 2/2, RAG 40/40, Workflow 3/3 | Current fixture set |
| Retrieval + rerank | MRR 1.000, NDCG@K 1.000, MRR delta 0.583 | Hybrid RAG fixture set with rules reranker |
| Python Agent Runtime | See bundled Python report when present | LangGraph task-level eval |

## Rerank Details

- `rerank_promotes_title_grounded_policy`: pass, MRR 0.333 -> 1.000, NDCG@K 0.500 -> 1.000
- `rerank_prefers_actionable_followup_evidence`: pass, MRR 0.500 -> 1.000, NDCG@K 0.631 -> 1.000

## Python Runtime Report Presence

- Python eval report: loaded
