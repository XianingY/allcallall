import { Platform } from "react-native";
import * as Device from "expo-device";

// 默认主机（不含协议）。协议按是否允许明文通道选择：
// - 生产（非 __DEV__ 且未显式 EXPO_PUBLIC_ALLOW_INSECURE=1）：https/wss 安全默认；
// - 开发期 __DEV__ 或显式 EXPO_PUBLIC_ALLOW_INSECURE=1：保留 localhost http/ws 便利。
const DEFAULT_HOST = "127.0.0.1:8080";

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

// 是否允许明文 http/ws 通道：
// - 显式 EXPO_PUBLIC_ALLOW_INSECURE=1 时允许（本地开发）；
// - __DEV__ 下保留 localhost http 便利（见上方注释）；
// - 否则强制 https/wss（生产安全默认）。
const allowInsecure =
  readEnv(process.env.EXPO_PUBLIC_ALLOW_INSECURE) === "1" || __DEV__;

const normalizeLang = (value: string | undefined, fallback: "zh" | "en"): "zh" | "en" => {
  if (!value) return fallback;
  const lang = value.trim().toLowerCase();
  if (lang === "zh" || lang === "zh-cn" || lang === "cn") return "zh";
  if (lang === "en" || lang === "en-us" || lang === "en-gb") return "en";
  return fallback;
};

const envHttp = readEnv(process.env.EXPO_PUBLIC_API_HTTP);
const envWs = readEnv(process.env.EXPO_PUBLIC_API_WS);
const forceTls = readEnv(process.env.EXPO_PUBLIC_FORCE_TLS) === "1";
const e2eeMode = readEnv(process.env.EXPO_PUBLIC_E2EE_MODE)?.toLowerCase();

// 明文通道仅在 allowInsecure 时开启；显式 EXPO_PUBLIC_FORCE_TLS=1 仍强制升级到
// https/wss（兼容历史用法）。
const httpBase = forceTls
  ? normalizeTls(envHttp ?? (allowInsecure ? `http://${DEFAULT_HOST}` : `https://${DEFAULT_HOST}`), "http")
  : envHttp ?? (allowInsecure ? `http://${DEFAULT_HOST}` : `https://${DEFAULT_HOST}`);
const wsBase = forceTls
  ? normalizeTls(envWs ?? (allowInsecure ? `ws://${DEFAULT_HOST}` : `wss://${DEFAULT_HOST}`), "ws")
  : envWs ?? (allowInsecure ? `ws://${DEFAULT_HOST}` : `wss://${DEFAULT_HOST}`);

// 导出的配置
// Exported configuration
export const API_HOST = httpBase;
export const WS_HOST = wsBase;

export const API_BASE_URL = `${httpBase}/api/v1`;
export const WS_URL = `${wsBase}/api/v1/ws`;
export const TRANSLATION_WS_URL = `${wsBase}/api/v1/translation/ws`;
export const REQUEST_TIMEOUT = 15_000; // 15秒超时，给邮件发送更多时间

export const RESTRICTED_NETWORK_MODE = readEnv(process.env.EXPO_PUBLIC_RESTRICTED_NETWORK) === "1";
export const SIGNALING_TRANSPORT_MODE = readEnv(process.env.EXPO_PUBLIC_SIGNALING_TRANSPORT) ?? "auto";
export const SIGNALING_SHAPING_ENABLED = readEnv(process.env.EXPO_PUBLIC_SIGNALING_SHAPING) === "1";
export const E2EE_MODE = e2eeMode === "experimental" ? "experimental" : "off";
export const E2EE_ENABLED = E2EE_MODE === "experimental";

export type TranslationMode = "online";

const readTranslationMode = (value: string | undefined): TranslationMode => {
  const mode = value?.trim().toLowerCase();
  if (mode && mode !== "online") {
    console.warn(`[config] EXPO_PUBLIC_TRANSLATION_MODE=${mode} is deprecated, forcing online mode`);
  }
  return "online";
};

export const TRANSLATION_MODE: TranslationMode = readTranslationMode(
  readEnv(process.env.EXPO_PUBLIC_TRANSLATION_MODE)
);
export const TRANSLATION_SOURCE_LANG = normalizeLang(
  readEnv(process.env.EXPO_PUBLIC_TRANSLATION_SOURCE_LANG),
  "zh"
);
export const TRANSLATION_TARGET_LANG = normalizeLang(
  readEnv(process.env.EXPO_PUBLIC_TRANSLATION_TARGET_LANG),
  "en"
);

export const isPhysicalAndroid = Platform.OS === "android" && Device.isDevice;
