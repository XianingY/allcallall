import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  Linking,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
  useWindowDimensions,
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import {
  createWorkflowRun,
  fetchWorkflowRun,
  listToolApprovals,
  listWorkflowRuns,
  processWorkflowRun,
  submitToolApprovalDecision,
  type AgentCitation,
  type ToolApprovalRecord,
  type WorkflowResult,
  type WorkflowTaskRecord,
} from "../api/agent";
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
} from "../api/knowledge";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";

type Props = NativeStackScreenProps<RootStackParamList, "AgentDemo">;
type LabTab = "knowledge" | "run" | "graph" | "approvals" | "eval";

const tabs: Array<{ key: LabTab; label: string }> = [
  { key: "knowledge", label: "Knowledge" },
  { key: "run", label: "Run" },
  { key: "graph", label: "Graph" },
  { key: "approvals", label: "Approvals" },
  { key: "eval", label: "Eval" },
];

const defaultGoal = "请基于会话消息、内部备注和知识库，给出客户当前诉求、风险点、下一步建议，并列出依据。";

const statusTone = (status: string) => {
  switch (status) {
    case "ready":
    case "executed":
    case "indexed":
    case "active":
      return styles.statusReady;
    case "running":
    case "pending":
    case "requires_action":
      return styles.statusRunning;
    case "failed":
    case "rejected":
      return styles.statusFailed;
    default:
      return styles.statusNeutral;
  }
};

const parseJSON = (raw?: string): unknown => {
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
};

const compact = (value: string, max = 180) => {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= max) return normalized;
  return `${normalized.slice(0, Math.max(0, max - 3))}...`;
};

const formatTime = (value?: string | null) => {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleTimeString();
};

const taskOrder = (task: WorkflowTaskRecord) => {
  const index = [
    "collect_context",
    "decompose",
    "searcher",
    "summarizer",
    "risk_analyst",
    "merge",
    "propose_tools",
    "approval",
    "commit_result",
  ].indexOf(task.name);
  return index >= 0 ? index : 99;
};

const AgentDemoScreen: React.FC<Props> = ({ navigation }) => {
  const { token } = useAuthContext();
  const { currentOrganization, loading: organizationLoading } = useOrganization();
  const { width } = useWindowDimensions();
  const isWide = width >= 1180;

  const [activeTab, setActiveTab] = useState<LabTab>("knowledge");
  const [conversations, setConversations] = useState<ConversationRecord[]>([]);
  const [selectedConversationId, setSelectedConversationId] = useState<number | null>(null);
  const [detail, setDetail] = useState<ConversationDetailRecord | null>(null);
  const [messages, setMessages] = useState<MessageRecord[]>([]);
  const [notes, setNotes] = useState<ConversationNoteRecord[]>([]);
  const [sources, setSources] = useState<KnowledgeSourceRecord[]>([]);
  const [sourceGroups, setSourceGroups] = useState<SourceGroupRecord[]>([]);
  const [duplicateCandidates, setDuplicateCandidates] = useState<DuplicateCandidateRecord[]>([]);
  const [sourceDetail, setSourceDetail] = useState<KnowledgeSourceDetail | null>(null);
  const [deadLetters, setDeadLetters] = useState<DeadLetterRecord[]>([]);
  const [workflows, setWorkflows] = useState<WorkflowResult[]>([]);
  const [activeWorkflow, setActiveWorkflow] = useState<WorkflowResult | null>(null);
  const [approvals, setApprovals] = useState<ToolApprovalRecord[]>([]);
  const [goal, setGoal] = useState(defaultGoal);
  const [manualTitle, setManualTitle] = useState("Demo knowledge note");
  const [manualText, setManualText] = useState("客户重点关注响应时延、翻译质量、数据留存和月底预算窗口。");
  const [urlTitle, setURLTitle] = useState("Reference URL");
  const [urlValue, setURLValue] = useState("");
  const [fileTitle, setFileTitle] = useState("Uploaded knowledge file");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const selectedConversation = useMemo(
    () => conversations.find((item) => item.id === selectedConversationId) ?? null,
    [conversations, selectedConversationId]
  );

  const orderedTasks = useMemo(
    () => [...(activeWorkflow?.tasks ?? [])].sort((a, b) => taskOrder(a) - taskOrder(b)),
    [activeWorkflow?.tasks]
  );

  const refreshKnowledge = useCallback(async () => {
    if (!token || !currentOrganization) return;
    const [nextSources, nextDeadLetters, nextGroups, nextDuplicates] = await Promise.all([
      listKnowledgeSources(token),
      listKnowledgeDeadLetters(token),
      listKnowledgeSourceGroups(token),
      listKnowledgeDuplicateCandidates(token),
    ]);
    setSources(nextSources);
    setDeadLetters(nextDeadLetters);
    setSourceGroups(nextGroups);
    setDuplicateCandidates(nextDuplicates);
    if (sourceDetail && nextSources.every((item) => item.id !== sourceDetail.source.id)) {
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
      return nextWorkflows.find((item) => item.workflow.id === current.workflow.id) ?? current;
    });
  }, [currentOrganization, token]);

  const refreshConversations = useCallback(async () => {
    if (!token || !currentOrganization) {
      setConversations([]);
      setSelectedConversationId(null);
      return;
    }
    const items = await listConversations(token, "open");
    setConversations(items);
    setSelectedConversationId((current) => {
      if (current && items.some((item) => item.id === current)) return current;
      return items[0]?.id ?? null;
    });
  }, [currentOrganization, token]);

  const refreshSelectedContext = useCallback(async () => {
    if (!token || !selectedConversationId) {
      setDetail(null);
      setMessages([]);
      setNotes([]);
      return;
    }
    const [nextDetail, nextMessages, nextNotes] = await Promise.all([
      fetchConversationDetail(token, selectedConversationId),
      listMessages(token, selectedConversationId),
      listConversationNotes(token, selectedConversationId),
    ]);
    setDetail(nextDetail);
    setMessages(nextMessages);
    setNotes(nextNotes);
  }, [selectedConversationId, token]);

  const refreshAll = useCallback(async () => {
    if (!token || !currentOrganization) return;
    try {
      setBusy(true);
      await Promise.all([refreshConversations(), refreshKnowledge(), refreshWorkflows()]);
    } catch (error) {
      console.error("[AgentLab] Refresh failed:", error);
      Alert.alert("刷新失败", "无法加载 Agent Lab 数据。");
    } finally {
      setBusy(false);
    }
  }, [currentOrganization, refreshConversations, refreshKnowledge, refreshWorkflows, token]);

  useEffect(() => {
    void refreshAll();
  }, [refreshAll]);

  useEffect(() => {
    void refreshSelectedContext().catch((error) => {
      console.error("[AgentLab] Context refresh failed:", error);
    });
  }, [refreshSelectedContext]);

  const selectSource = useCallback(async (sourceId: number) => {
    if (!token) return;
    try {
      const next = await fetchKnowledgeSource(token, sourceId);
      setSourceDetail(next);
      setActiveTab("knowledge");
    } catch (error) {
      console.error("[AgentLab] Source load failed:", error);
      Alert.alert("加载失败", "无法打开知识源。");
    }
  }, [token]);

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
      await createConversationNote(token, conversation.id, "内部备注：客户预算窗口在月底关闭，风险是法务审批可能拖慢签约。");
      await createConversationNote(token, conversation.id, "内部备注：下一步建议准备一页式安全说明，并约一次技术答疑。");
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
  }, [manualText, manualTitle, refreshKnowledge, selectSource, selectedConversationId, token]);

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
  }, [refreshKnowledge, selectSource, selectedConversationId, token, urlTitle, urlValue]);

  const handleFileSelected = useCallback(async (file: File) => {
    if (!token) return;
    try {
      setBusy(true);
      const source = await createFileKnowledgeSource(token, file, fileTitle.trim() || file.name, selectedConversationId);
      await refreshKnowledge();
      await selectSource(source.id);
      setNotice("File ingestion queued");
    } catch (error) {
      console.error("[AgentLab] File source failed:", error);
      Alert.alert("上传失败", "无法上传知识文件。");
    } finally {
      setBusy(false);
    }
  }, [fileTitle, refreshKnowledge, selectSource, selectedConversationId, token]);

  const handleStartWorkflow = useCallback(async () => {
    if (!token || !selectedConversationId) return;
    try {
      setBusy(true);
      const created = await createWorkflowRun(token, {
        conversation_id: selectedConversationId,
        goal: goal.trim() || defaultGoal,
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
  }, [goal, refreshWorkflows, selectedConversationId, token]);

  const handleProcessWorkflow = useCallback(async () => {
    if (!token || !activeWorkflow) return;
    try {
      setBusy(true);
      const processed = await processWorkflowRun(token, activeWorkflow.workflow.id);
      setActiveWorkflow(processed);
      await refreshWorkflows();
    } catch (error) {
      console.error("[AgentLab] Workflow process failed:", error);
      Alert.alert("处理失败", "无法推进 Workflow。");
    } finally {
      setBusy(false);
    }
  }, [activeWorkflow, refreshWorkflows, token]);

  const handleApproval = useCallback(async (approval: ToolApprovalRecord, decision: "approve" | "reject") => {
    if (!token) return;
    try {
      setBusy(true);
      const updated = await submitToolApprovalDecision(token, approval.id, decision);
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
  }, [refreshWorkflows, token]);

  const handleCitationPress = useCallback(async (citation: AgentCitation) => {
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
  }, [selectSource]);

  const handleDuplicateDecision = useCallback(async (duplicateId: number, decision: "confirm" | "reject") => {
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
  }, [refreshKnowledge, token]);

  const handleSetCanonical = useCallback(async (groupId: number, sourceId: number) => {
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
  }, [refreshKnowledge, token]);

  const renderSidebar = () => (
    <View style={isWide ? styles.sidebar : styles.panel}>
      <Text style={styles.eyebrow}>{currentOrganization?.name}</Text>
      <Text style={styles.heading}>Agent Lab</Text>
      <View style={styles.tabRow}>
        {tabs.map((tab) => (
          <Pressable
            key={tab.key}
            style={[styles.tabButton, activeTab === tab.key && styles.tabButtonActive]}
            onPress={() => setActiveTab(tab.key)}
          >
            <Text style={[styles.tabButtonText, activeTab === tab.key && styles.tabButtonTextActive]}>{tab.label}</Text>
          </Pressable>
        ))}
      </View>
      <View style={styles.actionRow}>
        <PrimaryButton title={busy ? "Working..." : "Refresh"} onPress={() => void refreshAll()} disabled={busy} style={styles.actionButton} />
        <PrimaryButton title="Demo" onPress={handleCreateDemoThread} disabled={busy} style={styles.secondaryButton} />
      </View>
      {notice ? <Text style={styles.notice}>{notice}</Text> : null}
      <ScrollView style={styles.threadList} contentContainerStyle={styles.threadListContent}>
        {conversations.map((conversation) => {
          const selected = conversation.id === selectedConversationId;
          return (
            <Pressable
              key={conversation.id}
              style={[styles.threadItem, selected && styles.threadItemActive]}
              onPress={() => setSelectedConversationId(conversation.id)}
            >
              <Text style={[styles.threadTitle, selected && styles.threadTitleActive]} numberOfLines={1}>
                {conversation.title || conversation.type}
              </Text>
              <Text style={[styles.threadMeta, selected && styles.threadMetaActive]}>
                {conversation.priority.toUpperCase()} · {conversation.status.toUpperCase()}
              </Text>
              <Text style={[styles.threadPreview, selected && styles.threadPreviewActive]} numberOfLines={2}>
                {conversation.last_message_preview || "No messages"}
              </Text>
            </Pressable>
          );
        })}
      </ScrollView>
    </View>
  );

  const renderKnowledgeTab = () => (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <View style={styles.grid}>
        <View style={[styles.panel, styles.formPanel]}>
          <Text style={styles.sectionTitle}>Add Text</Text>
          <TextField label="Title" value={manualTitle} onChangeText={setManualTitle} />
          <TextField label="Text" value={manualText} onChangeText={setManualText} multiline style={styles.textArea} />
          <PrimaryButton title="Add Text" onPress={handleCreateManualSource} disabled={busy || !manualText.trim()} style={styles.fullButton} />
        </View>
        <View style={[styles.panel, styles.formPanel]}>
          <Text style={styles.sectionTitle}>Add URL</Text>
          <TextField label="Title" value={urlTitle} onChangeText={setURLTitle} />
          <TextField label="URL" value={urlValue} onChangeText={setURLValue} autoCapitalize="none" />
          <PrimaryButton title="Add URL" onPress={handleCreateURLSource} disabled={busy || !urlValue.trim()} style={styles.fullButton} />
        </View>
        <View style={[styles.panel, styles.formPanel]}>
          <Text style={styles.sectionTitle}>Upload File</Text>
          <TextField label="Title" value={fileTitle} onChangeText={setFileTitle} />
          {Platform.OS === "web" ? (
            <WebFilePicker onFile={handleFileSelected} />
          ) : (
            <Text style={styles.emptyText}>File upload is available in the web build.</Text>
          )}
        </View>
      </View>

      <View style={styles.grid}>
        <View style={[styles.panel, styles.listPanel]}>
          <View style={styles.panelHeader}>
            <Text style={styles.sectionTitle}>Sources</Text>
            <Text style={styles.contextLine}>{sources.length}</Text>
          </View>
          {sources.map((source) => (
            <Pressable key={source.id} style={styles.rowItem} onPress={() => void selectSource(source.id)}>
              <View style={styles.rowTop}>
                <Text style={styles.rowTitle}>{source.title}</Text>
                <StatusPill status={source.status} />
              </View>
              <Text style={styles.rowMeta}>
                {source.kind} · group {source.source_group_id ?? "-"} · {source.dedupe_status || "unique"} · {formatTime(source.updated_at)}
              </Text>
              {source.last_error ? <Text style={styles.errorText}>{compact(source.last_error)}</Text> : null}
              <View style={styles.inlineActions}>
                <Pressable style={styles.inlineButton} onPress={() => void reingestKnowledgeSource(token ?? "", source.id).then(refreshKnowledge)}>
                  <Text style={styles.inlineButtonText}>Reingest</Text>
                </Pressable>
              </View>
            </Pressable>
          ))}
          {sources.length === 0 ? <Text style={styles.emptyText}>No sources yet.</Text> : null}
        </View>

        <View style={[styles.panel, styles.previewPanel]}>
          <Text style={styles.sectionTitle}>Source Preview</Text>
          {sourceDetail ? (
            <>
              <Text style={styles.previewTitle}>{sourceDetail.source.title}</Text>
              <Text style={styles.rowMeta}>
                {sourceDetail.source.kind} · {sourceDetail.versions[0]?.chunk_count ?? 0} chunks
              </Text>
              {sourceDetail.versions.map((version) => (
                <View key={version.id} style={styles.versionRow}>
                  <Text style={styles.rowTitle}>Version {version.version}</Text>
                  <StatusPill status={version.status} />
                </View>
              ))}
              {sourceDetail.chunks.map((chunk) => (
                <View key={chunk.id} style={styles.chunkBox}>
                  <View style={styles.rowTop}>
                    <Text style={styles.chunkTitle}>Chunk {chunk.chunk_index}</Text>
                    <StatusPill status={chunk.index_status} />
                  </View>
                  <Text style={styles.citationSnippet}>{chunk.snippet}</Text>
                </View>
              ))}
            </>
          ) : (
            <Text style={styles.emptyText}>Select a source.</Text>
          )}
        </View>
      </View>

      <View style={styles.grid}>
        <View style={[styles.panel, styles.listPanel]}>
          <View style={styles.panelHeader}>
            <Text style={styles.sectionTitle}>Source Groups</Text>
            <Text style={styles.contextLine}>{sourceGroups.length}</Text>
          </View>
          {sourceGroups.map((group) => (
            <View key={group.id} style={styles.rowItem}>
              <View style={styles.rowTop}>
                <Text style={styles.rowTitle}>{group.title}</Text>
                <StatusPill status={group.status} />
              </View>
              <Text style={styles.rowMeta}>
                canonical #{group.canonical_source_id ?? "-"} · authority {Math.round((group.authority_score ?? 0) * 100)}
              </Text>
              {sources
                .filter((source) => source.source_group_id === group.id)
                .map((source) => (
                  <View key={source.id} style={styles.inlineRow}>
                    <Text style={styles.rowMeta}>
                      #{source.id} {source.title}
                    </Text>
                    {group.canonical_source_id !== source.id ? (
                      <Pressable style={styles.inlineButton} onPress={() => void handleSetCanonical(group.id, source.id)}>
                        <Text style={styles.inlineButtonText}>Make canonical</Text>
                      </Pressable>
                    ) : null}
                  </View>
                ))}
            </View>
          ))}
          {sourceGroups.length === 0 ? <Text style={styles.emptyText}>No source groups.</Text> : null}
        </View>

        <View style={[styles.panel, styles.previewPanel]}>
          <View style={styles.panelHeader}>
            <Text style={styles.sectionTitle}>Duplicate Review</Text>
            <Text style={styles.contextLine}>{duplicateCandidates.length}</Text>
          </View>
          {duplicateCandidates.map((item) => (
            <View key={item.id} style={styles.rowItem}>
              <View style={styles.rowTop}>
                <Text style={styles.rowTitle}>
                  #{item.source_id} vs #{item.candidate_source_id}
                </Text>
                <StatusPill status={item.status} />
              </View>
              <Text style={styles.rowMeta}>
                {item.duplicate_kind} · similarity {Math.round(item.similarity * 100)} · group {item.source_group_id ?? "-"}
              </Text>
              {item.status === "pending" ? (
                <View style={styles.inlineActions}>
                  <Pressable style={styles.approveButton} onPress={() => void handleDuplicateDecision(item.id, "confirm")}>
                    <Text style={styles.approveButtonText}>Confirm</Text>
                  </Pressable>
                  <Pressable style={styles.rejectButton} onPress={() => void handleDuplicateDecision(item.id, "reject")}>
                    <Text style={styles.rejectButtonText}>Reject</Text>
                  </Pressable>
                </View>
              ) : null}
            </View>
          ))}
          {duplicateCandidates.length === 0 ? <Text style={styles.emptyText}>No duplicate candidates.</Text> : null}
        </View>
      </View>

      <View style={styles.panel}>
        <Text style={styles.sectionTitle}>Dead Letters</Text>
        {deadLetters.map((item) => (
          <View key={item.id} style={styles.rowItem}>
            <View style={styles.rowTop}>
              <Text style={styles.rowTitle}>{item.event}</Text>
              <StatusPill status={item.status} />
            </View>
            <Text style={styles.rowMeta}>attempts {item.attempts} · #{item.id}</Text>
            {item.last_error ? <Text style={styles.errorText}>{compact(item.last_error, 260)}</Text> : null}
            <Pressable style={styles.inlineButton} onPress={() => void retryKnowledgeDeadLetter(token ?? "", item.id).then(refreshKnowledge)}>
              <Text style={styles.inlineButtonText}>Retry</Text>
            </Pressable>
          </View>
        ))}
        {deadLetters.length === 0 ? <Text style={styles.emptyText}>No failed RAG events.</Text> : null}
      </View>
    </ScrollView>
  );

  const renderRunTab = () => (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <View style={styles.panel}>
        <View style={styles.panelHeader}>
          <View>
            <Text style={styles.sectionTitle}>{selectedConversation?.title || "Select a conversation"}</Text>
            <Text style={styles.contextLine}>Messages {messages.length} · Notes {notes.length}</Text>
          </View>
          <PrimaryButton title="Start Workflow" onPress={handleStartWorkflow} disabled={busy || !selectedConversationId} style={styles.startButton} />
        </View>
        <TextField label="Goal" value={goal} onChangeText={setGoal} multiline style={styles.goalInput} />
      </View>

      {activeWorkflow ? (
        <View style={styles.panel}>
          <View style={styles.panelHeader}>
            <View>
              <Text style={styles.sectionTitle}>Workflow #{activeWorkflow.workflow.id}</Text>
              <Text style={styles.contextLine}>{activeWorkflow.workflow.goal}</Text>
            </View>
            <StatusPill status={activeWorkflow.workflow.status} />
          </View>
          {activeWorkflow.workflow.summary ? <Text style={styles.answerText}>{activeWorkflow.workflow.summary}</Text> : null}
          {activeWorkflow.workflow.next_step ? <Text style={styles.nextStep}>Next step: {activeWorkflow.workflow.next_step}</Text> : null}
          <CitationList citations={activeWorkflow.citations} onPress={handleCitationPress} />
        </View>
      ) : null}

      <View style={styles.grid}>
        <View style={[styles.panel, styles.contextPanel]}>
          <Text style={styles.sectionTitle}>Messages</Text>
          {messages.slice(-6).map((message) => (
            <View key={message.id} style={styles.contextItem}>
              <Text style={styles.contextItemTitle}>{message.sender_display_name || message.sender_email || message.type}</Text>
              <Text style={styles.contextItemBody}>{message.body || message.type}</Text>
            </View>
          ))}
          {messages.length === 0 ? <Text style={styles.emptyText}>No messages.</Text> : null}
        </View>
        <View style={[styles.panel, styles.contextPanel]}>
          <Text style={styles.sectionTitle}>Notes</Text>
          {notes.slice(0, 6).map((note) => (
            <View key={note.id} style={styles.contextItem}>
              <Text style={styles.contextItemTitle}>{note.author_display_name || note.author_email}</Text>
              <Text style={styles.contextItemBody}>{note.body}</Text>
            </View>
          ))}
          {notes.length === 0 ? <Text style={styles.emptyText}>No notes.</Text> : null}
        </View>
      </View>
    </ScrollView>
  );

  const renderGraphTab = () => (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <View style={styles.panel}>
        <View style={styles.panelHeader}>
          <Text style={styles.sectionTitle}>Task Graph</Text>
          <PrimaryButton title="Process" onPress={handleProcessWorkflow} disabled={busy || !activeWorkflow} style={styles.processButton} />
        </View>
        {activeWorkflow ? (
          <Text style={styles.contextLine}>
            {activeWorkflow.workflow.workflow_version || "agent_lab_v1"} · {activeWorkflow.workflow.prompt_version || "-"} · {activeWorkflow.workflow.tool_schema_version || "-"}
          </Text>
        ) : null}
        {orderedTasks.map((task, index) => (
          <View key={task.id} style={styles.taskRow}>
            <View style={styles.taskIndex}>
              <Text style={styles.taskIndexText}>{index + 1}</Text>
            </View>
            <View style={styles.taskBody}>
              <View style={styles.rowTop}>
                <Text style={styles.rowTitle}>{task.name}</Text>
                <StatusPill status={task.status} />
              </View>
              <Text style={styles.rowMeta}>{task.role} · depends {compact(task.depends_on_json || "[]", 80)}</Text>
              {task.error_message ? <Text style={styles.errorText}>{task.error_message}</Text> : null}
            </View>
          </View>
        ))}
        {!activeWorkflow ? <Text style={styles.emptyText}>No workflow selected.</Text> : null}
      </View>

      <View style={styles.panel}>
        <Text style={styles.sectionTitle}>Agent Messages</Text>
        {(activeWorkflow?.messages ?? []).map((message) => (
          <View key={message.id} style={styles.messageBox}>
            <Text style={styles.rowTitle}>
              {message.from_role} → {message.to_role}
            </Text>
            <Text style={styles.rowMeta}>{message.message_type} · {message.correlation_id}</Text>
            <Text style={styles.messageBody}>{compact(JSON.stringify(parseJSON(message.content_json) ?? message.content_json), 460)}</Text>
          </View>
        ))}
        {activeWorkflow && activeWorkflow.messages.length === 0 ? <Text style={styles.emptyText}>No agent messages.</Text> : null}
      </View>

      <View style={styles.grid}>
        <View style={[styles.panel, styles.contextPanel]}>
          <Text style={styles.sectionTitle}>History</Text>
          {(activeWorkflow?.history ?? []).map((event) => (
            <View key={event.id} style={styles.messageBox}>
              <Text style={styles.rowTitle}>{event.event_type}</Text>
              <Text style={styles.rowMeta}>{event.ref_type || "workflow"} · {formatTime(event.created_at)}</Text>
              {event.attributes_json ? <Text style={styles.messageBody}>{compact(event.attributes_json, 260)}</Text> : null}
            </View>
          ))}
          {activeWorkflow && activeWorkflow.history.length === 0 ? <Text style={styles.emptyText}>No history events.</Text> : null}
        </View>
        <View style={[styles.panel, styles.contextPanel]}>
          <Text style={styles.sectionTitle}>Signals & Timers</Text>
          {(activeWorkflow?.signals ?? []).map((signal) => (
            <View key={`signal-${signal.id}`} style={styles.messageBox}>
              <Text style={styles.rowTitle}>{signal.signal_name}</Text>
              <Text style={styles.rowMeta}>{signal.status} · {formatTime(signal.created_at)}</Text>
            </View>
          ))}
          {(activeWorkflow?.timers ?? []).map((timer) => (
            <View key={`timer-${timer.id}`} style={styles.messageBox}>
              <Text style={styles.rowTitle}>{timer.timer_name}</Text>
              <Text style={styles.rowMeta}>{timer.status} · due {formatTime(timer.fire_at)}</Text>
            </View>
          ))}
          {activeWorkflow && activeWorkflow.signals.length === 0 && activeWorkflow.timers.length === 0 ? (
            <Text style={styles.emptyText}>No signals or timers.</Text>
          ) : null}
        </View>
      </View>
    </ScrollView>
  );

  const renderApprovalsTab = () => (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <View style={styles.panel}>
        <View style={styles.panelHeader}>
          <Text style={styles.sectionTitle}>Tool Approvals</Text>
          <Text style={styles.contextLine}>{approvals.length}</Text>
        </View>
        {approvals.map((approval) => (
          <View key={approval.id} style={styles.approvalBox}>
            <View style={styles.rowTop}>
              <Text style={styles.rowTitle}>{approval.tool_name}</Text>
              <StatusPill status={approval.status} />
            </View>
            <Text style={styles.rowMeta}>workflow #{approval.workflow_run_id} · requested by {approval.requested_by}</Text>
            <Text style={styles.messageBody}>{compact(JSON.stringify(parseJSON(approval.input_json) ?? approval.input_json), 360)}</Text>
            {approval.status === "pending" ? (
              <View style={styles.inlineActions}>
                <Pressable style={styles.approveButton} onPress={() => void handleApproval(approval, "approve")}>
                  <Text style={styles.approveButtonText}>Approve</Text>
                </Pressable>
                <Pressable style={styles.rejectButton} onPress={() => void handleApproval(approval, "reject")}>
                  <Text style={styles.rejectButtonText}>Reject</Text>
                </Pressable>
              </View>
            ) : null}
          </View>
        ))}
        {approvals.length === 0 ? <Text style={styles.emptyText}>No approvals.</Text> : null}
      </View>
    </ScrollView>
  );

  const renderEvalTab = () => (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <View style={styles.panel}>
        <Text style={styles.sectionTitle}>Eval</Text>
        <View style={styles.evalGrid}>
          <View style={styles.evalCard}>
            <Text style={styles.rowTitle}>RAG quality</Text>
            <StatusPill status="pending" />
          </View>
          <View style={styles.evalCard}>
            <Text style={styles.rowTitle}>Workflow planner</Text>
            <StatusPill status="pending" />
          </View>
        </View>
      </View>
      <View style={styles.panel}>
        <Text style={styles.sectionTitle}>Recent Workflows</Text>
        {workflows.map((item) => (
          <Pressable key={item.workflow.id} style={styles.rowItem} onPress={() => setActiveWorkflow(item)}>
            <View style={styles.rowTop}>
              <Text style={styles.rowTitle}>Workflow #{item.workflow.id}</Text>
              <StatusPill status={item.workflow.status} />
            </View>
            <Text style={styles.rowMeta}>{compact(item.workflow.goal, 120)}</Text>
          </Pressable>
        ))}
      </View>
    </ScrollView>
  );

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
        <PrimaryButton title="Organizations" onPress={() => navigation.navigate("Organizations")} style={styles.centerButton} />
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <View style={isWide ? styles.desktopLayout : styles.mobileLayout}>
        {renderSidebar()}
        {activeTab === "knowledge" ? renderKnowledgeTab() : null}
        {activeTab === "run" ? renderRunTab() : null}
        {activeTab === "graph" ? renderGraphTab() : null}
        {activeTab === "approvals" ? renderApprovalsTab() : null}
        {activeTab === "eval" ? renderEvalTab() : null}
      </View>
    </View>
  );
};

interface StatusPillProps {
  status: string;
}

const StatusPill: React.FC<StatusPillProps> = ({ status }) => (
  <View style={[styles.statusPill, statusTone(status)]}>
    <Text style={styles.statusText}>{status || "unknown"}</Text>
  </View>
);

interface CitationListProps {
  citations: AgentCitation[];
  onPress: (citation: AgentCitation) => void;
}

const CitationList: React.FC<CitationListProps> = ({ citations, onPress }) => {
  if (citations.length === 0) return null;
  return (
    <View style={styles.citationContainer}>
      <Text style={styles.citationsTitle}>Citations</Text>
      {citations.slice(0, 8).map((citation) => (
        <Pressable
          key={`${citation.source_type}:${citation.source_id}:${citation.chunk_id ?? ""}`}
          style={styles.citationItem}
          onPress={() => onPress(citation)}
        >
          <View style={styles.rowTop}>
            <Text style={styles.citationTitle}>{citation.source_title || citation.title}</Text>
            <Text style={styles.scoreText}>
              {citation.retrieval_mode || "source"} · {citation.score}
              {citation.rrf_score ? ` · rrf ${citation.rrf_score.toFixed(3)}` : ""}
            </Text>
          </View>
          <Text style={styles.citationSnippet}>{citation.snippet}</Text>
        </Pressable>
      ))}
    </View>
  );
};

interface WebFilePickerProps {
  onFile: (file: File) => void;
}

const WebFilePicker: React.FC<WebFilePickerProps> = ({ onFile }) => {
  if (Platform.OS !== "web") return null;
  return React.createElement("input", {
    type: "file",
    accept: ".txt,.md,.html,.pdf,text/plain,text/markdown,text/html,application/pdf",
    style: {
      border: "1px solid #cbd5e1",
      borderRadius: 8,
      padding: 12,
      width: "100%",
      boxSizing: "border-box",
      marginTop: 8,
      color: "#0f172a",
    },
    onChange: (event: React.ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0];
      if (file) onFile(file);
      event.target.value = "";
    },
  });
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#edf2f4",
    padding: 14,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "#edf2f4",
    padding: 24,
  },
  centerButton: {
    marginTop: 16,
    minWidth: 180,
  },
  desktopLayout: {
    flex: 1,
    flexDirection: "row",
    gap: 14,
  },
  mobileLayout: {
    flex: 1,
    gap: 12,
  },
  sidebar: {
    width: 350,
    backgroundColor: "#ffffff",
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "#cbd5e1",
    padding: 14,
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
    borderColor: "#cbd5e1",
    padding: 14,
  },
  panelHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
    marginBottom: 10,
  },
  eyebrow: {
    color: "#0f766e",
    fontSize: 12,
    fontWeight: "800",
    textTransform: "uppercase",
  },
  heading: {
    color: "#0f172a",
    fontSize: 26,
    fontWeight: "800",
    marginTop: 4,
  },
  tabRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 14,
    marginBottom: 12,
  },
  tabButton: {
    borderWidth: 1,
    borderColor: "#cbd5e1",
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 8,
    backgroundColor: "#f8fafc",
  },
  tabButtonActive: {
    backgroundColor: "#0f766e",
    borderColor: "#0f766e",
  },
  tabButtonText: {
    color: "#334155",
    fontSize: 13,
    fontWeight: "800",
  },
  tabButtonTextActive: {
    color: "#ffffff",
  },
  actionRow: {
    flexDirection: "row",
    gap: 8,
    marginBottom: 10,
  },
  actionButton: {
    flex: 1,
    borderRadius: 8,
    backgroundColor: "#0f766e",
  },
  secondaryButton: {
    flex: 1,
    borderRadius: 8,
    backgroundColor: "#334155",
  },
  startButton: {
    minWidth: 150,
    borderRadius: 8,
    backgroundColor: "#0f766e",
  },
  processButton: {
    minWidth: 120,
    borderRadius: 8,
    backgroundColor: "#334155",
  },
  fullButton: {
    borderRadius: 8,
    backgroundColor: "#0f766e",
  },
  notice: {
    color: "#0f766e",
    fontWeight: "700",
    marginBottom: 8,
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
    borderColor: "#cbd5e1",
    borderRadius: 8,
    padding: 12,
    backgroundColor: "#f8fafc",
  },
  threadItemActive: {
    backgroundColor: "#264653",
    borderColor: "#264653",
  },
  threadTitle: {
    color: "#0f172a",
    fontWeight: "800",
  },
  threadTitleActive: {
    color: "#ffffff",
  },
  threadMeta: {
    color: "#64748b",
    fontSize: 12,
    marginTop: 4,
    fontWeight: "800",
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
  grid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 12,
  },
  formPanel: {
    flex: 1,
    minWidth: 260,
  },
  listPanel: {
    flex: 0.9,
    minWidth: 320,
  },
  previewPanel: {
    flex: 1.2,
    minWidth: 360,
  },
  contextPanel: {
    flex: 1,
    minWidth: 300,
  },
  sectionTitle: {
    color: "#0f172a",
    fontSize: 18,
    fontWeight: "800",
  },
  contextLine: {
    color: "#64748b",
    marginTop: 4,
  },
  textArea: {
    minHeight: 116,
    textAlignVertical: "top",
    paddingTop: 12,
  },
  goalInput: {
    minHeight: 106,
    textAlignVertical: "top",
    paddingTop: 12,
  },
  rowItem: {
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
    paddingTop: 12,
    marginTop: 12,
  },
  rowTop: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 8,
  },
  rowTitle: {
    color: "#0f172a",
    fontWeight: "800",
    flexShrink: 1,
  },
  rowMeta: {
    color: "#64748b",
    marginTop: 5,
    fontSize: 12,
    fontWeight: "600",
  },
  inlineRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 12,
    marginTop: 10,
  },
  previewTitle: {
    color: "#0f172a",
    fontSize: 16,
    fontWeight: "800",
    marginTop: 8,
  },
  versionRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    marginTop: 12,
    paddingTop: 12,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  chunkBox: {
    marginTop: 10,
    padding: 12,
    borderRadius: 8,
    backgroundColor: "#f8fafc",
    borderWidth: 1,
    borderColor: "#e2e8f0",
  },
  chunkTitle: {
    color: "#334155",
    fontWeight: "800",
  },
  taskRow: {
    flexDirection: "row",
    gap: 12,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
    paddingTop: 12,
    marginTop: 12,
  },
  taskIndex: {
    width: 28,
    height: 28,
    borderRadius: 8,
    backgroundColor: "#e0f2fe",
    alignItems: "center",
    justifyContent: "center",
  },
  taskIndexText: {
    color: "#0369a1",
    fontWeight: "800",
  },
  taskBody: {
    flex: 1,
  },
  messageBox: {
    marginTop: 10,
    padding: 12,
    borderRadius: 8,
    backgroundColor: "#f8fafc",
    borderWidth: 1,
    borderColor: "#e2e8f0",
  },
  messageBody: {
    color: "#334155",
    marginTop: 8,
    lineHeight: 20,
  },
  approvalBox: {
    marginTop: 12,
    padding: 12,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "#cbd5e1",
    backgroundColor: "#f8fafc",
  },
  inlineActions: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 10,
  },
  inlineButton: {
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "#0f766e",
    paddingHorizontal: 10,
    paddingVertical: 7,
    alignSelf: "flex-start",
  },
  inlineButtonText: {
    color: "#0f766e",
    fontWeight: "800",
  },
  approveButton: {
    borderRadius: 8,
    backgroundColor: "#0f766e",
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  approveButtonText: {
    color: "#ffffff",
    fontWeight: "800",
  },
  rejectButton: {
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "#dc2626",
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  rejectButtonText: {
    color: "#dc2626",
    fontWeight: "800",
  },
  statusPill: {
    borderRadius: 8,
    paddingHorizontal: 8,
    paddingVertical: 5,
    flexShrink: 0,
  },
  statusText: {
    color: "#ffffff",
    fontSize: 11,
    fontWeight: "800",
    textTransform: "uppercase",
  },
  statusReady: {
    backgroundColor: "#0f766e",
  },
  statusRunning: {
    backgroundColor: "#ca8a04",
  },
  statusFailed: {
    backgroundColor: "#dc2626",
  },
  statusNeutral: {
    backgroundColor: "#64748b",
  },
  answerText: {
    color: "#0f172a",
    lineHeight: 22,
    fontSize: 15,
  },
  nextStep: {
    color: "#264653",
    fontWeight: "800",
    lineHeight: 21,
    marginTop: 10,
  },
  citationContainer: {
    marginTop: 12,
  },
  citationsTitle: {
    color: "#334155",
    fontWeight: "800",
    marginBottom: 8,
  },
  citationItem: {
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
    paddingTop: 10,
    marginTop: 10,
  },
  citationTitle: {
    color: "#0f172a",
    fontWeight: "800",
    flex: 1,
  },
  scoreText: {
    color: "#64748b",
    fontSize: 12,
    fontWeight: "700",
  },
  citationSnippet: {
    color: "#334155",
    lineHeight: 20,
    marginTop: 6,
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
    fontWeight: "800",
  },
  contextItemBody: {
    color: "#0f172a",
    lineHeight: 20,
    marginTop: 5,
  },
  evalGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 12,
    marginTop: 12,
  },
  evalCard: {
    flex: 1,
    minWidth: 220,
    borderWidth: 1,
    borderColor: "#cbd5e1",
    borderRadius: 8,
    padding: 12,
    backgroundColor: "#f8fafc",
  },
  emptyTitle: {
    color: "#0f172a",
    fontSize: 18,
    fontWeight: "800",
  },
  emptyText: {
    color: "#64748b",
    lineHeight: 20,
    marginTop: 8,
  },
  errorText: {
    color: "#dc2626",
    lineHeight: 20,
    marginTop: 6,
  },
});

export default AgentDemoScreen;
