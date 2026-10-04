import { QueryClient } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Conversation, ConversationDetail, Message } from "@/api/collaboration";
import { conversationKeys } from "@/pages/collaboration/conversationQueryKeys";
import { applyChatEventToQueryCache } from "./chatCache";
import type { ChatEvent } from "./chatEvents";

const organizationId = 7;
const conversationId = 5;
const currentUserId = 1;

const existingMessage = {
  id: 101,
  organization_id: organizationId,
  conversation_id: conversationId,
  sender_id: currentUserId,
  sender_email: "ada@example.com",
  sender_display_name: "Ada",
  type: "text",
  body: "Existing message",
  pinned: false,
  created_at: "2026-10-01T00:00:00Z",
} as unknown as Message;

const incomingMessage = {
  ...existingMessage,
  id: 102,
  sender_id: 2,
  sender_email: "bob@example.com",
  sender_display_name: "Bob",
  body: "Incoming message",
  created_at: "2026-10-01T00:01:00Z",
} as unknown as Message;

const conversation = {
  id: conversationId,
  organization_id: organizationId,
  type: "direct",
  title: "Customer support",
  status: "open",
  priority: "normal",
  unread_count: 0,
  last_message_preview: "Existing message",
  last_message_at: "2026-10-01T00:00:00Z",
} as unknown as Conversation;

const detail = {
  conversation,
  workspace: {
    agent_context: {
      transcript_segment_count: 0,
    },
    status: "open",
    priority: "normal",
  },
} as unknown as ConversationDetail;

const chatEvent = (event: string, payload: Record<string, unknown>): ChatEvent => ({
  event_id: 42,
  sequence: 1,
  event,
  organization_id: organizationId,
  payload,
  created_at: "2026-10-01T00:01:00Z",
});

function seedClient() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(conversationKeys.messages(organizationId, conversationId), {
    pages: [{ messages: [existingMessage], has_more_prev: false }],
    pageParams: [undefined],
  });
  client.setQueryData(conversationKeys.list(organizationId, ""), {
    pages: [{ conversations: [conversation], pagination: { total: 1, limit: 50, offset: 0, has_more: false } }],
    pageParams: [0],
  });
  client.setQueryData(conversationKeys.detail(organizationId, conversationId), detail);
  return client;
}

describe("chat cache updates", () => {
  let client: QueryClient;
  let invalidateQueries: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    client = seedClient();
    invalidateQueries = vi.spyOn(client, "invalidateQueries");
  });

  it("appends a complete incoming message and updates list state without refetching", () => {
    applyChatEventToQueryCache(
      client,
      organizationId,
      chatEvent("message.created", { ...incomingMessage, conversation_id: conversationId }),
      currentUserId,
    );

    expect(client.getQueryData(conversationKeys.messages(organizationId, conversationId))).toMatchObject({
      pages: [{ messages: [existingMessage, incomingMessage] }],
    });
    expect(client.getQueryData(conversationKeys.list(organizationId, ""))).toMatchObject({
      pages: [
        {
          conversations: [
            {
              unread_count: 1,
              last_message_preview: "Incoming message",
              last_message_at: "2026-10-01T00:01:00Z",
            },
          ],
        },
      ],
    });
    expect(client.getQueryData(conversationKeys.detail(organizationId, conversationId))).toMatchObject({
      conversation: { unread_count: 1 },
    });
    expect(invalidateQueries).not.toHaveBeenCalled();
  });

  it("does not duplicate a message that is already cached", () => {
    applyChatEventToQueryCache(
      client,
      organizationId,
      chatEvent("message.created", { ...existingMessage, conversation_id: conversationId }),
      currentUserId,
    );

    expect(client.getQueryData(conversationKeys.messages(organizationId, conversationId))).toMatchObject({
      pages: [{ messages: [existingMessage] }],
    });
    expect(invalidateQueries).not.toHaveBeenCalled();
  });

  it("does not change server queries for typing events", () => {
    applyChatEventToQueryCache(
      client,
      organizationId,
      chatEvent("typing.started", { conversation_id: conversationId, user_id: 2, typing: true }),
      currentUserId,
    );

    expect(invalidateQueries).not.toHaveBeenCalled();
    expect(client.getQueryData(conversationKeys.messages(organizationId, conversationId))).toMatchObject({
      pages: [{ messages: [existingMessage] }],
    });
  });

  it("invalidates only the affected message and detail queries for incomplete message events", () => {
    applyChatEventToQueryCache(
      client,
      organizationId,
      chatEvent("message.created", { conversation_id: conversationId, message_id: 103 }),
      currentUserId,
    );

    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: conversationKeys.messages(organizationId, conversationId),
    });
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: conversationKeys.detail(organizationId, conversationId),
    });
    expect(invalidateQueries).not.toHaveBeenCalledWith({
      queryKey: conversationKeys.all(organizationId),
    });
  });

  it("uses a narrow detail invalidation for unknown conversation events", () => {
    applyChatEventToQueryCache(
      client,
      organizationId,
      chatEvent("conversation.updated", { conversation_id: conversationId }),
      currentUserId,
    );

    expect(invalidateQueries).toHaveBeenCalledTimes(1);
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: conversationKeys.detail(organizationId, conversationId),
    });
  });
});
