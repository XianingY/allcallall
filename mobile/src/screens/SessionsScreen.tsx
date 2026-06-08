import React from "react";
import {
  ActivityIndicator,
  Alert,
  Platform,
  SafeAreaView,
  ScrollView,
  StyleSheet,
  Text,
  TouchableOpacity,
  View
} from "react-native";

import { listRefreshSessions, RefreshSessionRecord, revokeRefreshSession } from "../api/auth";
import { useAuthContext } from "../context/AuthContext";

const statusLabels: Record<RefreshSessionRecord["status"], string> = {
  active: "活跃 / Active",
  expired: "已过期 / Expired",
  revoked: "已撤销 / Revoked"
};

const formatDateTime = (value?: string | null) => {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "—";
  }
  return date.toLocaleString();
};

const SessionsScreen: React.FC = () => {
  const { token } = useAuthContext();
  const [sessions, setSessions] = React.useState<RefreshSessionRecord[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [revokingSessionId, setRevokingSessionId] = React.useState<number | null>(null);

  const loadSessions = React.useCallback(async () => {
    if (!token) {
      return;
    }
    setLoading(true);
    try {
      setSessions(await listRefreshSessions(token));
    } catch (error) {
      console.warn("[SessionsScreen] Failed to load refresh sessions:", error);
      Alert.alert("加载失败 / Load failed", "当前无法读取登录会话列表。");
    } finally {
      setLoading(false);
    }
  }, [token]);

  React.useEffect(() => {
    void loadSessions();
  }, [loadSessions]);

  const revokeSession = React.useCallback(async (session: RefreshSessionRecord) => {
    if (!token || session.current || session.status !== "active") {
      return;
    }
    setRevokingSessionId(session.id);
    try {
      await revokeRefreshSession(token, session.id);
      await loadSessions();
    } catch (error) {
      console.warn("[SessionsScreen] Failed to revoke refresh session:", error);
      Alert.alert("撤销失败 / Revoke failed", "当前无法撤销这个登录会话，请稍后再试。");
    } finally {
      setRevokingSessionId(null);
    }
  }, [loadSessions, token]);

  const confirmRevokeSession = React.useCallback((session: RefreshSessionRecord) => {
    const message = "这会阻止该设备继续刷新登录状态；已签发的短期 access token 可能会在过期前继续有效。";
    if (Platform.OS === "web" && typeof window !== "undefined") {
      if (window.confirm(message)) {
        void revokeSession(session);
      }
      return;
    }
    Alert.alert("撤销会话 / Revoke session", message, [
      { text: "取消 / Cancel", style: "cancel" },
      { text: "撤销 / Revoke", style: "destructive", onPress: () => void revokeSession(session) }
    ]);
  }, [revokeSession]);

  return (
    <SafeAreaView style={styles.container}>
      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.headerCard}>
          <Text style={styles.title}>登录会话 / Active Sessions</Text>
          <Text style={styles.description}>
            这里展示当前账号最近的 Web/Desktop/移动端 refresh session。列表不包含任何 token 或 token hash。
          </Text>
          <TouchableOpacity style={styles.refreshButton} onPress={() => void loadSessions()} disabled={loading}>
            <Text style={styles.refreshButtonText}>
              {loading ? "刷新中..." : "刷新 / Refresh"}
            </Text>
          </TouchableOpacity>
        </View>

        {loading && sessions.length === 0 ? (
          <View style={styles.loadingBox}>
            <ActivityIndicator />
            <Text style={styles.mutedText}>正在加载登录会话...</Text>
          </View>
        ) : sessions.length === 0 ? (
          <View style={styles.emptyBox}>
            <Text style={styles.emptyTitle}>暂无会话记录</Text>
            <Text style={styles.mutedText}>登录或刷新会话后，这里会显示脱敏的设备记录。</Text>
          </View>
        ) : (
          sessions.map((session) => (
            <View key={session.id} style={[styles.sessionCard, session.current && styles.currentCard]}>
              <View style={styles.sessionHeader}>
                <Text style={styles.sessionTitle}>
                  {session.current ? "当前设备 / Current device" : `Session #${session.id}`}
                </Text>
                <View style={[styles.statusPill, styles[session.status]]}>
                  <Text style={styles.statusText}>{statusLabels[session.status]}</Text>
                </View>
              </View>
              <Text style={styles.userAgent} numberOfLines={2}>
                {session.user_agent || "Unknown client"}
              </Text>
              <View style={styles.metaGrid}>
                <SessionMeta label="IP" value={session.ip_address || "—"} />
                <SessionMeta label="创建 / Created" value={formatDateTime(session.created_at)} />
                <SessionMeta label="最近使用 / Last used" value={formatDateTime(session.last_used_at)} />
                <SessionMeta label="过期 / Expires" value={formatDateTime(session.expires_at)} />
                <SessionMeta label="撤销 / Revoked" value={formatDateTime(session.revoked_at)} />
                <SessionMeta label="异常重放 / Invalid reuse" value={String(session.invalid_use_count ?? 0)} />
              </View>
              {session.last_invalid_use_at ? (
                <Text style={styles.warningText}>
                  最近一次异常重放：{formatDateTime(session.last_invalid_use_at)}
                </Text>
              ) : null}
              {session.status === "active" && !session.current ? (
                <TouchableOpacity
                  style={styles.revokeButton}
                  onPress={() => confirmRevokeSession(session)}
                  disabled={revokingSessionId === session.id}
                >
                  <Text style={styles.revokeButtonText}>
                    {revokingSessionId === session.id ? "撤销中..." : "撤销续期 / Revoke refresh"}
                  </Text>
                </TouchableOpacity>
              ) : session.current ? (
                <Text style={styles.currentHint}>
                  当前设备请使用“退出所有设备”或普通退出入口。
                </Text>
              ) : null}
            </View>
          ))
        )}
      </ScrollView>
    </SafeAreaView>
  );
};

const SessionMeta: React.FC<{ label: string; value: string }> = ({ label, value }) => (
  <View style={styles.metaItem}>
    <Text style={styles.metaLabel}>{label}</Text>
    <Text style={styles.metaValue}>{value}</Text>
  </View>
);

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f5f7fb"
  },
  content: {
    padding: 16
  },
  headerCard: {
    backgroundColor: "#ffffff",
    borderRadius: 16,
    padding: 16,
    marginBottom: 16,
    borderWidth: 1,
    borderColor: "#e5e7eb"
  },
  title: {
    fontSize: 20,
    fontWeight: "700",
    color: "#111827",
    marginBottom: 8
  },
  description: {
    fontSize: 13,
    lineHeight: 19,
    color: "#6b7280",
    marginBottom: 14
  },
  refreshButton: {
    alignSelf: "flex-start",
    backgroundColor: "#0f172a",
    borderRadius: 10,
    paddingHorizontal: 14,
    paddingVertical: 10
  },
  refreshButtonText: {
    color: "#ffffff",
    fontSize: 13,
    fontWeight: "700"
  },
  loadingBox: {
    alignItems: "center",
    padding: 24,
    gap: 10
  },
  emptyBox: {
    backgroundColor: "#ffffff",
    borderRadius: 16,
    padding: 18,
    borderWidth: 1,
    borderColor: "#e5e7eb"
  },
  emptyTitle: {
    fontSize: 16,
    fontWeight: "700",
    color: "#111827",
    marginBottom: 6
  },
  mutedText: {
    fontSize: 13,
    color: "#6b7280"
  },
  sessionCard: {
    backgroundColor: "#ffffff",
    borderRadius: 16,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: "#e5e7eb"
  },
  currentCard: {
    borderColor: "#2563eb",
    backgroundColor: "#eff6ff"
  },
  sessionHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
    marginBottom: 8
  },
  sessionTitle: {
    flex: 1,
    fontSize: 16,
    fontWeight: "700",
    color: "#111827"
  },
  statusPill: {
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 5
  },
  active: {
    backgroundColor: "#dcfce7"
  },
  expired: {
    backgroundColor: "#f3f4f6"
  },
  revoked: {
    backgroundColor: "#fee2e2"
  },
  statusText: {
    fontSize: 12,
    fontWeight: "700",
    color: "#111827"
  },
  userAgent: {
    fontSize: 13,
    color: "#374151",
    marginBottom: 12
  },
  metaGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    marginHorizontal: -4
  },
  metaItem: {
    width: "50%",
    paddingHorizontal: 4,
    marginBottom: 10
  },
  metaLabel: {
    fontSize: 11,
    color: "#6b7280",
    marginBottom: 3
  },
  metaValue: {
    fontSize: 13,
    color: "#111827",
    fontWeight: "600"
  },
  warningText: {
    color: "#b45309",
    fontSize: 12,
    fontWeight: "600",
    marginTop: 4
  },
  revokeButton: {
    alignSelf: "flex-start",
    borderRadius: 10,
    borderWidth: 1,
    borderColor: "#dc2626",
    paddingHorizontal: 12,
    paddingVertical: 9,
    marginTop: 10,
    backgroundColor: "#fff1f2"
  },
  revokeButtonText: {
    color: "#b91c1c",
    fontSize: 13,
    fontWeight: "700"
  },
  currentHint: {
    color: "#1d4ed8",
    fontSize: 12,
    fontWeight: "600",
    marginTop: 10
  }
});

export default SessionsScreen;
