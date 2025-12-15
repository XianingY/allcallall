#!/bin/bash

# AllCallAll 生产环境Docker Compose启动脚本
# 用于启动MySQL、Redis、Backend和Nginx服务

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目根目录
PROJECT_ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." && pwd )"
INFRA_DIR="$PROJECT_ROOT/infra"
ENV_FILE="$PROJECT_ROOT/.env"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}AllCallAll 生产环境启动脚本${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# 检查Docker是否安装
if ! command -v docker &> /dev/null; then
    echo -e "${RED}✗ Docker未安装或不在PATH中${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Docker已安装${NC}"

# 检查docker-compose
if ! command -v docker-compose &> /dev/null; then
    echo -e "${RED}✗ Docker Compose未安装${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Docker Compose已安装${NC}"

# 进入infra目录
cd "$INFRA_DIR"
echo -e "${GREEN}✓ 已进入目录: $INFRA_DIR${NC}"
echo ""

# 检查根目录 .env 文件
if [ ! -f "$ENV_FILE" ]; then
    echo -e "${RED}✗ 错误: 未找到环境配置文件: $ENV_FILE${NC}"
    echo ""
    echo "请创建 .env 文件并配置必需的环境变量"
    exit 1
fi

echo -e "${GREEN}✓ 找到环境配置文件: $ENV_FILE${NC}"

# 加载环境变量
set -a
source "$ENV_FILE"
set +a

echo -e "${GREEN}✓ 环境变量已加载${NC}"
echo ""

# 启动服务
echo -e "${YELLOW}========== 启动服务 ==========${NC}"
echo "正在启动所有服务 (MySQL, Redis, Backend, Nginx)..."
echo ""

docker-compose --env-file "$ENV_FILE" -f docker-compose.production.yml up -d

# 等待服务启动
echo ""
echo -e "${YELLOW}⏳ 等待服务启动（约30秒）...${NC}"
sleep 30

# 显示服务状态
echo ""
echo -e "${BLUE}========== 服务状态 ==========${NC}"
docker-compose -f docker-compose.production.yml ps

echo ""

# 验证服务
echo -e "${BLUE}========== 服务验证 ==========${NC}"

# 检查MySQL
echo -n "检查MySQL... "
if docker-compose -f docker-compose.production.yml exec mysql mysql -uallcallall -pallcallallpass -e "SELECT 1;" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 正常${NC}"
else
    echo -e "${RED}✗ 异常${NC}"
fi

# 检查Redis
echo -n "检查Redis... "
if docker-compose -f docker-compose.production.yml exec redis redis-cli -a redis_secure_password ping > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 正常${NC}"
else
    echo -e "${RED}✗ 异常${NC}"
fi

# 检查Backend
echo -n "检查Backend... "
if curl -s http://localhost:8080/ping > /dev/null 2>&1; then
    echo -e "${GREEN}✓ 正常${NC}"
else
    echo -e "${RED}✗ 异常 (可能仍在启动中)${NC}"
fi

echo ""
echo -e "${BLUE}========== 服务信息 ==========${NC}"
echo "MySQL:"
echo "  地址: localhost:3306"
echo "  用户: allcallall"
echo "  密码: $MYSQL_PASSWORD"
echo "  数据库: allcallall_db"
echo ""
echo "Redis:"
echo "  地址: localhost:6379"
echo "  密码: $REDIS_PASSWORD"
echo ""
echo "Backend API:"
echo "  地址: http://localhost:8080"
echo "  健康检查: http://localhost:8080/api/v1/health"
echo ""
echo "Nginx:"
echo "  HTTP: http://localhost:80"
echo "  HTTPS: https://localhost:443"
echo ""

echo -e "${BLUE}========== 常用命令 ==========${NC}"
echo "查看日志:        docker-compose --env-file $ENV_FILE -f docker-compose.production.yml logs -f"
echo "查看Backend日志: docker-compose --env-file $ENV_FILE -f docker-compose.production.yml logs -f backend"
echo "停止服务:        docker-compose --env-file $ENV_FILE -f docker-compose.production.yml stop"
echo "重启服务:        docker-compose --env-file $ENV_FILE -f docker-compose.production.yml restart"
echo "删除服务:        docker-compose --env-file $ENV_FILE -f docker-compose.production.yml down"
echo ""

echo -e "${GREEN}✅ 所有服务已启动!${NC}"
echo ""
echo -e "${BLUE}下一步:${NC}"
echo "1. 验证后端服务: curl http://localhost:8080/ping"
echo "2. 配置移动端API地址为: http://10.136.17.108:8080"
echo "3. 构建APK或使用Expo Go进行测试"
echo ""
