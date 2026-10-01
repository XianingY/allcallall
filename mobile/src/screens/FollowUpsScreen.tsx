import React, { useMemo, useRef, useState } from "react";
import { Alert, FlatList, RefreshControl, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { formatShortDateTime } from "@allcallall/shared";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import type { FollowUpListItem } from "../api/commercial";
import PrimaryButton from "../components/PrimaryButton";
import { useFollowUps } from "../context/FollowUpContext";
import { useSignaling } from "../context/signalingContextValue";
import { RootStackParamList } from "../navigation/AppNavigator";
import AnalyticsService from "../services/AnalyticsService";
import { createSingleFlight } from "./launchActionGuards";

type Props = NativeStackScreenProps<RootStackParamList, "FollowUps">;

const FollowUpsScreen: React.FC<Props> = ({ navigation }) => {
  const { items, loading, error, refreshFollowUps, completeTask } = useFollowUps();
  const { connectionReady, startCall, setTranslationLanguage, setTranslationSourceLanguage } = useSignaling();
  // completeTask rejects when the update fails; without the guard the
  // rejection escaped as an unhandled promise and the 回拨 button looked dead.
  const callbackFlight = useRef(createSingleFlight()).current;
  const [callbackPending, setCallbackPending] = useState(false);

  const sections = useMemo(() => items, [items]);

  const handleCallback = async (item: FollowUpListItem) => {
    if (!item.peer?.email) {
      return;
    }
    const peerEmail = item.peer.email;
    if (!connectionReady) {
      Alert.alert("正在重新连接", "信令服务暂时不可用，请稍后再试。");
      return;
    }
    if (callbackPending || callbackFlight.isBusy()) {
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
      AnalyticsService.track("missed_call_callback_started", { call_id: item.task.call_id, peer_email: peerEmail });
    }
    setCallbackPending(true);
    try {
      const result = await callbackFlight.run(async () => {
        await completeTask(item.task.id);
        startCall(peerEmail);
      });
      if (result.status === "error") {
        Alert.alert("回拨失败", "跟进任务更新失败，回拨未发起，请稍后再试。");
      }
    } finally {
      setCallbackPending(false);
    }
  };

  return (
    <View style={styles.container}>
      <FlatList
        data={sections}
        keyExtractor={(item) => `${item.task.id}`}
        refreshControl={<RefreshControl refreshing={loading} onRefresh={() => void refreshFollowUps()} />}
        contentContainerStyle={styles.content}
        ListEmptyComponent={
          loading ? null : error ? (
            // A failed load used to fall through to "nothing to follow up",
            // which reads as an empty account rather than a broken request.
            <View style={styles.emptyCard}>
              <Text style={styles.emptyTitle}>加载失败 / Could not load</Text>
              <Text style={styles.emptyText}>{error.message}</Text>
              <PrimaryButton title="重试 / Retry" onPress={() => void refreshFollowUps()} />
            </View>
          ) : (
            <View style={styles.emptyCard}>
              <Text style={styles.emptyTitle}>暂无待跟进任务</Text>
              <Text style={styles.emptyText}>通话完成后，系统会在这里汇总待回访联系人和跟进任务。</Text>
            </View>
          )
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
                  {item.task.due_at ? <Text style={styles.badge}>Due {formatShortDateTime(item.task.due_at)}</Text> : null}
                </View>
              </View>
              {item.task.type === "callback" && item.task.status !== "done" ? (
                <PrimaryButton
                  title={callbackPending ? "回拨中..." : "回拨"}
                  onPress={() => void handleCallback(item)}
                  disabled={callbackPending}
                  style={styles.button}
                />
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
