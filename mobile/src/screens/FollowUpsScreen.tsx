import React, { useMemo } from "react";
import { Alert, FlatList, RefreshControl, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import type { FollowUpListItem } from "../api/commercial";
import PrimaryButton from "../components/PrimaryButton";
import { useFollowUps } from "../context/FollowUpContext";
import { useSignaling } from "../context/SignalingContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import AnalyticsService from "../services/AnalyticsService";

type Props = NativeStackScreenProps<RootStackParamList, "FollowUps">;

const FollowUpsScreen: React.FC<Props> = ({ navigation }) => {
  const { items, loading, refreshFollowUps, completeTask } = useFollowUps();
  const { connectionReady, startCall, setTranslationLanguage, setTranslationSourceLanguage } = useSignaling();

  const sections = useMemo(() => items, [items]);

  const handleCallback = async (item: FollowUpListItem) => {
    if (!item.peer?.email) {
      return;
    }
    if (!connectionReady) {
      Alert.alert("正在重新连接", "信令服务暂时不可用，请稍后再试。");
      return;
    }
    if (item.contact?.default_source_lang) {
      setTranslationSourceLanguage(item.contact.default_source_lang);
    }
    if (item.contact?.default_target_lang) {
      setTranslationLanguage(item.contact.default_target_lang);
    }
    AnalyticsService.track("followup_task_completed", { task_id: item.task.id, type: item.task.type });
    if (item.task.call_id) {
      AnalyticsService.track("missed_call_callback_started", { call_id: item.task.call_id, peer_email: item.peer.email });
    }
    await completeTask(item.task.id);
    startCall(item.peer.email);
  };

  return (
    <View style={styles.container}>
      <FlatList
        data={sections}
        keyExtractor={(item) => `${item.task.id}`}
        refreshControl={<RefreshControl refreshing={loading} onRefresh={() => void refreshFollowUps()} />}
        contentContainerStyle={styles.content}
        ListEmptyComponent={
          !loading ? (
            <View style={styles.emptyCard}>
              <Text style={styles.emptyTitle}>暂无待跟进任务</Text>
              <Text style={styles.emptyText}>通话完成后，系统会在这里汇总待回访联系人和跟进任务。</Text>
            </View>
          ) : null
        }
        renderItem={({ item }) => (
          <TouchableOpacity
            style={styles.card}
            onPress={() => {
              if (item.peer) {
                navigation.navigate("ContactDetail", {
                  contact: {
                    id: item.peer.id,
                    email: item.peer.email,
                    display_name: item.peer.display_name,
                    profile: item.contact
                  }
                });
              }
            }}
          >
            <View style={styles.row}>
              <View style={styles.textCol}>
                <Text style={styles.title}>{item.peer?.display_name || item.peer?.email || item.task.title}</Text>
                <Text style={styles.meta}>{item.task.title}</Text>
                {item.followup?.summary_cn ? <Text style={styles.summary}>{item.followup.summary_cn}</Text> : null}
                <View style={styles.badges}>
                  {item.is_overdue ? <Text style={[styles.badge, styles.overdue]}>Overdue</Text> : null}
                  {item.task.status === "done" ? <Text style={[styles.badge, styles.done]}>Done</Text> : null}
                  {item.task.due_at ? <Text style={styles.badge}>Due {new Date(item.task.due_at).toLocaleString()}</Text> : null}
                </View>
              </View>
              {item.task.type === "callback" && item.task.status !== "done" ? (
                <PrimaryButton title="回拨" onPress={() => void handleCallback(item)} style={styles.button} />
              ) : null}
            </View>
          </TouchableOpacity>
        )}
      />
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc"
  },
  content: {
    padding: 16,
    paddingBottom: 48
  },
  emptyCard: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 24,
    alignItems: "center"
  },
  emptyTitle: {
    fontSize: 18,
    fontWeight: "800",
    color: "#0f172a"
  },
  emptyText: {
    marginTop: 8,
    color: "#64748b",
    textAlign: "center"
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 16,
    marginBottom: 12
  },
  row: {
    flexDirection: "row",
    alignItems: "center"
  },
  textCol: {
    flex: 1,
    paddingRight: 12
  },
  title: {
    fontSize: 17,
    fontWeight: "800",
    color: "#0f172a"
  },
  meta: {
    marginTop: 6,
    color: "#334155",
    fontWeight: "600"
  },
  summary: {
    marginTop: 8,
    color: "#475569",
    lineHeight: 20
  },
  badges: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 10
  },
  badge: {
    backgroundColor: "#e2e8f0",
    color: "#0f172a",
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 4,
    overflow: "hidden",
    fontSize: 12,
    fontWeight: "700"
  },
  overdue: {
    backgroundColor: "#fee2e2",
    color: "#b91c1c"
  },
  done: {
    backgroundColor: "#dcfce7",
    color: "#166534"
  },
  button: {
    minWidth: 80
  }
});

export default FollowUpsScreen;
