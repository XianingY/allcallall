import { useEffect, useLayoutEffect, useRef } from "react";

import type { MessageRecord } from "../api/collaboration";
import ChatRealtimeService from "../services/ChatRealtimeService";
import type { ChatEventPayload } from "../services/chatRealtimeCursor";
import type { ConversationUpdatedPayload } from "../services/conversationRealtimeReducer";

export interface UseConversationRealtimeOptions {
  token: string | null | undefined;
  organizationId: number | null | undefined;
  onOpen: () => void;
  onConversationUpdated: (payload: ConversationUpdatedPayload) => void;
  onMessageCreated: (message: MessageRecord) => void;
  onMessageUpdated: (message: MessageRecord) => void;
  onMessageRecalled: (message: MessageRecord) => void;
  onMessageDeleted: (message: MessageRecord) => void;
  onNoteCreated: (payload: unknown) => void;
  onRecordingUpdated: (payload: unknown) => void;
  onRoomStateUpdated: (payload: unknown) => void;
  onRoomEnded: (payload: unknown) => void;
}

export function useConversationRealtime(
  options: UseConversationRealtimeOptions,
) {
  const handlersRef = useRef(options);

  // Layout effects run before the passive connection effect below. A realtime
  // event that arrives between render and effect flush must therefore see the
  // callbacks from the latest render, not the previous conversation's handlers.
  useLayoutEffect(() => {
    handlersRef.current = options;
  });

  const { token, organizationId } = options;

  useEffect(() => {
    if (!token || organizationId == null) {
      ChatRealtimeService.disconnect();
      return;
    }

    const handleOpen = () => {
      handlersRef.current.onOpen();
    };

    const handleEvent = (event: ChatEventPayload) => {
      switch (event.event) {
        case "conversation.updated":
          handlersRef.current.onConversationUpdated(
            event.payload as ConversationUpdatedPayload,
          );
          break;
        case "message.created":
          handlersRef.current.onMessageCreated(event.payload as MessageRecord);
          break;
        case "message.updated":
          handlersRef.current.onMessageUpdated(event.payload as MessageRecord);
          break;
        case "message.recalled":
          handlersRef.current.onMessageRecalled(
            event.payload as MessageRecord,
          );
          break;
        case "message.deleted":
          handlersRef.current.onMessageDeleted(event.payload as MessageRecord);
          break;
        case "conversation.note.created":
          handlersRef.current.onNoteCreated(event.payload);
          break;
        case "room.recording.updated":
          handlersRef.current.onRecordingUpdated(event.payload);
          break;
        case "room.state.updated":
          handlersRef.current.onRoomStateUpdated(event.payload);
          break;
        case "room.ended":
          handlersRef.current.onRoomEnded(event.payload);
          break;
        default:
          break;
      }
    };

    ChatRealtimeService.connect(token, organizationId);
    ChatRealtimeService.on("open", handleOpen);
    ChatRealtimeService.on("event", handleEvent);

    return () => {
      ChatRealtimeService.off("open", handleOpen);
      ChatRealtimeService.off("event", handleEvent);
    };
  }, [organizationId, token]);
}
