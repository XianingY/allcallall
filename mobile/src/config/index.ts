import { Platform } from "react-native";
import * as Device from "expo-device";

// 开发环境（本地）
const DEV_API = {
  HTTP: "http://192.168.31.217:8080",
  WS: "ws://192.168.31.217:8080"
};

// 生产环境（云服务器）
const PROD_API = {
  HTTP: "https://allcall.cn", // 使用你的域名或直接用 IP
  WS: "wss://allcall.cn"      // 必须是 wss://（安全 WebSocket）
};

// 或者使用公网 IP（暂时）
const PROD_API_IP = {
  HTTP: "http://81.68.168.207",
  WS: "ws://81.68.168.207"
};

// 根据环境自动检测 __DEV__ 变量值
// __DEV__ 在 Metro bundler 中会自动设置：
// - 开发模式: true (使用 npx expo start 或开发构建)
// - 生产模式: false (使用 eas build --profile production)
const __DEV__ = global.__DEV__ ?? false;

// 开发模式下的自动检测日志
if (__DEV__) {
  console.log("🚀 开发模式: 使用本地API配置");
  console.log("📡 API:", DEV_API.HTTP);
  console.log("🔌 WebSocket:", DEV_API.WS);
} else {
  console.log("🏭 生产模式: 使用云服务器API配置");
  console.log("📡 API:", PROD_API_IP.HTTP);
  console.log("🔌 WebSocket:", PROD_API_IP.WS);
}

const API_CONFIG = __DEV__ ? DEV_API : PROD_API_IP;

const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;

const API_HOST = API_CONFIG.HTTP;
const WS_HOST = API_CONFIG.WS;

export const API_BASE_URL = `${API_HOST}/api/v1`;
export const WS_URL = `${WS_HOST}/api/v1/ws`;
export const REQUEST_TIMEOUT = 10_000;
