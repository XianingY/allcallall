import { memo } from "react";

import type { AgentCitation, WorkflowResult } from "../../api/agent";
import type { MessageRecord } from "../../api/collaboration";
import type { KnowledgeSourceDetail } from "../../api/knowledge";
import type { WorkflowTask } from "./types";
import {
  CitationPreviewModal,
  EditMessageModal,
  KnowledgePreviewModal,
  MessageActionMenuModal,
  WorkflowDebugModal,
} from "./Modals";

interface ConversationDetailModalsProps {
  knowledgePreview: KnowledgeSourceDetail | null;
  citationPreview: AgentCitation | null;
  workflowDebugVisible: boolean;
  activeWorkflow: WorkflowResult | null;
  orderedTasks: WorkflowTask[];
  workflowLoading: boolean;
  token: string | null;
  actionMenuMessage: MessageRecord | null;
  editingMessage: MessageRecord | null;
  editDraft: string;
  savingEdit: boolean;
  currentUserId: number | string | undefined;
  onCloseKnowledgePreview: () => void;
  onCloseCitationPreview: () => void;
  onCloseWorkflowDebug: () => void;
  onCloseActionMenu: () => void;
  onStartEditMessage: (message: MessageRecord) => void;
  onRecallMessage: (message: MessageRecord) => void;
  onDeleteMessage: (message: MessageRecord) => void;
  onDraftChange: (text: string) => void;
  onCloseEditMessage: () => void;
  onSaveEditMessage: () => void;
  onProcessCurrentWorkflow: () => Promise<void>;
}

const ConversationDetailModals = memo(
  ({
    knowledgePreview,
    citationPreview,
    workflowDebugVisible,
    activeWorkflow,
    orderedTasks,
    workflowLoading,
    token,
    actionMenuMessage,
    editingMessage,
    editDraft,
    savingEdit,
    currentUserId,
    onCloseKnowledgePreview,
    onCloseCitationPreview,
    onCloseWorkflowDebug,
    onCloseActionMenu,
    onStartEditMessage,
    onRecallMessage,
    onDeleteMessage,
    onDraftChange,
    onCloseEditMessage,
    onSaveEditMessage,
    onProcessCurrentWorkflow,
  }: ConversationDetailModalsProps) => (
    <>
      <KnowledgePreviewModal
        knowledgePreview={knowledgePreview}
        onClose={onCloseKnowledgePreview}
      />
      <CitationPreviewModal
        citationPreview={citationPreview}
        onClose={onCloseCitationPreview}
      />
      <WorkflowDebugModal
        visible={workflowDebugVisible}
        activeWorkflow={activeWorkflow}
        orderedTasks={orderedTasks}
        workflowLoading={workflowLoading}
        token={token}
        onClose={onCloseWorkflowDebug}
        onProcess={onProcessCurrentWorkflow}
      />
      <MessageActionMenuModal
        visible={actionMenuMessage !== null}
        message={actionMenuMessage}
        currentUserId={currentUserId}
        canDeleteAny={false}
        onClose={onCloseActionMenu}
        onEdit={onStartEditMessage}
        onRecall={onRecallMessage}
        onDelete={onDeleteMessage}
      />
      <EditMessageModal
        visible={editingMessage !== null}
        message={editingMessage}
        draft={editDraft}
        saving={savingEdit}
        onDraftChange={onDraftChange}
        onClose={onCloseEditMessage}
        onSave={onSaveEditMessage}
      />
    </>
  ),
);

export default ConversationDetailModals;
