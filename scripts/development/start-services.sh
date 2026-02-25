#!/bin/bash

# AllCallAll 项目启动脚本
# Usage: ./start.sh

set -e

echo "🚀 Starting AllCallAll Project..."

# 设置 Go 代理
export GOPROXY=https://goproxy.cn,direct
echo "✅ Go proxy set to: $GOPROXY"

# 进入 infra 目录并启动数据库服务
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$PROJECT_ROOT/infra"
echo "📦 Starting Docker services (MySQL, Redis)..."
docker compose up -d mysql redis

echo ""
echo "⏳ Waiting for services to be healthy..."
sleep 5

echo ""
echo "📊 Service Status:"
docker compose ps

echo ""
echo "✅ AllCallAll is starting!"
echo ""
echo "📝 Useful commands:"
echo "  - View logs:        docker compose logs -f"
echo "  - View MySQL logs:  docker compose logs -f mysql"
echo "  - View Redis logs:  docker compose logs -f redis"
echo "  - Stop services:    docker compose down"
echo "  - Service status:   docker compose ps"
echo ""
echo "🌐 Database Services:"
echo "  - MySQL: localhost:3306"
echo "  - Redis: localhost:6379"
echo ""
echo "💡 To start backend server:"
echo "  $ cd backend && go run cmd/server/main.go"
echo ""
