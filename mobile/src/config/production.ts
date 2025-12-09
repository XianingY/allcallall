/**
 * AllCallAll 生产环境配置
 * 
 * 使用 Cloudflare Tunnel 公网域名
 * 
 * 使用方法：
 * import { PRODUCTION_CONFIG } from './production';
 */

export const PRODUCTION_CONFIG = {
  // 后端 API 基础地址（使用 Cloudflare 域名）
  BASE_URL: 'https://api.allcallall.example.com',
  
  // WebSocket 信令服务地址
  WS_URL: 'wss://api.allcallall.example.com/ws',
  
  // 灾备地址（可选）
  FALLBACK_URLS: [
    'https://api-backup.allcallall.example.com',
  ],
  
  // API 请求超时时间（毫秒）
  API_TIMEOUT: 30000,
  
  // WebSocket 连接超时（毫秒）
  WS_TIMEOUT: 10000,
  
  // WebSocket 自动重连配置
  WS_RECONNECT: {
    enabled: true,
    maxAttempts: 10,
    initialDelay: 1000,      // 初始延迟 1 秒
    maxDelay: 30000,         // 最大延迟 30 秒
    backoffMultiplier: 1.5,  // 指数退避倍数
  },
  
  // 网络质量检测
  NETWORK_CHECK: {
    enabled: true,
    interval: 30000, // 每 30 秒检查一次
  },
  
  // 日志级别
  LOG_LEVEL: 'warn',
  
  // HTTPS 证书验证（生产环境必须启用）
  SSL_VERIFY: true,
  
  // STUN 服务器配置（用于 NAT 穿透）
  STUN_SERVERS: [
    'stun:stun.l.google.com:19302',
    'stun:stun1.l.google.com:19302',
    'stun:stun2.l.google.com:19302',
    'stun:stun3.l.google.com:19302',
    'stun:stun4.l.google.com:19302',
  ],
  
  // TURN 服务器配置（可选，用于绕过严格 NAT）
  TURN_SERVERS: [
    // {
    //   urls: 'turn:turn.allcallall.example.com:3478',
    //   username: 'user',
    //   credential: 'password',
    // },
  ],
  
  // ICE 候选地址收集超时
  ICE_GATHERING_TIMEOUT: 5000,
  
  // WebRTC 编码器配置
  CODEC_CONFIG: {
    audio: {
      opus: {
        bitrate: 24000, // 24kbps
        sampleRate: 48000,
      },
    },
    video: {
      h264: {
        profile: 'main',
        level: '3.1',
      },
    },
  },
  
  // 性能监控
  PERFORMANCE: {
    enabled: true,
    reportInterval: 60000, // 每分钟上报一次
  },
  
  // 错误上报
  ERROR_REPORTING: {
    enabled: true,
    endpoint: 'https://api.allcallall.example.com/errors',
  },
};
