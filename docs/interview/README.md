# AllCallAll Interview Positioning

AllCallAll is positioned as an AI-powered realtime collaboration backend project for backend engineering interviews. Treat product, legal, and billing surfaces as supporting domain data. The interview story should stay focused on distributed backend design, realtime systems, reliability, and an explainable AI Agent workflow.

## Why This Project Fits Backend Roles

- Realtime systems: WebSocket event replay, room state patching, WebRTC signaling, and recording lifecycle events.
- Data modeling: organizations, conversations, rooms, recordings, refresh sessions, event logs, outbox events, and Agent execution records.
- Reliability: request IDs, metrics, cleanup workers, S3-compatible recording storage, idempotent webhook/session handling, and an outbox worker.
- Security: organization-scoped access control, refresh session rotation, support-token protected internal APIs, and no raw media persistence by default.
- AI Agent readiness: deterministic rules-based Agent v1 with run state, steps, tool calls, memory, idempotency, outbox, and conversation write-back.

## Document Map

- [System Design](system-design.md): system-design interview view of the whole backend.
- [Backend Deep Dive](backend-deep-dive.md): Go, transactions, realtime, auth, storage, and reliability talking points.
- [AI Agent Design](ai-agent-design.md): Agent state machine, provider seam, tool calling, memory, guardrails.
- [API Surface](api-surface.md): APIs worth demoing in interviews.
- [Performance Report](performance-report.md): load-test template and metrics checklist.
- [Resume Bullets](resume-bullets.md): polished bullets for resumes and interviews.

## Suggested Interview Demo Path

1. Show the backend module boundaries: `auth`, `collaboration`, `agent`, `events`, `storage`, and `signaling`. Mention `commerce` only as supporting domain surface, not the main portfolio story.
2. Walk through `POST /api/v1/agent/runs`: auth claims, organization header, membership guard, pending run creation, `agent.run.requested` outbox enqueue, worker execution, steps, tool calls, and metrics.
3. Show how realtime collaboration data feeds the Agent: conversation messages, internal notes, priority, assignee, and status.
4. Explain why v1 is rules-based: stable tests, deterministic demos, no API-key dependency, and an easy seam for OpenAI-compatible providers later.
5. Show idempotency: repeat a run with the same `Idempotency-Key` and explain why tool side effects do not duplicate.
6. Show realtime replay: connect to `/api/v1/chat/ws?since_id=...` and point out `event_id`, `sequence`, and durable MySQL-backed replay.
7. Open `/api/v1/metrics` and point to Agent and outbox counters such as `agent_run_queued_total`, `agent_run_started_total`, `agent_run_total`, `agent_run_failed_total`, `agent_tool_call_total`, `agent_memory_write_total`, `outbox_publish_total`, and `outbox_publish_retry_total`.

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

## Resume Bullet Candidates

- Built an organization-scoped realtime collaboration backend in Go with Gin, Gorm, Redis, WebSocket replay, room-state patch events, and S3-compatible recording storage.
- Designed an explainable AI Agent execution model with persisted runs, intermediate steps, tool-call records, permission checks, metrics, and conversation write-back.
- Implemented production-oriented auth/session hardening with refresh-token rotation, reuse detection, logout-all, and support-side session inspection.
- Added recording lifecycle management with storage abstraction, retention cleanup worker, signed/proxy downloads, organization boundary checks, and support diagnostics.

## What To Improve Next For Interviews

- Add streaming/tool-call support behind the existing OpenAI-compatible planner if an interview demo needs live model traces.
- Extend benchmark/load tests to authenticated WebSocket replay and meeting room event throughput.
- Replace the current observed outbox handler with production publishers when the deployment target is clear.
- Capture measured baseline numbers in [Performance Report](performance-report.md).
