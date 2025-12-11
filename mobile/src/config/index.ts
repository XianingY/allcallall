import { Platform } from "react-native";
import * as Device from "expo-device";

// 读取 .env 文件中的 APP_ENV 配置
// Read APP_ENV configuration from .env file
const getAppEnv = (): string => {
  // 从全局变量或环境变量读取
  // Read from global or environment variables
  const appEnv = (global as any).__APP_ENV__ || process.env.APP_ENV || 'development';

  // 确保值为小写
  // Ensure lowercase value
  return appEnv.toLowerCase().trim();
};

// 环境变量
// Environment variables
const APP_ENV = getAppEnv();

// 根据 APP_ENV 选择配置
// Select configuration based on APP_ENV
const getApiConfig = () => {
  switch (APP_ENV) {
    case 'development':
      return {
        HTTP: "http://192.168.31.217:8080",
        WS: "ws://192.168.31.217:8080"
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
        HTTP: "http://192.168.31.217:8080",
        WS: "ws://192.168.31.217:8080"
      };
  }
};

// API配置
// API Configuration
const API_CONFIG = getApiConfig();

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

export const API_BASE_URL = `${API_HOST}/api/v1`;
export const WS_URL = `${API_CONFIG.WS}/api/v1/ws`;
export const REQUEST_TIMEOUT = 10_000;

// 环境信息导出
// Environment information exports
export const APP_ENVIRONMENT = APP_ENV;
export const IS_DEVELOPMENT = APP_ENV === 'development';
export const IS_STAGING = APP_ENV === 'staging';
export const IS_PRODUCTION = APP_ENV === 'production';

export const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;
