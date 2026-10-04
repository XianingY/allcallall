import type { QueryClient } from "@tanstack/react-query";

import type { Conversation, ConversationDetail, Message } from "@/api/collaboration";
import { conversationKeys } from "@/pages/collaboration/conversationQueryKeys";
import type { ChatEvent } from "@/realtime/chatEvents";

interface InfinitePageData<Page> {
  pages: Page[];
  pageParams: unknown[];
}

export type ChatCacheAction =
  | { type: "typing" }
  | { type: "append-message"; conversationId: number; message: Message }
  | { type: "patch-message"; conversationId: number; message: Message }
  | { type: "conversation-activity"; conversationId: number }
  | { type: "unknown"; conversationId?: number };

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null;

const isMessage = (value: unknown): value is Message => {
  if (!isRecord(value)) return false;
  const id = Number(value.id);
  const organizationId = Number(value.organization_id);
  const conversationId = Number(value.conversation_id);
  const senderId = Number(value.sender_id);

  return (
    Number.isSafeInteger(id) &&
    id > 0 &&
    Number.isSafeInteger(organizationId) &&
    organizationId > 0 &&
    Number.isSafeInteger(conversationId) &&
    conversationId > 0 &&
    Number.isSafeInteger(senderId) &&
    senderId > 0 &&
    typeof value.type === "string" &&
    typeof value.body === "string" &&
    typeof value.created_at === "string"
  );
};

const messageFromPayload = (payload: Record<string, unknown>): Message | null => {
  const candidate = isRecord(payload.message) ? payload.message : payload;
  return isMessage(candidate) ? candidate : null;
};

const conversationIdFromPayload = (payload: Record<string, unknown>): number | null => {
  const conversationId = Number(payload.conversation_id);
  return Number.isSafeInteger(conversationId) && conversationId > 0 ? conversationId : null;
};

export function planChatCacheAction(event: ChatEvent): ChatCacheAction {
  if (event.event.startsWith("typing.")) return { type: "typing" };

  const message = messageFromPayload(event.payload);
  const conversationId = conversationIdFromPayload(event.payload);

  if (message) {
    const targetConversationId = message.conversation_id;
    if (event.event === "message.created") {
      return { type: "append-message", conversationId: targetConversationId, message };
    }
    return { type: "patch-message", conversationId: targetConversationId, message };
  }

  if (conversationId) return { type: "conversation-activity", conversationId };
  return { type: "unknown" };
}

function appendMessage(
  queryClient: QueryClient,
  organizationId: number,
  conversationId: number,
  message: Message,
) {
  queryClient.setQueryData<InfinitePageData<{ messages: Message[] }>>(
    conversationKeys.messages(organizationId, conversationId),
    (data) => {
      if (!data?.pages.length) return data;
      if (data.pages.some((page) => page.messages.some((item) => item.id === message.id))) return data;

      const lastPage = data.pages.length - 1;
      const pages = data.pages.map((page, index) =>
        index === lastPage ? { ...page, messages: [...page.messages, message] } : page,
      );
      return { ...data, pages };
    },
  );
}

function patchMessage(
  queryClient: QueryClient,
  organizationId: number,
  conversationId: number,
  message: Message,
) {
  queryClient.setQueryData<InfinitePageData<{ messages: Message[] }>>(
    conversationKeys.messages(organizationId, conversationId),
    (data) => {
      if (!data?.pages.length) return data;

      const pages = data.pages.map((page) => ({
        ...page,
        messages: page.messages.map((item) => (item.id === message.id ? { ...item, ...message } : item)),
      }));
      return { ...data, pages };
    },
  );
}

function updateConversationState(
  queryClient: QueryClient,
  organizationId: number,
  message: Message,
  currentUserId?: number | null,
) {
  const fromCurrentUser = currentUserId === message.sender_id;
  const preview = message.body.trim() || "新消息";

  queryClient.setQueriesData({ queryKey: conversationKeys.all(organizationId) }, (data) => {
    if (
      isRecord(data) &&
      Array.isArray(data.pages) &&
      isRecord(data.pages[0]) &&
      Array.isArray(data.pages[0].conversations)
    ) {
      return {
        ...data,
        pages: data.pages.map((page: { conversations: Conversation[] }) => ({
          ...page,
          conversations: page.conversations.map((conversation) =>
            conversation.id === message.conversation_id
              ? {
                  ...conversation,
                  unread_count: fromCurrentUser
                    ? conversation.unread_count
                    : (conversation.unread_count ?? 0) + 1,
                  last_message_preview: preview,
                  last_message_at: message.created_at,
                }
              : conversation,
          ),
        })),
      };
    }

    if (isRecord(data) && isRecord(data.conversation)) {
      const detail = data as ConversationDetail;
      if (detail.conversation.id !== message.conversation_id) return data;

      return {
        ...detail,
        conversation: {
          ...detail.conversation,
          unread_count: fromCurrentUser
            ? detail.conversation.unread_count
            : (detail.conversation.unread_count ?? 0) + 1,
          last_message_preview: preview,
          last_message_at: message.created_at,
        },
      };
    }

    return data;
  });
}

export function applyChatEventToQueryCache(
  queryClient: QueryClient,
  organizationId: number,
  event: ChatEvent,
  currentUserId?: number | null,
) {
  const action = planChatCacheAction(event);

  switch (action.type) {
    case "typing":
      return;
    case "append-message":
      appendMessage(queryClient, organizationId, action.conversationId, action.message);
      updateConversationState(queryClient, organizationId, action.message, currentUserId);
      return;
    case "patch-message":
      patchMessage(queryClient, organizationId, action.conversationId, action.message);
      updateConversationState(queryClient, organizationId, action.message, currentUserId);
      return;
    case "conversation-activity":
      if (event.event.startsWith("message.")) {
        void queryClient.invalidateQueries({
          queryKey: conversationKeys.messages(organizationId, action.conversationId),
        });
      }
      void queryClient.invalidateQueries({
        queryKey: conversationKeys.detail(organizationId, action.conversationId),
      });
      return;
    case "unknown":
      return;
  }
}
