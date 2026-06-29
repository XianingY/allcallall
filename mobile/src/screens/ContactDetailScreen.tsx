import React, { useCallback, useEffect, useMemo, useState } from "react";
import * as Clipboard from "expo-clipboard";
import {
  Alert,
  ScrollView,
  StyleSheet,
  Text,
  TouchableOpacity,
  View
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import TextField from "../components/TextField";
import PrimaryButton from "../components/PrimaryButton";
import { RootStackParamList } from "../navigation/AppNavigator";
import { createConversation, fetchRoomState, listConversations, type ConversationRecord } from "../api/collaboration";
import {
  fetchCallFollowup,
  fetchCallHistory,
  generateCallFollowup,
  type CallFollowupRecord,
  type FollowUpTaskRecord
} from "../api/commercial";
import {
  ContactProfile,
  fetchContactProfile,
  removeContact,
  saveContactProfile
} from "../api/users";
import { useAuthContext } from "../context/AuthContext";
import { useSettings } from "../context/SettingsContext";
import { useSignaling } from "../context/signalingContextValue";
import AnalyticsService from "../services/AnalyticsService";

type Props = NativeStackScreenProps<RootStackParamList, "ContactDetail">;

const ContactDetailScreen: React.FC<Props> = ({ route, navigation }) => {
  const { contact } = route.params;
  const { token, user } = useAuthContext();
  const { settings } = useSettings();
  const {
    connectionReady,
    startCall,
    setTranslationLanguage,
    setTranslationSourceLanguage
  } = useSignaling();
  const [profile, setProfile] = useState<ContactProfile>({
    company: contact.profile?.company ?? "",
    role: contact.profile?.role ?? "",
    timezone: contact.profile?.timezone ?? "",
    default_source_lang: contact.profile?.default_source_lang ?? "",
    default_target_lang: contact.profile?.default_target_lang ?? "",
    relationship_status: contact.profile?.relationship_status ?? "",
    preferred_contact_start: contact.profile?.preferred_contact_start ?? "",
    preferred_contact_end: contact.profile?.preferred_contact_end ?? "",
    preferred_contact_days: contact.profile?.preferred_contact_days ?? "",
    last_followup_state: contact.profile?.last_followup_state ?? "",
    note: contact.profile?.note ?? ""
  });
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(false);
  const [lastCall, setLastCall] = useState<string | null>(null);
  const [lastResult, setLastResult] = useState<string | null>(null);
  const [lastCallId, setLastCallId] = useState<string | null>(null);
  const [followup, setFollowup] = useState<CallFollowupRecord | null>(null);
  const [tasks, setTasks] = useState<FollowUpTaskRecord[]>([]);
  const [linkedConversations, setLinkedConversations] = useState<ConversationRecord[]>([]);

  const loadProfile = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      setLoading(true);
      const [remoteProfile, calls, conversations] = await Promise.all([
        fetchContactProfile(token, contact.id),
        fetchCallHistory(token, 365),
        listConversations(token, "all", contact.id)
      ]);
      setProfile({
        company: remoteProfile.company ?? "",
        role: remoteProfile.role ?? "",
        timezone: remoteProfile.timezone ?? "",
        default_source_lang: remoteProfile.default_source_lang ?? "",
        default_target_lang: remoteProfile.default_target_lang ?? "",
        relationship_status: remoteProfile.relationship_status ?? "",
        preferred_contact_start: remoteProfile.preferred_contact_start ?? "",
        preferred_contact_end: remoteProfile.preferred_contact_end ?? "",
        preferred_contact_days: remoteProfile.preferred_contact_days ?? "",
        last_followup_state: remoteProfile.last_followup_state ?? "",
        note: remoteProfile.note ?? ""
      });
      setLinkedConversations(conversations);
      const contactCalls = calls.filter((item) => item.caller_email === contact.email || item.callee_email === contact.email);
      if (contactCalls.length > 0) {
        setLastCall(contactCalls[0].started_at);
        setLastResult(contactCalls[0].status);
        setLastCallId(contactCalls[0].call_id);
        try {
          const followupResponse = await fetchCallFollowup(token, contactCalls[0].call_id);
          setFollowup(followupResponse.followup);
          setTasks(followupResponse.tasks);
          AnalyticsService.track("followup_viewed", { call_id: contactCalls[0].call_id });
        } catch {
          if (settings.businessAssistantEnabled) {
            try {
              const generated = await generateCallFollowup(token, contactCalls[0].call_id);
              setFollowup(generated.followup);
              setTasks(generated.tasks);
              AnalyticsService.track("followup_generated", { call_id: contactCalls[0].call_id });
            } catch {
              setFollowup(null);
              setTasks([]);
            }
          } else {
            setFollowup(null);
            setTasks([]);
          }
        }
      }
    } catch (error) {
      console.warn("[ContactDetailScreen] Failed to load contact details:", error);
    } finally {
      setLoading(false);
    }
  }, [contact.email, contact.id, settings.businessAssistantEnabled, token]);

  useEffect(() => {
    void loadProfile();
  }, [loadProfile]);

  const handleCopyDraft = useCallback(async () => {
    const draft = `${followup?.followup_draft_cn ?? ""}\n\n${followup?.followup_draft_en ?? ""}`.trim();
    if (!draft) {
      return;
    }
    await Clipboard.setStringAsync(draft);
    AnalyticsService.track("draft_copied", { call_id: lastCallId ?? undefined });
    Alert.alert("已复制", "双语跟进草稿已复制到剪贴板。");
  }, [followup?.followup_draft_cn, followup?.followup_draft_en, lastCallId]);

  const handleSave = async () => {
    if (!token) {
      return;
    }
    try {
      setSaving(true);
      const saved = await saveContactProfile(token, contact.id, profile);
      setProfile({
        company: saved.company ?? "",
        role: saved.role ?? "",
        timezone: saved.timezone ?? "",
        default_source_lang: saved.default_source_lang ?? "",
        default_target_lang: saved.default_target_lang ?? "",
        relationship_status: saved.relationship_status ?? "",
        preferred_contact_start: saved.preferred_contact_start ?? "",
        preferred_contact_end: saved.preferred_contact_end ?? "",
        preferred_contact_days: saved.preferred_contact_days ?? "",
        last_followup_state: saved.last_followup_state ?? "",
        note: saved.note ?? ""
      });
      Alert.alert("已保存", "联系人业务资料已更新。");
    } catch (error) {
      console.error("[ContactDetailScreen] Failed to save contact profile:", error);
      Alert.alert("保存失败", "当前无法保存联系人资料。");
    } finally {
      setSaving(false);
    }
  };

  const handleCall = async () => {
    if (!connectionReady) {
      Alert.alert("正在重新连接", "信令服务暂时不可用，请稍后再试。");
      return;
    }
    if (profile.default_source_lang) {
      setTranslationSourceLanguage(profile.default_source_lang);
    }
    if (profile.default_target_lang) {
      setTranslationLanguage(profile.default_target_lang);
    }
    if (followup?.summary_cn) {
      Alert.alert("上次跟进摘要", followup.summary_cn, [
        { text: "继续呼叫", onPress: () => void startCall(contact.email) }
      ]);
      return;
    }
    await startCall(contact.email);
  };

  const handleOpenChat = useCallback(async () => {
    if (!token) {
      return;
    }
    try {
      const conversation = await createConversation(token, {
        type: "direct",
        member_ids: [contact.id]
      });
      navigation.navigate("ConversationDetail", { conversation });
    } catch (error) {
      console.error("[ContactDetailScreen] Failed to open direct conversation:", error);
      Alert.alert("打开聊天失败", "无法创建或打开与该联系人的私聊会话。");
    }
  }, [contact.id, navigation, token]);

  const handleRemove = async () => {
    if (!token) {
      return;
    }
    try {
      await removeContact(token, contact.id);
      Alert.alert("已删除", "联系人已移除。", [
        {
          text: "返回",
          onPress: () => navigation.goBack()
        }
      ]);
    } catch (error) {
      console.error("[ContactDetailScreen] Failed to remove contact:", error);
      Alert.alert("删除失败", "当前无法删除联系人。");
    }
  };

  const handleOpenLinkedConversation = useCallback((conversation: ConversationRecord) => {
    navigation.navigate("ConversationDetail", { conversation });
  }, [navigation]);

  const handleOpenRecentMeeting = useCallback(async (conversation: ConversationRecord) => {
    if (!token) {
      return;
    }
    const roomId = conversation.active_room_id || conversation.latest_room_id;
    if (!roomId) {
      return;
    }
    try {
      const room = await fetchRoomState(token, roomId);
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
      console.error("[ContactDetailScreen] Failed to open linked room:", error);
      Alert.alert("打开会议失败", "无法打开关联会议。");
    }
  }, [navigation, token]);

  const lastCallLabel = useMemo(() => {
    if (!lastCall) {
      return "暂无通话记录";
    }
    return `${new Date(lastCall).toLocaleString()} · ${lastResult ?? "unknown"}`;
  }, [lastCall, lastResult]);

  return (
    <ScrollView style={styles.container} contentContainerStyle={styles.content}>
      <View style={styles.heroCard}>
        <Text style={styles.name}>{contact.display_name || contact.email}</Text>
        <Text style={styles.email}>{contact.email}</Text>
        <Text style={styles.meta}>最近通话: {lastCallLabel}</Text>
        <Text style={styles.meta}>我的账号: {user?.email}</Text>
        <PrimaryButton title="一键再次呼叫" onPress={() => void handleCall()} />
        <PrimaryButton title="打开私聊" onPress={() => void handleOpenChat()} style={styles.chatButton} />
      </View>

      {settings.businessAssistantEnabled ? (
        <View style={styles.formCard}>
          <Text style={styles.sectionTitle}>AI 跟进卡 / Follow-up</Text>
          {followup ? (
            <>
              <Text style={styles.followupSummary}>{followup.summary_cn || followup.summary_en || "暂无摘要"}</Text>
              {followup.next_step ? <Text style={styles.followupMeta}>下一步: {followup.next_step}</Text> : null}
              {followup.followup_draft_cn ? (
                <Text style={styles.followupDraft}>跟进草稿: {followup.followup_draft_cn}</Text>
              ) : null}
              {tasks.length > 0 ? (
                <View style={styles.taskList}>
                  {tasks.map((task) => (
                    <Text key={task.id} style={styles.taskItem}>
                      • {task.title} {task.due_at ? `· ${new Date(task.due_at).toLocaleString()}` : ""}
                    </Text>
                  ))}
                </View>
              ) : null}
              {followup.followup_draft_cn || followup.followup_draft_en ? (
                <PrimaryButton
                  title="复制双语跟进草稿"
                  onPress={() => void handleCopyDraft()}
                />
              ) : null}
            </>
          ) : (
            <Text style={styles.impactText}>
              最近一通通话还没有生成跟进卡。完成一次接通通话后，这里会显示摘要、下一步和草稿。
            </Text>
          )}
          {lastCallId ? <Text style={styles.followupHint}>通话 ID: {lastCallId}</Text> : null}
        </View>
      ) : null}

      <View style={styles.formCard}>
        <Text style={styles.sectionTitle}>业务联系人资料</Text>
        <TextField label="公司 / Company" value={profile.company ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, company: value }))} />
        <TextField label="角色 / Role" value={profile.role ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, role: value }))} />
        <TextField label="时区 / Timezone" value={profile.timezone ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, timezone: value }))} />
        <TextField label="默认源语言 / Source Language" value={profile.default_source_lang ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, default_source_lang: value }))} />
        <TextField label="默认目标语言 / Target Language" value={profile.default_target_lang ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, default_target_lang: value }))} />
        <TextField label="关系阶段 / Relationship" value={profile.relationship_status ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, relationship_status: value }))} />
        <TextField label="可联系时间开始 / Preferred Start" value={profile.preferred_contact_start ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, preferred_contact_start: value }))} />
        <TextField label="可联系时间结束 / Preferred End" value={profile.preferred_contact_end ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, preferred_contact_end: value }))} />
        <TextField label="可联系日期 / Preferred Days" value={profile.preferred_contact_days ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, preferred_contact_days: value }))} />
        <TextField label="最近跟进状态 / Last Follow-up State" value={profile.last_followup_state ?? ""} onChangeText={(value) => setProfile((current) => ({ ...current, last_followup_state: value }))} />
        <TextField
          label="备注 / Note"
          value={profile.note ?? ""}
          onChangeText={(value) => setProfile((current) => ({ ...current, note: value }))}
          multiline
          numberOfLines={4}
        />
        <PrimaryButton title={saving ? "保存中..." : "保存联系人资料"} onPress={() => void handleSave()} disabled={saving || loading} />
      </View>

      <View style={styles.formCard}>
        <Text style={styles.sectionTitle}>关联 Inbox 线程</Text>
        {linkedConversations.length > 0 ? (
          linkedConversations.slice(0, 3).map((conversation) => (
            <View key={conversation.id} style={styles.linkedCard}>
              <Text style={styles.linkedTitle}>{conversation.title || conversation.type}</Text>
              <Text style={styles.linkedMeta}>状态 {conversation.status} · 优先级 {conversation.priority}</Text>
              <Text style={styles.linkedMeta}>负责人 {conversation.assignee_display_name || conversation.assignee_email || "未指派"}</Text>
              {conversation.active_room_id || conversation.latest_room_id ? (
                <Text style={styles.linkedMeta}>
                  最近会议 {conversation.active_room_title || conversation.latest_room_title || "Meeting"}
                </Text>
              ) : null}
              <View style={styles.linkedActions}>
                <PrimaryButton title="打开线程" onPress={() => handleOpenLinkedConversation(conversation)} style={styles.linkedButton} />
                {conversation.active_room_id || conversation.latest_room_id ? (
                  <PrimaryButton title="打开会议" onPress={() => void handleOpenRecentMeeting(conversation)} style={styles.linkedSecondaryButton} />
                ) : null}
              </View>
            </View>
          ))
        ) : (
          <Text style={styles.impactText}>该联系人暂时还没有绑定到协作线程。</Text>
        )}
      </View>

      <View style={styles.impactCard}>
        <Text style={styles.sectionTitle}>管理</Text>
        <Text style={styles.impactText}>
          删除联系人不会删除历史通话记录；回拨和业务资料会在重新添加后恢复为新的关系。
        </Text>
        <TouchableOpacity style={styles.removeButton} onPress={() => void handleRemove()}>
          <Text style={styles.removeButtonText}>删除联系人</Text>
        </TouchableOpacity>
      </View>
    </ScrollView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc"
  },
  content: {
    padding: 16,
    gap: 16
  },
  heroCard: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18
  },
  chatButton: {
    marginTop: 10,
    backgroundColor: "#0f172a"
  },
  name: {
    fontSize: 24,
    fontWeight: "800",
    color: "#0f172a"
  },
  email: {
    marginTop: 6,
    color: "#64748b"
  },
  meta: {
    marginTop: 8,
    color: "#334155"
  },
  formCard: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18
  },
  impactCard: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "800",
    color: "#0f172a",
    marginBottom: 12
  },
  impactText: {
    color: "#475569",
    lineHeight: 20,
    marginBottom: 14
  },
  followupSummary: {
    color: "#334155",
    lineHeight: 22
  },
  followupMeta: {
    marginTop: 10,
    color: "#0f172a",
    fontWeight: "700"
  },
  followupDraft: {
    marginTop: 10,
    color: "#475569",
    lineHeight: 20,
    marginBottom: 12
  },
  taskList: {
    marginTop: 10,
    marginBottom: 12,
    gap: 6
  },
  taskItem: {
    color: "#334155"
  },
  followupHint: {
    marginTop: 12,
    color: "#94a3b8",
    fontSize: 12
  },
  linkedCard: {
    borderWidth: 1,
    borderColor: "#e2e8f0",
    borderRadius: 14,
    padding: 14,
    marginBottom: 10
  },
  linkedTitle: {
    fontSize: 16,
    fontWeight: "700",
    color: "#0f172a"
  },
  linkedMeta: {
    marginTop: 6,
    color: "#475569"
  },
  linkedActions: {
    flexDirection: "row",
    gap: 10,
    marginTop: 12
  },
  linkedButton: {
    flex: 1
  },
  linkedSecondaryButton: {
    flex: 1,
    backgroundColor: "#475569"
  },
  removeButton: {
    backgroundColor: "#dc2626",
    borderRadius: 12,
    paddingVertical: 12,
    alignItems: "center"
  },
  removeButtonText: {
    color: "#fff",
    fontWeight: "700"
  }
});

export default ContactDetailScreen;
