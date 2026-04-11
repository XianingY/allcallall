#!/bin/bash

# AllCallAll 云服务器自动部署脚本
# Automated Cloud Deployment Script
# 使用方法: bash deploy-cloud.sh <server-ip> [domain-name] [work-dir] [repo-url]
# Usage: bash deploy-cloud.sh 121.40.22.172
#        bash deploy-cloud.sh 121.40.22.172 api.example.com /opt/allcallall git@github.com:example/AllCallAll.git

set -Eeuo pipefail

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

on_error() {
    local line="$1"
    local exit_code="$2"
    echo -e "${RED}✗ 脚本执行失败 (line=${line}, exit=${exit_code})${NC}"
}
trap 'on_error "${LINENO}" "$?"' ERR

usage() {
    echo -e "${RED}使用方法: bash deploy-cloud.sh <server-ip> [domain-name] [work-dir] [repo-url]${NC}"
    echo -e "${YELLOW}示例: bash deploy-cloud.sh 81.68.168.207 api.allcall.com /opt/myapp git@github.com:your-org/allcallall.git${NC}"
    echo -e "${YELLOW}也可通过环境变量设置: WORK_DIR=/opt/myapp REPO_URL=git@github.com:your-org/allcallall.git bash deploy-cloud.sh 81.68.168.207${NC}"
}

detect_compose_cmd() {
    if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
        echo "docker compose"
        return 0
    fi
    return 1
}

is_directory_empty() {
    local dir="$1"
    [ -z "$(find "$dir" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null)" ]
}

is_protected_path() {
    local path="$1"
    case "$path" in
        ""|"/"|"/root"|"/home"|"/opt"|"/usr"|"/var"|"/etc"|"/bin"|"/sbin"|"/lib"|"/lib64")
            return 0
            ;;
    esac
    if [ -n "${HOME:-}" ] && [ "$path" = "$HOME" ]; then
        return 0
    fi
    return 1
}

# 参数检查
if [ $# -lt 1 ]; then
    usage
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DEFAULT_REPO_URL="$(git -C "$SCRIPT_REPO_ROOT" remote get-url origin 2>/dev/null || true)"

SERVER_IP=$1
DOMAIN_NAME=${2:-""}
# 优先级: 命令行参数 > 环境变量 > 默认值
WORK_DIR=${3:-${WORK_DIR:-"/opt/allcallall"}}
REPO_URL=${4:-${REPO_URL:-"${DEFAULT_REPO_URL}"}}

if [ -z "$REPO_URL" ]; then
    echo -e "${RED}✗ 未检测到默认仓库地址，请通过第 4 个参数或 REPO_URL 显式传入仓库地址${NC}"
    usage
    exit 1
fi

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║        🚀 AllCallAll 云服务器自动部署脚本                    ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}服务器信息:${NC}"
echo "  IP 地址: $SERVER_IP"
echo "  域名: ${DOMAIN_NAME:-'未设置'}"
echo "  工作目录: $WORK_DIR"
echo "  仓库地址: $REPO_URL"
echo "  运行环境: 请在目标云服务器 SSH 会话内执行"
echo ""

# 1. 系统准备
echo -e "${BLUE}[1/8] 系统更新和软件安装...${NC}"
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl wget git net-tools htop apt-transport-https ca-certificates openssl ufw

# 2. 安装 Docker
echo -e "${BLUE}[2/8] 安装 Docker 和 Docker Compose...${NC}"
if ! command -v docker &> /dev/null; then
    curl -fsSL https://get.docker.com -o get-docker.sh
    sudo sh get-docker.sh
    rm get-docker.sh
    sudo usermod -aG docker $USER
else
    echo -e "${GREEN}✓ Docker 已安装${NC}"
fi

if ! detect_compose_cmd >/dev/null 2>&1; then
    # Docker Compose V2 插件（集成到 docker CLI）
    sudo apt install -y docker-compose-plugin || true
else
    echo -e "${GREEN}✓ Docker Compose 已安装${NC}"
fi

COMPOSE_CMD="$(detect_compose_cmd || true)"
if [ -z "$COMPOSE_CMD" ]; then
    echo -e "${RED}✗ 未找到 Docker Compose V2 命令 (docker compose)${NC}"
    exit 1
fi

# 3. 创建项目目录
echo -e "${BLUE}[3/8] 创建项目目录...${NC}"
if [ -d "$WORK_DIR" ]; then
    if [ -d "$WORK_DIR/.git" ]; then
        echo -e "${GREEN}✓ 目录 $WORK_DIR 已存在且是 git 仓库，将执行更新${NC}"
    elif is_directory_empty "$WORK_DIR"; then
        echo -e "${GREEN}✓ 目录 $WORK_DIR 已存在且为空，将直接使用${NC}"
    else
        echo -e "${YELLOW}⚠ 目录 $WORK_DIR 已存在且非空（不是 git 仓库）${NC}"
        read -p "是否清空该目录后继续? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            ABS_WORK_DIR="$(readlink -f "$WORK_DIR" 2>/dev/null || echo "$WORK_DIR")"
            if is_protected_path "$ABS_WORK_DIR"; then
                echo -e "${RED}✗ 拒绝删除高风险目录: $ABS_WORK_DIR${NC}"
                exit 1
            fi
            sudo rm -rf "$ABS_WORK_DIR"
            sudo mkdir -p "$ABS_WORK_DIR"
            sudo chown -R "$USER:$USER" "$ABS_WORK_DIR"
            WORK_DIR="$ABS_WORK_DIR"
        else
            echo -e "${RED}✗ 已取消：目录非空且不是 git 仓库，无法继续部署${NC}"
            exit 1
        fi
    fi
fi

sudo mkdir -p "$WORK_DIR"
sudo chown -R "$USER:$USER" "$WORK_DIR"

# 4. 克隆项目代码（完整仓库，包含所有分支）
echo -e "${BLUE}[4/8] 克隆项目代码...${NC}"
cd "$WORK_DIR"
if [ -d ".git" ]; then
    echo -e "${YELLOW}⚠ 已存在 git 仓库，执行 fetch 获取全部分支${NC}"
    CURRENT_REMOTE="$(git remote get-url origin 2>/dev/null || true)"
    if [ "$CURRENT_REMOTE" != "$REPO_URL" ]; then
        git remote set-url origin "$REPO_URL"
    fi
    git fetch --all --prune
else
    if ! git clone "$REPO_URL" .; then
        echo -e "${YELLOW}⚠ clone 失败（通常是 SSH key 未配置或无权限）。${NC}"
        echo -e "${YELLOW}  你可以先配置 SSH key，然后重试：git clone $REPO_URL .${NC}"
        exit 1
    fi
fi

# 5. 创建环境配置文件
echo -e "${BLUE}[5/8] 创建环境配置...${NC}"

if [ -f "$WORK_DIR/.env" ]; then
    echo -e "${YELLOW}⚠ $WORK_DIR/.env 已存在，跳过自动生成（保留现有密钥）${NC}"
else
cat > "$WORK_DIR/.env" << EOF
# Database
MYSQL_ROOT_PASSWORD=$(openssl rand -base64 32)
MYSQL_PASSWORD=$(openssl rand -base64 32)

# Redis
REDIS_PASSWORD=$(openssl rand -base64 32)

# JWT
JWT_SECRET=$(openssl rand -base64 32)

# Mail
MAIL_PASSWORD=your_qq_email_auth_code

# FCM (optional)
FCM_SERVICE_ACCOUNT_PATH=/opt/allcallall/secrets/firebase-service-account.json
EOF

echo -e "${GREEN}✓ .env 文件已创建 (请手动编辑邮箱授权码)${NC}"
fi

# docker compose 默认从当前目录读取 .env；为了在 infra/ 下执行时也能生效，创建一个软链接
ln -sf ../.env "$WORK_DIR/infra/.env" 2>/dev/null || true

# 6. 校验配置文件
echo -e "${BLUE}[6/8] 校验配置文件...${NC}"

if [ ! -f "$WORK_DIR/backend/configs/config.yaml" ]; then
    echo -e "${YELLOW}⚠ 未找到 backend/configs/config.yaml，将写入默认模板${NC}"
    mkdir -p "$WORK_DIR/backend/configs"
    cat > "$WORK_DIR/backend/configs/config.yaml" << 'EOF'
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout_seconds: 10
  write_timeout_seconds: 15
  idle_timeout_seconds: 60

database:
  # Docker Compose 会使用 DB_DSN 覆盖该字段
  dsn: ""
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_minutes: 30

redis:
  addr: "redis:6379"
  username: ""
  # Docker Compose 会使用 REDIS_PASSWORD 覆盖该字段
  password: ""
  db: 0

mail:
  host: "smtp.qq.com"
  port: 587
  username: "your_qq_email@qq.com"
  # Docker Compose 会使用 MAIL_PASSWORD 覆盖该字段
  password: ""
  from: "your_qq_email@qq.com"
  from_name: "AllCallAll"
  max_retries: 3
  retry_delay_seconds: 5

jwt:
  # Docker Compose 会使用 JWT_SECRET 覆盖该字段
  secret: ""
  issuer: "allcallall-backend"
  access_token_ttl_minutes: 60
  refresh_token_ttl_hours: 168

webrtc:
  ice_servers:
    - urls:
        - "stun:stun.l.google.com:19302"
        - "stun:stun1.l.google.com:19302"

logging:
  level: "info"
EOF
fi

echo -e "${GREEN}✓ backend/configs/config.yaml 已就绪${NC}"

# 7. 配置 Nginx (Docker Compose)
echo -e "${BLUE}[7/8] 配置 Nginx 和生产 Compose...${NC}"

NGINX_SERVER_NAME="$SERVER_IP"
if [ -n "$DOMAIN_NAME" ]; then
    NGINX_SERVER_NAME="$DOMAIN_NAME"
fi

# 将 infra/nginx.conf 的 server_name 更新为当前 server ip/domain
if [ -f "$WORK_DIR/infra/nginx.conf" ]; then
    sed -i "s/^\s*server_name\s\+.*;\s*$/    server_name ${NGINX_SERVER_NAME};/" "$WORK_DIR/infra/nginx.conf" || true
fi

echo -e "${GREEN}✓ infra/nginx.conf 已更新 (server_name=${NGINX_SERVER_NAME})${NC}"

if [ -f "$WORK_DIR/infra/docker-compose.production.yml" ]; then
    echo -e "${GREEN}✓ 检测到 infra/docker-compose.production.yml（生产端口策略就绪）${NC}"
else
    echo -e "${YELLOW}⚠ 未检测到 infra/docker-compose.production.yml，将回退使用 docker-compose.yml${NC}"
fi

# 8. 配置防火墙
echo -e "${BLUE}[8/8] 配置防火墙...${NC}"

# 先放行 SSH，避免启用防火墙时断开连接
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
if [ -n "$DOMAIN_NAME" ]; then
    sudo ufw allow 443/tcp
fi

if ! sudo ufw status | grep -q "active"; then
    sudo ufw --force enable
fi

# 显式拒绝数据库与后端直连端口，仅保留 Nginx 入口
sudo ufw deny 3306/tcp || true
sudo ufw deny 6379/tcp || true
sudo ufw deny 8080/tcp || true

echo -e "${GREEN}✓ 防火墙已配置${NC}"

echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║        ✅ 部署准备完成！                                     ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}后续步骤:${NC}"
echo ""
echo "1️⃣  编辑环境配置文件:"
echo "   nano $WORK_DIR/.env"
echo "   (修改 MAIL_PASSWORD 和其他敏感信息)"
echo ""
echo "2️⃣  启动服务:(确保在合适的分支执行)"
echo "   cd $WORK_DIR/infra"
if [ -f "$WORK_DIR/infra/docker-compose.production.yml" ]; then
    echo "   $COMPOSE_CMD -f docker-compose.production.yml up -d --build"
else
    echo "   $COMPOSE_CMD -f docker-compose.yml up -d --build"
fi
echo ""
if [ -n "$DOMAIN_NAME" ]; then
    echo "3️⃣  获取 SSL 证书:"
    echo "   sudo certbot certonly --standalone -d $DOMAIN_NAME"
    echo ""
    echo "4️⃣  更新 DNS 记录:"
    echo "   将 $DOMAIN_NAME 的 A 记录指向 $SERVER_IP"
    echo ""
fi
echo "5️⃣  验证服务:(确保先在云服务提供商打开服务器的相应端口)"
echo "   curl http://$SERVER_IP/api/v1/health"
echo ""
echo -e "${YELLOW}查看日志:${NC}"
if [ -f "$WORK_DIR/infra/docker-compose.production.yml" ]; then
    echo "   cd $WORK_DIR/infra && $COMPOSE_CMD -f docker-compose.production.yml logs -f backend"
else
    echo "   cd $WORK_DIR/infra && $COMPOSE_CMD -f docker-compose.yml logs -f backend"
fi
echo ""
echo -e "${YELLOW}更多信息请查看:${NC}"
echo "   $WORK_DIR/docs/deployment/deployment-guide.md"
echo ""
