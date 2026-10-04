# Agent Platform Architecture

AllCallAll separates product authority from Agent intelligence. Go controls
data access, permissions, approvals, audit, and writes; Python controls
orchestration, retrieval strategy, grounding, citations, and evaluation.

## Execution Paths

The product exposes two Agent styles:

- ReAct-style runs for conversation summaries, risks, actions, and follow-up
  proposals.
- Workflow runs for preset DAGs, role tasks, approval gates, timers, and
  resumable execution.

The backend creates idempotent run records and dispatches work through the
outbox. Results and trace events are persisted so polling and streaming clients
observe the same durable state.

## Context and Retrieval

Authorized context can include conversation metadata, recent messages, notes,
members, rooms, memories, contact profiles, follow-ups, call transcript
segments, meeting transcript segments, and knowledge chunks. Citations retain
source identity so clients can distinguish meeting recordings, calls,
conversation content, and external knowledge.

The optional Python RAG Runtime performs bounded source planning, retrieval
refinement, reranking, evidence selection, and sufficiency checks above
Go-authorized retrieval adapters.

## Tool Safety

Read-only tools may execute automatically within their authorized scope.
Mutating tools are proposals only: the Go backend validates the tool schema and
permissions, creates an approval record, audits the decision, and executes the
write after approval. Replaying an idempotent completed run must not repeat side
effects.

## Providers and Failure Behavior

Deterministic providers support local development and repeatable evaluation.
OpenAI-compatible providers are optional. Strict mode keeps configuration and
provider failures visible instead of silently presenting fallback output as a
model result.

Recording transcription is independent from realtime translation. Retrieval
or model failures do not transfer product-write authority to Python.

## Further Reading

- [System Overview](system-overview.md)
- [Workers and Processes](workers.md)
- [Python runtime documentation](https://github.com/XianingY/allcallall-agent-runtime/blob/main/docs/README.md)
- [Message privacy and compliance](../reference/security/privacy-and-compliance.md)
