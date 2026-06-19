# AllCallAll Agent Demo Eval Report

- Generated at: `2026-06-19T12:34:50Z`
- Planner provider: `rules`
- Overall status: `pass`

## Summary

| Suite | Cases | Passed | Failed |
| --- | ---: | ---: | ---: |
| Planner | 2 | 2 | 0 |
| RAG | 2 | 2 | 0 |
| Workflow | 3 | 3 | 0 |

## Planner Cases

- `high_priority_handoff`: pass - estimated tokens 371
- `thin_unassigned_context`: pass - estimated tokens 304

## RAG Cases

- `vector_hit_security_budget`: pass - mode `vector`, hits 2
  - `Security rollout note` via `vector`: Security approval is the main risk. The customer needs a data retention answer before the budget window closes.
  - `Training plan` via `vector`: Training should cover agent handoff, translation quality review, and support escalation.
- `sql_fallback_pricing_latency`: pass - mode `sql_fallback`, hits 1
  - `Pricing latency note` via `sql_fallback`: Pricing depends on the pilot seat count. Latency and translation quality are the key acceptance criteria.

## Workflow Cases

- `approval_parallel_merge_tool_commit`: pass - status `ready`, tasks 9, approvals 3
- `tool_policy_denies_write`: pass - status `failed`, tasks 9, approvals 0
- `meeting_brief_bounded_react_transcript`: pass - status `ready`, tasks 9, approvals 3
