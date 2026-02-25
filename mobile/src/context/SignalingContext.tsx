import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState
} from "react";
import {
  Alert,
  PermissionsAndroid,
  Platform
} from "react-native";
import {
  MediaStream,
  RTCPeerConnection,
  RTCIceCandidate,
  RTCSessionDescription,
  mediaDevices as webrtcMediaDevices,
  type RTCRtpTransceiver
} from "react-native-webrtc";

import {
  MediaUpdatePayload,
  SdpRenegotiationPayload,
  SignalingClient,
  SignalMessage
} from "../api/signaling";
import { PollingSignalingClient } from "../api/signalingPoll";
import { RESTRICTED_NETWORK_MODE, SIGNALING_TRANSPORT_MODE } from "../config";
import { fetchWebRTCConfig } from "../api/webrtc";
import { useAuthContext } from "./AuthContext";
import { useSettings } from "./SettingsContext";
import AudioService from "../services/AudioServiceExpo";
import VibrationService from "../services/VibrationService";
import VideoService, { CameraFacing, VideoQuality } from "../services/VideoService";
import CameraPermissionService from "../services/CameraPermissionService";
import TranslationService from "../services/translation/TranslationService";
import ParallelProcessor from "../services/translation/utils/ParallelProcessor";
import { useSubtitleStore } from "../store/useSubtitleStore";
import { E2EEKeyExchange, type E2EEKeyExchangeCallbacks, type KeyExchangeRole } from "../services/e2ee/E2EEKeyExchange";
import type { E2EESessionKey } from "../services/e2ee/E2EEService";

type CallDirection = "incoming" | "outgoing";

type SessionDescriptionPayload = RTCSessionDescriptionInit;

type IceCandidatePayload = RTCIceCandidateInit & { iceEpoch?: number };

// RTCIceServer 类型定义
interface RTCIceServer {
  urls: string | string[];
  username?: string;
  credential?: string;
}

const preferRestrictedIceServers = (servers: RTCIceServer[]) => {
  if (!RESTRICTED_NETWORK_MODE) return servers;

  const urlsOf = (srv: RTCIceServer) => (Array.isArray(srv.urls) ? srv.urls : [srv.urls]);
  const scoreUrl = (url: string) => {
    const lower = url.toLowerCase();
    if (lower.startsWith("turns:")) {
      if (lower.includes("transport=tcp")) return 0;
      return 1;
    }
    if (lower.startsWith("turn:")) {
      if (lower.includes("transport=tcp")) return 2;
      return 3;
    }
    if (lower.startsWith("stun:")) return 4;
    return 5;
  };

  const scoreServer = (srv: RTCIceServer) => Math.min(...urlsOf(srv).map(scoreUrl));
  return [...servers].sort((a, b) => scoreServer(a) - scoreServer(b));
};

interface CallSession {
  callId: string;
  peerEmail: string;
  direction: CallDirection;
  offer?: SessionDescriptionPayload;
}

type CallStatus = "idle" | "connecting" | "incoming" | "in_call";

export type NetworkQuality = "excellent" | "good" | "poor" | "bad" | "unknown";

interface SignalingContextValue {
  status: CallStatus;
  session: CallSession | null;
  connectionReady: boolean;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  networkQuality: NetworkQuality;
  // 视频通话相关状态
  isVideoEnabled: boolean;
  isAudioEnabled: boolean;
  isRemoteVideoEnabled: boolean;
  isRemoteAudioEnabled: boolean;
  cameraFacing: CameraFacing;
  // E2EE 端到端加密状态
  e2eeEnabled: boolean;
  e2eeFingerprint: string | null;
  e2eeSessionEstablished: boolean;
  // 翻译功能相关状态
  translationEnabled: boolean;
  translationLanguage: string;
  // 通话控制函数
  startCall: (email: string) => Promise<void>;
  acceptCall: () => Promise<void>;
  rejectCall: () => void;
  endCall: () => void;
  // 媒体控制函数
  toggleVideo: () => Promise<void>;
  toggleAudio: () => void;
  switchCamera: () => Promise<void>;
  toggleSpeaker: () => Promise<void>;
  isSpeakerOn: boolean;

  // Video bitrate/quality controls
  videoQuality: VideoQuality;
  setVideoQuality: (quality: VideoQuality) => void;
  videoMaxBitrateKbps: number;
  setVideoMaxBitrateKbps: (kbps: number) => void;
  // 翻译控制函数
  toggleTranslation: (enabled: boolean) => Promise<void>;
  setTranslationLanguage: (language: string) => void;
}

const SignalingContext = createContext<SignalingContextValue | undefined>(
  undefined
);

const DEFAULT_ICE_SERVERS: RTCIceServer[] = [
  { urls: "stun:stun.l.google.com:19302" },
  { urls: "stun:stun1.l.google.com:19302" },
  {
    urls: "turn:openrelay.metered.ca:80",
    username: "openrelayproject",
    credential: "openrelayproject"
  },
  {
    urls: "turn:openrelay.metered.ca:443",
    username: "openrelayproject",
    credential: "openrelayproject"
  },
  {
    urls: "turn:openrelay.metered.ca:443?transport=tcp",
    username: "openrelayproject",
    credential: "openrelayproject"
  }
];

const VIDEO_QUALITY_PRESETS: Record<VideoQuality, { width: number; height: number; frameRate: number }> = {
  low: { width: 320, height: 240, frameRate: 15 },
  medium: { width: 640, height: 480, frameRate: 24 },
  high: { width: 1280, height: 720, frameRate: 30 }
};

const isSessionDescriptionPayload = (
  value: unknown
): value is SessionDescriptionPayload => {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { sdp?: unknown }).sdp === "string" &&
    typeof (value as { type?: unknown }).type === "string"
  );
};

const isIceCandidatePayload = (
  value: unknown
): value is IceCandidatePayload => {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { candidate?: unknown }).candidate === "string"
  );
};

export const SignalingProvider: React.FC<{ children: React.ReactNode }> = ({
  children
}) => {
  const { token, user } = useAuthContext();
  const { settings } = useSettings();
  const [status, setStatus] = useState<CallStatus>("idle");
  const [session, setSession] = useState<CallSession | null>(null);
  const [connectionReady, setConnectionReady] = useState(false);
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [iceServers, setIceServers] = useState<RTCIceServer[]>(DEFAULT_ICE_SERVERS);

  const [isVideoEnabled, setIsVideoEnabled] = useState<boolean>(false);
  const [isAudioEnabled, setIsAudioEnabled] = useState<boolean>(true);
  const [isRemoteVideoEnabled, setIsRemoteVideoEnabled] = useState<boolean>(true);
  const [isRemoteAudioEnabled, setIsRemoteAudioEnabled] = useState<boolean>(true);
  const [cameraFacing, setCameraFacing] = useState<CameraFacing>("front");
  const [isSpeakerOn, setIsSpeakerOn] = useState<boolean>(false);
  const [networkQuality, setNetworkQuality] = useState<NetworkQuality>("unknown");

  const [videoQuality, setVideoQuality] = useState<VideoQuality>("medium");
  const [videoMaxBitrateKbps, setVideoMaxBitrateKbps] = useState<number>(900);
  const videoAdaptiveBitrateEnabledRef = useRef<boolean>(false);

  const [translationEnabled, setTranslationEnabled] = useState<boolean>(false);
  const [translationLanguage, setTranslationLanguage] = useState<string>("zh");
  const processorRef = useRef<ParallelProcessor | null>(null);

  const [e2eeEnabled, setE2eeEnabled] = useState<boolean>(false);
  const [e2eeFingerprint, setE2eeFingerprint] = useState<string | null>(null);
  const [e2eeSessionEstablished, setE2eeSessionEstablished] = useState<boolean>(false);
  const e2eeKeyExchangeRef = useRef<E2EEKeyExchange | null>(null);
  const e2eeDataChannelRef = useRef<any | null>(null);

  type SignalingEvents = {
    open: undefined;
    close: { code: number; reason?: string };
    message: SignalMessage;
    error: Error;
  };

  interface SignalingTransport {
    connect: () => void;
    disconnect: () => void;
    on<T extends keyof SignalingEvents>(
      event: T,
      handler: (value: SignalingEvents[T]) => void
    ): void;
    send: (message: SignalMessage) => boolean;
  }

  const signalingRef = useRef<SignalingTransport | null>(null);
  const peerRef = useRef<RTCPeerConnection | null>(null);
  const sessionRef = useRef<CallSession | null>(null);
  const statusRef = useRef<CallStatus>("idle");
  const isAudioEnabledRef = useRef<boolean>(true);
  const isVideoEnabledRef = useRef<boolean>(false);
  const videoMaxBitrateKbpsRef = useRef<number>(900);
  const pendingTarget = useRef<string | null>(null);
  const pendingLocalCandidates = useRef<IceCandidatePayload[]>([]);
  const pendingRemoteCandidates = useRef<IceCandidatePayload[]>([]);
  const subtitlesDataChannelRef = useRef<any | null>(null);

  const iceEpochRef = useRef<number>(0);
  const iceRestartAttemptsRef = useRef<number>(0);
  const restartTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const disconnectDeadlineRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const remoteVideoLastBytesRef = useRef<number | null>(null);
  const remoteVideoStallCountRef = useRef<number>(0);
  const remoteVideoLastRestartAtMsRef = useRef<number>(0);

  useEffect(() => {
    setIsSpeakerOn(AudioService.getSpeakerphone());
  }, []);

  useEffect(() => {
    sessionRef.current = session;
    statusRef.current = status;
    isAudioEnabledRef.current = isAudioEnabled;
    isVideoEnabledRef.current = isVideoEnabled;
    videoMaxBitrateKbpsRef.current = videoMaxBitrateKbps;
  }, [session, status, isAudioEnabled, isVideoEnabled, videoMaxBitrateKbps]);

  useEffect(() => {
    let cancelled = false;
    const loadIceServers = async () => {
      if (!token) {
        setIceServers(DEFAULT_ICE_SERVERS);
        return;
      }
      try {
        const config = await fetchWebRTCConfig(token);
        const servers = Array.isArray(config.ice_servers) ? config.ice_servers : [];
        if (!cancelled && servers.length) {
          setIceServers(preferRestrictedIceServers(servers as RTCIceServer[]));
        } else if (!cancelled) {
          setIceServers(DEFAULT_ICE_SERVERS);
        }
      } catch (error) {
        if (!cancelled) setIceServers(DEFAULT_ICE_SERVERS);
      }
    };
    loadIceServers();
    return () => { cancelled = true; };
  }, [token]);

  useEffect(() => {
    switch (status) {
      case "incoming":
        if (settings.audioNotificationsEnabled) AudioService.play("incoming_call");
        if (settings.vibrationEnabled) VibrationService.vibrate("incoming_call");
        break;
      case "connecting":
        if (settings.audioNotificationsEnabled) AudioService.play("ringback");
        if (settings.vibrationEnabled) VibrationService.vibrate("ringback");
        break;
      case "in_call":
        AudioService.stopAll();
        VibrationService.cancel();
        if (settings.vibrationEnabled) VibrationService.vibrate("call_connected");
        break;
      case "idle":
        AudioService.stopAll();
        VibrationService.cancel();
        break;
    }
  }, [status, session, settings.audioNotificationsEnabled, settings.vibrationEnabled]);

  const ensureAudioPermission = useCallback(async () => {
    if (Platform.OS === "android") {
      try {
        const permissions: string[] = [PermissionsAndroid.PERMISSIONS.RECORD_AUDIO];
        if (Platform.Version >= 31) permissions.push(PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT);
        permissions.push(PermissionsAndroid.PERMISSIONS.CAMERA);
        const result = await PermissionsAndroid.requestMultiple(permissions as any);
        return permissions.every((p) => (result as any)[p] === PermissionsAndroid.RESULTS.GRANTED);
      } catch (error) {
        return false;
      }
    }
    return true;
  }, []);

  const resetPeerResources = useCallback(() => {
    pendingLocalCandidates.current = [];
    pendingRemoteCandidates.current = [];
    iceEpochRef.current = 0;
    iceRestartAttemptsRef.current = 0;
    remoteVideoLastBytesRef.current = null;
    remoteVideoStallCountRef.current = 0;
    remoteVideoLastRestartAtMsRef.current = 0;
    if (restartTimerRef.current) clearTimeout(restartTimerRef.current);
    if (disconnectDeadlineRef.current) clearTimeout(disconnectDeadlineRef.current);

    if (peerRef.current) {
      (peerRef.current as any).onicecandidate = null;
      (peerRef.current as any).ontrack = null;
      (peerRef.current as any).onconnectionstatechange = null;
      peerRef.current.close();
      peerRef.current = null;
    }
    if (subtitlesDataChannelRef.current) {
      try { subtitlesDataChannelRef.current.close(); } catch (e) {}
      subtitlesDataChannelRef.current = null;
    }
    if (e2eeDataChannelRef.current) {
      try { e2eeDataChannelRef.current.close(); } catch (e) {}
      e2eeDataChannelRef.current = null;
    }
    if (e2eeKeyExchangeRef.current) {
      e2eeKeyExchangeRef.current.destroy();
      e2eeKeyExchangeRef.current = null;
    }
    setE2eeEnabled(false);
    setE2eeFingerprint(null);
    setE2eeSessionEstablished(false);
    if (localStream) localStream.getTracks().forEach((track) => track.stop());
    if (remoteStream) remoteStream.getTracks().forEach((track) => track.stop());
    setLocalStream(null);
    setRemoteStream(null);
    setIsRemoteVideoEnabled(true);
    setIsRemoteAudioEnabled(true);
  }, [localStream, remoteStream]);

  const attachSubtitlesDataChannel = useCallback((dc: any) => {
    if (!dc) return;
    subtitlesDataChannelRef.current = dc;
    dc.onmessage = (event: any) => {
      try {
        const parsed = JSON.parse(String(event?.data ?? ''));
        if (!parsed || parsed.t !== 'subtitle') return;
        const ts = typeof parsed.timestampMs === 'number' ? parsed.timestampMs : Date.now();
        useSubtitleStore.getState().addSubtitle({
          id: `subtitle-${ts}`,
          original: typeof parsed.originalText === 'string' ? parsed.originalText : '',
          translated: typeof parsed.translatedText === 'string' ? parsed.translatedText : '',
          timestamp: ts,
        });
      } catch (e) {}
    };
  }, []);

  const initializeE2EEKeyExchange = useCallback((role: KeyExchangeRole) => {
    const current = sessionRef.current;
    if (!current) return;

    const callbacks: E2EEKeyExchangeCallbacks = {
      onSessionEstablished: (session: E2EESessionKey) => {
        setE2eeFingerprint(session.fingerprint);
        setE2eeSessionEstablished(true);
        console.log(`[E2EE] Session established, fingerprint: ${session.fingerprint.slice(0, 16)}...`);
      },
      onError: (error: Error) => {
        console.error("[E2EE] Key exchange error:", error);
        setE2eeEnabled(false);
        Alert.alert("E2EE Error", `Failed to establish encrypted session: ${error.message}`);
      }
    };

    const keyExchange = new E2EEKeyExchange(role, current.callId, callbacks);
    e2eeKeyExchangeRef.current = keyExchange;
    setE2eeEnabled(true);

    keyExchange.initialize().then(() => {
      if (e2eeDataChannelRef.current) {
        keyExchange.attachDataChannel(e2eeDataChannelRef.current);
        if (role === "initiator") {
          keyExchange.sendPublicKey();
        }
      }
    });
  }, []);

  const sendMessage = useCallback((message: SignalMessage) => {
    const client = signalingRef.current;
    if (!client) return;
    try { client.send(message); } catch (error) {
      if (message.type !== "ice.candidate") {
        Alert.alert("错误 / Connection Issue", "无法发送信令消息 / Failed to send signaling message.");
      }
    }
  }, []);

  const resetCallState = useCallback(() => {
    pendingTarget.current = null;
    setSession(null);
    setStatus("idle");
    resetPeerResources();
  }, [resetPeerResources]);

  const rejectCall = useCallback(() => {
    if (!session) return;
    sendMessage({ type: "call.reject", call_id: session.callId, to: session.peerEmail });
    resetCallState();
  }, [resetCallState, sendMessage, session]);

  const endCall = useCallback(() => {
    if (!session) return;
    sendMessage({ type: "call.end", call_id: session.callId, to: session.peerEmail });
    resetCallState();
  }, [resetCallState, sendMessage, session]);

  const sendMediaUpdate = useCallback(
    (callId: string, peerEmail: string, update: MediaUpdatePayload) => {
      sendMessage({ type: "call.media_update", call_id: callId, to: peerEmail, payload: update });
    },
    [sendMessage]
  );

  const setVideoSenderMaxBitrate = useCallback(async (kbps: number) => {
    const pc = peerRef.current;
    if (!pc) return;
    const sender = pc.getSenders().find((s: any) => s?.track?.kind === "video");
    if (!sender) return;
    try {
      const params = sender.getParameters();
      if (!params.encodings || params.encodings.length === 0) params.encodings = [{ active: true }];
      params.encodings[0].maxBitrate = kbps * 1000;
      await sender.setParameters(params);
    } catch (e) {}
  }, []);

  const applyCurrentVideoBitrate = useCallback(async () => {
    const kbps = videoMaxBitrateKbpsRef.current;
    if (kbps > 0) await setVideoSenderMaxBitrate(kbps);
  }, [setVideoSenderMaxBitrate]);

  useEffect(() => {
    if (settings?.videoQuality) setVideoQuality(settings.videoQuality);
    if (typeof settings?.videoMaxBitrateKbps === "number") {
      setVideoMaxBitrateKbps(Math.max(100, Math.min(2500, Math.trunc(settings.videoMaxBitrateKbps))));
    }
    videoAdaptiveBitrateEnabledRef.current = !!settings?.videoAdaptiveBitrateEnabled;
  }, [settings]);

  const requestIceRestart = useCallback((callId: string, peerEmail: string, reason: string) => {
    sendMessage({ type: "call.ice-restart.request", call_id: callId, to: peerEmail, payload: { reason, iceEpoch: iceEpochRef.current } });
  }, [sendMessage]);

  const startIceRestartAsCaller = useCallback(async () => {
    const current = sessionRef.current;
    const pc = peerRef.current;
    if (!current || !pc || current.direction !== "outgoing" || iceRestartAttemptsRef.current >= 2) return;
    iceRestartAttemptsRef.current += 1;
    iceEpochRef.current += 1;
    pendingRemoteCandidates.current = [];
    try {
      const offer = await pc.createOffer({ iceRestart: true } as any);
      await pc.setLocalDescription(offer);
      sendMessage({ type: "call.sdp.offer", call_id: current.callId, to: current.peerEmail, payload: { sdp: offer.sdp, type: offer.type, iceEpoch: iceEpochRef.current } as SdpRenegotiationPayload });
    } catch (e) {}
  }, [sendMessage]);

  useEffect(() => {
    if (status !== "in_call") return;
    const pc = peerRef.current;
    if (!pc) return;
    let lastAppliedKbps: number | null = null;
    const timer = setInterval(async () => {
      if (statusRef.current !== "in_call" || !isVideoEnabledRef.current || !videoAdaptiveBitrateEnabledRef.current) return;
      try {
        const report = await pc.getStats();
        let availableBps: number | null = null;
        let currentRtt: number | null = null;
        report.forEach((stat: any) => {
          if (stat.type === "candidate-pair" && (stat.selected || stat.nominated)) {
            availableBps = stat.availableOutgoingBitrate;
            currentRtt = stat.currentRoundTripTime;
          }
        });
        if (currentRtt !== null) {
          if (currentRtt < 0.1) setNetworkQuality("excellent");
          else if (currentRtt < 0.3) setNetworkQuality("good");
          else if (currentRtt < 0.5) setNetworkQuality("poor");
          else setNetworkQuality("bad");
        }
        if (availableBps) {
          const userMaxKbps = videoMaxBitrateKbpsRef.current;
          const targetKbps = Math.max(100, Math.min(userMaxKbps, (availableBps * 0.85) / 1000));
          if (lastAppliedKbps === null || Math.abs(targetKbps - lastAppliedKbps) / lastAppliedKbps > 0.1) {
            lastAppliedKbps = targetKbps;
            await setVideoSenderMaxBitrate(targetKbps);
          }
        }
      } catch (e) {}
    }, 4000);
    return () => clearInterval(timer);
  }, [setVideoSenderMaxBitrate, status]);

  const createPeerConnection = useCallback(() => {
    const pc = new RTCPeerConnection({ iceServers, bundlePolicy: "max-bundle" } as any);
    (pc as any).onicecandidate = (event: any) => {
      if (!event.candidate) return;
      const candidateInit = { candidate: event.candidate.candidate, sdpMid: event.candidate.sdpMid, sdpMLineIndex: event.candidate.sdpMLineIndex, iceEpoch: iceEpochRef.current };
      const current = sessionRef.current;
      if (current?.callId) sendMessage({ type: "ice.candidate", call_id: current.callId, to: current.peerEmail, payload: candidateInit as any });
      else pendingLocalCandidates.current.push(candidateInit);
    };
    (pc as any).oniceconnectionstatechange = () => {
      const current = sessionRef.current;
      if (pc.iceConnectionState === "failed" && current && statusRef.current === "in_call") {
        if (current.direction === "outgoing") startIceRestartAsCaller();
        else requestIceRestart(current.callId, current.peerEmail, "ice_failed");
      }
    };
    (pc as any).ontrack = (event: any) => {
      try {
        const streams = Array.isArray(event?.streams) ? event.streams : [];
        let stream: MediaStream | null = streams[0] ?? null;

        // Some devices/transports do not populate event.streams; build a fallback.
        if (!stream && event?.track) {
          const fallbackStream = new MediaStream();
          fallbackStream.addTrack(event.track);
          stream = fallbackStream;
        }

        if (!stream) {
          console.warn("[SignalingContext] ontrack received no stream/track");
          return;
        }

        setRemoteStream(stream);

        const bindTrackState = (track: any) => {
          if (!track || typeof track.kind !== "string") return;
          track.onmute = () => {
            if (track.kind === "video") setIsRemoteVideoEnabled(false);
            if (track.kind === "audio") setIsRemoteAudioEnabled(false);
          };
          track.onunmute = () => {
            if (track.kind === "video") setIsRemoteVideoEnabled(true);
            if (track.kind === "audio") setIsRemoteAudioEnabled(true);
          };
        };

        stream.getTracks().forEach(bindTrackState);
        if (event?.track) bindTrackState(event.track);
      } catch (error) {
        console.error("[SignalingContext] ontrack handler failed:", error);
      }
    };
    (pc as any).ondatachannel = (event: any) => {
      if (event.channel?.label === 'subtitles') {
        attachSubtitlesDataChannel(event.channel);
      } else if (event.channel?.label === 'e2ee-key-exchange') {
        e2eeDataChannelRef.current = event.channel;
        event.channel.onopen = () => {
          console.log("[E2EE] Data Channel opened (responder)");
          initializeE2EEKeyExchange("responder");
        };
      }
    };
    (pc as any).onconnectionstatechange = () => {
      const state = pc.connectionState;
      const current = sessionRef.current;
      if ((state === "failed" || state === "closed" || state === "disconnected") && current && statusRef.current === "in_call") {
        if (restartTimerRef.current) clearTimeout(restartTimerRef.current);
        restartTimerRef.current = setTimeout(() => {
          if (current.direction === "outgoing") startIceRestartAsCaller();
          else requestIceRestart(current.callId, current.peerEmail, "connection_lost");
        }, state === "disconnected" ? 1500 : 0);
        if (!disconnectDeadlineRef.current) disconnectDeadlineRef.current = setTimeout(resetCallState, 10000);
      } else if (state === "connected") {
        if (restartTimerRef.current) { clearTimeout(restartTimerRef.current); restartTimerRef.current = null; }
        if (disconnectDeadlineRef.current) { clearTimeout(disconnectDeadlineRef.current); disconnectDeadlineRef.current = null; }
      }
    };
    peerRef.current = pc;
    return pc;
  }, [iceServers, resetCallState, sendMessage, attachSubtitlesDataChannel, startIceRestartAsCaller, requestIceRestart]);

  useEffect(() => {
    if (!token) {
      signalingRef.current?.disconnect();
      signalingRef.current = null;
      setConnectionReady(false);
      resetCallState();
      return;
    }
    const client = new SignalingClient(token);
    const pollingClient = new PollingSignalingClient(token);
    const transport: SignalingTransport =
      SIGNALING_TRANSPORT_MODE === "poll" ? pollingClient : client;
    signalingRef.current = transport;
    transport.connect();
    transport.on("open", () => setConnectionReady(true));
    transport.on("close", () => { setConnectionReady(false); resetCallState(); });
    transport.on("message", async (msg: SignalMessage) => {
      try {
        switch (msg.type) {
          case "call.invite.ack":
            if (pendingTarget.current) {
              const sess = { callId: msg.call_id ?? "", peerEmail: pendingTarget.current, direction: "outgoing" as CallDirection };
              setSession(sess); setStatus("connecting");
              pendingLocalCandidates.current.forEach(c => sendMessage({ type: "ice.candidate", call_id: sess.callId, to: sess.peerEmail, payload: c as any }));
              pendingLocalCandidates.current = [];
              pendingTarget.current = null;
            }
            break;
          case "call.invite":
            setSession({ callId: msg.call_id ?? "", peerEmail: msg.from ?? "", direction: "incoming", offer: msg.payload as any });
            setStatus("incoming");
            break;
          case "call.accept":
            if (peerRef.current && (msg.payload as any)?.sdp) {
              await peerRef.current.setRemoteDescription(new RTCSessionDescription(msg.payload as any));
              while (pendingRemoteCandidates.current.length) {
                const c = pendingRemoteCandidates.current.shift();
                if (c && c.iceEpoch === iceEpochRef.current) await peerRef.current.addIceCandidate(new RTCIceCandidate(c));
              }
            }
            setStatus("in_call");
            sendMediaUpdate(msg.call_id ?? "", msg.from ?? "", { audioEnabled: isAudioEnabledRef.current, videoEnabled: isVideoEnabledRef.current });
            break;
          case "call.media_update":
            const p = msg.payload as any;
            if (typeof p?.videoEnabled === "boolean") setIsRemoteVideoEnabled(p.videoEnabled);
            if (typeof p?.audioEnabled === "boolean") setIsRemoteAudioEnabled(p.audioEnabled);
            break;
          case "call.ice-restart.request":
            if (statusRef.current === "in_call" && sessionRef.current?.direction === "outgoing") startIceRestartAsCaller();
            break;
          case "call.sdp.offer":
            if (peerRef.current && statusRef.current === "in_call") {
              const payload = msg.payload as any;
              if ((payload.iceEpoch ?? 0) > iceEpochRef.current) { iceEpochRef.current = payload.iceEpoch; pendingRemoteCandidates.current = []; }
              await peerRef.current.setRemoteDescription(new RTCSessionDescription(payload));
              const answer = await peerRef.current.createAnswer();
              await peerRef.current.setLocalDescription(answer);
              sendMessage({ type: "call.sdp.answer", call_id: sessionRef.current?.callId ?? "", to: msg.from ?? "", payload: { sdp: answer.sdp, type: answer.type, iceEpoch: iceEpochRef.current } as any });
            }
            break;
          case "call.sdp.answer":
            if (peerRef.current && statusRef.current === "in_call") await peerRef.current.setRemoteDescription(new RTCSessionDescription(msg.payload as any));
            break;
          case "call.reject":
          case "call.end":
            Alert.alert("Call " + (msg.type === "call.reject" ? "rejected" : "ended"), `${msg.from ?? "Peer"} ${msg.type === "call.reject" ? "declined" : "ended"} the call.`);
            resetCallState();
            break;
          case "ice.candidate":
            const cand = msg.payload as any;
            if (peerRef.current?.remoteDescription) await peerRef.current.addIceCandidate(new RTCIceCandidate(cand));
            else pendingRemoteCandidates.current.push(cand);
            break;
          case "call.error":
            Alert.alert("Call error", (msg.payload as any)?.reason ?? "Error");
            resetCallState();
            break;
        }
      } catch (error) {
        console.error("[SignalingContext] failed to handle signaling message:", msg.type, error);
        if (msg.type !== "ice.candidate") {
          Alert.alert("Call error", "通话状态异常，已自动重置通话。");
        }
        resetCallState();
      }
    });
    return () => { transport.disconnect(); signalingRef.current = null; };
  }, [resetCallState, sendMessage, token]);

  const startCall = useCallback(async (email: string) => {
    if (status !== "idle") return;
    try {
      const perms = await ensureAudioPermission();
      if (!perms) return;
      resetPeerResources();
      await VideoService.initialize();
      const stream = await VideoService.getLocalStream(settings.defaultAudioEnabled, settings.defaultVideoEnabled, settings.cameraFacing, settings.videoQuality ?? "medium");
      if (!stream) throw new Error("No stream");
      setLocalStream(stream); setIsVideoEnabled(settings.defaultVideoEnabled); setIsAudioEnabled(settings.defaultAudioEnabled); setCameraFacing(settings.cameraFacing);
      const pc = createPeerConnection();
      if (stream.getVideoTracks().length === 0) pc.addTransceiver("video", { direction: "sendrecv" });
      const dc = (pc as any).createDataChannel?.('subtitles', { ordered: true });
      if (dc) attachSubtitlesDataChannel(dc);
      const e2eeDc = (pc as any).createDataChannel?.('e2ee-key-exchange', { ordered: true });
      if (e2eeDc) {
        e2eeDataChannelRef.current = e2eeDc;
        e2eeDc.onopen = () => {
          console.log("[E2EE] Data Channel opened (initiator)");
          initializeE2EEKeyExchange("initiator");
        };
      }
      stream.getTracks().forEach(t => pc.addTrack(t, stream));
      const offer = await pc.createOffer({ offerToReceiveAudio: true, offerToReceiveVideo: true });
      await pc.setLocalDescription(offer);
      if (settings.defaultVideoEnabled) await applyCurrentVideoBitrate();
      pendingTarget.current = email; setStatus("connecting");
      sendMessage({ type: "call.invite", to: email, payload: { sdp: offer.sdp, type: offer.type } });
    } catch (e) { resetCallState(); }
  }, [ensureAudioPermission, resetPeerResources, settings, createPeerConnection, attachSubtitlesDataChannel, applyCurrentVideoBitrate, sendMessage, resetCallState, status]);

  const acceptCall = useCallback(async () => {
    if (!session || session.direction !== "incoming" || !session.offer) return;
    try {
      const perms = await ensureAudioPermission();
      if (!perms) return;
      await VideoService.initialize();
      const stream = await VideoService.getLocalStream(settings.defaultAudioEnabled, settings.defaultVideoEnabled, settings.cameraFacing, settings.videoQuality ?? "medium");
      if (!stream) throw new Error("No stream");
      setLocalStream(stream); setIsVideoEnabled(settings.defaultVideoEnabled); setIsAudioEnabled(settings.defaultAudioEnabled); setCameraFacing(settings.cameraFacing);
      const pc = createPeerConnection();
      stream.getTracks().forEach(t => pc.addTrack(t, stream));
      await pc.setRemoteDescription(new RTCSessionDescription(session.offer as any));
      const answer = await pc.createAnswer();
      await pc.setLocalDescription(answer);
      if (settings.defaultVideoEnabled) await applyCurrentVideoBitrate();
      sendMessage({ type: "call.accept", call_id: session.callId, to: session.peerEmail, payload: { sdp: answer.sdp, type: answer.type } });
      sendMediaUpdate(session.callId, session.peerEmail, { audioEnabled: settings.defaultAudioEnabled, videoEnabled: settings.defaultVideoEnabled });
      setStatus("in_call");
    } catch (e) { resetCallState(); }
  }, [session, ensureAudioPermission, settings, createPeerConnection, applyCurrentVideoBitrate, sendMessage, sendMediaUpdate, resetCallState]);

  const toggleVideo = useCallback(async () => {
    if (!localStream || !peerRef.current) return;
    const next = !isVideoEnabled;
    try {
      if (next) {
        const perms = await CameraPermissionService.checkPermissions();
        if (!perms.camera) return;
        const videoStream = await webrtcMediaDevices.getUserMedia({ audio: false, video: { facingMode: cameraFacing === "front" ? "user" : "environment" } });
        const track = videoStream.getVideoTracks()[0];
        const tx = peerRef.current.getTransceivers().find(t => t.receiver?.track?.kind === "video");
        if (tx) {
          await tx.sender.replaceTrack(track);
          const nextStream = new MediaStream(); localStream.getAudioTracks().forEach(t => nextStream.addTrack(t)); nextStream.addTrack(track);
          setLocalStream(nextStream); setIsVideoEnabled(true); await applyCurrentVideoBitrate();
          localStream.getVideoTracks().forEach(t => t.stop());
          if (sessionRef.current) sendMediaUpdate(sessionRef.current.callId, sessionRef.current.peerEmail, { audioEnabled: isAudioEnabledRef.current, videoEnabled: true });
        }
      } else {
        const tx = peerRef.current.getTransceivers().find(t => t.receiver?.track?.kind === "video");
        if (tx) await tx.sender.replaceTrack(null);
        localStream.getVideoTracks().forEach(t => t.stop());
        const nextStream = new MediaStream(); localStream.getAudioTracks().forEach(t => nextStream.addTrack(t));
        setLocalStream(nextStream); setIsVideoEnabled(false);
        if (sessionRef.current) sendMediaUpdate(sessionRef.current.callId, sessionRef.current.peerEmail, { audioEnabled: isAudioEnabledRef.current, videoEnabled: false });
      }
    } catch (e) {}
  }, [localStream, isVideoEnabled, cameraFacing, applyCurrentVideoBitrate, sendMediaUpdate]);

  const toggleAudio = useCallback(() => {
    const next = !isAudioEnabled;
    VideoService.toggleAudioTrack(next); setIsAudioEnabled(next);
    if (sessionRef.current && status === "in_call") sendMediaUpdate(sessionRef.current.callId, sessionRef.current.peerEmail, { audioEnabled: next, videoEnabled: isVideoEnabled });
  }, [isAudioEnabled, isVideoEnabled, status, sendMediaUpdate]);

  const switchCamera = useCallback(async () => {
    if (!localStream || !isVideoEnabled || !peerRef.current) return;
    const nextFacing: CameraFacing = cameraFacing === "front" ? "back" : "front";
    try {
      const videoStream = await webrtcMediaDevices.getUserMedia({ audio: false, video: { facingMode: nextFacing === "front" ? "user" : "environment" } });
      const track = videoStream.getVideoTracks()[0];
      const tx = peerRef.current.getTransceivers().find(t => t.receiver?.track?.kind === "video");
      if (tx) {
        await tx.sender.replaceTrack(track);
        const nextStream = new MediaStream(); localStream.getAudioTracks().forEach(t => nextStream.addTrack(t)); nextStream.addTrack(track);
        setLocalStream(nextStream); setCameraFacing(nextFacing); await applyCurrentVideoBitrate();
        localStream.getVideoTracks().forEach(t => t.stop());
      }
    } catch (e) {}
  }, [localStream, isVideoEnabled, cameraFacing, applyCurrentVideoBitrate]);

  const toggleSpeaker = useCallback(async () => {
    const next = !isSpeakerOn; setIsSpeakerOn(next); await AudioService.setSpeakerphone(next);
  }, [isSpeakerOn]);

  const toggleTranslation = useCallback(async (enabled: boolean) => {
    try {
      setTranslationEnabled(enabled);
      if (!enabled) {
        useSubtitleStore.getState().clearSubtitles();
        await TranslationService.stopWebRTCCallMicTranslation();
        if (processorRef.current) { processorRef.current.stopProcessing(); processorRef.current = null; }
      }
    } catch (e) {}
  }, []);

  useEffect(() => {
    let cancelled = false;
    if (translationEnabled && status === 'in_call' && TranslationService.isReady() && sessionRef.current?.peerEmail) {
      TranslationService.startWebRTCCallMicTranslation(translationLanguage, (res) => {
        if (cancelled) return;
        const dc = subtitlesDataChannelRef.current;
        if (dc?.readyState === 'open') dc.send(JSON.stringify({ t: 'subtitle', originalText: res.originalText, translatedText: res.translatedText, timestampMs: res.timestampMs }));
      }).catch(() => {});
    }
    return () => { cancelled = true; TranslationService.stopWebRTCCallMicTranslation().catch(() => {}); };
  }, [translationEnabled, status, translationLanguage]);

  const value = useMemo<SignalingContextValue>(() => ({
    status, session, connectionReady, localStream, remoteStream, networkQuality, isVideoEnabled, isAudioEnabled, isRemoteVideoEnabled, isRemoteAudioEnabled, cameraFacing,
    videoQuality, setVideoQuality, videoMaxBitrateKbps, setVideoMaxBitrateKbps, e2eeEnabled, e2eeFingerprint, e2eeSessionEstablished, translationEnabled, translationLanguage,
    startCall, acceptCall, rejectCall, endCall, toggleVideo, toggleAudio, switchCamera, toggleSpeaker, isSpeakerOn, toggleTranslation, setTranslationLanguage: setTranslationLanguage
  }), [status, session, connectionReady, localStream, remoteStream, networkQuality, isVideoEnabled, isAudioEnabled, isRemoteVideoEnabled, isRemoteAudioEnabled, cameraFacing,
    videoQuality, videoMaxBitrateKbps, e2eeEnabled, e2eeFingerprint, e2eeSessionEstablished, translationEnabled, translationLanguage, startCall, acceptCall, rejectCall, endCall, toggleVideo, toggleAudio, switchCamera, toggleSpeaker, isSpeakerOn, toggleTranslation]);

  return <SignalingContext.Provider value={value}>{children}</SignalingContext.Provider>;
};

export const useSignaling = () => {
  const ctx = useContext(SignalingContext);
  if (!ctx) throw new Error("useSignaling must be used within SignalingProvider");
  return ctx;
};
