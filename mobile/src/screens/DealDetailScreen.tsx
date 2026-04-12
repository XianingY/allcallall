import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, StyleSheet, Text, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { fetchDeal, listDealActivities, type DealActivityRecord, type DealRecord } from "../api/collaboration";
import { useAuthContext } from "../context/AuthContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import PrimaryButton from "../components/PrimaryButton";

type Props = NativeStackScreenProps<RootStackParamList, "DealDetail">;

const DealDetailScreen: React.FC<Props> = ({ route, navigation }) => {
  const { deal: initialDeal } = route.params;
  const { token } = useAuthContext();
  const [deal, setDeal] = useState<DealRecord>(initialDeal);
  const [activities, setActivities] = useState<DealActivityRecord[]>([]);

  const load = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      const [freshDeal, freshActivities] = await Promise.all([
        fetchDeal(token, initialDeal.id),
        listDealActivities(token, initialDeal.id)
      ]);
      setDeal(freshDeal);
      setActivities(freshActivities);
    } catch (error) {
      console.error("[DealDetailScreen] Failed to load deal detail:", error);
      Alert.alert("加载失败", "无法加载商机详情。");
    }
  }, [initialDeal.id, token]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <View style={styles.container}>
      <Text style={styles.title}>{deal.title}</Text>
      <Text style={styles.stage}>{deal.stage_name || deal.status}</Text>
      <Text style={styles.value}>
        {deal.currency} {(deal.value_cents / 100).toFixed(2)}
      </Text>
      {deal.description ? <Text style={styles.description}>{deal.description}</Text> : null}

      <View style={styles.actions}>
        <PrimaryButton
          title="打开聊天"
          onPress={() => navigation.navigate("Conversations")}
          style={styles.actionButton}
        />
        <PrimaryButton
          title="返回联系人"
          onPress={() => navigation.navigate("Contacts")}
          style={styles.actionButton}
        />
      </View>

      <Text style={styles.sectionTitle}>最近活动</Text>
      <FlatList
        data={activities}
        keyExtractor={(item) => String(item.id)}
        renderItem={({ item }) => (
          <View style={styles.activityCard}>
            <Text style={styles.activitySummary}>{item.summary}</Text>
            <Text style={styles.activityMeta}>
              {item.type} · {new Date(item.created_at).toLocaleString()}
            </Text>
          </View>
        )}
        ListEmptyComponent={<Text style={styles.empty}>暂无活动记录。</Text>}
      />
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 16
  },
  title: {
    fontSize: 24,
    fontWeight: "700",
    color: "#0f172a"
  },
  stage: {
    marginTop: 8,
    color: "#2563eb",
    fontWeight: "600"
  },
  value: {
    marginTop: 6,
    color: "#334155"
  },
  description: {
    marginTop: 12,
    color: "#475569",
    lineHeight: 20
  },
  actions: {
    flexDirection: "row",
    gap: 12,
    marginTop: 16,
    marginBottom: 20
  },
  actionButton: {
    flex: 1
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "600",
    color: "#0f172a",
    marginBottom: 12
  },
  activityCard: {
    backgroundColor: "#fff",
    borderRadius: 14,
    borderWidth: 1,
    borderColor: "#e2e8f0",
    padding: 14,
    marginBottom: 10
  },
  activitySummary: {
    color: "#0f172a"
  },
  activityMeta: {
    color: "#64748b",
    marginTop: 6,
    fontSize: 12
  },
  empty: {
    color: "#64748b"
  }
});

export default DealDetailScreen;
