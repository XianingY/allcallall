import { Platform } from "react-native";
import * as Device from "expo-device";

const LAN_IP = "192.168.31.217";
const LAN_HTTP = `http://${LAN_IP}:8080`;
const LAN_WS = `ws://${LAN_IP}:8080`;

const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;

// 使用 ADB 反向转发时，真实设备访问 localhost 即可
// When using ADB reverse, physical devices can access localhost
const API_HOST = Platform.OS === "android"
  ? "http://localhost:8080"
  : "http://localhost:8080";

const WS_HOST = Platform.OS === "android"
  ? "ws://localhost:8080"
  : "ws://localhost:8080";

export const API_BASE_URL = `${API_HOST}/api/v1`;
export const WS_URL = `${WS_HOST}/api/v1/ws`;

export const REQUEST_TIMEOUT = 10_000;
