## AllCallAll 从本地开发到公网部署的迁移指南

本指南说明如何将 AllCallAll 从局域网开发环境迁移到公网生产环境。

---

## 步骤 1：服务器部署（云端）

### 1.1 购买云服务器

推荐选项：
- **Vultr** / **DigitalOcean** / **Linode** / **腾讯云** / **阿里云**
- 配置：2核 CPU, 2GB RAM, 20GB SSD
- 操作系统：Ubuntu 22.04 LTS

### 1.2 在服务器上执行部署

```bash
# SSH 连接到服务器
ssh root@your_server_ip

# 克隆项目
cd /opt
git clone https://github.com/XianingY/AllCallAll.git
cd AllCallAll/infra

# 启动部署脚本
chmod +x deploy.sh
sudo ./deploy.sh
```

脚本会自动：
- ✅ 安装 Docker 和 Docker Compose
- ✅ 启动 MySQL、Redis、后端服务
- ✅ 配置 Cloudflare Tunnel
- ✅ 设置备份计划
- ✅ 启动监控和日志

---

## 步骤 2：Cloudflare Tunnel 配置

### 2.1 创建 Cloudflare 账户

1. 访问 [Cloudflare 官网](https://www.cloudflare.com)
2. 注册账户（免费）
3. 添加你的域名（或使用免费的 cfargotunnel.com 子域）

### 2.2 创建 Tunnel

1. 登录 Cloudflare Dashboard
2. 左侧菜单 → **Access** → **Tunnels**
3. 点击 **Create a tunnel**
4. 选择 **Cloudflared**
5. 输入名称：`allcallall-tunnel`
6. 在部署脚本的提示中粘贴凭证

### 2.3 配置路由

在 Cloudflare Dashboard 中，配置以下路由：

| 公网域名 | 本地地址 | 说明 |
|---------|--------|------|
| api.allcallall.example.com | http://localhost:8080 | 后端 API |
| api.allcallall.example.com/ws* | http://localhost:8080 | WebSocket 信令 |

---

## 步骤 3：更新移动应用

### 3.1 修改后端地址配置

编辑 `mobile/src/config/index.ts`：

```typescript
import { PRODUCTION_CONFIG } from './production';

const isDevelopment = __DEV__;

const config = isDevelopment 
  ? {
      BASE_URL: `http://${LAN_IP}:8080`,
      WS_URL: `ws://${LAN_IP}:8080/ws`,
    }
  : PRODUCTION_CONFIG;

export default config;
```

### 3.2 验证 HTTPS 配置

```typescript
// src/api/client.ts
const client = axios.create({
  baseURL: config.BASE_URL,
  httpsAgent: {
    rejectUnauthorized: true,
  },
});
```

### 3.3 打包并发布

```bash
cd mobile

# 生产构建
eas build --platform android --auto-submit
```

---

## 步骤 4：DNS 配置（可选）

如果使用自定义域名：

### 4.1 在 Cloudflare 中添加 DNS 记录

```
名称: api
值: allcallall-tunnel.cfargotunnel.com
代理状态: 仅 DNS
```

### 4.2 验证 DNS

```bash
nslookup api.allcallall.example.com
```

---

## 步骤 5：测试和验证

### 5.1 测试后端连接

```bash
curl -X GET https://api.allcallall.example.com/health
```

### 5.2 测试 WebSocket 连接

```bash
websocat wss://api.allcallall.example.com/ws
```

### 5.3 在移动设备上测试

1. 在不同网络下安装应用
2. 测试用户注册和登录
3. 测试音视频通话

---

## 步骤 6：监控和维护

### 6.1 日常监控

```bash
# 查看后端日志
docker-compose -f docker-compose.production.yml logs -f backend

# 查看 Tunnel 日志
journalctl -u cloudflared -f

# 检查系统资源
docker stats
```

### 6.2 定期备份

```bash
# 手动备份
/opt/AllCallAll/scripts/backup.sh

# 查看备份文件
ls -lh /opt/backups/
```

### 6.3 更新和升级

```bash
cd /opt/AllCallAll
git pull origin main

cd infra
docker-compose -f docker-compose.production.yml up -d --build
```

---

## 常见问题

### Q1: 移动应用无法连接后端

**检查清单：**
1. 验证公网地址：`curl https://api.allcallall.example.com/health`
2. 检查 Tunnel：`systemctl status cloudflared`
3. 查看后端日志：`docker-compose logs backend`
4. 确认应用配置的地址正确
5. 检查 CORS 设置

### Q2: WebSocket 连接超时

**原因和解决：**
- ISP 阻止 WebSocket → 使用 HTTP 长轮询
- 防火墙规则 → 检查安全组
- Cloudflare 限制 → 升级计划

### Q3: 音视频质量差

**优化步骤：**
1. 检查网络带宽
2. 增加 TURN 服务器
3. 调整编码参数

---

## 生产环境检查清单

- [ ] 云服务器已创建
- [ ] Docker 已安装
- [ ] Cloudflare 隧道已配置
- [ ] 后端服务运行正常
- [ ] HTTPS/WSS 连接正常
- [ ] 移动应用配置已更新
- [ ] 生产密钥已修改
- [ ] 备份脚本已配置
- [ ] 监控告警已设置
- [ ] 已在真实设备上测试

---

## 成本估算

| 项目 | 月度成本 |
|------|---------|
| Cloudflare Tunnel（免费） | $0 |
| VPS 服务器（2核 2GB） | $5-15 |
| 域名（可选） | $10-15 |
| **总计** | **$15-30/月** |

---

祝部署顺利！如有问题，欢迎反馈。
