import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, FlatList, ScrollView, StyleSheet, Text, View, useWindowDimensions } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import * as Clipboard from "expo-clipboard";

import {
  buildRecordingDownloadRequest,
  createConversationNote,
  createConversationRoom,
  createMessage,
  fetchConversationDetail,
  fetchRecording,
  listConversationNotes,
  listMessages,
  markConversationRead,
  updateConversation,
  type ConversationNoteRecord,
  type ConversationDetailRecord,
  type MessageRecord,
  type RecordingRecord
} from "../api/collaboration";
import { listContacts, type User } from "../api/users";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import fileDownloadAdapter from "../platform/fileDownload";
import ChatRealtimeService from "../services/ChatRealtimeService";
import { buildConversationShareLinks } from "../utils/invitations";

type Props = NativeStackScreenProps<RootStackParamList, "ConversationDetail">;

const STATUS_OPTIONS = ["open", "pending", "resolved"] as const;
const PRIORITY_OPTIONS = ["low", "normal", "high", "urgent"] as const;

const ConversationDetailScreen: React.FC<Props> = ({ route, navigation }) => {
  const { token, user } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const { width } = useWindowDimensions();
  const [detail, setDetail] = useState<ConversationDetailRecord | null>(null);
  const [contacts, setContacts] = useState<User[]>([]);
  const [notes, setNotes] = useState<ConversationNoteRecord[]>([]);
  const [messages, setMessages] = useState<MessageRecord[]>([]);
  const [latestRecording, setLatestRecording] = useState<RecordingRecord | null>(null);
  const [draft, setDraft] = useState("");
  const [noteDraft, setNoteDraft] = useState("");
  const [loading, setLoading] = useState(false);
  const conversationId = route.params.conversationId ?? route.params.conversation?.id ?? 0;

  const conversation = detail?.conversation ?? route.params.conversation ?? {
    id: conversationId,
    organization_id: currentOrganization?.id ?? 0,
    type: "direct",
    title: "协作线程",
    status: "open",
    priority: "normal",
    unread_count: 0,
  };
  const isWideScreen = width >= 1180;

  const loadData = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      setLoading(true);
      const [nextDetail, nextMessages, nextNotes, nextContacts] = await Promise.all([
        fetchConversationDetail(token, conversationId),
        listMessages(token, conversationId),
        listConversationNotes(token, conversationId),
        listContacts(token)
      ]);
      const nextRecording = nextDetail.workspace?.latest_recording
        ?? (nextDetail.conversation.latest_recording_id
          ? await fetchRecording(token, nextDetail.conversation.latest_recording_id)
          : null);
      setDetail(nextDetail);
      setContacts(nextContacts);
      setNotes(nextNotes);
      setMessages(nextMessages);
      setLatestRecording(nextRecording);
      await markConversationRead(token, conversationId);
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to load conversation detail:", error);
      Alert.alert("加载失败", "无法加载协作线程详情。");
    } finally {
      setLoading(false);
    }
  }, [conversationId, token]);

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
    const handleEvent = (event: { event: string; organization_id: number; payload: unknown }) => {
      if (event.event === "conversation.updated") {
        const payload = event.payload as { conversation_id?: number; changes?: Partial<ConversationDetailRecord["conversation"]> } | undefined;
        if (payload?.conversation_id === conversationId && payload.changes) {
          const changes = payload.changes;
          setDetail((previous) => previous ? {
            ...previous,
            conversation: { ...previous.conversation, ...changes },
            workspace: {
              ...previous.workspace,
              assignee_user_id: changes.assignee_user_id ?? previous.workspace.assignee_user_id,
              assignee_label: changes.assignee_display_name || changes.assignee_email || previous.workspace.assignee_label,
              status: changes.status || previous.workspace.status,
              priority: changes.priority || previous.workspace.priority,
            },
          } : previous);
        }
        return;
      }
      if (["message.created", "conversation.note.created", "room.recording.updated", "room.state.updated", "room.ended"].includes(event.event)) {
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
  }, [conversationId, currentOrganization, loadData, token]);

  const assigneeLabel = useMemo(() => {
    if (conversation.assignee_user_id === user?.id) {
      return "我";
    }
    return conversation.assignee_display_name || conversation.assignee_email || "未指派";
  }, [conversation.assignee_display_name, conversation.assignee_email, conversation.assignee_user_id, user?.id]);

  const boundContact = useMemo(
    () => contacts.find((item) => item.id === conversation.contact_id),
    [contacts, conversation.contact_id]
  );

  const handleCopyConversationLink = async () => {
    const links = buildConversationShareLinks(conversationId);
    await Clipboard.setStringAsync(links.webURL);
    Alert.alert("已复制", "线程 Web 链接已复制到剪贴板。");
  };

  const handleSend = async () => {
    if (!token || !draft.trim()) {
      return;
    }
    try {
      await createMessage(token, conversationId, { body: draft.trim() });
      setDraft("");
      await loadData();
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to send message:", error);
      Alert.alert("发送失败", "无法发送消息。");
    }
  };

  const handleAddNote = async () => {
    if (!token || !noteDraft.trim()) {
      return;
    }
    try {
      await createConversationNote(token, conversationId, noteDraft.trim());
      setNoteDraft("");
      await loadData();
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to create note:", error);
      Alert.alert("备注失败", "无法添加内部备注。");
    }
  };

  const handleAssignSelf = async () => {
    if (!token || !user) {
      return;
    }
    try {
      const updated = await updateConversation(token, conversationId, { assignee_user_id: user.id });
      setDetail((previous) => previous ? { ...previous, conversation: updated } : previous);
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to assign self:", error);
      Alert.alert("更新失败", "无法更新负责人。");
    }
  };

  const handleUnassign = async () => {
    if (!token) {
      return;
    }
    try {
      const updated = await updateConversation(token, conversationId, { assignee_user_id: 0 });
      setDetail((previous) => previous ? { ...previous, conversation: updated } : previous);
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to unassign conversation:", error);
      Alert.alert("更新失败", "无法清空负责人。");
    }
  };

  const handleUpdateStatus = async (status: typeof STATUS_OPTIONS[number]) => {
    if (!token) {
      return;
    }
    try {
      const updated = await updateConversation(token, conversationId, { status });
      setDetail((previous) => previous ? { ...previous, conversation: updated } : previous);
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to update status:", error);
      Alert.alert("更新失败", "无法更新会话状态。");
    }
  };

  const handleUpdatePriority = async (priority: typeof PRIORITY_OPTIONS[number]) => {
    if (!token) {
      return;
    }
    try {
      const updated = await updateConversation(token, conversationId, { priority });
      setDetail((previous) => previous ? { ...previous, conversation: updated } : previous);
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to update priority:", error);
      Alert.alert("更新失败", "无法更新优先级。");
    }
  };

  const handleCreateMeeting = async () => {
    if (!token) {
      return;
    }
    try {
      const room = await createConversationRoom(token, conversationId, `${conversation.title || "协作线程"} 会议`);
      await loadData();
      navigation.navigate("PreJoin", {
        roomId: room.room.id,
        title: room.room.title,
        conversationId: room.conversation_id ?? null,
        joinOptions: {
          audioEnabled: true,
          videoEnabled: true,
          cameraFacing: "front",
          speakerOn: true,
        },
      });
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to create room:", error);
      Alert.alert("创建失败", "无法从当前线程创建会议。");
    }
  };

  const handleBindContact = async (contactId: number | null) => {
    if (!token) {
      return;
    }
    try {
      const updated = await updateConversation(token, conversationId, { contact_id: contactId });
      setDetail((previous) => previous ? { ...previous, conversation: updated } : previous);
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to bind contact:", error);
      Alert.alert("更新失败", "无法绑定联系人。");
    }
  };

  const handleDownloadRecording = async (recordingId: number, fileId: number, fileName: string) => {
    if (!token) {
      return;
    }
    try {
      const request = buildRecordingDownloadRequest(token, recordingId, fileId);
      const result = await fileDownloadAdapter.download(request, fileName || `recording-${fileId}`);
      try {
        await fileDownloadAdapter.open(result);
      } catch {
        Alert.alert("下载完成", `文件已保存到 ${result.location}`);
      }
    } catch (error) {
      console.error("[ConversationDetailScreen] Failed to download recording:", error);
      Alert.alert("下载失败", "无法下载最近录音资产。");
    }
  };

  const workspacePane = (
    <>
      <Text style={styles.heading}>{conversation.title || "协作线程"}</Text>

      <View style={styles.summaryCard}>
        <Text style={styles.summaryText}>负责人 {detail?.workspace.assignee_label || assigneeLabel}</Text>
        <Text style={styles.summaryText}>状态 {detail?.workspace.status || conversation.status}</Text>
        <Text style={styles.summaryText}>优先级 {detail?.workspace.priority || conversation.priority}</Text>
        <Text style={styles.summaryText}>关联联系人 {boundContact?.display_name || boundContact?.email || "未绑定"}</Text>
        <PrimaryButton title="复制线程 Web 链接" onPress={() => void handleCopyConversationLink()} style={styles.inlineButtonSecondary} />
        {detail?.latest_room ? (
          <PrimaryButton
            title="进入当前会议"
            onPress={() => navigation.navigate("PreJoin", {
              roomId: detail.latest_room?.id ?? 0,
              title: detail.latest_room?.title ?? "Meeting",
              conversationId: detail.latest_room?.conversation_id ?? null,
              joinOptions: {
                audioEnabled: true,
                videoEnabled: true,
                cameraFacing: "front",
                speakerOn: true,
              },
            })}
            style={styles.inlineButton}
          />
        ) : (
          <PrimaryButton title="升级为会议" onPress={handleCreateMeeting} style={styles.inlineButton} />
        )}
      </View>

      <View style={styles.buttonRow}>
        <PrimaryButton title="指派给我" onPress={handleAssignSelf} style={styles.button} />
        <PrimaryButton title="清空负责人" onPress={handleUnassign} style={styles.buttonSecondary} />
      </View>

      <Text style={styles.sectionTitle}>状态</Text>
      <View style={styles.optionRow}>
        {STATUS_OPTIONS.map((status) => (
          <PrimaryButton
            key={status}
            title={status}
            onPress={() => handleUpdateStatus(status)}
            style={conversation.status === status ? styles.optionActive : styles.option}
          />
        ))}
      </View>

      <Text style={styles.sectionTitle}>优先级</Text>
      <View style={styles.optionRow}>
        {PRIORITY_OPTIONS.map((priority) => (
          <PrimaryButton
            key={priority}
            title={priority}
            onPress={() => handleUpdatePriority(priority)}
            style={conversation.priority === priority ? styles.optionActive : styles.option}
          />
        ))}
      </View>

      <Text style={styles.sectionTitle}>联系人绑定</Text>
      {boundContact ? (
        <View style={styles.infoCard}>
          <Text style={styles.infoTitle}>{boundContact.display_name || boundContact.email}</Text>
          <Text style={styles.infoMeta}>{boundContact.email}</Text>
          {boundContact.profile?.company ? <Text style={styles.infoMeta}>公司 {boundContact.profile.company}</Text> : null}
          {boundContact.profile?.role ? <Text style={styles.infoMeta}>角色 {boundContact.profile.role}</Text> : null}
          {boundContact.profile?.timezone ? <Text style={styles.infoMeta}>时区 {boundContact.profile.timezone}</Text> : null}
          {boundContact.profile?.default_source_lang || boundContact.profile?.default_target_lang ? (
            <Text style={styles.infoMeta}>
              默认语言 {boundContact.profile?.default_source_lang || "-"} → {boundContact.profile?.default_target_lang || "-"}
            </Text>
          ) : null}
          <View style={styles.buttonRow}>
            <PrimaryButton
              title="查看联系人"
              onPress={() => navigation.navigate("ContactDetail", { contact: boundContact })}
              style={styles.button}
            />
            <PrimaryButton title="解除绑定" onPress={() => handleBindContact(null)} style={styles.buttonSecondary} />
          </View>
        </View>
      ) : (
        <View style={styles.contactList}>
          {contacts.slice(0, 4).map((contact) => (
            <PrimaryButton
              key={contact.id}
              title={`绑定 ${contact.display_name || contact.email}`}
              onPress={() => handleBindContact(contact.id)}
              style={styles.contactButton}
            />
          ))}
        </View>
      )}

      {detail?.workspace ? (
        <View style={styles.infoCard}>
          <Text style={styles.infoTitle}>顶部工作区</Text>
          {detail.workspace.latest_meeting ? <Text style={styles.infoMeta}>最近会议 {detail.workspace.latest_meeting.title}</Text> : null}
          {detail.workspace.latest_recording ? <Text style={styles.infoMeta}>最近录音资产 #{detail.workspace.latest_recording.session.id}</Text> : null}
          {detail.workspace.meeting_summary?.summary ? <Text style={styles.infoBody}>{detail.workspace.meeting_summary.summary}</Text> : null}
          {detail.workspace.meeting_summary?.action_items?.length ? (
            <Text style={styles.infoMeta}>Action items {detail.workspace.meeting_summary.action_items.join(" / ")}</Text>
          ) : null}
          {detail.workspace.meeting_summary?.next_step ? <Text style={styles.infoMeta}>Next step {detail.workspace.meeting_summary.next_step}</Text> : null}
          {detail.workspace.latest_note ? (
            <Text style={styles.infoMeta}>
              最近备注 {detail.workspace.latest_note.author_display_name || detail.workspace.latest_note.author_email} · {new Date(detail.workspace.latest_note.created_at).toLocaleString()}
            </Text>
          ) : null}
        </View>
      ) : null}

      {notes.length > 0 ? (
        <View style={styles.infoCard}>
          <Text style={styles.infoTitle}>最近三条内部备注</Text>
          {notes.slice(0, 3).map((note) => (
            <View key={note.id} style={styles.noteRow}>
              <Text style={styles.noteBody}>{note.body}</Text>
              <Text style={styles.infoMeta}>
                {note.author_display_name || note.author_email} · {new Date(note.created_at).toLocaleString()}
              </Text>
            </View>
          ))}
        </View>
      ) : null}

      {detail?.latest_followup ? (
        <View style={styles.infoCard}>
          <Text style={styles.infoTitle}>最近会议/通话摘要</Text>
          <Text style={styles.infoBody}>
            {detail.workspace.meeting_summary?.summary || detail.latest_followup.summary_cn || detail.latest_followup.summary_en || "暂无摘要"}
          </Text>
          {(detail.workspace.meeting_summary?.next_step || detail.latest_followup.next_step) ? (
            <Text style={styles.infoMeta}>下一步 {detail.workspace.meeting_summary?.next_step || detail.latest_followup.next_step}</Text>
          ) : null}
        </View>
      ) : null}

      {latestRecording ? (
        <View style={styles.infoCard}>
          <Text style={styles.infoTitle}>最近录音资产</Text>
          <Text style={styles.infoMeta}>录音会话 #{latestRecording.session.id}</Text>
          <Text style={styles.infoMeta}>状态 {latestRecording.session.status}</Text>
          <Text style={styles.infoMeta}>文件数 {latestRecording.files.length}</Text>
          {latestRecording.files.slice(0, 2).map((file) => (
            <View key={file.id} style={styles.recordingFileRow}>
              <Text style={styles.recordingFileTitle}>{file.file_name}</Text>
              <Text style={styles.infoMeta}>
                {file.recording_kind} · {file.file_size_bytes} bytes · {file.duration_seconds}s
              </Text>
              <PrimaryButton
                title="下载最近录音"
                onPress={() => void handleDownloadRecording(latestRecording.session.id, file.id, file.file_name)}
                style={styles.recordingButton}
              />
            </View>
          ))}
          <PrimaryButton
            title="查看全部录音资产"
            onPress={() => navigation.navigate("Recordings")}
            style={styles.recordingLinkButton}
          />
        </View>
      ) : null}

      <TextField
        label="内部备注"
        value={noteDraft}
        onChangeText={setNoteDraft}
        placeholder="记录交接说明、风险点或下一步动作"
      />
      <PrimaryButton title="添加内部备注" onPress={handleAddNote} disabled={!noteDraft.trim()} style={styles.createNoteButton} />
    </>
  );

  const messagePane = (
    <FlatList
      data={messages}
      keyExtractor={(item) => String(item.id)}
      refreshing={loading}
      onRefresh={() => void loadData()}
      contentContainerStyle={styles.listContent}
      renderItem={({ item }) => {
        const isMine = item.sender_id === user?.id;
        const isSystem = item.type === "system";
        return (
          <View style={[
            styles.messageBubble,
            isSystem ? styles.systemBubble : isMine ? styles.mine : styles.theirs
          ]}>
            <Text style={styles.sender}>{item.sender_display_name || item.sender_email}</Text>
            <Text style={styles.body}>{item.body || item.type}</Text>
            {item.metadata?.event_type ? (
              <Text style={styles.systemMeta}>{String(item.metadata.event_type)}</Text>
            ) : null}
            <Text style={styles.time}>{new Date(item.created_at).toLocaleString()}</Text>
          </View>
        );
      }}
      ListFooterComponent={
        <View style={styles.composer}>
          <TextField
            value={draft}
            onChangeText={setDraft}
            placeholder="输入线程消息"
          />
          <PrimaryButton title="发送消息" onPress={handleSend} disabled={!draft.trim()} />
        </View>
      }
    />
  );

  return (
    <View style={styles.container}>
      {isWideScreen ? (
        <View style={styles.desktopLayout}>
          <ScrollView style={styles.workspaceColumn} contentContainerStyle={styles.workspaceColumnContent}>
            {workspacePane}
          </ScrollView>
          <View style={styles.messageColumn}>
            {messagePane}
          </View>
        </View>
      ) : (
        <>
          {workspacePane}

          {messagePane}
        </>
      )}
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
  workspaceColumn: {
    flex: 0.95,
  },
  workspaceColumnContent: {
    paddingBottom: 24,
  },
  messageColumn: {
    flex: 1.1,
  },
  heading: {
    fontSize: 22,
    fontWeight: "700",
    color: "#0f172a",
    marginBottom: 12
  },
  summaryCard: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    borderWidth: 1,
    borderColor: "#e2e8f0"
  },
  summaryText: {
    color: "#334155",
    marginTop: 4
  },
  inlineButton: {
    marginTop: 12
  },
  inlineButtonSecondary: {
    marginTop: 12,
    backgroundColor: "#334155",
  },
  buttonRow: {
    flexDirection: "row",
    gap: 12,
    marginTop: 12
  },
  button: {
    flex: 1
  },
  buttonSecondary: {
    flex: 1,
    backgroundColor: "#475569"
  },
  sectionTitle: {
    marginTop: 16,
    marginBottom: 8,
    color: "#0f172a",
    fontWeight: "700"
  },
  optionRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8
  },
  option: {
    backgroundColor: "#64748b"
  },
  optionActive: {
    backgroundColor: "#0f172a"
  },
  infoCard: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginTop: 14,
    borderWidth: 1,
    borderColor: "#e2e8f0"
  },
  infoTitle: {
    fontWeight: "700",
    color: "#0f172a"
  },
  infoBody: {
    color: "#334155",
    marginTop: 8
  },
  infoMeta: {
    color: "#64748b",
    marginTop: 8
  },
  createNoteButton: {
    marginBottom: 12
  },
  recordingButton: {
    marginTop: 12,
    backgroundColor: "#0f172a"
  },
  recordingLinkButton: {
    marginTop: 12,
    backgroundColor: "#334155"
  },
  recordingFileRow: {
    marginTop: 12,
    paddingTop: 12,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0"
  },
  recordingFileTitle: {
    color: "#0f172a",
    fontWeight: "600"
  },
  contactList: {
    gap: 8
  },
  contactButton: {
    backgroundColor: "#1d4ed8"
  },
  listContent: {
    paddingBottom: 24
  },
  messageBubble: {
    borderRadius: 16,
    padding: 14,
    marginBottom: 12,
    maxWidth: "92%"
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
  systemBubble: {
    backgroundColor: "#ede9fe",
    alignSelf: "stretch"
  },
  sender: {
    fontWeight: "600",
    color: "#1e293b",
    marginBottom: 6
  },
  body: {
    color: "#0f172a"
  },
  systemMeta: {
    color: "#6d28d9",
    fontSize: 12,
    marginTop: 8
  },
  time: {
    color: "#64748b",
    fontSize: 12,
    marginTop: 8
  },
  noteRow: {
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
    paddingTop: 10,
    marginTop: 10
  },
  noteBody: {
    color: "#334155"
  },
  composer: {
    marginTop: 12
  }
});

export default ConversationDetailScreen;
