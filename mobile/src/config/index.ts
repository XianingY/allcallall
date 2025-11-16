import { Platform } from "react-native";
import * as Device from "expo-device";

const LAN_IP = "192.168.1.36";
const LAN_HTTP = `http://${LAN_IP}:8080`;
const LAN_WS = `ws://${LAN_IP}:8080`;

const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;

const API_HOST = isPhysicalAndroid
  ? LAN_HTTP
  : Platform.OS === "android"
  ? "http://10.0.2.2:8080"
  : "http://localhost:8080";

const WS_HOST = isPhysicalAndroid
  ? LAN_WS
  : Platform.OS === "android"
  ? "ws://10.0.2.2:8080"
  : "ws://localhost:8080";

export const API_BASE_URL = `${API_HOST}/api/v1`;
export const WS_URL = `${WS_HOST}/api/v1/ws`;

export const REQUEST_TIMEOUT = 10_000;
