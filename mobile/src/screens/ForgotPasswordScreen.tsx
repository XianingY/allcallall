import React, { useState } from "react";
import {
  Alert,
  KeyboardAvoidingView,
  Platform,
  StyleSheet,
  Text,
  TouchableOpacity,
  View
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import {
  confirmPasswordReset,
  sendPasswordResetCode
} from "../api/commercial";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import VerificationCodeInput from "../components/VerificationCodeInput";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "ForgotPassword">;

const ForgotPasswordScreen: React.FC<Props> = ({ navigation }) => {
  const [step, setStep] = useState<"email" | "confirm">("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [loading, setLoading] = useState(false);

  const normalizedEmail = email.trim().toLowerCase();

  const handleSend = async () => {
    if (!normalizedEmail) {
      Alert.alert("缺少邮箱", "请输入注册邮箱。");
      return;
    }
    try {
      setLoading(true);
      await sendPasswordResetCode(normalizedEmail);
      setStep("confirm");
      Alert.alert("验证码已发送", "如果邮箱存在，将收到一封重置验证码邮件。");
    } catch (error) {
      console.error("[ForgotPasswordScreen] send reset code failed:", error);
      Alert.alert("发送失败", "无法发送重置验证码，请稍后再试。");
    } finally {
      setLoading(false);
    }
  };

  const handleConfirm = async () => {
    if (code.trim().length !== 6) {
      Alert.alert("验证码无效", "请输入 6 位验证码。");
      return;
    }
    if (newPassword.length < 8) {
      Alert.alert("密码太短", "新密码至少需要 8 位。");
      return;
    }
    if (newPassword !== confirmPassword) {
      Alert.alert("密码不一致", "两次输入的新密码不一致。");
      return;
    }
    try {
      setLoading(true);
      await confirmPasswordReset(normalizedEmail, code.trim(), newPassword, confirmPassword);
      Alert.alert("密码已重置", "请使用新密码登录。", [
        {
          text: "返回登录",
          onPress: () => navigation.navigate("Login")
        }
      ]);
    } catch (error) {
      console.error("[ForgotPasswordScreen] confirm reset failed:", error);
      Alert.alert("重置失败", "验证码或新密码无效，请重试。");
    } finally {
      setLoading(false);
    }
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      style={styles.container}
    >
      <View style={styles.card}>
        <Text style={styles.title}>忘记密码 / Reset Password</Text>
        <Text style={styles.subtitle}>
          {step === "email"
            ? "输入注册邮箱，我们会发送密码重置验证码。"
            : "输入收到的验证码并设置新密码。"}
        </Text>

        <TextField
          label="邮箱 / Email"
          autoCapitalize="none"
          keyboardType="email-address"
          value={email}
          onChangeText={setEmail}
          editable={!loading && step === "email"}
        />

        {step === "confirm" ? (
          <>
            <VerificationCodeInput
              codeLength={6}
              onCodeChange={setCode}
              onCodeComplete={setCode}
              editable={!loading}
            />
            <TextField
              label="新密码 / New Password"
              secureTextEntry
              value={newPassword}
              onChangeText={setNewPassword}
              editable={!loading}
            />
            <TextField
              label="确认新密码 / Confirm Password"
              secureTextEntry
              value={confirmPassword}
              onChangeText={setConfirmPassword}
              editable={!loading}
            />
          </>
        ) : null}

        <PrimaryButton
          title={
            loading
              ? step === "email"
                ? "发送中..."
                : "提交中..."
              : step === "email"
                ? "发送重置验证码"
                : "确认重置密码"
          }
          onPress={step === "email" ? handleSend : handleConfirm}
          disabled={loading}
        />

        <TouchableOpacity
          style={styles.secondary}
          onPress={() => navigation.goBack()}
          disabled={loading}
        >
          <Text style={styles.secondaryText}>返回 / Back</Text>
        </TouchableOpacity>
      </View>
    </KeyboardAvoidingView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f4f7fb",
    justifyContent: "center",
    paddingHorizontal: 20
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 20,
    padding: 20,
    shadowColor: "#0f172a",
    shadowOpacity: 0.08,
    shadowOffset: { width: 0, height: 8 },
    shadowRadius: 20,
    elevation: 4
  },
  title: {
    fontSize: 26,
    fontWeight: "800",
    color: "#0f172a"
  },
  subtitle: {
    marginTop: 10,
    marginBottom: 20,
    color: "#475569",
    lineHeight: 20
  },
  secondary: {
    marginTop: 14,
    alignItems: "center"
  },
  secondaryText: {
    color: "#2563eb",
    fontWeight: "700"
  }
});

export default ForgotPasswordScreen;
