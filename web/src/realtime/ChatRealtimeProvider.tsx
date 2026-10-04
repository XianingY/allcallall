import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import { useAuth } from "@/auth/AuthContext";
import { useOrganization } from "@/organizations/OrganizationContext";
import { applyChatEventToQueryCache } from "@/realtime/chatCache";
import { ChatConnectionContext } from "@/realtime/ChatRealtimeContext";
import { initialChatCursor, reduceChatCursor, type ChatCursorState, type ChatEvent } from "@/realtime/chatEvents";
import { TicketSocket } from "@/realtime/TicketSocket";

const cursorStorageKey = (organizationId: number) => `allcallall:chat-cursor:${organizationId}`;

const loadCursor = (organizationId: number): ChatCursorState => {
  const value = Number(window.sessionStorage.getItem(cursorStorageKey(organizationId)) ?? 0);
  return Number.isSafeInteger(value) && value > 0 ? { cursor: value, recentIds: [] } : initialChatCursor;
};

export function ChatRealtimeProvider({ children }: { children: React.ReactNode }) {
  const { activeOrganization } = useOrganization();
  const { user } = useAuth();
  const organizationId = activeOrganization?.id;
  const queryClient = useQueryClient();
  const cursor = useRef<ChatCursorState>(initialChatCursor);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    if (!organizationId) return;
    cursor.current = loadCursor(organizationId);
    const socket = new TicketSocket<ChatEvent>("chat", () => ({
      organization_id: organizationId,
      since_id: cursor.current.cursor,
    }), (event) => {
      window.dispatchEvent(new CustomEvent("allcallall:chat-event", { detail: event }));
      if (event.event.startsWith("typing.")) return;
      const next = reduceChatCursor(cursor.current, event);
      if (next === cursor.current) return;
      cursor.current = next;
      window.sessionStorage.setItem(cursorStorageKey(organizationId), String(next.cursor));
      applyChatEventToQueryCache(queryClient, organizationId, event, user?.id);
    }, setConnected);
    socket.connect();
    return () => socket.disconnect();
  }, [organizationId, queryClient, user?.id]);

  return <ChatConnectionContext.Provider value={connected}>{children}</ChatConnectionContext.Provider>;
}
