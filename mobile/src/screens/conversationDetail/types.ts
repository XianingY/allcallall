import type { NativeStackScreenProps } from "@react-navigation/native-stack";
import type {
  AttachmentRecord,
  ConversationRecord,
  ConversationWorkspaceRecord,
  ConversationNoteRecord,
  ConversationFollowupRecord,
  RecordingRecord,
  RoomListItemRecord,
  MessageRecord,
} from "../../api/collaboration";
import type { User } from "../../api/users";
import type {
  AgentCitation,
  CreateWorkflowRequest,
  ToolApprovalRecord,
  WorkflowResult,
} from "../../api/agent";
import type { KnowledgeSourceDetail } from "../../api/knowledge";
import type { RootStackParamList } from "../../navigation/AppNavigator";
import { STATUS_OPTIONS, PRIORITY_OPTIONS } from "../conversationDetailUtils";

export type ScreenProps = NativeStackScreenProps<
  RootStackParamList,
  "ConversationDetail"
>;

/**
 * The screen resolves `conversation` as
 * `detail?.conversation ?? route.params.conversation ?? <fallback literal>`.
 * Both branches share the fields the workspace pane actually reads.
 */
export type ConversationLike =
  | ConversationRecord
  | {
      id: number;
      organization_id: number;
      type: string;
      title: string;
      status: string;
      priority: string;
      unread_count: number;
    };

export type AgentContextShape = ConversationWorkspaceRecord["agent_context"];

export type WorkflowTask = NonNullable<WorkflowResult["tasks"]>[number];

export interface MessageRowProps {
  item: MessageRecord;
  currentUserId?: number | string;
  onOpenTranscript: (recordingId: number) => void;
  /** 长按消息弹出操作菜单（编辑/撤回/删除）。 */
  onLongPress: (message: MessageRecord) => void;
  /** 点击附件触发授权下载。 */
  onDownloadAttachment: (attachment: AttachmentRecord) => void;
}

export interface WorkspacePaneProps {
  conversation: ConversationLike;
  workspace: ConversationWorkspaceRecord | undefined;
  latestRoom: RoomListItemRecord | null | undefined;
  latestFollowup: ConversationFollowupRecord | null | undefined;
  assigneeLabel: string;
  boundContact: User | undefined;
  agentContext: AgentContextShape | undefined;
  contacts: User[];
  notes: ConversationNoteRecord[];
  latestRecording: RecordingRecord | null;
  activeWorkflow: WorkflowResult | null;
  pendingApprovals: ToolApprovalRecord[];
  completedTaskCount: number;
  executedApprovalCount: number;
  rejectedApprovalCount: number;
  transcriptStatusText: string;
  agentStatusLabel: string;
  meetingTranscriptReady: boolean;
  workflowLoading: boolean;
  navigation: ScreenProps["navigation"];
  onCopyLink: () => Promise<void>;
  onCreateMeeting: () => Promise<void>;
  onAssignSelf: () => Promise<void>;
  onUnassign: () => Promise<void>;
  onRunMeetingAgent: (
    input: { preset?: CreateWorkflowRequest["preset"]; goal?: string },
  ) => Promise<boolean>;
  onOpenWorkflowDebug: () => void;
  onCitationPress: (citation: AgentCitation) => Promise<void>;
  onApprovalDecision: (
    approval: ToolApprovalRecord,
    decision: "approve" | "reject",
  ) => Promise<void>;
  onUpdateStatus: (
    status: (typeof STATUS_OPTIONS)[number],
  ) => Promise<void>;
  onUpdatePriority: (
    priority: (typeof PRIORITY_OPTIONS)[number],
  ) => Promise<void>;
  onBindContact: (contactId: number | null) => Promise<void>;
  onDownloadRecording: (
    recordingId: number,
    fileId: number,
    fileName: string,
  ) => Promise<void>;
  noteDraft: string;
  onNoteDraftChange: (text: string) => void;
  onAddNote: () => Promise<void>;
}

export interface MessagePaneProps {
  messages: MessageRecord[];
  loading: boolean;
  hasMorePrev: boolean;
  loadingMorePrev: boolean;
  sending: boolean;
  workflowLoading: boolean;
  currentUserId: number | string | undefined;
  /** 已上传、待随下一条消息发送的附件。 */
  pendingAttachments: AttachmentRecord[];
  uploadingAttachment: boolean;
  onRefresh: () => void;
  onLoadMorePrev: () => Promise<void>;
  onSend: (draft: string) => Promise<boolean>;
  onAskAgent: (draft: string) => Promise<boolean>;
  onOpenTranscript: (recordingId: number) => void;
  onLongPressMessage: (message: MessageRecord) => void;
  onDownloadAttachment: (attachment: AttachmentRecord) => void;
  onPickAttachment: () => void;
  onRemovePendingAttachment: (attachmentId: number) => void;
}

export interface MessageActionMenuModalProps {
  visible: boolean;
  message: MessageRecord | null;
  currentUserId: number | string | undefined;
  canDeleteAny: boolean;
  onClose: () => void;
  onEdit: (message: MessageRecord) => void;
  onRecall: (message: MessageRecord) => void;
  onDelete: (message: MessageRecord) => void;
}

export interface EditMessageModalProps {
  visible: boolean;
  message: MessageRecord | null;
  draft: string;
  saving: boolean;
  onDraftChange: (text: string) => void;
  onClose: () => void;
  onSave: () => void;
}

export interface KnowledgePreviewModalProps {
  knowledgePreview: KnowledgeSourceDetail | null;
  onClose: () => void;
}

export interface CitationPreviewModalProps {
  citationPreview: AgentCitation | null;
  onClose: () => void;
}

export interface WorkflowDebugModalProps {
  visible: boolean;
  activeWorkflow: WorkflowResult | null;
  orderedTasks: WorkflowTask[];
  workflowLoading: boolean;
  token: string | null;
  onClose: () => void;
  onProcess: () => Promise<void>;
}
