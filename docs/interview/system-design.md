# System Design: AI-Powered Realtime Collaboration Backend

This page frames AllCallAll like a system-design interview. The project is not presented as a finished commercial app here; it is a backend engineering case study.

## Problem Statement

Build a realtime collaboration backend for cross-border support teams. Users can join organizations, work in conversation threads, escalate a thread into a meeting, receive realtime updates, store recording assets, and run an AI Agent after meetings to produce action items.

## High-Level Architecture

```mermaid
flowchart LR
    Client["Android / Web / Desktop"] --> API["Gin HTTP API"]
    Client --> WS["Chat WebSocket"]
    Client --> RTC["WebRTC Signaling"]

    API --> Auth["Auth + Refresh Sessions"]
    API --> Collab["Collaboration Service"]
    API --> Agent["Agent Service"]
    API --> Storage["Recording Storage"]

    Collab --> MySQL[("MySQL")]
    Auth --> MySQL
    Agent --> MySQL
    Storage --> S3["S3-compatible / Local"]

    Collab --> Redis[("Redis")]
    WS --> Redis
    RTC --> Media["Pion Media Engine"]

    Agent --> Planner["Rules Planner / OpenAI-compatible seam"]
    Agent --> Tools["Backend Tool Executor"]
    Tools --> Outbox["Event Outbox"]
    Outbox --> MySQL
```

## Core Data Domains

- Identity: `users`, `refresh_sessions`
- Organization: `organizations`, `organization_members`, `organization_policies`
- Collaboration: `conversations`, `conversation_members`, `messages`, `conversation_notes`
- Realtime: `chat_events` with explicit `sequence` for replay and acknowledgement semantics
- Meeting: `call_rooms`, `call_room_members`, `call_room_events`
- Recording: `recording_sessions`, `recording_files`, `recording_exports`
- Agent: `agent_runs`, `agent_steps`, `agent_tool_calls`, `agent_memories`
- Reliability: `event_outbox`

## Realtime Event Flow

```mermaid
sequenceDiagram
    participant API as Collaboration API
    participant DB as MySQL
    participant Hub as WebSocket Hub
    participant Client as Client

    API->>DB: write message / room update
    API->>DB: create chat_event with sequence
    API->>Hub: publish event
    Hub-->>Client: event_id + sequence + payload
    Client->>API: reconnect with since_id
    API->>DB: list chat_events where id > since_id
    API-->>Client: replay missed events
```

Design notes:

- `event_id` remains backward compatible.
- `sequence` makes the event stream explicit for client ack/replay logic.
- Durable `chat_events` are the source of truth when WebSocket delivery is missed.

## Agent Execution Flow

```mermaid
sequenceDiagram
    participant Client
    participant Handler as AgentHandler
    participant Service as AgentService
    participant Planner
    participant Tools
    participant DB as MySQL

    Client->>Handler: POST /api/v1/agent/runs + Idempotency-Key
    Handler->>Service: RunConversationAssistant
    Service->>DB: verify conversation membership
    Service->>DB: find run by idempotency key
    Service->>DB: create agent_run running
    Service->>DB: load messages, notes, rooms, memories
    Service->>Planner: rules plan
    Planner-->>Service: summary, action_items, next_step, risk_flags
    Service->>Tools: write conversation message
    Tools->>DB: message + tool_call + outbox
    Service->>Tools: create follow-up task
    Service->>Tools: upsert agent memory
    Service->>DB: mark run ready
    Handler-->>Client: run, steps, tool_calls
```

## Reliability Choices

- Service-layer permission checks are repeated even when handlers already authenticate.
- Agent run idempotency prevents repeated tool side effects during retry.
- Outbox records durable side effects for async event delivery.
- WebSocket replay reduces dependency on perfect long-lived connections.
- Recording storage has local and S3-compatible drivers.

## Interview Tradeoffs

- Current Agent provider is deterministic for tests; an LLM provider is a replaceable seam.
- Current outbox is persisted but not yet backed by a dedicated publisher worker.
- Current realtime replay uses MySQL-backed event records; Redis Streams can be introduced if throughput requires it.
- Current media layer is sufficient for demonstrating WebRTC signaling and room state, not a production-grade Zoom-scale SFU.
