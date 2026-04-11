import type { MediaStream } from "react-native-webrtc";

import type {
  MediaUpdatePayload,
  SdpRenegotiationPayload,
  SignalMessage,
  SubtitlePayload,
} from "../api/signaling";
import type { CameraFacing, VideoQuality } from "../services/VideoService";
import type { E2EESessionKey } from "../services/e2ee/E2EEService";
import type {
  E2EEKeyExchangeCallbacks,
  KeyExchangeRole,
} from "../services/e2ee/E2EEKeyExchange";
import type { OnlineTranslationStatus } from "../services/translation/OnlineTranslationService";

export type CallDirection = "incoming" | "outgoing";
export type SessionDescriptionPayload = RTCSessionDescriptionInit;
export type IceCandidatePayload = RTCIceCandidateInit & { iceEpoch?: number };
export type CallStatus = "idle" | "connecting" | "incoming" | "in_call";
export type TranslationInitStatus = "idle" | "initializing" | "ready" | "failed";
export type NetworkQuality = "excellent" | "good" | "poor" | "bad" | "unknown";
export type TranslationMode = "online";

export interface RTCIceServerConfig {
  urls: string | string[];
  username?: string;
  credential?: string;
}

export interface CallSession {
  callId: string;
  peerEmail: string;
  direction: CallDirection;
  offer?: SessionDescriptionPayload;
}

export interface SignalingContextValue {
  status: CallStatus;
  session: CallSession | null;
  connectionReady: boolean;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  networkQuality: NetworkQuality;
  isVideoEnabled: boolean;
  isAudioEnabled: boolean;
  isRemoteVideoEnabled: boolean;
  isRemoteAudioEnabled: boolean;
  cameraFacing: CameraFacing;
  e2eeEnabled: boolean;
  e2eeFingerprint: string | null;
  e2eeSessionEstablished: boolean;
  translationEnabled: boolean;
  translationLanguage: string;
  translationSourceLanguage: string;
  translationMode: TranslationMode;
  translationOnlineStatus: OnlineTranslationStatus;
  translationInitStatus: TranslationInitStatus;
  translationInitError: string | null;
  startCall: (email: string) => Promise<void>;
  acceptCall: () => Promise<void>;
  rejectCall: () => void;
  endCall: () => void;
  toggleVideo: () => Promise<void>;
  toggleAudio: () => void;
  switchCamera: () => Promise<void>;
  toggleSpeaker: () => Promise<void>;
  isSpeakerOn: boolean;
  videoQuality: VideoQuality;
  setVideoQuality: (quality: VideoQuality) => void;
  videoMaxBitrateKbps: number;
  setVideoMaxBitrateKbps: (kbps: number) => void;
  toggleTranslation: (enabled: boolean) => Promise<void>;
  setTranslationLanguage: (language: string) => void;
  setTranslationSourceLanguage: (language: string) => void;
  retryTranslationInitialization: () => Promise<void>;
}

export type SignalingEvents = {
  open: undefined;
  close: { code: number; reason?: string };
  message: SignalMessage;
  error: Error;
};

export interface SignalingTransport {
  connect: () => void;
  disconnect: () => void;
  on<T extends keyof SignalingEvents>(
    event: T,
    handler: (value: SignalingEvents[T]) => void
  ): void;
  send: (message: SignalMessage) => boolean;
}

export type {
  E2EEKeyExchangeCallbacks,
  E2EESessionKey,
  KeyExchangeRole,
  MediaUpdatePayload,
  OnlineTranslationStatus,
  SdpRenegotiationPayload,
  SignalMessage,
  SubtitlePayload,
};
