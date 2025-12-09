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
  HTTP: "http://47.109.183.99",
  WS: "ws://47.109.183.99"
};

// 根据环境选择配置
const __DEV__ = false; // 在构建时修改为 false（生产环境）

const API_CONFIG = __DEV__ ? DEV_API : PROD_API_IP;

const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;

const API_HOST = API_CONFIG.HTTP;
const WS_HOST = API_CONFIG.WS;

export const API_BASE_URL = `${API_HOST}/api/v1`;
export const WS_URL = `${WS_HOST}/api/v1/ws`;
export const REQUEST_TIMEOUT = 10_000;
