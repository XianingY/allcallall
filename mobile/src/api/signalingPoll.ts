import mitt from "mitt";

import { createApiClient } from "./client";
import type { SignalMessage } from "./signaling";

type Events = {
  open: undefined;
  close: { code: number; reason?: string };
  message: SignalMessage;
  error: Error;
};

export class PollingSignalingClient {
  private token: string;
  private emitter = mitt<Events>();
  private shouldRun = false;
  private pollInFlight = false;
  private firstOpenEmitted = false;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pendingMessages: SignalMessage[] = [];
  private static readonly MAX_PENDING_MESSAGES = 50;

  constructor(token: string) {
    this.token = token;
  }

  connect() {
    if (this.shouldRun) return;
    this.shouldRun = true;
    this.loop();
  }

  private scheduleNext(ms: number) {
    if (!this.shouldRun) return;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = setTimeout(() => this.loop(), ms);
  }

  private async loop() {
    if (!this.shouldRun) return;
    if (this.pollInFlight) return;
    this.pollInFlight = true;

    try {
      const api = createApiClient(this.token);
      const resp = await api.get<string>("/signaling/poll", {
        params: { timeout_ms: 25_000 },
        responseType: "text",
        validateStatus: (s) => (s >= 200 && s < 300) || s === 204
      });

      if (!this.firstOpenEmitted) {
        this.firstOpenEmitted = true;
        this.emitter.emit("open", undefined);
      }

      if (resp.status === 204) {
        this.flushPending();
        this.pollInFlight = false;
        this.scheduleNext(0);
        return;
      }

      const data = typeof resp.data === "string" ? resp.data : "";
      if (data) {
        const parsed = JSON.parse(data) as SignalMessage;
        this.emitter.emit("message", parsed);
      }
      this.flushPending();
      this.pollInFlight = false;
      this.scheduleNext(0);
    } catch (error) {
      this.emitter.emit("error", error as Error);
      this.pollInFlight = false;
      this.scheduleNext(1500);
    }
  }

  private async flushPending() {
    if (!this.shouldRun) return;
    if (this.pendingMessages.length === 0) return;
    const queue = [...this.pendingMessages];
    this.pendingMessages = [];

    const api = createApiClient(this.token);
    for (let index = 0; index < queue.length; index += 1) {
      const message = queue[index];
      try {
        await api.post("/signaling/send", message);
      } catch (error) {
        this.emitter.emit("error", error as Error);
        const remaining = queue.slice(index);
        this.pendingMessages = remaining.concat(this.pendingMessages);
        break;
      }
    }
  }

  send(message: SignalMessage): boolean {
    if (!this.shouldRun) {
      this.connect();
    }

    if (this.pendingMessages.length >= PollingSignalingClient.MAX_PENDING_MESSAGES) {
      throw new Error("signaling queue overflow");
    }

    this.pendingMessages.push(message);
    this.flushPending().catch(() => {
      console.warn("[PollingSignalingClient] flushPending failed");
    });
    return true;
  }

  disconnect() {
    this.shouldRun = false;
    this.pollInFlight = false;
    this.firstOpenEmitted = false;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.pendingMessages = [];
    this.emitter.emit("close", { code: 1000, reason: "client disconnect" });
  }

  on<T extends keyof Events>(event: T, handler: (value: Events[T]) => void) {
    this.emitter.on(event, handler);
  }

  off<T extends keyof Events>(event: T, handler: (value: Events[T]) => void) {
    this.emitter.off(event, handler);
  }
}
