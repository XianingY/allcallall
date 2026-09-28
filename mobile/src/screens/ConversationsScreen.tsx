import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, Pressable, StyleSheet, Text, TouchableOpacity, View, useWindowDimensions } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { createConversation, listConversations, type ConversationRecord } from "../api/collaboration";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import LoadError from "../components/LoadError";
import { resolveLoadView } from "../components/loadViewState";
import { canSubmitInput } from "../components/submitGuard";
import ChatRealtimeService from "../services/ChatRealtimeService";
import {
  applyConversationListPatch,
  type ConversationUpdatedPayload,
} from "../services/conversationRealtimeReducer";

type Props = NativeStackScreenProps<RootStackParamList, "Conversations">;
// "" is the server's "no filter" value (see conversation_service.go). These
// values are the status enum the backend accepts; anything else is treated as
// "all" there, so a typo silently shows everything.
type InboxFilter = "" | "my" | "open" | "pending" | "resolved" | "channels";

const LOAD_ERROR_MESSAGE = "无法读取协作线程。";

const FILTERS: Array<{ key: InboxFilter; label: string }> = [
  // Must come first and be the default: "my" means assignee = me, and a newly
  // created conversation has no assignee, so starting on "My" hides exactly
  // the conversation the user just created. Web defaults to "all" too.
  { key: "", label: "All" },
  { key: "my", label: "My" },
  { key: "open", label: "Open" },
  { key: "pending", label: "Pending" },
  { key: "resolved", label: "Resolved" },
  { key: "channels", label: "Channels" }
];

const ConversationsScreen: React.FC<Props> = ({ navigation }) => {
  const { token } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const { width } = useWindowDimensions();
  const [items, setItems] = useState<ConversationRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [convOffset, setConvOffset] = useState(0);
  const [convHasMore, setConvHasMore] = useState(false);
  const [convTotal, setConvTotal] = useState(0);
  const [channelName, setChannelName] = useState("");
  const [creating, setCreating] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [activeFilter, setActiveFilter] = useState<InboxFilter>("");
  const isWideScreen = width >= 1100;

  const loadData = useCallback(async () => {
    if (!token || !currentOrganization) {
      setItems([]);
      return;
    }
    try {
      setLoading(true);
      setLoadError(null);
      const data = await listConversations(token, activeFilter, undefined, { limit: 50, offset: 0 });
      setItems(data.conversations);
      setConvOffset(data.conversations.length);
      setConvHasMore(data.pagination.has_more);
      setConvTotal(data.pagination.total);
    } catch (error) {
      console.error("[ConversationsScreen] Failed to load conversations:", error);
      setLoadError(LOAD_ERROR_MESSAGE);
    } finally {
      setLoading(false);
    }
  }, [activeFilter, currentOrganization, token]);

  const loadMore = useCallback(async () => {
    if (!token || !currentOrganization || loadingMore || !convHasMore) {
      return;
    }
    try {
      setLoadingMore(true);
      const data = await listConversations(token, activeFilter, undefined, { limit: 50, offset: convOffset });
      setItems((previous) => [...previous, ...data.conversations]);
      setConvOffset((previous) => previous + data.conversations.length);
      setConvHasMore(data.pagination.has_more);
      setConvTotal(data.pagination.total);
    } catch (error) {
      console.error("[ConversationsScreen] Failed to load more conversations:", error);
      Alert.alert("加载失败", "无法加载更多协作线程。");
    } finally {
      setLoadingMore(false);
    }
  }, [activeFilter, convHasMore, convOffset, currentOrganization, loadingMore, token]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  useEffect(() => {
    if (!token || !currentOrganization) {
      ChatRealtimeService.disconnect();
      return;
    }
    const handleOpen = () => {
      void loadData();
    };
    const handleEvent = (payload: { event: string; payload?: unknown }) => {
      if (payload.event === "conversation.updated" && payload.payload && typeof payload.payload === "object") {
        setItems((current) => applyConversationListPatch(current, payload.payload as ConversationUpdatedPayload));
        return;
      }
      if (["message.created", "conversation.note.created"].includes(payload.event)) {
        void loadData();
      }
    };
    ChatRealtimeService.connect(token, currentOrganization.id);
    ChatRealtimeService.on("open", handleOpen);
    ChatRealtimeService.on("event", handleEvent);
    return () => {
      ChatRealtimeService.off("open", handleOpen);
      ChatRealtimeService.off("event", handleEvent);
    };
  }, [currentOrganization, loadData, token]);

  const handleCreateChannel = async () => {
    if (!token || !canSubmitInput(channelName, creating)) {
      return;
    }
    try {
      setCreating(true);
      const conversation = await createConversation(token, {
        type: "channel",
        title: channelName.trim()
      });
      setChannelName("");
      await loadData();
      navigation.navigate("ConversationDetail", { conversation });
    } catch (error) {
      console.error("[ConversationsScreen] Failed to create channel:", error);
      Alert.alert("创建失败", "无法创建团队频道。");
    } finally {
      setCreating(false);
    }
  };

  const view = resolveLoadView({ loading, error: loadError, itemCount: items.length });

  return (
    <View style={styles.container}>
      <View style={isWideScreen ? styles.desktopLayout : undefined}>
        <View style={isWideScreen ? styles.desktopSidebar : undefined}>
          <Text style={styles.heading}>
            {currentOrganization?.name ?? "当前工作区"} Inbox
          </Text>
          <Text style={styles.subheading}>围绕负责人、状态和会议推进团队协作。</Text>

          <View style={styles.filterRow}>
            {FILTERS.map((filter) => (
              <Pressable
                key={filter.key}
                style={[styles.filterChip, activeFilter === filter.key && styles.filterChipActive]}
                onPress={() => setActiveFilter(filter.key)}
              >
                <Text style={[styles.filterText, activeFilter === filter.key && styles.filterTextActive]}>{filter.label}</Text>
              </Pressable>
            ))}
          </View>

          <TextField
            label="新建频道"
            value={channelName}
            onChangeText={setChannelName}
            placeholder="例如：跨境客服升级处理"
          />
          <PrimaryButton
            title={creating ? "创建中…" : "创建频道"}
            onPress={handleCreateChannel}
            disabled={!canSubmitInput(channelName, creating)}
            style={styles.createButton}
          />
        </View>

        <View style={isWideScreen ? styles.desktopMain : undefined}>
          {view === "error" && items.length > 0 ? (
            <LoadError message={LOAD_ERROR_MESSAGE} onRetry={() => void loadData()} />
          ) : null}
          <FlatList
            data={items}
            keyExtractor={(item) => String(item.id)}
            refreshing={loading}
            onRefresh={() => void loadData()}
            onEndReached={() => {
              if (convHasMore && !loadingMore) {
                void loadMore();
              }
            }}
            onEndReachedThreshold={0.2}
            ListFooterComponent={
              convTotal > 0 ? (
                <View style={styles.footer}>
                  <Text style={styles.footerText}>共 {convTotal} 个会话</Text>
                  {convHasMore ? (
                    <TouchableOpacity
                      style={styles.footerButton}
                      onPress={() => void loadMore()}
                      disabled={loadingMore}
                    >
                      <Text style={styles.footerButtonText}>{loadingMore ? "加载中…" : "加载更多"}</Text>
                    </TouchableOpacity>
                  ) : null}
                </View>
              ) : null
            }
            renderItem={({ item }) => {
              const assignee = item.assignee_display_name || item.assignee_email || "未指派";
              return (
                <TouchableOpacity
                  style={styles.card}
                  onPress={() => navigation.navigate("ConversationDetail", { conversation: item })}
                >
                  <View style={styles.row}>
                    <Text style={styles.title}>{item.title || item.type}</Text>
                    {item.unread_count > 0 ? <Text style={styles.badge}>{item.unread_count}</Text> : null}
                  </View>
                  <View style={styles.metaRow}>
                    <Text style={styles.metaPill}>{item.status.toUpperCase()}</Text>
                    <Text style={styles.metaPill}>{item.priority.toUpperCase()}</Text>
                    {item.active_room_id ? <Text style={styles.metaPill}>MEETING</Text> : null}
                    {item.latest_recording_id ? <Text style={styles.metaPill}>RECORDING</Text> : null}
                  </View>
                  <Text style={styles.assignee}>负责人 {assignee}</Text>
                  {item.active_room_title || item.latest_room_title ? (
                    <Text style={styles.roomHint}>会议 {item.active_room_title || item.latest_room_title}</Text>
                  ) : null}
                  <Text style={styles.preview}>{item.last_message_preview || "暂无消息"}</Text>
                </TouchableOpacity>
              );
            }}
            ListEmptyComponent={
              view === "error" ? (
                <LoadError message={LOAD_ERROR_MESSAGE} onRetry={() => void loadData()} />
              ) : view === "loading" ? null : (
                <View style={styles.empty}>
                  <Text style={styles.emptyText}>当前筛选下还没有协作线程。</Text>
                </View>
              )
            }
          />
        </View>
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 16
  },
  desktopLayout: {
    flex: 1,
    flexDirection: "row",
    gap: 18,
  },
  desktopSidebar: {
    width: 320,
  },
  desktopMain: {
    flex: 1,
  },
  heading: {
    fontSize: 22,
    fontWeight: "700",
    color: "#0f172a"
  },
  subheading: {
    marginTop: 6,
    marginBottom: 14,
    color: "#475569"
  },
  filterRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginBottom: 12
  },
  filterChip: {
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "#cbd5e1",
    paddingHorizontal: 12,
    paddingVertical: 8,
    backgroundColor: "#fff"
  },
  filterChipActive: {
    backgroundColor: "#0f172a",
    borderColor: "#0f172a"
  },
  filterText: {
    color: "#334155",
    fontWeight: "600"
  },
  filterTextActive: {
    color: "#fff"
  },
  createButton: {
    marginBottom: 16
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: "#e2e8f0"
  },
  row: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center"
  },
  metaRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 10
  },
  title: {
    fontSize: 17,
    fontWeight: "600",
    color: "#0f172a"
  },
  badge: {
    backgroundColor: "#2563eb",
    color: "#fff",
    paddingHorizontal: 10,
    paddingVertical: 3,
    borderRadius: 999,
    overflow: "hidden"
  },
  metaPill: {
    color: "#334155",
    backgroundColor: "#e2e8f0",
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 4,
    overflow: "hidden",
    fontSize: 12,
    fontWeight: "600"
  },
  assignee: {
    color: "#475569",
    marginTop: 10
  },
  roomHint: {
    color: "#0f172a",
    marginTop: 6,
    fontWeight: "600"
  },
  preview: {
    color: "#334155",
    marginTop: 8
  },
  empty: {
    alignItems: "center",
    paddingTop: 48
  },
  emptyText: {
    color: "#64748b"
  },
  footer: {
    alignItems: "center",
    paddingVertical: 16,
    gap: 8
  },
  footerText: {
    color: "#64748b"
  },
  footerButton: {
    borderRadius: 999,
    borderWidth: 1,
    borderColor: "#cbd5e1",
    paddingHorizontal: 16,
    paddingVertical: 8,
    backgroundColor: "#fff"
  },
  footerButtonText: {
    color: "#334155",
    fontWeight: "600"
  }
});

export default ConversationsScreen;
