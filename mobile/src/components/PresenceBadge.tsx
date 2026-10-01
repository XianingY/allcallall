import React from "react";
import { View, Text, StyleSheet } from "react-native";
import { formatShortDateTime } from "@allcallall/shared";

interface Props {
  online: boolean;
  lastSeen?: string | null;
}

const PresenceBadge: React.FC<Props> = ({ online, lastSeen }) => {
  return (
    <View style={styles.container}>
      <View style={[styles.dot, online ? styles.online : styles.offline]} />
      <Text style={styles.text}>
        {online
          ? "在线 / Online"
          : lastSeen
          ? `离线 / Offline • ${formatShortDateTime(lastSeen)}`
          : "离线 / Offline"}
      </Text>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "center"
  },
  dot: {
    width: 10,
    height: 10,
    borderRadius: 5,
    marginRight: 6
  },
  online: {
    backgroundColor: "#22c55e"
  },
  offline: {
    backgroundColor: "#9ca3af"
  },
  text: {
    fontSize: 12,
    color: "#4b5563"
  }
});

export default PresenceBadge;
