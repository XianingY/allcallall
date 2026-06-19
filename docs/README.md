# AllCallAll Documentation

This directory is the maintained documentation set for AllCallAll. The current project positioning is:

**AI-powered realtime collaboration backend for backend systems and AI Agent engineering interviews.**

Historical status reports, temporary migration notes, and duplicated setup guides should not be treated as implementation authority. If a document conflicts with current code, prefer the root README, backend README, and the interview/system design docs.

## Start Here

- [Root README](../README.md): project positioning, repo map, fast start, and runtime summary.
- [Backend README](../backend/README.md): backend entrypoints, commands, API areas, and environment variables.
- [Quick Start](./getting-started/quick-start.md): local startup flow.
- [Configuration](./configuration/configuration.md): backend/mobile/infra configuration reference.

## Interview / Portfolio

- [Interview README](./interview/README.md): suggested reading path and interview narrative.
- [System Design](./interview/system-design.md): current architecture and data flow.
- [Backend Deep Dive](./interview/backend-deep-dive.md): backend module explanation.
- [AI Agent Design](./interview/ai-agent-design.md): Agent runs, tools, approvals, memory, and RAG.
- [Agent RAG Current State](./interview/agent-rag-current-state.md): retrieval implementation notes.
- [Agent Demo Eval Report](./interview/agent-demo-report.md): reproducible planner, RAG, and workflow eval report.
- [Resume Eval](./interview/resume-eval.md): resume-safe KPI summary and generation command.
- [Agent UX Eval](./interview/agent-ux-eval.md): black-box task eval and manual pilot UX rubric.
- [MCP Tool Server](./interview/mcp-tool-server.md): read-only MCP tools and repo-native AI Skill demo.
- [gRPC, Kafka, and Elasticsearch Evolution](./interview/grpc-kafka-es-evolution.md): extracted boundaries and infra extensions.
- [Worker Runtime](./interview/worker-runtime.md): embedded and standalone workers.
- [API Surface](./interview/api-surface.md): route map for interview discussion.
- [Performance Report](./interview/performance-report.md): benchmark evidence and limitations.
- [Resume Bullets](./interview/resume-bullets.md): resume-ready descriptions.

## Backend / API / Data

- [API Documentation](./api/api-documentation.md): maintained API reference.
- [Database Notes](./api/database.md): current model/table notes.
- [Support Runbook: Meetings](./maintenance/support-runbook-meetings.md): operational support checks for rooms and recordings.

## Runtime / Deployment

- [Deployment Guide](./deployment/deployment-guide.md): production-style deployment and environment setup.
- [Recording Storage Deployment](./deployment/recording-storage-deployment.md): local and S3-compatible recording storage.
- [Restricted Network Setup](./deployment/restricted-network-setup.md): mobile networking and TURN/restricted-network notes.
- [Meeting Room State Protocol](./deployment/meeting-room-state-protocol.md): room state event contract.
- [Web Auth Session](./deployment/web-auth-session.md): browser refresh-cookie session behavior.
- [Security Guidelines](./configuration/security-guidelines.md): security and privacy rules.

## Client / Development

- [Mobile Docs](./mobile/README.md): current Expo mobile/Web status.
- [Mobile Scripts](../mobile/scripts/README.md): app-side helper scripts.
- [Web / Desktop Workflow](./development/web-desktop-workflow.md): Web export and Electron wrapper workflow.
- [Web Smoke Test](./testing/web-smoke.md): browser smoke checklist.

## Supporting Docs

- [Firebase / FCM Push Notifications](./features/push-notifications/firebase-integration-guide.md): current push setup and verification guide.
- [Privacy and Account Deletion Support](./deployment/privacy-and-account-deletion-support.md): app-store support material.
- [Android Data Safety Mapping](./deployment/android-data-safety-mapping.md): Play Console data safety reference.
- [PR Description Template](./pr/pr-description-template.md): optional PR template.

## Current Runtime Summary

- Backend default: one API process with embedded workers.
- Extracted processes: `user-service`, `agent-worker`, `outbox-worker`, `data-worker`, `search-worker`, `cleanup-worker`.
- Optional infra profiles: Kafka-compatible broker and Elasticsearch.
- Agent providers: `rules`, `mock`, and OpenAI-compatible provider paths.
- Recording: local or S3-compatible storage.
- Meeting transcription: optional recording-end transcription through `TRANSCRIPTION_ENABLED=true` and `TRANSCRIPTION_PROVIDER=mock`; v1 requires locally readable recording files.
- Realtime translation: backend code remains, but mobile UI entry points are currently hidden.
- Mobile/Web config: only `EXPO_PUBLIC_*`; `APP_ENV` is historical.
- Push: `FCM_SERVICE_ACCOUNT_PATH` enables real Firebase Admin SDK delivery; missing config disables FCM safely.
- Kubernetes: intentionally not implemented in this stage.

## Documentation Maintenance Rules

- Keep `README.md`, `backend/README.md`, `docs/README.md`, and `docs/interview/*` aligned with code first.
- Do not describe `APP_ENV` as active mobile configuration.
- Do not describe realtime translation as a primary current feature while its UI is hidden.
- Do not describe meeting transcription as dependent on realtime translation.
- Do not describe FCM as a placeholder; it is real when `FCM_SERVICE_ACCOUNT_PATH` is configured.
- Do not claim Kafka/Elasticsearch live smoke has run unless the optional Compose profiles were actually started.
- When adding backend entrypoints, update `backend/README.md`, `docs/interview/worker-runtime.md`, and `docs/interview/api-surface.md`.
- When adding models, outbox events, or Agent context sources, update `docs/api/database.md`, `docs/interview/system-design.md`, and `docs/interview/ai-agent-design.md`.
