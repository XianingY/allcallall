import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  FlatList,
  RefreshControl,
  StyleSheet,
  Text,
  View
} from "react-native";
import { useFocusEffect } from "@react-navigation/native";

import { listBlocks, removeBlock, type UserBlockRecord } from "../api/commercial";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";

const BlockedUsersScreen: React.FC = () => {
  const { token } = useAuthContext();
  const [blocks, setBlocks] = useState<UserBlockRecord[]>([]);
  const [loading, setLoading] = useState(false);

  const loadBlocks = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      setLoading(true);
      setBlocks(await listBlocks(token));
    } catch (error) {
      console.error("[BlockedUsersScreen] Failed to load blocks:", error);
      Alert.alert("加载失败", "无法获取黑名单列表。");
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    void loadBlocks();
  }, [loadBlocks]);

  useFocusEffect(
    useCallback(() => {
      void loadBlocks();
    }, [loadBlocks])
  );

  const handleUnblock = async (blockedUserId: number) => {
    if (!token) {
      return;
    }
    try {
      await removeBlock(token, blockedUserId);
      await loadBlocks();
    } catch (error) {
      console.error("[BlockedUsersScreen] Failed to unblock user:", error);
      Alert.alert("解除失败", "无法解除拉黑。");
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.title}>已拉黑用户 / Blocked Users</Text>
      <Text style={styles.subtitle}>拉黑后，对方将无法搜索、加联系人或呼叫你。</Text>
      <FlatList
        data={blocks}
        keyExtractor={(item) => `${item.id}`}
        refreshControl={<RefreshControl refreshing={loading} onRefresh={() => void loadBlocks()} />}
        contentContainerStyle={styles.listContent}
        ListEmptyComponent={
          !loading ? (
            <View style={styles.emptyCard}>
              <Text style={styles.emptyText}>当前没有已拉黑用户。</Text>
            </View>
          ) : null
        }
        renderItem={({ item }) => (
          <View style={styles.row}>
            <View style={styles.rowText}>
              <Text style={styles.rowTitle}>
                {item.blocked_user_display_name || item.blocked_user_email || `用户 ID #${item.blocked_user_id}`}
              </Text>
              {item.blocked_user_email ? (
                <Text style={styles.rowSubTitle}>{item.blocked_user_email}</Text>
              ) : null}
              <Text style={styles.rowMeta}>拉黑时间 {new Date(item.created_at).toLocaleString()}</Text>
              {item.blocked_user_status === "deleted" ? (
                <Text style={styles.deletedMeta}>该账号已删除</Text>
              ) : null}
            </View>
            <PrimaryButton title="解除" onPress={() => void handleUnblock(item.blocked_user_id)} />
          </View>
        )}
      />
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 18
  },
  title: {
    fontSize: 28,
    fontWeight: "800",
    color: "#0f172a"
  },
  subtitle: {
    marginTop: 8,
    color: "#475569",
    marginBottom: 16
  },
  listContent: {
    paddingBottom: 30
  },
  emptyCard: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 20,
    alignItems: "center"
  },
  emptyText: {
    color: "#64748b"
  },
  row: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginBottom: 12,
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center"
  },
  rowText: {
    flex: 1,
    paddingRight: 12
  },
  rowTitle: {
    fontWeight: "700",
    color: "#0f172a"
  },
  rowSubTitle: {
    marginTop: 4,
    color: "#475569"
  },
  rowMeta: {
    marginTop: 6,
    color: "#64748b"
  },
  deletedMeta: {
    marginTop: 6,
    color: "#b45309",
    fontWeight: "600"
  }
});

export default BlockedUsersScreen;
