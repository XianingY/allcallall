import { useEffect } from "react";
import { Alert } from "react-native";
import { RTCSessionDescription } from "../../platform/rtc";
import {
  MediaUpdatePayload,
  SessionDescriptionPayload,
  SignalMessage,
  SubtitlePayload,
  SdpRenegotiationPayload,
} from "../../api/signaling";
import { createSignalingTransport } from "../signalingTransports";
import { useSubtitleStore } from "../../store/useSubtitleStore";
import type {
  CallDirection,
  CallSession,
  CallStatus,
  IceCandidatePayload,
  SignalingTransport,
} from "../signalingTypes";
import type { SignalingPeerConnectionResult } from "./useSignalingPeerConnection";

export interface UseSignalingInboundArgs {
  token: string | null;
  signalingTransportMode: string;
  signalingRef: React.MutableRefObject<SignalingTransport | null>;
  sessionRef: React.MutableRefObject<CallSession | null>;
  statusRef: React.MutableRefObject<CallStatus>;
  pendingTarget: React.MutableRefObject<string | null>;
  pendingLocalCandidates: React.MutableRefObject<IceCandidatePayload[]>;
  isAudioEnabledRef: React.MutableRefObject<boolean>;
  isVideoEnabledRef: React.MutableRefObject<boolean>;
  setConnectionReady: React.Dispatch<React.SetStateAction<boolean>>;
  sendMessage: (message: SignalMessage) => void;
  resetCallState: () => void;
  sendMediaUpdate: (callId: string, peerEmail: string, update: MediaUpdatePayload) => void;
  setSession: React.Dispatch<React.SetStateAction<CallSession | null>>;
  setStatus: React.Dispatch<React.SetStateAction<CallStatus>>;
  pc: SignalingPeerConnectionResult;
}

/**
 * Owns the signaling transport setup and the inbound `SignalMessage` dispatch.
 *
 * The provider passes in the cross-cutting refs/setters/peer-connection it still
 * owns so this hook stays a thin transport + message-routing layer. The effect
 * body (transport creation, open/close/message handlers, and the big
 * `SignalMessage` switch) is moved here verbatim from the provider — note
 * `setSession`/`setStatus` are included as params because the verbatim handler
 * writes call lifecycle state directly.
 */
export const useSignalingInbound = ({
  token,
  signalingTransportMode,
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
}: UseSignalingInboundArgs): void => {
  // Destructure the stable peer-connection members so the effect dependency
  // array lists stable callbacks/refs (matching the original provider pattern).
  const {
    peerRef,
    iceEpochRef,
    flushRemoteCandidatesForCurrentEpoch,
    queueOrApplyRemoteIceCandidate,
    startIceRestartAsCaller,
    setIsRemoteVideoEnabled,
    setIsRemoteAudioEnabled,
    advanceIceEpoch,
  } = pc;

  useEffect(() => {
    if (!token) {
      signalingRef.current?.disconnect();
      signalingRef.current = null;
      setConnectionReady(false);
      resetCallState();
      return;
    }
    const transport = createSignalingTransport(token, signalingTransportMode);
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
              pendingLocalCandidates.current.forEach(c => sendMessage({ type: "ice.candidate", call_id: sess.callId, to: sess.peerEmail, payload: c as IceCandidatePayload }));
              pendingLocalCandidates.current = [];
              pendingTarget.current = null;
            }
            break;
          case "call.invite":
            setSession({ callId: msg.call_id ?? "", peerEmail: msg.from ?? "", direction: "incoming", offer: msg.payload as SessionDescriptionPayload });
            setStatus("incoming");
            break;
          case "call.accept": {
            const pcObj = peerRef.current;
            if (pcObj && (msg.payload as SessionDescriptionPayload | undefined)?.sdp) {
              await pcObj.setRemoteDescription(new RTCSessionDescription(msg.payload as SessionDescriptionPayload));
              await flushRemoteCandidatesForCurrentEpoch();
            }
            setStatus("in_call");
            sendMediaUpdate(msg.call_id ?? "", msg.from ?? "", { audioEnabled: isAudioEnabledRef.current, videoEnabled: isVideoEnabledRef.current });
            break;
          }
          case "call.media_update": {
            const p = msg.payload as MediaUpdatePayload | undefined;
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
          case "call.sdp.offer": {
            const pcObj = peerRef.current;
            if (pcObj && statusRef.current === "in_call") {
              const payload = msg.payload as SdpRenegotiationPayload | undefined;
              const nextEpoch = payload && typeof payload.iceEpoch === "number" ? payload.iceEpoch : 0;
              advanceIceEpoch(nextEpoch);
              await pcObj.setRemoteDescription(new RTCSessionDescription(payload));
              await flushRemoteCandidatesForCurrentEpoch();
              const answer = await pcObj.createAnswer();
              await pcObj.setLocalDescription(answer);
              sendMessage({ type: "call.sdp.answer", call_id: sessionRef.current?.callId ?? "", to: msg.from ?? "", payload: { sdp: answer.sdp, type: answer.type, iceEpoch: iceEpochRef.current } as SdpRenegotiationPayload });
            }
            break;
          }
          case "call.sdp.answer": {
            const pcObj = peerRef.current;
            if (pcObj && statusRef.current === "in_call") {
              const payload = msg.payload as SdpRenegotiationPayload | undefined;
              const nextEpoch = typeof payload?.iceEpoch === "number" ? payload.iceEpoch : 0;
              advanceIceEpoch(nextEpoch);
              await pcObj.setRemoteDescription(new RTCSessionDescription(payload));
              await flushRemoteCandidatesForCurrentEpoch();
            }
            break;
          }
          case "call.reject":
          case "call.end":
            Alert.alert("Call " + (msg.type === "call.reject" ? "rejected" : "ended"), `${msg.from ?? "Peer"} ${msg.type === "call.reject" ? "declined" : "ended"} the call.`);
            resetCallState();
            break;
          case "ice.candidate":
            await queueOrApplyRemoteIceCandidate(msg.payload as IceCandidatePayload);
            break;
          case "call.error": {
            const reason = (msg.payload as Record<string, unknown> | undefined)?.reason;
            Alert.alert("Call error", typeof reason === "string" && reason ? reason : "Error");
            resetCallState();
            break;
          }
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
    startIceRestartAsCaller,
    setIsRemoteVideoEnabled,
    setIsRemoteAudioEnabled,
    advanceIceEpoch,
    peerRef,
    iceEpochRef,
    resetCallState,
    sendMediaUpdate,
    sendMessage,
    token,
    isAudioEnabledRef,
    isVideoEnabledRef,
    pendingLocalCandidates,
    pendingTarget,
    sessionRef,
    setConnectionReady,
    setSession,
    setStatus,
    signalingRef,
    signalingTransportMode,
    statusRef,
  ]);
};
