import { useCallback, useRef, useState } from "react";
import { MediaStream, RTCPeerConnection, RTCIceCandidate } from "../../platform/rtc";
import { createSignalingPeerConnection } from "./peerConnectionFactory";
import { E2EE_ENABLED } from "../../config";
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
} from "../signalingRtcUtils";
import { useSubtitleStore } from "../../store/useSubtitleStore";
import type {
  CallSession,
  CallStatus,
  IceCandidatePayload,
  NetworkQuality,
  SignalMessage,
} from "../signalingTypes";
import { type KeyExchangeRole } from "../../services/e2ee/E2EEKeyExchange";

type MediaTrack = RemoteTrackLike;

export interface SignalingPeerConnectionResult {
  // State
  localStream: MediaStream | null;
  setLocalStream: React.Dispatch<React.SetStateAction<MediaStream | null>>;
  remoteStream: MediaStream | null;
  setRemoteStream: React.Dispatch<React.SetStateAction<MediaStream | null>>;
  isRemoteVideoEnabled: boolean;
  setIsRemoteVideoEnabled: React.Dispatch<React.SetStateAction<boolean>>;
  isRemoteAudioEnabled: boolean;
  setIsRemoteAudioEnabled: React.Dispatch<React.SetStateAction<boolean>>;
  networkQuality: NetworkQuality;
  videoMaxBitrateKbps: number;
  setVideoMaxBitrateKbps: React.Dispatch<React.SetStateAction<number>>;
  // Refs consumed by the provider
  peerRef: React.MutableRefObject<RTCPeerConnection | null>;
  iceEpochRef: React.MutableRefObject<number>;
  subtitlesDataChannelRef: React.MutableRefObject<any | null>;
  // Handlers
  applyRemoteIceCandidate: (candidate: IceCandidatePayload) => Promise<void>;
  flushRemoteCandidatesForCurrentEpoch: () => Promise<void>;
  queueOrApplyRemoteIceCandidate: (candidate: IceCandidatePayload) => Promise<void>;
  resetPeerResources: () => void;
  upsertRemoteTrack: (track: MediaTrack) => void;
  attachSubtitlesDataChannel: (dc: any) => void;
  updateNetworkQualityFromReport: (pc: RTCPeerConnection) => Promise<number | null>;
  startIceRestartAsCaller: () => Promise<void>;
  requestIceRestart: (callId: string, peerEmail: string, reason: string) => void;
  advanceIceEpoch: (nextEpoch: number) => void;
  createPeerConnection: () => RTCPeerConnection;
  setVideoSenderMaxBitrate: (kbps: number) => Promise<void>;
  applyCurrentVideoBitrate: () => Promise<void>;
}

export interface UseSignalingPeerConnectionArgs {
  iceServers: RTCIceServer[];
  sendMessage: (message: SignalMessage) => void;
  sessionRef: React.MutableRefObject<CallSession | null>;
  statusRef: React.MutableRefObject<CallStatus>;
  pendingLocalCandidates: React.MutableRefObject<IceCandidatePayload[]>;
  resetCallState: () => void;
  initializeE2EEKeyExchange: (role: KeyExchangeRole) => void;
  e2eeDataChannelRef: React.MutableRefObject<any | null>;
  e2eeKeyExchangeRef: React.MutableRefObject<any | null>;
  resetE2EEState: () => void;
  videoMaxBitrateKbpsRef: React.MutableRefObject<number>;
}

/**
 * Owns the RTCPeerConnection lifecycle and everything derived from it: remote
 * track bookkeeping, ICE candidate queueing/application, ICE epoch + restart
 * orchestration, network-quality stats, adaptive bitrate, and the subtitles
 * data channel. The provider passes in the cross-cutting refs/handlers it still
 * owns (transport send, call-state reset, E2EE bootstrap) so this hook stays the
 * single owner of peer-connection state.
 */
export const useSignalingPeerConnection = ({
  iceServers,
  sendMessage,
  sessionRef,
  statusRef,
  pendingLocalCandidates,
  resetCallState,
  initializeE2EEKeyExchange,
  e2eeDataChannelRef,
  e2eeKeyExchangeRef,
  resetE2EEState,
  videoMaxBitrateKbpsRef,
}: UseSignalingPeerConnectionArgs): SignalingPeerConnectionResult => {
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [isRemoteVideoEnabled, setIsRemoteVideoEnabled] = useState<boolean>(true);
  const [isRemoteAudioEnabled, setIsRemoteAudioEnabled] = useState<boolean>(true);
  const [networkQuality, setNetworkQuality] = useState<NetworkQuality>("unknown");
  const [videoMaxBitrateKbps, setVideoMaxBitrateKbps] = useState<number>(900);

  const peerRef = useRef<RTCPeerConnection | null>(null);
  const iceEpochRef = useRef<number>(0);
  const iceRestartAttemptsRef = useRef<number>(0);
  const restartTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const disconnectDeadlineRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingRemoteCandidates = useRef<IceCandidatePayload[]>([]);
  const remoteStreamRef = useRef<MediaStream | null>(null);
  const remoteTrackStateRef = useRef<RemoteTrackState<MediaTrack>>(
    createEmptyRemoteTrackState()
  );
  const subtitlesDataChannelRef = useRef<any | null>(null);
  const remoteVideoLastBytesRef = useRef<number | null>(null);
  const remoteVideoStallCountRef = useRef<number>(0);
  const remoteVideoLastRestartAtMsRef = useRef<number>(0);

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
    pendingRemoteCandidates.current = [];
    iceEpochRef.current = 0;
    iceRestartAttemptsRef.current = 0;
    remoteVideoLastBytesRef.current = null;
    remoteVideoStallCountRef.current = 0;
    remoteVideoLastRestartAtMsRef.current = 0;
    if (restartTimerRef.current) clearTimeout(restartTimerRef.current);
    if (disconnectDeadlineRef.current) clearTimeout(disconnectDeadlineRef.current);

    if (peerRef.current) {
      peerRef.current.onicecandidate = null;
      peerRef.current.ontrack = null;
      peerRef.current.onconnectionstatechange = null;
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
    resetE2EEState();
    if (localStream) localStream.getTracks().forEach((track) => track.stop());
    if (remoteStreamRef.current) remoteStreamRef.current.getTracks().forEach((track) => track.stop());
    remoteStreamRef.current = null;
    remoteTrackStateRef.current = createEmptyRemoteTrackState();
    setLocalStream(null);
    setRemoteStream(null);
    setIsRemoteVideoEnabled(true);
    setIsRemoteAudioEnabled(true);
  }, [localStream, resetE2EEState, e2eeDataChannelRef, e2eeKeyExchangeRef]);

  const attachSubtitlesDataChannel = useCallback((dc: any) => {
    if (!dc) return;
    subtitlesDataChannelRef.current = dc;
    dc.onmessage = (event: any) => {
      try {
        const parsed = JSON.parse(String(event?.data ?? ""));
        if (!parsed || parsed.t !== "subtitle") return;
        const ts = typeof parsed.timestampMs === "number" ? parsed.timestampMs : Date.now();
        const originalText = typeof parsed.originalText === "string" ? parsed.originalText.trim() : "";
        const translatedText = typeof parsed.translatedText === "string" ? parsed.translatedText.trim() : "";
        const segmentId =
          typeof parsed.segmentId === "string" && parsed.segmentId.trim().length > 0
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
  }, [setVideoSenderMaxBitrate, videoMaxBitrateKbpsRef]);

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
      const offer = await pc.createOffer({ iceRestart: true });
      await pc.setLocalDescription(offer);
      sendMessage({
        type: "call.sdp.offer",
        call_id: current.callId,
        to: current.peerEmail,
        payload: { sdp: offer.sdp, type: offer.type, iceEpoch: iceEpochRef.current },
      });
    } catch {
      // Ignore ICE restart failures; connection state logic will retry.
    }
  }, [sendMessage, sessionRef]);

  const requestIceRestart = useCallback((callId: string, peerEmail: string, reason: string) => {
    sendMessage({
      type: "call.ice-restart.request",
      call_id: callId,
      to: peerEmail,
      payload: { reason, iceEpoch: iceEpochRef.current },
    });
  }, [sendMessage]);

  const advanceIceEpoch = useCallback((nextEpoch: number) => {
    if (nextEpoch > iceEpochRef.current) {
      iceEpochRef.current = nextEpoch;
      pendingRemoteCandidates.current = discardStaleRemoteCandidates(
        pendingRemoteCandidates.current,
        iceEpochRef.current
      );
    }
  }, []);

  const createPeerConnection = useCallback(() => {
    const createdPc = createSignalingPeerConnection(iceServers, {
      onIceCandidate: (candidate) => {
        if (!candidate) return;
        const candidateInit = {
          candidate: candidate.candidate,
          sdpMid: candidate.sdpMid,
          sdpMLineIndex: candidate.sdpMLineIndex,
          iceEpoch: iceEpochRef.current,
        };
        const current = sessionRef.current;
        if (current?.callId) {
          sendMessage({ type: "ice.candidate", call_id: current.callId, to: current.peerEmail, payload: candidateInit as IceCandidatePayload });
        } else {
          pendingLocalCandidates.current.push(candidateInit);
        }
      },
      onIceConnectionStateChange: () => {
        const current = sessionRef.current;
        const pcObj = peerRef.current;
        if (pcObj && pcObj.iceConnectionState === "failed" && current && statusRef.current === "in_call") {
          if (current.direction === "outgoing") startIceRestartAsCaller();
          else requestIceRestart(current.callId, current.peerEmail, "ice_failed");
        }
      },
      onTrack: (event) => {
        try {
          const tracks = collectRemoteTracks<MediaTrack>({
            track: event?.track,
            streams: Array.isArray(event?.streams) ? event.streams : [],
          });

          if (!tracks.length) {
            console.warn("[SignalingContext] ontrack received no stream/track");
            return;
          }

          tracks.forEach((track) => upsertRemoteTrack(track));
        } catch (error) {
          console.error("[SignalingContext] ontrack handler failed:", error);
        }
      },
      onDataChannel: (event) => {
        if (event.channel?.label === "subtitles") {
          attachSubtitlesDataChannel(event.channel);
        } else if (event.channel?.label === "e2ee-key-exchange") {
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
      },
      onConnectionStateChange: () => {
        const pcObj = peerRef.current;
        if (!pcObj) return;
        const state = pcObj.connectionState;
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
      },
    });
    peerRef.current = createdPc;
    return createdPc;
  }, [
    attachSubtitlesDataChannel,
    e2eeDataChannelRef,
    iceServers,
    initializeE2EEKeyExchange,
    pendingLocalCandidates,
    requestIceRestart,
    resetCallState,
    sendMessage,
    sessionRef,
    startIceRestartAsCaller,
    statusRef,
    upsertRemoteTrack,
  ]);

  return {
    localStream,
    setLocalStream,
    remoteStream,
    setRemoteStream,
    isRemoteVideoEnabled,
    setIsRemoteVideoEnabled,
    isRemoteAudioEnabled,
    setIsRemoteAudioEnabled,
    networkQuality,
    videoMaxBitrateKbps,
    setVideoMaxBitrateKbps,
    peerRef,
    iceEpochRef,
    subtitlesDataChannelRef,
    applyRemoteIceCandidate,
    flushRemoteCandidatesForCurrentEpoch,
    queueOrApplyRemoteIceCandidate,
    resetPeerResources,
    upsertRemoteTrack,
    attachSubtitlesDataChannel,
    updateNetworkQualityFromReport,
    startIceRestartAsCaller,
    requestIceRestart,
    advanceIceEpoch,
    createPeerConnection,
    setVideoSenderMaxBitrate,
    applyCurrentVideoBitrate,
  };
};
