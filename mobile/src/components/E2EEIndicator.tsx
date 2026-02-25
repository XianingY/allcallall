import React from "react";
import { View, Text, StyleSheet, TouchableOpacity, Alert } from "react-native";
import { useSignaling } from "../context/SignalingContext";
import { formatFingerprint } from "../services/e2ee/E2EEService";

interface E2EEIndicatorProps {
  onPress?: () => void;
}

export const E2EEIndicator: React.FC<E2EEIndicatorProps> = ({ onPress }) => {
  const { e2eeEnabled, e2eeSessionEstablished, e2eeFingerprint } = useSignaling();

  if (!e2eeEnabled) {
    return null;
  }

  const handlePress = () => {
    if (onPress) {
      onPress();
    } else if (e2eeFingerprint) {
      Alert.alert(
        "E2EE Session Fingerprint",
        formatFingerprint(e2eeFingerprint),
        [{ text: "OK" }]
      );
    }
  };

  const getStatusIcon = () => {
    if (!e2eeSessionEstablished) {
      return "🔓";
    }
    return "🔒";
  };

  const getStatusText = () => {
    if (!e2eeSessionEstablished) {
      return "Establishing E2EE...";
    }
    return "E2EE Active";
  };

  return (
    <TouchableOpacity onPress={handlePress} style={styles.container}>
      <View style={styles.content}>
        <Text style={styles.icon}>{getStatusIcon()}</Text>
        <Text style={styles.text}>{getStatusText()}</Text>
      </View>
    </TouchableOpacity>
  );
};

const styles = StyleSheet.create({
  container: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    backgroundColor: "rgba(0, 0, 0, 0.6)",
    borderRadius: 8,
    alignSelf: "flex-start",
  },
  content: {
    flexDirection: "row",
    alignItems: "center",
  },
  icon: {
    fontSize: 16,
    marginRight: 6,
  },
  text: {
    color: "#FFFFFF",
    fontSize: 12,
    fontWeight: "600",
  },
});
