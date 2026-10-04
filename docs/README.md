# AllCallAll Documentation

This is the canonical documentation index for the AllCallAll product repository.
The maintained pages below describe current behavior; historical plans, reports,
and portfolio material are kept separately under the archive.

English documentation is authoritative. Start with the root
[project overview](../README.md), then use the task-oriented and reference pages
below. Contributor workflow and project policies live in
[CONTRIBUTING.md](../CONTRIBUTING.md), [SECURITY.md](../SECURITY.md), and
[SUPPORT.md](../SUPPORT.md).

## Getting Started

- [Quick Start](getting-started/quick-start.md) — install dependencies, start local services, and run the product clients.
- [Frequently Asked Questions](guides/faq.md) — common repository, development, CI, configuration, and security questions.
- [Backend README](../backend/README.md) — Go service entrypoints and backend-specific commands.

## Architecture

- [System Overview](architecture/system-overview.md) — repository boundaries, major components, and end-to-end data flow.
- [Backend Architecture](architecture/backend.md) — Go domains, handlers, persistence, and authority boundaries.
- [Agent Platform Architecture](architecture/agent-platform.md) — product integration with Agent and RAG services.
- [Workers and Processes](architecture/workers.md) — embedded and extracted worker responsibilities.
- [Agent Execution and Approval Flow](architecture/agent-execution.md) — execution lifecycle, approval gates, and write authority.
- [Bounded Agentic RAG](architecture/agentic-rag.md) — retrieval, grounding, and citation boundaries.
- [Python Runtime Integration](architecture/python-runtime-integration.md) — cross-repository APIs and ownership.

## Guides

### Development

- [Web and Desktop Workflow](guides/development/web-desktop-workflow.md)
- [Web Feature Matrix](guides/development/web-feature-matrix.md)
- [Web Smoke Testing](guides/development/web-smoke.md)
- [Beta Smoke Checklist](guides/development/beta-smoke.md)
- [MCP Tool Server](guides/development/mcp-tool-server.md)
- [Sandbox Supervisor Protocol](guides/development/sandbox-supervisor-protocol.md)
- [Worktree Artifacts](guides/development/worktree-artifacts.md)

### Deployment

- [Deployment Guide](guides/deployment/deployment-guide.md)
- [Agent Platform on Kubernetes](guides/deployment/agent-platform-kubernetes.md)
- [Recording Storage and Transcription](guides/deployment/recording-storage.md)
- [Restricted Network Deployment](guides/deployment/restricted-networks.md)
- [Push Notifications](guides/deployment/push-notifications.md)
- [Android Data Safety](guides/deployment/android-data-safety.md)
- [Release Readiness](guides/deployment/release-readiness.md)

### Operations

- [Observability](guides/operations/observability.md)
- [Meetings and Recordings Runbook](guides/operations/meetings-and-recordings.md)
- [Agent Troubleshooting](guides/operations/agent-troubleshooting.md)
- [Privacy and Account Deletion](guides/operations/privacy-and-account-deletion.md)

## Reference

### API and Data

- [HTTP API](reference/api/http-api.md)
- [API Route Map](reference/api/route-map.md)
- [Database Model](reference/api/data-model.md)
- [Meeting Room State Protocol](reference/api/meeting-room-state.md)
- [OpenAPI Contract](api/openapi.yaml)
- [Cross-Repository Contracts](../contracts/README.md)

### Configuration and Security

- [Runtime Configuration](reference/configuration/runtime.md)
- [Deployment Security](reference/security/deployment-security.md)
- [Message Privacy and Compliance](reference/security/privacy-and-compliance.md)
- [Web Authentication Session](reference/security/web-auth-session.md)

## Client Documentation

- [Web and Desktop Workflow](guides/development/web-desktop-workflow.md)
- [Mobile Native Client](mobile/README.md)
- [Mobile Runtime Configuration](mobile/setup/app-env-usage.md)
- [Mobile Audio Assets](mobile/setup/audio-files-setup.md)
- [Historical Mobile Environment Detection](mobile/setup/auto-env-detection.md)
- [Mobile Troubleshooting](mobile/troubleshooting/README.md)
- [Mobile Scripts](../mobile/scripts/README.md)
- [Desktop README](../desktop/README.md)

## Runtime Repository

The Python Agent/RAG runtime is maintained in the sibling
[`allcallall-agent-runtime`](https://github.com/XianingY/allcallall-agent-runtime)
repository. It owns orchestration, retrieval, reranking, grounding, citations,
tool proposals, and evaluation. Its own `docs/README.md` is the canonical runtime
documentation index.

## Archive

[Archived documentation](archive/README.md) contains historical portfolio
material, reports, and implementation plans. Archive content is retained for
traceability and is not maintained as current product guidance.
