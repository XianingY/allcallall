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
  RTCSessionDescription,
  mediaDevices as webrtcMediaDevices
} from "../platform/rtc";

import {
  MediaUpdatePayload,
  SdpRenegotiationPayload,
  SignalMessage
} from "../api/signaling";
import {
  E2EE_ENABLED,
  RESTRICTED_NETWORK_MODE,
  SIGNALING_TRANSPORT_MODE,
  TRANSLATION_MODE
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
import {
  FIRST_CALL_STARTED_STORAGE_KEY,
} from "../constants/onboarding";
import { DEFAULT_ICE_SERVERS } from "./signalingConstants";
import { SignalingContext } from "./signalingContextValue";
import { preferRestrictedIceServers } from "./signalingHelpers";
import type {
  CallSession,
  CallStatus,
  IceCandidatePayload,
  RTCIceServerConfig,
  SignalingContextValue,
  SignalingTransport,
  TranslationMode
} from "./signalingTypes";
import { findTranslationUsage } from "../utils/usage";
import { useSignalingE2EE } from "./signaling/useSignalingE2EE";
import { useSignalingPeerConnection } from "./signaling/useSignalingPeerConnection";
import { useSignalingInbound } from "./signaling/useSignalingInbound";
import { useSignalingTranslation } from "./signaling/useSignalingTranslation";

export const SignalingProvider: React.FC<{ children: React.ReactNode }> = ({
  children
}) => {
  const { token } = useAuthContext();
  const { tier, usage, refreshCommercialState } = useCommercial();
  const { settings } = useSettings();
  const [status, setStatus] = useState<CallStatus>("idle");
  const [session, setSession] = useState<CallSession | null>(null);
  const [connectionReady, setConnectionReady] = useState(false);
  const [iceServers, setIceServers] = useState<RTCIceServer[]>(DEFAULT_ICE_SERVERS);
  const [isVideoEnabled, setIsVideoEnabled] = useState<boolean>(false);
  const [isAudioEnabled, setIsAudioEnabled] = useState<boolean>(true);
  const [cameraFacing, setCameraFacing] = useState<CameraFacing>("front");
  const [isSpeakerOn, setIsSpeakerOn] = useState<boolean>(false);
  const [videoQuality, setVideoQuality] = useState<VideoQuality>("medium");

  const [translationMode] = useState<TranslationMode>(TRANSLATION_MODE);

  const signalingRef = useRef<SignalingTransport | null>(null);
  const sessionRef = useRef<CallSession | null>(null);
  const statusRef = useRef<CallStatus>("idle");
  const isAudioEnabledRef = useRef<boolean>(true);
  const isVideoEnabledRef = useRef<boolean>(false);
  const videoMaxBitrateKbpsRef = useRef<number>(900);
  const videoAdaptiveBitrateEnabledRef = useRef<boolean>(false);
  const pendingTarget = useRef<string | null>(null);
  const pendingLocalCandidates = useRef<IceCandidatePayload[]>([]);
  const pcRef = useRef<ReturnType<typeof useSignalingPeerConnection> | null>(null);
  // The translation hook publishes its reset closure here so the provider's
  // resetCallState can reset translation state alongside the rest of the call.
  const translationResetRef = useRef<() => void>(() => {});

  useEffect(() => {
    setIsSpeakerOn(AudioService.getSpeakerphone());
  }, []);

  const translationQuota = useMemo(() => findTranslationUsage(usage), [usage]);
  const translationQuotaRemaining = translationQuota?.unlimited
    ? null
    : translationQuota?.remaining_units ?? null;
  const translationRequiresPremium = tier !== "premium" && translationQuotaRemaining === 0;

  const ensureAudioPermission = useCallback(async () => {
    const result = await permissionsAdapter.requestMeetingPermissions();
    return result.allGranted;
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

  const e2ee = useSignalingE2EE(sessionRef);
  const {
    e2eeEnabled,
    e2eeFingerprint,
    e2eeSessionEstablished,
    e2eeDataChannelRef,
    initializeE2EEKeyExchange,
  } = e2ee;

  const resetCallState = useCallback(() => {
    pendingTarget.current = null;
    translationResetRef.current();
    setSession(null);
    setStatus("idle");
    pcRef.current?.resetPeerResources();
  }, []);

  const pc = useSignalingPeerConnection({
    iceServers,
    sendMessage,
    sessionRef,
    statusRef,
    pendingLocalCandidates,
    resetCallState,
    initializeE2EEKeyExchange,
    e2eeDataChannelRef,
    e2eeKeyExchangeRef: e2ee.e2eeKeyExchangeRef,
    resetE2EEState: e2ee.resetE2EEState,
    videoMaxBitrateKbpsRef,
  });
  pcRef.current = pc;

  const {
    localStream,
    remoteStream,
    networkQuality,
    isRemoteVideoEnabled,
    isRemoteAudioEnabled,
    videoMaxBitrateKbps,
    setLocalStream,
    setVideoMaxBitrateKbps,
    peerRef,
    flushRemoteCandidatesForCurrentEpoch,
    resetPeerResources,
    attachSubtitlesDataChannel,
    updateNetworkQualityFromReport,
    advanceIceEpoch,
    createPeerConnection,
    setVideoSenderMaxBitrate,
    applyCurrentVideoBitrate,
  } = pc;

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

  useEffect(() => {
    if (settings?.videoQuality) setVideoQuality(settings.videoQuality);
    if (typeof settings?.videoMaxBitrateKbps === "number") {
      setVideoMaxBitrateKbps(Math.max(100, Math.min(2500, Math.trunc(settings.videoMaxBitrateKbps))));
    }
    videoAdaptiveBitrateEnabledRef.current = !!settings?.videoAdaptiveBitrateEnabled;
  }, [settings, setVideoMaxBitrateKbps]);

  useEffect(() => {
    if (status !== "in_call") return;
    const pcObj = peerRef.current;
    if (!pcObj) return;
    let lastAppliedKbps: number | null = null;
    const timer = setInterval(async () => {
      if (statusRef.current !== "in_call") return;
      try {
        const availableBps = await updateNetworkQualityFromReport(pcObj);
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
  }, [status, peerRef, updateNetworkQualityFromReport, setVideoSenderMaxBitrate]);

  // Transport setup + inbound SignalMessage dispatch (extracted to its own hook).
  useSignalingInbound({
    token,
    signalingTransportMode: SIGNALING_TRANSPORT_MODE,
    signalingRef,
    sessionRef,
    statusRef,
    pendingTarget,
    pendingLocalCandidates,
    isAudioEnabledRef,
    isVideoEnabledRef,
    setConnectionReady,
    sendMessage,
    resetCallState,
    sendMediaUpdate,
    setSession,
    setStatus,
    pc,
  });

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
      const createdPc = createPeerConnection();
      if (stream.getVideoTracks().length === 0) createdPc.addTransceiver("video", { direction: "sendrecv" });
      const dc = createdPc.createDataChannel("subtitles", { ordered: true });
      if (dc) attachSubtitlesDataChannel(dc);
      if (E2EE_ENABLED) {
        const e2eeDc = createdPc.createDataChannel("e2ee-key-exchange", { ordered: true });
        if (e2eeDc) {
          e2eeDataChannelRef.current = e2eeDc;
          e2eeDc.onopen = () => {
            console.log("[E2EE] Data Channel opened (initiator)");
            initializeE2EEKeyExchange("initiator");
          };
        }
      }
      stream.getTracks().forEach(t => createdPc.addTrack(t, stream));
      const offer = await createdPc.createOffer({ offerToReceiveAudio: true, offerToReceiveVideo: true });
      await createdPc.setLocalDescription(offer);
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
    e2eeDataChannelRef,
    ensureAudioPermission,
    initializeE2EEKeyExchange,
    resetPeerResources,
    resetCallState,
    sendMessage,
    settings,
    status,
    setLocalStream,
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
      const createdPc = createPeerConnection();
      stream.getTracks().forEach(t => createdPc.addTrack(t, stream));
      const incomingOfferEpoch = typeof (session.offer as SdpRenegotiationPayload).iceEpoch === "number"
        ? (session.offer as SdpRenegotiationPayload).iceEpoch ?? 0
        : 0;
      advanceIceEpoch(incomingOfferEpoch);
      await createdPc.setRemoteDescription(new RTCSessionDescription(session.offer));
      await flushRemoteCandidatesForCurrentEpoch();
      const answer = await createdPc.createAnswer();
      await createdPc.setLocalDescription(answer);
      if (settings.defaultVideoEnabled) await applyCurrentVideoBitrate();
      sendMessage({ type: "call.accept", call_id: session.callId, to: session.peerEmail, payload: { sdp: answer.sdp, type: answer.type } });
      sendMediaUpdate(session.callId, session.peerEmail, { audioEnabled: settings.defaultAudioEnabled, videoEnabled: settings.defaultVideoEnabled });
      setStatus("in_call");
    } catch {
      resetCallState();
    }
  }, [
    session,
    ensureAudioPermission,
    settings,
    createPeerConnection,
    flushRemoteCandidatesForCurrentEpoch,
    applyCurrentVideoBitrate,
    sendMessage,
    sendMediaUpdate,
    resetCallState,
    advanceIceEpoch,
    setLocalStream,
  ]);

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
  }, [localStream, isVideoEnabled, cameraFacing, applyCurrentVideoBitrate, sendMediaUpdate, setLocalStream, peerRef]);

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
  }, [localStream, isVideoEnabled, cameraFacing, applyCurrentVideoBitrate, setLocalStream, peerRef]);

  const toggleSpeaker = useCallback(async () => {
    const next = !isSpeakerOn; setIsSpeakerOn(next); await AudioService.setSpeakerphone(next);
  }, [isSpeakerOn]);

  // All real-time translation state + behaviour (extracted to its own hook).
  const translation = useSignalingTranslation({
    token,
    sessionRef,
    statusRef,
    status,
    translationRequiresPremium,
    translationMode,
    refreshCommercialState,
    pc,
    sendMessage,
    translationResetRef,
  });
  const {
    translationEnabled,
    translationLanguage,
    translationSourceLanguage,
    translationOnlineStatus,
    translationInitStatus,
    translationInitError,
    translationPaywallReason,
    setTranslationLanguage,
    setTranslationSourceLanguage,
    setTranslationPaywallReason,
    toggleTranslation,
    retryTranslationInitialization,
  } = translation;

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
    startCall, acceptCall, rejectCall, endCall, toggleVideo, toggleAudio, switchCamera, toggleSpeaker, isSpeakerOn, toggleTranslation, retryTranslationInitialization, setVideoMaxBitrateKbps, setTranslationLanguage, setTranslationSourceLanguage, setTranslationPaywallReason]);

  return <SignalingContext.Provider value={value}>{children}</SignalingContext.Provider>;
};
