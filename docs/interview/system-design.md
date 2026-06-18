# System Design: AI-Powered Realtime Collaboration Backend

AllCallAll is best presented as a backend system-design case study: realtime collaboration, WebRTC meetings, recording storage/transcription, AI Agent execution, durable outbox workers, and a microservice-friendly evolution path.

## Problem Statement

Build a collaboration backend where teams can work in organization-scoped conversations, escalate a thread into a meeting, preserve recording assets, transcribe meeting recordings, search historical context, and run an Agent that produces summaries, risks, action items, and follow-ups.

## High-Level Architecture

```mermaid
flowchart LR
    Client["Android / Web / Desktop"] --> API["Gin API / Signaling Gateway"]
    Client --> ChatWS["/chat/ws Replay"]
    Client --> RTC["/ws + signaling fallback"]

    API --> Auth["JWT / Refresh Sessions"]
    Auth -- optional --> UserSvc["gRPC User Service"]
    UserSvc --> MySQL[("MySQL")]

    API --> Collab["Collaboration Service"]
    API --> Agent["Agent Service"]
    API --> Knowledge["Knowledge Service"]
    API --> Search["Search Service"]
    API --> Storage["Recording Storage"]

    Collab --> MySQL
    Agent --> MySQL
    Knowledge --> MySQL
    Storage --> Obj["Local / S3-compatible Object Storage"]
    Search --> ES[("Elasticsearch")]

    Collab --> Outbox[("event_outbox")]
    Agent --> Outbox
    Knowledge --> Outbox

    AgentWorker["Agent Worker"] --> Outbox
    OutboxWorker["Outbox Worker"] --> Outbox
    SearchWorker["Search Worker"] --> Outbox

    OutboxWorker --> Transcription["Transcription Provider"]
    Transcription --> MySQL

    OutboxWorker -- settlement events --> Kafka[("Kafka-compatible Broker")]
    DataWorker["Data Worker"] --> Kafka
    DataWorker --> MySQL

    SearchWorker --> ES
    ChatWS --> Redis[("Redis")]
    RTC --> Redis
```

## Core Data Domains

- Identity/security: `users`, `refresh_sessions`, `email_verification_codes`, `push_devices`.
- Collaboration: `organizations`, `organization_members`, `conversations`, `messages`, `conversation_notes`, `chat_events`.
- Meetings: `call_rooms`, `call_room_members`, `call_room_events`.
- Recording/transcription: `recording_sessions`, `recording_files`, `recording_transcriptions`, `meeting_transcript_segments`, `recording_exports`.
- Agent/workflow/RAG: `agent_runs`, `agent_steps`, `agent_tool_calls`, `agent_memories`, `agent_context_chunks`, `workflow_*`, `rag_*`, `tool_approvals`.
- Async backbone: `event_outbox`.
- Infra read/write models: `room_settlements`, Elasticsearch message/chunk indexes.

## Realtime Flow

```mermaid
sequenceDiagram
    participant API
    participant DB as MySQL
    participant Hub as WebSocket Hub
    participant Client

    API->>DB: create message / room event
    API->>DB: write recipient-scoped chat_events
    API->>Hub: publish live event
    Hub-->>Client: event_id + sequence + payload
    Client->>API: reconnect with since_id
    API->>DB: load chat_events where id > since_id
    API-->>Client: replay missed events
```

Design points:

- `/api/v1/chat/ws` is collaboration replay.
- `/api/v1/ws` and `/api/v1/signaling/*` are 1:1 WebRTC signaling.
- `sequence` mirrors durable event ordering.
- Per-recipient events keep reconnect replay authorization-friendly.

## Recording Transcription Flow

```mermaid
sequenceDiagram
    participant Client
    participant API as Recording API
    participant DB as MySQL
    participant Outbox
    participant Worker as Outbox Worker
    participant Provider as Transcription Provider
    participant Agent

    Client->>API: POST /rooms/:id/recording/stop
    API->>DB: persist recording_session + recording_files
    API->>DB: create recording_transcriptions pending
    API->>Outbox: recording.transcription.requested
    API-->>Client: recording saved
    Worker->>Outbox: claim transcription event
    Worker->>Provider: transcribe local audio file
    Provider-->>Worker: transcript segments
    Worker->>DB: write meeting_transcript_segments
    Worker->>DB: mark transcription ready/failed/skipped
    Agent->>DB: load meeting_transcript_segments as context
```

v1 uses `TRANSCRIPTION_PROVIDER=mock` and requires locally readable recording files. S3-only files are marked failed until Reader/download support is implemented. This path is independent of realtime translation, whose UI is currently hidden.

## Agent Execution Flow

```mermaid
sequenceDiagram
    participant Client
    participant Handler as AgentHandler
    participant Service as AgentService
    participant Outbox
    participant Worker
    participant Planner
    participant Tools
    participant DB

    Client->>Handler: POST /agent/runs + Idempotency-Key
    Handler->>Service: verify org + conversation membership
    Service->>DB: create/load pending run
    Service->>Outbox: agent.run.requested
    Handler-->>Client: 202 pending run
    Worker->>Outbox: claim requested event
    Worker->>Service: ExecuteRun(run_id)
    Service->>DB: load messages, notes, rooms, memories, transcripts
    Service->>Planner: rules / mock_llm / openai_compatible
    Planner-->>Service: summary, actions, risks, tool plan
    Service->>Tools: backend-owned side effects
    Tools->>DB: message, follow-up, memory, tool logs
    Service->>DB: mark run ready
```

The Agent sees both older 1:1 `call_transcript_segments` and newer meeting recording `meeting_transcript_segments`, with separate source attribution.

## gRPC, Kafka, Elasticsearch

- gRPC: `USER_SERVICE_GRPC_ADDR` switches API auth validation to the extracted User Service.
- Kafka: room-ended events go outbox -> Kafka -> `data-worker` -> `room_settlements`.
- Elasticsearch: message creation goes outbox -> `search-worker` -> ES; query results are re-authorized through MySQL membership checks.

## Reliability Choices

- Service-layer permission checks are repeated after handler auth.
- `X-Request-ID` is propagated through HTTP errors, Agent runs, and outbox rows.
- Outbox rows use idempotency keys, attempts, and claim leases.
- Agent runs use idempotency keys plus `attempts`/`lease_until`.
- Recording transcription failure does not undo recording persistence.
- WebSocket replay is durable, not only in-memory.
- Kafka consumers and message fan-out are idempotent.
- ES is a read model, never the authorization boundary.

## Tradeoffs

- Single repo and shared schema keep demos runnable.
- Extracted workers/gRPC/Kafka/ES prove boundaries without forcing a full service mesh.
- Kubernetes, real ASR, and S3 transcription reads are future enhancements.
- Current media layer is enough for signaling/meeting-state/recording demos, not a production SFU claim.
