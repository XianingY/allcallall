import React, { useState } from "react";
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  KeyboardAvoidingView,
  Platform,
  Alert,
  Switch,
  Linking
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import type { RegistrationErrors } from "@allcallall/shared";
import { hasErrors, isValidEmail, MESSAGES, validateRegistration } from "@allcallall/shared";

import TextField from "../components/TextField";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import { fetchCurrentLegal, type LegalInfo } from "../api/commercial";

type Props = NativeStackScreenProps<RootStackParamList, "Register">;

const RegisterScreen: React.FC<Props> = ({ navigation, route }) => {
  const { register } = useAuthContext();
  const { email: prefilledEmail } = route.params || {};
  const emailLocked = Boolean(prefilledEmail);
  const [email, setEmail] = useState(prefilledEmail || "");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  // Web asks for a password confirmation and enforces a minimum display-name
  // length; mobile did neither, so the same weak input was accepted here and
  // rejected there. Both now run the shared rules.
  const [confirmPassword, setConfirmPassword] = useState("");
  const [errors, setErrors] = useState<RegistrationErrors>({});
  const [acceptCurrentLegal, setAcceptCurrentLegal] = useState(false);
  const [legal, setLegal] = useState<LegalInfo | null>(null);
  const [loading, setLoading] = useState(false);

  const normalizedEmail = email.trim().toLowerCase();

  React.useEffect(() => {
    const loadLegal = async () => {
      try {
        setLegal(await fetchCurrentLegal());
      } catch (error) {
        console.warn("[RegisterScreen] Failed to load legal metadata:", error);
      }
    };
    void loadLegal();
  }, []);

  // Field errors render under each input instead of in an Alert, so the user
  // can see what to fix without dismissing a dialog first.
  const validateEmail = () => {
    const emailError = isValidEmail(normalizedEmail) ? undefined : MESSAGES.emailInvalid;
    setErrors((current) => ({ ...current, email: emailError }));
    return !emailError;
  };

  const handleStartVerification = () => {
    if (!validateEmail()) {
      return;
    }

    navigation.navigate("EmailVerification", {
      email: normalizedEmail,
      returnToRegister: true
    });
  };

  const handleRegister = async () => {
    if (!emailLocked) {
      navigation.navigate("EmailVerification", {
        email: normalizedEmail,
        returnToRegister: true
      });
      return;
    }

    try {
      // Same rules as the web signup form - see packages/shared/validation.
      const validationErrors = validateRegistration({
        email: normalizedEmail,
        password,
        confirmPassword,
        displayName,
        acceptedLegal: acceptCurrentLegal,
      });
      setErrors(validationErrors);
      if (hasErrors(validationErrors)) {
        return;
      }

      setLoading(true);
      await register(normalizedEmail, password, displayName.trim(), true);
    } catch (error) {
      console.error("Register error:", error);
      if (error instanceof Error) {
        Alert.alert("错误", error.message || "请检查输入信息");
      } else {
        Alert.alert("错误", "请检查输入信息");
      }
    } finally {
      setLoading(false);
    }
  };

  const openLegalLink = async (url?: string | null) => {
    if (!url) {
      return;
    }
    try {
      await Linking.openURL(url);
    } catch (error) {
      console.warn("[RegisterScreen] Failed to open legal link:", error);
      Alert.alert("打开失败", "当前无法打开法律文档链接。");
    }
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      style={styles.container}
    >
      <View style={styles.header}>
        <Text style={styles.title}>创建账号 / Create account</Text>
        <Text style={styles.subtitle}>
          {emailLocked
            ? "邮箱已验证，请继续设置显示名称和密码"
            : "先完成邮箱验证，再继续创建账号"}
        </Text>
      </View>
      <View style={styles.form}>
        <TextField
          label="邮箱 / Email"
          autoCapitalize="none"
          keyboardType="email-address"
          value={email}
          onChangeText={setEmail}
          editable={!loading && !emailLocked}
          error={errors.email ?? null}
        />
        {emailLocked ? (
          <>
            <TextField
              label="显示名称 / Display name"
              autoCapitalize="words"
              value={displayName}
              onChangeText={setDisplayName}
              editable={!loading}
              error={errors.displayName ?? null}
            />
            <TextField
              label="密码 / Password"
              secureTextEntry
              value={password}
              onChangeText={setPassword}
              editable={!loading}
              error={errors.password ?? null}
            />
            <TextField
              label="确认密码 / Confirm password"
              secureTextEntry
              value={confirmPassword}
              onChangeText={setConfirmPassword}
              editable={!loading}
              error={errors.confirmPassword ?? null}
            />
            <View style={styles.legalCard}>
              <View style={styles.legalToggleRow}>
                <Switch
                  value={acceptCurrentLegal}
                  onValueChange={setAcceptCurrentLegal}
                  disabled={loading}
                />
                <Text style={styles.legalText}>
                  我已阅读并接受当前服务条款与隐私政策
                </Text>
              </View>
              <View style={styles.legalLinks}>
                <TouchableOpacity
                  onPress={() => void openLegalLink(legal?.terms_url)}
                  disabled={!legal?.terms_url}
                >
                  <Text style={styles.legalLinkText}>查看条款</Text>
                </TouchableOpacity>
                <TouchableOpacity
                  onPress={() => void openLegalLink(legal?.privacy_policy_url)}
                  disabled={!legal?.privacy_policy_url}
                >
                  <Text style={styles.legalLinkText}>查看隐私政策</Text>
                </TouchableOpacity>
              </View>
            </View>
          </>
        ) : (
          <Text style={styles.hintText}>
            验证成功后会返回此页面继续填写显示名称和密码。
          </Text>
        )}
        <PrimaryButton
          title={
            emailLocked
              ? loading
                ? "注册中..."
                : "完成注册 / Register"
              : "验证邮箱 / Verify Email"
          }
          onPress={emailLocked ? handleRegister : handleStartVerification}
          disabled={loading}
        />
        <TouchableOpacity
          onPress={() => navigation.pop()}
          style={styles.linkButton}
          disabled={loading}
        >
          <Text style={styles.linkText}>已有账号？登录 / Already have one?</Text>
        </TouchableOpacity>
      </View>
    </KeyboardAvoidingView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f9fafb",
    paddingHorizontal: 24,
    paddingTop: 48
  },
  header: {
    marginBottom: 36
  },
  title: {
    fontSize: 28,
    fontWeight: "700",
    color: "#1f2937"
  },
  subtitle: {
    marginTop: 12,
    fontSize: 16,
    color: "#6b7280",
    lineHeight: 22
  },
  form: {
    flex: 1
  },
  hintText: {
    marginTop: 12,
    color: "#6b7280",
    lineHeight: 20
  },
  legalCard: {
    marginTop: 12,
    padding: 14,
    borderRadius: 12,
    backgroundColor: "#eef2ff"
  },
  legalToggleRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10
  },
  legalText: {
    flex: 1,
    color: "#1f2937",
    lineHeight: 20
  },
  legalLinks: {
    marginTop: 12,
    flexDirection: "row",
    gap: 16
  },
  legalLinkText: {
    color: "#2563eb",
    fontWeight: "600"
  },
  linkButton: {
    marginTop: 16,
    alignItems: "center"
  },
  linkText: {
    color: "#2563eb",
    fontWeight: "600"
  }
});

export default RegisterScreen;
