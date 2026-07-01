# Resume Bullets

Use these as raw material. Pick 2-3 bullets and tune them for the target role.

## Backend / Realtime

- Built an AI-powered realtime collaboration backend in Go with Gin, Gorm/MySQL, Redis, WebSocket replay, WebRTC room signaling, and S3-compatible recording storage.
- Designed organization-scoped collaboration threads with replayable realtime events, explicit event sequence numbers, room state patches, recording lifecycle events, and conversation timeline recovery.
- Added recording-end meeting transcription with provider abstraction, mock ASR provider, transcription job state, transcript segment persistence, and Agent retrieval over meeting recording content.
- Delivered `message.created` outbox fan-out into per-recipient WebSocket replay records with dedup keys, so Agent-generated system messages and user messages share the same reconnect path.
- Implemented refresh session hardening with HttpOnly refresh cookies, token rotation, suspicious refresh reuse tracking, logout-all, and support-side session inspection.

## Full-Stack / Enterprise Web

- Built a primary React + Vite + TypeScript Web app for an enterprise collaboration workspace, covering authentication, organization management, conversations, meetings, recordings/transcripts, Agent Lab, approvals, and responsive desktop/mobile browser layouts.
- Added an organization admin console with overview metrics, members, invites, teams, policies, and audit tabs, backed by a Go/Gin summary API and OpenAPI-generated Web client types.
- Implemented organization-scoped admin summary caching with Redis short TTL, explicit invalidation on member/invite/team/conversation/message mutations, Prometheus-style hit/miss/latency counters, and reproducible local benchmarks.
- Added Node.js engineering gates for the Web app: OpenAPI contract drift checks, Vite bundle budget checks, lazy/manual chunk splitting, and Make targets for repeatable CI-style validation.
- Improved collaboration workspace UX with message cursor pagination, lightweight windowed rendering for long conversations, optimistic chat interactions, replies, reactions, pins, attachments, and durable WebSocket replay.
- Built the project as a full-stack enterprise management system first, with AI Agent meeting recap and approval-gated write-back as product enhancements rather than a standalone black-box Agent demo.

## AI Agent Engineering

- Implemented an asynchronous explainable AI Agent run queue with persisted pending/running/ready/failed states, outbox-triggered worker execution, intermediate steps, tool-call records, scoped memory, idempotency keys, and backend-controlled tool execution.
- Added Agent execution recovery with attempt counters and `lease_until`, allowing transient planner failures to retry and stale `running` runs to recover after worker crashes.
- Added `AGENT_PROVIDER` selection with deterministic rules, mock structured-output, and configurable OpenAI-compatible planners, preserving stable tests while supporting LLM-backed planning with fallback metrics.
- Made Python FastAPI + LangGraph the Beta/demo Agent orchestration runtime: Go keeps auth, data ownership, read-tool authorization, tool policy, approvals, audit, and final writes, while Python handles ReAct runs, workflow registry, DAG orchestration, bounded role loops, provider adapters, trace events, citations, and write-tool proposals.
- Upgraded the external Python runtime into an Agent Runtime Harness: each run records route decision, loop spec, loop budget, stop reason, critic result, evidence pack, grounding status, and approval-only tool proposals, making the Agent behavior inspectable rather than black-box.
- Split out a Python RAG Runtime microservice for Agentic retrieval planning, rerank, evidence pack construction, grounding checks, and RAG eval; production retrieval still goes through Go's organization-scoped authorized bridge instead of direct Python DB access.
- Added optional Qdrant and eval-only LlamaIndex adapters to compare retrieval strategies without bypassing Go's organization isolation and approval/audit boundary.
- Added bounded Agentic RAG in the Python LangGraph runtime: source-specific retrieval planning, max-3 read-tool retrieval loop, observation/refinement trace, evidence pack, context sufficiency gate, and approval-only write-tool proposals.
- Added an explicit RAG rerank layer over BM25/vector/RRF candidates with deterministic `rules` and configurable cross-encoder-compatible providers; Agent citations now carry original retrieval metadata, rerank score, rerank reason, and final rank for traceable ranking decisions.
- Added versioned Python prompt templates and grounding-check trace events in the LangGraph runtime, making prompt changes and citation support inspectable during Agent Lab demos.
- Added an eval-only LlamaIndex adapter for fixture-based knowledge retrieval comparison while keeping production knowledge access, organization isolation, and approval boundaries in Go.
- Built a deterministic regression harness plus black-box task eval for planner, RAG, workflow, and natural-language task paths; current fixture sets cover 2 planner cases, 40 RAG cases, 3 workflow cases, and 8 task-level cases.
- Added Python-side Agent/RAG eval runners covering task success, citation grounding, tool intent match, approval safety, prompt schema presence, grounding-check trace, Agentic RAG refinement, citation coverage, iteration compliance, unsupported-claim guarding, rerank ordering, sufficiency gating, and JD-oriented evidence reports.
- Added RAG retrieval metrics and safety-oriented evaluation outputs, including `Recall@K`, `Precision@K`, `MRR`, Top-K hit rate, negative/no-answer pass rate, citation error rate, p50/p95 latency, approval interception, and meeting-transcript grounding on the current fixture set.
- Integrated Agent tools for writing collaboration messages, creating follow-up tasks, upserting memory, and persisting outbox events for durable async delivery.
- Extended Agent context retrieval to include meeting transcript segments separately from 1:1 call transcript segments, improving source attribution for meeting summaries and follow-up reasoning.

## Reliability / Distributed Systems

- Added durable realtime event replay and explicit event sequence fields to reduce missed updates after WebSocket reconnects.
- Introduced an event outbox model and worker with idempotent enqueue semantics, request-id propagation, worker claim/lease fields, retry limits, configurable batch/interval controls, and publish/retry/failure metrics.
- Extracted a gRPC User Service boundary for internal access-token validation, enabling the connection-heavy signaling/API gateway to scale independently from user-center IO workloads.
- Added a Kafka-compatible room settlement pipeline: room-ended events are written to outbox, bridged to Kafka, and consumed by a Data Worker into idempotent `room_settlements`.
- Added an Elasticsearch-backed message search read model with async outbox indexing and post-search service-layer permission filtering.
- Propagated `X-Request-ID` through HTTP error responses, Agent runs, outbox rows, and async worker handlers for traceable backend diagnostics.
- Built recording lifecycle infrastructure with local/S3-compatible storage drivers, retention metadata, cleanup worker, download authorization, and support diagnostics.
- Decoupled meeting transcription from realtime translation, allowing recording-end transcription even when realtime translation UI is hidden.

## Performance Evidence

- Captured deterministic local Agent/outbox benchmark evidence: 25 queued runs, 25 ready runs, 0 failures, 75 processed outbox events, 175 tool calls, 75 indexed context chunks, 7.0 tool calls per run, and low-latency execute-run p95 on temporary SQLite.
- Captured enterprise dashboard benchmark evidence: organization admin summary DB path around 162 us/op, Redis cache-hit path around 71 us/op, and long conversation message page around 280 us/op on a local SQLite/miniredis functional benchmark.
- Captured live local MySQL/Redis API QPS baseline at 20 concurrency for 60 seconds: `GET messages` 35,286 requests / 587.99 QPS / p95 63 ms, `POST message` 2,878 requests / 47.71 QPS / p95 574 ms, and `POST agent/runs` create/enqueue 4,258 requests / 70.81 QPS / p95 492 ms, all with 0% error rate.
- Captured realtime replay benchmark evidence: 2000 persisted events, 100 replayed events, scoped replay correctness, monotonic IDs/sequences, and 3 ms write p95 on temporary SQLite.
- Captured authenticated WebSocket replay evidence: 5 clients, 500 total replayed events, 0 upgrade/client errors, 0 duplicates, and 9 ms connect-to-last p95 against the real `/api/v1/chat/ws` path.

## Interview Short Pitch

AllCallAll is a full-stack enterprise collaboration portfolio project that combines a React/Vite Web admin workspace, Go/Gin + MySQL/Redis backend services, WebSocket/WebRTC realtime collaboration, recording/transcription, reproducible benchmarks, and an explainable AI Agent pipeline. For full-stack roles, lead with the enterprise management workflow and engineering quality; for AI roles, emphasize Agent orchestration, RAG, citations, and approval boundaries.
