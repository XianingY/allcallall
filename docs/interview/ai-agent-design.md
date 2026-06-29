# AI Agent Design Notes

AllCallAll has two Agent execution styles:

- **ReAct-style single Agent** through `/api/v1/agent/runs`, used for quick conversation questions, summaries, risks, follow-ups, and write-back.
- **Workflow/DAG Agent** through `/api/v1/agent/workflows`, used for Agent Lab style task graphs, decomposition, tool policies, and human approvals.

The default provider is deterministic `rules`, so tests and demos do not require model credentials. `mock_llm` exercises prompt construction and structured JSON parsing without network calls. `openai_compatible` is the live-provider seam.

## ReAct Run Scope

A run analyzes one organization-scoped conversation and can produce:

- Summary.
- Action items.
- Next step.
- Risk flags.
- System message written back to the conversation.
- Follow-up task.
- Scoped memory for future runs.

API:

- `POST /api/v1/agent/runs`
- `GET /api/v1/agent/runs/:id`
- `GET /api/v1/agent/runs/:id/events`
- `GET /api/v1/agent/runs/:id/events/stream`

Required:

- JWT authentication.
- `X-Organization-ID`.
- Membership in the target conversation.
- Optional `Idempotency-Key` for retry-safe execution.

## Execution Flow

```mermaid
sequenceDiagram
    participant Client
    participant Handler
    participant Service
    participant Outbox
    participant Worker
    participant Planner
    participant Tools
    participant DB

    Client->>Handler: POST /agent/runs
    Handler->>Service: validate org + conversation membership
    Service->>DB: create/load pending run by idempotency key
    Service->>Outbox: agent.run.requested
    Handler-->>Client: 202 pending run
    Worker->>Outbox: claim event
    Worker->>Service: ExecuteRun(run_id)
    Service->>DB: pending -> running
    Service->>DB: load context
    Service->>Planner: plan with configured provider
    Planner-->>Service: structured output
    Service->>DB: record read-only context tool calls
    Service->>Tools: write message / create task / upsert memory
    Tools->>DB: side effects + tool logs
    Service->>DB: mark run ready or failed
```

## Context Sources

`loadConversationContext` gathers:

- Conversation metadata, status, priority, assignee, contact binding.
- Recent messages.
- Internal notes.
- Recent rooms.
- Conversation members.
- Scoped Agent memories.
- Contact profile when bound.
- Call follow-ups and `call_transcript_segments` for older 1:1 call context.
- `meeting_transcript_segments` from recording-end meeting transcription.
- RAG context chunks from conversation artifacts and knowledge sources.

Meeting transcript segments are separate from call transcript segments. Retrieved references use `meeting_transcript` so citations can distinguish "meeting recording transcription" from older "call subtitles". Each meeting citation carries the recording session, recording file, transcript segment, and start/end offsets, allowing the client to open the transcript at the cited passage.

## Recording Transcription Integration

Recording stop can enqueue `recording.transcription.requested` when:

```bash
TRANSCRIPTION_ENABLED=true
TRANSCRIPTION_PROVIDER=openai_compatible
```

The outbox worker materializes local or S3 audio, calls the configured provider, and writes `meeting_transcript_segments`. The Agent prioritizes the latest recording session so it can answer questions such as "what did we just discuss in the meeting?" after the recording is processed.

Failure behavior:

- No conversation binding: transcription job is `skipped`.
- No audio file: `skipped`.
- Storage and permanent provider errors: `failed` with an explicit reason.
- Retryable provider and network errors: returned to the outbox lease/attempt mechanism.
- Recording persistence still succeeds even if transcription later fails.

## Tools

Tool descriptors live in `backend/internal/agent/tool_registry.go`. Important tools:

- `query_recent_meetings`
- `query_conversation_members`
- `query_contact_profile`
- `query_context_chunks`
- `write_conversation_message`
- `create_follow_up_task`
- `upsert_agent_memory`

Read-only context tools are persisted as `agent_tool_calls` for auditability. Mutating tools are backend-owned: the planner proposes intent, and service code performs permission checks, transactions, idempotency, metrics, and outbox writes.

## Workflow Agent / Agent Lab

Workflow endpoints:

- `POST /api/v1/agent/workflows`
- `GET /api/v1/agent/workflows`
- `GET /api/v1/agent/workflows/:id`
- `POST /api/v1/agent/workflows/:id/process`
- `GET /api/v1/agent/approvals`
- `POST /api/v1/agent/approvals/:id/decision`

Models:

- `workflow_runs`
- `workflow_tasks`
- `workflow_history_events`
- `workflow_signals`
- `workflow_timers`
- `agent_messages`
- `tool_policies`
- `tool_approvals`

This path is for DAG-style decomposition and human approval. Approval statuses include `pending`, `approved`, `rejected`, `executed`, and `failed`.

### Bounded ReAct Inside Role Tasks

The Workflow Agent now combines DAG control with bounded role-level ReAct:

- `searcher` runs a read-only ReAct loop with `max_iterations=3` and can call `query_context_chunks`.
- `risk_analyst` runs a read-only ReAct loop with `max_iterations=2` and can call `query_context_chunks` plus `query_recent_meetings`.
- `summarizer` stays as direct synthesis for stability.

Each role iteration records a plan and observation in `agent_messages`, and the task output includes `react_trace`. Only read-only tools are allowed inside role ReAct. Side-effect tools such as `write_conversation_message`, `create_follow_up_task`, and `upsert_agent_memory` still enter the `propose_tools -> approval -> commit_result` path.

### Python LangGraph Runtime

Selected workflow presets can run through the optional Python Agent Runtime:

```bash
AGENT_RUNTIME=python_langgraph
PY_AGENT_RUNTIME_BASE_URL=http://127.0.0.1:8090
PY_AGENT_RUNTIME_STRICT=true
```

Current Python-supported presets are `meeting_brief`, `risk_review`, `follow_up_planner`, and `context_qa`. The split is intentionally narrow:

- Go remains the source of truth for auth, organization membership, conversation state, meeting transcripts, tool schemas, policy, approval records, audit history, and final side effects.
- Python owns Agent orchestration: workflow registry, LangGraph DAG execution, provider adapters, versioned prompts, retrieval/rerank trace, grounding checks, bounded ReAct-style read reasoning inside `searcher` and `risk_analyst`, structured trace events, citations, and write-tool proposals.
- Python can call Go-owned read-only tools through an internal token-protected tool bridge (`AGENT_RUNTIME_TOOL_TOKEN` / `PY_AGENT_TOOL_BRIDGE_TOKEN`). If the bridge is not configured, Python uses the context preloaded by Go.
- Python does not write the main database and does not execute write tools. Returned proposals become `tool_approvals` in Go and must pass the existing approval path before `commit_result`.
- `PY_AGENT_PROVIDER=rules` is deterministic for eval; `PY_AGENT_PROVIDER=openai_compatible` uses an OpenAI-compatible `/chat/completions` provider with JSON structured output and explicit error classification.

### Bounded Agentic RAG

The Python runtime can optionally enable bounded Agentic RAG with `PY_AGENT_ENABLE_AGENTIC_RAG=true`. This does not replace the Go Hybrid RAG or rerank layer. Instead, it controls retrieval strategy above that layer:

- `retrieval_planner` creates source-specific retrieval steps for meeting transcript, knowledge, and conversation context.
- `retrieval_loop` runs at most three read-only tool calls through the Go bridge and records `rag.plan`, `rag.tool_call`, `rag.observe`, and `rag.refine` events.
- `evidence_pack` selects final citations and keeps source coverage, confidence, and rejected count.
- `sufficiency_gate` blocks write-tool proposals when evidence is missing, so unsupported answers remain conservative.

This design is intentionally bounded: Python plans retrieval and synthesis, while Go still owns data access, permissions, approval, audit, and side effects.

Runtime flow:

```mermaid
flowchart TD
    A["Go workflow worker"] --> B["Load conversation + transcript context"]
    B --> C["Python LangGraph Runtime"]
    C --> D["collect_context"]
    D --> E["decompose"]
    E --> F["searcher bounded ReAct max 3"]
    E --> G["summarizer synthesis"]
    E --> H["risk_analyst bounded ReAct max 2"]
    F --> T["Go read-only tool bridge"]
    H --> T
    F --> I["merge"]
    G --> I
    H --> I
    I --> J["propose write tools"]
    J --> K["Go tool approval records"]
    K --> L["Go approval / reject / commit_result"]
```

This gives the project a defensible AI microservice boundary without splitting stable product domains such as auth, chat, meetings, or organization management too early.

## Trace And Events

Agent API responses include a derived trace from persisted rows, not a separate trace table:

- `agent_runs`
- `agent_steps`
- `agent_tool_calls`

Run events:

- `run_queued`
- `run_started`
- `step_started`
- `step_done`
- `tool_called`
- `tool_done`
- `run_ready`
- `run_failed`

`events/stream` uses SSE over persisted rows, so clients can reconnect or fall back to polling.

## Provider Seam

```bash
AGENT_PROVIDER=rules
AGENT_PROVIDER=mock_llm
AGENT_PROVIDER=openai_compatible
AGENT_PROVIDER_STRICT=true
AGENT_OPENAI_BASE_URL=https://api.example.com/v1
AGENT_OPENAI_MODEL=example-model
AGENT_OPENAI_API_KEY=...
```

Development may leave strict mode disabled and record a fallback to `rules`. Beta/production sets `AGENT_PROVIDER_STRICT=true`: missing provider configuration fails startup and request failures remain visible rather than being presented as model output. Tool execution remains backend-mediated; the model never mutates application state directly.

The `meeting_brief` workflow has an additional grounding gate: it starts only when the conversation has a `ready` recording transcription with persisted segments. Read-only tools execute automatically, while conversation writes, follow-up creation, and memory updates remain in the approval path.

## RAG And Knowledge

Conversation context chunks and knowledge chunks provide the retrieval layer:

- Conversation messages, notes, memories, call transcripts, meeting transcripts, and follow-ups become retrievable chunks.
- Knowledge sources support text/URL/file import, grouping, duplicates, canonical version selection, dead letters, and reingest.
- Elasticsearch can back chunk/message indexing where configured.
- Hybrid retrieval can combine BM25, dense vector search, and RRF fusion; an explicit rerank layer can then annotate candidates with `rerank_score`, `rerank_reason`, and `final_rank`.
- Rerank supports deterministic `rules` for fixture eval and a `cross_encoder_compatible` HTTP provider for future local bge-reranker/Cohere-compatible services.
- Retrieval remains permission-scoped through organization and conversation membership.

## Idempotency And Safety

- `POST /agent/runs` accepts `Idempotency-Key`.
- Outbox events use idempotency keys such as `agent.run.requested:<run_id>`.
- Replaying a completed run does not repeat side effects.
- Mutating tools write visible system messages/tasks/memory rather than silently editing conversation state.
- The Agent does not read secrets, raw tokens, private keys, or unbounded raw audio.

## Eval Harness

```bash
make agent-eval
make rag-eval
make rerank-eval
make workflow-eval
make python-agent-eval
make agent-demo-report
make ai-portfolio-eval
```

The eval harnesses run deterministic planner, RAG, rerank, workflow, and Python LangGraph task cases without external credentials. Workflow eval includes a meeting recap case that verifies bounded role ReAct retrieves `meeting_transcript` citations while read tools bypass human approval and write tools still require it. Python eval writes `agent-runtime/evals/reports/python-agent-eval.{json,md}` and separately checks task success, citation grounding, tool intent, approval safety, prompt schema presence, grounding-check trace, and unsupported-claim guarding.
