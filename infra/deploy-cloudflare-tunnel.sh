#!/bin/bash

#############################################################################
# AllCallAll Cloudflare Tunnel 部署脚本
# 专用于通过 Cloudflare Tunnel 在云服务器上部署 AllCallAll 应用
#
# 使用场景:
#   - 需要 Cloudflare Tunnel 代理的部署
#   - 不想直接暴露服务器 IP 的场景
#
# 使用方法:
#   chmod +x deploy-cloudflare-tunnel.sh
#   sudo ./deploy-cloudflare-tunnel.sh
#
# 注意: 如果不需要 Cloudflare Tunnel，请使用:
#   scripts/deployment/deploy-cloud.sh
#############################################################################

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 部署路径配置 (可通过环境变量覆盖)
DEPLOY_DIR="${DEPLOY_DIR:-/opt/allcallall}"

# 日志函数
log_info() {
  echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
  echo -e "${RED}[ERROR]${NC} $1"
}

#############################################################################
# 环境检查
#############################################################################

log_info "检查系统环境..."

if ! command -v docker &> /dev/null; then
  log_error "Docker 未安装"
  exit 1
fi

if ! command -v docker-compose &> /dev/null; then
  log_error "Docker Compose 未安装"
  exit 1
fi

log_info "系统环境检查完成"

#############################################################################
# 参数配置
#############################################################################

echo ""
echo "=========================================="
echo "  AllCallAll 公网部署向导"
echo "=========================================="
echo ""

# 获取用户输入
read -p "请输入你的 Cloudflare 公网域名 (例: api.allcallall.example.com): " DOMAIN
if [ -z "$DOMAIN" ]; then
  log_error "域名不能为空"
  exit 1
fi

read -s -p "请输入 MySQL 密码: " MYSQL_PASSWORD
echo ""
if [ -z "$MYSQL_PASSWORD" ]; then
  log_error "MySQL 密码不能为空"
  exit 1
fi

read -s -p "请输入 Redis 密码: " REDIS_PASSWORD
echo ""
if [ -z "$REDIS_PASSWORD" ]; then
  log_error "Redis 密码不能为空"
  exit 1
fi

read -s -p "请输入 JWT Secret (留空自动生成): " JWT_SECRET
echo ""
if [ -z "$JWT_SECRET" ]; then
  JWT_SECRET=$(openssl rand -base64 32)
  log_info "JWT Secret 已自动生成"
fi

#############################################################################
# 项目初始化
#############################################################################

log_info "初始化项目..."

# 创建 .env 文件
cat > "${DEPLOY_DIR}/infra/.env.production" << EOF
# 数据库配置
MYSQL_ROOT_PASSWORD=${MYSQL_PASSWORD}
MYSQL_PASSWORD=${MYSQL_PASSWORD}

# Redis 配置
REDIS_PASSWORD=${REDIS_PASSWORD}

# JWT 配置
JWT_SECRET=${JWT_SECRET}

# 应用配置
APP_ENV=production
EOF

log_info ".env.production 文件已创建"

#############################################################################
# 启动服务
#############################################################################

log_info "启动 Docker 容器..."

cd "${DEPLOY_DIR}/infra"

# 使用默认配置启动
docker-compose up -d

# 等待服务启动
log_info "等待服务启动..."
sleep 10

# 检查服务状态
if docker-compose ps | grep -q "healthy"; then
  log_info "✓ 所有服务已启动"
else
  log_warn "某些服务可能仍在启动中，请稍候..."
fi

log_info "Docker 容器启动完成"

#############################################################################
# Cloudflare Tunnel 配置
#############################################################################

log_info "配置 Cloudflare Tunnel..."

# 提示用户获取凭证
echo ""
echo "=========================================="
echo "  Cloudflare Tunnel 设置"
echo "=========================================="
echo ""
echo "请按照以下步骤获取 Tunnel 凭证:"
echo ""
echo "1. 访问 https://dash.cloudflare.com"
echo "2. 左侧菜单 → 访问 → Tunnel"
echo "3. 点击'创建隧道' → 选择 Cloudflared"
echo "4. 输入隧道名称: allcallall-tunnel"
echo "5. 在'Linux - 64 位'部分，复制 credentials.json 内容"
echo "6. 粘贴到下面的提示中"
echo ""
read -p "按 Enter 键继续..."

mkdir -p /etc/cloudflared
chmod 700 /etc/cloudflared

echo "请粘贴 credentials.json 内容并按 Ctrl+D 完成:"
cat > /etc/cloudflared/credentials.json
chmod 600 /etc/cloudflared/credentials.json

# 配置 cloudflared
cp "${DEPLOY_DIR}/infra/cloudflared-config.yml" /etc/cloudflared/config.yml
sed -i "s/api\.allcallall\.example\.com/${DOMAIN}/g" /etc/cloudflared/config.yml

log_info "Cloudflare Tunnel 配置已完成"

#############################################################################
# 启动 Cloudflare Tunnel 服务
#############################################################################

log_info "启动 Cloudflare Tunnel 服务..."

# 创建 systemd 服务
cat > /etc/systemd/system/cloudflared.service << 'EOF'
[Unit]
Description=Cloudflare Tunnel for AllCallAll
After=network.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/bin/cloudflared tunnel run --config /etc/cloudflared/config.yml
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable cloudflared
systemctl start cloudflared

log_info "Cloudflare Tunnel 服务已启动"

#############################################################################
# 配置备份脚本
#############################################################################

log_info "配置备份脚本..."

mkdir -p /opt/backups

cat > "${DEPLOY_DIR}/scripts/backup.sh" << 'EOF'
#!/bin/bash

BACKUP_DIR="/opt/backups"
MYSQL_PASSWORD=${MYSQL_PASSWORD}
mkdir -p $BACKUP_DIR

# 备份 MySQL
docker exec infra-mysql-1 mysqldump -u allcallall \
  --password=${MYSQL_PASSWORD} allcallall_db \
  | gzip > "$BACKUP_DIR/allcallall_db_$(date +%Y%m%d_%H%M%S).sql.gz"

# 保留最近7天的备份
find $BACKUP_DIR -name "allcallall_db_*.sql.gz" -mtime +7 -delete

echo "数据库备份完成: $(ls -lh $BACKUP_DIR | tail -1)"
EOF

chmod +x "${DEPLOY_DIR}/scripts/backup.sh"

# 添加定时任务
(crontab -l 2>/dev/null; echo "0 2 * * * ${DEPLOY_DIR}/scripts/backup.sh") | crontab -

log_info "备份脚本已配置"

#############################################################################
# 验证部署
#############################################################################

log_info "验证部署..."

echo ""
echo "=========================================="
echo "  部署验证"
echo "=========================================="
echo ""

# 检查后端服务
log_info "检查后端服务..."
if curl -s http://localhost:8080/health | grep -q "ok"; then
  log_info "✓ 后端服务正常"
else
  log_warn "✗ 后端服务异常，请检查日志: docker-compose logs backend"
fi

# 检查 Cloudflare Tunnel
log_info "检查 Cloudflare Tunnel..."
if systemctl is-active --quiet cloudflared; then
  log_info "✓ Cloudflare Tunnel 正常运行"
else
  log_warn "✗ Cloudflare Tunnel 异常，请检查日志: journalctl -u cloudflared -f"
fi

#############################################################################
# 生成配置摘要
#############################################################################

echo ""
echo "=========================================="
echo "  部署完成！"
echo "=========================================="
echo ""
echo "📋 配置信息摘要:"
echo "  - 公网域名: ${DOMAIN}"
echo "  - MySQL 用户: allcallall"
echo "  - Cloudflare Tunnel 状态: $(systemctl is-active cloudflared)"
echo ""
echo "🔗 访问地址:"
echo "  - 后端 API: https://${DOMAIN}"
echo "  - WebSocket: wss://${DOMAIN}/ws"
echo ""
echo "📊 日志查看:"
echo "  - 后端日志: docker-compose logs -f backend"
echo "  - Tunnel 日志: journalctl -u cloudflared -f"
echo ""
echo "💾 备份管理:"
echo "  - 备份目录: /opt/backups"
echo "  - 每日自动备份时间: 凌晨 2 点"
echo ""
echo "⚠️  安全提示:"
echo "  1. 修改所有默认密码（见 ${DEPLOY_DIR}/infra/.env.production）"
echo "  2. 定期备份数据库"
echo "  3. 监控服务日志"
echo "  4. 定期更新系统和依赖"
echo ""
echo "下一步:"
echo "  1. 在移动应用中更新后端地址为: https://${DOMAIN}"
echo "  2. 测试音视频通话功能"
echo "  3. 配置 DNS 记录（如使用自定义域名）"
echo ""
