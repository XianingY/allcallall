import type { NativeStackScreenProps } from "@react-navigation/native-stack";

import type {
  AgentCitation,
  AgentRunEventRecord,
  AgentRunResult,
  ToolApprovalRecord,
  WorkflowResult,
} from "../../api/agent";
import type {
  ConversationNoteRecord,
  ConversationRecord,
  MessageRecord,
} from "../../api/collaboration";
import type {
  DeadLetterRecord,
  DuplicateCandidateRecord,
  KnowledgeSourceDetail,
  KnowledgeSourceRecord,
  SourceGroupRecord,
} from "../../api/knowledge";
import type { OrganizationRecord } from "../../api/collaboration";
import type { RootStackParamList } from "../../navigation/AppNavigator";
import type { ApprovalFilter, LabTab, WorkflowPreset } from "../agentDemoUtils";

export type Props =
  | NativeStackScreenProps<RootStackParamList, "AgentDemo">
  | NativeStackScreenProps<RootStackParamList, "KnowledgeCenter">;

/**
 * 所有 tab/侧边栏视图共享的控制器。AgentDemoScreen 持有全部状态与写操作，
 * 构造这个对象后透传给拆分出来的各个视图组件，避免在多个文件间重复 prop 列表。
 */
export interface AgentLabController {
  currentOrganization: OrganizationRecord | null;
  knowledgeOnly: boolean;
  visibleTabs: { key: LabTab; label: string }[];
  busy: boolean;
  refreshAll: () => Promise<void>;
  navigation: Props["navigation"];
  notice: string;
  conversations: ConversationRecord[];
  selectedConversationId: number | null;
  setSelectedConversationId: (id: number) => void;
  isWide: boolean;
  activeTab: LabTab;
  setActiveTab: (tab: LabTab) => void;
  manualTitle: string;
  setManualTitle: (value: string) => void;
  manualText: string;
  setManualText: (value: string) => void;
  urlTitle: string;
  setURLTitle: (value: string) => void;
  urlValue: string;
  setURLValue: (value: string) => void;
  fileTitle: string;
  setFileTitle: (value: string) => void;
  handleCreateDemoThread: () => Promise<void>;
  handleCreateManualSource: () => Promise<void>;
  handleCreateURLSource: () => Promise<void>;
  handleFileSelected: (file: File) => Promise<void>;
  selectSource: (sourceId: number) => Promise<void>;
  sources: KnowledgeSourceRecord[];
  sourceDetail: KnowledgeSourceDetail | null;
  selectedChunkStats: {
    indexed: number;
    failed: number;
    skipped: number;
    pending: number;
  };
  reingestKnowledgeSource: (token: string, id: number) => Promise<unknown>;
  token: string | null;
  refreshKnowledge: () => Promise<void>;
  sourceGroups: SourceGroupRecord[];
  handleSetCanonical: (groupId: number, sourceId: number) => Promise<void>;
  sourceTitleById: Map<number, string>;
  duplicateCandidates: DuplicateCandidateRecord[];
  handleDuplicateDecision: (
    id: number,
    decision: "confirm" | "reject",
  ) => Promise<void>;
  deadLetters: DeadLetterRecord[];
  retryKnowledgeDeadLetter: (token: string, id: number) => Promise<unknown>;
  selectedConversation: ConversationRecord | null;
  messages: MessageRecord[];
  notes: ConversationNoteRecord[];
  handleStartReactRun: () => Promise<void>;
  handleStartWorkflow: () => Promise<void>;
  goal: string;
  setGoal: (value: string) => void;
  workflowPreset: WorkflowPreset;
  setWorkflowPreset: (preset: WorkflowPreset) => void;
  activeRun: AgentRunResult | null;
  runEvents: AgentRunEventRecord[];
  refreshActiveRun: (runId: number) => Promise<void>;
  handleCitationPress: (citation: AgentCitation) => Promise<void>;
  activeWorkflow: WorkflowResult | null;
  pendingWorkflowApprovals: WorkflowResult["approvals"];
  handleProcessWorkflow: () => Promise<void>;
  orderedTasks: WorkflowResult["tasks"];
  approvals: ToolApprovalRecord[];
  approvalFilter: ApprovalFilter;
  setApprovalFilter: (filter: ApprovalFilter) => void;
  visibleApprovals: ToolApprovalRecord[];
  handleApproval: (
    approval: ToolApprovalRecord,
    decision: "approve" | "reject",
  ) => Promise<void>;
  workflows: WorkflowResult[];
  setActiveWorkflow: (workflow: WorkflowResult | null) => void;
}
