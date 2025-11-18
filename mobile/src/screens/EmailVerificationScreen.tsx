import React, { useState, useEffect } from "react";
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  KeyboardAvoidingView,
  Platform,
  Alert,
  ActivityIndicator,
  ScrollView,
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import TextField from "../components/TextField";
import PrimaryButton from "../components/PrimaryButton";
import VerificationCodeInput from "../components/VerificationCodeInput";
import { sendVerificationCode, verifyCode } from "../api/email";
import { RootStackParamList } from "../navigation/AppNavigator";
import { useAuthContext } from "../context/AuthContext";

type Props = NativeStackScreenProps<RootStackParamList, "EmailVerification">;

/**
 * 邮箱验证屏幕
 * 两步流程：1. 输入邮箱 2. 输入验证码
 */
const EmailVerificationScreen: React.FC<Props> = ({ navigation, route }) => {
  const { email: initialEmail, onVerified } = route.params || {};
  const { register } = useAuthContext();

  // ... 现有代码 ...

  // UI 状态
  const [step, setStep] = useState<"input" | "verify">("input");
  const [email, setEmail] = useState(initialEmail || "");
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [resendLoading, setResendLoading] = useState(false);
  const [countdown, setCountdown] = useState(0);

  // 倒计时逻辑
  useEffect(() => {
    let timer: NodeJS.Timeout;
    if (countdown > 0) {
      timer = setTimeout(() => setCountdown(countdown - 1), 1000);
    }
    return () => clearTimeout(timer);
  }, [countdown]);

  /**
   * 发送验证码
   */
  const handleSendCode = async () => {
    try {
      // 基础验证
      if (!email.trim()) {
        Alert.alert("错误", "请输入邮箱地址");
        return;
      }

      // 邮箱格式验证
      const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
      if (!emailRegex.test(email)) {
        Alert.alert("错误", "请输入有效的邮箱地址");
        return;
      }

      setLoading(true);
      await sendVerificationCode(email.trim().toLowerCase());

      Alert.alert("成功", "验证码已发送到您的邮箱");
      setStep("verify");
      setCountdown(60); // 60秒倒计时
    } catch (error) {
      console.error("Send code error:", error);
      const errorMessage =
        error instanceof Error
          ? error.message
          : "请检查网络连接后重试";
      Alert.alert("发送失败", errorMessage);
    } finally {
      setLoading(false);
    }
  };

  /**
   * 验证码校验
   */
  const handleVerifyCode = async () => {
    try {
      if (code.length !== 6) {
        Alert.alert("错误", "请输入完整的6位验证码");
        return;
      }

      setLoading(true);
      await verifyCode(email.trim().toLowerCase(), code);

      Alert.alert("成功", "邮箱验证完成");

      // 如果是从注册流程来的，需要调用 onVerified 回调并完成注册
      if (onVerified) {
        // 从注册流程来，第二步是提供注册信息
        // 帮割 email 地址，让用户冒充其他信息
        navigation.navigate("Register", { email: email.trim().toLowerCase() });
      } else {
        // 单纯邮箱验证流程，正常返回
        navigation.goBack();
      }
    } catch (error) {
      console.error("Verify code error:", error);
      const errorMessage =
        error instanceof Error
          ? error.message
          : "请检查验证码后重试";
      Alert.alert("验证失败", errorMessage);
    } finally {
      setLoading(false);
    }
  };

  /**
   * 重新发送验证码
   */
  const handleResendCode = async () => {
    try {
      setResendLoading(true);
      await sendVerificationCode(email.trim().toLowerCase());
      Alert.alert("成功", "验证码已重新发送");
      setCountdown(60);
      setCode(""); // 清空之前输入的code
    } catch (error) {
      console.error("Resend code error:", error);
      Alert.alert("重新发送失败", "请稍后重试");
    } finally {
      setResendLoading(false);
    }
  };

  // 第一步：邮箱输入
  if (step === "input") {
    return (
      <KeyboardAvoidingView
        behavior={Platform.OS === "ios" ? "padding" : undefined}
        style={styles.container}
      >
        <ScrollView
          contentContainerStyle={styles.scrollContent}
          showsVerticalScrollIndicator={false}
        >
          <View style={styles.header}>
            <Text style={styles.title}>邮箱验证</Text>
            <Text style={styles.subtitle}>
              我们会向您的邮箱发送验证码
            </Text>
          </View>

          <View style={styles.form}>
            <TextField
              label="邮箱 / Email"
              autoCapitalize="none"
              keyboardType="email-address"
              value={email}
              onChangeText={setEmail}
              placeholder="example@email.com"
              editable={!loading}
            />

            <PrimaryButton
              title={loading ? "发送中..." : "发送验证码 / Send Code"}
              onPress={handleSendCode}
              disabled={loading || !email.trim()}
            />

            <TouchableOpacity
              onPress={() => navigation.goBack()}
              style={styles.linkButton}
              disabled={loading}
            >
              <Text style={styles.linkText}>返回 / Go Back</Text>
            </TouchableOpacity>
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    );
  }

  // 第二步：验证码输入
  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      style={styles.container}
    >
      <ScrollView
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.header}>
          <Text style={styles.title}>验证邮箱</Text>
          <Text style={styles.subtitle}>
            请输入发送到 {email} 的验证码
          </Text>
        </View>

        <View style={styles.form}>
          <VerificationCodeInput
            codeLength={6}
            onCodeChange={setCode}
            onCodeComplete={handleVerifyCode}
            editable={!loading}
          />

          <PrimaryButton
            title={loading ? "验证中..." : "确认验证 / Verify"}
            onPress={handleVerifyCode}
            disabled={loading || code.length !== 6}
          />

          <View style={styles.resendContainer}>
            {countdown > 0 ? (
              <Text style={styles.countdownText}>
                {countdown}秒后可重新发送
              </Text>
            ) : (
              <TouchableOpacity
                onPress={handleResendCode}
                disabled={resendLoading}
              >
                {resendLoading ? (
                  <ActivityIndicator size="small" color="#2563eb" />
                ) : (
                  <Text style={styles.resendText}>未收到验证码？重新发送</Text>
                )}
              </TouchableOpacity>
            )}
          </View>

          <TouchableOpacity
            onPress={() => setStep("input")}
            style={styles.linkButton}
            disabled={loading}
          >
            <Text style={styles.linkText}>更改邮箱 / Change Email</Text>
          </TouchableOpacity>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f9fafb",
  },
  scrollContent: {
    flexGrow: 1,
    paddingHorizontal: 24,
    paddingTop: 48,
    paddingBottom: 24,
  },
  header: {
    marginBottom: 36,
  },
  title: {
    fontSize: 28,
    fontWeight: "700",
    color: "#1f2937",
  },
  subtitle: {
    marginTop: 12,
    fontSize: 14,
    color: "#6b7280",
    lineHeight: 20,
  },
  form: {
    flex: 1,
  },
  resendContainer: {
    alignItems: "center",
    marginVertical: 20,
  },
  countdownText: {
    fontSize: 14,
    color: "#6b7280",
  },
  resendText: {
    fontSize: 14,
    color: "#2563eb",
    fontWeight: "600",
  },
  linkButton: {
    marginTop: 16,
    alignItems: "center",
  },
  linkText: {
    color: "#2563eb",
    fontWeight: "600",
    fontSize: 14,
  },
});

export default EmailVerificationScreen;
