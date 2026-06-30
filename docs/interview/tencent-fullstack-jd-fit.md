# Tencent Full-Stack JD Fit

This page maps AllCallAll to the Tencent campus full-stack development JD. Use it when the interview is about internal IT systems, business management platforms, React engineering, backend services, MySQL/Redis, and performance optimization.

## Positioning

AllCallAll should be presented as an enterprise collaboration and management system, not only as an AI Agent demo:

- Organization management: organizations, members, invites, teams, policies, audit events, and admin summary dashboard.
- Collaboration workspace: conversations, messages, replies, reactions, pins, attachments, notes, follow-ups, and durable realtime replay.
- Meeting workflow: browser meetings, recording lifecycle, recording transcription, transcript timeline, and meeting recap.
- Backend platform: Go/Gin APIs, Gorm/MySQL models, Redis cache/realtime/rate-limit support, outbox workers, metrics, and Docker Compose deployment.
- Engineering quality: OpenAPI generated Web client, contract checks, bundle budget checks, Vitest/MSW/Playwright tests, local benchmarks, and explicit docs.
- AI as a product enhancement: Agent meeting recap, RAG/citations, and approval-gated write tools are useful features, but the full-stack interview story should lead with product and platform completeness.

## JD Mapping

| JD requirement | Project evidence | How to explain it |
| --- | --- | --- |
| React/Vue frontend | `web/` uses React + Vite + TypeScript, React Router, TanStack Query/Table, Zustand, React Hook Form, Zod, Tailwind/Radix, Lucide, Vitest, MSW, and Playwright. | The primary Web app is an independent browser client, not Expo Web. It covers auth, organizations, collaboration, meetings, recordings, transcripts, Agent Lab, knowledge, approvals, and settings. |
| Internal IT / business management system | Organization admin console with dashboard, members, invites, teams, policies, and audit tabs. | This matches enterprise internal management systems better than a pure chat demo. The dashboard summarizes member/team/invite/conversation/approval/meeting/recording state. |
| Mainstream backend language/framework | Go backend with Gin, Gorm, MySQL, Redis, WebSocket/WebRTC signaling, and async workers; Python FastAPI is used only for Agent orchestration. | Go remains the source of truth for auth, organization isolation, permissions, audit, and writes. Python is a separate AI orchestration runtime, not the core business backend. |
| MySQL database | Gorm models cover organizations, users, teams, conversations, messages, rooms, recordings, transcript segments, Agent runs, tool approvals, knowledge sources, and audit events. | Explain table ownership and why business data stays relational: permissions, auditability, transactions, and query consistency. |
| Redis/cache middleware | Redis is used for cache, realtime tickets, rate limit paths, presence/signaling support, and metrics-backed admin summary cache behavior. | Recent optimization: organization admin summary uses a short-TTL org-scoped cache with explicit invalidation on member/invite/team/conversation/message mutations. |
| Node.js engineering experience | `scripts/web/openapi-contract-check.mjs` and `scripts/web/bundle-budget.mjs`; Web npm scripts `contract:check` and `bundle:budget`; Make targets `web-contract-check` and `web-performance-check`. | This is practical Node.js tooling around a React app: contract drift prevention and bundle budget gates, not superficial framework stacking. |
| Performance optimization | `make interview-bench`, `make dashboard-bench`, Web bundle budget check, Redis cache hit/miss metrics, and documented local benchmark boundaries. | Keep wording precise: these are local functional benchmarks and engineering evidence, not production SLA claims. |
| Learning ability / problem solving | Project evolved from mobile-first prototype to primary Web app, then added API contract generation, admin dashboard, cache, benchmarks, Python Agent runtime, and eval docs. | Emphasize tradeoffs: keep Go for stable business boundaries; use Python where AI iteration is faster; avoid unnecessary MongoDB/jQuery just to match keywords. |

## Concrete Demo Flow For This JD

1. Open the Web app and show organization management: Overview, Members, Invites, Teams, Policies, Audit.
2. Explain the backend route: `GET /api/v1/organizations/:organizationId/admin/summary`.
3. Show that the Web client uses generated OpenAPI types instead of hand-written DTOs.
4. Show Redis cache behavior:
   - first dashboard summary call is a miss;
   - second call is a hit;
   - creating an invite or team invalidates the org summary cache.
5. Open `/api/v1/metrics` and point to:
   - `admin_summary_cache_hit_total`
   - `admin_summary_cache_miss_total`
   - `admin_summary_latency_ms_sum`
   - `admin_summary_latency_ms_count`
6. Run or cite:

```bash
make dashboard-bench
make web-contract-check
make web-performance-check
```

7. If time remains, show collaboration workspace: message pagination/windowing, realtime replay, meeting recording/transcript, and Agent meeting recap.

## Performance Evidence

Current local functional benchmarks, measured on June 30, 2026 with temporary SQLite and miniredis:

| Scenario | Result | Boundary |
| --- | ---: | --- |
| Admin summary DB path | ~162 us/op | Service-level local benchmark, not production SLA |
| Admin summary Redis hit path | ~71 us/op | Short-TTL org-scoped cache, miniredis |
| Long conversation message page | ~280 us/op | 500 seeded messages, 50-message page |
| Agent/outbox benchmark | 25/25 ready runs, 0 failed, execute-run p95 6 ms | Deterministic rules provider, temporary SQLite |

Use these as reproducible engineering evidence. Do not describe them as Tencent-scale performance numbers.

## Resume Wording

Use one of these for a full-stack internship resume:

- 基于 React + Vite + TypeScript 与 Go/Gin + MySQL/Redis 构建企业协作管理系统，覆盖组织成员/团队/邀请/审计后台、会话协作、会议录音转写和审批工作流，并通过 OpenAPI 生成类型化 Web client。
- 补充企业后台性能与工程化链路：为组织管理 summary API 增加 Redis 短 TTL 缓存、显式失效和 `/metrics` 命中率指标，新增 `make dashboard-bench` 复跑本地 benchmark；同时用 Node.js 脚本做 OpenAPI contract check 和 Vite bundle budget gate。
- 在实时协作场景中实现 WebSocket 可重放事件、消息分页/窗口化渲染、组织级权限校验和审计日志；AI Agent 作为会议复盘与审批写回增强能力，而不是绕过业务系统的独立黑盒。

## Honest Boundaries

- The project does not add jQuery just for keyword matching; the frontend is modern React.
- MongoDB is not used because the current domain benefits from relational MySQL transactions and organization-scoped joins.
- Benchmarks are local functional evidence, not production capacity tests.
- Web Push, billing, large-scale SFU, SSO/SCIM, and Kubernetes are not the core Beta v1 acceptance scope.
- AI Agent features are a differentiator, but the Tencent full-stack story should primarily emphasize product workflow, frontend engineering, backend services, database/cache, tests, and performance gates.
