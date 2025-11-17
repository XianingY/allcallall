import React, { useState, useRef } from "react";
import {
  View,
  Text,
  TextInput,
  StyleSheet,
  AccessibilityInfo,
} from "react-native";

interface VerificationCodeInputProps {
  onCodeComplete: (code: string) => void;
  onCodeChange?: (code: string) => void;
  codeLength?: number;
  editable?: boolean;
}

/**
 * 验证码输入组件
 * 支持自动跳到下一个输入框，以及退格时回到前一个输入框
 */
const VerificationCodeInput: React.FC<VerificationCodeInputProps> = ({
  onCodeComplete,
  onCodeChange,
  codeLength = 6,
  editable = true,
}) => {
  const [code, setCode] = useState<string[]>(Array(codeLength).fill(""));
  const [focusedIndex, setFocusedIndex] = useState<number | null>(null);
  const inputRefs = useRef<TextInput[]>([]);

  /**
   * 处理单个输入框的内容变化
   */
  const handleCodeChange = (index: number, value: string) => {
    // 只允许数字
    const numericValue = value.replace(/[^0-9]/g, "");

    // 如果粘贴了多个数字，分散到各个输入框
    if (numericValue.length > 1) {
      const newCode = [...code];
      for (let i = 0; i < Math.min(numericValue.length, codeLength - index); i++) {
        newCode[index + i] = numericValue[i];
      }
      setCode(newCode);
      const codeString = newCode.join("");
      onCodeChange?.(codeString);

      // 焦点移到最后填充的位置或最后一个输入框
      const nextIndex = Math.min(index + numericValue.length - 1, codeLength - 1);
      inputRefs.current[nextIndex]?.focus();

      // 如果已填满，触发完成
      if (codeString.length === codeLength && !codeString.includes("")) {
        onCodeComplete(codeString);
      }
      return;
    }

    // 单个数字输入
    if (numericValue.length === 0) {
      // 清空
      const newCode = [...code];
      newCode[index] = "";
      setCode(newCode);
      onCodeChange?.(newCode.join(""));
      return;
    }

    // 输入新数字
    const newCode = [...code];
    newCode[index] = numericValue;
    setCode(newCode);

    const codeString = newCode.join("");
    onCodeChange?.(codeString);

    // 自动跳到下一个输入框
    if (index < codeLength - 1) {
      inputRefs.current[index + 1]?.focus();
    }

    // 所有框都填满时触发完成
    if (codeString.length === codeLength) {
      onCodeComplete(codeString);
    }
  };

  /**
   * 处理键盘事件（主要用于处理退格）
   */
  const handleKeyPress = (index: number, key: string) => {
    if (key === "Backspace") {
      if (code[index]) {
        // 当前框有内容，清空当前框
        const newCode = [...code];
        newCode[index] = "";
        setCode(newCode);
        onCodeChange?.(newCode.join(""));
      } else if (index > 0) {
        // 当前框无内容，移到前一个框并清空
        inputRefs.current[index - 1]?.focus();
      }
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.label}>请输入验证码</Text>
      <View style={styles.codeInputContainer}>
        {Array.from({ length: codeLength }).map((_, index) => (
          <TextInput
            key={index}
            ref={(ref) => {
              if (ref) inputRefs.current[index] = ref;
            }}
            style={[
              styles.codeBox,
              focusedIndex === index ? styles.codeBoxFocused : undefined,
              code[index] ? styles.codeBoxFilled : undefined,
            ]}
            maxLength={1}
            keyboardType="number-pad"
            value={code[index]}
            onChangeText={(value) => handleCodeChange(index, value)}
            onFocus={() => setFocusedIndex(index)}
            onBlur={() => setFocusedIndex(null)}
            onKeyPress={({ nativeEvent }) =>
              handleKeyPress(index, nativeEvent.key)
            }
            editable={editable}
            placeholderTextColor="#9ca3af"
            accessibilityLabel={`验证码第 ${index + 1} 位`}
          />
        ))}
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    alignItems: "center",
    marginVertical: 24,
  },
  label: {
    fontSize: 14,
    color: "#6b7280",
    marginBottom: 16,
    fontWeight: "500",
  },
  codeInputContainer: {
    flexDirection: "row",
    justifyContent: "center",
    gap: 12,
  },
  codeBox: {
    width: 50,
    height: 50,
    borderRadius: 8,
    borderWidth: 2,
    borderColor: "#e5e7eb",
    textAlign: "center",
    fontSize: 20,
    fontWeight: "600",
    color: "#1f2937",
    backgroundColor: "#f9fafb",
  },
  codeBoxFocused: {
    borderColor: "#2563eb",
    backgroundColor: "#fff",
    borderWidth: 3,
  },
  codeBoxFilled: {
    backgroundColor: "#f0f4f8",
  },
});

export default VerificationCodeInput;
