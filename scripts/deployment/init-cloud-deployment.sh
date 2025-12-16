#!/bin/bash

# AllCallAll 云部署 - 初始化脚本
# Cloud Deployment - Initialization Script

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
INFRA_DIR="$PROJECT_ROOT/infra"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║        AllCallAll 云部署初始化工具                           ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# 1. 检查前置条件
echo -e "${YELLOW}[1/5] 检查前置条件...${NC}"

check_command() {
    if ! command -v $1 &> /dev/null; then
        echo -e "${RED}✗ $1 未安装${NC}"
        return 1
    else
        echo -e "${GREEN}✓ $1 已安装${NC}"
        return 0
    fi
}

check_command "docker" || exit 1
check_command "docker-compose" || exit 1
check_command "git" || exit 1
check_command "openssl" || exit 1

echo ""

# 2. 创建环境配置
echo -e "${YELLOW}[2/5] 生成环境配置文件...${NC}"

ENV_FILE="$PROJECT_ROOT/.env"

if [ -f "$ENV_FILE" ]; then
    echo -e "${YELLOW}⚠ $ENV_FILE 已存在，跳过生成${NC}"
else
    cat > "$ENV_FILE" << 'EOF'
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# AllCallAll 云部署环境配置
# Cloud Deployment Environment Variables
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

# 数据库配置
MYSQL_ROOT_PASSWORD=CHANGE_THIS_PASSWORD
MYSQL_PASSWORD=CHANGE_THIS_PASSWORD
MYSQL_USER=allcallall
MYSQL_DATABASE=allcallall_db

# Redis 配置
REDIS_PASSWORD=CHANGE_THIS_PASSWORD

# JWT 配置
JWT_SECRET=CHANGE_THIS_SECRET_KEY

# 邮箱配置
MAIL_PASSWORD=your_qq_email_auth_code

# 环境配置
APP_ENV=production
HTTP_PORT=8080

# Nginx 配置
DOMAIN_NAME=api.allcall.com
# ENABLE_HTTPS=true
EOF

    echo -e "${GREEN}✓ 环境配置文件已创建${NC}"
    echo -e "${YELLOW}⚠ 请编辑 $ENV_FILE 并修改所有密码${NC}"
fi

echo ""

# 3. 生成强密码
echo -e "${YELLOW}[3/5] 生成强密码...${NC}"

generate_password() {
    openssl rand -base64 32 | tr -d '/' | tr -d '+'
}

echo -e "${GREEN}✓ 生成 MySQL Root 密码: $(generate_password)${NC}"
echo -e "${GREEN}✓ 生成 MySQL 用户密码: $(generate_password)${NC}"
echo -e "${GREEN}✓ 生成 Redis 密码: $(generate_password)${NC}"
echo -e "${GREEN}✓ 生成 JWT Secret: $(generate_password)${NC}"

echo -e "${YELLOW}💡 将上述密码复制到 .env 文件中${NC}"

echo ""

# 4. 验证 docker-compose 配置
echo -e "${YELLOW}[4/5] 验证 Docker Compose 配置...${NC}"

cd "$INFRA_DIR"

if docker-compose config > /dev/null; then
    echo -e "${GREEN}✓ docker-compose.yml 配置有效${NC}"
else
    echo -e "${RED}✗ docker-compose.yml 配置有错误${NC}"
    exit 1
fi

echo ""

# 5. 显示后续步骤
echo -e "${YELLOW}[5/5] 显示后续步骤...${NC}"

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║        ✅ 初始化完成！                                      ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}📋 后续步骤:${NC}"
echo ""
echo "1️⃣  编辑环境配置文件:"
echo -e "   ${BLUE}nano $ENV_FILE${NC}"
echo "   (修改所有 CHANGE_THIS_* 的值)"
echo ""
echo "2️⃣  启动所有服务:"
echo -e "   ${BLUE}cd $INFRA_DIR${NC}"
echo -e "   ${BLUE}docker-compose up -d${NC}"
echo ""
echo "3️⃣  查看服务状态:"
echo -e "   ${BLUE}docker-compose ps${NC}"
echo ""
echo "4️⃣  查看后端日志:"
echo -e "   ${BLUE}docker-compose logs -f backend${NC}"
echo ""
echo "5️⃣  验证后端服务:"
echo -e "   ${BLUE}curl http://localhost:8080/api/v1/health${NC}"
echo ""
echo -e "${YELLOW}📚 完整部署指南:${NC}"
echo -e "   ${BLUE}$PROJECT_ROOT/DEPLOYMENT_GUIDE.md${NC}"
echo ""
echo -e "${YELLOW}⚠️  重要提示:${NC}"
echo "   • 所有密码必须修改为强密码"
echo "   • 不要将 .env 文件提交到 Git"
echo "   • 在生产环境启用 HTTPS"
echo "   • 定期备份数据库"
echo ""
