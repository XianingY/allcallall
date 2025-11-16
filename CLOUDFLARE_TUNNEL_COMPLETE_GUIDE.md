# AllCallAll Cloudflare Tunnel 完整部署实施指南

本指南提供了使用 Cloudflare Tunnel 部署 AllCallAll 应用的完整步骤，包括本地开发、公网测试和生产部署。

---

## 📋 快速概览

```
┌──────────────────────────────────────┐
│   全球不同地区的移动应用用户          │
│   (不同运营商/网络环境)              │
└────────────┬─────────────────────────┘
             │
             │ HTTPS/WSS (公网加密)
             ▼
┌──────────────────────────────────────┐
│  Cloudflare Tunnel 公网域名           │
│  api.allcallall.example.com          │
│  (Cloudflare 提供免费 SSL/TLS)      │
└────────────┬─────────────────────────┘
             │
             │ cloudflared 代理
             │ (出站隧道，无需开放端口)
             ▼
┌──────────────────────────────────────┐
│   本地电脑/云服务器                  │
├──────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐ │
│  │ Go后端服务   │  │   MySQL      │ │
│  │ :8080        │  │   :3306      │ │
│  └──────────────┘  └──────────────┘ │
│  ┌──────────────┐                   │
│  │   Redis      │                   │
│  │   :6379      │                   │
│  └──────────────┘                   │
└──────────────────────────────────────┘
```

---

## ✅ 第一阶段：本地环境准备（开发机）

### 1.1 安装 Docker 和 Docker Compose

```bash
# macOS
brew install docker docker-compose

# 或使用 Docker Desktop (推荐)
# https://www.docker.com/products/docker-desktop

# 验证安装
docker --version
docker-compose --version
```

### 1.2 启动本地后端服务

```bash
cd /Users/byzantium/github/allcall/infra

# 创建开发环境配置
cat > .env.local << 'EOF'
MYSQL_ROOT_PASSWORD=devpass123
MYSQL_PASSWORD=devpass123
REDIS_PASSWORD=redispass123
JWT_SECRET=dev-secret-key-change-in-production
APP_ENV=development
EOF

# 启动所有服务（MySQL、Redis、后端）
docker-compose -f docker-compose.yml up -d

# 验证服务运行
docker-compose ps

# 检查后端健康状态
curl -s http://localhost:8080/health | jq .

# 查看后端日志
docker-compose logs -f backend
```

### 1.3 本地移动应用测试

编辑 `mobile/src/config/index.ts` 指向本地服务：

```typescript
// development 环境使用本地 IP
const DEV_CONFIG = {
  BASE_URL: 'http://192.168.1.X:8080',  // 替换为你的本地 IP
  WS_URL: 'ws://192.168.1.X:8080/ws',
  // 其他配置...
};
```

启动移动应用进行本地测试：

```bash
cd /Users/byzantium/github/allcall/mobile

# 启动 Expo
npx expo start

# 在移动设备上扫描二维码连接
```

---

## 🌍 第二阶段：Cloudflare Tunnel 配置

### 2.1 Cloudflare 账户设置

#### 步骤 1：创建 Cloudflare 账户

1. 访问 [Cloudflare 官网](https://www.cloudflare.com)
2. 点击 **注册 (Sign Up)** → 输入邮箱和密码
3. 验证邮箱 (Verify Email)（重要！）
4. 完成账户创建

#### 步骤 2：添加网站（可选，但推荐）

如果你有自己的域名：

1. 登录 Cloudflare Dashboard
2. 左侧菜单 → **网站 (Websites)** → **添加网站 (Add a Site)**
3. 输入你的域名（如 `allcallall.com`）
4. 选择 **免费套餐 (Free)**
5. 按提示修改 DNS 服务商的 Name Server (NS 记录) 指向 Cloudflare
6. 等待 DNS 传播（通常 24 小时）

**如果没有域名，不用担心** — Cloudflare 会自动分配 `xxx.cfargotunnel.com` 子域。

### 2.2 创建 Tunnel（隧道）

#### 步骤 1：在 Dashboard 创建 Tunnel

1. 登录 [Cloudflare Dashboard](https://dash.cloudflare.com)
2. 左侧菜单 → **访问 (Access)** → **Tunnel** → **隧道 (Tunnels)**
3. 点击 **创建隧道 (Create a Tunnel)**
4. 选择连接器类型 (Connector Type)：**Cloudflared**
5. 输入隧道名称 (Tunnel Name)：`allcallall-tunnel`
6. 点击 **保存隧道 (Save Tunnel)**

#### 步骤 2：下载凭证文件

1. Cloudflare Dashboard 会显示一个命令，类似：
   ```
   cloudflared tunnel run --token eyJhIjoiexx...
   ```

2. 或下载凭证 JSON 文件：
   1. 在 **隧道详情 (Tunnel Details)** 页面，向下滚动找到 **凭证 (Credentials)** 部分
   2. 点击 **下载凭证 (Download credentials)** 或 **复制凭证 Token (Copy token)**
   3. 保存 `credentials.json` 文件到安全位置（例如 `~/.cloudflared/credentials.json`）

### 2.3 配置 Tunnel 路由规则

#### 步骤 1：获取你的公网域名或子域

**方案 A：使用 Cloudflare 自动分配的子域（推荐快速测试）**
- Cloudflare 会自动分配：`allcallall-xxxx.cfargotunnel.com`

**方案 B：使用自己的域名（需要已添加到 Cloudflare）**
- 在 Dashboard 里，自定义域名为：`api.allcallall.com`

#### 步骤 2：配置 Public Hostname（公共主机名）

1. 在 Tunnel 详情页面，向下滚动找到 **Public Hostname（公共主机名）** 部分
2. 点击 **配置路由 (Configure Route)** 或 **添加公共主机名 (Add a public hostname)**
3. 配置如下：

**配置 1：API 服务**
- 子域名 (Subdomain)：`api` 或留空（如果用 cfargotunnel.com）
- 域名 (Domain)：`allcallall.com` 或 `cfargotunnel.com`
- 路径 (Path) (可选)：留空
- 协议 (Protocol)：`HTTP`
- URL：`localhost:8080`
- 点击 **保存 (Save)**

**配置 2：WebSocket 服务 (WebSocket Service)**（通常同一个端口，自动处理）
- 子域名 (Subdomain)：`api` （和 API 一样）
- 域名 (Domain)：同上
- 路径 (Path)：`/ws*` （WebSocket 路径）
- 协议 (Protocol)：`HTTP`
- URL：`localhost:8080`
- 点击 **保存 (Save)**

#### 步骤 3：获取最终的公网域名

**创建完成后，你会看到：**
```
公网域名 (Public Hostname): https://api.allcallall.cfargotunnel.com
或：                         https://api.allcallall.com (如果绑定了自己的域名)
```

**记录这个域名，后面需要用到！这是你移动应用将要连接的地址。**

---

## 🖥️ 第三阶段：本地服务器启动 Tunnel

### 3.1 在开发机上安装 cloudflared

```bash
# macOS
brew install cloudflare/cloudflare/cloudflared

# Ubuntu/Linux
wget https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb
sudo dpkg -i cloudflared-linux-amd64.deb

# 验证安装
cloudflared --version
```

### 3.2 配置 cloudflared 配置文件

编辑或创建 `~/.cloudflared/config.yml`：

```yaml
# Cloudflare Tunnel 配置文件
tunnel: allcallall-tunnel

# 凭证文件路径
credentials-file: ~/.cloudflared/credentials.json

# 日志级别
loglevel: info

# 指标和健康检查
metrics: 127.0.0.1:16010
healthcheck:
  uri: http://127.0.0.1:8080/health
  interval: 30s

# 入站规则
ingress:
  # 后端 API 服务
  - hostname: api.allcallall.example.com
    service: http://127.0.0.1:8080
    originRequest:
      httpHostHeader: api.allcallall.example.com
  
  # WebSocket 信令（通常和 API 同一个）
  - hostname: api.allcallall.example.com
    path: /ws*
    service: http://127.0.0.1:8080
    originRequest:
      httpHostHeader: api.allcallall.example.com
      websocketOriginHeader: true
  
  # 健康检查端点
  - hostname: api.allcallall.example.com
    path: /health*
    service: http://127.0.0.1:8080
  
  # 默认：404
  - service: http_status:404

# 出站连接配置
originRequest:
  connectTimeout: 30s
  tlsVersion: "1.2"
  tlsSkipVerify: false
  preserveHostHeader: true
  disableChunkedEncoding: false
```

**注意**：
- 将 `api.allcallall.example.com` 替换为你从 Cloudflare 获得的实际域名
- `credentials.json` 的路径应该指向你下载的凭证文件

### 3.3 保存凭证文件

```bash
# 创建 .cloudflared 目录
mkdir -p ~/.cloudflared
chmod 700 ~/.cloudflared

# 将下载的 credentials.json 复制到这里
cp ~/Downloads/credentials.json ~/.cloudflared/credentials.json
chmod 600 ~/.cloudflared/credentials.json
```

### 3.4 启动 Tunnel

#### 方案 A：前台运行（调试用） (Run in Foreground - for debugging)

```bash
# 测试连接
cloudflared tunnel run --config ~/.cloudflared/config.yml

# 输出应该显示：
# 2025-11-15T14:39:54Z INF Tunnel credentials have been saved
# 2025-11-15T14:39:54Z INF Registered tunnel connection...
# 2025-11-15T14:39:54Z INF Tunnel is now available...
```

#### 方案 B：后台运行（macOS） (Run in Background - macOS)

```bash
# 创建 LaunchAgent
mkdir -p ~/Library/LaunchAgents
cat > ~/Library/LaunchAgents/com.cloudflare.tunnel.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.cloudflare.tunnel</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/cloudflared</string>
        <string>tunnel</string>
        <string>run</string>
        <string>--config</string>
        <string>/Users/byzantium/.cloudflared/config.yml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/cloudflared.log</string>
    <key>StandardErrorPath</string>
    <string>/tmp/cloudflared-error.log</string>
</dict>
</plist>
EOF

# 加载并启动
launchctl load ~/Library/LaunchAgents/com.cloudflare.tunnel.plist
launchctl start com.cloudflare.tunnel

# 验证运行
launchctl list | grep cloudflare

# 查看日志
tail -f /tmp/cloudflared.log
```

#### 方案 C：后台运行（Linux） (Run in Background - Linux)

```bash
# 创建 systemd 服务
sudo tee /etc/systemd/system/cloudflared.service > /dev/null << 'EOF'
[Unit]
Description=Cloudflare Tunnel
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
User=$USER
ExecStart=/usr/local/bin/cloudflared tunnel run --config ~/.cloudflared/config.yml
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

# 启动服务
sudo systemctl daemon-reload
sudo systemctl enable cloudflared
sudo systemctl start cloudflared

# 查看状态
sudo systemctl status cloudflared
sudo journalctl -u cloudflared -f
```

### 3.5 验证 Tunnel 连接 (Verify Tunnel Connection)

```bash
# 测试 HTTPS API 连接
curl -s https://api.allcallall.cfargotunnel.com/health | jq .

# 如果返回类似以下内容，说明连接成功：
# {
#   "status": "ok"
# }

# 测试 WebSocket（需要 wscat）
npm install -g wscat
wscat -c wss://api.allcallall.cfargotunnel.com/ws
```

---

## 📱 第四阶段：移动应用配置

### 4.1 更新生产环境配置 (Update Production Configuration)

编辑 `mobile/src/config/production.ts`：

```typescript
/**
 * AllCallAll 生产环境配置 - Cloudflare Tunnel
 */

export const PRODUCTION_CONFIG = {
  // 后端 API 基础地址（使用 Cloudflare 公网域名）
  BASE_URL: 'https://api.allcallall.cfargotunnel.com',
  
  // WebSocket 信令服务地址
  WS_URL: 'wss://api.allcallall.cfargotunnel.com/ws',
  
  // 备用地址（可选）
  FALLBACK_URLS: [],
  
  // API 请求超时
  API_TIMEOUT: 30000,
  
  // WebSocket 连接超时
  WS_TIMEOUT: 10000,
  
  // WebSocket 自动重连配置
  WS_RECONNECT: {
    enabled: true,
    maxAttempts: 10,
    initialDelay: 1000,
    maxDelay: 30000,
    backoffMultiplier: 1.5,
  },
  
  // 网络质量检测
  NETWORK_CHECK: {
    enabled: true,
    interval: 30000,
  },
  
  // 日志级别
  LOG_LEVEL: 'warn',
  
  // HTTPS 证书验证（Cloudflare 提供免费 SSL）
  SSL_VERIFY: true,
  
  // STUN 服务器（NAT 穿透）
  STUN_SERVERS: [
    'stun:stun.l.google.com:19302',
    'stun:stun1.l.google.com:19302',
    'stun:stun2.l.google.com:19302',
    'stun:stun3.l.google.com:19302',
    'stun:stun4.l.google.com:19302',
  ],
  
  // TURN 服务器（可选）
  TURN_SERVERS: [],
  
  // ICE 候选收集超时
  ICE_GATHERING_TIMEOUT: 5000,
};
```

**重要**：将 `api.allcallall.cfargotunnel.com` 替换为你的实际 Cloudflare 公网域名！

### 4.2 配置 API 客户端 (Configure API Client)

编辑 `mobile/src/api/client.ts`（或相应的 HTTP 客户端）：

```typescript
import axios from 'axios';
import { PRODUCTION_CONFIG } from '../config/production';

// 创建 API 客户端
const apiClient = axios.create({
  baseURL: PRODUCTION_CONFIG.BASE_URL,
  timeout: PRODUCTION_CONFIG.API_TIMEOUT,
  // Cloudflare 提供 HTTPS，不需要额外的 CA 证书
  httpsAgent: {
    rejectUnauthorized: true,
  },
});

// 添加请求拦截器
apiClient.interceptors.request.use(
  (config) => {
    // 添加认证 token
    const token = localStorage.getItem('authToken');
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  (error) => Promise.reject(error),
);

export default apiClient;
```

### 4.3 配置 WebSocket 连接 (Configure WebSocket Connection)

编辑 `mobile/src/services/signaling.ts`（或相应的 WebSocket 服务）：

```typescript
import { PRODUCTION_CONFIG } from '../config/production';

export class SignalingService {
  private ws: WebSocket | null = null;
  private reconnectAttempts = 0;
  private reconnectTimer: NodeJS.Timeout | null = null;
  
  connect(token: string): Promise<void> {
    return new Promise((resolve, reject) => {
      try {
        // 使用 Cloudflare 公网域名连接
        this.ws = new WebSocket(
          `${PRODUCTION_CONFIG.WS_URL}?token=${token}`
        );
        
        this.ws.onopen = () => {
          console.log('WebSocket connected to Cloudflare Tunnel');
          this.reconnectAttempts = 0;
          
          // 启动心跳保活（Cloudflare 100 秒超时）
          this.startHeartbeat();
          
          resolve();
        };
        
        this.ws.onerror = (error) => {
          console.error('WebSocket error:', error);
          reject(error);
        };
        
        this.ws.onclose = () => {
          console.warn('WebSocket disconnected');
          this.stopHeartbeat();
          
          // 自动重连
          if (PRODUCTION_CONFIG.WS_RECONNECT.enabled) {
            this.reconnect(token);
          }
        };
        
        this.ws.onmessage = (event) => {
          this.handleMessage(event.data);
        };
      } catch (error) {
        reject(error);
      }
    });
  }
  
  // 心跳保活（防止 Cloudflare 100 秒空闲超时）
  private heartbeatTimer: NodeJS.Timeout | null = null;
  
  private startHeartbeat() {
    this.heartbeatTimer = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: 'ping' }));
      }
    }, 30000); // 每 30 秒发送一次 ping
  }
  
  private stopHeartbeat() {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }
  
  // 自动重连逻辑
  private reconnect(token: string) {
    const config = PRODUCTION_CONFIG.WS_RECONNECT;
    
    if (this.reconnectAttempts >= config.maxAttempts) {
      console.error('Max reconnect attempts reached');
      return;
    }
    
    this.reconnectAttempts++;
    const delay = Math.min(
      config.initialDelay * Math.pow(config.backoffMultiplier, this.reconnectAttempts),
      config.maxDelay,
    );
    
    console.log(`Attempting to reconnect in ${delay}ms...`);
    
    this.reconnectTimer = setTimeout(() => {
      this.connect(token).catch(console.error);
    }, delay);
  }
  
  disconnect() {
    this.stopHeartbeat();
    
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
  
  private handleMessage(data: string) {
    try {
      const message = JSON.parse(data);
      // 处理信令消息
      console.log('Received message:', message);
    } catch (error) {
      console.error('Failed to parse message:', error);
    }
  }
}
```

### 4.4 打包生产版本 (Package Production Build)

#### iOS (iOS App)

```bash
cd /Users/byzantium/github/allcall/mobile

# 使用 EAS 构建（推荐） (Build with EAS - Recommended)
eas login
eas build --platform ios --auto-submit

# 或本地构建 (Or build locally)
cd ios
xcodebuild -configuration Release -scheme AllCallAll
```

#### Android (Android App)

```bash
cd /Users/byzantium/github/allcall/mobile

# 使用 EAS 构建 (Build with EAS)
eas build --platform android --auto-submit

# 或本地构建 (Or build locally)
cd android
./gradlew assembleRelease
```

---

## 🔍 第五阶段：测试与验证 (Phase 5: Testing & Verification)

### 5.1 测试 API 连接 (Test API Connection)

```bash
# 替换为你的实际域名
DOMAIN="api.allcallall.cfargotunnel.com"

# 测试 HTTPS API
curl -v https://${DOMAIN}/health

# 测试 WebSocket（需要 wscat）
npm install -g wscat
wscat -c wss://${DOMAIN}/ws

# 发送测试消息
Connected (press CTRL+C to quit)
> {"type":"ping"}
< {"type":"pong"}
```

### 5.2 测试移动应用 (Test Mobile App)

1. **本地 WiFi 测试**
   - 确保手机和开发机在同一 WiFi 网络
   - 安装应用并配置为连接到 Cloudflare Tunnel 域名
   - 进行音视频通话测试

2. **不同网络测试**
   - 用 4G/5G 网络测试应用
   - 验证跨运营商连接是否正常

3. **WebSocket 连接测试**
   - 在应用中观察信令服务日志
   - 检查是否有连接错误或超时

### 5.3 性能监控 (Performance Monitoring)

```bash
# 监控 Tunnel 连接状态
tail -f /tmp/cloudflared.log

# 查看后端日志
docker-compose logs -f backend

# 检查网络延迟
ping api.allcallall.cfargotunnel.com

# 检查 WebSocket 连接
curl -v -H "Connection: upgrade" \
     -H "Upgrade: websocket" \
     https://api.allcallall.cfargotunnel.com/ws
```

---

## 🛠️ 第六阶段：故障排查

### 问题 1：Tunnel 无法连接 (Problem 1: Tunnel Cannot Connect)

**症状**：
```
Error: Failed to connect to Cloudflare edge
```

**解决步骤**：
```bash
# 1. 检查 cloudflared 是否运行
ps aux | grep cloudflared

# 2. 验证凭证文件是否存在和有效
ls -la ~/.cloudflared/credentials.json

# 3. 重新启动 Tunnel
# macOS:
launchctl stop com.cloudflare.tunnel
launchctl start com.cloudflare.tunnel

# Linux:
sudo systemctl restart cloudflared

# 4. 查看详细日志
tail -f /tmp/cloudflared.log
# 或
sudo journalctl -u cloudflared -f
```

### 问题 2：移动应用无法连接后端 (Problem 2: Mobile App Cannot Connect to Backend)

**症状**：
- API 请求失败，提示连接超时
- WebSocket 连接失败

**检查清单**：
```bash
# 1. 验证本地后端服务是否运行
curl -s http://localhost:8080/health | jq .

# 2. 验证 Cloudflare Tunnel 是否连接
curl -s https://api.allcallall.cfargotunnel.com/health | jq .

# 3. 检查 cloudflared 配置是否正确
cat ~/.cloudflared/config.yml

# 4. 查看后端日志中是否有错误
docker-compose logs backend | tail -50

# 5. 检查防火墙规则
# macOS: System Preferences → Security & Privacy → Firewall
# 允许 cloudflared 通过防火墙
```

### 问题 3：WebSocket 连接超时 (Problem 3: WebSocket Connection Timeout)

**症状**：
```
WebSocket connection failed after 10 seconds
```

**原因和解决**：

1. **Cloudflare Tunnel 的 100 秒空闲超时**
   - 原因：长时间没有传输数据
   - 解决：实现心跳机制（上面的代码已包含）

2. **防火墙阻止 WebSocket**
   - 原因：ISP 或防火墙限制
   - 解决：Cloudflare 已处理，应该不会发生

3. **应用服务器问题**
   - 原因：后端 WebSocket 处理逻辑有问题
   - 解决：检查后端日志和 WebSocket 实现

### 问题 4：HTTPS 证书错误 (Problem 4: HTTPS Certificate Error)

**症状**：
```
SSL certificate problem: self signed certificate
```

**解决**：
```typescript
// 不要禁用证书验证！
// ❌ 错误做法：
https.Agent({ rejectUnauthorized: false }),

// ✅ 正确做法：
https.Agent({ rejectUnauthorized: true }), // Cloudflare 提供有效的 SSL
```

Cloudflare 提供免费的有效 SSL 证书，无需任何额外配置。

### 问题 5：应用体验不稳定 (Problem 5: Unstable App Experience)

**症状**：
- 延迟大
- 音视频卡顿
- 经常断线

**优化步骤**：

1. **检查网络状况**
   ```bash
   # 测试延迟
   ping api.allcallall.cfargotunnel.com
   
   # 应该 < 100ms（国内）或 < 200ms（国外）
   ```

2. **增加 TURN 服务器**
   ```typescript
   // 在 production.ts 中配置 TURN
   TURN_SERVERS: [
     {
       urls: 'turn:turn.example.com:3478',
       username: 'user',
       credential: 'pass',
     },
   ],
   ```

3. **调整 WebRTC 编码参数**
   ```typescript
   // 降低视频码率以适应弱网
   CODEC_CONFIG: {
     video: {
       bitrate: 500000, // 500kbps（从默认 1Mbps 降低）
     },
   },
   ```

4. **增加重连尝试次数**
   ```typescript
   WS_RECONNECT: {
     maxAttempts: 20, // 增加尝试次数
   },
   ```

---

## 📊 监控与日志 (Monitoring & Logging)

### 监控 Tunnel 状态 (Monitor Tunnel Status)

```bash
# 实时查看 Tunnel 日志
tail -f /tmp/cloudflared.log

# 查看 Tunnel 指标
curl http://127.0.0.1:16010/metrics
```

### 监控后端服务 (Monitor Backend Service)

```bash
# 查看后端日志
docker-compose logs -f backend

# 查看所有容器状态
docker-compose ps

# 查看资源使用情况
docker stats
```

### 性能基准 (Performance Benchmark)

在良好的网络条件下，应该达到：
- **API 响应时间**：< 500ms
- **WebSocket 连接建立时间**：< 1s
- **音视频延迟**：< 1s
- **丢包率**：< 1%

---

## 🔐 安全建议 (Security Recommendations)

### 1. 生产环境密钥管理

```bash
# 修改 .env 中的所有默认密钥
MYSQL_PASSWORD=your_secure_password_$(openssl rand -base64 32)
REDIS_PASSWORD=your_secure_password_$(openssl rand -base64 32)
JWT_SECRET=$(openssl rand -base64 32)
```

### 2. 域名和 DNS 安全 (Domain & DNS Security)

- 使用 Cloudflare 提供的 DNS（自动配置）
- 启用 Cloudflare 的 DDoS 保护（免费版已包含）

### 3. 应用层安全 (Application-Level Security)

```typescript
// 启用 HTTPS 证书验证
SSL_VERIFY: true,

// 添加请求验证
apiClient.interceptors.request.use((config) => {
  // 验证请求来源
  // 添加安全头
  return config;
});
```

### 4. 日志和审计 (Logging & Audit)

```bash
# 定期检查日志中的异常
docker-compose logs backend | grep ERROR
tail -f /tmp/cloudflared.log | grep error
```

---

## 📝 部署检查清单

- [ ] Docker 和 Docker Compose 已安装
- [ ] 本地后端服务运行正常（`http://localhost:8080/health`）
- [ ] Cloudflare 账户已创建
- [ ] Tunnel 已创建（`allcallall-tunnel`）
- [ ] Tunnel 凭证文件已下载和保存
- [ ] cloudflared 已安装
- [ ] `~/.cloudflared/config.yml` 已配置
- [ ] Tunnel 正在运行并已连接到 Cloudflare
- [ ] HTTPS API 可访问（`curl https://api.allcallall.cfargotunnel.com/health`）
- [ ] WebSocket 可连接（wscat 测试）
- [ ] 移动应用配置已更新为 Cloudflare 域名
- [ ] 生产版本已打包
- [ ] 已进行跨网络测试（4G/5G）
- [ ] 后端日志已验证
- [ ] 所有默认密钥已修改

---

## 💰 成本分析 (Cost Analysis)

| 项目 | 成本 |
|------|------|
| Cloudflare Tunnel | **免费 (Free)** ✅ |
| Cloudflare SSL 证书 | **免费 (Free)** ✅ |
| 域名 (可选) (Domain - Optional) | ¥50-100/年 (¥2.5-5/month) |
| 云服务器或家庭网络 (Cloud Server or Home Network) | 取决于选择 (Depends) |
| **总计 (Total)** | **免费-¥50/年 (Free - ¥2.5/month)** |

---

## 获取帮助

遇到问题时：

1. **检查官方文档**：https://developers.cloudflare.com/cloudflare-one/connections/connect-applications/

2. **查看日志**：
   ```bash
   # Tunnel 日志
   tail -f /tmp/cloudflared.log
   
   # 后端日志
   docker-compose logs backend
   ```

3. **Cloudflare 状态页面**：https://www.cloudflarestatus.com

4. **提交问题**：在 AllCallAll GitHub 仓库提交 Issue

---

## 总结 (Summary)

通过本指南，你可以：
- ✅ 在本地开发和测试 AllCallAll (Develop and test AllCallAll locally)
- ✅ 免费部署到公网（Cloudflare Tunnel）(Deploy to public internet for free)
- ✅ 实现不同网络用户的音视频通话 (Enable video calls between users on different networks)
- ✅ 获得自动 HTTPS 和 DDoS 保护 (Automatic HTTPS and DDoS protection)
- ✅ 轻松扩展到多台服务器 (Easy to scale to multiple servers)

祝部署顺利！🚀
