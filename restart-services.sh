#!/bin/bash

# AllCallAll 服务重启脚本
# Restart backend and database services

set -e

PROJECT_ROOT="/Users/byzantium/github/allcall"
cd "$PROJECT_ROOT"

echo "════════════════════════════════════════════════════════════════"
echo "【AllCallAll 服务重启脚本】"
echo "════════════════════════════════════════════════════════════════"
echo ""

# 第一步：停止现有服务
echo "【步骤 1】停止现有服务..."
pkill -f "go run.*server" 2>/dev/null || true
cd "$PROJECT_ROOT/infra"
docker-compose down 2>/dev/null || true
sleep 2
echo "✅ 已停止所有服务"

echo ""
echo "【步骤 2】启动数据库 (MySQL + Redis)..."
cd "$PROJECT_ROOT/infra"
docker-compose up -d mysql redis
echo "✅ Docker 容器已启动"

echo ""
echo "【步骤 3】等待数据库就绪..."
sleep 5

# 检查 MySQL
echo "检查 MySQL..."
for i in {1..30}; do
    if nc -z localhost 3306 2>/dev/null; then
        echo "✅ MySQL 就绪"
        break
    fi
    echo "  等待 MySQL ($i/30)..."
    sleep 1
done

# 检查 Redis
echo "检查 Redis..."
for i in {1..30}; do
    if nc -z localhost 6379 2>/dev/null; then
        echo "✅ Redis 就绪"
        break
    fi
    echo "  等待 Redis ($i/30)..."
    sleep 1
done

echo ""
echo "【步骤 4】启动后端服务..."
cd "$PROJECT_ROOT/backend"

# 设置环境变量
export MAIL_PASSWORD=$(grep MAIL_PASSWORD ../.env | cut -d= -f2)
export CONFIG_PATH=./configs/config.yaml
export GOPROXY=https://goproxy.cn,direct

# 启动后端
nohup go run cmd/server/main.go > backend.log 2>&1 &
BACKEND_PID=$!
echo "✅ 后端进程已启动 (PID: $BACKEND_PID)"

echo ""
echo "【步骤 5】等待后端服务启动..."
sleep 5

# 检查后端
echo "检查后端 API..."
for i in {1..20}; do
    if curl -s http://192.168.31.217:8080/api/v1/health 2>/dev/null | grep -q "message"; then
        echo "✅ 后端服务已就绪"
        break
    fi
    echo "  等待后端启动 ($i/20)..."
    sleep 1
done

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "【服务启动完成！】"
echo "════════════════════════════════════════════════════════════════"
echo ""
echo "📊 服务状态："
echo "  MySQL: localhost:3306"
echo "  Redis: localhost:6379"
echo "  后端 API: http://192.168.31.217:8080"
echo "  后端进程 ID: $BACKEND_PID"
echo ""
echo "📋 查看日志："
echo "  后端日志: tail -f backend/backend.log"
echo "  数据库日志: cd infra && docker-compose logs -f mysql"
echo ""
echo "✅ 所有服务已就绪，可以继续真机调试！"
