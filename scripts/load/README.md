# Load And Smoke Scripts

These scripts are lightweight interview/demo helpers. They are not a replacement for a full load-test platform.

## Agent Run Smoke

```bash
TOKEN=<jwt> \
ORGANIZATION_ID=<id> \
CONVERSATION_ID=<id> \
CONCURRENCY=10 \
./scripts/load/agent-run-smoke.sh
```

What it validates:

- `POST /api/v1/agent/runs`
- Idempotency-key handling per request
- Agent write amplification: run, steps, tool calls, memory, outbox, message, follow-up task

## WebSocket Connection Smoke

```bash
TOKEN=<jwt> \
ORGANIZATION_ID=<id> \
CLIENTS=10 \
DURATION_MS=10000 \
node scripts/load/ws-connections.mjs
```

What it validates:

- WebSocket connection acceptance
- Basic connection stability
- Message/error counters

Note: this script uses the global `WebSocket` runtime. Use Node 22+ or adapt it to a `ws` dependency if your local Node runtime does not expose global WebSocket.
