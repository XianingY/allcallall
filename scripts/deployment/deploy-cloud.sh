#!/bin/bash

# AllCallAll 云服务器自动部署脚本
# Automated Cloud Deployment Script
# 使用方法: bash deploy-cloud.sh <server-ip> <domain-name>
# Usage: bash deploy-cloud.sh 81.68.168.207 api.allcall.com

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 参数检查
if [ $# -lt 1 ]; then
    echo -e "${RED}使用方法: bash deploy-cloud.sh <server-ip> [domain-name]${NC}"
    echo -e "${YELLOW}示例: bash deploy-cloud.sh 81.68.168.207 api.allcall.com${NC}"
    exit 1
fi

SERVER_IP=$1
DOMAIN_NAME=${2:-""}
WORK_DIR="/opt/allcall"

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║        🚀 AllCallAll 云服务器自动部署脚本                    ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}服务器信息:${NC}"
echo "  IP 地址: $SERVER_IP"
echo "  域名: ${DOMAIN_NAME:-'未设置'}"
echo "  工作目录: $WORK_DIR"
echo ""

# 1. 系统准备
echo -e "${BLUE}[1/8] 系统更新和软件安装...${NC}"
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl wget git net-tools htop apt-transport-https ca-certificates

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

if ! command -v docker-compose &> /dev/null; then
    sudo curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    sudo chmod +x /usr/local/bin/docker-compose
else
    echo -e "${GREEN}✓ Docker Compose 已安装${NC}"
fi

# 3. 创建项目目录
echo -e "${BLUE}[3/8] 创建项目目录...${NC}"
if [ -d "$WORK_DIR" ]; then
    echo -e "${YELLOW}⚠ 目录 $WORK_DIR 已存在${NC}"
    read -p "是否覆盖? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo rm -rf "$WORK_DIR"
    else
        echo -e "${YELLOW}跳过目录创建${NC}"
    fi
fi

sudo mkdir -p "$WORK_DIR"
sudo chown -R $USER:$USER "$WORK_DIR"

# 4. 克隆项目代码
echo -e "${BLUE}[4/8] 克隆项目代码...${NC}"
cd "$WORK_DIR"
git clone --depth 1 https://github.com/yourusername/allcall.git . || echo "使用已有的代码"

# 5. 创建环境配置文件
echo -e "${BLUE}[5/8] 创建环境配置...${NC}"

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

# Environment
APP_ENV=production
EOF

echo -e "${GREEN}✓ .env 文件已创建 (请手动编辑邮箱授权码)${NC}"

# 6. 创建生产环境配置
echo -e "${BLUE}[6/8] 创建生产环境配置...${NC}"

cat > "$WORK_DIR/backend/configs/config.yaml" << 'EOF'
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout_seconds: 10
  write_timeout_seconds: 15
  idle_timeout_seconds: 60

database:
  dsn: "allcallall:allcallallpass@tcp(mysql:3306)/allcallall_db?parseTime=true&charset=utf8mb4&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_minutes: 30

redis:
  addr: "redis:6379"
  username: ""
  password: ""
  db: 0

mail:
  host: "smtp.qq.com"
  port: 587
  username: "1569297330@qq.com"
  password: "${MAIL_PASSWORD}"
  from: "1569297330@qq.com"
  from_name: "AllCallAll"
  max_retries: 3
  retry_delay_seconds: 5

jwt:
  secret: "${JWT_SECRET}"
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

echo -e "${GREEN}✓ 生产环境配置已创建${NC}"

# 7. 配置 Nginx
echo -e "${BLUE}[7/8] 配置 Nginx...${NC}"

mkdir -p "$WORK_DIR/nginx"

if [ -n "$DOMAIN_NAME" ]; then
    # 使用 HTTPS 配置
    cat > "$WORK_DIR/nginx/nginx.conf" << EOF
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 4096;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main '\$remote_addr - \$remote_user [\$time_local] "\$request" '
                    '\$status \$body_bytes_sent "\$http_referer" '
                    '"\$http_user_agent" "\$http_x_forwarded_for"';

    access_log /var/log/nginx/access.log main;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    client_max_body_size 100M;

    gzip on;
    gzip_vary on;
    gzip_min_length 10240;
    gzip_types text/plain text/css text/xml text/javascript 
               application/x-javascript application/xml+rss 
               application/javascript application/json;

    # HTTP 重定向到 HTTPS
    server {
        listen 80;
        server_name $DOMAIN_NAME;
        return 301 https://\$server_name\$request_uri;
    }

    # HTTPS 服务器
    server {
        listen 443 ssl http2;
        server_name $DOMAIN_NAME;

        ssl_certificate /etc/letsencrypt/live/$DOMAIN_NAME/fullchain.pem;
        ssl_certificate_key /etc/letsencrypt/live/$DOMAIN_NAME/privkey.pem;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;
        ssl_prefer_server_ciphers on;

        location /api/v1/ {
            proxy_pass http://backend:8080/api/v1/;
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto \$scheme;
        }

        location /api/v1/ws {
            proxy_pass http://backend:8080/api/v1/ws;
            proxy_http_version 1.1;
            proxy_set_header Upgrade \$http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto \$scheme;
            proxy_read_timeout 86400;
        }

        location / {
            root /usr/share/nginx/html;
            try_files \$uri \$uri/ /index.html;
        }
    }
}
EOF
else
    # 使用 HTTP 配置（仅用于测试）
    cat > "$WORK_DIR/nginx/nginx.conf" << 'EOF'
user nginx;
worker_processes auto;

events {
    worker_connections 4096;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    sendfile on;
    keepalive_timeout 65;

    gzip on;
    gzip_min_length 10240;

    server {
        listen 80;
        
        location /api/v1/ {
            proxy_pass http://backend:8080/api/v1/;
            proxy_set_header Host $host;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        }

        location /api/v1/ws {
            proxy_pass http://backend:8080/api/v1/ws;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host $host;
            proxy_read_timeout 86400;
        }
    }
}
EOF
fi

echo -e "${GREEN}✓ Nginx 配置已创建${NC}"

# 8. 配置防火墙
echo -e "${BLUE}[8/8] 配置防火墙...${NC}"

if ! sudo ufw status | grep -q "active"; then
    sudo ufw --force enable
fi

sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

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
echo "2️⃣  启动服务:"
echo "   cd $WORK_DIR/infra"
echo "   docker-compose up -d"
echo ""
if [ -n "$DOMAIN_NAME" ]; then
    echo "3️⃣  获取 SSL 证书:"
    echo "   sudo certbot certonly --standalone -d $DOMAIN_NAME"
    echo ""
    echo "4️⃣  更新 DNS 记录:"
    echo "   将 $DOMAIN_NAME 的 A 记录指向 $SERVER_IP"
    echo ""
fi
echo "5️⃣  验证服务:"
echo "   curl http://$SERVER_IP:8080/api/v1/health"
echo ""
echo -e "${YELLOW}查看日志:${NC}"
echo "   cd $WORK_DIR/infra && docker-compose logs -f backend"
echo ""
echo -e "${YELLOW}更多信息请查看:${NC}"
echo "   $WORK_DIR/DEPLOYMENT_GUIDE.md"
echo ""
