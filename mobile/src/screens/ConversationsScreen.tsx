import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, Pressable, StyleSheet, Text, TouchableOpacity, View, useWindowDimensions } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { createConversation, listConversations, type ConversationRecord } from "../api/collaboration";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import ChatRealtimeService from "../services/ChatRealtimeService";

type Props = NativeStackScreenProps<RootStackParamList, "Conversations">;
type InboxFilter = "my" | "open" | "pending" | "resolved" | "channels";

const FILTERS: Array<{ key: InboxFilter; label: string }> = [
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
  const [channelName, setChannelName] = useState("");
  const [activeFilter, setActiveFilter] = useState<InboxFilter>("my");
  const isWideScreen = width >= 1100;

  const loadData = useCallback(async () => {
    if (!token || !currentOrganization) {
      setItems([]);
      return;
    }
    try {
      setLoading(true);
      const data = await listConversations(token, activeFilter);
      setItems(data);
    } catch (error) {
      console.error("[ConversationsScreen] Failed to load conversations:", error);
      Alert.alert("加载失败", "无法加载协作线程。");
    } finally {
      setLoading(false);
    }
  }, [activeFilter, currentOrganization, token]);

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
        const maybePayload = payload.payload as {
          conversation_id?: number;
          changes?: Partial<ConversationRecord>;
        };
        if (maybePayload.conversation_id && maybePayload.changes) {
          setItems((current) => current.map((item) => (
            item.id === maybePayload.conversation_id ? { ...item, ...maybePayload.changes } : item
          )));
          return;
        }
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
    if (!token || !channelName.trim()) {
      return;
    }
    try {
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
    }
  };

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
            title="创建频道"
            onPress={handleCreateChannel}
            disabled={!channelName.trim()}
            style={styles.createButton}
          />
        </View>

        <View style={isWideScreen ? styles.desktopMain : undefined}>
          <FlatList
            data={items}
            keyExtractor={(item) => String(item.id)}
            refreshing={loading}
            onRefresh={() => void loadData()}
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
              <View style={styles.empty}>
                <Text style={styles.emptyText}>当前筛选下还没有协作线程。</Text>
              </View>
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
  }
});

export default ConversationsScreen;
