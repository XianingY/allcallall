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
import { useFocusEffect } from "@react-navigation/native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { fetchCallHistory, type CallHistoryRecord } from "../api/commercial";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { useCommercial } from "../context/CommercialContext";
import { useSignaling } from "../context/SignalingContext";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "CallHistory">;

const CallHistoryScreen: React.FC<Props> = ({ navigation }) => {
  const { token, user } = useAuthContext();
  const { tier } = useCommercial();
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
    (peerEmail: string) => {
      if (!connectionReady) {
        Alert.alert("正在重新连接", "信令服务暂时不可用，请稍后再试。");
        return;
      }
      startCall(peerEmail);
    },
    [connectionReady, startCall]
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
            </View>
            <PrimaryButton
              title="回拨"
              style={styles.callButton}
              onPress={() => handleCallBack(item.peerEmail)}
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
          <Text style={styles.upgradeText}>解锁 365 天通话记录和无限实时翻译。</Text>
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
