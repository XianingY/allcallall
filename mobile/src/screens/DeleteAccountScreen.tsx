import React, { useState } from "react";
import {
  Alert,
  ScrollView,
  StyleSheet,
  Text,
  View
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { sendVerificationCode } from "../api/email";
import { deleteAccount as submitDeletion } from "../api/commercial";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import VerificationCodeInput from "../components/VerificationCodeInput";
import { useAuthContext } from "../context/AuthContext";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "DeleteAccount">;

const DeleteAccountScreen: React.FC<Props> = ({ navigation }) => {
  const { token, user, logout } = useAuthContext();
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);

  const sendCode = async () => {
    if (!user?.email) {
      return;
    }
    try {
      await sendVerificationCode(user.email, "account_deletion");
      Alert.alert("验证码已发送", "已向当前账号邮箱发送删除确认验证码。");
    } catch (error) {
      console.error("[DeleteAccountScreen] Failed to send deletion code:", error);
      Alert.alert("发送失败", "当前无法发送删除确认验证码。");
    }
  };

  const handleDelete = async () => {
    if (!token) {
      return;
    }
    if (!password.trim() && !code.trim()) {
      Alert.alert("需要确认", "请输入密码或邮箱验证码后再删除账号。");
      return;
    }
    Alert.alert("确认删除", "账号删除后不可恢复，联系人、通话历史和订阅记录都会被清理。", [
      { text: "取消", style: "cancel" },
      {
        text: "确认删除",
        style: "destructive",
        onPress: async () => {
          try {
            setLoading(true);
            await submitDeletion(token, {
              password: password.trim() || undefined,
              code: code.trim() || undefined
            });
            await logout();
            Alert.alert("账号已删除", "你的账号数据已按策略清理。", [
              { text: "返回登录", onPress: () => navigation.navigate("Login") }
            ]);
          } catch (error) {
            console.error("[DeleteAccountScreen] Failed to delete account:", error);
            Alert.alert("删除失败", "密码或验证码无效，或当前无法完成删除。");
          } finally {
            setLoading(false);
          }
        }
      }
    ]);
  };

  return (
    <ScrollView style={styles.container} contentContainerStyle={styles.content}>
      <Text style={styles.title}>删除账号 / Delete Account</Text>
      <Text style={styles.subtitle}>
        这是上线合规要求的一部分。删除后会移除联系人、通话历史、推送 token、权益和翻译用量，只保留非 PII 审计摘要。
      </Text>

      <View style={styles.card}>
        <TextField
          label="当前密码 / Current Password"
          secureTextEntry
          value={password}
          onChangeText={setPassword}
          editable={!loading}
        />
        <Text style={styles.helper}>如果你忘记密码，可以改用邮箱验证码确认删除。</Text>
        <VerificationCodeInput
          codeLength={6}
          onCodeChange={setCode}
          onCodeComplete={setCode}
          editable={!loading}
        />
        <PrimaryButton title="发送删除验证码" onPress={() => void sendCode()} disabled={loading} />
      </View>

      <PrimaryButton
        title={loading ? "删除中..." : "永久删除账号"}
        onPress={() => void handleDelete()}
        disabled={loading}
        style={styles.deleteButton}
      />
    </ScrollView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#fff7ed"
  },
  content: {
    padding: 18
  },
  title: {
    fontSize: 28,
    fontWeight: "800",
    color: "#7c2d12"
  },
  subtitle: {
    marginTop: 10,
    color: "#9a3412",
    lineHeight: 22,
    marginBottom: 18
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18,
    marginBottom: 18
  },
  helper: {
    color: "#9a3412",
    marginBottom: 12
  },
  deleteButton: {
    backgroundColor: "#dc2626"
  }
});

export default DeleteAccountScreen;
