import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ChatEvent } from "@/realtime/chatEvents";
import { useChatConnected } from "@/realtime/ChatRealtimeContext";
import { ChatRealtimeProvider } from "@/realtime/ChatRealtimeProvider";

const { instances, activeOrganization } = vi.hoisted(() => ({
  instances: [] as Array<{
    channel: string;
    query: () => Record<string, string | number>;
    onMessage: (event: ChatEvent) => void;
    onState: (connected: boolean) => void;
    connect: ReturnType<typeof vi.fn>;
    disconnect: ReturnType<typeof vi.fn>;
  }>,
  activeOrganization: { id: 7 },
}));

vi.mock("@/realtime/TicketSocket", () => ({
  // Vitest 5 requires constructable mocks to use a regular function or
  // class; an arrow-function implementation cannot be invoked with `new`.
  TicketSocket: vi.fn().mockImplementation(function (
    channel: string,
    query: () => Record<string, string | number>,
    onMessage: (event: ChatEvent) => void,
    onState: (connected: boolean) => void,
  ) {
    const socket = {
      channel,
      query,
      onMessage,
      onState,
      connect: vi.fn(),
      disconnect: vi.fn(),
    };
    instances.push(socket);
    return socket;
  }),
}));

vi.mock("@/organizations/OrganizationContext", () => ({
  useOrganization: () => ({ activeOrganization }),
}));

function Probe() {
  const connected = useChatConnected();
  return <span data-testid="connected">{String(connected)}</span>;
}

const chatEvent = (overrides: Partial<ChatEvent>): ChatEvent => ({
  event_id: 42,
  sequence: 1,
  event: "message.created",
  organization_id: 7,
  payload: { conversation_id: 99 },
  created_at: "2026-10-01T00:00:00Z",
  ...overrides,
});

function renderProvider() {
  const client = new QueryClient();
  const invalidateQueries = vi.spyOn(client, "invalidateQueries");
  const rendered = render(
    <QueryClientProvider client={client}>
      <ChatRealtimeProvider>
        <Probe />
      </ChatRealtimeProvider>
    </QueryClientProvider>,
  );
  return { client, invalidateQueries, ...rendered };
}

describe("ChatRealtimeProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    instances.length = 0;
    window.sessionStorage.clear();
  });
  afterEach(cleanup);

  it("does not connect without an active organization", () => {
    const previous = activeOrganization.id;
    activeOrganization.id = 0;
    try {
      renderProvider();
      expect(instances).toHaveLength(0);
    } finally {
      activeOrganization.id = previous;
    }
  });

  it("connects a chat socket with the persisted cursor and tears it down on unmount", () => {
    window.sessionStorage.setItem("allcallall:chat-cursor:7", "12");
    const view = renderProvider();

    expect(instances).toHaveLength(1);
    expect(instances[0].channel).toBe("chat");
    expect(instances[0].query()).toEqual({ organization_id: 7, since_id: 12 });
    expect(instances[0].connect).toHaveBeenCalledTimes(1);

    view.unmount();
    expect(instances[0].disconnect).toHaveBeenCalledTimes(1);
  });

  it("starts disconnected and reflects socket state", () => {
    const view = renderProvider();
    expect(view.getByTestId("connected").textContent).toBe("false");
    act(() => { instances[0].onState(true); });
    expect(view.getByTestId("connected").textContent).toBe("true");
    act(() => { instances[0].onState(false); });
    expect(view.getByTestId("connected").textContent).toBe("false");
  });

  it("persists the cursor and invalidates conversation queries on events", () => {
    const windowEvents: Array<{ type: string; detail: ChatEvent }> = [];
    const listener = (event: Event) => {
      windowEvents.push({ type: event.type, detail: (event as CustomEvent<ChatEvent>).detail });
    };
    window.addEventListener("allcallall:chat-event", listener);
    try {
      const { invalidateQueries } = renderProvider();
      instances[0].onMessage(chatEvent({ event_id: 42, payload: { conversation_id: 99 } }));

      expect(windowEvents).toHaveLength(1);
      expect(windowEvents[0].detail.event_id).toBe(42);
      expect(window.sessionStorage.getItem("allcallall:chat-cursor:7")).toBe("42");
      expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["organizations", 7, "conversations"] });
      expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["organizations", 7, "conversations", 99] });
    } finally {
      window.removeEventListener("allcallall:chat-event", listener);
    }
  });

  it("broadcasts typing events but does not advance or rewrite the cursor", () => {
    window.sessionStorage.setItem("allcallall:chat-cursor:7", "50");
    const { invalidateQueries } = renderProvider();
    instances[0].onMessage(chatEvent({ event_id: 51, event: "typing.start", payload: {} }));

    expect(window.sessionStorage.getItem("allcallall:chat-cursor:7")).toBe("50");
    expect(invalidateQueries).not.toHaveBeenCalled();
  });

  it("ignores duplicate and stale event ids", () => {
    window.sessionStorage.setItem("allcallall:chat-cursor:7", "50");
    const { invalidateQueries } = renderProvider();
    instances[0].onMessage(chatEvent({ event_id: 50, payload: { conversation_id: 99 } }));
    instances[0].onMessage(chatEvent({ event_id: 49, payload: { conversation_id: 99 } }));

    expect(window.sessionStorage.getItem("allcallall:chat-cursor:7")).toBe("50");
    expect(invalidateQueries).not.toHaveBeenCalled();
  });
});
