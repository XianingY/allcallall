import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, StyleSheet, Text, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { createMessage, listMessages, markConversationRead, type MessageRecord } from "../api/collaboration";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import ChatRealtimeService from "../services/ChatRealtimeService";

type Props = NativeStackScreenProps<RootStackParamList, "ConversationDetail">;

const ConversationDetailScreen: React.FC<Props> = ({ route }) => {
  const { conversation } = route.params;
  const { token, user } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const [messages, setMessages] = useState<MessageRecord[]>([]);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(false);

  const loadMessages = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      setLoading(true);
      const data = await listMessages(token, conversation.id);
      setMessages(data);
      await markConversationRead(token, conversation.id);
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to load messages:", error);
      Alert.alert("加载失败", "无法加载消息。");
    } finally {
      setLoading(false);
    }
  }, [conversation.id, token]);

  useEffect(() => {
    void loadMessages();
  }, [loadMessages]);

  useEffect(() => {
    if (!token || !currentOrganization) {
      ChatRealtimeService.disconnect();
      return;
    }
    const handleEvent = (event: { event: string; organization_id: number; payload: unknown }) => {
      if (event.event === "message.created") {
        void loadMessages();
      }
    };
    ChatRealtimeService.connect(token, currentOrganization.id);
    ChatRealtimeService.on("event", handleEvent);
    return () => {
      ChatRealtimeService.off("event", handleEvent);
    };
  }, [currentOrganization, loadMessages, token]);

  const handleSend = async () => {
    if (!token || !draft.trim()) {
      return;
    }
    try {
      await createMessage(token, conversation.id, { body: draft.trim() });
      setDraft("");
      await loadMessages();
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to send message:", error);
      Alert.alert("发送失败", "无法发送消息。");
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.heading}>{conversation.title || "会话详情"}</Text>
      <FlatList
        data={messages}
        keyExtractor={(item) => String(item.id)}
        refreshing={loading}
        onRefresh={() => void loadMessages()}
        contentContainerStyle={styles.listContent}
        renderItem={({ item }) => {
          const isMine = item.sender_id === user?.id;
          return (
            <View style={[styles.messageBubble, isMine ? styles.mine : styles.theirs]}>
              <Text style={styles.sender}>{item.sender_display_name || item.sender_email}</Text>
              <Text style={styles.body}>{item.body || item.type}</Text>
              <Text style={styles.time}>{new Date(item.created_at).toLocaleString()}</Text>
            </View>
          );
        }}
      />
      <TextField
        value={draft}
        onChangeText={setDraft}
        placeholder="输入消息"
      />
      <PrimaryButton title="发送消息" onPress={handleSend} disabled={!draft.trim()} />
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
  listContent: {
    paddingBottom: 12
  },
  messageBubble: {
    borderRadius: 16,
    padding: 14,
    marginBottom: 12,
    maxWidth: "88%"
  },
  mine: {
    backgroundColor: "#dbeafe",
    alignSelf: "flex-end"
  },
  theirs: {
    backgroundColor: "#fff",
    borderWidth: 1,
    borderColor: "#e2e8f0",
    alignSelf: "flex-start"
  },
  sender: {
    fontWeight: "600",
    color: "#1e293b",
    marginBottom: 6
  },
  body: {
    color: "#0f172a"
  },
  time: {
    color: "#64748b",
    fontSize: 12,
    marginTop: 8
  }
});

export default ConversationDetailScreen;
