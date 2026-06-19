# Agent UX Eval

This page describes the user-experience-oriented evaluation layer for AllCallAll Agent work. It sits beside deterministic regression and benchmark artifacts; it does not replace them.

## Two Evidence Layers

Use these as separate interview talking points:

- `Reproducible metrics`: planner / RAG / workflow / task-eval fixture sets and local SQLite benchmark.
- `Manual pilot UX evaluation`: a lightweight human scoring pass over realistic tasks.

The reproducible layer answers "does the engineered path still work?" The manual layer answers "would a user feel this was useful?"

## Reproducible Black-box Task Eval

Current command:

```bash
make task-eval
```

The deterministic task fixture set currently covers:

- meeting recap grounded by meeting transcripts
- sparse-context conservative response
- RAG-style note/message grounding
- workflow meeting brief
- workflow follow-up planning
- workflow risk review
- policy-denied write path
- missing-context guardrail behavior

What it checks automatically:

- `task_success_rate`
- `tool_intent_match_rate`
- `approval_safety_rate`
- `citation_presence_rate`
- `meeting_grounding_rate`

Interpret this as `current deterministic fixture set coverage`, not open-ended user satisfaction.

## Manual Pilot Sample

The table below is an **illustrative manual sample** for interview discussion.

It is **not part of reproducible benchmark artifacts** and should not be presented as measured production quality.

| Task | Task Completion | Answer Relevance | Faithfulness / Grounding | Tool Appropriateness | User Usefulness | Notes |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| Meeting recap from recording transcript | 4.5 / 5 | 4.4 / 5 | 4.6 / 5 | 4.3 / 5 | 4.4 / 5 | Strong grounding and useful action items; language can still be templated. |
| Risk extraction for hardware review | 4.2 / 5 | 4.1 / 5 | 4.3 / 5 | 4.0 / 5 | 4.0 / 5 | Good at surfacing blockers; sometimes risk labels are generic. |
| Follow-up plan after customer discussion | 4.0 / 5 | 4.1 / 5 | 3.9 / 5 | 4.2 / 5 | 4.2 / 5 | Approval boundary is clear; output can need manual polish before external send. |
| Sparse-context request | 4.3 / 5 | 4.0 / 5 | 4.5 / 5 | 3.9 / 5 | 3.8 / 5 | Conservative behavior is correct; usefulness is limited by missing context. |
| Policy-denied write action | 4.1 / 5 | 3.9 / 5 | 4.4 / 5 | 4.5 / 5 | 3.9 / 5 | Failure mode is explainable and safe, though not always satisfying to the user. |

## Suggested Interview Framing

Use wording like:

- "The deterministic eval layer is mainly for regression and safety boundaries."
- "For user-facing quality, I would look more at task completion, grounding, and whether the chosen tools match the user's intent."
- "I separated reproducible metrics from a manual pilot rubric because this project does not yet have a reliable answer judge."

Avoid wording like:

- "These manual scores are benchmark results."
- "The Agent is already optimized for general user satisfaction."
