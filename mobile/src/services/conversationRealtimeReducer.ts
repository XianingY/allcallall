import type {
  ConversationDetailRecord,
  ConversationRecord,
} from "../api/collaboration";

export interface ConversationUpdatedPayload {
  conversation_id?: number;
  changes?: Partial<ConversationRecord>;
}

export const applyConversationListPatch = (
  conversations: ConversationRecord[],
  payload: ConversationUpdatedPayload | undefined
): ConversationRecord[] => {
  if (!payload?.conversation_id || !payload.changes) {
    return conversations;
  }

  let changed = false;
  const next = conversations.map((conversation) => {
    if (conversation.id !== payload.conversation_id) {
      return conversation;
    }
    changed = true;
    return { ...conversation, ...payload.changes };
  });
  return changed ? next : conversations;
};

export const applyConversationDetailPatch = (
  detail: ConversationDetailRecord | null,
  payload: ConversationUpdatedPayload | undefined
): ConversationDetailRecord | null => {
  if (!detail || !payload?.conversation_id || !payload.changes) {
    return detail;
  }
  if (detail.conversation.id !== payload.conversation_id) {
    return detail;
  }

  const changes = payload.changes;
  const nextConversation = { ...detail.conversation, ...changes };
  return {
    ...detail,
    conversation: nextConversation,
    workspace: {
      ...detail.workspace,
      assignee_user_id: changes.assignee_user_id ?? detail.workspace.assignee_user_id,
      assignee_label:
        changes.assignee_display_name ||
        changes.assignee_email ||
        detail.workspace.assignee_label,
      status: changes.status || detail.workspace.status,
      priority: changes.priority || detail.workspace.priority,
    },
  };
};
