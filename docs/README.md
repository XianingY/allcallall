# AllCallAll Documentation

This directory is the maintained documentation set for AllCallAll. The current project positioning is:

**Enterprise realtime collaboration system for full-stack, backend systems, and AI Agent engineering interviews.**

Historical status reports, temporary migration notes, and duplicated setup guides should not be treated as implementation authority. If a document conflicts with current code, prefer the root README, backend README, and the interview/system design docs.

## Start Here

- [Cross-Repo Index](../INDEX.md): master index covering this repo and the `allcallall-agent-runtime` sibling repo.
- [Root README](../README.md): project positioning, repo map, fast start, and runtime summary.
- [Backend README](../backend/README.md): backend entrypoints, commands, API areas, and environment variables.
- [Quick Start](./getting-started/quick-start.md): local startup flow.
- [Configuration](./configuration/configuration.md): backend, Web, mobile, and infra configuration reference.
- [FAQ](./faq.md): repository layout, local development, CI/CD, and security defaults.
- [Contributing](../CONTRIBUTING.md) and [Security Policy](../SECURITY.md): workflow and vulnerability reporting.
- [Agent Guide (AGENTS.md)](../AGENTS.md): repo map, build/test commands, and code style for coding agents.

## Interview / Portfolio

- [Interview README](./interview/README.md): suggested reading path and interview narrative.
- [Interview Chain](./interview/interview-chain.md): current Compose Agent/MCP chain, authority boundaries, approval recovery, security, and failure matrix.
- [Interview Acceptance](./interview/interview-acceptance.md): latest live smoke/chaos evidence and non-claims.
- [Five-Minute Demo Script](./interview/demo-script.md): exact Web and terminal walkthrough for the primary interview path.
- [Tencent Interview Questions](./interview/tencent-interview-questions.md): evidence-backed reference answers for Go, LangGraph, MCP, security, RAG, and frontend.
- [System Design](./interview/system-design.md): current architecture and data flow.
- [Backend Deep Dive](./interview/backend-deep-dive.md): backend module explanation.
- [AI Agent Design](./interview/ai-agent-design.md): Agent runs, tools, approvals, memory, and RAG.
- [Python Agent Runtime](./interview/python-agent-runtime.md): LangGraph runtime split, RAG Runtime split, Go/Python boundary, tool bridge, and Python eval.
- [Agentic RAG](./interview/agentic-rag.md): bounded Agentic RAG architecture, tool boundary, configuration, and eval.
- [AI Agent JD Fit](./interview/ai-portfolio-jd-fit.md): maps LangGraph, LangChain, Rerank, LlamaIndex, prompt engineering, and eval evidence to AI Agent JD requirements.
- [Tencent Full-Stack JD Fit](./interview/tencent-fullstack-jd-fit.md): maps React/Vite, Go/Gin, MySQL, Redis, Node.js tooling, admin dashboard, and performance evidence to Tencent full-stack JD requirements.
- [Agent RAG Current State](./interview/agent-rag-current-state.md): retrieval implementation notes.
- [Agent Demo Eval Report](./interview/agent-demo-report.md): reproducible planner, RAG, and workflow eval report.
- [Resume Eval](./interview/resume-eval.md): resume-safe KPI summary and generation command.
- [Agent UX Eval](./interview/agent-ux-eval.md): black-box task eval and manual pilot UX rubric.
- [Agent Task Eval Cases](./interview/agent-task-eval-cases.md): recommended task-level eval cases and resume/interview wording.
- [MCP Tool Server](./interview/mcp-tool-server.md): read-only MCP tools and repo-native AI Skill demo.
- [gRPC, Kafka, and Elasticsearch Evolution](./interview/grpc-kafka-es-evolution.md): extracted boundaries and infra extensions.
- [Worker Runtime](./interview/worker-runtime.md): embedded and standalone workers.
- [API Surface](./interview/api-surface.md): route map for interview discussion.
- [Agent Trace Example](./interview/agent-trace-example.md): sample agent trace and tool registry.
- [gRPC/Kafka/ES and Microservice Evolution](./interview/microservice-evolution.md): modular monolith to microservice evolution plan.
- [Performance Report](./interview/performance-report.md): benchmark evidence and limitations.
- [Load Test Results](./interview/load-test-results.md): historical local load-test snapshot (June 2026).
- [Troubleshooting](./interview/troubleshooting.md): agent/WebSocket/recording/CI troubleshooting.
- [Resume Bullets](./interview/resume-bullets.md): resume-ready descriptions.

## Backend / API / Data

- [API Documentation](./api/api-documentation.md): maintained API reference.
- [OpenAPI Contract](./api/openapi.yaml): OpenAPI 3.1 contract consumed by the Web client.
- [Database Notes](./api/database.md): current model/table notes.
- [Runtime Contracts](../contracts/README.md): Go/Python runtime API boundary governance (authoritative record of the 2026-07-22 Python runtime extraction).
- [Support Runbook: Meetings](./maintenance/support-runbook-meetings.md): operational support checks for rooms and recordings.
- [Worktree Artifacts](./maintenance/worktree-artifacts.md): local artifacts that must not be committed.
- [Unit Test Coverage Analysis](./unit-test-coverage-analysis.md): architecture walkthrough and test coverage inventory (2026-07-23).
- [Optimization Roadmap](./optimization-roadmap.md): P1#8 media RoomEngine 外置 + P2 架构/技术债改造方案（#15/#16/#17/#18/#24）。
- [代码审查报告](./code-review-2026-07-29.md) — 六维代码审查与分阶段修复计划

## Runtime / Deployment

- [Deployment Guide](./deployment/deployment-guide.md): production-style deployment and environment setup.
- [Agent Platform on Kubernetes](./deployment/agent-platform-kubernetes.md): Helm/K8s/OpenBao/gVisor production deployment for the agent platform.
- [Recording Storage Deployment](./deployment/recording-storage-deployment.md): local and S3-compatible recording storage.
- [Restricted Network Setup](./deployment/restricted-network-setup.md): mobile networking and TURN/restricted-network notes.
- [Meeting Room State Protocol](./deployment/meeting-room-state-protocol.md): room state event contract.
- [Web Auth Session](./deployment/web-auth-session.md): browser refresh-cookie session behavior.
- [Observability](./observability.md): optional Prometheus + Grafana + Loki stack shipped with the repo.
- [Security Guidelines](./configuration/security-guidelines.md): security and privacy rules.
- [Beta Smoke Checklist](./testing/beta-smoke-checklist.md): small-team Beta validation checklist and seed-data flow.

## Client / Development

- [Web / Desktop Workflow](./development/web-desktop-workflow.md): primary Web app and Electron wrapper workflow.
- [Web Migration Feature Matrix](./development/web-migration-feature-matrix.md): current Web coverage against migrated product surfaces.
- [Sandbox Supervisor Protocol](./development/sandbox-supervisor-protocol.md): sandbox supervisor frame protocol.
- [Mobile Docs](./mobile/README.md): current Expo native Android/iOS status.
- [Mobile Setup: App Env Usage](./mobile/setup/app-env-usage.md): `EXPO_PUBLIC_*` variable usage.
- [Mobile Setup: Audio Files](./mobile/setup/audio-files-setup.md): call audio asset placement.
- [Mobile Troubleshooting](./mobile/troubleshooting/README.md): dependency/build/environment fixes.
- [Mobile Scripts](../mobile/scripts/README.md): app-side helper scripts.
- [Web Smoke Test](./testing/web-smoke.md): browser smoke checklist.

## Supporting Docs

- [Firebase / FCM Push Notifications](./features/push-notifications/firebase-integration-guide.md): current push setup and verification guide.
- [Privacy and Account Deletion Support](./deployment/privacy-and-account-deletion-support.md): app-store support material.
- [Android Data Safety Mapping](./deployment/android-data-safety-mapping.md): Play Console data safety reference.
- [PR Description Template](./pr/pr-description-template.md): optional PR template.

## Historical Plans (execution records, not current guidance)

- [Cross-Stack Refactoring Plan (2026-06-30)](./superpowers/plans/2026-06-30-cross-stack-refactoring.md): predates the 2026-07-22 Python runtime extraction.
- [High Concurrency Implementation Plan (2026-07-07)](./superpowers/plans/2026-07-07-high-concurrency-implementation.md): tech stack section predates the runtime split.

## Python Agent / RAG Runtime (sibling repository)

The Python runtime lives in `../allcallall-agent-runtime` (must be checked out as a sibling directory). Its documentation index is `allcallall-agent-runtime/INDEX.md`, covering architecture, harness decoupling, CheckAgent loop, context compression, skill registry, MCP tools/async queue, eval methodology, and the full `PY_AGENT_*`/`PY_RAG_*` configuration reference.

## Current Runtime Summary

- Backend default: one API process with embedded workers.
- Extracted processes: `user-service`, `agent-worker`, `outbox-worker`, `data-worker`, `search-worker`, `cleanup-worker`.
- Optional infra profiles: Kafka-compatible broker and Elasticsearch.
- Agent providers: `rules`, `mock_llm`, and `openai_compatible`.
- Recording: local or S3-compatible storage.
- Primary Web: independent `web/` React + Vite app; Expo Web is no longer the production Web bundle.
- Meeting transcription: optional recording-end transcription through `TRANSCRIPTION_ENABLED=true`; `mock` is for deterministic local paths, while Beta should use `openai_compatible`.
- Realtime translation: backend code remains, but mobile UI entry points are currently hidden.
- Web config: production runtime `/config.js` generated from `PUBLIC_*`, `FIREBASE_*`, and `REVENUECAT_PUBLIC_API_KEY`.
- Mobile native config: only `EXPO_PUBLIC_*`; `APP_ENV` is historical.
- Push: `FCM_SERVICE_ACCOUNT_PATH` enables real Firebase Admin SDK delivery; Web Push additionally requires Firebase Web public config and VAPID key.
- Kubernetes: intentionally not implemented in this stage.

## Documentation Maintenance Rules

- Keep `README.md`, `backend/README.md`, `docs/README.md`, and `docs/interview/*` aligned with code first.
- Do not describe `APP_ENV` as active mobile configuration.
- Do not describe realtime translation as a primary current feature while its UI is hidden.
- Do not describe meeting transcription as dependent on realtime translation.
- Do not describe FCM as a placeholder; it is real when `FCM_SERVICE_ACCOUNT_PATH` is configured.
- Do not describe Expo Web as the primary Web client; `web/` is now authoritative for browser production.
- Do not claim Kafka/Elasticsearch live smoke has run unless the optional Compose profiles were actually started.
- When adding backend entrypoints, update `backend/README.md`, `docs/interview/worker-runtime.md`, and `docs/interview/api-surface.md`.
- When adding models, outbox events, or Agent context sources, update `docs/api/database.md`, `docs/interview/system-design.md`, and `docs/interview/ai-agent-design.md`.
