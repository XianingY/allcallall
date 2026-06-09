# Interview Demo Script

This script is optimized for a backend/AI Agent interview. It keeps the story focused on engineering evidence: deterministic Agent evaluation, async outbox execution, durable realtime replay, and authenticated WebSocket replay.

## 5-Minute Demo Path

1. Start with the positioning: AllCallAll is an AI-powered realtime collaboration backend, not primarily a mobile app demo.
2. Run the local demo command:

```bash
make interview-demo
```

3. Point to the generated report directory printed at the end.
4. Explain the four artifacts:

- `agent-eval.json`: deterministic Agent eval cases for summary, action items, next step, and risk flags.
- `interview-bench.json`: Agent run queue, outbox drain, tool calls, memory writes, messages, and follow-up tasks.
- `realtime-replay-bench.json`: durable `chat_events` write/replay behavior with recipient scoping.
- `chat-ws-replay-bench.json`: authenticated `/api/v1/chat/ws` replay through an in-process Gin/WebSocket server.

## Suggested Talking Points

- Agent reliability: `POST /agent/runs` creates a pending run and enqueues `agent.run.requested`; the worker executes the run and records steps, tool calls, memory, and outbox events.
- Tool boundary: read-only tools gather context, while side-effect tools write conversation messages, tasks, and memory. The registry documents schemas, permissions, and idempotency key templates.
- Realtime reliability: replay is based on durable event IDs and sequence numbers, not only live WebSocket delivery.
- Determinism: the default demo does not require external LLM keys, MySQL, Redis, JWTs, or a running backend.

## Live Backend Variant

Use this only when Docker and local database services are available:

```bash
make interview-demo-live
```

This starts MySQL/Redis through `scripts/development/start-services.sh` and runs `backend/cmd/interview-seed` with `CONFIG_PATH=./configs/config.yaml`.

After starting the backend in another terminal:

```bash
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/server/main.go
```

Inspect health and metrics:

```bash
curl -s http://localhost:8080/api/v1/health
curl -s http://localhost:8080/api/v1/metrics
```

After logging in and obtaining a JWT, run live smoke scripts:

```bash
BASE_URL=http://localhost:8080 \
TOKEN=<jwt> \
ORGANIZATION_ID=<id> \
CONVERSATION_ID=<id> \
./scripts/load/agent-run-smoke.sh
```

```bash
WS_URL=ws://localhost:8080/api/v1/chat/ws \
TOKEN=<jwt> \
ORGANIZATION_ID=<id> \
node scripts/load/ws-connections.mjs
```

## Common Interview Questions

- Why not call an LLM directly from the handler?
  The handler only creates an auditable run. Planner execution, tool calls, idempotency, and retries are backend-owned and observable.

- Why keep a rules provider?
  It gives deterministic CI/eval behavior and a stable fallback when a real provider is unavailable.

- Where is the distributed-systems part?
  Outbox events, WebSocket replay, idempotency keys, request IDs, metrics, and durable event storage are the core reliability mechanisms.
