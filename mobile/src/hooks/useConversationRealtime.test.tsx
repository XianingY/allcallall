import { renderHook } from "@testing-library/react-native";

import ChatRealtimeService from "../services/ChatRealtimeService";
import type { ChatEventPayload } from "../services/chatRealtimeCursor";
import { useConversationRealtime } from "./useConversationRealtime";

jest.mock("../services/ChatRealtimeService", () => ({
  __esModule: true,
  default: {
    connect: jest.fn(),
    disconnect: jest.fn(),
    on: jest.fn(),
    off: jest.fn(),
  },
}));

const createHandlers = () => ({
  onOpen: jest.fn(),
  onConversationUpdated: jest.fn(),
  onMessageCreated: jest.fn(),
  onMessageUpdated: jest.fn(),
  onMessageRecalled: jest.fn(),
  onMessageDeleted: jest.fn(),
  onNoteCreated: jest.fn(),
  onRecordingUpdated: jest.fn(),
  onRoomStateUpdated: jest.fn(),
  onRoomEnded: jest.fn(),
});

describe("useConversationRealtime", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("connects and subscribes when token and organization are present", () => {
    renderHook(() =>
      useConversationRealtime({
        token: "token",
        organizationId: 10,
        ...createHandlers(),
      }),
    );

    expect(ChatRealtimeService.connect).toHaveBeenCalledWith("token", 10);
    expect(ChatRealtimeService.on).toHaveBeenCalledWith(
      "open",
      expect.any(Function),
    );
    expect(ChatRealtimeService.on).toHaveBeenCalledWith(
      "event",
      expect.any(Function),
    );
  });

  it("routes each realtime event to its handler", () => {
    const handlers = createHandlers();
    renderHook(() =>
      useConversationRealtime({
        token: "token",
        organizationId: 10,
        ...handlers,
      }),
    );

    const eventHandler = getEventHandler();
    const openHandler = getRegisteredHandler("open");

    const cases = [
      ["conversation.updated", handlers.onConversationUpdated, { id: 1 }],
      ["message.created", handlers.onMessageCreated, { id: 2 }],
      ["message.updated", handlers.onMessageUpdated, { id: 3 }],
      ["message.recalled", handlers.onMessageRecalled, { id: 4 }],
      ["message.deleted", handlers.onMessageDeleted, { id: 5 }],
      ["conversation.note.created", handlers.onNoteCreated, { id: 6 }],
      ["room.recording.updated", handlers.onRecordingUpdated, { id: 7 }],
      ["room.state.updated", handlers.onRoomStateUpdated, { id: 8 }],
      ["room.ended", handlers.onRoomEnded, { id: 9 }],
    ] as const;

    for (const [eventName, handler, payload] of cases) {
      eventHandler({
        event: eventName,
        organization_id: 10,
        payload,
      });
      expect(handler).toHaveBeenCalledWith(payload);
    }

    openHandler(undefined);

    for (const handler of Object.values(handlers)) {
      expect(handler).toHaveBeenCalledTimes(1);
    }

    eventHandler({
      event: "workflow.updated",
      organization_id: 10,
      payload: { id: 10 },
    });
    for (const handler of Object.values(handlers)) {
      expect(handler).toHaveBeenCalledTimes(1);
    }
  });

  it("keeps one subscription across callback changes and dispatches to the latest callback", () => {
    const firstOnMessageCreated = jest.fn();
    const { rerender } = renderHook(
      (props: { onMessageCreated: typeof firstOnMessageCreated }) =>
        useConversationRealtime({
          token: "token",
          organizationId: 10,
          ...createHandlers(),
          onMessageCreated: props.onMessageCreated,
        }),
      {
        initialProps: { onMessageCreated: firstOnMessageCreated },
      },
    );

    expect(getConnectCount()).toBe(1);
    expect(getSubscriptionCount("event")).toBe(1);

    const latestOnMessageCreated = jest.fn();
    rerender({ onMessageCreated: latestOnMessageCreated });

    expect(getConnectCount()).toBe(1);
    expect(getSubscriptionCount("event")).toBe(1);

    getEventHandler()({
      event: "message.created",
      organization_id: 10,
      payload: { id: 11 },
    });
    expect(firstOnMessageCreated).not.toHaveBeenCalled();
    expect(latestOnMessageCreated).toHaveBeenCalledWith({ id: 11 });
  });

  it("disconnects without subscribing when credentials are missing", () => {
    renderHook(() =>
      useConversationRealtime({
        token: null,
        organizationId: null,
        ...createHandlers(),
      }),
    );

    expect(ChatRealtimeService.disconnect).toHaveBeenCalledTimes(1);
    expect(ChatRealtimeService.connect).not.toHaveBeenCalled();
    expect(ChatRealtimeService.on).not.toHaveBeenCalled();
  });

  it("cleans up subscriptions on unmount", () => {
    const { unmount } = renderHook(() =>
      useConversationRealtime({
        token: "token",
        organizationId: 10,
        ...createHandlers(),
      }),
    );

    unmount();

    const openHandler = getRegisteredHandler("open");
    const eventHandler = getEventHandler();
    expect(ChatRealtimeService.off).toHaveBeenCalledWith("open", openHandler);
    expect(ChatRealtimeService.off).toHaveBeenCalledWith(
      "event",
      eventHandler,
    );
    expect(ChatRealtimeService.off).toHaveBeenCalledTimes(2);
  });
});

function getRegisteredHandler(event: "open" | "event") {
  const calls = (ChatRealtimeService.on as jest.Mock).mock.calls.filter(
    ([registeredEvent]) => registeredEvent === event,
  );
  if (calls.length !== 1) {
    throw new Error(`Expected exactly one ${event} subscription`);
  }
  return calls[0][1] as (value: unknown) => void;
}

function getEventHandler() {
  return getRegisteredHandler("event") as (
    event: ChatEventPayload,
  ) => void;
}

function getConnectCount() {
  return (ChatRealtimeService.connect as jest.Mock).mock.calls.length;
}

function getSubscriptionCount(event: "open" | "event") {
  return (ChatRealtimeService.on as jest.Mock).mock.calls.filter(
    ([registeredEvent]) => registeredEvent === event,
  ).length;
}
