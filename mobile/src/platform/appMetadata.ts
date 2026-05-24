import { Platform } from "react-native";
import * as Device from "expo-device";

const packageJson = require("../../package.json") as { version?: string };

const detectDesktopShell = () => {
  if (Platform.OS !== "web" || typeof navigator === "undefined") {
    return false;
  }
  return /electron/i.test(navigator.userAgent);
};

export const getPlatformTarget = (): "android" | "ios" | "web" | "desktop" => {
  if (detectDesktopShell()) {
    return "desktop";
  }
  if (Platform.OS === "android" || Platform.OS === "ios" || Platform.OS === "web") {
    return Platform.OS;
  }
  return "web";
};

export const getDeviceName = (): string => {
  if (Platform.OS === "web") {
    if (detectDesktopShell()) {
      return "Desktop";
    }
    return "Browser";
  }
  return Device.modelName?.trim() || Device.deviceName?.trim() || Platform.OS;
};

export const getAppVersion = (): string => {
  const envVersion = process.env.EXPO_PUBLIC_APP_VERSION?.trim();
  if (envVersion) {
    return envVersion;
  }
  return packageJson.version?.trim() || "0.1.0";
};
