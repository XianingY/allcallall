import mitt from "mitt";

import {
  SIGNALING_SHAPING_ENABLED,
  WS_URL
} from "../config";

export type SessionDescriptionPayload = {
  type: "offer" | "answer";
  sdp: string;
};

export type SdpRenegotiationPayload = SessionDescriptionPayload & {
  iceEpoch?: number;
};

export type MediaUpdatePayload = {
  audioEnabled: boolean;
  videoEnabled: boolean;
};

export type SubtitlePayload = {
  segment_id: string;
  revision: number;
  is_final: boolean;
  original_text: string;
  translated_text: string;
  timestamp_ms: number;
  source?: "online" | "offline" | "remote";
};

export type SignalMessageType =
  | "call.invite"
  | "call.invite.ack"
  | "call.accept"
  | "call.reject"
  | "call.end"
  | "ice.candidate"
  | "call.sdp.offer"
  | "call.sdp.answer"
  | "call.ice-restart.request"
  | "call.media_update"
  | "call.subtitle"
  | "call.error"
  | "client.ping"
  | "server.pong";

export interface SignalMessage {
  type: SignalMessageType;
  call_id?: string;
  to: string;
  from?: string;
  payload?:
    | Record<string, unknown>
    | RTCIceCandidateInit
    | SdpRenegotiationPayload
    | MediaUpdatePayload
    | SubtitlePayload
    | null;
}

type Events = {
  open: undefined;
  close: { code: number; reason?: string };
  message: SignalMessage;
  error: Error;
};

export class SignalingClient {
  private token: string;
  private ws: WebSocket | null = null;
  private emitter = mitt<Events>();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private keepaliveTimer: ReturnType<typeof setInterval> | null = null;
  private shouldReconnect = true;
  private pendingMessages: SignalMessage[] = [];
  private static readonly MAX_PENDING_MESSAGES = 50;

  constructor(token: string) {
    this.token = token;
  }

  connect() {
    if (this.ws) {
      return;
    }

    this.shouldReconnect = true;
    this.openSocket();
  }

  private flushPendingMessages() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return;
    }
    const queue = [...this.pendingMessages];
    this.pendingMessages = [];
    for (let index = 0; index < queue.length; index += 1) {
      const message = queue[index];
      try {
        this.sendWithShaping(message);
      } catch (error) {
        console.warn("Failed to flush signaling message", error);
        const remaining = queue.slice(index);
        this.pendingMessages = remaining.concat(this.pendingMessages);
        try {
          this.ws?.close();
        } catch (closeError) {
          console.warn("Failed to close signaling socket after flush error", closeError);
        }
        break;
      }
    }
  }

  private openSocket() {
    if (this.ws) {
      return;
    }
    // Include token as query parameter for authorization
    const wsUrlWithAuth = `${WS_URL}?token=${encodeURIComponent(this.token)}`;
    this.ws = new WebSocket(wsUrlWithAuth);

    this.ws.onopen = () => {
      this.emitter.emit("open", undefined);
      this.startKeepalive();
      this.flushPendingMessages();
    };

    this.ws.onclose = (event) => {
      this.emitter.emit("close", { code: event.code, reason: event.reason });
      this.stopKeepalive();
      this.cleanup();
      if (this.shouldReconnect) {
        this.reconnectTimer = setTimeout(() => this.openSocket(), 3000);
      }
    };

    this.ws.onerror = (e) => {
      const error = e as unknown as Error;
      this.emitter.emit("error", error);
    };

    this.ws.onmessage = (event) => {
      try {
        const parsed: SignalMessage = JSON.parse(event.data);
        this.emitter.emit("message", parsed);
      } catch (err) {
        this.emitter.emit("error", err as Error);
      }
    };
  }

  private startKeepalive() {
    if (!SIGNALING_SHAPING_ENABLED) return;
    if (this.keepaliveTimer) return;
    this.keepaliveTimer = setInterval(() => {
      try {
        this.sendWithShaping({ type: "client.ping", to: "_", payload: null });
      } catch (error) {
        console.warn("[SignalingClient] keepalive send failed", error);
      }
    }, 25_000);
  }

  private stopKeepalive() {
    if (!this.keepaliveTimer) return;
    clearInterval(this.keepaliveTimer);
    this.keepaliveTimer = null;
  }

  private sendWithShaping(message: SignalMessage) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
    this.ws.send(JSON.stringify(message));
  }

  send(message: SignalMessage): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      if (this.pendingMessages.length >= SignalingClient.MAX_PENDING_MESSAGES) {
        throw new Error("signaling queue overflow");
      }
      this.pendingMessages.push(message);
      if (!this.ws && this.shouldReconnect) {
        this.openSocket();
      }
      return false;
    }
    this.sendWithShaping(message);
    return true;
  }

  disconnect() {
    this.shouldReconnect = false;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.stopKeepalive();
      this.cleanup();
    }
    this.pendingMessages = [];
  }

  on<T extends keyof Events>(event: T, handler: (value: Events[T]) => void) {
    this.emitter.on(event, handler);
  }

  off<T extends keyof Events>(event: T, handler: (value: Events[T]) => void) {
    this.emitter.off(event, handler);
  }

  private cleanup() {
    if (this.ws) {
      this.ws.onopen = null;
      this.ws.onclose = null;
      this.ws.onerror = null;
      this.ws.onmessage = null;
      this.ws = null;
    }
  }
}
