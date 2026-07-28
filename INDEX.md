# AllCallAll 文档总索引 / Documentation Master Index

跨仓库总索引。AllCallAll 系统由两个 git 仓库组成：

| 仓库 | 内容 | 本地路径约定 |
| --- | --- | --- |
| `AllCallAll`（主仓，本仓库） | Go 后端、React Web、Expo Mobile、Electron Desktop、infra | — |
| `allcallall-agent-runtime`（运行时仓） | Python Agent + RAG 运行时（FastAPI + LangGraph） | 必须作为兄弟目录 `../allcallall-agent-runtime` |

维护规则：新增文档时同步更新本索引与对应仓库的局部索引（主仓
[`docs/README.md`](docs/README.md)、运行时仓 `allcallall-agent-runtime/INDEX.md`）。
`generated-*` 目录为机器生成产物，勿手工编辑。

## 入门 / Getting Started

| 标题 | 仓库 | 路径 |
| --- | --- | --- |
| 项目定位、仓库地图、快速启动 | 主仓 | [`README.md`](README.md) |
| 本地启动流程 | 主仓 | [`docs/getting-started/quick-start.md`](docs/getting-started/quick-start.md) |
| FAQ（双仓布局、开发、CI、安全默认值） | 主仓 | [`docs/faq.md`](docs/faq.md) |
| 运行时仓总览与 Quick Start | 运行时仓 | `README.md` |
| Agent Runtime 服务指南 | 运行时仓 | `services/agent-runtime/README.md` |
| RAG Runtime 服务指南 | 运行时仓 | `services/rag-runtime/README.md` |

## 规范与治理 / Governance

| 标题 | 仓库 | 路径 |
| --- | --- | --- |
| 贡献指南 | 主仓 | [`CONTRIBUTING.md`](CONTRIBUTING.md) |
| 安全策略与漏洞上报 | 主仓 | [`SECURITY.md`](SECURITY.md) |
| Coding Agent 指南（仓库地图/命令/风格） | 主仓 | [`AGENTS.md`](AGENTS.md) |
| Go/Python 运行时契约边界（迁出权威记录） | 主仓 | [`contracts/README.md`](contracts/README.md) |
| 运行时契约与 golden fixtures | 运行时仓 | `contracts/README.md` |
| PR 描述模板 | 主仓 | [`docs/pr/pr-description-template.md`](docs/pr/pr-description-template.md) |

## API 与数据 / API & Data

| 标题 | 仓库 | 路径 |
| --- | --- | --- |
| API 参考（人工维护路由总表） | 主仓 | [`docs/api/api-documentation.md`](docs/api/api-documentation.md) |
| OpenAPI 3.1 契约 | 主仓 | [`docs/api/openapi.yaml`](docs/api/openapi.yaml) |
| 数据库模型说明 | 主仓 | [`docs/api/database.md`](docs/api/database.md) |
| Tool Bridge HTTP 协议 | 运行时仓 | `docs/tool-bridge-protocol.md` |

## 配置 / Configuration

| 标题 | 仓库 | 路径 |
| --- | --- | --- |
| 后端/Web/移动端/infra 配置参考 | 主仓 | [`docs/configuration/configuration.md`](docs/configuration/configuration.md) |
| Python 运行时全量环境变量参考（`PY_AGENT_*`/`PY_RAG_*`） | 运行时仓 | `docs/configuration.md` |
| 双仓集成接线（跨服务环境变量） | 运行时仓 | `docs/allcallall-integration.md` |
| 安全指南（JWT/密码/传输） | 主仓 | [`docs/configuration/security-guidelines.md`](docs/configuration/security-guidelines.md) |

## 架构与设计 / Architecture & Design

| 标题 | 仓库 | 路径 |
| --- | --- | --- |
| 系统设计与数据流（协作/WebRTC/gRPC/Kafka/ES） | 主仓 | [`docs/interview/system-design.md`](docs/interview/system-design.md) |
| 后端模块深潜 | 主仓 | [`docs/interview/backend-deep-dive.md`](docs/interview/backend-deep-dive.md) |
| AI Agent 设计（状态机/工具/记忆/护栏） | 主仓 | [`docs/interview/ai-agent-design.md`](docs/interview/ai-agent-design.md) |
| 运行时架构（工作流/安全模型） | 运行时仓 | `docs/architecture.md` |
| 三层 Harness 解耦（调度/持久化/工具） | 运行时仓 | `docs/harness-architecture.md` |
| Loop Engineering（有界角色循环契约） | 运行时仓 | `docs/loop-engineering.md` |
| 双层 CheckAgent 质量/安全循环 | 运行时仓 | `docs/check-agents.md` |
| 分层记忆与上下文压缩 | 运行时仓 | `docs/context-compression.md` |
| Skill Registry（能力目录 + SecurityOverlay 实现） | 运行时仓 | `docs/skill-registry.md` |
| MCP 工具封装与异步任务队列 | 运行时仓 | `docs/mcp-tools-async-queue.md` |
| Agentic RAG 架构 | 主仓 | [`docs/interview/agentic-rag.md`](docs/interview/agentic-rag.md) |
| Python Agent Runtime 拆分与边界 | 主仓 | [`docs/interview/python-agent-runtime.md`](docs/interview/python-agent-runtime.md) |
| gRPC/Kafka/ES 演进 | 主仓 | [`docs/interview/grpc-kafka-es-evolution.md`](docs/interview/grpc-kafka-es-evolution.md) |
| 微服务演进计划 | 主仓 | [`docs/interview/microservice-evolution.md`](docs/interview/microservice-evolution.md) |
| Worker 运行时 | 主仓 | [`docs/interview/worker-runtime.md`](docs/interview/worker-runtime.md) |

## 部署与运维 / Deployment & Operations

| 标题 | 仓库 | 路径 |
| --- | --- | --- |
| 部署指南（本地/Staging/Beta） | 主仓 | [`docs/deployment/deployment-guide.md`](docs/deployment/deployment-guide.md) |
| Agent 平台 Kubernetes 部署（Helm/OpenBao/gVisor） | 主仓 | [`docs/deployment/agent-platform-kubernetes.md`](docs/deployment/agent-platform-kubernetes.md) |
| 录制存储部署（local/S3） | 主仓 | [`docs/deployment/recording-storage-deployment.md`](docs/deployment/recording-storage-deployment.md) |
| 受限网络/TURN 配置 | 主仓 | [`docs/deployment/restricted-network-setup.md`](docs/deployment/restricted-network-setup.md) |
| 会议房间状态协议 | 主仓 | [`docs/deployment/meeting-room-state-protocol.md`](docs/deployment/meeting-room-state-protocol.md) |
| Web 认证会话模型 | 主仓 | [`docs/deployment/web-auth-session.md`](docs/deployment/web-auth-session.md) |
| 隐私与删号支持流程 | 主仓 | [`docs/deployment/privacy-and-account-deletion-support.md`](docs/deployment/privacy-and-account-deletion-support.md) |
| Android Data Safety 映射 | 主仓 | [`docs/deployment/android-data-safety-mapping.md`](docs/deployment/android-data-safety-mapping.md) |
| 会议支持 Runbook | 主仓 | [`docs/maintenance/support-runbook-meetings.md`](docs/maintenance/support-runbook-meetings.md) |
| 本地产物清单（勿提交） | 主仓 | [`docs/maintenance/worktree-artifacts.md`](docs/maintenance/worktree-artifacts.md) |
| Infra Compose 说明 | 主仓 | [`infra/README.md`](infra/README.md) |

## 开发工作流 / Development

| 标题 | 仓库 | 路径 |
| --- | --- | --- |
| 后端 README（入口/命令/变量） | 主仓 | [`backend/README.md`](backend/README.md) |
| Web/Desktop 工作流 | 主仓 | [`docs/development/web-desktop-workflow.md`](docs/development/web-desktop-workflow.md) |
| Web 迁移功能矩阵 | 主仓 | [`docs/development/web-migration-feature-matrix.md`](docs/development/web-migration-feature-matrix.md) |
| 沙箱 Supervisor 协议 | 主仓 | [`docs/development/sandbox-supervisor-protocol.md`](docs/development/sandbox-supervisor-protocol.md) |
| Desktop（Electron）README | 主仓 | [`desktop/README.md`](desktop/README.md) |
| 脚本说明 | 主仓 | [`scripts/README.md`](scripts/README.md) |
| 负载测试脚本 | 主仓 | [`scripts/load/README.md`](scripts/load/README.md) |
| Firebase/FCM 推送集成 | 主仓 | [`docs/features/push-notifications/firebase-integration-guide.md`](docs/features/push-notifications/firebase-integration-guide.md) |

## 移动端 / Mobile

| 标题 | 仓库 | 路径 |
| --- | --- | --- |
| 移动端现状与运行配置 | 主仓 | [`docs/mobile/README.md`](docs/mobile/README.md) |
| Mobile 仓内 README | 主仓 | [`mobile/README.md`](mobile/README.md) |
| `EXPO_PUBLIC_*` 变量用法 | 主仓 | [`docs/mobile/setup/app-env-usage.md`](docs/mobile/setup/app-env-usage.md) |
| 通话音频资源配置 | 主仓 | [`docs/mobile/setup/audio-files-setup.md`](docs/mobile/setup/audio-files-setup.md) |
| 环境自动检测（已弃用说明） | 主仓 | [`docs/mobile/setup/auto-env-detection.md`](docs/mobile/setup/auto-env-detection.md) |
| 移动端排障 | 主仓 | [`docs/mobile/troubleshooting/README.md`](docs/mobile/troubleshooting/README.md) |
| 移动端脚本 | 主仓 | [`mobile/scripts/README.md`](mobile/scripts/README.md) |

## 测试与评测 / Testing & Evaluation

| 标题 | 仓库 | 路径 |
| --- | --- | --- |
| Beta 冒烟检查清单 | 主仓 | [`docs/testing/beta-smoke-checklist.md`](docs/testing/beta-smoke-checklist.md) |
| Web 冒烟清单 | 主仓 | [`docs/testing/web-smoke.md`](docs/testing/web-smoke.md) |
| 单测覆盖盘点（2026-07-23） | 主仓 | [`docs/unit-test-coverage-analysis.md`](docs/unit-test-coverage-analysis.md) |
| 评测方法论与当前证据 | 运行时仓 | `docs/eval-methodology.md` |
| 工程 Harness 与 IR 指标（HitRate@5=0.9667 / MRR=0.9083） | 运行时仓 | `docs/engineering-harness.md` |
| 简历安全指标口径 | 运行时仓 | `docs/resume-agent-metrics.md` |
| 人工试点 UX 样例（仅示意） | 运行时仓 | `docs/manual-pilot-ux-sample.md` |

## 面试资料 / Interview & Portfolio

入口与阅读顺序见 [`docs/interview/README.md`](docs/interview/README.md)。全部文件位于主仓 `docs/interview/`：

| 标题 | 路径 |
| --- | --- |
| 面试链路（Compose Agent/MCP 链、故障矩阵） | [`docs/interview/interview-chain.md`](docs/interview/interview-chain.md) |
| 验收记录（smoke/chaos） | [`docs/interview/interview-acceptance.md`](docs/interview/interview-acceptance.md) |
| 五分钟演示脚本 | [`docs/interview/demo-script.md`](docs/interview/demo-script.md) |
| 腾讯面试追问参考 | [`docs/interview/tencent-interview-questions.md`](docs/interview/tencent-interview-questions.md) |
| AI Agent JD 匹配 | [`docs/interview/ai-portfolio-jd-fit.md`](docs/interview/ai-portfolio-jd-fit.md) |
| 腾讯全栈 JD 匹配 | [`docs/interview/tencent-fullstack-jd-fit.md`](docs/interview/tencent-fullstack-jd-fit.md) |
| Agent+RAG 现状 | [`docs/interview/agent-rag-current-state.md`](docs/interview/agent-rag-current-state.md) |
| Agent Demo 评测报告入口 | [`docs/interview/agent-demo-report.md`](docs/interview/agent-demo-report.md) |
| 任务级评测用例 | [`docs/interview/agent-task-eval-cases.md`](docs/interview/agent-task-eval-cases.md) |
| Agent Trace 示例 | [`docs/interview/agent-trace-example.md`](docs/interview/agent-trace-example.md) |
| Agent UX 评测 | [`docs/interview/agent-ux-eval.md`](docs/interview/agent-ux-eval.md) |
| MCP 工具服务器 | [`docs/interview/mcp-tool-server.md`](docs/interview/mcp-tool-server.md) |
| API Surface | [`docs/interview/api-surface.md`](docs/interview/api-surface.md) |
| 性能报告 | [`docs/interview/performance-report.md`](docs/interview/performance-report.md) |
| 负载测试结果（2026-06 历史快照） | [`docs/interview/load-test-results.md`](docs/interview/load-test-results.md) |
| 简历要点 / 简历评测 | [`docs/interview/resume-bullets.md`](docs/interview/resume-bullets.md) / [`docs/interview/resume-eval.md`](docs/interview/resume-eval.md) |
| 排障手册 | [`docs/interview/troubleshooting.md`](docs/interview/troubleshooting.md) |

## 历史计划与生成产物 / Historical Plans & Generated Artifacts

| 标题 | 仓库 | 路径 | 备注 |
| --- | --- | --- | --- |
| 跨栈重构计划（2026-06-30） | 主仓 | [`docs/superpowers/plans/2026-06-30-cross-stack-refactoring.md`](docs/superpowers/plans/2026-06-30-cross-stack-refactoring.md) | 历史记录；早于 Python 运行时迁出 |
| 高并发实施计划（2026-07-07） | 主仓 | [`docs/superpowers/plans/2026-07-07-high-concurrency-implementation.md`](docs/superpowers/plans/2026-07-07-high-concurrency-implementation.md) | 历史记录 |
| 评测生成报告（多组） | 主仓 | `docs/interview/generated-*/` | 机器生成，用 `make` 目标再生 |
| Portfolio 评测生成报告 | 运行时仓 | `docs/generated-ai-agent-portfolio-eval/` | 机器生成 |

## 文档书写约定 / Conventions

- 标题：每个文档一个 H1；分节用 `##`/`###`，不跳级。
- 代码块：一律使用带语言标注的 fenced code block（```bash、```python 等）。
- 列表：统一使用 `-`；参考型内容优先使用表格。
- 单一事实源：配置变量只在配置参考文档维护（主仓 `docs/configuration/configuration.md`、运行时仓 `docs/configuration.md`），其他文档只链接不复制。
- 生成产物（`generated-*`）不手工编辑；过时的计划类文档在文首加 HISTORICAL 声明而不是删除。
