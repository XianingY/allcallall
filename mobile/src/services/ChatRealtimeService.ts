import mitt from "mitt";

import { WS_HOST } from "../config";
import { ChatEventPayload, ChatRealtimeCursor } from "./chatRealtimeCursor";

type Events = {
  open: undefined;
  close: undefined;
  event: ChatEventPayload;
  error: Error;
};

class ChatRealtimeService {
  private emitter = mitt<Events>();
  private socket: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private shouldReconnect = false;
  private token: string | null = null;
  private organizationId: number | null = null;
  private readonly cursor = new ChatRealtimeCursor(ChatRealtimeService.DEDUPE_WINDOW_MS);

  private static readonly HEARTBEAT_INTERVAL_MS = 20_000;
  private static readonly DEDUPE_WINDOW_MS = 5_000;

  connect(token: string, organizationId: number) {
    if (this.socket && this.token === token && this.organizationId === organizationId) {
      return;
    }
    this.disconnect();
    this.token = token;
    this.organizationId = organizationId;
    this.shouldReconnect = true;
    this.openSocket();
  }

  disconnect() {
    this.shouldReconnect = false;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
  }

  on<T extends keyof Events>(event: T, handler: (value: Events[T]) => void) {
    this.emitter.on(event, handler);
  }

  off<T extends keyof Events>(event: T, handler: (value: Events[T]) => void) {
    this.emitter.off(event, handler);
  }

  private openSocket() {
    if (!this.token || !this.organizationId || this.socket) {
      return;
    }
    const sinceId = this.cursor.getSinceId(this.organizationId);
    const url = `${WS_HOST}/api/v1/chat/ws?token=${encodeURIComponent(this.token)}&organization_id=${this.organizationId}&since_id=${sinceId}`;
    const socket = new WebSocket(url);
    this.socket = socket;

    socket.onopen = () => {
      this.startHeartbeat(socket);
      this.emitter.emit("open", undefined);
    };

    socket.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data) as ChatEventPayload;
        if (this.cursor.shouldSkip(payload)) {
          return;
        }
        this.emitter.emit("event", payload);
      } catch (error) {
        this.emitter.emit("error", error as Error);
      }
    };

    socket.onerror = (event) => {
      this.emitter.emit("error", event as unknown as Error);
    };

    socket.onclose = () => {
      this.socket = null;
      if (this.heartbeatTimer) {
        clearInterval(this.heartbeatTimer);
        this.heartbeatTimer = null;
      }
      this.emitter.emit("close", undefined);
      if (this.shouldReconnect && this.token && this.organizationId) {
        this.reconnectTimer = setTimeout(() => this.openSocket(), 2000);
      }
    };
  }

  private startHeartbeat(socket: WebSocket) {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
    }
    this.heartbeatTimer = setInterval(() => {
      if (socket.readyState !== WebSocket.OPEN) {
        return;
      }
      try {
        socket.send(JSON.stringify({
          type: "ping",
          sent_at: new Date().toISOString()
        }));
      } catch (error) {
        this.emitter.emit("error", error as Error);
      }
    }, ChatRealtimeService.HEARTBEAT_INTERVAL_MS);
  }
}

export default new ChatRealtimeService();
