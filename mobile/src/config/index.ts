import { Platform } from "react-native";
import * as Device from "expo-device";

// 环境配置
// Environment configuration
// 注意: Release APK 始终使用 production 配置
// Note: Release APK always uses production configuration
const APP_ENV: string = __DEV__ ? 'development' : 'production';

// 根据 APP_ENV 选择配置
// Select configuration based on APP_ENV
const getApiConfig = () => {
  switch (APP_ENV) {
    case 'development':
      return {
        HTTP: "http://192.168.1.30:8080",
        WS: "ws://192.168.1.30:8080"
      };
    case 'staging':
      return {
        HTTP: "http://81.68.168.207:8080",
        WS: "ws://81.68.168.207:8080"
      };
    case 'production':
      return {
        HTTP: "http://81.68.168.207",
        WS: "ws://81.68.168.207"
      };
    default:
      // 默认使用开发环境配置
      // Default to development configuration
      return {
        HTTP: "http://192.168.1.30:8080",
        WS: "ws://192.168.1.30:8080"
      };
  }
};

// API配置
// API Configuration
const API_CONFIG = getApiConfig();

const getEnv = (key: string): string | undefined => {
  const value = process.env[key];
  return typeof value === "string" && value.length > 0 ? value : undefined;
};

const normalizeTls = (value: string, scheme: "http" | "ws") => {
  if (scheme === "http") return value.replace(/^http:\/\//, "https://");
  return value.replace(/^ws:\/\//, "wss://");
};

const envHttp = getEnv("EXPO_PUBLIC_API_HTTP");
const envWs = getEnv("EXPO_PUBLIC_API_WS");
const forceTls = getEnv("EXPO_PUBLIC_FORCE_TLS") === "1";

const httpBase = forceTls
  ? normalizeTls(envHttp ?? API_CONFIG.HTTP, "http")
  : envHttp ?? API_CONFIG.HTTP;
const wsBase = forceTls
  ? normalizeTls(envWs ?? API_CONFIG.WS, "ws")
  : envWs ?? API_CONFIG.WS;

// 环境信息日志
// Environment information logging
const getEnvDisplayName = () => {
  switch (APP_ENV) {
    case 'development': return '🚀 开发模式';
    case 'staging': return '🧪 测试模式';
    case 'production': return '🏭 生产模式';
    default: return '❓ 未知模式';
  }
};

const getEnvDescription = () => {
  switch (APP_ENV) {
    case 'development': return '使用本地开发环境配置';
    case 'staging': return '使用测试环境配置';
    case 'production': return '使用生产环境配置';
    default: return '使用默认开发配置';
  }
};

// 输出环境信息
// Output environment information
console.log('='.repeat(50));
console.log(`📋 环境检测结果`);
console.log('='.repeat(50));
console.log(`环境类型: ${APP_ENV}`);
console.log(`显示名称: ${getEnvDisplayName()}`);
console.log(`描述: ${getEnvDescription()}`);
console.log(`API地址: ${API_CONFIG.HTTP}`);
console.log(`WebSocket: ${API_CONFIG.WS}`);
console.log(`设备信息: ${Device.modelName} (${Platform.OS})`);
console.log('='.repeat(50));

// 导出的配置
// Exported configuration
export const API_HOST = API_CONFIG.HTTP;
export const WS_HOST = API_CONFIG.WS;

export const API_BASE_URL = `${httpBase}/api/v1`;
export const WS_URL = `${wsBase}/api/v1/ws`;
export const REQUEST_TIMEOUT = 15_000; // 15秒超时，给邮件发送更多时间

export const RESTRICTED_NETWORK_MODE = getEnv("EXPO_PUBLIC_RESTRICTED_NETWORK") === "1";
export const SIGNALING_TRANSPORT_MODE = getEnv("EXPO_PUBLIC_SIGNALING_TRANSPORT") ?? "auto";
export const SIGNALING_SHAPING_ENABLED = getEnv("EXPO_PUBLIC_SIGNALING_SHAPING") === "1";

// 环境信息导出
// Environment information exports
export const APP_ENVIRONMENT = APP_ENV;
export const IS_DEVELOPMENT = APP_ENV === 'development';
export const IS_STAGING = APP_ENV === 'staging';
export const IS_PRODUCTION = APP_ENV === 'production';

export const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;
