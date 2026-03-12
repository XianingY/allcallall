import { EmitterSubscription, NativeEventEmitter, NativeModules } from "react-native";

import { TRANSLATION_WS_URL } from "~/config";

const { WebRTCModule } = NativeModules;

export type OnlineTranslationStatus = "idle" | "connecting" | "connected" | "retrying" | "error";

export interface OnlineTranslationResult {
  sessionId: string;
  segmentId: string;
  revision: number;
  isFinal: boolean;
  originalText: string;
  translatedText: string;
  timestampMs: number;
  latencyMs: number;
}

export interface OnlineTranslationStartParams {
  token: string;
  callId: string;
  to: string;
  sourceLang: "zh" | "en";
  targetLang: "zh" | "en";
  chunkMs: number;
}

interface OnlineTranslationCallbacks {
  onStatus: (status: OnlineTranslationStatus) => void;
  onResult: (result: OnlineTranslationResult) => void;
  onProviderError: (code: string, message: string, recoverable: boolean) => void;
}

class OnlineTranslationService {
  private ws: WebSocket | null = null;
  private active = false;
  private connected = false;
  private startParams: OnlineTranslationStartParams | null = null;
  private callbacks: OnlineTranslationCallbacks | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempts = 0;

  private audioEmitter: NativeEventEmitter | null = null;
  private audioSub: EmitterSubscription | null = null;
  private seq = 1;
  private sessionId = "";

  private pendingConnectResolve: (() => void) | null = null;
  private pendingConnectReject: ((error: Error) => void) | null = null;
  private connectTimeout: ReturnType<typeof setTimeout> | null = null;

  async start(params: OnlineTranslationStartParams, callbacks: OnlineTranslationCallbacks): Promise<void> {
    await this.stop();

    this.active = true;
    this.connected = false;
    this.startParams = params;
    this.callbacks = callbacks;
    this.reconnectAttempts = 0;
    this.seq = 1;
    this.sessionId = "";

    await this.connect(false);
  }

  async stop(): Promise<void> {
    this.active = false;
    this.connected = false;
    this.startParams = null;

    if (this.connectTimeout) {
      clearTimeout(this.connectTimeout);
      this.connectTimeout = null;
    }

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    this.stopCapture();

    if (this.ws) {
      try {
        this.ws.send(JSON.stringify({ type: "translation.stop", reason: "user_toggle" }));
      } catch {}

      try {
        this.ws.close();
      } catch {}

      this.ws.onopen = null;
      this.ws.onmessage = null;
      this.ws.onerror = null;
      this.ws.onclose = null;
      this.ws = null;
    }

    this.resolveConnectWaiter();

    this.callbacks?.onStatus("idle");
  }

  isConnected(): boolean {
    return this.connected;
  }

  private connect(isRetry: boolean): Promise<void> {
    const params = this.startParams;
    const callbacks = this.callbacks;
    if (!params || !callbacks) {
      return Promise.reject(new Error("online translation not initialized"));
    }

    callbacks.onStatus(isRetry ? "retrying" : "connecting");

    return new Promise((resolve, reject) => {
      this.pendingConnectResolve = resolve;
      this.pendingConnectReject = reject;

      const url = `${TRANSLATION_WS_URL}?token=${encodeURIComponent(params.token)}`;
      this.ws = new WebSocket(url);

      this.connectTimeout = setTimeout(() => {
        this.connectTimeout = null;
        if (!this.connected) {
          try {
            this.ws?.close();
          } catch {}
          const err = new Error("translation websocket ack timeout");
          callbacks.onProviderError("ACK_TIMEOUT", err.message, true);
          this.rejectConnectWaiter(err);
          this.scheduleReconnect();
        }
      }, 8000);

      this.ws.onopen = () => {
        this.sendStart();
      };

      this.ws.onmessage = (event) => {
        this.handleMessage(event.data);
      };

      this.ws.onerror = () => {
        callbacks.onStatus("error");
      };

      this.ws.onclose = () => {
        const shouldReconnect = this.active;
        this.connected = false;
        this.stopCapture();

        if (this.connectTimeout) {
          clearTimeout(this.connectTimeout);
          this.connectTimeout = null;
        }

        if (shouldReconnect) {
          this.scheduleReconnect();
        } else {
          this.resolveConnectWaiter();
        }
      };
    });
  }

  private scheduleReconnect() {
    if (!this.active || !this.startParams || !this.callbacks) {
      return;
    }

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    this.reconnectAttempts += 1;
    this.reconnectTimer = setTimeout(() => {
      if (!this.active) {
        return;
      }

      this.connect(true).catch((error) => {
        const message = error instanceof Error ? error.message : String(error);
        this.callbacks?.onProviderError("RECONNECT_FAILED", message, true);
      });
    }, 2000);
  }

  private sendStart() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN || !this.startParams) {
      return;
    }

    this.ws.send(
      JSON.stringify({
        type: "translation.start",
        call_id: this.startParams.callId,
        to: this.startParams.to,
        source_lang: this.startParams.sourceLang,
        target_lang: this.startParams.targetLang,
        chunk_ms: this.startParams.chunkMs,
      })
    );
  }

  private handleMessage(raw: string) {
    const callbacks = this.callbacks;
    if (!callbacks) {
      return;
    }

    let msg: Record<string, unknown>;
    try {
      msg = JSON.parse(raw);
    } catch {
      callbacks.onProviderError("BAD_PAYLOAD", "invalid provider payload", true);
      return;
    }

    const type = typeof msg.type === "string" ? msg.type : "";

    if (type === "translation.ack") {
      this.connected = true;
      this.sessionId = typeof msg.session_id === "string" ? msg.session_id : "";
      callbacks.onStatus("connected");
      this.resolveConnectWaiter();
      this.startCapture();
      return;
    }

    if (type === "translation.error") {
      const code = typeof msg.code === "string" ? msg.code : "PROVIDER_ERROR";
      const message = typeof msg.message === "string" ? msg.message : "provider error";
      const recoverable = msg.recoverable === true;
      callbacks.onProviderError(code, message, recoverable);
      return;
    }

    if (type !== "translation.partial" && type !== "translation.final") {
      return;
    }

    const result: OnlineTranslationResult = {
      sessionId: typeof msg.session_id === "string" ? msg.session_id : this.sessionId,
      segmentId: typeof msg.segment_id === "string" ? msg.segment_id : `seg-${Date.now()}`,
      revision: typeof msg.revision === "number" ? msg.revision : 1,
      isFinal: msg.is_final === true || type === "translation.final",
      originalText: typeof msg.original_text === "string" ? msg.original_text : "",
      translatedText: typeof msg.translated_text === "string" ? msg.translated_text : "",
      timestampMs: typeof msg.timestamp_ms === "number" ? msg.timestamp_ms : Date.now(),
      latencyMs: typeof msg.latency_ms === "number" ? msg.latency_ms : 0,
    };

    callbacks.onResult(result);
  }

  private startCapture() {
    if (!WebRTCModule || typeof WebRTCModule.startLocalAudioCapture !== "function") {
      this.callbacks?.onProviderError("AUDIO_CAPTURE_UNAVAILABLE", "WebRTCModule.startLocalAudioCapture not available", false);
      return;
    }
    if (this.audioSub) {
      return;
    }

    this.audioEmitter = new NativeEventEmitter(WebRTCModule);
    this.audioSub = this.audioEmitter.addListener("webrtcAudioChunk", (payload: unknown) => {
      if (!this.connected || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
        return;
      }

      const data = payload as {
        pcm16Base64?: string;
        sampleRate?: number;
        channels?: number;
        timestampMs?: number;
      };

      const pcm16Base64 = typeof data?.pcm16Base64 === "string" ? data.pcm16Base64 : "";
      if (!pcm16Base64) {
        return;
      }

      this.ws.send(
        JSON.stringify({
          type: "translation.audio",
          seq: this.seq++,
          pcm16_base64: pcm16Base64,
          sample_rate: typeof data.sampleRate === "number" ? data.sampleRate : 16000,
          channels: typeof data.channels === "number" ? data.channels : 1,
          timestamp_ms: typeof data.timestampMs === "number" ? data.timestampMs : Date.now(),
        })
      );
    });

    const chunkMS = this.startParams?.chunkMs ?? 400;
    WebRTCModule.startLocalAudioCapture(chunkMS);
  }

  private stopCapture() {
    if (this.audioSub) {
      this.audioSub.remove();
      this.audioSub = null;
    }
    this.audioEmitter = null;

    if (WebRTCModule && typeof WebRTCModule.stopLocalAudioCapture === "function") {
      try {
        WebRTCModule.stopLocalAudioCapture();
      } catch {}
    }
  }

  private resolveConnectWaiter() {
    if (this.connectTimeout) {
      clearTimeout(this.connectTimeout);
      this.connectTimeout = null;
    }
    if (this.pendingConnectResolve) {
      this.pendingConnectResolve();
      this.pendingConnectResolve = null;
      this.pendingConnectReject = null;
    }
  }

  private rejectConnectWaiter(error: Error) {
    if (this.connectTimeout) {
      clearTimeout(this.connectTimeout);
      this.connectTimeout = null;
    }
    if (this.pendingConnectReject) {
      this.pendingConnectReject(error);
      this.pendingConnectResolve = null;
      this.pendingConnectReject = null;
    }
  }
}

export default new OnlineTranslationService();
