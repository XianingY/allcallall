# AllCallAll RAG Rerank Eval

This report compares the deterministic baseline retrieval order with the rules rerank order on the current fixture set. It is a regression and ranking-quality check, not a production user-satisfaction benchmark.

## Summary

| Metric | Value |
| --- | ---: |
| Cases | 2 |
| Passed | 2 |
| Recall@K | 1.000 |
| Precision@K | 0.417 |
| MRR | 1.000 |
| NDCG@K | 1.000 |
| Rerank MRR delta | 0.583 |
| Rerank NDCG delta | 0.435 |

## Cases

### rerank_promotes_title_grounded_policy

- Status: `pass`
- MRR: 0.333 -> 1.000 (delta 0.667)
- NDCG@K: 0.500 -> 1.000 (delta 0.500)

| Rank | Source | Retrieval | Rerank score | Reason |
| ---: | --- | --- | ---: | --- |
| 1 | Security Approval Blocker | sql_fallback | 58.346 | rules keyword_overlap=3 title_overlap=3 source=knowledge retrieval=sql_fallback |
| 2 | Generic Security Log | sql_fallback | 43.030 | rules keyword_overlap=3 title_overlap=1 source=knowledge retrieval=sql_fallback |
| 3 | Budget Note | sql_fallback | 34.934 | rules keyword_overlap=3 title_overlap=0 source=knowledge retrieval=sql_fallback |

### rerank_prefers_actionable_followup_evidence

- Status: `pass`
- MRR: 0.500 -> 1.000 (delta 0.500)
- NDCG@K: 0.631 -> 1.000 (delta 0.369)

| Rank | Source | Retrieval | Rerank score | Reason |
| ---: | --- | --- | ---: | --- |
| 1 | Customer Follow Up Owner Deadline | sql_fallback | 94.526 | rules keyword_overlap=5 title_overlap=5 source=knowledge retrieval=sql_fallback |
| 2 | Customer Chatter | sql_fallback | 43.030 | rules keyword_overlap=3 title_overlap=1 source=knowledge retrieval=sql_fallback |

