import React, { useState } from "react";
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  KeyboardAvoidingView,
  Platform,
  Alert,
  ScrollView
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import { AxiosError } from "axios";

import TextField from "../components/TextField";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import { changePassword, ChangePasswordRequest } from "../api/users";

type Props = NativeStackScreenProps<RootStackParamList, "ChangePassword">;

interface PasswordValidation {
  isValid: boolean;
  errors: string[];
}

const validatePassword = (password: string): PasswordValidation => {
  const errors: string[] = [];

  if (password.length < 8) {
    errors.push("密码至少需要 8 个字符");
  }
  if (password.length > 128) {
    errors.push("密码最多 128 个字符");
  }

  const hasLetter = /[a-zA-Z]/.test(password);
  const hasDigit = /[0-9]/.test(password);
  // 检查是否仅包含字母和数字（不允许空格或特殊字符）
  const onlyLettersAndDigits = /^[a-zA-Z0-9]*$/.test(password);

  if (!hasLetter) {
    errors.push("密码必需包含字母（A-Z, a-z）");
  }
  if (!hasDigit) {
    errors.push("密码必需包含数字（0-9）");
  }
  if (!onlyLettersAndDigits && password.length > 0) {
    errors.push("密码不能包含特殊字符或空格");
  }

  return {
    isValid: errors.length === 0,
    errors
  };
};

const ChangePasswordScreen: React.FC<Props> = ({ navigation }) => {
  const { token } = useAuthContext();
  const [oldPassword, setOldPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);

  // 检查 token 是否存在，如果不存在则跳转回登陆
  React.useEffect(() => {
    if (!token) {
      Alert.alert("授权失效", "请先登陆");
      navigation.goBack();
    }
  }, [token, navigation]);

  const newPasswordValidation = validatePassword(newPassword);
  const passwordsMatch = newPassword === confirmPassword && newPassword !== "";
  const isFormValid =
    oldPassword.length > 0 &&
    newPasswordValidation.isValid &&
    passwordsMatch;

  const handleChangePassword = async () => {
    if (!isFormValid) {
      Alert.alert("表单错误", "请检查所有字段");
      return;
    }

    try {
      setLoading(true);

      const request: ChangePasswordRequest = {
        old_password: oldPassword,
        new_password: newPassword,
        confirm_password: confirmPassword
      };

      await changePassword(token || "", request);

      Alert.alert(
        "成功",
        "密码已成功修改",
        [
          {
            text: "确定",
            onPress: () => navigation.goBack()
          }
        ]
      );
    } catch (error) {
      let errorMessage = "修改密码失败，请重试";

      if (error instanceof AxiosError && error.response?.data) {
        const data = error.response.data as any;
        // 后端返回结构: { error: "message", success: false }
        if (data.error) {
          errorMessage = data.error;
        }
      }

      Alert.alert("修改失败", errorMessage);
      console.error("Change password error:", error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      style={styles.container}
    >
      <ScrollView style={styles.scrollView} showsVerticalScrollIndicator={false}>
        <View style={styles.header}>
          <Text style={styles.title}>修改密码</Text>
          <Text style={styles.subtitle}>
            请输入您的当前密码和新密码
          </Text>
        </View>

        <View style={styles.form}>
          <View style={styles.fieldContainer}>
            <Text style={styles.label}>当前密码 *</Text>
            <TextField
              placeholder="输入当前密码"
              secureTextEntry={!showPassword}
              value={oldPassword}
              onChangeText={setOldPassword}
            />
            {oldPassword.length === 0 && (
              <Text style={styles.helperText}>此字段为必填项</Text>
            )}
          </View>

          <View style={styles.fieldContainer}>
            <Text style={styles.label}>新密码 *</Text>
            <TextField
              placeholder="输入新密码"
              secureTextEntry={!showPassword}
              value={newPassword}
              onChangeText={setNewPassword}
            />
            <Text style={styles.helperText}>
              需要至少 8 个字符，必须包含字母和数字
            </Text>

            {newPassword.length > 0 && (
              <View style={styles.validationContainer}>
                {newPasswordValidation.errors.map((error, index) => (
                  <View key={index} style={styles.errorItem}>
                    <Text style={styles.errorText}>✗ {error}</Text>
                  </View>
                ))}
                {newPasswordValidation.isValid && (
                  <View style={styles.successItem}>
                    <Text style={styles.successText}>✓ 密码符合要求</Text>
                  </View>
                )}
              </View>
            )}
          </View>

          <View style={styles.fieldContainer}>
            <Text style={styles.label}>确认新密码 *</Text>
            <TextField
              placeholder="再次输入新密码"
              secureTextEntry={!showPassword}
              value={confirmPassword}
              onChangeText={setConfirmPassword}
            />
            {confirmPassword.length > 0 && newPassword.length > 0 && (
              <>
                {passwordsMatch ? (
                  <Text style={styles.successText}>✓ 两次密码一致</Text>
                ) : (
                  <Text style={styles.errorText}>✗ 两次密码不一致</Text>
                )}
              </>
            )}
          </View>

          <TouchableOpacity
            onPress={() => setShowPassword(!showPassword)}
            style={styles.toggleButton}
          >
            <Text style={styles.toggleText}>
              {showPassword ? "隐藏密码" : "显示密码"}
            </Text>
          </TouchableOpacity>

          <PrimaryButton
            title={loading ? "修改中..." : "修改密码"}
            onPress={handleChangePassword}
            disabled={!isFormValid || loading}
          />

          <TouchableOpacity
            onPress={() => navigation.goBack()}
            style={styles.cancelButton}
          >
            <Text style={styles.cancelText}>取消</Text>
          </TouchableOpacity>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f9fafb"
  },
  scrollView: {
    flex: 1,
    paddingHorizontal: 24,
    paddingTop: 24
  },
  header: {
    marginBottom: 32
  },
  title: {
    fontSize: 28,
    fontWeight: "800",
    color: "#1f2937"
  },
  subtitle: {
    marginTop: 8,
    fontSize: 14,
    color: "#6b7280",
    lineHeight: 20
  },
  form: {
    marginBottom: 32
  },
  fieldContainer: {
    marginBottom: 24
  },
  label: {
    fontSize: 14,
    fontWeight: "600",
    color: "#374151",
    marginBottom: 8
  },
  helperText: {
    marginTop: 8,
    fontSize: 12,
    color: "#6b7280"
  },
  validationContainer: {
    marginTop: 12,
    paddingHorizontal: 12,
    paddingVertical: 8,
    backgroundColor: "#f3f4f6",
    borderRadius: 6
  },
  errorItem: {
    marginVertical: 4
  },
  errorText: {
    fontSize: 12,
    color: "#dc2626"
  },
  successItem: {
    marginVertical: 4
  },
  successText: {
    fontSize: 12,
    color: "#16a34a"
  },
  toggleButton: {
    paddingVertical: 8,
    marginBottom: 16
  },
  toggleText: {
    color: "#2563eb",
    fontSize: 14,
    fontWeight: "500"
  },
  cancelButton: {
    marginTop: 12,
    paddingVertical: 12,
    alignItems: "center"
  },
  cancelText: {
    color: "#6b7280",
    fontSize: 16,
    fontWeight: "600"
  }
});

export default ChangePasswordScreen;
