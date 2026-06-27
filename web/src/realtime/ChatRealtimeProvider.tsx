import { useQueryClient } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useRef, useState } from "react";

import { useOrganization } from "@/organizations/OrganizationProvider";
import { initialChatCursor, reduceChatCursor, type ChatCursorState, type ChatEvent } from "@/realtime/chatEvents";
import { TicketSocket } from "@/realtime/TicketSocket";

const ChatConnectionContext = createContext(false);

export function ChatRealtimeProvider({ children }: { children: React.ReactNode }) {
  const { activeOrganization } = useOrganization();
  const organizationId = activeOrganization?.id;
  const queryClient = useQueryClient();
  const cursor = useRef<ChatCursorState>(initialChatCursor);
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    if (!organizationId) return;
    cursor.current = initialChatCursor;
    const socket = new TicketSocket<ChatEvent>("chat", { organization_id: organizationId, since_id: 0 }, (event) => {
      const next = reduceChatCursor(cursor.current, event);
      if (next === cursor.current) return;
      cursor.current = next;
      const conversationId = Number(event.payload.conversation_id || 0);
      void queryClient.invalidateQueries({ queryKey: ["organizations", organizationId, "conversations"] });
      if (conversationId) void queryClient.invalidateQueries({ queryKey: ["organizations", organizationId, "conversations", conversationId] });
    }, setConnected);
    socket.connect();
    return () => socket.disconnect();
  }, [organizationId, queryClient]);

  return <ChatConnectionContext.Provider value={connected}>{children}</ChatConnectionContext.Provider>;
}

export const useChatConnected = () => useContext(ChatConnectionContext);
