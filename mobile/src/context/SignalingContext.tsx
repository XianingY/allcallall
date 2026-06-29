import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState
} from "react";
import {
  Alert,
} from "react-native";
import AsyncStorage from "@react-native-async-storage/async-storage";
import {
  MediaStream,
  RTCPeerConnection,
  RTCIceCandidate,
  RTCSessionDescription,
  mediaDevices as webrtcMediaDevices
} from "../platform/rtc";

import {
  MediaUpdatePayload,
  SdpRenegotiationPayload,
  SubtitlePayload,
  SignalMessage
} from "../api/signaling";
import {
  E2EE_ENABLED,
  RESTRICTED_NETWORK_MODE,
  SIGNALING_TRANSPORT_MODE,
  TRANSLATION_MODE,
  TRANSLATION_SOURCE_LANG,
  TRANSLATION_TARGET_LANG
} from "../config";
import { fetchWebRTCConfig } from "../api/webrtc";
import { useAuthContext } from "./AuthContext";
import { useCommercial } from "./CommercialContext";
import { useSettings } from "./SettingsContext";
import AudioService from "../services/AudioServiceExpo";
import VibrationService from "../services/VibrationService";
import VideoService, { CameraFacing, VideoQuality } from "../services/VideoService";
import CameraPermissionService from "../services/CameraPermissionService";
import permissionsAdapter from "../platform/permissionsAdapter";
import OnlineTranslationService, {
  type OnlineTranslationStatus,
  type OnlineTranslationResult
} from "../services/translation/OnlineTranslationService";
import AnalyticsService from "../services/AnalyticsService";
import {
  FIRST_CALL_STARTED_STORAGE_KEY,
  FIRST_TRANSLATION_ENABLED_STORAGE_KEY,
  FIRST_TRANSLATION_HINT_SEEN_STORAGE_KEY
} from "../constants/onboarding";
import { useSubtitleStore } from "../store/useSubtitleStore";
import { E2EEKeyExchange, type E2EEKeyExchangeCallbacks, type KeyExchangeRole } from "../services/e2ee/E2EEKeyExchange";
import {
  E2EEUnsupportedError,
  isE2EECryptoSupported,
  type E2EESessionKey
} from "../services/e2ee/E2EEService";
import { DEFAULT_ICE_SERVERS } from "./signalingConstants";
import { SignalingContext } from "./signalingContextValue";
import { preferRestrictedIceServers } from "./signalingHelpers";
import { createSignalingTransport } from "./signalingTransports";
import {
  collectRemoteTracks,
  createEmptyRemoteTrackState,
  deriveNetworkQualityUpdate,
  discardStaleRemoteCandidates,
  flushPendingRemoteCandidatesForCurrentEpoch,
  normalizeIceEpoch,
  queueOrApplyRemoteCandidate,
  removeRemoteTrackState,
  toRTCIceCandidateInit,
  upsertRemoteTrackState,
  type RemoteTrackLike,
  type RemoteTrackState,
} from "./signalingRtcUtils";
import type {
  CallDirection,
  CallSession,
  CallStatus,
  IceCandidatePayload,
  NetworkQuality,
  RTCIceServerConfig,
  SignalingContextValue,
  SignalingTransport,
  TranslationInitStatus,
  TranslationMode
} from "./signalingTypes";
import { findTranslationUsage } from "../utils/usage";
type MediaTrack = RemoteTrackLike;

export const SignalingProvider: React.FC<{ children: React.ReactNode }> = ({
  children
}) => {
  const { token } = useAuthContext();
  const { tier, usage, refreshCommercialState } = useCommercial();
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
  const [translationLanguage, setTranslationLanguage] = useState<string>(TRANSLATION_TARGET_LANG);
  const [translationSourceLanguage, setTranslationSourceLanguage] = useState<string>(TRANSLATION_SOURCE_LANG);
  const [translationMode] = useState<TranslationMode>(TRANSLATION_MODE);
  const [translationOnlineStatus, setTranslationOnlineStatus] = useState<OnlineTranslationStatus>("idle");
  const [translationInitStatus, setTranslationInitStatus] = useState<TranslationInitStatus>("idle");
  const [translationInitError, setTranslationInitError] = useState<string | null>(null);
  const [translationPaywallReason, setTranslationPaywallReason] = useState<string | null>(null);

  const [e2eeEnabled, setE2eeEnabled] = useState<boolean>(false);
  const [e2eeFingerprint, setE2eeFingerprint] = useState<string | null>(null);
  const [e2eeSessionEstablished, setE2eeSessionEstablished] = useState<boolean>(false);
  const e2eeKeyExchangeRef = useRef<E2EEKeyExchange | null>(null);
  const e2eeDataChannelRef = useRef<any | null>(null);

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
  const remoteStreamRef = useRef<MediaStream | null>(null);
  const remoteTrackStateRef = useRef<RemoteTrackState<MediaTrack>>(createEmptyRemoteTrackState());
  const subtitlesDataChannelRef = useRef<any | null>(null);
  const translationStartedTrackedRef = useRef<boolean>(false);

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

  const translationQuota = useMemo(() => findTranslationUsage(usage), [usage]);
  const translationQuotaRemaining = translationQuota?.unlimited
    ? null
    : translationQuota?.remaining_units ?? null;
  const translationRequiresPremium = tier !== "premium" && translationQuotaRemaining === 0;

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
          setIceServers(preferRestrictedIceServers(servers as RTCIceServerConfig[], RESTRICTED_NETWORK_MODE));
        } else if (!cancelled) {
          setIceServers(DEFAULT_ICE_SERVERS);
        }
      } catch {
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
    const result = await permissionsAdapter.requestMeetingPermissions();
    return result.allGranted;
  }, []);

  const syncRemoteMediaFlags = useCallback((trackState: RemoteTrackState<MediaTrack>) => {
    setIsRemoteAudioEnabled(
      Boolean(trackState.audio && trackState.audio.readyState !== "ended" && trackState.audio.muted !== true)
    );
    setIsRemoteVideoEnabled(
      Boolean(trackState.video && trackState.video.readyState !== "ended" && trackState.video.muted !== true)
    );
  }, []);

  const rebuildRemoteStreamWithTracks = useCallback((trackState: RemoteTrackState<MediaTrack>) => {
    const stream = new MediaStream();
    if (trackState.audio) {
      stream.addTrack(trackState.audio as never);
    }
    if (trackState.video) {
      stream.addTrack(trackState.video as never);
    }
    remoteStreamRef.current = stream;
    setRemoteStream(trackState.audio || trackState.video ? stream : null);
    syncRemoteMediaFlags(trackState);
    return stream;
  }, [syncRemoteMediaFlags]);

  const removeRemoteTrack = useCallback((track: MediaTrack | null | undefined) => {
    if (!track || (track.kind !== "audio" && track.kind !== "video")) return;
    remoteTrackStateRef.current = removeRemoteTrackState(remoteTrackStateRef.current, track);
    rebuildRemoteStreamWithTracks(remoteTrackStateRef.current);
  }, [rebuildRemoteStreamWithTracks]);

  const bindRemoteTrackState = useCallback((track: MediaTrack) => {
    if (track.kind !== "audio" && track.kind !== "video") return;
    track.onmute = () => {
      syncRemoteMediaFlags(remoteTrackStateRef.current);
    };
    track.onunmute = () => {
      syncRemoteMediaFlags(remoteTrackStateRef.current);
    };
    track.onended = () => {
      removeRemoteTrack(track);
    };
  }, [removeRemoteTrack, syncRemoteMediaFlags]);

  const upsertRemoteTrack = useCallback((track: MediaTrack) => {
    if (track.kind !== "audio" && track.kind !== "video") return;
    remoteTrackStateRef.current = upsertRemoteTrackState(remoteTrackStateRef.current, track);
    bindRemoteTrackState(track);
    rebuildRemoteStreamWithTracks(remoteTrackStateRef.current);
  }, [bindRemoteTrackState, rebuildRemoteStreamWithTracks]);

  const applyRemoteIceCandidate = useCallback(async (candidate: IceCandidatePayload) => {
    const pc = peerRef.current;
    if (!pc) return;

    const candidateEpoch = normalizeIceEpoch(candidate);
    if (candidateEpoch < iceEpochRef.current) {
      return;
    }

    try {
      await pc.addIceCandidate(new RTCIceCandidate(toRTCIceCandidateInit(candidate)));
    } catch (error) {
      if (candidateEpoch < iceEpochRef.current) {
        return;
      }
      console.warn("[SignalingContext] failed to apply remote ICE candidate:", error);
    }
  }, []);

  const flushRemoteCandidatesForCurrentEpoch = useCallback(async () => {
    pendingRemoteCandidates.current = await flushPendingRemoteCandidatesForCurrentEpoch({
      currentEpoch: iceEpochRef.current,
      pendingCandidates: pendingRemoteCandidates.current,
      applyCandidate: applyRemoteIceCandidate,
    });
  }, [applyRemoteIceCandidate]);

  const queueOrApplyRemoteIceCandidate = useCallback(async (candidate: IceCandidatePayload) => {
    pendingRemoteCandidates.current = discardStaleRemoteCandidates(
      pendingRemoteCandidates.current,
      iceEpochRef.current
    );

    await queueOrApplyRemoteCandidate({
      candidate,
      currentEpoch: iceEpochRef.current,
      hasRemoteDescription: Boolean(peerRef.current?.remoteDescription),
      pendingCandidates: pendingRemoteCandidates.current,
      applyCandidate: applyRemoteIceCandidate,
    });
  }, [applyRemoteIceCandidate]);

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
      try {
        subtitlesDataChannelRef.current.close();
      } catch {
        // Ignore channel cleanup failures during teardown.
      }
      subtitlesDataChannelRef.current = null;
    }
    if (e2eeDataChannelRef.current) {
      try {
        e2eeDataChannelRef.current.close();
      } catch {
        // Ignore channel cleanup failures during teardown.
      }
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
    if (remoteStreamRef.current) remoteStreamRef.current.getTracks().forEach((track) => track.stop());
    remoteStreamRef.current = null;
    remoteTrackStateRef.current = createEmptyRemoteTrackState();
    setLocalStream(null);
    setRemoteStream(null);
    setIsRemoteVideoEnabled(true);
    setIsRemoteAudioEnabled(true);
  }, [localStream]);

  const attachSubtitlesDataChannel = useCallback((dc: any) => {
    if (!dc) return;
    subtitlesDataChannelRef.current = dc;
    dc.onmessage = (event: any) => {
      try {
        const parsed = JSON.parse(String(event?.data ?? ''));
        if (!parsed || parsed.t !== 'subtitle') return;
        const ts = typeof parsed.timestampMs === 'number' ? parsed.timestampMs : Date.now();
        const originalText = typeof parsed.originalText === 'string' ? parsed.originalText.trim() : '';
        const translatedText = typeof parsed.translatedText === 'string' ? parsed.translatedText.trim() : '';
        const segmentId = typeof parsed.segmentId === "string" && parsed.segmentId.trim().length > 0
          ? parsed.segmentId
          : `dc-remote-${ts}`;
        const revision = typeof parsed.revision === "number" ? parsed.revision : 1;
        const isFinal = parsed.isFinal !== false;
        if (!originalText && !translatedText) return;
        useSubtitleStore.getState().upsertSubtitle({
          segmentId,
          revision,
          isFinal,
          source: "remote",
          original: originalText,
          translated: translatedText,
          timestamp: ts,
          expiresAt: ts + (isFinal ? 8000 : 3000),
        });
      } catch {
        // Ignore malformed backward-compatible subtitle payloads.
      }
    };
  }, []);

  const initializeE2EEKeyExchange = useCallback((role: KeyExchangeRole) => {
    const current = sessionRef.current;
    if (!current) return;

    if (!E2EE_ENABLED) {
      setE2eeEnabled(false);
      setE2eeSessionEstablished(false);
      setE2eeFingerprint(null);
      return;
    }

    if (!isE2EECryptoSupported()) {
      // Degrade gracefully: DTLS-SRTP still protects transport; skip app-layer key exchange.
      console.warn("[E2EE] WebCrypto SubtleCrypto unavailable; skipping E2EE key exchange");
      setE2eeEnabled(false);
      setE2eeSessionEstablished(false);
      setE2eeFingerprint(null);
      return;
    }

    const callbacks: E2EEKeyExchangeCallbacks = {
      onSessionEstablished: (session: E2EESessionKey) => {
        setE2eeFingerprint(session.fingerprint);
        setE2eeSessionEstablished(true);
        console.log(`[E2EE] Session established, fingerprint: ${session.fingerprint.slice(0, 16)}...`);
      },
      onError: (error: Error) => {
        console.error("[E2EE] Key exchange error:", error);
        setE2eeEnabled(false);
        setE2eeSessionEstablished(false);
        setE2eeFingerprint(null);

        // Unsupported runtime is expected on some RN/Hermes builds; avoid disruptive popup.
        if (error instanceof E2EEUnsupportedError) {
          return;
        }

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
    try { client.send(message); } catch {
      if (message.type !== "ice.candidate") {
        Alert.alert("错误 / Connection Issue", "无法发送信令消息 / Failed to send signaling message.");
      }
    }
  }, []);

  const resetCallState = useCallback(() => {
    pendingTarget.current = null;
    translationStartedTrackedRef.current = false;
    setTranslationEnabled(false);
    setTranslationPaywallReason(null);
    setTranslationInitStatus("idle");
    setTranslationInitError(null);
    setTranslationOnlineStatus("idle");
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
    } catch {
      // Ignore bitrate updates that are unsupported by the current sender.
    }
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

  const updateNetworkQualityFromReport = useCallback((pc: RTCPeerConnection) => {
    return pc.getStats().then((report) => {
      let availableBps: number | null = null;
      let currentRtt: number | null = null;
      const connectionState = pc.connectionState;
      const iceConnectionState = pc.iceConnectionState;

      report.forEach((stat: any) => {
        if (stat.type === "candidate-pair" && (stat.selected || stat.nominated)) {
          availableBps = stat.availableOutgoingBitrate ?? availableBps;
          currentRtt = stat.currentRoundTripTime ?? currentRtt;
        }
      });

      const nextNetworkQuality = deriveNetworkQualityUpdate({
        currentRtt,
        connectionState,
        iceConnectionState,
      });
      if (nextNetworkQuality) setNetworkQuality(nextNetworkQuality);

      return availableBps;
    });
  }, []);

  const startIceRestartAsCaller = useCallback(async () => {
    const current = sessionRef.current;
    const pc = peerRef.current;
    if (!current || !pc || current.direction !== "outgoing" || iceRestartAttemptsRef.current >= 2) return;
    iceRestartAttemptsRef.current += 1;
    iceEpochRef.current += 1;
    pendingRemoteCandidates.current = discardStaleRemoteCandidates(
      pendingRemoteCandidates.current,
      iceEpochRef.current
    );
    try {
      const offer = await pc.createOffer({ iceRestart: true } as any);
      await pc.setLocalDescription(offer);
      sendMessage({ type: "call.sdp.offer", call_id: current.callId, to: current.peerEmail, payload: { sdp: offer.sdp, type: offer.type, iceEpoch: iceEpochRef.current } as SdpRenegotiationPayload });
    } catch {
      // Ignore ICE restart failures; connection state logic will retry.
    }
  }, [sendMessage]);

  useEffect(() => {
    if (status !== "in_call") return;
    const pc = peerRef.current;
    if (!pc) return;
    let lastAppliedKbps: number | null = null;
    const timer = setInterval(async () => {
      if (statusRef.current !== "in_call") return;
      try {
        const availableBps = await updateNetworkQualityFromReport(pc);
        if (isVideoEnabledRef.current && videoAdaptiveBitrateEnabledRef.current && availableBps) {
          const userMaxKbps = videoMaxBitrateKbpsRef.current;
          const targetKbps = Math.max(100, Math.min(userMaxKbps, (availableBps * 0.85) / 1000));
          if (lastAppliedKbps === null || Math.abs(targetKbps - lastAppliedKbps) / lastAppliedKbps > 0.1) {
            lastAppliedKbps = targetKbps;
            await setVideoSenderMaxBitrate(targetKbps);
          }
        }
      } catch {
        // Ignore transient stats collection failures.
      }
    }, 4000);
    return () => clearInterval(timer);
  }, [status, setVideoSenderMaxBitrate, updateNetworkQualityFromReport]);

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
        const tracks = collectRemoteTracks<MediaTrack>({
          track: event?.track,
          streams: Array.isArray(event?.streams) ? event.streams : [],
        });

        if (!tracks.length) {
          console.warn("[SignalingContext] ontrack received no stream/track");
          return;
        }

        tracks.forEach(upsertRemoteTrack);
      } catch (error) {
        console.error("[SignalingContext] ontrack handler failed:", error);
      }
    };
    (pc as any).ondatachannel = (event: any) => {
      if (event.channel?.label === 'subtitles') {
        attachSubtitlesDataChannel(event.channel);
      } else if (event.channel?.label === 'e2ee-key-exchange') {
        if (!E2EE_ENABLED) {
          try { event.channel?.close?.(); } catch {}
          return;
        }
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
  }, [
    attachSubtitlesDataChannel,
    iceServers,
    initializeE2EEKeyExchange,
    requestIceRestart,
    resetCallState,
    sendMessage,
    startIceRestartAsCaller,
    upsertRemoteTrack
  ]);

  useEffect(() => {
    if (!token) {
      signalingRef.current?.disconnect();
      signalingRef.current = null;
      setConnectionReady(false);
      resetCallState();
      return;
    }
    const transport = createSignalingTransport(token, SIGNALING_TRANSPORT_MODE);
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
              await flushRemoteCandidatesForCurrentEpoch();
            }
            setStatus("in_call");
            sendMediaUpdate(msg.call_id ?? "", msg.from ?? "", { audioEnabled: isAudioEnabledRef.current, videoEnabled: isVideoEnabledRef.current });
            break;
          case "call.media_update": {
            const p = msg.payload as any;
            if (typeof p?.videoEnabled === "boolean") setIsRemoteVideoEnabled(p.videoEnabled);
            if (typeof p?.audioEnabled === "boolean") setIsRemoteAudioEnabled(p.audioEnabled);
            break;
          }
          case "call.subtitle": {
            const subtitle = msg.payload as SubtitlePayload | undefined;
            const subtitleTimestamp = typeof subtitle?.timestamp_ms === "number" ? subtitle.timestamp_ms : Date.now();
            const subtitleOriginal = typeof subtitle?.original_text === "string" ? subtitle.original_text.trim() : "";
            const subtitleTranslated = typeof subtitle?.translated_text === "string" ? subtitle.translated_text.trim() : "";
            if (!subtitleOriginal && !subtitleTranslated) break;
            useSubtitleStore.getState().upsertSubtitle({
              segmentId:
                typeof subtitle?.segment_id === "string" && subtitle.segment_id.length > 0
                  ? subtitle.segment_id
                  : `signal-remote-${subtitleTimestamp}`,
              revision: typeof subtitle?.revision === "number" ? subtitle.revision : 1,
              isFinal: subtitle?.is_final !== false,
              source: "remote",
              original: subtitleOriginal,
              translated: subtitleTranslated,
              timestamp: subtitleTimestamp,
            });
            break;
          }
          case "call.ice-restart.request":
            if (statusRef.current === "in_call" && sessionRef.current?.direction === "outgoing") startIceRestartAsCaller();
            break;
          case "call.sdp.offer":
            if (peerRef.current && statusRef.current === "in_call") {
              const payload = msg.payload as any;
              const nextEpoch = typeof payload.iceEpoch === "number" ? payload.iceEpoch : 0;
              if (nextEpoch > iceEpochRef.current) {
                iceEpochRef.current = nextEpoch;
                pendingRemoteCandidates.current = discardStaleRemoteCandidates(
                  pendingRemoteCandidates.current,
                  iceEpochRef.current
                );
              }
              await peerRef.current.setRemoteDescription(new RTCSessionDescription(payload));
              await flushRemoteCandidatesForCurrentEpoch();
              const answer = await peerRef.current.createAnswer();
              await peerRef.current.setLocalDescription(answer);
              sendMessage({ type: "call.sdp.answer", call_id: sessionRef.current?.callId ?? "", to: msg.from ?? "", payload: { sdp: answer.sdp, type: answer.type, iceEpoch: iceEpochRef.current } as any });
            }
            break;
          case "call.sdp.answer":
            if (peerRef.current && statusRef.current === "in_call") {
              const payload = msg.payload as any;
              const nextEpoch = typeof payload?.iceEpoch === "number" ? payload.iceEpoch : 0;
              if (nextEpoch > iceEpochRef.current) {
                iceEpochRef.current = nextEpoch;
                pendingRemoteCandidates.current = discardStaleRemoteCandidates(
                  pendingRemoteCandidates.current,
                  iceEpochRef.current
                );
              }
              await peerRef.current.setRemoteDescription(new RTCSessionDescription(payload));
              await flushRemoteCandidatesForCurrentEpoch();
            }
            break;
          case "call.reject":
          case "call.end":
            Alert.alert("Call " + (msg.type === "call.reject" ? "rejected" : "ended"), `${msg.from ?? "Peer"} ${msg.type === "call.reject" ? "declined" : "ended"} the call.`);
            resetCallState();
            break;
          case "ice.candidate":
            await queueOrApplyRemoteIceCandidate(msg.payload as IceCandidatePayload);
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
  }, [
    flushRemoteCandidatesForCurrentEpoch,
    queueOrApplyRemoteIceCandidate,
    resetCallState,
    sendMediaUpdate,
    sendMessage,
    startIceRestartAsCaller,
    token
  ]);

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
      if (E2EE_ENABLED) {
        const e2eeDc = (pc as any).createDataChannel?.('e2ee-key-exchange', { ordered: true });
        if (e2eeDc) {
          e2eeDataChannelRef.current = e2eeDc;
          e2eeDc.onopen = () => {
            console.log("[E2EE] Data Channel opened (initiator)");
            initializeE2EEKeyExchange("initiator");
          };
        }
      }
      stream.getTracks().forEach(t => pc.addTrack(t, stream));
      const offer = await pc.createOffer({ offerToReceiveAudio: true, offerToReceiveVideo: true });
      await pc.setLocalDescription(offer);
      if (settings.defaultVideoEnabled) await applyCurrentVideoBitrate();
      pendingTarget.current = email; setStatus("connecting");
      void AsyncStorage.setItem(FIRST_CALL_STARTED_STORAGE_KEY, "true");
      sendMessage({ type: "call.invite", to: email, payload: { sdp: offer.sdp, type: offer.type } });
    } catch {
      resetCallState();
    }
  }, [
    applyCurrentVideoBitrate,
    attachSubtitlesDataChannel,
    createPeerConnection,
    ensureAudioPermission,
    initializeE2EEKeyExchange,
    resetCallState,
    resetPeerResources,
    sendMessage,
    settings,
    status
  ]);

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
      const incomingOfferEpoch = typeof (session.offer as SdpRenegotiationPayload).iceEpoch === "number"
        ? (session.offer as SdpRenegotiationPayload).iceEpoch ?? 0
        : 0;
      if (incomingOfferEpoch > iceEpochRef.current) {
        iceEpochRef.current = incomingOfferEpoch;
        pendingRemoteCandidates.current = discardStaleRemoteCandidates(
          pendingRemoteCandidates.current,
          iceEpochRef.current
        );
      }
      await pc.setRemoteDescription(new RTCSessionDescription(session.offer as any));
      await flushRemoteCandidatesForCurrentEpoch();
      const answer = await pc.createAnswer();
      await pc.setLocalDescription(answer);
      if (settings.defaultVideoEnabled) await applyCurrentVideoBitrate();
      sendMessage({ type: "call.accept", call_id: session.callId, to: session.peerEmail, payload: { sdp: answer.sdp, type: answer.type } });
      sendMediaUpdate(session.callId, session.peerEmail, { audioEnabled: settings.defaultAudioEnabled, videoEnabled: settings.defaultVideoEnabled });
      setStatus("in_call");
    } catch {
      resetCallState();
    }
  }, [session, ensureAudioPermission, settings, createPeerConnection, flushRemoteCandidatesForCurrentEpoch, applyCurrentVideoBitrate, sendMessage, sendMediaUpdate, resetCallState]);

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
    } catch {
      // Ignore toggle failures and keep the current local media state.
    }
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
    } catch {
      // Ignore camera switch failures and preserve the current camera state.
    }
  }, [localStream, isVideoEnabled, cameraFacing, applyCurrentVideoBitrate]);

  const toggleSpeaker = useCallback(async () => {
    const next = !isSpeakerOn; setIsSpeakerOn(next); await AudioService.setSpeakerphone(next);
  }, [isSpeakerOn]);

  const pushLocalSubtitle = useCallback((
    segmentId: string,
    revision: number,
    isFinal: boolean,
    source: "online",
    original: string,
    translated: string,
    timestamp: number
  ) => {
    if (!original.trim() && !translated.trim()) return;
    useSubtitleStore.getState().upsertSubtitle({
      segmentId,
      revision,
      isFinal,
      source,
      original,
      translated,
      timestamp,
    });
  }, []);

  const sendFinalSubtitleToPeer = useCallback((
    segmentId: string,
    revision: number,
    original: string,
    translated: string,
    timestampMs: number,
    source: "online"
  ) => {
    const current = sessionRef.current;
    if (!current?.peerEmail) return;

    sendMessage({
      type: "call.subtitle",
      call_id: current.callId,
      to: current.peerEmail,
      payload: {
        segment_id: segmentId,
        revision,
        is_final: true,
        original_text: original,
        translated_text: translated,
        timestamp_ms: timestampMs,
        source,
      },
    });

    // 兼容旧版本 DataChannel 接收端，一个版本周期后可删除。
    // Keep DataChannel compatibility for one release cycle.
    const dc = subtitlesDataChannelRef.current;
    if (dc?.readyState === "open") {
      dc.send(JSON.stringify({
        t: "subtitle",
        segmentId,
        revision,
        isFinal: true,
        originalText: original,
        translatedText: translated,
        timestampMs,
      }));
    }
  }, [sendMessage]);

  const stopOnlinePipeline = useCallback(async () => {
    await OnlineTranslationService.stop();
    setTranslationOnlineStatus("idle");
  }, []);

  const startOnlinePipeline = useCallback(async (): Promise<boolean> => {
    const current = sessionRef.current;
    if (!token || !current) return false;

    setTranslationInitStatus("initializing");
    setTranslationInitError(null);
    setTranslationOnlineStatus("connecting");

    try {
      await OnlineTranslationService.start(
        {
          token,
          callId: current.callId,
          to: current.peerEmail,
          sourceLang: (translationSourceLanguage === "en" ? "en" : "zh"),
          targetLang: (translationLanguage === "zh" ? "zh" : "en"),
          chunkMs: 400,
        },
        {
          onStatus: (nextStatus) => {
            setTranslationOnlineStatus(nextStatus);
            if (nextStatus === "connected") {
              setTranslationInitStatus("ready");
              setTranslationInitError(null);
            }
          },
          onProviderError: (code, message, recoverable) => {
            if (code === "TRANSLATION_QUOTA_EXHAUSTED") {
              setTranslationPaywallReason("基础通话仍可继续使用，仅实时翻译额度已用尽。升级 Premium 后可立即恢复翻译。");
              setTranslationEnabled(false);
              void refreshCommercialState();
              AnalyticsService.track("paywall_viewed", { reason: "translation_quota_exhausted" });
              return;
            }
            setTranslationInitError(`${code}: ${message}`);
            if (!recoverable) {
              setTranslationInitStatus("failed");
              setTranslationOnlineStatus("error");
            }
          },
          onResult: (result: OnlineTranslationResult) => {
            if (!translationStartedTrackedRef.current) {
              translationStartedTrackedRef.current = true;
              AnalyticsService.track("translation_started");
            }
            const originalText = result.originalText.trim();
            const translatedText = result.translatedText.trim();
            pushLocalSubtitle(
              result.segmentId,
              result.revision,
              result.isFinal,
              "online",
              originalText,
              translatedText,
              result.timestampMs
            );

            if (result.isFinal) {
              sendFinalSubtitleToPeer(
                result.segmentId,
                result.revision,
                originalText,
                translatedText,
                result.timestampMs,
                "online"
              );
            }
          },
        }
      );
      return true;
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setTranslationInitStatus("failed");
      setTranslationInitError(message);
      setTranslationOnlineStatus("error");
      return false;
    }
  }, [pushLocalSubtitle, refreshCommercialState, sendFinalSubtitleToPeer, token, translationLanguage, translationSourceLanguage]);

  const retryTranslationInitialization = useCallback(async () => {
    if (!translationEnabled || statusRef.current !== "in_call") {
      setTranslationInitStatus("idle");
      setTranslationInitError(null);
      return;
    }

    await stopOnlinePipeline();
    await startOnlinePipeline();
  }, [startOnlinePipeline, stopOnlinePipeline, translationEnabled]);

  const toggleTranslation = useCallback(async (enabled: boolean) => {
    if (!enabled) {
      translationStartedTrackedRef.current = false;
      setTranslationEnabled(false);
      await stopOnlinePipeline();
      setTranslationInitStatus("idle");
      setTranslationInitError(null);
      setTranslationPaywallReason(null);
      useSubtitleStore.getState().clearSubtitles();
      return;
    }

    if (translationRequiresPremium) {
      setTranslationPaywallReason("基础通话仍可继续使用，仅实时翻译额度已用尽。升级 Premium 后可立即恢复翻译。");
      AnalyticsService.track("paywall_viewed", { reason: "translation_upgrade_required" });
      setTranslationEnabled(false);
      return;
    }

    translationStartedTrackedRef.current = false;
    setTranslationEnabled(true);
    setTranslationInitStatus("idle");
    setTranslationInitError(null);
    setTranslationPaywallReason(null);
    void AsyncStorage.setItem(FIRST_TRANSLATION_ENABLED_STORAGE_KEY, "true");
    void AsyncStorage.setItem(FIRST_TRANSLATION_HINT_SEEN_STORAGE_KEY, "true");
  }, [stopOnlinePipeline, translationRequiresPremium]);

  useEffect(() => {
    if (!translationEnabled || status !== "in_call" || !sessionRef.current?.peerEmail) {
      void stopOnlinePipeline();
      return;
    }

    let cancelled = false;

    const start = async () => {
      const onlineReady = await startOnlinePipeline();
      if (!onlineReady && !cancelled) {
        setTranslationInitStatus("failed");
        setTranslationOnlineStatus("error");
        setTranslationInitError((prev) => prev ?? "online translation start failed");
      }
    };

    void start();
    return () => {
      cancelled = true;
      void stopOnlinePipeline();
    };
  }, [
    startOnlinePipeline,
    status,
    stopOnlinePipeline,
    translationEnabled,
    translationLanguage,
    translationMode,
    translationSourceLanguage,
  ]);

  useEffect(() => {
    if (!translationEnabled) return;
    const timer = setInterval(() => {
      useSubtitleStore.getState().pruneExpired();
    }, 500);
    return () => clearInterval(timer);
  }, [translationEnabled]);

  useEffect(() => {
    return () => {
      void OnlineTranslationService.stop();
    };
  }, []);

  const value = useMemo<SignalingContextValue>(() => ({
    status, session, connectionReady, localStream, remoteStream, networkQuality, isVideoEnabled, isAudioEnabled, isRemoteVideoEnabled, isRemoteAudioEnabled, cameraFacing,
    videoQuality, setVideoQuality, videoMaxBitrateKbps, setVideoMaxBitrateKbps, e2eeEnabled, e2eeFingerprint, e2eeSessionEstablished, translationEnabled, translationLanguage,
    translationSourceLanguage, translationMode, translationOnlineStatus, translationInitStatus, translationInitError, translationQuotaRemaining, translationRequiresPremium, translationPaywallReason,
    startCall, acceptCall, rejectCall, endCall, toggleVideo, toggleAudio, switchCamera, toggleSpeaker, isSpeakerOn, toggleTranslation,
    setTranslationLanguage: setTranslationLanguage,
    setTranslationSourceLanguage: setTranslationSourceLanguage,
    retryTranslationInitialization,
    dismissTranslationPaywall: () => setTranslationPaywallReason(null)
  }), [status, session, connectionReady, localStream, remoteStream, networkQuality, isVideoEnabled, isAudioEnabled, isRemoteVideoEnabled, isRemoteAudioEnabled, cameraFacing,
    videoQuality, videoMaxBitrateKbps, e2eeEnabled, e2eeFingerprint, e2eeSessionEstablished, translationEnabled, translationLanguage, translationSourceLanguage, translationMode,
    translationOnlineStatus, translationInitStatus, translationInitError, translationQuotaRemaining, translationRequiresPremium, translationPaywallReason,
    startCall, acceptCall, rejectCall, endCall, toggleVideo, toggleAudio, switchCamera, toggleSpeaker, isSpeakerOn, toggleTranslation, retryTranslationInitialization]);

  return <SignalingContext.Provider value={value}>{children}</SignalingContext.Provider>;
};
