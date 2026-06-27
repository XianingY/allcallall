import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  FlatList,
  Linking,
  Modal,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
  useWindowDimensions,
} from "react-native";
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
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import fileDownloadAdapter from "../platform/fileDownload";
import ChatRealtimeService from "../services/ChatRealtimeService";
import {
  createWorkflowRun,
  fetchWorkflowRun,
  listWorkflowRuns,
  processWorkflowRun,
  submitToolApprovalDecision,
  type AgentCitation,
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

type Props = NativeStackScreenProps<RootStackParamList, "ConversationDetail">;

const STATUS_OPTIONS = ["open", "pending", "resolved"] as const;
const PRIORITY_OPTIONS = ["low", "normal", "high", "urgent"] as const;
const MEETING_PRESETS = [
  { key: "meeting_brief", label: "Meeting Brief" },
  { key: "follow_up", label: "Follow-up" },
  { key: "risk_review", label: "Risk Review" },
] as const;
const WORKFLOW_TASK_ORDER = [
  "collect_context",
  "decompose",
  "searcher",
  "summarizer",
  "risk_analyst",
  "merge",
  "propose_tools",
  "approval",
  "commit_result",
];

const WORKFLOW_TERMINAL_STATUSES = new Set([
  "ready",
  "requires_action",
  "failed",
]);

const parseJSONRecord = (raw?: string): Record<string, unknown> => {
  if (!raw) return {};
  try {
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
};

const toTextList = (value: unknown): string[] =>
  Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];

const workflowStatusLabel = (
  workflow: WorkflowResult | null,
  pendingApprovalCount: number,
  loading: boolean,
) => {
  if (loading) return "运行中";
  const status = workflow?.workflow.status;
  if (!status) return "未运行";
  if (status === "requires_action" || pendingApprovalCount > 0) {
    return "等待审批";
  }
  if (status === "ready") return "已写回";
  if (status === "failed") return "失败";
  if (status === "running" || status === "pending") return "运行中";
  return status;
};

const meetingTranscriptStatusLabel = (
  status: string | undefined,
  meetingCount: number,
  directCount: number,
  error?: string,
) => {
  if (meetingCount > 0) {
    return `${meetingCount} recording transcript segments ready`;
  }
  if (directCount > 0) {
    return `${directCount} final transcript segments`;
  }
  switch (status) {
    case "pending":
      return "Recording transcription queued";
    case "processing":
      return "Recording transcription processing";
    case "failed":
      return error
        ? `Recording transcription failed: ${error}`
        : "Recording transcription failed";
    case "skipped":
      return "Recording transcription skipped";
    default:
      return "No transcript yet; using notes and messages";
  }
};

const citationModeLabel = (mode?: string) => {
  switch (mode) {
    case "hybrid_rrf":
      return "Hybrid";
    case "bm25":
      return "BM25";
    case "vector":
      return "Vector";
    case "sql_fallback":
      return "Fallback";
    default:
      return mode || "Context";
  }
};

const readableToolName = (toolName: string) => {
  switch (toolName) {
    case "write_conversation_message":
      return "写入会话消息";
    case "create_follow_up_task":
      return "创建跟进任务";
    case "upsert_agent_memory":
      return "更新 Agent 记忆";
    case "delegate_task":
      return "委派子任务";
    default:
      return toolName;
  }
};

const approvalPreview = (approval: ToolApprovalRecord) => {
  const input = parseJSONRecord(approval.input_json);
  const actionItems = toTextList(input.action_items);
  const riskFlags = toTextList(input.risk_flags);
  const lines: string[] = [];
  if (typeof input.summary === "string" && input.summary.trim()) {
    lines.push(`摘要：${input.summary.trim()}`);
  }
  if (typeof input.next_step === "string" && input.next_step.trim()) {
    lines.push(`下一步：${input.next_step.trim()}`);
  }
  if (typeof input.key === "string" && input.key.trim()) {
    lines.push(`Memory key：${input.key.trim()}`);
  }
  if (actionItems.length > 0) {
    lines.push(`行动项：${actionItems.join(" / ")}`);
  }
  if (riskFlags.length > 0) {
    lines.push(`风险：${riskFlags.join(" / ")}`);
  }
  if (lines.length === 0 && approval.input_json) {
    lines.push(approval.input_json);
  }
  return {
    title: readableToolName(approval.tool_name),
    lines,
  };
};

const ConversationDetailScreen: React.FC<Props> = ({ route, navigation }) => {
  const { token, user } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const { width } = useWindowDimensions();
  const [detail, setDetail] = useState<ConversationDetailRecord | null>(null);
  const [contacts, setContacts] = useState<User[]>([]);
  const [notes, setNotes] = useState<ConversationNoteRecord[]>([]);
  const [messages, setMessages] = useState<MessageRecord[]>([]);
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
      setMessages(nextMessages);
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
      if (
        [
          "message.created",
          "conversation.note.created",
          "room.recording.updated",
          "room.state.updated",
          "room.ended",
        ].includes(event.event)
      ) {
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

  useEffect(() => {
    if (!token || !activeWorkflow) {
      return;
    }
    const workflowId = activeWorkflow.workflow.id;
    if (WORKFLOW_TERMINAL_STATUSES.has(activeWorkflow.workflow.status)) {
      return;
    }
    let cancelled = false;
    const timer = setInterval(() => {
      void fetchWorkflowRun(token, workflowId)
        .then((next) => {
          if (cancelled) return;
          setActiveWorkflow(next);
          if (WORKFLOW_TERMINAL_STATUSES.has(next.workflow.status)) {
            void loadData();
          }
        })
        .catch((error) => {
          console.error(
            "[ConversationDetailScreen] Workflow polling failed:",
            error,
          );
        });
    }, 1500);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [
    activeWorkflow?.workflow.id,
    activeWorkflow?.workflow.status,
    loadData,
    token,
  ]);

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
      await createMessage(token, conversationId, { body: draft.trim() });
      setDraft("");
      void loadData();
    } catch (e) {
      console.error(e);
      Alert.alert("发送失败");
    }
  }, [token, draft, conversationId, loadData]);

  const runMeetingAgent = useCallback(
    async (input: {
      preset?: "meeting_brief" | "follow_up" | "risk_review";
      goal?: string;
    }) => {
      if (!token) return;
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
    [conversationId, loadData, token],
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

  const workspacePane = (
    <>
      <Text style={styles.heading}>{conversation.title || "协作线程"}</Text>

      <View style={styles.summaryCard}>
        <Text style={styles.summaryText}>
          负责人 {detail?.workspace.assignee_label || assigneeLabel}
        </Text>
        <Text style={styles.summaryText}>
          状态 {detail?.workspace.status || conversation.status}
        </Text>
        <Text style={styles.summaryText}>
          优先级 {detail?.workspace.priority || conversation.priority}
        </Text>
        <Text style={styles.summaryText}>
          关联联系人{" "}
          {boundContact?.display_name || boundContact?.email || "未绑定"}
        </Text>
        <PrimaryButton
          title="复制线程 Web 链接"
          onPress={() => void handleCopyConversationLink()}
          style={styles.inlineButtonSecondary}
        />
        {detail?.latest_room ? (
          <PrimaryButton
            title="进入当前会议"
            onPress={() =>
              navigation.navigate("PreJoin", {
                roomId: detail.latest_room?.id ?? 0,
                title: detail.latest_room?.title ?? "Meeting",
                conversationId: detail.latest_room?.conversation_id ?? null,
                joinOptions: {
                  audioEnabled: true,
                  videoEnabled: true,
                  cameraFacing: "front",
                  speakerOn: true,
                },
              })
            }
            style={styles.inlineButton}
          />
        ) : (
          <PrimaryButton
            title="升级为会议"
            onPress={handleCreateMeeting}
            style={styles.inlineButton}
          />
        )}
      </View>

      <View style={styles.buttonRow}>
        <PrimaryButton
          title="指派给我"
          onPress={handleAssignSelf}
          style={styles.button}
        />
        <PrimaryButton
          title="清空负责人"
          onPress={handleUnassign}
          style={styles.buttonSecondary}
        />
      </View>

      <View style={styles.infoCard}>
        <View style={styles.agentHeader}>
          <View>
            <Text style={styles.infoTitle}>Meeting Agent</Text>
            <Text style={styles.infoMeta}>{transcriptStatusText}</Text>
          </View>
          <View style={styles.agentStatusBadge}>
            <Text style={styles.agentStatusText}>{agentStatusLabel}</Text>
          </View>
        </View>
        <View style={styles.agentContextGrid}>
          <Text style={styles.agentContextItem}>
            Call {agentContext?.latest_call_id || "-"}
          </Text>
          <Text style={styles.agentContextItem}>
            Knowledge {agentContext?.knowledge_source_count ?? 0}
          </Text>
          <Text style={styles.agentContextItem}>
            Approvals {agentContext?.pending_approval_count ?? pendingApprovals.length}
          </Text>
          <Text style={styles.agentContextItem}>
            Workflow {agentContext?.last_workflow_id ?? activeWorkflow?.workflow.id ?? "-"}
          </Text>
        </View>
        {agentContext?.latest_transcript_at ? (
          <Text style={styles.infoMeta}>
            Latest transcript{" "}
            {new Date(agentContext.latest_transcript_at).toLocaleString()}
          </Text>
        ) : null}
        {agentContext?.last_agent_run_at ? (
          <Text style={styles.infoMeta}>
            Last workflow {agentContext.last_agent_status || "-"} ·{" "}
            {agentContext.last_workflow_preset || activeWorkflow?.workflow.preset || "custom"} ·{" "}
            {new Date(agentContext.last_agent_run_at).toLocaleString()}
          </Text>
        ) : null}
        {activeWorkflow ? (
          <Text style={styles.infoMeta}>
            Progress {completedTaskCount}/{activeWorkflow.tasks.length} tasks ·
            write-back executed {executedApprovalCount}
            {rejectedApprovalCount ? ` · rejected ${rejectedApprovalCount}` : ""}
          </Text>
        ) : null}
        {agentContext?.latest_memory_keys?.length ? (
          <View style={styles.memoryChipRow}>
            {agentContext.latest_memory_keys.map((key) => (
              <Text key={key} style={styles.memoryChip}>
                {key}
              </Text>
            ))}
          </View>
        ) : null}
        <View style={styles.optionRow}>
          {MEETING_PRESETS.map((preset) => (
            <PrimaryButton
              key={preset.key}
              title={preset.label}
              onPress={() => void runMeetingAgent({ preset: preset.key })}
              disabled={workflowLoading}
              style={styles.option}
            />
          ))}
        </View>
        <View style={styles.buttonRow}>
          <PrimaryButton
            title="Knowledge Center"
            onPress={() => navigation.navigate("KnowledgeCenter")}
            style={styles.button}
          />
          <PrimaryButton
            title="Workflow Debug"
            onPress={() => setWorkflowDebugVisible(true)}
            disabled={!activeWorkflow}
            style={styles.buttonSecondary}
          />
        </View>
        {activeWorkflow?.workflow?.summary ? (
          <View style={styles.agentResultBox}>
            <Text style={styles.citationTitle}>Result</Text>
            <Text style={styles.infoBody}>{activeWorkflow.workflow.summary}</Text>
          </View>
        ) : (
          <Text style={styles.infoMeta}>
            基于 final transcript、follow-up、memory 和线程上下文生成 grounded
            结果。
          </Text>
        )}
        {activeWorkflow?.workflow?.next_step ? (
          <Text style={styles.infoMeta}>
            Next step {activeWorkflow.workflow.next_step}
          </Text>
        ) : null}
        {activeWorkflow?.workflow?.action_items?.length ? (
          <Text style={styles.infoMeta}>
            Action items {activeWorkflow.workflow.action_items.join(" / ")}
          </Text>
        ) : null}
        {activeWorkflow?.citations?.length ? (
          <View style={styles.citationList}>
            {activeWorkflow.citations.slice(0, 4).map((citation, index) => (
              <Pressable
                key={`${citation.source_type}:${citation.source_id}:${index}`}
                style={styles.citationItem}
                onPress={() => void handleCitationPress(citation)}
              >
                <View style={styles.citationHeader}>
                  <Text style={styles.citationTitle}>{citation.title}</Text>
                  <Text style={styles.citationBadge}>
                    {citationModeLabel(citation.retrieval_mode)}
                  </Text>
                </View>
                <Text style={styles.citationMeta}>
                  {citation.source_type} · score {citation.score}
                </Text>
                <Text style={styles.citationSnippet}>{citation.snippet}</Text>
              </Pressable>
            ))}
          </View>
        ) : null}
        {pendingApprovals.length ? (
          <View style={styles.citationList}>
            <Text style={styles.infoMeta}>
              Pending approvals{" "}
              {pendingApprovals.map((item) => item.tool_name).join(" / ")}
            </Text>
            {pendingApprovals.map((approval) => (
              <View key={approval.id} style={styles.approvalItem}>
                <Text style={styles.citationTitle}>
                  {approvalPreview(approval).title}
                </Text>
                {approvalPreview(approval).lines.map((line) => (
                  <Text key={line} style={styles.citationSnippet}>
                    {line}
                  </Text>
                ))}
                <View style={styles.inlineActionRow}>
                  <Pressable
                    style={styles.approveChip}
                    onPress={() =>
                      void handleApprovalDecision(approval, "approve")
                    }
                  >
                    <Text style={styles.approveChipText}>Approve</Text>
                  </Pressable>
                  <Pressable
                    style={styles.rejectChip}
                    onPress={() =>
                      void handleApprovalDecision(approval, "reject")
                    }
                  >
                    <Text style={styles.rejectChipText}>Reject</Text>
                  </Pressable>
                </View>
              </View>
            ))}
          </View>
        ) : null}
      </View>

      <Text style={styles.sectionTitle}>状态</Text>
      <View style={styles.optionRow}>
        {STATUS_OPTIONS.map((status) => (
          <PrimaryButton
            key={status}
            title={status}
            onPress={() => handleUpdateStatus(status)}
            style={
              conversation.status === status
                ? styles.optionActive
                : styles.option
            }
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
            style={
              conversation.priority === priority
                ? styles.optionActive
                : styles.option
            }
          />
        ))}
      </View>

      <Text style={styles.sectionTitle}>联系人绑定</Text>
      {boundContact ? (
        <View style={styles.infoCard}>
          <Text style={styles.infoTitle}>
            {boundContact.display_name || boundContact.email}
          </Text>
          <Text style={styles.infoMeta}>{boundContact.email}</Text>
          {boundContact.profile?.company ? (
            <Text style={styles.infoMeta}>
              公司 {boundContact.profile.company}
            </Text>
          ) : null}
          {boundContact.profile?.role ? (
            <Text style={styles.infoMeta}>
              角色 {boundContact.profile.role}
            </Text>
          ) : null}
          {boundContact.profile?.timezone ? (
            <Text style={styles.infoMeta}>
              时区 {boundContact.profile.timezone}
            </Text>
          ) : null}
          {boundContact.profile?.default_source_lang ||
          boundContact.profile?.default_target_lang ? (
            <Text style={styles.infoMeta}>
              默认语言 {boundContact.profile?.default_source_lang || "-"} →{" "}
              {boundContact.profile?.default_target_lang || "-"}
            </Text>
          ) : null}
          <View style={styles.buttonRow}>
            <PrimaryButton
              title="查看联系人"
              onPress={() =>
                navigation.navigate("ContactDetail", { contact: boundContact })
              }
              style={styles.button}
            />
            <PrimaryButton
              title="解除绑定"
              onPress={() => handleBindContact(null)}
              style={styles.buttonSecondary}
            />
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
          {detail.workspace.latest_meeting ? (
            <Text style={styles.infoMeta}>
              最近会议 {detail.workspace.latest_meeting.title}
            </Text>
          ) : null}
          {detail.workspace.latest_recording ? (
            <Text style={styles.infoMeta}>
              最近录音资产 #{detail.workspace.latest_recording.session.id}
              {detail.workspace.latest_recording.transcription
                ? ` · 转写 ${detail.workspace.latest_recording.transcription.status}`
                : ""}
            </Text>
          ) : null}
          {detail.workspace.meeting_summary?.summary ? (
            <Text style={styles.infoBody}>
              {detail.workspace.meeting_summary.summary}
            </Text>
          ) : null}
          {detail.workspace.meeting_summary?.action_items?.length ? (
            <Text style={styles.infoMeta}>
              Action items{" "}
              {detail.workspace.meeting_summary.action_items.join(" / ")}
            </Text>
          ) : null}
          {detail.workspace.meeting_summary?.next_step ? (
            <Text style={styles.infoMeta}>
              Next step {detail.workspace.meeting_summary.next_step}
            </Text>
          ) : null}
          {detail.workspace.latest_note ? (
            <Text style={styles.infoMeta}>
              最近备注{" "}
              {detail.workspace.latest_note.author_display_name ||
                detail.workspace.latest_note.author_email}{" "}
              ·{" "}
              {new Date(
                detail.workspace.latest_note.created_at,
              ).toLocaleString()}
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
                {note.author_display_name || note.author_email} ·{" "}
                {new Date(note.created_at).toLocaleString()}
              </Text>
            </View>
          ))}
        </View>
      ) : null}

      {detail?.latest_followup ? (
        <View style={styles.infoCard}>
          <Text style={styles.infoTitle}>最近会议/通话摘要</Text>
          <Text style={styles.infoBody}>
            {detail.workspace.meeting_summary?.summary ||
              detail.latest_followup.summary_cn ||
              detail.latest_followup.summary_en ||
              "暂无摘要"}
          </Text>
          {detail.workspace.meeting_summary?.next_step ||
          detail.latest_followup.next_step ? (
            <Text style={styles.infoMeta}>
              下一步{" "}
              {detail.workspace.meeting_summary?.next_step ||
                detail.latest_followup.next_step}
            </Text>
          ) : null}
        </View>
      ) : null}

      {latestRecording ? (
        <View style={styles.infoCard}>
          <Text style={styles.infoTitle}>最近录音资产</Text>
          <Text style={styles.infoMeta}>
            录音会话 #{latestRecording.session.id}
          </Text>
          <Text style={styles.infoMeta}>
            状态 {latestRecording.session.status}
          </Text>
          <Text style={styles.infoMeta}>
            文件数 {latestRecording.files.length}
          </Text>
          <Text style={styles.infoMeta}>
            转写 {latestRecording.transcription?.status ?? "not_requested"}
            {latestRecording.transcription?.segment_count
              ? ` · ${latestRecording.transcription.segment_count} segments`
              : ""}
          </Text>
          {latestRecording.transcription ? (
            <PrimaryButton
              title="查看会议转写"
              onPress={() =>
                navigation.navigate("RecordingTranscript", {
                  recordingId: latestRecording.session.id,
                })
              }
              style={styles.recordingButton}
            />
          ) : null}
          {latestRecording.files.slice(0, 2).map((file) => (
            <View key={file.id} style={styles.recordingFileRow}>
              <Text style={styles.recordingFileTitle}>{file.file_name}</Text>
              <Text style={styles.infoMeta}>
                {file.recording_kind} · {file.file_size_bytes} bytes ·{" "}
                {file.duration_seconds}s
              </Text>
              <PrimaryButton
                title="下载最近录音"
                onPress={() =>
                  void handleDownloadRecording(
                    latestRecording.session.id,
                    file.id,
                    file.file_name,
                  )
                }
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
      <PrimaryButton
        title="添加内部备注"
        onPress={handleAddNote}
        disabled={!noteDraft.trim()}
        style={styles.createNoteButton}
      />
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
          <View
            style={[
              styles.messageBubble,
              isSystem
                ? styles.systemBubble
                : isMine
                  ? styles.mine
                  : styles.theirs,
            ]}
          >
            <Text style={styles.sender}>
              {item.sender_display_name || item.sender_email}
            </Text>
            <Text style={styles.body}>{item.body || item.type}</Text>
            {item.metadata?.event_type ? (
              <Text style={styles.systemMeta}>
                {String(item.metadata.event_type)}
              </Text>
            ) : null}
            {item.metadata?.event_type === "meeting.transcription.ready" &&
            typeof item.metadata.recording_id === "number" ? (
              <PrimaryButton
                title="查看会议转写"
                onPress={() =>
                  navigation.navigate("RecordingTranscript", {
                    recordingId: item.metadata!.recording_id as number,
                  })
                }
                style={styles.systemAction}
              />
            ) : null}
            <Text style={styles.time}>
              {new Date(item.created_at).toLocaleString()}
            </Text>
          </View>
        );
      }}
      ListFooterComponent={
        <View>
          <View style={styles.composer}>
            <TextField
              value={draft}
              onChangeText={setDraft}
              placeholder="输入线程消息，或输入自定义 Agent goal"
            />
            <View style={styles.buttonRow}>
              <PrimaryButton
                title="发送消息"
                onPress={handleSend}
                disabled={!draft.trim()}
                style={styles.button}
              />
              <PrimaryButton
                title="Run Agent"
                onPress={() => void handleAskAgent()}
                disabled={workflowLoading}
                style={styles.buttonSecondary}
              />
            </View>
          </View>
        </View>
      }
    />
  );

  return (
    <View style={styles.container}>
      {isWideScreen ? (
        <View style={styles.desktopLayout}>
          <ScrollView
            style={styles.workspaceColumn}
            contentContainerStyle={styles.workspaceColumnContent}
          >
            {workspacePane}
          </ScrollView>
          <View style={styles.messageColumn}>{messagePane}</View>
        </View>
      ) : (
        <>
          {workspacePane}

          {messagePane}
        </>
      )}

      <Modal
        visible={knowledgePreview !== null}
        transparent
        animationType="slide"
        onRequestClose={() => setKnowledgePreview(null)}
      >
        <View style={styles.modalBackdrop}>
          <View style={styles.modalCard}>
            <Text style={styles.modalTitle}>
              {knowledgePreview?.source.title || "Knowledge Preview"}
            </Text>
            {knowledgePreview ? (
              <ScrollView style={styles.modalScroll}>
                <Text style={styles.infoMeta}>
                  {knowledgePreview.source.kind} · versions{" "}
                  {knowledgePreview.versions.length} · chunks{" "}
                  {knowledgePreview.chunks.length}
                </Text>
                {knowledgePreview.source.uri ? (
                  <Pressable
                    style={styles.linkRow}
                    onPress={() =>
                      void Linking.openURL(knowledgePreview.source.uri || "")
                    }
                  >
                    <Text style={styles.linkText}>Open origin URL</Text>
                  </Pressable>
                ) : null}
                {knowledgePreview.chunks.slice(0, 8).map((chunk) => (
                  <View key={chunk.id} style={styles.modalSection}>
                    <Text style={styles.citationTitle}>
                      Chunk {chunk.chunk_index}
                    </Text>
                    <Text style={styles.citationMeta}>
                      {chunk.index_status} · offsets {chunk.start_offset}-
                      {chunk.end_offset}
                    </Text>
                    <Text style={styles.citationSnippet}>{chunk.snippet}</Text>
                  </View>
                ))}
              </ScrollView>
            ) : null}
            <PrimaryButton
              title="Close"
              onPress={() => setKnowledgePreview(null)}
              style={styles.modalButton}
            />
          </View>
        </View>
      </Modal>

      <Modal
        visible={citationPreview !== null}
        transparent
        animationType="fade"
        onRequestClose={() => setCitationPreview(null)}
      >
        <View style={styles.modalBackdrop}>
          <View style={styles.modalCard}>
            <Text style={styles.modalTitle}>
              {citationPreview?.source_title ||
                citationPreview?.title ||
                "Citation"}
            </Text>
            {citationPreview ? (
              <>
                <Text style={styles.infoMeta}>
                  {citationPreview.source_type} ·{" "}
                  {citationPreview.retrieval_mode || "context"} · score{" "}
                  {citationPreview.score}
                </Text>
                <Text style={styles.citationSnippet}>
                  {citationPreview.snippet}
                </Text>
              </>
            ) : null}
            <PrimaryButton
              title="Close"
              onPress={() => setCitationPreview(null)}
              style={styles.modalButton}
            />
          </View>
        </View>
      </Modal>

      <Modal
        visible={workflowDebugVisible}
        transparent
        animationType="slide"
        onRequestClose={() => setWorkflowDebugVisible(false)}
      >
        <View style={styles.modalBackdrop}>
          <View style={styles.debugDrawer}>
            <Text style={styles.modalTitle}>Workflow Debug</Text>
            <Text style={styles.infoMeta}>
              {activeWorkflow?.workflow.workflow_version || "-"} ·{" "}
              {activeWorkflow?.workflow.status || "no workflow"}
            </Text>
            <ScrollView style={styles.modalScroll}>
              <Text style={styles.debugHeader}>Tasks</Text>
              {orderedWorkflowTasks.map((task) => (
                <View key={task.id} style={styles.modalSection}>
                  <Text style={styles.citationTitle}>
                    {task.name} · {task.status}
                  </Text>
                  <Text style={styles.citationMeta}>
                    {task.role} · attempts {task.attempts}
                  </Text>
                  {task.error_message ? (
                    <Text style={styles.errorText}>{task.error_message}</Text>
                  ) : null}
                </View>
              ))}
              <Text style={styles.debugHeader}>History</Text>
              {(activeWorkflow?.history ?? []).map((event) => (
                <View key={event.id} style={styles.modalSection}>
                  <Text style={styles.citationTitle}>{event.event_type}</Text>
                  <Text style={styles.citationMeta}>
                    {event.ref_type || "workflow"} ·{" "}
                    {new Date(event.created_at).toLocaleString()}
                  </Text>
                </View>
              ))}
              <Text style={styles.debugHeader}>Signals & Timers</Text>
              {(activeWorkflow?.signals ?? []).map((signal) => (
                <View key={`signal-${signal.id}`} style={styles.modalSection}>
                  <Text style={styles.citationTitle}>
                    {signal.signal_name} · {signal.status}
                  </Text>
                </View>
              ))}
              {(activeWorkflow?.timers ?? []).map((timer) => (
                <View key={`timer-${timer.id}`} style={styles.modalSection}>
                  <Text style={styles.citationTitle}>
                    {timer.timer_name} · {timer.status}
                  </Text>
                  <Text style={styles.citationMeta}>
                    due {new Date(timer.fire_at).toLocaleString()}
                  </Text>
                </View>
              ))}
              <Text style={styles.debugHeader}>Agent Messages</Text>
              {(activeWorkflow?.messages ?? []).map((message) => (
                <View key={message.id} style={styles.modalSection}>
                  <Text style={styles.citationTitle}>
                    {message.from_role} → {message.to_role}
                  </Text>
                  <Text style={styles.citationMeta}>
                    {message.message_type}
                  </Text>
                  <Text style={styles.citationSnippet}>
                    {message.content_json}
                  </Text>
                </View>
              ))}
            </ScrollView>
            <View style={styles.buttonRow}>
              <PrimaryButton
                title="Close"
                onPress={() => setWorkflowDebugVisible(false)}
                style={styles.button}
              />
              <PrimaryButton
                title="Process"
                onPress={() => void handleProcessCurrentWorkflow()}
                disabled={!activeWorkflow || workflowLoading || !token}
                style={styles.buttonSecondary}
              />
            </View>
          </View>
        </View>
      </Modal>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 16,
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
    marginBottom: 12,
  },
  summaryCard: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    borderWidth: 1,
    borderColor: "#e2e8f0",
  },
  summaryText: {
    color: "#334155",
    marginTop: 4,
  },
  inlineButton: {
    marginTop: 12,
  },
  inlineButtonSecondary: {
    marginTop: 12,
    backgroundColor: "#334155",
  },
  buttonRow: {
    flexDirection: "row",
    gap: 12,
    marginTop: 12,
  },
  button: {
    flex: 1,
  },
  buttonSecondary: {
    flex: 1,
    backgroundColor: "#475569",
  },
  sectionTitle: {
    marginTop: 16,
    marginBottom: 8,
    color: "#0f172a",
    fontWeight: "700",
  },
  optionRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  option: {
    backgroundColor: "#64748b",
  },
  optionActive: {
    backgroundColor: "#0f172a",
  },
  infoCard: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginTop: 14,
    borderWidth: 1,
    borderColor: "#e2e8f0",
  },
  infoTitle: {
    fontWeight: "700",
    color: "#0f172a",
  },
  agentHeader: {
    flexDirection: "row",
    alignItems: "flex-start",
    justifyContent: "space-between",
    gap: 12,
  },
  agentStatusBadge: {
    backgroundColor: "#0f172a",
    borderRadius: 999,
    paddingHorizontal: 12,
    paddingVertical: 6,
  },
  agentStatusText: {
    color: "#fff",
    fontWeight: "700",
    fontSize: 12,
  },
  agentContextGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 12,
  },
  agentContextItem: {
    color: "#334155",
    backgroundColor: "#f1f5f9",
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 6,
    fontSize: 12,
    overflow: "hidden",
  },
  memoryChipRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 12,
  },
  memoryChip: {
    color: "#1e293b",
    backgroundColor: "#e0f2fe",
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 6,
    fontSize: 12,
    overflow: "hidden",
  },
  agentResultBox: {
    marginTop: 12,
    paddingTop: 12,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  infoBody: {
    color: "#334155",
    marginTop: 8,
  },
  infoMeta: {
    color: "#64748b",
    marginTop: 8,
  },
  errorText: {
    color: "#b91c1c",
    marginTop: 8,
  },
  createNoteButton: {
    marginBottom: 12,
  },
  recordingButton: {
    marginTop: 12,
    backgroundColor: "#0f172a",
  },
  systemAction: {
    marginTop: 8,
    paddingVertical: 9,
    backgroundColor: "#0f766e",
  },
  recordingLinkButton: {
    marginTop: 12,
    backgroundColor: "#334155",
  },
  recordingFileRow: {
    marginTop: 12,
    paddingTop: 12,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  recordingFileTitle: {
    color: "#0f172a",
    fontWeight: "600",
  },
  contactList: {
    gap: 8,
  },
  contactButton: {
    backgroundColor: "#1d4ed8",
  },
  listContent: {
    paddingBottom: 24,
  },
  messageBubble: {
    borderRadius: 16,
    padding: 14,
    marginBottom: 12,
    maxWidth: "92%",
  },
  mine: {
    backgroundColor: "#dbeafe",
    alignSelf: "flex-end",
  },
  theirs: {
    backgroundColor: "#fff",
    borderWidth: 1,
    borderColor: "#e2e8f0",
    alignSelf: "flex-start",
  },
  systemBubble: {
    backgroundColor: "#ede9fe",
    alignSelf: "stretch",
  },
  sender: {
    fontWeight: "600",
    color: "#1e293b",
    marginBottom: 6,
  },
  body: {
    color: "#0f172a",
  },
  systemMeta: {
    color: "#6d28d9",
    fontSize: 12,
    marginTop: 8,
  },
  time: {
    color: "#64748b",
    fontSize: 12,
    marginTop: 8,
  },
  noteRow: {
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
    paddingTop: 10,
    marginTop: 10,
  },
  noteBody: {
    color: "#334155",
  },
  citationList: {
    marginTop: 12,
    gap: 10,
  },
  citationItem: {
    paddingTop: 10,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  citationHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    gap: 8,
  },
  citationBadge: {
    color: "#075985",
    backgroundColor: "#e0f2fe",
    borderRadius: 999,
    paddingHorizontal: 8,
    paddingVertical: 3,
    fontSize: 11,
    fontWeight: "700",
    overflow: "hidden",
  },
  approvalItem: {
    paddingTop: 10,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  inlineActionRow: {
    flexDirection: "row",
    gap: 10,
    marginTop: 10,
  },
  approveChip: {
    backgroundColor: "#166534",
    borderRadius: 999,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  approveChipText: {
    color: "#fff",
    fontWeight: "600",
  },
  rejectChip: {
    backgroundColor: "#991b1b",
    borderRadius: 999,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  rejectChipText: {
    color: "#fff",
    fontWeight: "600",
  },
  citationTitle: {
    color: "#0f172a",
    fontWeight: "600",
  },
  citationMeta: {
    color: "#475569",
    marginTop: 4,
    fontSize: 12,
  },
  citationSnippet: {
    color: "#334155",
    marginTop: 6,
  },
  modalBackdrop: {
    flex: 1,
    backgroundColor: "rgba(15, 23, 42, 0.45)",
    justifyContent: "center",
    padding: 18,
  },
  modalCard: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18,
    maxHeight: "80%",
  },
  debugDrawer: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18,
    maxHeight: "88%",
  },
  modalTitle: {
    color: "#0f172a",
    fontWeight: "700",
    fontSize: 18,
  },
  modalScroll: {
    marginTop: 12,
  },
  modalSection: {
    paddingTop: 12,
    marginTop: 12,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  modalButton: {
    marginTop: 16,
  },
  linkRow: {
    marginTop: 12,
  },
  linkText: {
    color: "#2563eb",
    fontWeight: "600",
  },
  debugHeader: {
    marginTop: 16,
    color: "#0f172a",
    fontWeight: "700",
  },
  composer: {
    marginTop: 12,
  },
});

export default ConversationDetailScreen;
