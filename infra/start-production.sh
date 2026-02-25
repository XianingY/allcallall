#!/bin/bash

# AllCallAll 生产环境启动脚本
# Usage: bash infra/start-production.sh

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker 未安装或不在 PATH 中"
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose V2 未安装 (docker compose)"
  exit 1
fi

if [ ! -f "${PROJECT_ROOT}/.env" ]; then
  echo "未找到 ${PROJECT_ROOT}/.env，请先创建生产环境变量文件"
  exit 1
fi

# 确保在 infra/ 下执行 compose 时可读取同一份 .env
ln -sf ../.env "${SCRIPT_DIR}/.env" 2>/dev/null || true

cd "${SCRIPT_DIR}"
docker compose -f docker-compose.production.yml up -d --build
docker compose -f docker-compose.production.yml ps

echo "已启动生产服务，健康检查: curl http://localhost/api/v1/health"
