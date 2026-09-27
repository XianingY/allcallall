import React from "react";
import { StyleSheet, Text, View, ViewStyle } from "react-native";

import PrimaryButton from "./PrimaryButton";

interface Props {
  message: string;
  onRetry: () => void;
  style?: ViewStyle;
}

/**
 * Persistent load-failure block with a retry action, modelled on the
 * `loadError` pattern in RoomsScreen. Renders "加载失败：<message>" so the copy
 * stays consistent with neighbouring screens.
 */
const LoadError: React.FC<Props> = ({ message, onRetry, style }) => {
  return (
    <View style={[styles.block, style]}>
      <Text style={styles.text}>加载失败：{message}</Text>
      <PrimaryButton title="重试" onPress={onRetry} />
    </View>
  );
};

const styles = StyleSheet.create({
  block: {
    marginTop: 24,
    gap: 12,
    padding: 16,
    borderRadius: 10,
    backgroundColor: "#fef2f2"
  },
  text: {
    color: "#b91c1c",
    fontSize: 14,
    lineHeight: 20
  }
});

export default LoadError;
