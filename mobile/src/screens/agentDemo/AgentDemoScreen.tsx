import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Linking,
  Text,
  View,
  useWindowDimensions,
} from "react-native";

import {
  createAgentRun,
  createWorkflowRun,
  fetchAgentRun,
  fetchAgentRunEvents,
  listToolApprovals,
  listWorkflowRuns,
  processWorkflowRun,
  submitToolApprovalDecision,
  type AgentCitation,
  type AgentRunEventRecord,
  type AgentRunResult,
  type ToolApprovalRecord,
  type WorkflowResult,
} from "../../api/agent";
import {
  createConversation,
  createConversationNote,
  createMessage,
  listConversationNotes,
  listConversations,
  listMessages,
  type ConversationNoteRecord,
  type ConversationRecord,
  type MessageRecord,
} from "../../api/collaboration";
import {
  createFileKnowledgeSource,
  createManualKnowledgeSource,
  createURLKnowledgeSource,
  decideKnowledgeDuplicateCandidate,
  fetchKnowledgeSource,
  listKnowledgeDeadLetters,
  listKnowledgeDuplicateCandidates,
  listKnowledgeSourceGroups,
  listKnowledgeSources,
  reingestKnowledgeSource,
  retryKnowledgeDeadLetter,
  setKnowledgeSourceGroupCanonical,
  type DeadLetterRecord,
  type DuplicateCandidateRecord,
  type KnowledgeSourceDetail,
  type KnowledgeSourceRecord,
  type SourceGroupRecord,
} from "../../api/knowledge";
import PrimaryButton from "../../components/PrimaryButton";
import { useAuthContext } from "../../context/AuthContext";
import { useOrganization } from "../../context/OrganizationContext";
import {
  defaultGoal,
  tabs,
  taskOrder,
  terminalRunStatuses,
  type ApprovalFilter,
  type LabTab,
  type WorkflowPreset,
} from "../agentDemoUtils";
import { AgentLabSidebar } from "./Sidebar";
import { ApprovalsTab } from "./ApprovalsTab";
import { EvalTab } from "./EvalTab";
import { GraphTab } from "./GraphTab";
import { KnowledgeTab } from "./KnowledgeTab";
import { RunTab } from "./RunTab";
import { styles } from "./styles";
import type { AgentLabController, Props } from "./types";

const AgentDemoScreen: React.FC<Props> = ({ navigation, route }) => {
  const { token } = useAuthContext();
  const { currentOrganization, loading: organizationLoading } =
    useOrganization();
  const { width } = useWindowDimensions();
  const isWide = width >= 1180;
  const knowledgeOnly = route.name === "KnowledgeCenter";

  const [activeTab, setActiveTab] = useState<LabTab>("knowledge");
  const [conversations, setConversations] = useState<ConversationRecord[]>([]);
  const [selectedConversationId, setSelectedConversationId] = useState<
    number | null
  >(null);
  const [messages, setMessages] = useState<MessageRecord[]>([]);
  const [notes, setNotes] = useState<ConversationNoteRecord[]>([]);
  const [sources, setSources] = useState<KnowledgeSourceRecord[]>([]);
  const [sourceGroups, setSourceGroups] = useState<SourceGroupRecord[]>([]);
  const [duplicateCandidates, setDuplicateCandidates] = useState<
    DuplicateCandidateRecord[]
  >([]);
  const [sourceDetail, setSourceDetail] =
    useState<KnowledgeSourceDetail | null>(null);
  const [deadLetters, setDeadLetters] = useState<DeadLetterRecord[]>([]);
  const [workflows, setWorkflows] = useState<WorkflowResult[]>([]);
  const [activeRun, setActiveRun] = useState<AgentRunResult | null>(null);
  const [runEvents, setRunEvents] = useState<AgentRunEventRecord[]>([]);
  const [activeWorkflow, setActiveWorkflow] = useState<WorkflowResult | null>(
    null,
  );
  const [approvals, setApprovals] = useState<ToolApprovalRecord[]>([]);
  const [approvalFilter, setApprovalFilter] =
    useState<ApprovalFilter>("pending");
  const [workflowPreset, setWorkflowPreset] =
    useState<WorkflowPreset>("meeting_brief");
  const [goal, setGoal] = useState(defaultGoal);
  const [manualTitle, setManualTitle] = useState("Demo knowledge note");
  const [manualText, setManualText] = useState(
    "客户重点关注响应时延、翻译质量、数据留存和月底预算窗口。",
  );
  const [urlTitle, setURLTitle] = useState("Reference URL");
  const [urlValue, setURLValue] = useState("");
  const [fileTitle, setFileTitle] = useState("Uploaded knowledge file");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const selectedConversation = useMemo(
    () =>
      conversations.find((item) => item.id === selectedConversationId) ?? null,
    [conversations, selectedConversationId],
  );

  const orderedTasks = useMemo(
    () =>
      [...(activeWorkflow?.tasks ?? [])].sort(
        (a, b) => taskOrder(a) - taskOrder(b),
      ),
    [activeWorkflow?.tasks],
  );
  const pendingWorkflowApprovals = useMemo(
    () =>
      (activeWorkflow?.approvals ?? []).filter(
        (approval) => approval.status === "pending",
      ),
    [activeWorkflow?.approvals],
  );
  const visibleApprovals = useMemo(
    () =>
      approvalFilter === "pending"
        ? approvals.filter((approval) => approval.status === "pending")
        : approvals,
    [approvalFilter, approvals],
  );
  const visibleTabs = useMemo(
    () =>
      knowledgeOnly ? tabs.filter((item) => item.key === "knowledge") : tabs,
    [knowledgeOnly],
  );
  const sourceTitleById = useMemo(() => {
    const index = new Map<number, string>();
    sources.forEach((source) => {
      index.set(source.id, source.title);
    });
    return index;
  }, [sources]);
  const selectedChunkStats = useMemo(() => {
    const stats = { indexed: 0, failed: 0, skipped: 0, pending: 0 };
    sourceDetail?.chunks.forEach((chunk) => {
      const key = chunk.index_status as keyof typeof stats;
      if (key in stats) {
        stats[key] += 1;
      }
    });
    return stats;
  }, [sourceDetail?.chunks]);

  useEffect(() => {
    if (knowledgeOnly) {
      setActiveTab("knowledge");
    }
  }, [knowledgeOnly]);

  const refreshKnowledge = useCallback(async () => {
    if (!token || !currentOrganization) return;
    const [nextSources, nextDeadLetters, nextGroups, nextDuplicates] =
      await Promise.all([
        listKnowledgeSources(token),
        listKnowledgeDeadLetters(token),
        listKnowledgeSourceGroups(token),
        listKnowledgeDuplicateCandidates(token),
      ]);
    setSources(nextSources);
    setDeadLetters(nextDeadLetters);
    setSourceGroups(nextGroups);
    setDuplicateCandidates(nextDuplicates);
    if (
      sourceDetail &&
      nextSources.every((item) => item.id !== sourceDetail.source.id)
    ) {
      setSourceDetail(null);
    }
  }, [currentOrganization, sourceDetail, token]);

  const refreshWorkflows = useCallback(async () => {
    if (!token || !currentOrganization) return;
    const [nextWorkflows, nextApprovals] = await Promise.all([
      listWorkflowRuns(token, 20),
      listToolApprovals(token),
    ]);
    setWorkflows(nextWorkflows);
    setApprovals(nextApprovals);
    setActiveWorkflow((current) => {
      if (!current) return nextWorkflows[0] ?? null;
      return (
        nextWorkflows.find(
          (item) => item.workflow.id === current.workflow.id,
        ) ?? current
      );
    });
  }, [currentOrganization, token]);

  const refreshActiveRun = useCallback(
    async (runId: number) => {
      if (!token) return;
      const [nextRun, nextEvents] = await Promise.all([
        fetchAgentRun(token, runId),
        fetchAgentRunEvents(token, runId),
      ]);
      setActiveRun(nextRun);
      setRunEvents(nextEvents);
    },
    [token],
  );

  const refreshConversations = useCallback(async () => {
    if (!token || !currentOrganization) {
      setConversations([]);
      setSelectedConversationId(null);
      return;
    }
    const page = await listConversations(token, "open");
    const items = page.conversations;
    setConversations(items);
    setSelectedConversationId((current) => {
      if (current && items.some((item) => item.id === current)) return current;
      return items[0]?.id ?? null;
    });
  }, [currentOrganization, token]);

  const refreshSelectedContext = useCallback(async () => {
    if (!token || !selectedConversationId) {
      setMessages([]);
      setNotes([]);
      return;
    }
    const [nextMessages, nextNotes] = await Promise.all([
      listMessages(token, selectedConversationId),
      listConversationNotes(token, selectedConversationId),
    ]);
    setMessages(nextMessages.messages);
    setNotes(nextNotes);
  }, [selectedConversationId, token]);

  const refreshAll = useCallback(async () => {
    if (!token || !currentOrganization) return;
    try {
      setBusy(true);
      await Promise.all([
        refreshConversations(),
        refreshKnowledge(),
        refreshWorkflows(),
      ]);
    } catch (error) {
      console.error("[AgentLab] Refresh failed:", error);
      Alert.alert("刷新失败", "无法加载 Agent Lab 数据。");
    } finally {
      setBusy(false);
    }
  }, [
    currentOrganization,
    refreshConversations,
    refreshKnowledge,
    refreshWorkflows,
    token,
  ]);

  useEffect(() => {
    void refreshAll();
  }, [refreshAll]);

  useEffect(() => {
    void refreshSelectedContext().catch((error) => {
      console.error("[AgentLab] Context refresh failed:", error);
    });
  }, [refreshSelectedContext]);

  useEffect(() => {
    if (!token || !activeRun) return;
    if (terminalRunStatuses.has(activeRun.run.status)) return;
    const timer = setInterval(() => {
      void refreshActiveRun(activeRun.run.id).catch((error) => {
        console.error("[AgentLab] Agent run refresh failed:", error);
      });
    }, 2500);
    return () => clearInterval(timer);
  }, [activeRun, refreshActiveRun, token]);

  const selectSource = useCallback(
    async (sourceId: number) => {
      if (!token) return;
      try {
        const next = await fetchKnowledgeSource(token, sourceId);
        setSourceDetail(next);
        setActiveTab("knowledge");
      } catch (error) {
        console.error("[AgentLab] Source load failed:", error);
        Alert.alert("加载失败", "无法打开知识源。");
      }
    },
    [token],
  );

  const handleCreateDemoThread = useCallback(async () => {
    if (!token) return;
    try {
      setBusy(true);
      const conversation = await createConversation(token, {
        type: "channel",
        title: `Agent Lab ${new Date().toLocaleTimeString()}`,
        topic: "Web Agent Lab demo thread",
      });
      await createMessage(token, conversation.id, {
        body: "客户希望本周确认跨境客服试点方案，重点关注响应时延、翻译质量和后续培训安排。",
      });
      await createMessage(token, conversation.id, {
        body: "销售侧已承诺先交付 20 个坐席的试点报价，但客户还在等待安全与数据留存说明。",
      });
      await createConversationNote(
        token,
        conversation.id,
        "内部备注：客户预算窗口在月底关闭，风险是法务审批可能拖慢签约。",
      );
      await createConversationNote(
        token,
        conversation.id,
        "内部备注：下一步建议准备一页式安全说明，并约一次技术答疑。",
      );
      setSelectedConversationId(conversation.id);
      await refreshConversations();
      setNotice("Demo conversation created");
    } catch (error) {
      console.error("[AgentLab] Demo thread failed:", error);
      Alert.alert("创建失败", "无法创建演示会话。");
    } finally {
      setBusy(false);
    }
  }, [refreshConversations, token]);

  const handleCreateManualSource = useCallback(async () => {
    if (!token || !manualText.trim()) return;
    try {
      setBusy(true);
      const source = await createManualKnowledgeSource(token, {
        title: manualTitle.trim() || "Manual knowledge",
        text: manualText,
        conversation_id: selectedConversationId,
      });
      await refreshKnowledge();
      await selectSource(source.id);
      setNotice("Knowledge source queued");
    } catch (error) {
      console.error("[AgentLab] Manual source failed:", error);
      Alert.alert("创建失败", "无法创建文本知识源。");
    } finally {
      setBusy(false);
    }
  }, [
    manualText,
    manualTitle,
    refreshKnowledge,
    selectSource,
    selectedConversationId,
    token,
  ]);

  const handleCreateURLSource = useCallback(async () => {
    if (!token || !urlValue.trim()) return;
    try {
      setBusy(true);
      const source = await createURLKnowledgeSource(token, {
        title: urlTitle.trim() || urlValue,
        url: urlValue.trim(),
        conversation_id: selectedConversationId,
      });
      await refreshKnowledge();
      await selectSource(source.id);
      setNotice("URL ingestion queued");
    } catch (error) {
      console.error("[AgentLab] URL source failed:", error);
      Alert.alert("创建失败", "无法创建 URL 知识源。");
    } finally {
      setBusy(false);
    }
  }, [
    refreshKnowledge,
    selectSource,
    selectedConversationId,
    token,
    urlTitle,
    urlValue,
  ]);

  const handleFileSelected = useCallback(
    async (file: File) => {
      if (!token) return;
      try {
        setBusy(true);
        const source = await createFileKnowledgeSource(
          token,
          file,
          fileTitle.trim() || file.name,
          selectedConversationId,
        );
        await refreshKnowledge();
        await selectSource(source.id);
        setNotice("File ingestion queued");
      } catch (error) {
        console.error("[AgentLab] File source failed:", error);
        Alert.alert("上传失败", "无法上传知识文件。");
      } finally {
        setBusy(false);
      }
    },
    [fileTitle, refreshKnowledge, selectSource, selectedConversationId, token],
  );

  const handleStartReactRun = useCallback(async () => {
    if (!token || !selectedConversationId) return;
    try {
      setBusy(true);
      const created = await createAgentRun(token, {
        conversation_id: selectedConversationId,
        goal: goal.trim() || defaultGoal,
      });
      setActiveRun(created);
      setRunEvents([]);
      await refreshActiveRun(created.run.id);
      setActiveTab("run");
      setNotice("ReAct run started");
    } catch (error) {
      console.error("[AgentLab] ReAct run failed:", error);
      Alert.alert("启动失败", "无法启动 ReAct Agent。");
    } finally {
      setBusy(false);
    }
  }, [goal, refreshActiveRun, selectedConversationId, token]);

  const handleStartWorkflow = useCallback(async () => {
    if (!token || !selectedConversationId) return;
    try {
      setBusy(true);
      const created = await createWorkflowRun(token, {
        conversation_id: selectedConversationId,
        goal: goal.trim() || defaultGoal,
        preset: workflowPreset,
      });
      const processed = await processWorkflowRun(token, created.workflow.id);
      setActiveWorkflow(processed);
      await refreshWorkflows();
      setActiveTab("graph");
      setNotice("Workflow started");
    } catch (error) {
      console.error("[AgentLab] Workflow start failed:", error);
      Alert.alert("启动失败", "无法启动 Workflow Agent。");
    } finally {
      setBusy(false);
    }
  }, [goal, refreshWorkflows, selectedConversationId, token, workflowPreset]);

  const handleProcessWorkflow = useCallback(async () => {
    if (!token || !activeWorkflow) return;
    try {
      setBusy(true);
      const processed = await processWorkflowRun(
        token,
        activeWorkflow.workflow.id,
      );
      setActiveWorkflow(processed);
      await refreshWorkflows();
    } catch (error) {
      console.error("[AgentLab] Workflow process failed:", error);
      Alert.alert("处理失败", "无法推进 Workflow。");
    } finally {
      setBusy(false);
    }
  }, [activeWorkflow, refreshWorkflows, token]);

  const handleApproval = useCallback(
    async (approval: ToolApprovalRecord, decision: "approve" | "reject") => {
      if (!token) return;
      try {
        setBusy(true);
        const updated = await submitToolApprovalDecision(
          token,
          approval.id,
          decision,
        );
        const processed = await processWorkflowRun(token, updated.workflow.id);
        setActiveWorkflow(processed);
        await refreshWorkflows();
        setNotice(`Tool ${decision}d`);
      } catch (error) {
        console.error("[AgentLab] Approval failed:", error);
        Alert.alert("审批失败", "无法提交工具审批。");
      } finally {
        setBusy(false);
      }
    },
    [refreshWorkflows, token],
  );

  const handleCitationPress = useCallback(
    async (citation: AgentCitation) => {
      if (citation.knowledge_source_id) {
        await selectSource(citation.knowledge_source_id);
        return;
      }
      if (citation.origin_url) {
        await Linking.openURL(citation.origin_url);
        return;
      }
      if (citation.conversation_id) {
        setSelectedConversationId(citation.conversation_id);
        setActiveTab("run");
      }
    },
    [selectSource],
  );

  const handleDuplicateDecision = useCallback(
    async (duplicateId: number, decision: "confirm" | "reject") => {
      if (!token) return;
      try {
        setBusy(true);
        await decideKnowledgeDuplicateCandidate(token, duplicateId, decision);
        await refreshKnowledge();
        setNotice(`Duplicate ${decision}ed`);
      } catch (error) {
        console.error("[AgentLab] Duplicate decision failed:", error);
        Alert.alert("处理失败", "无法提交重复源决策。");
      } finally {
        setBusy(false);
      }
    },
    [refreshKnowledge, token],
  );

  const handleSetCanonical = useCallback(
    async (groupId: number, sourceId: number) => {
      if (!token) return;
      try {
        setBusy(true);
        await setKnowledgeSourceGroupCanonical(token, groupId, sourceId);
        await refreshKnowledge();
        setNotice("Canonical source updated");
      } catch (error) {
        console.error("[AgentLab] Canonical source update failed:", error);
        Alert.alert("更新失败", "无法更新 canonical source。");
      } finally {
        setBusy(false);
      }
    },
    [refreshKnowledge, token],
  );

  const controller: AgentLabController = {
    currentOrganization,
    knowledgeOnly,
    visibleTabs,
    busy,
    refreshAll,
    navigation,
    notice,
    conversations,
    selectedConversationId,
    setSelectedConversationId,
    isWide,
    activeTab,
    setActiveTab,
    manualTitle,
    setManualTitle,
    manualText,
    setManualText,
    urlTitle,
    setURLTitle,
    urlValue,
    setURLValue,
    fileTitle,
    setFileTitle,
    handleCreateDemoThread,
    handleCreateManualSource,
    handleCreateURLSource,
    handleFileSelected,
    selectSource,
    sources,
    sourceDetail,
    selectedChunkStats,
    reingestKnowledgeSource,
    token,
    refreshKnowledge,
    sourceGroups,
    handleSetCanonical,
    sourceTitleById,
    duplicateCandidates,
    handleDuplicateDecision,
    deadLetters,
    retryKnowledgeDeadLetter,
    selectedConversation,
    messages,
    notes,
    handleStartReactRun,
    handleStartWorkflow,
    goal,
    setGoal,
    workflowPreset,
    setWorkflowPreset,
    activeRun,
    runEvents,
    refreshActiveRun,
    handleCitationPress,
    activeWorkflow,
    pendingWorkflowApprovals,
    handleProcessWorkflow,
    orderedTasks,
    approvals,
    approvalFilter,
    setApprovalFilter,
    visibleApprovals,
    handleApproval,
    workflows,
    setActiveWorkflow,
  };

  if (organizationLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0f766e" />
      </View>
    );
  }

  if (!currentOrganization) {
    return (
      <View style={styles.centered}>
        <Text style={styles.emptyTitle}>No workspace</Text>
        <PrimaryButton
          title="Organizations"
          onPress={() => navigation.navigate("Organizations")}
          style={styles.centerButton}
        />
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <View style={isWide ? styles.desktopLayout : styles.mobileLayout}>
        <AgentLabSidebar controller={controller} />
        {activeTab === "knowledge" ? (
          <KnowledgeTab controller={controller} />
        ) : null}
        {!knowledgeOnly && activeTab === "run" ? (
          <RunTab controller={controller} />
        ) : null}
        {!knowledgeOnly && activeTab === "graph" ? (
          <GraphTab controller={controller} />
        ) : null}
        {!knowledgeOnly && activeTab === "approvals" ? (
          <ApprovalsTab controller={controller} />
        ) : null}
        {!knowledgeOnly && activeTab === "eval" ? (
          <EvalTab controller={controller} />
        ) : null}
      </View>
    </View>
  );
};

export default AgentDemoScreen;
