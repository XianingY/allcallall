import mitt from "mitt";

import { WS_HOST } from "../config";

type ChatEventPayload = {
  event: string;
  organization_id: number;
  payload: unknown;
};

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
  private shouldReconnect = false;
  private token: string | null = null;
  private organizationId: number | null = null;

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
    const url = `${WS_HOST}/api/v1/chat/ws?token=${encodeURIComponent(this.token)}&organization_id=${this.organizationId}`;
    const socket = new WebSocket(url);
    this.socket = socket;

    socket.onopen = () => {
      this.emitter.emit("open", undefined);
    };

    socket.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data) as ChatEventPayload;
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
      this.emitter.emit("close", undefined);
      if (this.shouldReconnect && this.token && this.organizationId) {
        this.reconnectTimer = setTimeout(() => this.openSocket(), 2000);
      }
    };
  }
}

export default new ChatRealtimeService();
