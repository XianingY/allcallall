import type {
  ConversationDetailRecord,
  ConversationRecord,
} from "../api/collaboration";

export interface ConversationUpdatedPayload {
  conversation_id?: number;
  changes?: Partial<ConversationRecord>;
}

const sameOptionalValue = (left: unknown, right: unknown): boolean =>
  Object.is(left, right) || (left == null && right == null);

const shallowEqual = (left: object, right: object): boolean => {
  const leftRecord = left as Record<string, unknown>;
  const rightRecord = right as Record<string, unknown>;
  const keys = new Set([...Object.keys(leftRecord), ...Object.keys(rightRecord)]);

  return (
    [...keys].every((key) => sameOptionalValue(leftRecord[key], rightRecord[key]))
  );
};

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
    const patched = { ...conversation, ...payload.changes };
    if (shallowEqual(patched, conversation)) {
      return conversation;
    }
    changed = true;
    return patched;
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
  const hasAssigneeIdentityPatch =
    changes.assignee_user_id !== undefined ||
    changes.assignee_display_name !== undefined ||
    changes.assignee_email !== undefined;
  const assigneeId =
    changes.assignee_user_id !== undefined
      ? changes.assignee_user_id
      : detail.workspace.assignee_user_id;
  const nextWorkspace = {
    ...detail.workspace,
    assignee_user_id: assigneeId,
    assignee_label:
      hasAssigneeIdentityPatch
        ? changes.assignee_display_name ||
            changes.assignee_email ||
            "未指派"
        : detail.workspace.assignee_label,
    status: changes.status || detail.workspace.status,
    priority: changes.priority || detail.workspace.priority,
  };

  if (shallowEqual(nextConversation, detail.conversation) && shallowEqual(nextWorkspace, detail.workspace)) {
    return detail;
  }

  return {
    ...detail,
    conversation: nextConversation,
    workspace: nextWorkspace,
  };
};
