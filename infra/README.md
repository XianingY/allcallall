# AllCallAll Infrastructure

基础设施配置，包含 Docker 环境和部署脚本。

## 📚 文档导航

- **[部署指南](../docs/deployment/deployment-guide.md)** - 生产环境部署手册
- **[部署检查清单](../docs/deployment/deployment-checklist.md)** - 上线前检查
- **[受限网络配置](../docs/deployment/restricted-network-setup.md)** - 防火墙和 TURN 配置

## 📂 目录结构

- `docker-compose.yml`: 开发环境配置
- `docker-compose.production.yml`: 生产环境配置
- `cloudflared-config.yml`: Cloudflare Tunnel 配置
- `deploy-cloudflare-tunnel.sh`: 部署脚本

## 🚀 常用命令

```bash
# 启动开发环境
docker compose up -d

# 停止开发环境
docker compose down

# 启动生产环境
docker compose -f docker-compose.production.yml up -d
```
