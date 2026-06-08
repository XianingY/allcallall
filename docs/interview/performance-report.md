# Performance Report Template

This report is a living document. Fill in numbers after running load tests against a local or staging environment.

## Goals

- Validate WebSocket replay behavior under concurrent clients.
- Validate meeting event throughput and database write pressure.
- Validate Agent run idempotency and tool-call write amplification.
- Validate recording download authorization and storage latency.

## Test Environment

- Backend: Go 1.24, Gin, Gorm
- Database: MySQL 8.0 or local Docker MySQL
- Redis: Redis 7.2
- Storage: local or S3-compatible MinIO
- Machine:
- Commit SHA:

## Load Scripts

Scripts live in `scripts/load/`.

- `agent-run-smoke.sh`: concurrent Agent run creation against one conversation.
- `ws-connections.mjs`: WebSocket connection smoke/load template.

## Metrics To Capture

Backend `/api/v1/metrics`:

- `agent_run_total`
- `agent_run_failed_total`
- `agent_tool_call_total`
- `agent_memory_write_total`
- `chat_realtime_delivery_fail_total`
- `meeting_join_total`
- `recording_download_total`
- `recording_storage_write_fail_total`

System metrics:

- p50/p95/p99 API latency
- MySQL CPU, slow queries, active connections
- Redis ops/sec and memory
- WebSocket connected clients
- Error rate

## Baseline Results

| Scenario | Concurrency | Duration | p95 Latency | Error Rate | Notes |
| --- | ---: | ---: | ---: | ---: | --- |
| Agent run creation | TBD | TBD | TBD | TBD | Idempotency on/off |
| WebSocket connections | TBD | TBD | TBD | TBD | Anonymous or authenticated |
| Meeting event replay | TBD | TBD | TBD | TBD | `since_id` replay |
| Recording download | TBD | TBD | TBD | TBD | local vs S3 |

## Bottlenecks To Discuss

- MySQL is currently the durable realtime replay store; high event throughput may require Redis Streams or Kafka later.
- Agent run writes multiple rows per request: run, steps, tool calls, memory, outbox, message, follow-up task.
- WebRTC media relay and recording are separate bottleneck domains from HTTP API latency.

## Optimization Ideas

- Batch realtime event writes per recipient group.
- Add an outbox publisher worker with retry/backoff.
- Add Redis Streams for high-volume room events.
- Add SQL indexes for hottest replay/query paths.
- Split collaboration service into chat, room, recording, and support services.
