import React from "react";
import { Linking, Pressable, StyleSheet, Text, View } from "react-native";

import { storeUrl, type AppVersionCheck } from "../api/appVersion";

interface Props {
  check: AppVersionCheck;
  onDismiss: () => void;
}

/**
 * Shown when a newer release exists but this build is still supported.
 *
 * Dismissible on purpose: this is advisory, and a user mid-call should not be
 * held behind a modal because the app was updated last week. The blocking
 * screen is the one that must not be dismissible.
 */
export const UpdateAvailableBanner: React.FC<Props> = ({ check, onDismiss }) => (
  <View style={styles.banner}>
    <View style={styles.text}>
      <Text style={styles.title}>
        {check.latest_version ? `新版本 ${check.latest_version} 可用` : "有新版本可用"}
      </Text>
      {check.message ? <Text style={styles.message}>{check.message}</Text> : null}
    </View>
    <View style={styles.actions}>
      <Pressable
        style={styles.update}
        onPress={() => {
          void Linking.openURL(storeUrl()).catch((error: unknown) => {
            console.error("[UpdateAvailable] failed to open the store", error);
          });
        }}
      >
        <Text style={styles.updateText}>去更新</Text>
      </Pressable>
      <Pressable style={styles.dismiss} onPress={onDismiss} accessibilityRole="button" accessibilityLabel="忽略此提示">
        <Text style={styles.dismissText}>稍后</Text>
      </Pressable>
    </View>
  </View>
);

const styles = StyleSheet.create({
  banner: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
    margin: 12,
    padding: 12,
    borderRadius: 12,
    backgroundColor: "#eef4f2",
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#c7d9d3",
  },
  text: {
    flex: 1,
  },
  title: {
    fontSize: 14,
    fontWeight: "500",
    color: "#1f2937",
  },
  message: {
    marginTop: 2,
    fontSize: 12,
    color: "#4b5563",
  },
  actions: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
  },
  update: {
    paddingHorizontal: 12,
    paddingVertical: 8,
    borderRadius: 8,
    backgroundColor: "#1f2937",
  },
  updateText: {
    color: "#ffffff",
    fontSize: 13,
    fontWeight: "500",
  },
  dismiss: {
    paddingHorizontal: 8,
    paddingVertical: 8,
  },
  dismissText: {
    color: "#4b5563",
    fontSize: 13,
  },
});
