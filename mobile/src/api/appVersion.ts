import Constants from "expo-constants";
import { Platform } from "react-native";

import { createApiClient } from "./client";

/**
 * Asks the backend whether this build may keep running.
 *
 * The server holds a per-platform policy (see backend's app_version_handler.go).
 * A build below the supported floor is refused, which is the only mechanism
 * that can guarantee a client actually picked up a release - useful when a
 * release contains a security fix or a protocol change.
 */
import type { AppVersionCheck } from "../versionGate";

export type { AppVersionCheck };

/** The version this build reports. Matches app.json, the single source of truth. */
export function currentAppVersion(): string {
  // preferExpoConfig is false in SDK 57 for bare workflows, where the manifest
  // value is the real one; fall back to the package version so a misconfigured
  // build reports something rather than an empty string.
  const fromConfig = Constants.expoConfig?.version;
  if (typeof fromConfig === "string" && fromConfig.length > 0) {
    return fromConfig;
  }
  return "0.0.0";
}

export function currentPlatform(): string {
  return Platform.OS === "ios" ? "ios" : "android";
}

export const checkAppVersion = async (): Promise<AppVersionCheck> => {
  const api = createApiClient();
  const response = await api.get<AppVersionCheck>("/app/version-check", {
    params: { platform: currentPlatform(), version: currentAppVersion() },
  });
  return response.data;
};

/**
 * Where the user should go to update.
 *
 * The Play URL is derived from the applicationId and is therefore correct. The
 * App Store one is a placeholder: Apple identifies apps by a numeric ID that is
 * only known after the first submission, and shipping a guessed link would send
 * users to the wrong app. Replace IOS_APP_STORE_ID with the real ID from the
 * App Store Connect record before the first TestFlight build - the blocking
 * screen's only exit depends on it.
 */
const IOS_APP_STORE_ID = "id0000000000";

export function storeUrl(): string {
  const appId = Constants.expoConfig?.android?.package ?? "com.allcallall.mobile";
  return Platform.OS === "ios"
    ? `https://apps.apple.com/app/${IOS_APP_STORE_ID}`
    : `https://play.google.com/store/apps/details?id=${appId}`;
}
