# AllCallAll 面试材料入口

当前主演示是一个企业 Agent 工具平台：Web 发起 run，Go 通过 outbox 调度 Python
LangGraph，Python 使用 MySQL checkpoint 暂停/恢复，所有 MCP 工具再经 Go Gateway、
Sandbox Runner 和 HTTPS MCP 执行。默认 rules Provider 保证离线稳定；真实模型是显式配置项。

第一次阅读请按以下顺序：

1. [面试级主演示链路](interview-chain.md)：当前事实、架构图、权威边界、数据关系、故障矩阵和安全边界。
2. [五分钟演示脚本](demo-script.md)：现场按分钟执行的 Web + chaos 路径。
3. [腾讯面试问题与参考回答](tencent-interview-questions.md)：Go、Python、MCP、RAG、Web 和故障恢复追问。
4. [最新链路验收记录](interview-acceptance.md)：smoke/chaos 的可复查结果和边界。
5. [Resume Eval](resume-eval.md)：可重跑 fixture/benchmark 的事实边界。

实时协作、WebRTC、录音转写、gRPC/Kafka 演进仍是项目能力，但不应抢占主演示链路。

## Why This Project Fits Full-Stack / Backend Roles

- Full-stack product surface: React/Vite Web app, organization admin dashboard, generated OpenAPI client, responsive workspace, and browser meeting/transcript/Agent Lab flows.
- Realtime systems: WebSocket event replay, room state patching, WebRTC signaling, and recording lifecycle events.
- Data modeling: organizations, conversations, rooms, recordings, recording transcription jobs, transcript segments, refresh sessions, event logs, outbox events, and Agent execution records.
- Database/cache engineering: MySQL-backed business data, Redis-backed tickets/rate limits/presence/cache, admin summary cache invalidation, and local benchmark evidence.
- Engineering quality: contract check, bundle budget check, Vitest/MSW/Playwright, Go package tests, benchmarks, Docker Compose, and maintained docs.
- Reliability: request IDs propagated through HTTP, Agent runs, and outbox workers; metrics; cleanup workers; S3-compatible recording storage; idempotent webhook/session handling; and an outbox worker.
- Security: organization-scoped access control, refresh session rotation, support-token protected internal APIs, and no raw media persistence by default.
- AI Agent readiness: Python FastAPI + LangGraph Beta/demo runtime, deterministic rules provider for eval, run state, steps, tool calls, memory, idempotency, outbox, conversation write-back, meeting transcript retrieval, and approval-gated side effects.

## Document Map

- [Interview Chain](interview-chain.md): current Compose architecture, authority boundaries, approval sequence, data relationships, failure matrix, and security boundary.
- [Interview Acceptance](interview-acceptance.md): latest smoke/chaos evidence and explicit non-claims.
- [System Design](system-design.md): system-design interview view of the whole backend.
- [Backend Deep Dive](backend-deep-dive.md): Go, transactions, realtime, auth, storage, and reliability talking points.
- [AI Agent Design](ai-agent-design.md): Agent state machine, provider seam, tool calling, memory, guardrails.
- [Bounded Agentic RAG](agentic-rag.md): Agentic retrieval planning, evidence pack, sufficiency gate, tool boundary, and eval positioning.
- [Python Agent Runtime](python-agent-runtime.md): LangGraph runtime split, Go/Python boundary, tool bridge, and Python eval.
- [AI Agent JD Fit](ai-portfolio-jd-fit.md): JD capability mapping for LangGraph, LangChain, Rerank, LlamaIndex, prompt engineering, and eval.
- [Tencent Full-Stack JD Fit](tencent-fullstack-jd-fit.md): JD capability mapping for React/Vite, Go/Gin, MySQL, Redis, Node.js tooling, admin dashboard, and performance evidence.
- [Tencent Interview Questions](tencent-interview-questions.md): evidence-backed Go, LangGraph, MCP, security, RAG, frontend, and recovery questions.
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

1. Open MCP installations and show immutable revision, risk, Skill binding, and secret-configured state.
2. Launch `lookup_policy` from its prefilled Agent Lab link; show automatic read execution and untrusted MCP output.
3. Launch `create_support_ticket`; show LangGraph interrupt, checkpoint version, approval reason, and fixed tool revision.
4. Run `make interview-chaos`; explain restart recovery and why execution/ticket counts remain one under replay.
5. Run `make interview-smoke`; show Elasticsearch IK, source-filtered Chinese retrieval, jieba grounding, metrics, and secret-leak checks.
6. Close with boundaries: Compose uses rules/OpenBao dev mode/interview trust; external OpenBao, gVisor, NetworkPolicy, multi-Pod capacity, and real LLM quality require staging evidence.

Use gRPC/Kafka/WebRTC/recording paths only as follow-up system-design material after this chain is clear.

## One-Command Demo

The current primary demo starts the full MySQL/Redis/OpenBao/Elasticsearch/Go/Python/Sandbox/MCP/Web stack. It does not require external model credentials:

```bash
make interview-demo
```

Use the printed credentials at `http://localhost:3000/agent-tools`, then verify restart recovery:

```bash
make interview-chaos
```

`make interview-smoke` checks service readiness, secret leakage, Elasticsearch IK, RAG source filtering, jieba grounding, and metrics. The older SQLite benchmark and partial live-backend commands remain secondary evidence, not the primary demo.

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

For the Python-first Agent demo, run `make run-agent-runtime`, `make run-rag-runtime`, and set `AGENT_RUNTIME=python_langgraph`. Use `PY_AGENT_PROVIDER=rules` for deterministic local eval and `PY_AGENT_PROVIDER=openai_compatible` only when real credentials are available.

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
make python-agent-eval
make python-rag-eval
make ai-agent-jd-eval
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

- Run the Helm/gVisor design in a real multi-node staging cluster and capture NetworkPolicy and Pod-kill evidence.
- Replace in-process demo counters with OTel/Prometheus persistence and dashboards.
- Add live LLM token streaming behind the existing OpenAI-compatible planner if a demo needs model-output streaming in addition to backend tool-event streaming.
- Extend benchmark/load tests to authenticated WebSocket replay and meeting room event throughput.
- Replace the current observed outbox handler with production publishers when the deployment target is clear.
- Capture measured baseline numbers in [Performance Report](performance-report.md), reproducible KPI summaries in [Resume Eval](resume-eval.md), and user-facing scoring notes in [Agent UX Eval](agent-ux-eval.md).
