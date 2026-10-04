# AllCallAll

[简体中文](README.zh-CN.md) · [Documentation](docs/README.md) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md)

AllCallAll is an open-source realtime collaboration platform with an AI Agent
layer. It combines team conversations, meetings, recordings, transcripts,
knowledge retrieval, approval-gated automation, and administration in one
self-hostable system.

## Why AllCallAll

Collaboration products often separate discussion, meetings, searchable context,
and AI automation into disconnected systems. AllCallAll keeps those workflows
together while preserving a strict authority boundary: the Go platform owns
product data and writes, while the Python runtime proposes and explains Agent
actions.

The project is designed for contributors who want to study or extend a real
cross-platform system rather than a single-framework demo.

## What Is Included

- Organization workspaces, invitations, contacts, conversations, notes, and
  audit trails.
- Realtime messaging, presence, WebSocket replay, WebRTC signaling, meeting
  rooms, recordings, and transcript ingestion.
- A React/Vite production Web client, an Expo Android/iOS client, and a thin
  Electron desktop shell.
- Agent runs with retrieval, citations, memory, approvals, tool proposals, and
  trace inspection.
- MySQL, Redis, S3-compatible recording storage, optional Kafka-compatible
  event transport, optional Elasticsearch, and Kubernetes/Helm assets.
- Contract, unit, integration, browser, mobile, security, and image-validation
  checks in CI.

Realtime translation code remains for compatibility, but its mobile UI entry
points are currently hidden. Deterministic evaluation results are regression
evidence for checked fixtures, not general model-quality claims.

## Architecture

```text
React Web ─┐
Expo Mobile├──> Go API / workers ───> MySQL, Redis, object storage
Electron ──┘            │
                        ├──> Python Agent Runtime ──> model providers
                        └──> Python RAG Runtime ────> retrieval adapters
```

The repositories remain independently buildable:

- This repository owns users, organizations, conversations, meetings,
  transcripts, permissions, approvals, audit logs, and write execution.
- [`allcallall-agent-runtime`](https://github.com/XianingY/allcallall-agent-runtime)
  owns LangGraph orchestration, retrieval planning, reranking, grounding,
  citations, tool proposals, and evaluation.
- HTTP, OpenAPI, JSON Schema, and the Go Tool Bridge form the integration
  boundary. Python never writes product data directly.

See the [system documentation](docs/README.md) for data flows and component
details.

## Quick Start

Prerequisites: Go 1.26, Node.js 24 with npm, Docker Compose, and a sibling
checkout of the Python runtime when Agent/RAG features are needed.

Install JavaScript dependencies from the repository root and start MySQL and
Redis:

```bash
npm ci
./scripts/development/start-services.sh
```

Run the API:

```bash
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/server
```

In another terminal, start the Web client:

```bash
cd web
npm run dev
```

The Web app is served at `http://localhost:5173`; the API health endpoint is
`http://localhost:8080/api/v1/health`. For native, desktop, extracted-worker,
and Agent runtime setup, follow the maintained [Quick Start](docs/getting-started/quick-start.md).

## Repository Map

| Path | Responsibility |
| --- | --- |
| `backend/` | Go/Gin API, product domains, persistence, workers, and process entrypoints |
| `web/` | React + Vite production Web application |
| `mobile/` | Expo native Android/iOS application |
| `desktop/` | Electron shell around the Web application |
| `packages/` | Shared TypeScript packages, including generated API types |
| `infra/` | Local infrastructure, images, Kubernetes manifests, and Helm chart |
| `deploy/` | Deployment-oriented configuration and scripts |
| `contracts/` | Legacy fixtures; authoritative schemas live in the Python runtime repository |
| `docs/` | Maintained product documentation and clearly labeled archives |

## Documentation

- [Documentation index](docs/README.md)
- [Quick start](docs/getting-started/quick-start.md)
- [Backend guide](backend/README.md)
- [API reference](docs/reference/api/http-api.md)
- [Configuration reference](docs/reference/configuration/runtime.md)
- [Deployment guide](docs/guides/deployment/deployment-guide.md)
- [Message privacy and compliance](docs/reference/security/privacy-and-compliance.md)
- [Python runtime repository](https://github.com/XianingY/allcallall-agent-runtime)

`INDEX.md` remains as a compatibility pointer for older links.

## Project Status

AllCallAll is under active development and remains pre-1.0. The core product
flows and automated checks are implemented, while deployment hardening,
large-scale validation, and some optional integrations continue to evolve.

The merge-ready local gate is:

```bash
make verify-full
```

This command expects the Python runtime as a sibling checkout. Use `make verify`
for a faster development check; it intentionally omits lint and several client
test suites.

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md), follow the
[Code of Conduct](CODE_OF_CONDUCT.md), and use the issue and pull-request
templates. Dependency installation for `web/` and `mobile/` must always run
from the repository root so the root `package-lock.json` remains authoritative.

## Security and Support

Do not disclose vulnerabilities in public issues. Follow the private reporting
instructions in [SECURITY.md](SECURITY.md). For usage questions, reproducible
bugs, and feature discussions, see [SUPPORT.md](SUPPORT.md).

## License

AllCallAll is available under the [MIT License](LICENSE).
