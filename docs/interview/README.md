# AllCallAll Interview Positioning

AllCallAll is now positioned as an AI-powered realtime collaboration backend project for backend engineering interviews. The commercial surface still exists, but the interview story should focus on distributed backend design, realtime systems, production reliability, and an explainable AI Agent workflow.

## Why This Project Fits Backend Roles

- Realtime systems: WebSocket event replay, room state patching, WebRTC signaling, and recording lifecycle events.
- Data modeling: organizations, conversations, rooms, recordings, usage ledgers, refresh sessions, and Agent execution records.
- Reliability: request IDs, metrics, cleanup workers, S3-compatible recording storage, and idempotent webhook/session handling.
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

1. Show the backend module boundaries: `auth`, `collaboration`, `agent`, `commerce`, `storage`, `signaling`.
2. Walk through `POST /api/v1/agent/runs`: auth claims, organization header, membership guard, run creation, steps, tool call, metrics.
3. Show how realtime collaboration data feeds the Agent: conversation messages, internal notes, priority, assignee, and status.
4. Explain why v1 is rules-based: stable tests, deterministic demos, no API-key dependency, and an easy seam for OpenAI-compatible providers later.
5. Show idempotency: repeat a run with the same `Idempotency-Key` and explain why tool side effects do not duplicate.
6. Open `/api/v1/metrics` and point to Agent counters such as `agent_run_total`, `agent_run_failed_total`, `agent_tool_call_total`, and `agent_memory_write_total`.

## Resume Bullet Candidates

- Built an organization-scoped realtime collaboration backend in Go with Gin, Gorm, Redis, WebSocket replay, room-state patch events, and S3-compatible recording storage.
- Designed an explainable AI Agent execution model with persisted runs, intermediate steps, tool-call records, permission checks, metrics, and conversation write-back.
- Implemented production-oriented auth/session hardening with refresh-token rotation, reuse detection, logout-all, and support-side session inspection.
- Added recording lifecycle management with storage abstraction, retention cleanup worker, signed/proxy downloads, organization boundary checks, and support diagnostics.

## What To Improve Next For Interviews

- Add an LLM provider interface behind the current rules-based Agent service.
- Add a small benchmark/load test for conversation event replay and Agent run creation.
- Add architecture diagrams for room state sync and Agent execution.
- Create a scripted demo seed command that creates an organization, conversation, notes, messages, and one Agent run.
