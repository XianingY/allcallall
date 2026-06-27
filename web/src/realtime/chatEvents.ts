export interface ChatEvent {
  event_id: number;
  sequence: number;
  event: string;
  organization_id: number;
  payload: Record<string, unknown>;
  created_at: string;
}

export interface ChatCursorState { cursor: number; recentIds: number[] }

export const initialChatCursor: ChatCursorState = { cursor: 0, recentIds: [] };

export function reduceChatCursor(state: ChatCursorState, event: ChatEvent): ChatCursorState {
  if (event.event_id <= state.cursor || state.recentIds.includes(event.event_id)) return state;
  return { cursor: Math.max(state.cursor, event.event_id), recentIds: [...state.recentIds.slice(-199), event.event_id] };
}

