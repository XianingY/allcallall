import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { TicketSocket } from "./TicketSocket";
import { issueRealtimeTicket } from "@/api/realtime";
import { runtimeConfig } from "@/lib/runtime-config";

vi.mock("@/api/realtime", () => ({
  issueRealtimeTicket: vi.fn(),
}));

vi.mock("@/lib/runtime-config", () => ({
  runtimeConfig: { wsBaseUrl: "wss://test.example.com" },
}));

// Minimal WebSocket mock that lets tests drive lifecycle events.
let lastSocket: MockWebSocket | null = null;

class MockWebSocket {
  static readonly OPEN = 1;
  static readonly CLOSED = 3;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  readyState = 0;
  sent: string[] = [];
  readonly url: string;

  constructor(url: string) {
    this.url = url;
    lastSocket = this;
  }

  send(data: string): void {
    this.sent.push(data);
  }

  close(): void {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.();
  }

  triggerOpen(): void {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  triggerMessage(data: unknown): void {
    this.onmessage?.({ data });
  }

  triggerClose(): void {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.();
  }

  triggerError(): void {
    this.onerror?.();
  }
}

const RealWebSocket = (globalThis as unknown as { WebSocket?: unknown }).WebSocket;

beforeEach(() => {
  vi.clearAllMocks();
  lastSocket = null;
  (globalThis as unknown as { WebSocket: unknown }).WebSocket = MockWebSocket;
  (issueRealtimeTicket as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
    ticket: "tk_123",
    websocket_path: "/rt",
  });
});

afterEach(() => {
  (globalThis as unknown as { WebSocket: unknown }).WebSocket = RealWebSocket;
});

describe("TicketSocket", () => {
  it("connects, opens the socket, and reports connected state", async () => {
    const onMessage = vi.fn();
    const onState = vi.fn();
    const socket = new TicketSocket<{ hello: string }>(
      "chat",
      { room: "r1" },
      onMessage,
      onState,
    );

    socket.connect();

    // Wait for issueRealtimeTicket to resolve and the WebSocket to be created.
    await vi.waitFor(() => expect(lastSocket).not.toBeNull());
    lastSocket!.triggerOpen();

    expect(lastSocket!.url).toBe(
      "wss://test.example.com/rt?ticket=tk_123&room=r1",
    );
    expect(onState).toHaveBeenCalledWith(true);
    socket.disconnect();
  });

  it("parses incoming messages and forwards them to onMessage", async () => {
    const onMessage = vi.fn();
    const onState = vi.fn();
    const socket = new TicketSocket<{ hello: string }>(
      "chat",
      { room: "r1" },
      onMessage,
      onState,
    );

    socket.connect();
    await vi.waitFor(() => expect(lastSocket).not.toBeNull());
    lastSocket!.triggerOpen();
    lastSocket!.triggerMessage(JSON.stringify({ hello: "world" }));

    expect(onMessage).toHaveBeenCalledWith({ hello: "world" });
    socket.disconnect();
  });

  it("ignores malformed realtime messages", async () => {
    const onMessage = vi.fn();
    const onState = vi.fn();
    const socket = new TicketSocket<{ hello: string }>(
      "chat",
      { room: "r1" },
      onMessage,
      onState,
    );

    socket.connect();
    await vi.waitFor(() => expect(lastSocket).not.toBeNull());
    lastSocket!.triggerOpen();
    lastSocket!.triggerMessage("not-json{{");

    expect(onMessage).not.toHaveBeenCalled();
    socket.disconnect();
  });

  it("send returns false when the socket is not open", () => {
    const onMessage = vi.fn();
    const onState = vi.fn();
    const socket = new TicketSocket<unknown>("chat", {}, onMessage, onState);

    expect(socket.send({ a: 1 })).toBe(false);
  });

  it("send serializes and returns true when the socket is open", async () => {
    const onMessage = vi.fn();
    const onState = vi.fn();
    const socket = new TicketSocket<unknown>("chat", {}, onMessage, onState);

    socket.connect();
    await vi.waitFor(() => expect(lastSocket).not.toBeNull());
    lastSocket!.triggerOpen();

    expect(socket.send({ a: 1 })).toBe(true);
    expect(lastSocket!.sent).toEqual([JSON.stringify({ a: 1 })]);
    socket.disconnect();
  });

  it("disconnect closes the socket and reports disconnected state", async () => {
    const onMessage = vi.fn();
    const onState = vi.fn();
    const socket = new TicketSocket<unknown>("chat", {}, onMessage, onState);

    socket.connect();
    await vi.waitFor(() => expect(lastSocket).not.toBeNull());
    lastSocket!.triggerOpen();
    onState.mockClear();

    socket.disconnect();

    expect(lastSocket!.readyState).toBe(MockWebSocket.CLOSED);
    expect(onState).toHaveBeenCalledWith(false);
  });

  it("reschedules a reconnect after the socket closes", async () => {
    const onMessage = vi.fn();
    const onState = vi.fn();
    const realSetTimeout = window.setTimeout;
    // Run the reconnect callback synchronously so we can observe re-open.
    window.setTimeout = (((fn: () => void) => {
      fn();
      return 0 as unknown as number;
    }) as unknown) as typeof window.setTimeout;

    try {
      const socket = new TicketSocket<unknown>("chat", {}, onMessage, onState);

      socket.connect();
      await vi.waitFor(() => expect(lastSocket).not.toBeNull());
      lastSocket!.triggerOpen();
      expect(issueRealtimeTicket).toHaveBeenCalledTimes(1);

      lastSocket!.triggerClose();
      // The synchronous reconnect callback re-runs open() -> issueRealtimeTicket.
      await vi.waitFor(() =>
        expect(issueRealtimeTicket).toHaveBeenCalledTimes(2),
      );

      socket.disconnect();
    } finally {
      window.setTimeout = realSetTimeout;
    }
  });
});
