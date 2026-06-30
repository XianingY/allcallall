# Python RAG Runtime Eval

Deterministic fixture eval for rerank ordering, agentic retrieval, grounding, and insufficient-context handling.

## Summary

- Cases: 3/3
- Rerank top-match rate: 100%
- Grounding pass rate: 100%
- Sufficiency pass rate: 100%
- Retrieval refinement success rate: 100%

## Cases

- `PASS` meeting recap uses transcript evidence: top_source_type=meeting_transcript
- `PASS` knowledge policy outranks distractor message: top_source_type=knowledge
- `PASS` insufficient context is conservative: top_source_type=message
