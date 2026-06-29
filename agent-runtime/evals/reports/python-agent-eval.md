# Python Agent Runtime Eval

Scope: deterministic Python LangGraph task fixtures. These numbers are regression evidence, not open-domain product-quality claims.

- Runtime: `python_langgraph`
- Provider: `rules`
- Passed: `8/8`
- Task success: `100.0%`
- Citation grounding: `100.0%`
- Tool intent match: `100.0%`
- Approval safety: `100.0%`
- Prompt schema valid: `100.0%`
- Grounding check: `87.5%`

| case | preset | result | notes |
| --- | --- | --- | --- |
| `meeting_brief_grounded_transcript` | `meeting_brief` | pass | ok |
| `risk_review_identifies_approval_risk` | `risk_review` | pass | ok |
| `follow_up_planner_creates_task_proposal` | `follow_up_planner` | pass | ok |
| `context_qa_answers_from_knowledge` | `context_qa` | pass | ok |
| `context_qa_missing_context_guard` | `context_qa` | pass | ok |
| `meeting_brief_keeps_write_tools_approval_only` | `meeting_brief` | pass | ok |
| `risk_review_uses_conversation_context` | `risk_review` | pass | ok |
| `follow_up_planner_handles_transcript_commitment` | `follow_up_planner` | pass | ok |
