> English documentation is canonical. See [README.md](README.md).

# AllCallAll 简介

AllCallAll 是一个开源的实时协作与 AI Agent 平台，将团队会话、会议、录制、
转写、知识检索、审批式自动化和管理能力整合在同一套可自托管系统中。

## 核心能力

- Go 后端负责用户、组织、会话、会议、权限、审批、审计和所有业务写入。
- `web/` 是 React + Vite 的生产 Web 客户端。
- `mobile/` 是 Expo Android/iOS 原生客户端。
- `desktop/` 是围绕 Web 客户端构建的轻量 Electron 外壳。
- 独立的
  [`allcallall-agent-runtime`](https://github.com/XianingY/allcallall-agent-runtime)
  仓库负责 LangGraph 编排、RAG、重排、引用、工具提案和评测。

## 快速开始

准备 Go 1.26、Node.js 24、npm 和 Docker Compose，然后在仓库根目录执行：

```bash
npm ci
./scripts/development/start-services.sh
```

启动后端：

```bash
cd backend
CONFIG_PATH=./configs/config.yaml go run ./cmd/server
```

另开终端启动 Web：

```bash
cd web
npm run dev
```

更完整的本地、移动端、桌面端及 Agent Runtime 配置，请阅读
[快速开始](docs/getting-started/quick-start.md)。

## 文档与参与

- [文档索引](docs/README.md)
- [贡献指南](CONTRIBUTING.md)
- [安全策略](SECURITY.md)
- [支持方式](SUPPORT.md)
- [行为准则](CODE_OF_CONDUCT.md)

项目仍处于 1.0 之前的活跃开发阶段。代码以 [MIT License](LICENSE) 发布。
