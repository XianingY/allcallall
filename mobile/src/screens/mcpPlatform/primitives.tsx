import React from "react";
import { Pressable, Text, TextInput, View } from "react-native";

import { STATUS_COLORS } from "./constants";
import { styles } from "./styles";

/** 统一的按钮：primary / danger / compact / disabled 四种视觉状态。 */
export const ActionButton: React.FC<{
  label: string;
  onPress: () => void;
  disabled?: boolean;
  danger?: boolean;
  compact?: boolean;
}> = ({ label, onPress, disabled, danger, compact }) => (
  <Pressable
    accessibilityRole="button"
    disabled={disabled}
    onPress={onPress}
    style={({ pressed }) => [
      styles.actionButton,
      compact && styles.actionButtonCompact,
      danger && styles.actionButtonDanger,
      disabled && styles.actionButtonDisabled,
      pressed && !disabled && styles.actionButtonPressed
    ]}
  >
    <Text style={[styles.actionButtonText, danger && styles.actionButtonDangerText]}>
      {label}
    </Text>
  </Pressable>
);

export const Field: React.FC<{
  label: string;
  value: string;
  onChangeText: (value: string) => void;
  placeholder?: string;
  error?: string;
  multiline?: boolean;
  secureTextEntry?: boolean;
  editable?: boolean;
  autoCapitalize?: "none" | "sentences" | "words" | "characters";
}> = ({ label, error, multiline, ...props }) => (
  <View style={styles.field}>
    <Text style={styles.fieldLabel}>{label}</Text>
    <TextInput
      {...props}
      autoCorrect={false}
      multiline={multiline}
      placeholderTextColor="#9ca3af"
      style={[
        styles.input,
        multiline && styles.multilineInput,
        Boolean(error) && styles.inputError
      ]}
    />
    {error ? <Text style={styles.errorText}>{error}</Text> : null}
  </View>
);

export const SegmentedControl = <T extends string>({
  value,
  options,
  onChange
}: {
  value: T;
  options: Array<{ value: T; label: string }>;
  onChange: (value: T) => void;
}) => (
  <View style={styles.segmentedControl}>
    {options.map((option) => (
      <Pressable
        key={option.value}
        accessibilityRole="button"
        accessibilityState={{ selected: value === option.value }}
        onPress={() => onChange(option.value)}
        style={[
          styles.segment,
          value === option.value && styles.segmentSelected
        ]}
      >
        <Text
          style={[
            styles.segmentText,
            value === option.value && styles.segmentTextSelected
          ]}
        >
          {option.label}
        </Text>
      </Pressable>
    ))}
  </View>
);

export const StatusBadge: React.FC<{ status: string }> = ({ status }) => {
  const color = STATUS_COLORS[status] ?? {
    background: "#f3f4f6",
    foreground: "#374151"
  };
  return (
    <View style={[styles.badge, { backgroundColor: color.background }]}>
      <Text style={[styles.badgeText, { color: color.foreground }]}>
        {status}
      </Text>
    </View>
  );
};
