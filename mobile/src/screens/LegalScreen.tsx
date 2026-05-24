import React, { useEffect, useState } from "react";
import {
  Alert,
  Linking,
  ScrollView,
  StyleSheet,
  Text,
  TouchableOpacity,
  View
} from "react-native";

import { fetchCurrentLegal, type LegalInfo } from "../api/commercial";
import PrimaryButton from "../components/PrimaryButton";
import { useCommercial } from "../context/CommercialContext";

const LegalScreen: React.FC = () => {
  const { markLegalAccepted } = useCommercial();
  const [legal, setLegal] = useState<LegalInfo | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const load = async () => {
      try {
        setLegal(await fetchCurrentLegal());
      } catch (error) {
        console.error("[LegalScreen] Failed to fetch legal docs:", error);
      }
    };
    void load();
  }, []);

  const openURL = async (url: string) => {
    try {
      await Linking.openURL(url);
    } catch (error) {
      console.error("[LegalScreen] Failed to open legal URL:", error);
      Alert.alert("打开失败", "当前无法打开链接。");
    }
  };

  const openSupportEmail = async () => {
    if (!legal?.support_email) {
      Alert.alert("未配置", "当前环境未配置支持邮箱。");
      return;
    }
    await openURL(`mailto:${legal.support_email}`);
  };

  const handleAcknowledge = async () => {
    try {
      setLoading(true);
      await markLegalAccepted();
      Alert.alert("已记录", "已在当前账号下记录条款与隐私版本。");
    } catch (error) {
      console.error("[LegalScreen] Failed to accept legal:", error);
      Alert.alert("提交失败", "无法记录法律文档接受状态。");
    } finally {
      setLoading(false);
    }
  };

  return (
    <ScrollView style={styles.container} contentContainerStyle={styles.content}>
      <Text style={styles.title}>法律与合规 / Legal</Text>
      <Text style={styles.subtitle}>
        商用版要求条款、隐私政策和账号删除路径都可在应用内外访问。
      </Text>

      <View style={styles.card}>
        <Text style={styles.sectionTitle}>当前版本</Text>
        <Text style={styles.rowText}>Terms: {legal?.terms_version ?? "--"}</Text>
        <Text style={styles.rowText}>Privacy: {legal?.privacy_version ?? "--"}</Text>
      </View>

      <TouchableOpacity style={styles.linkCard} onPress={() => legal && void openURL(legal.terms_url)}>
        <Text style={styles.linkTitle}>服务条款</Text>
        <Text style={styles.linkValue}>{legal?.terms_url}</Text>
      </TouchableOpacity>

      <TouchableOpacity
        style={styles.linkCard}
        onPress={() => legal && void openURL(legal.privacy_policy_url)}
      >
        <Text style={styles.linkTitle}>隐私政策</Text>
        <Text style={styles.linkValue}>{legal?.privacy_policy_url}</Text>
      </TouchableOpacity>

      <TouchableOpacity
        style={styles.linkCard}
        onPress={() => legal && void openURL(legal.account_deletion_url)}
      >
        <Text style={styles.linkTitle}>账号删除说明</Text>
        <Text style={styles.linkValue}>{legal?.account_deletion_url}</Text>
      </TouchableOpacity>

      <View style={styles.card}>
        <Text style={styles.sectionTitle}>联系我们</Text>
        <Text style={styles.rowText}>{legal?.support_email ?? "未配置 / Not configured"}</Text>
        <PrimaryButton
          title="联系支持 / Contact Support"
          onPress={() => void openSupportEmail()}
          style={styles.supportButton}
        />
      </View>

      <PrimaryButton
        title={loading ? "提交中..." : "记录当前接受版本"}
        onPress={() => void handleAcknowledge()}
        disabled={loading}
      />
    </ScrollView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc"
  },
  content: {
    padding: 18
  },
  title: {
    fontSize: 28,
    fontWeight: "800",
    color: "#0f172a"
  },
  subtitle: {
    marginTop: 10,
    color: "#475569",
    lineHeight: 22,
    marginBottom: 20
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18,
    marginBottom: 12
  },
  sectionTitle: {
    fontWeight: "800",
    color: "#0f172a",
    marginBottom: 10
  },
  rowText: {
    color: "#475569",
    marginTop: 6
  },
  supportButton: {
    marginTop: 16
  },
  linkCard: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18,
    marginBottom: 12
  },
  linkTitle: {
    fontWeight: "800",
    color: "#0f172a"
  },
  linkValue: {
    marginTop: 8,
    color: "#2563eb"
  }
});

export default LegalScreen;
