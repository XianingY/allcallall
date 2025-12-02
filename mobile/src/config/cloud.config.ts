// AllCallAll 移动应用 - 云部署环境配置
// Configuration for cloud deployment
// 
// 说明:
// - 开发环境: 使用本地 IP（192.168.31.217）通过 ADB 调试
// - 生产环境: 使用公网 IP 或域名（81.68.168.207 或 api.allcall.com）
// - 注意: 生产环境 WebSocket 必须使用 wss://（安全连接）
//
// Instructions:
// - Development: Use local IP (192.168.31.217) via ADB reverse forwarding
// - Production: Use public IP (81.68.168.207) or domain (api.allcall.com)
// - Note: Production WebSocket MUST use wss:// (secure connection)

import { Platform } from "react-native";
import * as Device from "expo-device";

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 环境配置
// Environment Configurations
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const ENV_CONFIG = {
  development: {
    // 开发环境：本地局域网 IP
    // Development: Local LAN IP
    HTTP: "http://192.168.31.217:8080",
    WS: "ws://192.168.31.217:8080"
  },
  
  staging: {
    // 测试环境：公网 IP（不使用 HTTPS）
    // Staging: Public IP (without HTTPS)
    HTTP: "http://81.68.168.207:8080",
    WS: "ws://81.68.168.207:8080"
  },
  
  production: {
    // 生产环境：使用域名 + HTTPS
    // Production: Domain name with HTTPS
    // ⚠️ 配置你自己的域名
    HTTP: "http://81.68.168.207",
    WS: "ws://81.68.168.207"
  },
  
  production_ip: {
    // 生产环境备选：直接使用公网 IP（仅用于紧急情况）
    // Production Fallback: Direct public IP (emergency only)
    HTTP: "http://81.68.168.207:8080",
    WS: "ws://81.68.168.207:8080"
  }
};

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 当前环境选择
// Current Environment Selection
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ⚠️ 构建生产版本前，必须改为 "production"
// ⚠️ MUST change to "production" before building release
const CURRENT_ENV = __DEV__ ? "development" : "production";

const API_CONFIG = ENV_CONFIG[CURRENT_ENV];

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 网络配置导出
// Network Configuration Exports
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

export const API_BASE_URL = `${API_CONFIG.HTTP}/api/v1`;
export const WS_URL = `${API_CONFIG.WS}/api/v1/ws`;
export const REQUEST_TIMEOUT = 10_000;

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 环境信息（用于调试）
// Environment Info (for debugging)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

export const ENV_INFO = {
  env: CURRENT_ENV,
  api_base: API_BASE_URL,
  ws_url: WS_URL,
  platform: Platform.OS,
  is_device: Device.isDevice,
  build_type: __DEV__ ? "development" : "production"
};

// 调试时输出环境信息
if (__DEV__) {
  console.log("🌍 Environment Configuration:");
  console.log(JSON.stringify(ENV_INFO, null, 2));
}
