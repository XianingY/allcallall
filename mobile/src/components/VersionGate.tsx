import React, { useState } from "react";
import { ActivityIndicator, StyleSheet, Text, View } from "react-native";

import { UpdateAvailableBanner } from "../components/UpdateAvailableBanner";
import { UpdateRequiredScreen } from "../components/UpdateRequiredScreen";
import { useVersionGate } from "../hooks/useVersionGate";

/**
 * Gates the app on the server's version policy.
 *
 * Only one of the three states actually stops the user: a build the server has
 * withdrawn support for. An advisory update shows a dismissible banner, and a
 * failed or slow check passes straight through - a gate that blocks on a
 * network problem would turn a backend hiccup into an outage for every
 * installed client.
 */
export const VersionGate: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { state, recheck } = useVersionGate();
  const [dismissed, setDismissed] = useState(false);

  if (state.status === "blocked") {
    return <UpdateRequiredScreen check={state.check} onRetry={recheck} />;
  }

  if (state.status === "checking") {
    // A brief, explicit wait rather than the app appearing and then being
    // covered: flashing the UI and replacing it a moment later reads as a bug.
    return (
      <View style={styles.splash}>
        <ActivityIndicator size="large" color="#31584e" />
        <Text style={styles.splashText}>正在检查版本…</Text>
      </View>
    );
  }

  return (
    <>
      {state.status === "optional" && !dismissed ? (
        <UpdateAvailableBanner check={state.check} onDismiss={() => setDismissed(true)} />
      ) : null}
      {children}
    </>
  );
};

const styles = StyleSheet.create({
  splash: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "#f8fafc",
    gap: 12,
  },
  splashText: {
    fontSize: 13,
    color: "#64748b",
  },
});
