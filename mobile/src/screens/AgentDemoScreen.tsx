import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
  useWindowDimensions,
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { createAgentRun, type AgentRunResult } from "../api/agent";
import {
  createConversation,
  createConversationNote,
  createMessage,
  fetchConversationDetail,
  listConversationNotes,
  listConversations,
  listMessages,
  type ConversationDetailRecord,
  type ConversationNoteRecord,
  type ConversationRecord,
  type MessageRecord,
} from "../api/collaboration";
import AgentMessageBubble from "../components/AgentMessageBubble";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "AgentDemo">;

const defaultGoal = "请基于会话消息和内部备注，给出客户当前诉求、风险点、下一步建议，并列出依据。";

const AgentDemoScreen: React.FC<Props> = ({ navigation }) => {
  const { token } = useAuthContext();
  const { currentOrganization, loading: organizationLoading } = useOrganization();
  const { width } = useWindowDimensions();
  const isWide = width >= 1100;
  const [conversations, setConversations] = useState<ConversationRecord[]>([]);
  const [selectedConversationId, setSelectedConversationId] = useState<number | null>(null);
  const [detail, setDetail] = useState<ConversationDetailRecord | null>(null);
  const [messages, setMessages] = useState<MessageRecord[]>([]);
  const [notes, setNotes] = useState<ConversationNoteRecord[]>([]);
  const [goal, setGoal] = useState(defaultGoal);
  const [activeRun, setActiveRun] = useState<AgentRunResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [creatingDemo, setCreatingDemo] = useState(false);
  const [asking, setAsking] = useState(false);

  const selectedConversation = useMemo(
    () => conversations.find((item) => item.id === selectedConversationId) ?? null,
    [conversations, selectedConversationId]
  );

  const loadConversations = useCallback(async () => {
    if (!token || !currentOrganization) {
      setConversations([]);
      setSelectedConversationId(null);
      return;
    }
    try {
      setLoading(true);
      const items = await listConversations(token, "open");
      setConversations(items);
      setSelectedConversationId((current) => {
        if (current && items.some((item) => item.id === current)) {
          return current;
        }
        return items[0]?.id ?? null;
      });
    } catch (error) {
      console.error("[AgentDemoScreen] Failed to load conversations:", error);
      Alert.alert("加载失败", "无法加载可用于 Agent Demo 的会话。");
    } finally {
      setLoading(false);
    }
  }, [currentOrganization, token]);

  const loadSelectedContext = useCallback(async () => {
    if (!token || !selectedConversationId) {
      setDetail(null);
      setMessages([]);
      setNotes([]);
      return;
    }
    try {
      setLoading(true);
      const [nextDetail, nextMessages, nextNotes] = await Promise.all([
        fetchConversationDetail(token, selectedConversationId),
        listMessages(token, selectedConversationId),
        listConversationNotes(token, selectedConversationId),
      ]);
      setDetail(nextDetail);
      setMessages(nextMessages);
      setNotes(nextNotes);
    } catch (error) {
      console.error("[AgentDemoScreen] Failed to load conversation context:", error);
      Alert.alert("加载失败", "无法加载当前会话上下文。");
    } finally {
      setLoading(false);
    }
  }, [selectedConversationId, token]);

  useEffect(() => {
    void loadConversations();
  }, [loadConversations]);

  useEffect(() => {
    setActiveRun(null);
    void loadSelectedContext();
  }, [loadSelectedContext]);

  const handleCreateDemoThread = useCallback(async () => {
    if (!token) return;
    try {
      setCreatingDemo(true);
      const conversation = await createConversation(token, {
        type: "channel",
        title: `Agent RAG Demo ${new Date().toLocaleTimeString()}`,
        topic: "Web Agent demo thread",
      });
      await createMessage(token, conversation.id, {
        body: "客户希望本周确认跨境客服试点方案，重点关注响应时延、翻译质量和后续培训安排。",
      });
      await createMessage(token, conversation.id, {
        body: "销售侧已承诺先交付 20 个坐席的试点报价，但客户还在等待安全与数据留存说明。",
      });
      await createConversationNote(token, conversation.id, "内部备注：客户预算窗口在月底关闭，风险是法务审批可能拖慢签约。");
      await createConversationNote(token, conversation.id, "内部备注：下一步建议准备一页式安全说明，并约一次技术答疑。");
      await loadConversations();
      setSelectedConversationId(conversation.id);
    } catch (error) {
      console.error("[AgentDemoScreen] Failed to create demo thread:", error);
      Alert.alert("创建失败", "无法创建 Agent Demo 会话。");
    } finally {
      setCreatingDemo(false);
    }
  }, [loadConversations, token]);

  const handleAskAgent = useCallback(async () => {
    if (!token || !selectedConversationId) return;
    try {
      setAsking(true);
      const result = await createAgentRun(token, {
        conversation_id: selectedConversationId,
        goal: goal.trim() || defaultGoal,
      });
      setActiveRun(result);
    } catch (error) {
      console.error("[AgentDemoScreen] Failed to create agent run:", error);
      Alert.alert("Agent 调用失败", "无法创建 Agent run。");
    } finally {
      setAsking(false);
    }
  }, [goal, selectedConversationId, token]);

  if (organizationLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#2563eb" />
      </View>
    );
  }

  if (!currentOrganization) {
    return (
      <View style={styles.centered}>
        <Text style={styles.emptyTitle}>还没有可用工作区</Text>
        <PrimaryButton title="去创建工作区" onPress={() => navigation.navigate("Organizations")} style={styles.centerButton} />
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <View style={isWide ? styles.desktopLayout : styles.mobileLayout}>
        <View style={isWide ? styles.sidebar : styles.panel}>
          <Text style={styles.eyebrow}>{currentOrganization.name}</Text>
          <Text style={styles.heading}>Agent RAG Demo</Text>
          <Text style={styles.subheading}>Web 端会话助手</Text>

          <View style={styles.actionRow}>
            <PrimaryButton
              title={creatingDemo ? "创建中..." : "创建演示线程"}
              onPress={handleCreateDemoThread}
              disabled={creatingDemo}
              style={styles.actionButton}
            />
            <PrimaryButton
              title="刷新"
              onPress={() => void loadConversations()}
              style={styles.refreshButton}
            />
          </View>

          {loading && conversations.length === 0 ? (
            <ActivityIndicator color="#2563eb" />
          ) : null}

          <ScrollView style={styles.threadList} contentContainerStyle={styles.threadListContent}>
            {conversations.map((conversation) => {
              const selected = conversation.id === selectedConversationId;
              return (
                <Pressable
                  key={conversation.id}
                  style={[styles.threadItem, selected && styles.threadItemActive]}
                  onPress={() => setSelectedConversationId(conversation.id)}
                >
                  <Text style={[styles.threadTitle, selected && styles.threadTitleActive]}>
                    {conversation.title || conversation.type}
                  </Text>
                  <Text style={[styles.threadMeta, selected && styles.threadMetaActive]}>
                    {conversation.priority.toUpperCase()} · {conversation.status.toUpperCase()}
                  </Text>
                  <Text style={[styles.threadPreview, selected && styles.threadPreviewActive]} numberOfLines={2}>
                    {conversation.last_message_preview || "暂无消息"}
                  </Text>
                </Pressable>
              );
            })}
            {conversations.length === 0 && !loading ? (
              <View style={styles.emptyBox}>
                <Text style={styles.emptyText}>暂无 open 会话，可以创建一条演示线程。</Text>
              </View>
            ) : null}
          </ScrollView>
        </View>

        <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
          <View style={styles.panel}>
            <Text style={styles.sectionTitle}>{selectedConversation?.title || "选择一个会话"}</Text>
            <Text style={styles.contextLine}>消息 {messages.length} · 内部备注 {notes.length}</Text>
            {detail?.latest_followup?.summary_cn || detail?.latest_followup?.summary_en ? (
              <Text style={styles.followupText}>
                {detail.latest_followup.summary_cn || detail.latest_followup.summary_en}
              </Text>
            ) : null}

            <TextField
              label="Agent 问题"
              value={goal}
              onChangeText={setGoal}
              multiline
              style={styles.goalInput}
            />
            <PrimaryButton
              title={asking ? "提交中..." : "Ask AI"}
              onPress={handleAskAgent}
              disabled={!selectedConversationId || asking}
              style={styles.askButton}
            />
          </View>

          {activeRun ? (
            <AgentMessageBubble
              runId={activeRun.run.id}
              initialResult={activeRun}
              onComplete={() => {
                void loadSelectedContext();
              }}
            />
          ) : null}

          <View style={styles.grid}>
            <View style={[styles.panel, styles.contextPanel]}>
              <Text style={styles.sectionTitle}>会话消息</Text>
              {messages.slice(-6).map((message) => (
                <View key={message.id} style={styles.contextItem}>
                  <Text style={styles.contextItemTitle}>
                    {message.sender_display_name || message.sender_email || message.type}
                  </Text>
                  <Text style={styles.contextItemBody}>{message.body || message.type}</Text>
                </View>
              ))}
              {messages.length === 0 ? <Text style={styles.emptyText}>暂无消息。</Text> : null}
            </View>

            <View style={[styles.panel, styles.contextPanel]}>
              <Text style={styles.sectionTitle}>内部备注</Text>
              {notes.slice(0, 6).map((note) => (
                <View key={note.id} style={styles.contextItem}>
                  <Text style={styles.contextItemTitle}>
                    {note.author_display_name || note.author_email}
                  </Text>
                  <Text style={styles.contextItemBody}>{note.body}</Text>
                </View>
              ))}
              {notes.length === 0 ? <Text style={styles.emptyText}>暂无内部备注。</Text> : null}
            </View>
          </View>
        </ScrollView>
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#eef2f7",
    padding: 16,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "#eef2f7",
    padding: 24,
  },
  centerButton: {
    marginTop: 16,
    minWidth: 180,
  },
  desktopLayout: {
    flex: 1,
    flexDirection: "row",
    gap: 16,
  },
  mobileLayout: {
    flex: 1,
    gap: 12,
  },
  sidebar: {
    width: 340,
    backgroundColor: "#ffffff",
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "#dbe3ef",
    padding: 16,
  },
  main: {
    flex: 1,
  },
  mainContent: {
    gap: 12,
    paddingBottom: 24,
  },
  panel: {
    backgroundColor: "#ffffff",
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "#dbe3ef",
    padding: 16,
  },
  eyebrow: {
    color: "#0f766e",
    fontSize: 12,
    fontWeight: "700",
    textTransform: "uppercase",
  },
  heading: {
    color: "#111827",
    fontSize: 24,
    fontWeight: "800",
    marginTop: 4,
  },
  subheading: {
    color: "#475569",
    marginTop: 4,
    marginBottom: 14,
  },
  actionRow: {
    flexDirection: "row",
    gap: 10,
    marginBottom: 12,
  },
  actionButton: {
    flex: 1.2,
    borderRadius: 8,
  },
  refreshButton: {
    flex: 0.8,
    borderRadius: 8,
    backgroundColor: "#334155",
  },
  threadList: {
    flex: 1,
  },
  threadListContent: {
    gap: 8,
    paddingBottom: 12,
  },
  threadItem: {
    borderWidth: 1,
    borderColor: "#dbe3ef",
    borderRadius: 8,
    padding: 12,
    backgroundColor: "#f8fafc",
  },
  threadItemActive: {
    backgroundColor: "#1f3a5f",
    borderColor: "#1f3a5f",
  },
  threadTitle: {
    color: "#111827",
    fontWeight: "700",
  },
  threadTitleActive: {
    color: "#ffffff",
  },
  threadMeta: {
    color: "#64748b",
    fontSize: 12,
    marginTop: 4,
    fontWeight: "700",
  },
  threadMetaActive: {
    color: "#cbd5e1",
  },
  threadPreview: {
    color: "#475569",
    marginTop: 8,
    lineHeight: 18,
  },
  threadPreviewActive: {
    color: "#e2e8f0",
  },
  emptyBox: {
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "#dbe3ef",
    padding: 14,
    backgroundColor: "#f8fafc",
  },
  emptyTitle: {
    color: "#111827",
    fontSize: 18,
    fontWeight: "700",
  },
  emptyText: {
    color: "#64748b",
  },
  sectionTitle: {
    color: "#111827",
    fontSize: 18,
    fontWeight: "800",
  },
  contextLine: {
    color: "#64748b",
    marginTop: 6,
    marginBottom: 12,
  },
  followupText: {
    color: "#334155",
    lineHeight: 20,
    padding: 12,
    borderRadius: 8,
    backgroundColor: "#ecfdf5",
    marginBottom: 12,
  },
  goalInput: {
    minHeight: 96,
    textAlignVertical: "top",
    paddingTop: 12,
  },
  askButton: {
    borderRadius: 8,
  },
  grid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 12,
  },
  contextPanel: {
    flex: 1,
    minWidth: 280,
  },
  contextItem: {
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
    paddingTop: 10,
    marginTop: 10,
  },
  contextItemTitle: {
    color: "#334155",
    fontSize: 12,
    fontWeight: "700",
  },
  contextItemBody: {
    color: "#111827",
    lineHeight: 20,
    marginTop: 5,
  },
});

export default AgentDemoScreen;
