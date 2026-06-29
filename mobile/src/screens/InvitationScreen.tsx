import React, { useMemo, useState } from "react";
import {
  Alert,
  KeyboardAvoidingView,
  Linking,
  Platform,
  Share,
  StyleSheet,
  Text,
  View
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import TextField from "../components/TextField";
import PrimaryButton from "../components/PrimaryButton";
import { RootStackParamList } from "../navigation/AppNavigator";
import { createInvitation } from "../api/users";
import { useAuthContext } from "../context/AuthContext";
import AnalyticsService from "../services/AnalyticsService";
import { useSignaling } from "../context/signalingContextValue";

type Props = NativeStackScreenProps<RootStackParamList, "Invitation">;

const InvitationScreen: React.FC<Props> = () => {
  const { token } = useAuthContext();
  const { translationSourceLanguage, translationLanguage } = useSignaling();
  const [targetEmail, setTargetEmail] = useState("");
  const [note, setNote] = useState("");
  const [loading, setLoading] = useState(false);
  const [shareURL, setShareURL] = useState<string | null>(null);
  const [inviteCode, setInviteCode] = useState<string | null>(null);

  const expiresAt = useMemo(() => {
    const value = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000);
    return value.toISOString();
  }, []);

  const handleCreate = async () => {
    if (!token) {
      return;
    }
    const email = targetEmail.trim().toLowerCase();
    if (!email) {
      Alert.alert("请输入邮箱", "邀请需要目标联系人邮箱。");
      return;
    }
    try {
      setLoading(true);
      const invitation = await createInvitation(token, {
        target_email: email,
        default_source_lang: translationSourceLanguage,
        default_target_lang: translationLanguage,
        note,
        expires_at: expiresAt
      });
      setShareURL(invitation.share_url);
      setInviteCode(invitation.code);
      AnalyticsService.track("invite_created", { target_email: email });
    } catch (error) {
      console.error("[InvitationScreen] Failed to create invitation:", error);
      Alert.alert("创建失败", "当前无法创建邀请，请稍后重试。");
    } finally {
      setLoading(false);
    }
  };

  const handleShare = async () => {
    if (!shareURL) {
      return;
    }
    try {
      await Share.share({
        message: `Join me on AllCallAll for a translated business call: ${shareURL}`,
        url: shareURL
      });
    } catch (error) {
      console.warn("[InvitationScreen] Share failed:", error);
    }
  };

  const handleOpen = async () => {
    if (!shareURL) {
      return;
    }
    try {
      await Linking.openURL(shareURL);
    } catch (error) {
      console.warn("[InvitationScreen] Open invitation link failed:", error);
    }
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      style={styles.container}
    >
      <View style={styles.card}>
        <Text style={styles.title}>邀请业务联系人试用</Text>
        <Text style={styles.subtitle}>
          生成一个深链邀请，让对方注册后自动成为联系人，并带上默认翻译方向。
        </Text>
        <TextField
          label="目标邮箱 / Target Email"
          autoCapitalize="none"
          keyboardType="email-address"
          value={targetEmail}
          onChangeText={setTargetEmail}
        />
        <TextField
          label="备注 / Note"
          value={note}
          onChangeText={setNote}
          multiline
          numberOfLines={3}
        />
        <View style={styles.languageRow}>
          <Text style={styles.languageMeta}>
            默认翻译: {translationSourceLanguage || "auto"} → {translationLanguage || "auto"}
          </Text>
        </View>
        <PrimaryButton
          title={loading ? "创建中..." : "生成邀请"}
          onPress={() => void handleCreate()}
          disabled={loading}
        />
        {shareURL ? (
          <View style={styles.resultCard}>
            <Text style={styles.resultTitle}>邀请已生成</Text>
            <Text style={styles.resultText}>{shareURL}</Text>
            {inviteCode ? <Text style={styles.resultMeta}>邀请码: {inviteCode}</Text> : null}
            <PrimaryButton title="分享邀请链接" onPress={() => void handleShare()} />
            <PrimaryButton title="打开邀请页" onPress={() => void handleOpen()} style={styles.secondaryButton} />
          </View>
        ) : null}
      </View>
    </KeyboardAvoidingView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 20
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 20
  },
  title: {
    fontSize: 24,
    fontWeight: "800",
    color: "#0f172a"
  },
  subtitle: {
    marginTop: 8,
    marginBottom: 18,
    color: "#475569",
    lineHeight: 21
  },
  languageRow: {
    marginBottom: 16
  },
  languageMeta: {
    color: "#0f172a",
    fontWeight: "600"
  },
  resultCard: {
    marginTop: 18,
    backgroundColor: "#eff6ff",
    borderRadius: 14,
    padding: 14,
    gap: 10
  },
  resultTitle: {
    fontSize: 16,
    fontWeight: "800",
    color: "#1d4ed8"
  },
  resultText: {
    color: "#1e293b"
  },
  resultMeta: {
    color: "#475569",
    fontWeight: "600"
  },
  secondaryButton: {
    backgroundColor: "#334155"
  }
});

export default InvitationScreen;
