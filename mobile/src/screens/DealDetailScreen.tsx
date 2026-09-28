import React, { useCallback, useEffect, useState } from "react";
import { FlatList, StyleSheet, Text, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { DEAL_STATUS_LABELS, DEAL_STATUS_ORDER, isDealStatus, formatShortDateTime } from "@allcallall/shared";

import { fetchDeal, listDealActivities, updateDeal, type DealActivityRecord, type DealRecord } from "../api/collaboration";
import { useAuthContext } from "../context/AuthContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import LoadError from "../components/LoadError";
import PrimaryButton from "../components/PrimaryButton";
import { resolveLoadView } from "../components/loadViewState";

type Props = NativeStackScreenProps<RootStackParamList, "DealDetail">;

const LOAD_ERROR_MESSAGE = "无法读取商机详情。";

const DealDetailScreen: React.FC<Props> = ({ route, navigation }) => {
  const { deal: initialDeal } = route.params;
  const { token } = useAuthContext();
  const [deal, setDeal] = useState<DealRecord>(initialDeal);
  const [activities, setActivities] = useState<DealActivityRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      setLoading(true);
      setLoadError(null);
      const [freshDeal, freshActivities] = await Promise.all([
        fetchDeal(token, initialDeal.id),
        listDealActivities(token, initialDeal.id)
      ]);
      setDeal(freshDeal);
      setActivities(freshActivities);
    } catch (error) {
      console.error("[DealDetailScreen] Failed to load deal detail:", error);
      setLoadError(LOAD_ERROR_MESSAGE);
    } finally {
      setLoading(false);
    }
  }, [initialDeal.id, token]);

  useEffect(() => {
    void load();
  }, [load]);

  // Deals could be created on mobile but never advanced: this screen was
  // read-only, so changing an amount, moving a stage or marking a win/loss
  // all required going back to web. Status is the part that matters most and
  // needs no extra lookups, so it is editable here.
  const changeStatus = useCallback(
    async (status: string) => {
      if (!token || saving) {
        return;
      }
      setSaving(true);
      setActionError(null);
      try {
        const updated = await updateDeal(token, initialDeal.id, { status });
        setDeal(updated);
      } catch (error) {
        console.error("[DealDetailScreen] Failed to update deal status:", error);
        setActionError(error instanceof Error ? error.message : "无法更新商机状态。");
      } finally {
        setSaving(false);
      }
    },
    [initialDeal.id, saving, token],
  );

  const currentStatus = isDealStatus(deal.status) ? deal.status : DEAL_STATUS_ORDER[0];
  const view = resolveLoadView({ loading, error: loadError, itemCount: activities.length });

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

      <Text style={styles.sectionTitle}>状态</Text>
      <View style={styles.statusRow}>
        {DEAL_STATUS_ORDER.map((status) => (
          <PrimaryButton
            key={status}
            title={DEAL_STATUS_LABELS[status]}
            onPress={() => void changeStatus(status)}
            disabled={saving || status === currentStatus}
            style={styles.statusButton}
          />
        ))}
      </View>
      {actionError ? <Text style={styles.actionError}>{actionError}</Text> : null}

      <Text style={styles.sectionTitle}>最近活动</Text>
      {view === "error" && activities.length > 0 ? (
        <LoadError message={LOAD_ERROR_MESSAGE} onRetry={() => void load()} />
      ) : null}
      <FlatList
        data={activities}
        keyExtractor={(item) => String(item.id)}
        renderItem={({ item }) => (
          <View style={styles.activityCard}>
            <Text style={styles.activitySummary}>{item.summary}</Text>
            <Text style={styles.activityMeta}>
              {item.type} · {formatShortDateTime(item.created_at)}
            </Text>
          </View>
        )}
        ListEmptyComponent={
          view === "error" ? (
            <LoadError message={LOAD_ERROR_MESSAGE} onRetry={() => void load()} />
          ) : view === "loading" ? null : (
            <Text style={styles.empty}>暂无活动记录。</Text>
          )
        }
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
  },
  statusRow: {
    flexDirection: "row",
    gap: 8,
    marginBottom: 20
  },
  statusButton: {
    flex: 1
  },
  actionError: {
    color: "#b91c1c",
    marginBottom: 12,
    fontSize: 13
  }
});

export default DealDetailScreen;
