# AllCallAll Interview Positioning

AllCallAll is positioned as an AI-powered realtime collaboration backend project for backend engineering interviews. Treat product, legal, and billing surfaces as supporting domain data. The interview story should stay focused on distributed backend design, realtime systems, reliability, and an explainable AI Agent workflow.

## Why This Project Fits Backend Roles

- Realtime systems: WebSocket event replay, room state patching, WebRTC signaling, and recording lifecycle events.
- Data modeling: organizations, conversations, rooms, recordings, recording transcription jobs, transcript segments, refresh sessions, event logs, outbox events, and Agent execution records.
- Reliability: request IDs propagated through HTTP, Agent runs, and outbox workers; metrics; cleanup workers; S3-compatible recording storage; idempotent webhook/session handling; and an outbox worker.
- Security: organization-scoped access control, refresh session rotation, support-token protected internal APIs, and no raw media persistence by default.
- AI Agent readiness: deterministic rules-based Agent v1 with run state, steps, tool calls, memory, idempotency, outbox, conversation write-back, and meeting transcript retrieval.

## Document Map

- [System Design](system-design.md): system-design interview view of the whole backend.
- [Backend Deep Dive](backend-deep-dive.md): Go, transactions, realtime, auth, storage, and reliability talking points.
- [AI Agent Design](ai-agent-design.md): Agent state machine, provider seam, tool calling, memory, guardrails.
- [Python Agent Runtime](python-agent-runtime.md): LangGraph runtime split, Go/Python boundary, tool bridge, and Python eval.
- [AI Agent JD Fit](ai-portfolio-jd-fit.md): JD capability mapping for LangGraph, LangChain, Rerank, LlamaIndex, prompt engineering, and eval.
- [Microservice Evolution](microservice-evolution.md): modular monolith to microservice-ready worker migration path.
- [gRPC, Kafka, and Elasticsearch Evolution](grpc-kafka-es-evolution.md): synchronous service split, async settlement pipeline, and message search index.
- [Worker Runtime](worker-runtime.md): API-embedded workers, standalone worker commands, event ownership, and failure semantics.
- [Demo Script](demo-script.md): 5-minute interview demo flow and live backend variant.
- [Agent Demo Eval Report](agent-demo-report.md): one-command planner, RAG, and workflow eval report.
- [Resume Eval](resume-eval.md): resume-ready KPI summary and deterministic evidence path.
- [Agent UX Eval](agent-ux-eval.md): black-box task eval and manual pilot UX rubric.
- [Agent Task Eval Cases](agent-task-eval-cases.md): recommended black-box task set, scoring dimensions, and interview phrasing.
- [Agent Trace Example](agent-trace-example.md): run/step/tool timeline and tool registry explanation.
- [API Surface](api-surface.md): APIs worth demoing in interviews.
- [Performance Report](performance-report.md): load-test template and metrics checklist.
- [Load Test Results](load-test-results.md): latest local suite results and live MySQL/Redis checklist.
- [Troubleshooting](troubleshooting.md): Agent, outbox, WebSocket replay, recording, and CI debugging.
- [Resume Bullets](resume-bullets.md): polished bullets for resumes and interviews.

## Suggested Interview Demo Path

1. Show the backend module boundaries: `auth`, `collaboration`, `agent`, `events`, `storage`, and `signaling`. Mention `commerce` only as supporting domain surface, not the main portfolio story.
2. Walk through `POST /api/v1/agent/runs`: auth claims, organization header, membership guard, pending run creation, `agent.run.requested` outbox enqueue, worker execution, steps, tool calls, and metrics.
3. Show how collaboration and meeting data feed the Agent: messages, internal notes, priority, assignee, room state, call follow-ups, and meeting recording transcript segments.
4. Explain the microservice evolution path: API/signaling gateway calls User Service through gRPC when `USER_SERVICE_GRPC_ADDR` is configured.
5. Explain async peak shaving: room-ended settlement events are written to outbox, bridged to Kafka, then consumed by `data-worker` into `room_settlements`.
6. Explain search scaling: message writes enqueue `search.message.index_requested`, `search-worker` indexes ES, and `/search/messages` re-applies conversation membership checks.
7. Explain why v1 Agent is rules-based by default: stable tests, deterministic demos, no API-key dependency, and a seam for mock/OpenAI-compatible providers.
8. Show idempotency: repeat a run with the same `Idempotency-Key` and explain why tool side effects do not duplicate.
9. Show observability: send `X-Request-ID`, trigger an Agent run, and explain how the same ID is saved on `agent_runs` and `event_outbox`.
10. Show realtime replay: connect to `/api/v1/chat/ws?since_id=...` and point out `event_id`, `sequence`, and durable MySQL-backed replay.
11. Open `/api/v1/metrics` and point to Agent and outbox counters such as `agent_run_queued_total`, `agent_run_started_total`, `agent_run_total`, `agent_run_failed_total`, `agent_tool_call_total`, `agent_memory_write_total`, `outbox_publish_total`, and `outbox_publish_retry_total`.
12. If recording transcription is enabled, stop a meeting recording and show `recording.transcription.requested`, `recording_transcriptions`, `meeting_transcript_segments`, and the Agent's ability to retrieve meeting transcript context.

## One-Command Demo

For a deterministic demo that does not require MySQL, Redis, JWTs, or external model credentials:

```bash
make interview-demo
```

For a live backend seed path that starts MySQL/Redis and writes demo data:

```bash
make interview-demo-live
```

See [Demo Script](demo-script.md) for the full walkthrough.

## Demo Seed Command

After starting MySQL and configuring `CONFIG_PATH`, generate a deterministic interview demo dataset:

```bash
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/interview-seed
```

Optional provider selection:

```bash
CONFIG_PATH=./configs/config.yaml AGENT_PROVIDER=rules go run ./cmd/interview-seed
CONFIG_PATH=./configs/config.yaml AGENT_PROVIDER=mock_llm go run ./cmd/interview-seed
```

The command prints organization, conversation, room, and Agent run IDs. It creates users, an organization, a conversation, notes/messages, a meeting record, contact profile, and one idempotent Agent run with the stable key `interview-seed-agent-run`.

`AGENT_PROVIDER=rules` is the default and the safest interview demo mode. `AGENT_PROVIDER=mock_llm` demonstrates prompt construction and structured-output parsing without external credentials. `AGENT_PROVIDER=openai_compatible` calls a configured Chat Completions-compatible endpoint when `AGENT_OPENAI_BASE_URL` and `AGENT_OPENAI_MODEL` are set; otherwise service execution falls back to `rules` and records fallback metrics.

## Local Benchmark Command

For a database-free interview demo, run the Agent + outbox pipeline against a temporary SQLite database:

```bash
cd backend
go run ./cmd/interview-bench -conversations 25 -batch-size 50
```

The command seeds conversations, queues Agent runs, drains `agent.run.requested` through the outbox processor, executes tool calls, writes conversation messages/tasks/memory, and prints JSON with ready/failed run counts, processed outbox events, latency summaries, and metric counters. Use `-provider=mock_llm` to show prompt construction and structured-output parsing; use `-provider=openai_compatible` to show the unavailable-provider fallback path.

Run deterministic Agent evals directly:

```bash
make agent-eval
make rag-eval
make workflow-eval
make task-eval
make rerank-eval
make agent-demo-report
make resume-eval
make ai-portfolio-eval
```

Run the modular monolith plus standalone worker demo:

```bash
make interview-microservice-demo
```

Run extracted gRPC/Kafka/ES services locally:

```bash
make run-user-service
make run-outbox-worker
make run-data-worker
make run-search-worker
```

Generate a local load-suite report:

```bash
make interview-load-suite
```

## Resume Bullet Candidates

- Built an organization-scoped realtime collaboration backend in Go with Gin, Gorm, Redis, WebSocket replay, room-state patch events, S3-compatible recording storage, and recording-end meeting transcription.
- Designed an explainable AI Agent execution model with persisted runs, intermediate steps, tool-call records, permission checks, metrics, and conversation write-back.
- Added a gRPC User Service boundary for request-time auth validation, allowing the signaling/API gateway to scale separately from user-center IO workloads.
- Added Kafka-compatible room settlement events and a Data Worker with idempotent consumption to demonstrate async peak shaving for meeting end storms.
- Added Elasticsearch-backed message search with async outbox indexing and service-layer membership filtering to avoid MySQL wildcard scans.
- Implemented production-oriented auth/session hardening with refresh-token rotation, reuse detection, logout-all, and support-side session inspection.
- Added recording lifecycle management with storage abstraction, retention cleanup worker, signed/proxy downloads, transcription job tracking, meeting transcript segments, organization boundary checks, and support diagnostics.

## What To Improve Next For Interviews

- Add live LLM token streaming behind the existing OpenAI-compatible planner if a demo needs model-output streaming in addition to backend tool-event streaming.
- Extend benchmark/load tests to authenticated WebSocket replay and meeting room event throughput.
- Replace the current observed outbox handler with production publishers when the deployment target is clear.
- Capture measured baseline numbers in [Performance Report](performance-report.md), reproducible KPI summaries in [Resume Eval](resume-eval.md), and user-facing scoring notes in [Agent UX Eval](agent-ux-eval.md).
