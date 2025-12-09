#!/bin/bash
set -e

cd /opt/AllCallAll/infra

echo "════════════════════════════════════════════════════"
echo "🔧 重新构建后端服务"
echo "════════════════════════════════════════════════════"

# 1️⃣ 停止所有容器
echo ""
echo "✅ 步骤 1: 停止容器"
docker-compose -f docker-compose.production.yml down
echo "✓ 容器已停止"

# 2️⃣ 删除旧镜像（强制使用最新代码）
echo ""
echo "✅ 步骤 2: 删除旧镜像"
docker rmi infra-backend:latest 2>/dev/null || echo "⚠️  旧镜像不存在"
echo "✓ 旧镜像已删除"

# 3️⃣ 重新启动（会自动重新构建）
echo ""
echo "✅ 步骤 3: 重新启动容器（自动构建镜像）"
docker-compose -f docker-compose.production.yml up -d --build
echo "✓ 容器已启动"

# 4️⃣ 等待 MySQL 和后端初始化
echo ""
echo "⏳ 等待服务初始化 (40秒)..."
sleep 40

# 5️⃣ 检查容器状态
echo ""
echo "✅ 步骤 4: 检查容器状态"
docker-compose -f docker-compose.production.yml ps

# 6️⃣ 查看后端日志（最关键！）
echo ""
echo "✅ 步骤 5: 后端日志"
docker-compose -f docker-compose.production.yml logs backend --tail=30

# 7️⃣ 测试本地连接
echo ""
echo "✅ 步骤 6: 测试本地连接"
sleep 5
curl -v http://localhost:8080/api/v1/health 2>&1 | head -20 || echo "❌ 连接失败"

# 8️⃣ 测试外部连接
echo ""
echo "✅ 步骤 7: 测试外部连接"
curl -v http://81.68.168.207:8080/api/v1/health 2>&1 | head -20 || echo "❌ 连接失败"

echo ""
echo "════════════════════════════════════════════════════"
echo "✅ 重新构建完成！"
echo "════════════════════════════════════════════════════"