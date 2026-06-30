# AI Agent JD Eval Bundle

Regression/eval evidence for current fixtures; not an open-domain production quality claim.

## Reproducible Metrics

### Python Agent Runtime

- `approval_safety_rate`: `1.000`
- `citation_coverage_rate`: `1.000`
- `citation_grounding_rate`: `1.000`
- `grounding_check_rate`: `0.889`
- `max_iteration_compliance_rate`: `1.000`
- `passed_cases`: `9`
- `prompt_schema_valid_rate`: `1.000`
- `retrieval_refinement_success_rate`: `1.000`
- `task_success_rate`: `1.000`
- `tool_intent_match_rate`: `1.000`
- `total_cases`: `9`
- `unnecessary_tool_call_rate`: `1.000`
- `unsupported_claim_guard_rate`: `1.000`

### Python RAG Runtime

- `grounding_pass_rate`: `1.000`
- `passed_cases`: `3`
- `rerank_top_match_rate`: `1.000`
- `retrieval_refinement_success_rate`: `1.000`
- `sufficiency_pass_rate`: `1.000`
- `total_cases`: `3`

## JD Mapping

| JD requirement | Project evidence |
| --- | --- |
| Agent framework usage and secondary development | Python FastAPI Agent Runtime uses LangGraph workflow nodes, bounded ReAct role loops, prompt registry, trace events, and tool proposals. |
| LLM calling and prompt engineering | OpenAI-compatible provider adapter supports structured JSON output; deterministic rules provider keeps eval reproducible. |
| Knowledge base, embedding, hybrid retrieval, rerank | Go owns authorized business retrieval; Python RAG Runtime adds agentic retrieval orchestration, rerank, grounding check, and fixture eval. |
| AI product landing in business scenarios | Meeting recap, risk review, follow-up planning, and context QA all run against collaboration context with citation and approval boundaries. |
