import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  FlatList,
  RefreshControl,
  StyleSheet,
  Text,
  TouchableOpacity,
  View
} from "react-native";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { useFocusEffect } from "@react-navigation/native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { fetchCallHistory, type CallHistoryRecord } from "../api/commercial";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { useCommercial } from "../context/CommercialContext";
import { useFollowUps } from "../context/FollowUpContext";
import { useSignaling } from "../context/signalingContextValue";
import { RootStackParamList } from "../navigation/AppNavigator";
import AnalyticsService from "../services/AnalyticsService";
import { FOLLOW_UP_CALLS_STORAGE_KEY } from "../constants/invitations";

type Props = NativeStackScreenProps<RootStackParamList, "CallHistory">;

const CallHistoryScreen: React.FC<Props> = ({ navigation }) => {
  const { token, user } = useAuthContext();
  const { tier } = useCommercial();
  const { items: followUpItems, completeTask } = useFollowUps();
  const { startCall, connectionReady } = useSignaling();
  const [history, setHistory] = useState<CallHistoryRecord[]>([]);
  const [loading, setLoading] = useState(false);

  const loadHistory = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      setLoading(true);
      const data = await fetchCallHistory(token, tier === "premium" ? 365 : 30);
      setHistory(data);
    } catch (error) {
      console.error("[CallHistoryScreen] Failed to load call history:", error);
      Alert.alert("加载失败", "无法获取最近通话记录。");
    } finally {
      setLoading(false);
    }
  }, [tier, token]);

  useEffect(() => {
    void loadHistory();
  }, [loadHistory]);

  useFocusEffect(
    useCallback(() => {
      void loadHistory();
    }, [loadHistory])
  );

  const handleCallBack = useCallback(
    async (item: CallHistoryRecord, peerEmail: string) => {
      if (!connectionReady) {
        Alert.alert("正在重新连接", "信令服务暂时不可用，请稍后再试。");
        return;
      }
      try {
        const stored = await AsyncStorage.getItem(FOLLOW_UP_CALLS_STORAGE_KEY);
        const existing = stored ? JSON.parse(stored) as string[] : [];
        const next = Array.from(new Set([...existing, item.call_id]));
        await AsyncStorage.setItem(FOLLOW_UP_CALLS_STORAGE_KEY, JSON.stringify(next));
      } catch {
        // Ignore local follow-up storage failures.
      }
      AnalyticsService.track("missed_call_callback_started", { call_id: item.call_id, peer_email: peerEmail });
      const matchedTask = followUpItems.find((candidate) => candidate.task.call_id === item.call_id && candidate.task.type === "callback" && candidate.task.status !== "done");
      if (matchedTask) {
        await completeTask(matchedTask.task.id);
        AnalyticsService.track("followup_task_completed", { task_id: matchedTask.task.id, type: matchedTask.task.type });
      }
      startCall(peerEmail);
    },
    [completeTask, connectionReady, followUpItems, startCall]
  );

  const rows = useMemo(() => {
    return history.map((item) => {
      const isCaller = item.caller_email === user?.email;
      const peerEmail = isCaller ? item.callee_email : item.caller_email;
      const peerName = isCaller
        ? item.callee_display_name || item.callee_email
        : item.caller_display_name || item.caller_email;
      return {
        ...item,
        peerEmail,
        peerName,
        directionLabel: isCaller ? "拨出" : "来电",
        followupStatus: item.followup_status,
        nextTaskDueAt: item.next_task_due_at,
        isOverdue: Boolean(item.is_overdue),
        statusLabel:
          item.status === "missed"
            ? "未接"
            : item.status === "rejected"
              ? "已拒接"
              : item.status === "answered"
                ? "已接通"
                : item.status === "ended"
                  ? "已结束"
                  : item.status
      };
    }).sort((left, right) => {
      const leftPriority = left.status === "missed" ? 0 : left.isOverdue ? 1 : left.followupStatus ? 2 : 3;
      const rightPriority = right.status === "missed" ? 0 : right.isOverdue ? 1 : right.followupStatus ? 2 : 3;
      if (leftPriority !== rightPriority) {
        return leftPriority - rightPriority;
      }
      return new Date(right.started_at).getTime() - new Date(left.started_at).getTime();
    });
  }, [history, user?.email]);

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.title}>最近通话 / Recent Calls</Text>
        <Text style={styles.subtitle}>
          {tier === "premium" ? "保留 365 天记录" : "免费版保留最近 30 天"}
        </Text>
      </View>

      <FlatList
        data={rows}
        keyExtractor={(item) => `${item.id}`}
        refreshControl={<RefreshControl refreshing={loading} onRefresh={() => void loadHistory()} />}
        contentContainerStyle={styles.listContent}
        ListEmptyComponent={
          !loading ? (
            <View style={styles.emptyCard}>
              <Text style={styles.emptyTitle}>还没有通话记录</Text>
              <Text style={styles.emptyText}>完成首次通话后，这里会显示未接、已接和拒接记录。</Text>
            </View>
          ) : null
        }
        renderItem={({ item }) => (
          <View style={styles.row}>
            <View style={styles.rowText}>
              <Text style={styles.peerName}>{item.peerName}</Text>
              <Text style={styles.peerMeta}>
                {item.directionLabel} · {new Date(item.started_at).toLocaleString()}
              </Text>
              <View style={[styles.statusBadge, item.status === "missed" ? styles.missedBadge : item.status === "rejected" ? styles.rejectedBadge : styles.normalBadge]}>
                <Text style={styles.statusBadgeText}>{item.statusLabel}</Text>
              </View>
              {item.followupStatus ? (
                <Text style={[styles.followUpMeta, item.isOverdue ? styles.overdueText : undefined]}>
                  {item.isOverdue ? "已逾期" : `跟进状态: ${item.followupStatus}`}
                  {item.nextTaskDueAt ? ` · ${new Date(item.nextTaskDueAt).toLocaleString()}` : ""}
                </Text>
              ) : null}
            </View>
            <PrimaryButton
              title="回拨"
              style={styles.callButton}
              onPress={() => void handleCallBack(item, item.peerEmail)}
            />
          </View>
        )}
      />

      {tier !== "premium" ? (
        <TouchableOpacity
          style={styles.upgradeBanner}
          onPress={() => navigation.navigate("Subscription")}
        >
          <Text style={styles.upgradeTitle}>升级 Premium</Text>
          <Text style={styles.upgradeText}>解锁 365 天通话记录、高清画质档位和更多协作能力。</Text>
        </TouchableOpacity>
      ) : null}
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 16
  },
  header: {
    marginBottom: 16
  },
  title: {
    fontSize: 26,
    fontWeight: "800",
    color: "#0f172a"
  },
  subtitle: {
    color: "#64748b",
    marginTop: 6
  },
  listContent: {
    paddingBottom: 120
  },
  row: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginBottom: 12,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between"
  },
  rowText: {
    flex: 1,
    paddingRight: 12
  },
  peerName: {
    fontSize: 17,
    fontWeight: "700",
    color: "#0f172a"
  },
  peerMeta: {
    marginTop: 6,
    color: "#64748b"
  },
  statusBadge: {
    alignSelf: "flex-start",
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 4,
    marginTop: 10
  },
  statusBadgeText: {
    color: "#fff",
    fontWeight: "700",
    fontSize: 12
  },
  missedBadge: {
    backgroundColor: "#dc2626"
  },
  rejectedBadge: {
    backgroundColor: "#b45309"
  },
  normalBadge: {
    backgroundColor: "#475569"
  },
  callButton: {
    minWidth: 88
  },
  followUpMeta: {
    marginTop: 8,
    color: "#1d4ed8",
    fontWeight: "600"
  },
  overdueText: {
    color: "#b91c1c"
  },
  emptyCard: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 20,
    alignItems: "center"
  },
  emptyTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0f172a"
  },
  emptyText: {
    marginTop: 8,
    textAlign: "center",
    color: "#64748b"
  },
  upgradeBanner: {
    position: "absolute",
    left: 16,
    right: 16,
    bottom: 20,
    backgroundColor: "#0f172a",
    borderRadius: 18,
    padding: 18
  },
  upgradeTitle: {
    color: "#fff",
    fontWeight: "800",
    fontSize: 16
  },
  upgradeText: {
    marginTop: 6,
    color: "#cbd5e1"
  }
});

export default CallHistoryScreen;
