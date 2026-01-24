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
import { fetchWebRTCConfig } from "../api/webrtc";
import { useAuthContext } from "./AuthContext";
import { useSettings } from "./SettingsContext";
import AudioService from "../services/AudioServiceExpo";
import VibrationService from "../services/VibrationService";
import VideoService, { CameraFacing, VideoQuality } from "../services/VideoService";
import CameraPermissionService from "../services/CameraPermissionService";
import TranslationService from "../services/translation/TranslationService";
import ParallelProcessor from "../services/translation/utils/ParallelProcessor";
import { SubtitleItem } from "../components/translation/TranslationOverlay";

type CallDirection = "incoming" | "outgoing";

type SessionDescriptionPayload = RTCSessionDescriptionInit;

type IceCandidatePayload = RTCIceCandidateInit & { iceEpoch?: number };

// RTCIceServer 类型定义
interface RTCIceServer {
  urls: string | string[];
  username?: string;
  credential?: string;
}

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
  // 翻译功能相关状态
  translationEnabled: boolean;
  translationLanguage: string;
  subtitles: SubtitleItem[];
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
  clearSubtitles: () => void;
}

const SignalingContext = createContext<SignalingContextValue | undefined>(
  undefined
);

const DEFAULT_ICE_SERVERS: RTCIceServer[] = [
  // Google STUN servers (免费，用于获取公网 IP)
  { urls: "stun:stun.l.google.com:19302" },
  { urls: "stun:stun1.l.google.com:19302" },

  // OpenRelay TURN 服务器 (免费公共服务，用于 NAT 穿透)
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
  },

  // Metered.ca 免费 TURN 服务器备用
  {
    urls: "turn:a.relay.metered.ca:80",
    username: "e8dd65d92c62e9f15c0165f4",
    credential: "uWdWNmkhvyqTmFWr"
  },
  {
    urls: "turn:a.relay.metered.ca:443",
    username: "e8dd65d92c62e9f15c0165f4",
    credential: "uWdWNmkhvyqTmFWr"
  },
  {
    urls: "turn:a.relay.metered.ca:443?transport=tcp",
    username: "e8dd65d92c62e9f15c0165f4",
    credential: "uWdWNmkhvyqTmFWr"
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

  // 视频通话状态
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

  // 翻译功能状态
  const [translationEnabled, setTranslationEnabled] = useState<boolean>(false);
  const [translationLanguage, setTranslationLanguage] = useState<string>("zh");
  const [subtitles, setSubtitles] = useState<SubtitleItem[]>([]);
  const processorRef = useRef<ParallelProcessor | null>(null);

  const signalingRef = useRef<SignalingClient | null>(null);
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
    // Sync initial speaker state
    setIsSpeakerOn(AudioService.getSpeakerphone());
  }, []);

  useEffect(() => {
    sessionRef.current = session;
  }, [session]);

  useEffect(() => {
    statusRef.current = status;
  }, [status]);

  useEffect(() => {
    isAudioEnabledRef.current = isAudioEnabled;
  }, [isAudioEnabled]);

  useEffect(() => {
    isVideoEnabledRef.current = isVideoEnabled;
  }, [isVideoEnabled]);

  useEffect(() => {
    videoMaxBitrateKbpsRef.current = videoMaxBitrateKbps;
  }, [videoMaxBitrateKbps]);

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
          setIceServers(servers as RTCIceServer[]);
          console.log("[SignalingContext] Using ICE servers from backend", servers);
        } else if (!cancelled) {
          setIceServers(DEFAULT_ICE_SERVERS);
        }
      } catch (error) {
        console.warn("[SignalingContext] Failed to load ICE servers, fallback to defaults", error);
        if (!cancelled) {
          setIceServers(DEFAULT_ICE_SERVERS);
        }
      }
    };
    loadIceServers();
    return () => {
      cancelled = true;
    };
  }, [token]);

  // 监听通话状态变化，播放相应的音频和震动提醒
  useEffect(() => {
    console.log("[SignalingContext] Status changed to:", status, "Session:", session?.direction);

    switch (status) {
      case "incoming":
        // 接到来电，播放来电铃声和震动
        console.log("[SignalingContext] Playing incoming call ringtone with vibration");
        if (settings.audioNotificationsEnabled) {
          AudioService.play("incoming_call");
        }
        if (settings.vibrationEnabled) {
          VibrationService.vibrate("incoming_call");
        }
        break;

      case "connecting":
        // 正在呼叫，播放回铃音和震动
        console.log("[SignalingContext] Connecting to remote peer, playing ringback");
        if (settings.audioNotificationsEnabled) {
          AudioService.play("ringback");
        }
        if (settings.vibrationEnabled) {
          VibrationService.vibrate("ringback");
        }
        break;

      case "in_call":
        // 通话接通，停止所有音频和震动
        console.log("[SignalingContext] Call connected, stopping all audio and vibration");
        AudioService.stopAll();
        VibrationService.cancel();
        if (settings.vibrationEnabled) {
          VibrationService.vibrate("call_connected"); // 通话接通提示音
        }
        break;

      case "idle":
        // 通话结束，停止所有音频和震动
        // 仅在从非idle状态改变到idle时才播放结束提示
        console.log("[SignalingContext] Call ended/idle, stopping all audio and vibration");
        AudioService.stopAll();
        VibrationService.cancel();
        // 只有当通话真实结束时才提示（由其他地方触发），避免应用启动时的误触
        break;
    }
  }, [status, session, settings.audioNotificationsEnabled, settings.vibrationEnabled]);

  const ensureAudioPermission = useCallback(async () => {
    console.log("[ensureAudioPermission] Platform:", Platform.OS);

    if (Platform.OS === "android") {
      try {
        const permissions: string[] = [
          PermissionsAndroid.PERMISSIONS.RECORD_AUDIO
        ];

        if (Platform.Version >= 31) {
          permissions.push(PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT);
        }

        // 部分厂商在仅采集音频时也会检查摄像头权限，提前申请避免崩溃
        permissions.push(PermissionsAndroid.PERMISSIONS.CAMERA);

        console.log("[ensureAudioPermission] Requesting permissions:", permissions);

        // 直接请求权限，不使用超时（真机上应该正常工作）
        const result = await PermissionsAndroid.requestMultiple(permissions as any);
        console.log("[ensureAudioPermission] Permission result:", result);

        const allGranted = permissions.every(
          (permission) => (result as Record<string, any>)[permission] === PermissionsAndroid.RESULTS.GRANTED
        );

        console.log("[ensureAudioPermission] All permissions granted:", allGranted);
        return allGranted;
      } catch (error) {
        console.error("[ensureAudioPermission] Permission request error:", error);
        Alert.alert("权限错误 / Permission Error", `无法获取权限: ${error instanceof Error ? error.message : String(error)} / Failed to get permissions.`);
        return false;
      }
    }
    console.log("[ensureAudioPermission] iOS platform, returning true");
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
    if (restartTimerRef.current) {
      clearTimeout(restartTimerRef.current);
      restartTimerRef.current = null;
    }
    if (disconnectDeadlineRef.current) {
      clearTimeout(disconnectDeadlineRef.current);
      disconnectDeadlineRef.current = null;
    }

    if (peerRef.current) {
      (peerRef.current as any).onicecandidate = null;
      (peerRef.current as any).ontrack = null;
      (peerRef.current as any).onconnectionstatechange = null;
      peerRef.current.close();
      peerRef.current = null;
    }

    if (subtitlesDataChannelRef.current) {
      try {
        subtitlesDataChannelRef.current.close();
      } catch (e) {
        // ignore
      }
      subtitlesDataChannelRef.current = null;
    }

    if (localStream) {
      localStream.getTracks().forEach((track) => track.stop());
    }
    if (remoteStream) {
      remoteStream.getTracks().forEach((track) => track.stop());
    }

    setLocalStream(null);
    setRemoteStream(null);

    setIsRemoteVideoEnabled(true);
    setIsRemoteAudioEnabled(true);
  }, [localStream, remoteStream]);

  const attachSubtitlesDataChannel = useCallback((dc: any) => {
    if (!dc) return;
    subtitlesDataChannelRef.current = dc;

    dc.onopen = () => {
      console.log('[SubtitlesDataChannel] open');
    };
    dc.onclose = () => {
      console.log('[SubtitlesDataChannel] close');
      if (subtitlesDataChannelRef.current === dc) {
        subtitlesDataChannelRef.current = null;
      }
    };
    dc.onerror = (err: any) => {
      console.warn('[SubtitlesDataChannel] error', err);
    };
    dc.onmessage = (event: any) => {
      try {
        const parsed = JSON.parse(String(event?.data ?? ''));
        if (!parsed || parsed.t !== 'subtitle') {
          return;
        }

        const ts = typeof parsed.timestampMs === 'number' ? parsed.timestampMs : Date.now();
        const subtitle: SubtitleItem = {
          id: `subtitle-${ts}`,
          original: typeof parsed.originalText === 'string' ? parsed.originalText : '',
          translated: typeof parsed.translatedText === 'string' ? parsed.translatedText : '',
          timestamp: ts,
        };
        setSubtitles((prev) => [...prev.slice(-9), subtitle]);
      } catch (e) {
        console.warn('[SubtitlesDataChannel] failed to parse message', e);
      }
    };
  }, []);

  const resetCallState = useCallback(() => {
    pendingTarget.current = null;
    setSession(null);
    sessionRef.current = null;
    setStatus("idle");
    resetPeerResources();
  }, [resetPeerResources]);

  const sendMessage = useCallback((message: SignalMessage) => {
    const client = signalingRef.current;
    console.log("[sendMessage] Attempting to send message:", message.type, "to:", message.to);

    if (!client) {
      console.warn("[sendMessage] No active signaling client, message dropped", message);
      if (message.type !== "ice.candidate") {
        Alert.alert("错误 / Connection Issue", "信令服务未连接 / Signaling service not connected.");
      }
      return;
    }

    try {
      console.log("[sendMessage] Sending message via client.send()...");
      const sent = client.send(message);
      if (!sent) {
        console.debug("[sendMessage] Signaling message queued until connection recovers", message.type);
      } else {
        console.log("[sendMessage] Message sent successfully");
      }
    } catch (error) {
      console.error("[sendMessage] Failed to send signaling message", error);
      if (message.type !== "ice.candidate") {
        Alert.alert("错误 / Connection Issue", "无法发送信令消息 / Failed to send signaling message.");
      }
    }
  }, []);

  const sendMediaUpdate = useCallback(
    (callId: string, peerEmail: string, update: MediaUpdatePayload) => {
      sendMessage({
        type: "call.media_update",
        call_id: callId,
        to: peerEmail,
        payload: update
      });
    },
    [sendMessage]
  );

  const setVideoSenderMaxBitrate = useCallback(async (kbps: number) => {
    const pc = peerRef.current;
    if (!pc) return;

    const senders = pc.getSenders();
    const sender = senders.find((s: any) => s?.track?.kind === "video");
    if (!sender) return;

    try {
      const params = sender.getParameters();
      if (!params.encodings || params.encodings.length === 0) {
        params.encodings = [{ active: true }];
      }
      params.encodings[0].maxBitrate = kbps * 1000;
      await sender.setParameters(params);
    } catch (e) {
      console.warn("[webrtc] Failed to set video sender maxBitrate", e);
    }
  }, []);

  const applyCurrentVideoBitrate = useCallback(async () => {
    const kbps = videoMaxBitrateKbpsRef.current;
    if (kbps <= 0) return;
    await setVideoSenderMaxBitrate(kbps);
  }, [setVideoSenderMaxBitrate]);

  useEffect(() => {
    const clampKbps = (kbps: number) => {
      if (!Number.isFinite(kbps)) return 900;
      return Math.max(100, Math.min(2500, Math.trunc(kbps)));
    };

    if (settings?.videoQuality) {
      setVideoQuality(settings.videoQuality);
    }
    if (typeof settings?.videoMaxBitrateKbps === "number") {
      const kbps = clampKbps(settings.videoMaxBitrateKbps);
      setVideoMaxBitrateKbps(kbps);
      if (statusRef.current === "in_call" && isVideoEnabledRef.current) {
        applyCurrentVideoBitrate();
      }
    }
    videoAdaptiveBitrateEnabledRef.current =
      typeof settings?.videoAdaptiveBitrateEnabled === "boolean"
        ? settings.videoAdaptiveBitrateEnabled
        : false;
  }, [applyCurrentVideoBitrate, settings]);

  const requestIceRestart = useCallback(
    (callId: string, peerEmail: string, reason: string) => {
      sendMessage({
        type: "call.ice-restart.request",
        call_id: callId,
        to: peerEmail,
        payload: { reason, iceEpoch: iceEpochRef.current }
      });
    },
    [sendMessage]
  );

  const startIceRestartAsCaller = useCallback(async () => {
    const current = sessionRef.current;
    const pc = peerRef.current;
    if (!current || !pc) return;
    if (current.direction !== "outgoing") return;

    if (iceRestartAttemptsRef.current >= 2) {
      console.warn("[webrtc] ICE restart attempt limit reached");
      return;
    }

    iceRestartAttemptsRef.current += 1;
    iceEpochRef.current += 1;
    pendingRemoteCandidates.current = [];

    try {
      const offer = await pc.createOffer({ iceRestart: true } as any);
      await pc.setLocalDescription(offer);
      sendMessage({
        type: "call.sdp.offer",
        call_id: current.callId,
        to: current.peerEmail,
        payload: { sdp: offer.sdp, type: offer.type, iceEpoch: iceEpochRef.current } as SdpRenegotiationPayload
      });
    } catch (e) {
      console.error("[webrtc] ICE restart failed", e);
    }
  }, [sendMessage]);

  useEffect(() => {
    if (status !== "in_call") {
      return;
    }

    const pc = peerRef.current;
    if (!pc) {
      return;
    }

    let cancelled = false;
    let lastAppliedKbps: number | null = null;

    const clampKbps = (kbps: number) => {
      if (!Number.isFinite(kbps)) return 900;
      return Math.max(100, Math.min(2500, Math.trunc(kbps)));
    };

    const timer = setInterval(async () => {
      if (cancelled) return;
      if (statusRef.current !== "in_call") return;
      if (!isVideoEnabledRef.current) return;
      if (!videoAdaptiveBitrateEnabledRef.current) return;

        try {
        const report = await pc.getStats();
        let availableOutgoingBitrateBps: number | null = null;
        let currentRtt: number | null = null;

        report.forEach((stat: unknown) => {
          const s = stat as Record<string, unknown>;
          const type = typeof s.type === "string" ? s.type : "";
          if (type !== "candidate-pair") return;

          const selected =
            typeof s.selected === "boolean"
              ? s.selected
              : typeof s.nominated === "boolean"
                ? s.nominated
                : false;
          if (!selected) return;

          // 获取可用带宽
          const raw = s.availableOutgoingBitrate;
          const bps =
            typeof raw === "number" ? raw : typeof raw === "string" ? Number(raw) : NaN;
          if (Number.isFinite(bps) && bps > 0) {
            availableOutgoingBitrateBps = bps;
          }

          // 获取 RTT
          const rttRaw = s.currentRoundTripTime;
          const rtt = typeof rttRaw === "number" ? rttRaw : typeof rttRaw === "string" ? Number(rttRaw) : NaN;
          if (Number.isFinite(rtt)) {
             currentRtt = rtt;
          }
        });

        // 计算网络质量
        let quality: NetworkQuality = "unknown";
        if (currentRtt !== null) {
          // RTT thresholds: <0.1s excellent, <0.3s good, <0.5s poor, >=0.5s bad
          if (currentRtt < 0.1) quality = "excellent";
          else if (currentRtt < 0.3) quality = "good";
          else if (currentRtt < 0.5) quality = "poor";
          else quality = "bad";
        }
        setNetworkQuality(quality);

        if (!availableOutgoingBitrateBps) {
          return;
        }

        const userMaxKbps = clampKbps(videoMaxBitrateKbpsRef.current);
        const targetKbps = clampKbps(Math.min(userMaxKbps, (availableOutgoingBitrateBps * 0.85) / 1000));

        if (lastAppliedKbps !== null) {
          const delta = Math.abs(targetKbps - lastAppliedKbps) / Math.max(1, lastAppliedKbps);
          if (delta < 0.1) {
            return;
          }
        }

        lastAppliedKbps = targetKbps;
        await setVideoSenderMaxBitrate(targetKbps);
      } catch (e) {
        console.warn("[webrtc] adaptive bitrate polling failed", e);
      }
    }, 4000);

    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [setVideoSenderMaxBitrate, status]);

  useEffect(() => {
    if (status !== "in_call") {
      remoteVideoLastBytesRef.current = null;
      remoteVideoStallCountRef.current = 0;
      remoteVideoLastRestartAtMsRef.current = 0;
      return;
    }

    const pc = peerRef.current;
    if (!pc) {
      return;
    }

    let cancelled = false;

    const timer = setInterval(async () => {
      if (cancelled) return;
      if (statusRef.current !== "in_call") return;

      const current = sessionRef.current;
      if (!current?.callId) return;
      if (!isRemoteVideoEnabled) {
        remoteVideoLastBytesRef.current = null;
        remoteVideoStallCountRef.current = 0;
        return;
      }

      const hasRemoteVideoTrack = (remoteStream?.getVideoTracks().length ?? 0) > 0;
      if (!hasRemoteVideoTrack) {
        remoteVideoLastBytesRef.current = null;
        remoteVideoStallCountRef.current = 0;
        return;
      }

      try {
        const report = await pc.getStats();
        let maxBytesReceived = 0;
        report.forEach((stat: unknown) => {
          const s = stat as unknown as Record<string, unknown>;
          const type = typeof s.type === "string" ? s.type : "";
          if (type !== "inbound-rtp") return;

          const kind = typeof s.kind === "string" ? s.kind : undefined;
          const mediaType = typeof s.mediaType === "string" ? s.mediaType : undefined;
          const isVideo = kind === "video" || mediaType === "video";
          if (!isVideo) return;

          const bytesReceivedRaw = s.bytesReceived;
          const bytesReceived =
            typeof bytesReceivedRaw === "number"
              ? bytesReceivedRaw
              : typeof bytesReceivedRaw === "string"
                ? Number(bytesReceivedRaw)
                : 0;
          if (Number.isFinite(bytesReceived)) {
            maxBytesReceived = Math.max(maxBytesReceived, bytesReceived);
          }
        });

        if (maxBytesReceived <= 0) {
          return;
        }

        const last = remoteVideoLastBytesRef.current;
        if (last === null) {
          remoteVideoLastBytesRef.current = maxBytesReceived;
          remoteVideoStallCountRef.current = 0;
          return;
        }

        if (maxBytesReceived > last) {
          remoteVideoLastBytesRef.current = maxBytesReceived;
          remoteVideoStallCountRef.current = 0;
          return;
        }

        remoteVideoStallCountRef.current += 1;
        if (remoteVideoStallCountRef.current < 3) {
          return;
        }
        remoteVideoStallCountRef.current = 0;

        const now = Date.now();
        if (now - remoteVideoLastRestartAtMsRef.current < 15000) {
          return;
        }
        remoteVideoLastRestartAtMsRef.current = now;

        console.warn("[webrtc] Remote video appears frozen; triggering ICE restart");
        if (current.direction === "outgoing") {
          startIceRestartAsCaller();
        } else {
          requestIceRestart(current.callId, current.peerEmail, "remote_video_frozen");
        }
      } catch (e) {
        console.warn("[webrtc] getStats polling failed", e);
      }
    }, 2000);

    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [isRemoteVideoEnabled, remoteStream, requestIceRestart, startIceRestartAsCaller, status]);

  const enqueueRemoteCandidate = useCallback((candidate: IceCandidatePayload) => {
    const expectedEpoch = iceEpochRef.current;
    const candidateEpoch = typeof candidate.iceEpoch === "number" ? candidate.iceEpoch : 0;
    if (candidateEpoch !== expectedEpoch) {
      return;
    }
    const alreadyQueued = pendingRemoteCandidates.current.some(
      (item) =>
        item.candidate === candidate.candidate &&
        item.sdpMid === candidate.sdpMid &&
        item.sdpMLineIndex === candidate.sdpMLineIndex
    );
    if (!alreadyQueued) {
      pendingRemoteCandidates.current.push(candidate);
    }
  }, []);

  const flushPendingLocalCandidates = useCallback(
    (callId: string, peerEmail: string) => {
      if (!pendingLocalCandidates.current.length) {
        return;
      }
      const items = [...pendingLocalCandidates.current];
      pendingLocalCandidates.current = [];
      items.forEach((candidate) =>
        sendMessage({
          type: "ice.candidate",
          call_id: callId,
          to: peerEmail,
          payload: candidate
        })
      );
    },
    [sendMessage]
  );

  const drainRemoteCandidates = useCallback(async () => {
    const pc = peerRef.current;
    if (!pc || !pendingRemoteCandidates.current.length) {
      return;
    }
    const items = [...pendingRemoteCandidates.current];
    pendingRemoteCandidates.current = [];
    const expectedEpoch = iceEpochRef.current;
    for (const candidate of items) {
      const candidateEpoch = typeof candidate.iceEpoch === "number" ? candidate.iceEpoch : 0;
      if (candidateEpoch !== expectedEpoch) {
        continue;
      }
      try {
        await pc.addIceCandidate(new RTCIceCandidate(candidate));
      } catch (error) {
        console.warn("Failed to add queued ICE candidate", error);
      }
    }
  }, []);

  const createPeerConnection = useCallback(() => {
    const pc = new RTCPeerConnection({
      iceServers,
      bundlePolicy: "max-bundle",
      iceTransportPolicy: "all"
    } as any);

    (pc as any).onicecandidate = (event: any) => {
      if (!event.candidate) {
        console.log("[PeerConnection] ICE gathering completed (null candidate)");
        return;
      }
      console.log("[PeerConnection] ICE candidate:", event.candidate.type, event.candidate.address);
      const candidateInit: IceCandidatePayload = {
        candidate: event.candidate.candidate,
        sdpMid: event.candidate.sdpMid ?? undefined,
        sdpMLineIndex: event.candidate.sdpMLineIndex ?? undefined,
        iceEpoch: iceEpochRef.current
      };
      const current = sessionRef.current;
      if (current?.callId) {
        sendMessage({
          type: "ice.candidate",
          call_id: current.callId,
          to: current.peerEmail,
          payload: candidateInit as any
        });
      } else {
        pendingLocalCandidates.current.push(candidateInit);
      }
    };

    // 添加 ICE 连接状态监控
    (pc as any).oniceconnectionstatechange = () => {
      const iceState = pc.iceConnectionState;
      console.log("[PeerConnection] ICE connection state:", iceState);

      const current = sessionRef.current;
      if (!current || statusRef.current !== "in_call") {
        return;
      }

      if (iceState === "failed") {
        if (current.direction === "outgoing") {
          startIceRestartAsCaller();
        } else {
          requestIceRestart(current.callId, current.peerEmail, "ice_failed");
        }
      }
    };

    // 添加 ICE gathering 状态监控
    (pc as any).onicegatheringstatechange = () => {
      console.log("[PeerConnection] ICE gathering state:", pc.iceGatheringState);
    };

    (pc as any).ontrack = (event: any) => {
      const [stream] = event.streams;
      if (stream) {
        setRemoteStream(stream);

        stream.getTracks().forEach((track: any) => {
          track.onmute = () => {
            if (track.kind === "video") setIsRemoteVideoEnabled(false);
            if (track.kind === "audio") setIsRemoteAudioEnabled(false);
          };
          track.onunmute = () => {
            if (track.kind === "video") setIsRemoteVideoEnabled(true);
            if (track.kind === "audio") setIsRemoteAudioEnabled(true);
          };

          if (track.kind === "video") setIsRemoteVideoEnabled(track.enabled && !track.muted);
          if (track.kind === "audio") setIsRemoteAudioEnabled(track.enabled && !track.muted);
        });
      }
    };

    // Subtitles over DataChannel (peer-to-peer).
    (pc as any).ondatachannel = (event: any) => {
      const dc = event?.channel;
      console.log('[PeerConnection] ondatachannel', dc?.label);
      if (dc && dc.label === 'subtitles') {
        attachSubtitlesDataChannel(dc);
      }
    };

    (pc as any).onconnectionstatechange = () => {
      const state = pc.connectionState;
      console.log("[PeerConnection] Connection state changed:", state);

      if (state === "failed" || state === "closed") {
        const current = sessionRef.current;
        if (current && statusRef.current === "in_call") {
          if (current.direction === "outgoing") {
            startIceRestartAsCaller();
          } else {
            requestIceRestart(current.callId, current.peerEmail, "connection_failed");
          }
          if (disconnectDeadlineRef.current) clearTimeout(disconnectDeadlineRef.current);
          disconnectDeadlineRef.current = setTimeout(() => {
            resetCallState();
          }, 10000);
        } else {
          console.log("[PeerConnection] Connection failed or closed, ending call");
          resetCallState();
        }
      } else if (state === "disconnected") {
        const current = sessionRef.current;
        if (current && statusRef.current === "in_call") {
          if (restartTimerRef.current) clearTimeout(restartTimerRef.current);
          restartTimerRef.current = setTimeout(() => {
            if (current.direction === "outgoing") {
              startIceRestartAsCaller();
            } else {
              requestIceRestart(current.callId, current.peerEmail, "disconnected");
            }
          }, 1500);

          if (disconnectDeadlineRef.current) clearTimeout(disconnectDeadlineRef.current);
          disconnectDeadlineRef.current = setTimeout(() => {
            const currentState = pc.connectionState;
            if (currentState === "disconnected" || currentState === "failed") {
              resetCallState();
            }
          }, 10000);
        }
      } else if (state === "connected") {
        console.log("[PeerConnection] Connection established successfully!");
        if (restartTimerRef.current) {
          clearTimeout(restartTimerRef.current);
          restartTimerRef.current = null;
        }
        if (disconnectDeadlineRef.current) {
          clearTimeout(disconnectDeadlineRef.current);
          disconnectDeadlineRef.current = null;
        }
      }
    };

    peerRef.current = pc;
    return pc;
  }, [attachSubtitlesDataChannel, iceServers, resetCallState, sendMessage]);

  useEffect(() => {
    if (!token) {
      console.log("[SignalingContext] No token available, disconnecting");
      signalingRef.current?.disconnect();
      signalingRef.current = null;
      setConnectionReady(false);
      resetCallState();
      return;
    }

    console.log("[SignalingContext] Token available, initializing signaling client", {
      tokenLength: token.length,
      tokenPrefix: token.substring(0, 20) + "..."
    });
    const client = new SignalingClient(token);
    signalingRef.current = client;
    client.connect();

    const handleOpen = () => {
      console.log("[SignalingContext] Signaling connection opened successfully!");
      setConnectionReady(true);
    };
    const handleClose = () => {
      console.warn("[SignalingContext] Signaling connection closed");
      setConnectionReady(false);
      resetCallState();
    };

  const handleMessage = async (message: SignalMessage) => {
      console.log("[SignalingContext] Received message:", message.type, "from:", message.from);
      switch (message.type) {
        case "call.invite.ack":
          console.log("[SignalingContext] Received call.invite.ack, callId:", message.call_id, "pendingTarget:", pendingTarget.current);
          if (pendingTarget.current) {
            const newSession: CallSession = {
              callId: message.call_id ?? "",
              peerEmail: pendingTarget.current,
              direction: "outgoing"
            };
            console.log("[SignalingContext] Creating new session:", newSession);
            sessionRef.current = newSession;
            setSession(newSession);
            setStatus("connecting");
            if (newSession.callId) {
              console.log("[SignalingContext] Flushing pending local candidates");
              flushPendingLocalCandidates(
                newSession.callId,
                newSession.peerEmail
              );
            }
            pendingTarget.current = null;
          } else {
            console.warn("[SignalingContext] Received call.invite.ack but no pending target");
          }
          break;
        case "call.invite":
          if (!message.from || !isSessionDescriptionPayload(message.payload)) {
            Alert.alert("呼叫错误", "收到无效的呼叫请求");
            break;
          }
          setSession({
            callId: message.call_id ?? "",
            peerEmail: message.from,
            direction: "incoming",
            offer: message.payload as SessionDescriptionPayload
          });
          setStatus("incoming");
          break;
        case "call.accept":
          if (isSessionDescriptionPayload(message.payload)) {
            const pc = peerRef.current;
            if (pc && message.payload.sdp) {
              try {
                await pc.setRemoteDescription(
                  new RTCSessionDescription(message.payload as any)
                );
                await drainRemoteCandidates();
              } catch (error) {
                console.warn("Failed to apply remote answer", error);
              }
            }
          }
          setStatus("in_call");
          setSession((current) =>
            current
              ? {
                ...current,
                callId: message.call_id ?? current.callId
              }
              : current
          );
          if (sessionRef.current && message.call_id) {
            const current = {
              ...sessionRef.current,
              callId: message.call_id
            };
            sessionRef.current = current;
            flushPendingLocalCandidates(current.callId, current.peerEmail);
          }

          if (sessionRef.current?.callId) {
            sendMediaUpdate(sessionRef.current.callId, sessionRef.current.peerEmail, {
              audioEnabled: isAudioEnabledRef.current,
              videoEnabled: isVideoEnabledRef.current
            });
          }
          break;
        case "call.media_update":
          if (message.payload && typeof message.payload === "object") {
            const payload = message.payload as any;
            if (typeof payload.videoEnabled === "boolean") {
              setIsRemoteVideoEnabled(payload.videoEnabled);
            }
            if (typeof payload.audioEnabled === "boolean") {
              setIsRemoteAudioEnabled(payload.audioEnabled);
            }
          }
          break;
        case "call.ice-restart.request": {
          const current = sessionRef.current;
          if (!current || statusRef.current !== "in_call") {
            break;
          }
          if (current.direction === "outgoing") {
            startIceRestartAsCaller();
          }
          break;
        }
        case "call.sdp.offer": {
          if (!isSessionDescriptionPayload(message.payload)) {
            break;
          }
          const pc = peerRef.current;
          const current = sessionRef.current;
          if (!pc || !current || statusRef.current !== "in_call") {
            break;
          }

          const payload = message.payload as any;
          const incomingEpoch = typeof payload.iceEpoch === "number" ? payload.iceEpoch : 0;
          if (incomingEpoch > iceEpochRef.current) {
            iceEpochRef.current = incomingEpoch;
            pendingRemoteCandidates.current = [];
          }

          try {
            await pc.setRemoteDescription(new RTCSessionDescription(payload));
            const answer = await pc.createAnswer();
            await pc.setLocalDescription(answer);
            sendMessage({
              type: "call.sdp.answer",
              call_id: current.callId,
              to: current.peerEmail,
              payload: { sdp: answer.sdp, type: answer.type, iceEpoch: iceEpochRef.current } as SdpRenegotiationPayload
            });
            await drainRemoteCandidates();
          } catch (e) {
            console.warn("Failed to apply renegotiation offer", e);
          }
          break;
        }
        case "call.sdp.answer": {
          if (!isSessionDescriptionPayload(message.payload)) {
            break;
          }
          const pc = peerRef.current;
          if (!pc || statusRef.current !== "in_call") {
            break;
          }
          try {
            await pc.setRemoteDescription(new RTCSessionDescription(message.payload as any));
            await drainRemoteCandidates();
          } catch (e) {
            console.warn("Failed to apply renegotiation answer", e);
          }
          break;
        }
        case "call.reject":
          Alert.alert("Call rejected", `${message.from} declined the call.`);
          resetCallState();
          break;
        case "call.end":
          Alert.alert("Call ended", `${message.from ?? "Peer"} ended the call.`);
          resetCallState();
          break;
        case "ice.candidate":
          if (isIceCandidatePayload(message.payload)) {
            const pc = peerRef.current;
            if (pc) {
              const hasRemoteDescription =
                pc.remoteDescription !== null &&
                typeof pc.remoteDescription?.type === "string";
              if (hasRemoteDescription) {
                try {
                  await pc.addIceCandidate(
                    new RTCIceCandidate(message.payload)
                  );
                } catch (error) {
                  console.warn("Failed to add ICE candidate", error);
                }
              } else {
                enqueueRemoteCandidate(message.payload);
              }
            } else {
              enqueueRemoteCandidate(message.payload);
            }
          }
          break;
        case "call.error":
          if (message.payload && typeof message.payload === "object" && "reason" in message.payload) {
            Alert.alert("Call error", String((message.payload as any).reason ?? "Error"));
          } else {
            Alert.alert("Call error", "Error");
          }
          resetCallState();
          break;
        default:
          break;
      }
    };

    client.on("open", handleOpen);
    client.on("close", handleClose);
    client.on("message", handleMessage);
    client.on("error", (err) => console.warn("signaling error", err));

    return () => {
      client.off("open", handleOpen);
      client.off("close", handleClose);
      client.off("message", handleMessage);
      client.disconnect();
      signalingRef.current = null;
    };
  }, [flushPendingLocalCandidates, resetCallState, drainRemoteCandidates, enqueueRemoteCandidate, token]);

  const startCall = useCallback(
    async (email: string) => {
      console.log("[startCall] Starting call to:", email, "Current status:", status);

      if (!user) {
        console.warn("[startCall] No user logged in");
        Alert.alert("错误 / Error", "请先登录 / Please log in first.");
        return;
      }

      if (status !== "idle") {
        console.warn("[startCall] Call already in progress. Current status:", status);
        Alert.alert("提示 / Tip", "已有通话在进行中，请先结束该通话 / A call is already in progress. Please end it first.");
        return;
      }

      try {
        // 从设置中获取默认值
        const shouldEnableVideo = settings.defaultVideoEnabled;
        const shouldEnableAudio = settings.defaultAudioEnabled;
        const defaultCameraFacing = settings.cameraFacing;

        console.log("[startCall] Media settings:", {
          video: shouldEnableVideo,
          audio: shouldEnableAudio,
          camera: defaultCameraFacing
        });

        // 检查权限
        const permissionResult = await CameraPermissionService.checkPermissions();
        if (!permissionResult.microphone) {
          Alert.alert("需要麦克风权限 / Microphone Permission Required", "请在系统设置中授予麦克风权限 / Please grant microphone permission in system settings.");
          return;
        }
        if (shouldEnableVideo && !permissionResult.camera) {
          Alert.alert("需要摄像头权限 / Camera Permission Required", "请在系统设置中授予摄像头权限 / Please grant camera permission in system settings.");
          return;
        }

        console.log("[startCall] Resetting peer resources...");
        resetPeerResources();

        console.log("[startCall] Requesting media stream...");

        if (!webrtcMediaDevices) {
          throw new Error("WebRTC mediaDevices not available. Please use 'expo run:android' to build a native app.");
        }

        // 使用 VideoService 获取媒体流
        await VideoService.initialize();
        const clampKbps = (kbps: number) => Math.max(100, Math.min(2500, Math.trunc(kbps)));
        const initialQuality: VideoQuality = settings.videoQuality ?? "medium";
        const initialMaxKbps =
          typeof settings.videoMaxBitrateKbps === "number"
            ? clampKbps(settings.videoMaxBitrateKbps)
            : 900;
        setVideoQuality(initialQuality);
        setVideoMaxBitrateKbps(initialMaxKbps);

        const stream = await VideoService.getLocalStream(
          shouldEnableAudio,
          shouldEnableVideo,
          defaultCameraFacing,
          initialQuality
        );

        if (!stream) {
          throw new Error("Failed to get media stream");
        }

        console.log("[startCall] Media stream obtained:", stream.getTracks().length, "tracks");
        stream.getTracks().forEach((track) => {
          console.log("[startCall] Track obtained - Kind:", track.kind, "Enabled:", track.enabled);
        });

        setLocalStream(stream);
        setIsVideoEnabled(shouldEnableVideo);
        setIsAudioEnabled(shouldEnableAudio);
        setCameraFacing(defaultCameraFacing);

        console.log("[startCall] Creating peer connection...");
        const pc = createPeerConnection();

        // Ensure we always have a video m-line so we can toggle video later via replaceTrack
        // without renegotiation (as long as the remote also supports it).
        if (stream.getVideoTracks().length === 0) {
          try {
            const transceivers = pc.getTransceivers();
            const hasVideo = transceivers.some(
              (t) => t?.receiver?.track?.kind === "video"
            );
            if (!hasVideo) {
              pc.addTransceiver("video", { direction: "sendrecv" });
            }
          } catch (e) {
            console.warn("[webrtc] Failed to ensure video transceiver", e);
          }
        }

        // Create subtitles DataChannel on the offerer before creating offer.
        try {
          const dc = (pc as any).createDataChannel?.('subtitles', {
            ordered: true,
          });
          if (dc) {
            console.log('[startCall] Subtitles DataChannel created');
            attachSubtitlesDataChannel(dc);
          }
        } catch (e) {
          console.warn('[startCall] Failed to create subtitles DataChannel', e);
        }

        stream.getTracks().forEach((track) => {
          console.log("[startCall] Adding track:", track.kind);
          pc.addTrack(track, stream);
        });

        console.log("[startCall] Creating offer...");
        const offer = await pc.createOffer({
          offerToReceiveAudio: true,
          offerToReceiveVideo: true
        });
        console.log("[startCall] Offer created, SDP length:", offer.sdp?.length);

        console.log("[startCall] Setting local description...");
        await pc.setLocalDescription(offer);
        console.log("[startCall] Local description set");

        if (shouldEnableVideo) {
          await applyCurrentVideoBitrate();
        }

        pendingTarget.current = email;
        setStatus("connecting");
        console.log("[startCall] Status changed to 'connecting'");

        console.log("[startCall] Sending call.invite message...");
        sendMessage({
          type: "call.invite",
          to: email,
          payload: {
            sdp: offer.sdp,
            type: offer.type
          }
        });
        console.log("[startCall] call.invite message sent");
      } catch (error) {
        console.error("[startCall] Error occurred:", error);
        console.error("[startCall] Error name:", (error as Error)?.name);
        console.error("[startCall] Error message:", (error as Error)?.message);
        const errorMsg = error instanceof Error ? error.message : String(error);
        Alert.alert("错误 / Error", "请确认麦克风/摄像头未被占用或已授权 / Please ensure the microphone/camera is not in use or permissions are granted.");
        resetPeerResources();
        setStatus("idle");
      }
    },
    [applyCurrentVideoBitrate, attachSubtitlesDataChannel, createPeerConnection, resetPeerResources, sendMessage, status, user, settings]
  );

  const acceptCall = useCallback(async () => {
    if (!session || session.direction !== "incoming" || !session.offer) {
      return;
    }

    try {
      // 从设置中获取默认值
      const shouldEnableVideo = settings.defaultVideoEnabled;
      const shouldEnableAudio = settings.defaultAudioEnabled;
      const defaultCameraFacing = settings.cameraFacing;

      console.log("[acceptCall] Media settings:", {
        video: shouldEnableVideo,
        audio: shouldEnableAudio,
        camera: defaultCameraFacing
      });

      // 检查权限
      const permissionResult = await CameraPermissionService.checkPermissions();
      if (!permissionResult.microphone) {
        Alert.alert("需要麦克风权限 / Microphone Permission Required", "请在系统设置中授予麦克风权限 / Please grant microphone permission in system settings.");
        return;
      }
      if (shouldEnableVideo && !permissionResult.camera) {
        Alert.alert("需要摄像头权限 / Camera Permission Required", "请在系统设置中授予摄像头权限 / Please grant camera permission in system settings.");
        return;
      }

      console.log("[acceptCall] Requesting media stream...");

      if (!webrtcMediaDevices) {
        throw new Error("WebRTC mediaDevices not available. Please use 'expo run:android' to build a native app.");
      }

      // 使用 VideoService 获取媒体流
       const clampKbps = (kbps: number) => Math.max(100, Math.min(2500, Math.trunc(kbps)));
       const initialQuality: VideoQuality = settings.videoQuality ?? "medium";
       const initialMaxKbps =
         typeof settings.videoMaxBitrateKbps === "number"
           ? clampKbps(settings.videoMaxBitrateKbps)
           : 900;
       setVideoQuality(initialQuality);
       setVideoMaxBitrateKbps(initialMaxKbps);

      await VideoService.initialize();
      const stream = await VideoService.getLocalStream(
        shouldEnableAudio,
        shouldEnableVideo,
        defaultCameraFacing,
        initialQuality
      );

      if (!stream) {
        throw new Error("Failed to get media stream");
      }

      console.log("[acceptCall] Media stream obtained:", stream.getTracks().length, "tracks");
      stream.getTracks().forEach((track) => {
        console.log("[acceptCall] Track obtained - Kind:", track.kind, "Enabled:", track.enabled);
      });

      setLocalStream(stream);
      setIsVideoEnabled(shouldEnableVideo);
      setIsAudioEnabled(shouldEnableAudio);
      setCameraFacing(defaultCameraFacing);

      const pc = createPeerConnection();
      stream.getTracks().forEach((track) => pc.addTrack(track, stream));

      try {
        await pc.setRemoteDescription(new RTCSessionDescription(session.offer as any));
      } catch (error) {
        console.warn("setRemoteDescription failed", error);
        Alert.alert("呼叫错误 / Call Error", "无法解析对方的连接请求 / Failed to parse the remote call request.");
        resetCallState();
        return;
      }

      const answer = await pc.createAnswer();
      await pc.setLocalDescription(answer);
      await drainRemoteCandidates();

      sendMessage({
        type: "call.accept",
        call_id: session.callId,
        to: session.peerEmail,
        payload: {
          sdp: answer.sdp,
          type: answer.type
        }
      });

      if (shouldEnableVideo) {
        await applyCurrentVideoBitrate();
      }

      sendMediaUpdate(session.callId, session.peerEmail, {
        audioEnabled: shouldEnableAudio,
        videoEnabled: shouldEnableVideo
      });

      setStatus("in_call");
    } catch (error) {
      console.error("acceptCall error", error);
      Alert.alert("无法接通 / Failed to Answer", "请确认麦克风/摄像头权限已授权 / Please ensure microphone/camera permissions are granted.");
      resetCallState();
    }
  }, [applyCurrentVideoBitrate, createPeerConnection, drainRemoteCandidates, resetCallState, sendMediaUpdate, sendMessage, session, settings]);

  const rejectCall = useCallback(() => {
    if (!session) {
      return;
    }
    sendMessage({
      type: "call.reject",
      call_id: session.callId,
      to: session.peerEmail
    });
    resetCallState();
  }, [resetCallState, sendMessage, session]);

  const endCall = useCallback(() => {
    if (!session) {
      return;
    }
    sendMessage({
      type: "call.end",
      call_id: session.callId,
      to: session.peerEmail
    });
    resetCallState();
  }, [resetCallState, sendMessage, session]);

  /**
   * 切换视频开关
   */
  const toggleVideo = useCallback(async () => {
    try {
      console.log("[toggleVideo] Current video enabled:", isVideoEnabled);

      if (!localStream) {
        console.warn("[toggleVideo] No local stream available");
        return;
      }

      const newVideoEnabled = !isVideoEnabled;

      const pc = peerRef.current;
      if (!pc) {
        return;
      }

      const buildVideoConstraints = (facing: CameraFacing, quality: VideoQuality) => {
        const preset = VIDEO_QUALITY_PRESETS[quality];
        return {
          width: { ideal: preset.width },
          height: { ideal: preset.height },
          frameRate: { ideal: preset.frameRate },
          facingMode: facing === "front" ? "user" : "environment"
        };
      };

      const findVideoTransceiver = () => {
        try {
          const transceivers = pc.getTransceivers();
          return (
            transceivers.find((t) => t?.receiver?.track?.kind === "video") ?? null
          );
        } catch {
          return null;
        }
      };

      if (newVideoEnabled) {
        // 开启视频：需要检查权限并重新获取流
        const permissionResult = await CameraPermissionService.checkPermissions();
        if (!permissionResult.camera) {
          Alert.alert("权限不足 / Permission Required", "需要摄像头权限才能开启视频 / Camera permission is required to enable video.");
          return;
        }

        const oldVideoTracks = localStream.getVideoTracks();

        const videoStream = await webrtcMediaDevices.getUserMedia({
          audio: false,
          video: buildVideoConstraints(cameraFacing, videoQuality)
        });
        const videoTrack = videoStream.getVideoTracks()[0];
        if (!videoTrack) {
          throw new Error("Failed to acquire video track");
        }

        const videoTransceiver = findVideoTransceiver();
        if (!videoTransceiver) {
          videoTrack.stop();
          Alert.alert(
            "提示 / Tip",
            "当前通话未协商视频轨道，需要重新拨号开启视频 / Video was not negotiated for this call; please restart the call with video enabled."
          );
          return;
        }

        await videoTransceiver.sender.replaceTrack(videoTrack);

        const nextStream = new MediaStream();
        localStream.getAudioTracks().forEach((t) => nextStream.addTrack(t));
        nextStream.addTrack(videoTrack);
        setLocalStream(nextStream);
        setIsVideoEnabled(true);
        await applyCurrentVideoBitrate();

        oldVideoTracks.forEach((t) => t.stop());

        const current = sessionRef.current;
        if (current?.callId && statusRef.current === "in_call") {
          sendMediaUpdate(current.callId, current.peerEmail, {
            audioEnabled: isAudioEnabledRef.current,
            videoEnabled: true
          });
        }
        console.log("[toggleVideo] Video enabled successfully");
      } else {
        // 关闭视频：直接禁用轨道
        const videoTransceiver = findVideoTransceiver();
        if (videoTransceiver) {
          await videoTransceiver.sender.replaceTrack(null);
        }

        localStream.getVideoTracks().forEach((track) => track.stop());

        const nextStream = new MediaStream();
        localStream.getAudioTracks().forEach((t) => nextStream.addTrack(t));
        setLocalStream(nextStream);
        setIsVideoEnabled(false);

        const current = sessionRef.current;
        if (current?.callId && statusRef.current === "in_call") {
          sendMediaUpdate(current.callId, current.peerEmail, {
            audioEnabled: isAudioEnabledRef.current,
            videoEnabled: false
          });
        }
        console.log("[toggleVideo] Video disabled");
      }
    } catch (error) {
      console.error("[toggleVideo] Error:", error);
      Alert.alert("错误 / Error", "无法切换视频状态 / Failed to toggle video.");
    }
  }, [applyCurrentVideoBitrate, cameraFacing, isVideoEnabled, localStream, sendMediaUpdate, videoQuality]);

  /**
   * 切换麦克风开关
   */
  const toggleAudio = useCallback(() => {
    console.log("[toggleAudio] Current audio enabled:", isAudioEnabled);

    if (!localStream) {
      console.warn("[toggleAudio] No local stream available");
      return;
    }

    const newAudioEnabled = !isAudioEnabled;
    VideoService.toggleAudioTrack(newAudioEnabled);
    setIsAudioEnabled(newAudioEnabled);

    const current = sessionRef.current;
    if (current?.callId && status === "in_call") {
      sendMediaUpdate(current.callId, current.peerEmail, {
        audioEnabled: newAudioEnabled,
        videoEnabled: isVideoEnabled
      });
    }
    console.log(`[toggleAudio] Audio ${newAudioEnabled ? "enabled" : "disabled"}`);
  }, [isAudioEnabled, isVideoEnabled, localStream, sendMediaUpdate, status]);

  /**
   * 切换摄像头（前置/后置）
   */
  const switchCamera = useCallback(async () => {
    try {
      console.log("[switchCamera] Current facing:", cameraFacing);

      if (!localStream || !isVideoEnabled) {
        console.warn("[switchCamera] No video stream or video not enabled");
        Alert.alert("提示 / Tip", "请先开启视频 / Please enable video first.");
        return;
      }

      const pc = peerRef.current;
      if (!pc) {
        return;
      }

      const newFacing: CameraFacing = cameraFacing === "front" ? "back" : "front";

      const preset = VIDEO_QUALITY_PRESETS[videoQuality];
      const videoStream = await webrtcMediaDevices.getUserMedia({
        audio: false,
        video: {
          width: { ideal: preset.width },
          height: { ideal: preset.height },
          frameRate: { ideal: preset.frameRate },
          facingMode: newFacing === "front" ? "user" : "environment"
        }
      });
      const newVideoTrack = videoStream.getVideoTracks()[0];
      if (!newVideoTrack) {
        throw new Error("Failed to acquire video track");
      }

      let videoTransceiver: RTCRtpTransceiver | null = null;
      try {
        videoTransceiver =
          pc.getTransceivers().find((t) => t?.receiver?.track?.kind === "video") ?? null;
      } catch {
        videoTransceiver = null;
      }
      if (!videoTransceiver) {
        newVideoTrack.stop();
        Alert.alert(
          "提示 / Tip",
          "当前通话未协商视频轨道，需要重新拨号开启视频 / Video was not negotiated for this call; please restart the call with video enabled."
        );
        return;
      }

      const oldVideoTracks = localStream.getVideoTracks();
      await videoTransceiver.sender.replaceTrack(newVideoTrack);

      const nextStream = new MediaStream();
      localStream.getAudioTracks().forEach((t) => nextStream.addTrack(t));
      nextStream.addTrack(newVideoTrack);
      setLocalStream(nextStream);
      setCameraFacing(newFacing);
      await applyCurrentVideoBitrate();

      oldVideoTracks.forEach((t) => t.stop());

      console.log("[switchCamera] Camera switched to:", newFacing);
    } catch (error) {
      console.error("[switchCamera] Error:", error);
      Alert.alert("错误 / Error", "无法切换摄像头 / Failed to switch camera.");
    }
  }, [applyCurrentVideoBitrate, cameraFacing, isVideoEnabled, localStream, videoQuality]);

  /**
   * 切换扬声器/听筒
   */
  const toggleSpeaker = useCallback(async () => {
    const newState = !isSpeakerOn;
    setIsSpeakerOn(newState);
    await AudioService.setSpeakerphone(newState);
  }, [isSpeakerOn]);

  // 翻译控制函数
  const toggleTranslation = useCallback(async (enabled: boolean) => {
    try {
      if (enabled) {
        // 启用翻译
        console.log("[toggleTranslation] Enabling translation");

        // 初始化翻译服务
        if (!TranslationService.isReady()) {
          await TranslationService.initialize({
            whisperModel: 'small',
            targetLanguage: translationLanguage,
            quantization: 'int8'
          });
        }

        setTranslationEnabled(true);
      } else {
        // 禁用翻译
        console.log("[toggleTranslation] Disabling translation");

        try {
          await TranslationService.stopWebRTCCallMicTranslation();
        } catch (e) {
          console.warn('[toggleTranslation] stopWebRTCCallMicTranslation failed:', e);
        }

        if (processorRef.current) {
          processorRef.current.stopProcessing();
          processorRef.current = null;
        }

        setTranslationEnabled(false);
        setSubtitles([]);
      }
    } catch (error) {
      console.error("[toggleTranslation] Error:", error);
      Alert.alert("错误 / Error", `翻译功能开关失败 / Failed to toggle translation: ${error instanceof Error ? error.message : String(error)}`);
    }
  }, [translationLanguage]);

  useEffect(() => {
    let cancelled = false;

    const start = async () => {
      if (!translationEnabled) return;
      if (status !== 'in_call') return;
      if (!TranslationService.isReady()) return;

      const currentSession = sessionRef.current;
      if (!currentSession?.peerEmail) return;

      try {
        await TranslationService.startWebRTCCallMicTranslation(
          translationLanguage,
          (result) => {
            if (cancelled) return;

            const dc = subtitlesDataChannelRef.current;
            if (!dc || dc.readyState !== 'open') {
              // Privacy-first: do not fall back to signaling here.
              console.warn('[translation] subtitles DataChannel not open; dropping subtitle');
              return;
            }

            dc.send(
              JSON.stringify({
                t: 'subtitle',
                originalText: result.originalText,
                translatedText: result.translatedText,
                timestampMs: result.timestampMs,
              })
            );
          }
        );
      } catch (e) {
        console.error('[translation] startWebRTCCallMicTranslation failed:', e);
      }
    };

    start();

    return () => {
      cancelled = true;
      if (translationEnabled) {
        TranslationService.stopWebRTCCallMicTranslation().catch(() => undefined);
      }
    };
  }, [translationEnabled, status, translationLanguage]);

  const handleSetTranslationLanguage = useCallback((language: string) => {
    setTranslationLanguage(language);

    // 如果翻译功能开启，更新处理器的目标语言
    if (processorRef.current) {
      processorRef.current.setTargetLanguage(language);
    }

    console.log("[setTranslationLanguage] Language changed to:", language);
  }, []);

  const clearSubtitles = useCallback(() => {
    setSubtitles([]);
  }, []);

  const value = useMemo<SignalingContextValue>(
    () => ({
      status,
      session,
      connectionReady,
      localStream,
      remoteStream,
      networkQuality,
      isVideoEnabled,
      isAudioEnabled,
      isRemoteVideoEnabled,
      isRemoteAudioEnabled,
      cameraFacing,
      videoQuality,
      setVideoQuality,
      videoMaxBitrateKbps,
      setVideoMaxBitrateKbps,
      translationEnabled,
      translationLanguage,
      subtitles,
      startCall,
      acceptCall,
      rejectCall,
      endCall,
      toggleVideo,
      toggleAudio,
      switchCamera,
      toggleSpeaker,
      isSpeakerOn,
      toggleTranslation,
      setTranslationLanguage: handleSetTranslationLanguage,
      clearSubtitles
    }),
    [
      status,
      session,
      connectionReady,
      localStream,
      remoteStream,
      networkQuality,
      isVideoEnabled,
      isAudioEnabled,
      isRemoteVideoEnabled,
      isRemoteAudioEnabled,
      cameraFacing,
      videoQuality,
      setVideoQuality,
      videoMaxBitrateKbps,
      setVideoMaxBitrateKbps,
      translationEnabled,
      translationLanguage,
      subtitles,
      startCall,
      acceptCall,
      rejectCall,
      endCall,
      toggleVideo,
      toggleAudio,
      switchCamera,
      toggleSpeaker,
      isSpeakerOn,
      toggleTranslation,
      handleSetTranslationLanguage,
      clearSubtitles
    ]
  );

  return (
    <SignalingContext.Provider value={value}>
      {children}
    </SignalingContext.Provider>
  );
};

export const useSignaling = () => {
  const ctx = useContext(SignalingContext);
  if (!ctx) {
    throw new Error("useSignaling must be used within SignalingProvider");
  }
  return ctx;
};
