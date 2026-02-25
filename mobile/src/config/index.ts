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
        HTTP: "http://47.97.84.202:8080",
        WS: "ws://47.97.84.202:8080"
      };
    case 'production':
      return {
        HTTP: "http://47.97.84.202",
        WS: "ws://47.97.84.202"
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

// Expo 的 EXPO_PUBLIC_* 变量需要使用静态属性访问，不能用 process.env[key]
// Expo EXPO_PUBLIC_* variables must be accessed via static property names.
const readEnv = (value: string | undefined): string | undefined => {
  if (typeof value !== "string") return undefined;
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : undefined;
};

const normalizeTls = (value: string, scheme: "http" | "ws") => {
  if (scheme === "http") return value.replace(/^http:\/\//, "https://");
  return value.replace(/^ws:\/\//, "wss://");
};

const envHttp = readEnv(process.env.EXPO_PUBLIC_API_HTTP);
const envWs = readEnv(process.env.EXPO_PUBLIC_API_WS);
const forceTls = readEnv(process.env.EXPO_PUBLIC_FORCE_TLS) === "1";
const e2eeMode = readEnv(process.env.EXPO_PUBLIC_E2EE_MODE)?.toLowerCase();

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
console.log(`API地址(默认): ${API_CONFIG.HTTP}`);
console.log(`WebSocket(默认): ${API_CONFIG.WS}`);
console.log(`API地址(生效): ${httpBase}`);
console.log(`WebSocket(生效): ${wsBase}`);
console.log(`E2EE模式: ${e2eeMode === "experimental" ? "experimental" : "off (default)"}`);
console.log(`设备信息: ${Device.modelName} (${Platform.OS})`);
console.log('='.repeat(50));

// 导出的配置
// Exported configuration
export const API_HOST = httpBase;
export const WS_HOST = wsBase;

export const API_BASE_URL = `${httpBase}/api/v1`;
export const WS_URL = `${wsBase}/api/v1/ws`;
export const REQUEST_TIMEOUT = 15_000; // 15秒超时，给邮件发送更多时间

export const RESTRICTED_NETWORK_MODE = readEnv(process.env.EXPO_PUBLIC_RESTRICTED_NETWORK) === "1";
export const SIGNALING_TRANSPORT_MODE = readEnv(process.env.EXPO_PUBLIC_SIGNALING_TRANSPORT) ?? "auto";
export const SIGNALING_SHAPING_ENABLED = readEnv(process.env.EXPO_PUBLIC_SIGNALING_SHAPING) === "1";
export const E2EE_MODE = e2eeMode === "experimental" ? "experimental" : "off";
export const E2EE_ENABLED = E2EE_MODE === "experimental";

// 环境信息导出
// Environment information exports
export const APP_ENVIRONMENT = APP_ENV;
export const IS_DEVELOPMENT = APP_ENV === 'development';
export const IS_STAGING = APP_ENV === 'staging';
export const IS_PRODUCTION = APP_ENV === 'production';

export const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;
