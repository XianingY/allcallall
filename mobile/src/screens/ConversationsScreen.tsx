import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { createConversation, listConversations, type ConversationRecord } from "../api/collaboration";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import ChatRealtimeService from "../services/ChatRealtimeService";

type Props = NativeStackScreenProps<RootStackParamList, "Conversations">;

const ConversationsScreen: React.FC<Props> = ({ navigation }) => {
  const { token } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const [items, setItems] = useState<ConversationRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [channelName, setChannelName] = useState("");

  const loadData = useCallback(async () => {
    if (!token || !currentOrganization) {
      setItems([]);
      return;
    }
    try {
      setLoading(true);
      const data = await listConversations(token);
      setItems(data);
    } catch (error) {
      console.error("[ConversationsScreen] Failed to load conversations:", error);
      Alert.alert("加载失败", "无法加载会话列表。");
    } finally {
      setLoading(false);
    }
  }, [currentOrganization, token]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  useEffect(() => {
    if (!token || !currentOrganization) {
      ChatRealtimeService.disconnect();
      return;
    }
    const handleEvent = () => {
      void loadData();
    };
    ChatRealtimeService.connect(token, currentOrganization.id);
    ChatRealtimeService.on("event", handleEvent);
    return () => {
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
      Alert.alert("创建失败", "无法创建频道会话。");
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.heading}>
        {currentOrganization?.name ?? "当前工作区"} 会话
      </Text>
      <TextField
        label="新建频道"
        value={channelName}
        onChangeText={setChannelName}
        placeholder="例如：APAC 销售协作"
      />
      <PrimaryButton
        title="创建频道"
        onPress={handleCreateChannel}
        disabled={!channelName.trim()}
        style={styles.createButton}
      />

      <FlatList
        data={items}
        keyExtractor={(item) => String(item.id)}
        refreshing={loading}
        onRefresh={() => void loadData()}
        renderItem={({ item }) => (
          <TouchableOpacity
            style={styles.card}
            onPress={() => navigation.navigate("ConversationDetail", { conversation: item })}
          >
            <View style={styles.row}>
              <Text style={styles.title}>{item.title || item.type}</Text>
              {item.unread_count > 0 ? <Text style={styles.badge}>{item.unread_count}</Text> : null}
            </View>
            <Text style={styles.meta}>{item.type.toUpperCase()}</Text>
            <Text style={styles.preview}>{item.last_message_preview || "暂无消息"}</Text>
          </TouchableOpacity>
        )}
        ListEmptyComponent={
          <View style={styles.empty}>
            <Text style={styles.emptyText}>当前工作区还没有聊天会话。</Text>
          </View>
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
  heading: {
    fontSize: 22,
    fontWeight: "700",
    color: "#0f172a",
    marginBottom: 12
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
  meta: {
    color: "#64748b",
    marginTop: 4
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
