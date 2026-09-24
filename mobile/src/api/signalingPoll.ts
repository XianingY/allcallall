// #22：/signaling/poll 与 /signaling/send 已纳入 openapi.yaml，线上报文形状改用
// @allcallall/api-types 生成的共享契约描述。SignalMessage 仍是 ./signaling 中的应用内
// 领域类型（被 SignalingContext / signalingTransports 共用），故仅在收发边界做转换。
import mitt from "mitt";

import type { operations } from "@allcallall/api-types";
import { createApiClient } from "./client";
import type { SignalMessage } from "./signaling";

type PollResponse =
  operations["pollSignaling"]["responses"][200]["content"]["application/json"];
type SendRequestBody =
  operations["sendSignaling"]["requestBody"]["content"]["application/json"];

const fromWireMessage = (payload: PollResponse): SignalMessage =>
  payload as unknown as SignalMessage;

const toWireMessage = (message: SignalMessage): SendRequestBody =>
  message as unknown as SendRequestBody;

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
        const parsed = fromWireMessage(JSON.parse(data) as PollResponse);
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
        await api.post("/signaling/send", toWireMessage(message));
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
