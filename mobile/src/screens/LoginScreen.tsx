import React, { useState } from "react";
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  KeyboardAvoidingView,
  Platform,
  Alert
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import TextField from "../components/TextField";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "Login">;

const LoginScreen: React.FC<Props> = ({ navigation }) => {
  const { login } = useAuthContext();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);

  const handleLogin = async () => {
    try {
      setLoading(true);
      await login(email.trim(), password);
    } catch (error) {
      console.error(error);
      Alert.alert(
        "登录失败 / Login failed",
        "请检查邮箱和密码 / Please verify your email and password."
      );
    } finally {
      setLoading(false);
    }
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      style={styles.container}
    >
      <View style={styles.header}>
        <Text style={styles.title}>AllCallAll</Text>
        <Text style={styles.subtitle}>
          以邮箱为唯一地址的实时通话系统{"\n"}Email-first calling experience
        </Text>
      </View>
      <View style={styles.form}>
        <TextField
          label="邮箱 / Email"
          autoCapitalize="none"
          keyboardType="email-address"
          value={email}
          onChangeText={setEmail}
        />
        <TextField
          label="密码 / Password"
          secureTextEntry
          value={password}
          onChangeText={setPassword}
        />
        <PrimaryButton
          title={loading ? "登录中..." : "登录 / Login"}
          onPress={handleLogin}
          disabled={loading}
        />
        <TouchableOpacity
          onPress={() => navigation.navigate("Register", {})}
          style={styles.linkButton}
        >
          <Text style={styles.linkText}>
            还没有账号？注册 / Need an account? Sign up
          </Text>
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
    fontSize: 32,
    fontWeight: "800",
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
  linkButton: {
    marginTop: 16,
    alignItems: "center"
  },
  linkText: {
    color: "#2563eb",
    fontWeight: "600"
  }
});

export default LoginScreen;
