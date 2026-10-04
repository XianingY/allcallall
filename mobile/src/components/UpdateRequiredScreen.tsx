import React from "react";
import { Linking, Platform, StyleSheet, Text, View } from "react-native";

import { storeUrl, type AppVersionCheck } from "../api/appVersion";
import PrimaryButton from "./PrimaryButton";

interface Props {
  check: AppVersionCheck;
  /** Shown when the check itself could not be completed, so the user is not stuck. */
  onRetry?: () => void;
}

/**
 * The blocking screen for a build the server no longer supports.
 *
 * There is deliberately no dismiss control. `update_allowed: false` is the
 * server saying this build must not keep running, and offering a way around it
 * would make the whole mechanism advisory - at which point an incompatible
 * client talks to a newer API and produces confusing failures instead of a clear
 * message. The only exit is the store.
 */
export const UpdateRequiredScreen: React.FC<Props> = ({ check, onRetry }) => {
  const target = check.latest_version || check.min_supported_version;
  const failedCheck = check.policy_configured === false && Boolean(onRetry);

  return (
    <View style={styles.container}>
      <View style={styles.card}>
        <Text style={styles.eyebrow}>需要更新 / Update required</Text>
        <Text style={styles.title}>这个版本已不再受支持</Text>
        <Text style={styles.body}>
          当前版本 {check.current_version}
          {check.min_supported_version ? `，最低要求 ${check.min_supported_version}` : ""}
          {target ? `。请更新到 ${target} 或更高版本后继续使用。` : "。请安装最新版本后继续使用。"}
        </Text>
        {check.message ? <Text style={styles.message}>{check.message}</Text> : null}

        <PrimaryButton
          title={Platform.OS === "ios" ? "前往 App Store" : "前往 Google Play"}
          onPress={() => {
            void Linking.openURL(storeUrl()).catch((error: unknown) => {
              console.error("[UpdateRequired] failed to open the store", error);
            });
          }}
          style={styles.action}
        />
        {onRetry ? (
          <PrimaryButton title="重试检查" onPress={onRetry} style={styles.secondary} />
        ) : null}
        {failedCheck ? null : null}
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    alignItems: "center",
    justifyContent: "center",
    padding: 24,
  },
  card: {
    width: "100%",
    maxWidth: 420,
    backgroundColor: "#ffffff",
    borderRadius: 16,
    padding: 24,
    gap: 12,
  },
  eyebrow: {
    fontSize: 12,
    color: "#b45309",
    letterSpacing: 0.4,
  },
  title: {
    fontSize: 20,
    fontWeight: "600",
    color: "#0f172a",
  },
  body: {
    fontSize: 14,
    lineHeight: 22,
    color: "#334155",
  },
  message: {
    fontSize: 13,
    lineHeight: 20,
    color: "#64748b",
  },
  action: {
    marginTop: 8,
  },
  secondary: {
    backgroundColor: "#e2e8f0",
  },
});
