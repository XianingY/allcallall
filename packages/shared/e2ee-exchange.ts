/**
 * Key-exchange handshake over a WebRTC DataChannel, shared by web and mobile.
 *
 * The exchange is deliberately symmetric: both sides generate a key pair and
 * send a pubkey; the responder also confirms with an ack carrying the
 * fingerprint. callId is baked into the derived key, so both sides derive the
 * same key only for the same call.
 */

import {
  deriveSessionKey,
  generateEcdhKeyPair,
  type E2EEKeyPair,
  type E2EESessionKey,
} from "./e2ee";

export type KeyExchangeMessage =
  | { type: "pubkey"; publicKey: string }
  | { type: "ack"; fingerprint: string };

export type KeyExchangeRole = "initiator" | "responder";

/** The slice of RTCDataChannel this class needs. */
export interface DataChannelLike {
  readyState: string;
  send: (data: string) => void;
  onmessage: ((event: { data: string }) => void) | null;
}

export interface E2EEKeyExchangeCallbacks {
  onSessionEstablished: (session: E2EESessionKey) => void;
  onError: (error: Error) => void;
}

export class E2EEKeyExchange {
  private role: KeyExchangeRole;
  private callId: string;
  private myKeyPair: E2EEKeyPair | null = null;
  private peerPublicKey: string | null = null;
  private sessionKey: E2EESessionKey | null = null;
  private callbacks: E2EEKeyExchangeCallbacks;
  private dataChannel: DataChannelLike | null = null;

  constructor(role: KeyExchangeRole, callId: string, callbacks: E2EEKeyExchangeCallbacks) {
    this.role = role;
    this.callId = callId;
    this.callbacks = callbacks;
  }

  async initialize(): Promise<void> {
    try {
      this.myKeyPair = await generateEcdhKeyPair();
    } catch (error) {
      this.callbacks.onError(error as Error);
    }
  }

  attachDataChannel(dataChannel: DataChannelLike): void {
    this.dataChannel = dataChannel;
    this.dataChannel.onmessage = (event) => {
      void this.handleMessage(event.data);
    };
  }

  async sendPublicKey(): Promise<void> {
    if (!this.myKeyPair || !this.dataChannel) {
      return;
    }
    const message: KeyExchangeMessage = { type: "pubkey", publicKey: this.myKeyPair.publicKey };
    if (this.dataChannel.readyState === "open") {
      this.dataChannel.send(JSON.stringify(message));
    }
  }

  private async handleMessage(data: string): Promise<void> {
    try {
      const message = JSON.parse(data) as KeyExchangeMessage;

      if (message.type === "pubkey") {
        this.peerPublicKey = message.publicKey;
        await this.deriveAndNotify();
      } else if (message.type === "ack") {
        if (this.sessionKey && message.fingerprint === this.sessionKey.fingerprint) {
          this.callbacks.onSessionEstablished(this.sessionKey);
        } else {
          this.callbacks.onError(new Error("Fingerprint mismatch"));
        }
      }
    } catch (error) {
      this.callbacks.onError(error as Error);
    }
  }

  private async deriveAndNotify(): Promise<void> {
    if (!this.myKeyPair || !this.peerPublicKey) {
      return;
    }

    try {
      this.sessionKey = await deriveSessionKey(this.myKeyPair.privateKey, this.peerPublicKey, this.callId);

      if (this.role === "responder" && this.dataChannel) {
        const ackMessage: KeyExchangeMessage = { type: "ack", fingerprint: this.sessionKey.fingerprint };
        this.dataChannel.send(JSON.stringify(ackMessage));
      }

      this.callbacks.onSessionEstablished(this.sessionKey);
    } catch (error) {
      this.callbacks.onError(error as Error);
    }
  }

  getSessionKey(): E2EESessionKey | null {
    return this.sessionKey;
  }

  destroy(): void {
    this.myKeyPair = null;
    this.peerPublicKey = null;
    this.sessionKey = null;
    this.dataChannel = null;
  }
}