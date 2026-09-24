import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Alert, Linking, ScrollView, View, useWindowDimensions } from "react-native";
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
  type RecordingRecord,
} from "../api/collaboration";
import { listContacts, type User } from "../api/users";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import fileDownloadAdapter from "../platform/fileDownload";
import ChatRealtimeService from "../services/ChatRealtimeService";
import {
  createWorkflowRun,
  fetchWorkflowRun,
  listWorkflowRuns,
  processWorkflowRun,
  submitToolApprovalDecision,
  type AgentCitation,
  type CreateWorkflowRequest,
  type ToolApprovalRecord,
  type WorkflowResult,
} from "../api/agent";
import {
  fetchKnowledgeSource,
  type KnowledgeSourceDetail,
} from "../api/knowledge";
import {
  applyConversationDetailPatch,
  type ConversationUpdatedPayload,
} from "../services/conversationRealtimeReducer";
import { buildConversationShareLinks } from "../utils/invitations";
import {
  meetingTranscriptStatusLabel,
  workflowStatusLabel,
  STATUS_OPTIONS,
  PRIORITY_OPTIONS,
  WORKFLOW_TASK_ORDER,
  WORKFLOW_TERMINAL_STATUSES,
} from "./conversationDetailUtils";
import {
  WorkspacePane,
  MessagePane,
  KnowledgePreviewModal,
  CitationPreviewModal,
  WorkflowDebugModal,
  styles,
} from "./conversationDetail";

type Props = NativeStackScreenProps<RootStackParamList, "ConversationDetail">;

const ConversationDetailScreen: React.FC<Props> = ({ route, navigation }) => {
  const { token, user } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const { width } = useWindowDimensions();
  const [detail, setDetail] = useState<ConversationDetailRecord | null>(null);
  const [contacts, setContacts] = useState<User[]>([]);
  const [notes, setNotes] = useState<ConversationNoteRecord[]>([]);
  const [messages, setMessages] = useState<MessageRecord[]>([]);
  const [loadingMorePrev, setLoadingMorePrev] = useState(false);
  const [hasMorePrev, setHasMorePrev] = useState(false);
  const [latestRecording, setLatestRecording] =
    useState<RecordingRecord | null>(null);
  const [draft, setDraft] = useState("");
  const [noteDraft, setNoteDraft] = useState("");
  const [loading, setLoading] = useState(false);
  const [activeWorkflow, setActiveWorkflow] = useState<WorkflowResult | null>(
    null,
  );
  const [workflowLoading, setWorkflowLoading] = useState(false);
  const [workflowDebugVisible, setWorkflowDebugVisible] = useState(false);
  const [citationPreview, setCitationPreview] = useState<AgentCitation | null>(
    null,
  );
  const [knowledgePreview, setKnowledgePreview] =
    useState<KnowledgeSourceDetail | null>(null);
  const conversationId =
    route.params.conversationId ?? route.params.conversation?.id ?? 0;

  const conversation = detail?.conversation ??
    route.params.conversation ?? {
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
      const [nextDetail, nextMessages, nextNotes, nextContacts, nextWorkflows] =
        await Promise.all([
          fetchConversationDetail(token, conversationId),
          listMessages(token, conversationId),
          listConversationNotes(token, conversationId),
          listContacts(token),
          listWorkflowRuns(token, { conversation_id: conversationId, limit: 20 }),
        ]);
      const nextRecording =
        nextDetail.workspace?.latest_recording ??
        (nextDetail.conversation.latest_recording_id
          ? await fetchRecording(
              token,
              nextDetail.conversation.latest_recording_id,
            )
          : null);
      setDetail(nextDetail);
      setContacts(nextContacts);
      setNotes(nextNotes);
      setMessages(nextMessages.messages);
      setHasMorePrev(Boolean(nextMessages.has_more_prev));
      setLatestRecording(nextRecording);
      setActiveWorkflow(
        nextWorkflows[0] ?? null,
      );
      await markConversationRead(token, conversationId);
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Failed to load conversation detail:",
        error,
      );
      Alert.alert("加载失败", "无法加载协作线程详情。");
    } finally {
      setLoading(false);
    }
  }, [conversationId, token]);

  // #25：增量刷新——note / recording 事件只重拉对应的单个端点，不再触发 5 路全量重载。
  // loadData() 保留为全量兜底（首次加载、以及会改变会话状态本身的事件）。
  const refreshNotes = useCallback(async () => {
    if (!token) return;
    try {
      setNotes(await listConversationNotes(token, conversationId));
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Failed to refresh notes:",
        error,
      );
    }
  }, [conversationId, token]);

  // 用 ref 持有最新 recording id：避免把它放进订阅 effect 的依赖里，
  // 否则每次 detail 变化都会重新订阅实时通道。
  const recordingIdRef = useRef<number | null>(null);
  useEffect(() => {
    // RecordingRecord 的 id 在 session 上而非顶层，这里直接用会话上的
    // latest_recording_id（与 loadData 的取数路径一致）。
    recordingIdRef.current = detail?.conversation.latest_recording_id ?? null;
  }, [detail]);

  const refreshRecording = useCallback(async () => {
    const recordingId = recordingIdRef.current;
    if (!token || !recordingId) return;
    try {
      setLatestRecording(await fetchRecording(token, recordingId));
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Failed to refresh recording:",
        error,
      );
    }
  }, [token]);

  const loadMorePrev = useCallback(async () => {
    if (!token || loadingMorePrev || !hasMorePrev || messages.length === 0) {
      return;
    }
    try {
      setLoadingMorePrev(true);
      const oldestId = messages[0].id;
      const page = await listMessages(token, conversationId, {
        beforeId: oldestId,
        limit: 50,
      });
      setMessages((previous) => [...page.messages, ...previous]);
      setHasMorePrev(Boolean(page.has_more_prev));
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Failed to load earlier messages:",
        error,
      );
      Alert.alert("加载失败", "无法加载更早的消息。");
    } finally {
      setLoadingMorePrev(false);
    }
  }, [conversationId, hasMorePrev, loadingMorePrev, messages, token]);

  // Append a message in place (oldest→newest order is preserved by the list).
  // Dedupe by id so the realtime echo of a message we already have is ignored,
  // and skip messages that belong to a different conversation in the same org.
  const appendMessage = useCallback((incoming: MessageRecord) => {
    if (incoming.conversation_id !== conversationId) {
      return;
    }
    setMessages((previous) => {
      if (previous.some((item) => item.id === incoming.id)) {
        return previous;
      }
      return [...previous, incoming];
    });
  }, [conversationId]);

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
    const handleEvent = (event: {
      event: string;
      organization_id: number;
      payload: unknown;
    }) => {
      if (event.event === "conversation.updated") {
        setDetail((previous) =>
          applyConversationDetailPatch(
            previous,
            event.payload as ConversationUpdatedPayload,
          ),
        );
        return;
      }
      if (event.event === "message.created") {
        // Append in place instead of full reload so any "load earlier" history
        // the user scrolled into is preserved; dedupe by id guards the echo.
        appendMessage(event.payload as MessageRecord);
        return;
      }
      // 窄事件只做定点增量刷新（#25）；会改变会话状态本身的事件仍走全量兜底。
      if (event.event === "conversation.note.created") {
        void refreshNotes();
        return;
      }
      if (event.event === "room.recording.updated") {
        void refreshRecording();
        return;
      }
      if (["room.state.updated", "room.ended"].includes(event.event)) {
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
  }, [
    conversationId,
    currentOrganization,
    loadData,
    refreshNotes,
    refreshRecording,
    token,
  ]);

  const activeWorkflowId = activeWorkflow?.workflow.id;
  const activeWorkflowStatus = activeWorkflow?.workflow.status;

  // Workflow progress is pushed over the websocket as `workflow.updated` — the
  // backend publishes it synchronously on every lifecycle transition — so this
  // screen no longer polls fetchWorkflowRun every 1.5s.
  useEffect(() => {
    if (!token || !activeWorkflowId || !activeWorkflowStatus) {
      return;
    }
    if (WORKFLOW_TERMINAL_STATUSES.has(activeWorkflowStatus)) {
      return;
    }
    let cancelled = false;
    const refreshWorkflow = () => {
      void fetchWorkflowRun(token, activeWorkflowId)
        .then((next) => {
          if (cancelled) return;
          setActiveWorkflow(next);
          if (WORKFLOW_TERMINAL_STATUSES.has(next.workflow.status)) {
            void loadData();
          }
        })
        .catch((error) => {
          console.error(
            "[ConversationDetailScreen] Workflow refresh failed:",
            error,
          );
        });
    };
    const handleWorkflowEvent = (event: {
      event: string;
      organization_id: number;
      payload: unknown;
    }) => {
      if (event.event !== "workflow.updated") return;
      const payload = event.payload as
        | {
            workflow_run_id?: number | string;
            conversation_id?: number | string;
          }
        | undefined;
      if (payload?.workflow_run_id != null) {
        if (String(payload.workflow_run_id) !== String(activeWorkflowId)) return;
      } else if (payload?.conversation_id != null) {
        if (String(payload.conversation_id) !== String(conversationId)) return;
      } else {
        return;
      }
      refreshWorkflow();
    };
    ChatRealtimeService.on("event", handleWorkflowEvent);
    return () => {
      cancelled = true;
      ChatRealtimeService.off("event", handleWorkflowEvent);
    };
  }, [activeWorkflowId, activeWorkflowStatus, conversationId, loadData, token]);

  const assigneeLabel = useMemo(() => {
    if (conversation.assignee_user_id === user?.id) {
      return "我";
    }
    return (
      conversation.assignee_display_name ||
      conversation.assignee_email ||
      "未指派"
    );
  }, [
    conversation.assignee_display_name,
    conversation.assignee_email,
    conversation.assignee_user_id,
    user?.id,
  ]);

  const boundContact = useMemo(
    () => contacts.find((item) => item.id === conversation.contact_id),
    [contacts, conversation.contact_id],
  );
  const agentContext = detail?.workspace.agent_context;
  const directTranscriptCount = agentContext?.transcript_segment_count ?? 0;
  const meetingTranscriptCount =
    agentContext?.meeting_transcript_segment_count ??
    detail?.workspace.latest_recording?.transcription?.segment_count ??
    0;
  const meetingTranscriptionStatus =
    agentContext?.meeting_transcription_status ||
    detail?.workspace.latest_recording?.transcription?.status;
  const meetingTranscriptionError =
    agentContext?.meeting_transcription_error ||
    detail?.workspace.latest_recording?.transcription?.error_message;
  const transcriptStatusText = meetingTranscriptStatusLabel(
    meetingTranscriptionStatus,
    meetingTranscriptCount,
    directTranscriptCount,
    meetingTranscriptionError,
  );
  const meetingTranscriptReady =
    meetingTranscriptionStatus === "ready" && meetingTranscriptCount > 0;
  const pendingApprovals =
    activeWorkflow?.approvals?.filter((item) => item.status === "pending") ??
    [];
  const agentStatusLabel = workflowStatusLabel(
    activeWorkflow,
    agentContext?.pending_approval_count ?? pendingApprovals.length,
    workflowLoading,
  );
  const completedTaskCount =
    activeWorkflow?.tasks.filter((item) => item.status === "ready").length ?? 0;
  const executedApprovalCount =
    activeWorkflow?.approvals.filter((item) => item.status === "executed")
      .length ?? 0;
  const rejectedApprovalCount =
    activeWorkflow?.approvals.filter((item) => item.status === "rejected")
      .length ?? 0;
  const orderedWorkflowTasks = useMemo(
    () =>
      [...(activeWorkflow?.tasks ?? [])].sort(
        (left, right) =>
          WORKFLOW_TASK_ORDER.indexOf(left.name) -
          WORKFLOW_TASK_ORDER.indexOf(right.name),
      ),
    [activeWorkflow?.tasks],
  );

  const handleCopyConversationLink = async () => {
    const links = buildConversationShareLinks(conversationId);
    await Clipboard.setStringAsync(links.webURL);
    Alert.alert("已复制", "线程 Web 链接已复制到剪贴板。");
  };

  const handleSend = useCallback(async () => {
    if (!token || !draft.trim()) return;
    try {
      const created = await createMessage(token, conversationId, { body: draft.trim() });
      setDraft("");
      // Append rather than full-reload: preserves any "load earlier" history and
      // avoids flicker. The realtime echo of this message is deduped by id.
      appendMessage(created);
    } catch (e) {
      console.error(e);
      Alert.alert("发送失败");
    }
  }, [token, draft, conversationId, appendMessage]);

  const runMeetingAgent = useCallback(
    async (input: {
      preset?: CreateWorkflowRequest["preset"];
      goal?: string;
    }) => {
      if (!token) return;
      if ((input.preset ?? "meeting_brief") === "meeting_brief" && !meetingTranscriptReady) {
        Alert.alert(
          "会议转写尚未就绪",
          "会议复盘必须基于已完成的录音转写。请等待转写完成后重试。",
        );
        return;
      }
      try {
        setWorkflowLoading(true);
        const created = await createWorkflowRun(token, {
          conversation_id: conversationId,
          preset: input.preset ?? "meeting_brief",
          goal: input.goal,
        });
        const processed = await processWorkflowRun(token, created.workflow.id);
        setActiveWorkflow(processed);
        if (processed.workflow.status === "ready") {
          await loadData();
        } else if (processed.workflow.status === "requires_action") {
          Alert.alert(
            "等待审批",
            "Meeting Agent 已完成分析，但写回线程前还需要审批。",
          );
        } else if (processed.workflow.status === "failed") {
          Alert.alert(
            "Agent 运行失败",
            processed.workflow.error_message || "workflow 执行失败。",
          );
        }
      } catch (e) {
        console.error("Run Meeting Agent failed", e);
        Alert.alert("Agent 调用失败");
      } finally {
        setWorkflowLoading(false);
      }
    },
    [conversationId, loadData, meetingTranscriptReady, token],
  );

  const handleAskAgent = useCallback(async () => {
    const goal = draft.trim();
    await runMeetingAgent({ goal: goal || undefined });
    if (goal) {
      setDraft("");
    }
  }, [draft, runMeetingAgent]);

  const handleApprovalDecision = useCallback(
    async (approval: ToolApprovalRecord, decision: "approve" | "reject") => {
      if (!token) return;
      try {
        setWorkflowLoading(true);
        const updated = await submitToolApprovalDecision(
          token,
          approval.id,
          decision,
        );
        const processed = await processWorkflowRun(token, updated.workflow.id);
        setActiveWorkflow(processed);
        await loadData();
      } catch (error) {
        console.error(
          "[ConversationDetailScreen] Approval submission failed:",
          error,
        );
        Alert.alert("审批失败", "无法处理当前工具审批。");
      } finally {
        setWorkflowLoading(false);
      }
    },
    [loadData, token],
  );

  const handleCitationPress = useCallback(
    async (citation: AgentCitation) => {
      if (!token) return;
      if (citation.knowledge_source_id) {
        try {
          const detail = await fetchKnowledgeSource(
            token,
            citation.knowledge_source_id,
          );
          setKnowledgePreview(detail);
          setCitationPreview(null);
          return;
        } catch (error) {
          console.error(
            "[ConversationDetailScreen] Knowledge citation load failed:",
            error,
          );
          Alert.alert("加载失败", "无法打开知识源预览。");
          return;
        }
      }
      if (citation.source_type === "meeting_transcript" && citation.recording_session_id) {
        navigation.navigate("RecordingTranscript", {
          recordingId: citation.recording_session_id,
          segmentId: citation.transcript_segment_id,
          startMs: citation.start_ms,
        });
        return;
      }
      if (citation.origin_url) {
        try {
          await Linking.openURL(citation.origin_url);
          return;
        } catch (error) {
          console.error(
            "[ConversationDetailScreen] Citation URL open failed:",
            error,
          );
        }
      }
      if (
        citation.conversation_id &&
        citation.conversation_id !== conversationId
      ) {
        navigation.navigate("ConversationDetail", {
          conversationId: citation.conversation_id,
        });
        return;
      }
      setCitationPreview(citation);
      setKnowledgePreview(null);
    },
    [conversationId, navigation, token],
  );

  const handleProcessCurrentWorkflow = useCallback(async () => {
    if (!token || !activeWorkflow) {
      return;
    }
    try {
      setWorkflowLoading(true);
      const next = await processWorkflowRun(token, activeWorkflow.workflow.id);
      setActiveWorkflow(next);
      await loadData();
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Workflow process failed:",
        error,
      );
      Alert.alert("处理失败", "无法推进 workflow。");
    } finally {
      setWorkflowLoading(false);
    }
  }, [activeWorkflow, loadData, token]);

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
      const updated = await updateConversation(token, conversationId, {
        assignee_user_id: user.id,
      });
      setDetail((previous) =>
        previous ? { ...previous, conversation: updated } : previous,
      );
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
      const updated = await updateConversation(token, conversationId, {
        assignee_user_id: 0,
      });
      setDetail((previous) =>
        previous ? { ...previous, conversation: updated } : previous,
      );
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Failed to unassign conversation:",
        error,
      );
      Alert.alert("更新失败", "无法清空负责人。");
    }
  };

  const handleUpdateStatus = async (
    status: (typeof STATUS_OPTIONS)[number],
  ) => {
    if (!token) {
      return;
    }
    try {
      const updated = await updateConversation(token, conversationId, {
        status,
      });
      setDetail((previous) =>
        previous ? { ...previous, conversation: updated } : previous,
      );
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Failed to update status:",
        error,
      );
      Alert.alert("更新失败", "无法更新会话状态。");
    }
  };

  const handleUpdatePriority = async (
    priority: (typeof PRIORITY_OPTIONS)[number],
  ) => {
    if (!token) {
      return;
    }
    try {
      const updated = await updateConversation(token, conversationId, {
        priority,
      });
      setDetail((previous) =>
        previous ? { ...previous, conversation: updated } : previous,
      );
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Failed to update priority:",
        error,
      );
      Alert.alert("更新失败", "无法更新优先级。");
    }
  };

  const handleCreateMeeting = async () => {
    if (!token) {
      return;
    }
    try {
      const room = await createConversationRoom(
        token,
        conversationId,
        `${conversation.title || "协作线程"} 会议`,
      );
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
      const updated = await updateConversation(token, conversationId, {
        contact_id: contactId,
      });
      setDetail((previous) =>
        previous ? { ...previous, conversation: updated } : previous,
      );
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Failed to bind contact:",
        error,
      );
      Alert.alert("更新失败", "无法绑定联系人。");
    }
  };

  const handleDownloadRecording = async (
    recordingId: number,
    fileId: number,
    fileName: string,
  ) => {
    if (!token) {
      return;
    }
    try {
      const request = buildRecordingDownloadRequest(token, recordingId, fileId);
      const result = await fileDownloadAdapter.download(
        request,
        fileName || `recording-${fileId}`,
      );
      try {
        await fileDownloadAdapter.open(result);
      } catch {
        Alert.alert("下载完成", `文件已保存到 ${result.location}`);
      }
    } catch (error) {
      console.error(
        "[ConversationDetailScreen] Failed to download recording:",
        error,
      );
      Alert.alert("下载失败", "无法下载最近录音资产。");
    }
  };

  const handleOpenTranscript = useCallback(
    (recordingId: number) => {
      navigation.navigate("RecordingTranscript", { recordingId });
    },
    [navigation],
  );

  const workspacePaneProps = {
    conversation,
    workspace: detail?.workspace,
    latestRoom: detail?.latest_room,
    latestFollowup: detail?.latest_followup,
    assigneeLabel,
    boundContact,
    agentContext,
    contacts,
    notes,
    latestRecording,
    activeWorkflow,
    pendingApprovals,
    completedTaskCount,
    executedApprovalCount,
    rejectedApprovalCount,
    transcriptStatusText,
    agentStatusLabel,
    meetingTranscriptReady,
    workflowLoading,
    navigation,
    onCopyLink: handleCopyConversationLink,
    onCreateMeeting: handleCreateMeeting,
    onAssignSelf: handleAssignSelf,
    onUnassign: handleUnassign,
    onRunMeetingAgent: runMeetingAgent,
    onOpenWorkflowDebug: () => setWorkflowDebugVisible(true),
    onCitationPress: handleCitationPress,
    onApprovalDecision: handleApprovalDecision,
    onUpdateStatus: handleUpdateStatus,
    onUpdatePriority: handleUpdatePriority,
    onBindContact: handleBindContact,
    onDownloadRecording: handleDownloadRecording,
    noteDraft,
    onNoteDraftChange: setNoteDraft,
    onAddNote: handleAddNote,
  };

  const messagePaneProps = {
    messages,
    loading,
    hasMorePrev,
    loadingMorePrev,
    draft,
    workflowLoading,
    currentUserId: user?.id,
    onRefresh: () => void loadData(),
    onLoadMorePrev: loadMorePrev,
    onDraftChange: setDraft,
    onSend: handleSend,
    onAskAgent: handleAskAgent,
    onOpenTranscript: handleOpenTranscript,
  };

  return (
    <View style={styles.container}>
      {isWideScreen ? (
        <View style={styles.desktopLayout}>
          <ScrollView
            style={styles.workspaceColumn}
            contentContainerStyle={styles.workspaceColumnContent}
          >
            <WorkspacePane {...workspacePaneProps} />
          </ScrollView>
          <View style={styles.messageColumn}>
            <MessagePane {...messagePaneProps} />
          </View>
        </View>
      ) : (
        <>
          <WorkspacePane {...workspacePaneProps} />
          <MessagePane {...messagePaneProps} />
        </>
      )}

      <KnowledgePreviewModal
        knowledgePreview={knowledgePreview}
        onClose={() => setKnowledgePreview(null)}
      />
      <CitationPreviewModal
        citationPreview={citationPreview}
        onClose={() => setCitationPreview(null)}
      />
      <WorkflowDebugModal
        visible={workflowDebugVisible}
        activeWorkflow={activeWorkflow}
        orderedTasks={orderedWorkflowTasks}
        workflowLoading={workflowLoading}
        token={token}
        onClose={() => setWorkflowDebugVisible(false)}
        onProcess={handleProcessCurrentWorkflow}
      />
    </View>
  );
};

export default ConversationDetailScreen;
