import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { PAGE_SIZE } from "@/api/pagination";
import {
  getConversation,
  listConversations,
  listMessages,
  listNotes,
  listPinnedMessages,
  searchMessages,
} from "@/api/collaboration";
import { conversationKeys } from "@/pages/collaboration/conversationQueryKeys";
import { windowMessages } from "@/pages/collaboration/messageWindow";

export interface ConversationFilterState {
  status: string;
  keyword: string;
  unreadOnly: boolean;
  messageQuery: string;
}

export function useConversationQueries(input: {
  organizationId: number | undefined;
  conversationId: number | null;
  filter: ConversationFilterState;
}) {
  const { organizationId, conversationId, filter } = input;

  const messageHits = useQuery({
    queryKey: conversationKeys.messageSearch(organizationId, filter.messageQuery),
    queryFn: () => searchMessages(filter.messageQuery),
    enabled: Boolean(organizationId) && filter.messageQuery.trim().length >= 2,
    retry: false,
  });

  const conversations = useInfiniteQuery({
    queryKey: conversationKeys.list(organizationId, filter.status),
    queryFn: ({ pageParam }) =>
      listConversations(filter.status, { limit: PAGE_SIZE, offset: pageParam }),
    initialPageParam: 0 as number,
    getNextPageParam: (lastPage) =>
      lastPage.pagination.has_more
        ? lastPage.pagination.offset + lastPage.pagination.limit
        : undefined,
    maxPages: 20,
    enabled: Boolean(organizationId),
  });

  const detail = useQuery({
    queryKey: conversationKeys.detail(organizationId, conversationId),
    queryFn: () => getConversation(conversationId!),
    enabled: Boolean(organizationId && conversationId),
  });

  const messages = useInfiniteQuery({
    queryKey: conversationKeys.messages(organizationId, conversationId),
    queryFn: ({ pageParam }) =>
      listMessages(conversationId!, { beforeId: pageParam, limit: 50 }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (page) =>
      page.has_more_prev ? page.next_before_id ?? undefined : undefined,
    enabled: Boolean(organizationId && conversationId),
  });

  const pins = useQuery({
    queryKey: conversationKeys.pins(organizationId, conversationId),
    queryFn: () => listPinnedMessages(conversationId!),
    enabled: Boolean(organizationId && conversationId),
  });

  const notes = useQuery({
    queryKey: conversationKeys.notes(organizationId, conversationId),
    queryFn: () => listNotes(conversationId!),
    enabled: Boolean(organizationId && conversationId),
  });

  const visibleConversations = useMemo(() => {
    const loaded = (conversations.data?.pages ?? []).flatMap((page) => page.conversations);
    const needle = filter.keyword.trim().toLowerCase();
    return loaded.filter((item) => {
      if (filter.unreadOnly && !(item.unread_count > 0)) return false;
      if (!needle) return true;
      return `${item.title} ${item.last_message_preview ?? ""} ${item.topic ?? ""}`
        .toLowerCase()
        .includes(needle);
    });
  }, [conversations.data?.pages, filter.keyword, filter.unreadOnly]);

  const messageItems = useMemo(() => {
    const pages = messages.data?.pages ?? [];
    return pages.slice().reverse().flatMap((page) => page.messages);
  }, [messages.data?.pages]);

  const messageWindow = useMemo(
    () => windowMessages(messageItems),
    [messageItems],
  );

  return {
    messageHits,
    conversations,
    detail,
    messages,
    pins,
    notes,
    visibleConversations,
    messageItems,
    messageWindow,
  };
}

export type ConversationQueries = ReturnType<typeof useConversationQueries>;
