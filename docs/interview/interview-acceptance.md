# Interview Chain Acceptance Record

This record captures the latest local acceptance run for the current interview branch. It is
evidence for protocol and recovery behavior, not a throughput benchmark or production SLA.

| Field | Value |
| --- | --- |
| Code before documentation commit | `d9072264` |
| Provider | deterministic `rules` |
| Environment | Docker Compose Interview overlay, local MySQL/Redis/OpenBao/Elasticsearch |
| Web | `http://localhost:3000` |
| Generated secret state | `/tmp/allcallall-interview-${USER}` with mode `0700/0600` |
| External model credentials | not required |

## Commands

```bash
make interview-smoke
make interview-chaos
```

## Observed result

```text
smoke passed: stack, metrics, Elasticsearch IK, RAG source filter, and jieba grounding
read chain passed: run=4 MCP output marked untrusted and execution=1
write chain passed: run=5 resumed from checkpoint, execution=1, external_tickets=1
interview chain passed: Go -> Python LangGraph -> checkpoint -> approval -> Sandbox -> HTTPS MCP -> audit
```

## What this proves

- Services wait for migration and public-API seed before readiness is accepted.
- Chinese knowledge is created through public API, indexed in Elasticsearch with IK, and returned
  only when the RAG request asks for `source_types=[knowledge]`.
- Python grounding accepts a supported Chinese claim and rejects an unrelated claim using jieba.
- A read MCP result is represented as untrusted context.
- A write MCP call pauses at a Python checkpoint, survives Agent Runtime restart, resumes after Go
  approval, and creates one MCP execution and one external SQLite ticket.
- The smoke path checks that the MCP bearer token is absent from service logs and MySQL dump.

## What this does not prove

- It does not prove open-domain LLM quality, multi-node throughput, or production SLO.
- It does not prove gVisor is active in Docker Compose; gVisor and NetworkPolicy are Helm/production
  design constraints.
- It does not make arbitrary third-party MCPs exactly-once; that requires a compatible external
  idempotency contract.
