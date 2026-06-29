import { useCallback, useEffect, useMemo, useRef } from "react";

import { getWebRTCConfig } from "@/api/realtime";
import { useAuth } from "@/auth/AuthContext";
import { CallContext } from "@/calls/CallContext";
import { useCallStore } from "@/calls/callStore";
import { TicketSocket } from "@/realtime/TicketSocket";

interface SignalMessage {
  type: string;
  call_id?: string;
  to: string;
  from?: string;
  payload?: Record<string, unknown> | RTCIceCandidateInit | null;
}

export function CallProvider({ children }: { children: React.ReactNode }) {
  const { status: authStatus } = useAuth();
  const socket = useRef<TicketSocket<SignalMessage> | null>(null);
  const peer = useRef<RTCPeerConnection | null>(null);
  const offer = useRef<RTCSessionDescriptionInit | null>(null);
  const pendingTarget = useRef("");
  const pendingCandidates = useRef<RTCIceCandidateInit[]>([]);

  const send = useCallback((message: SignalMessage) => socket.current?.send(message) ?? false, []);
  const cleanup = useCallback(() => {
    peer.current?.close(); peer.current = null; offer.current = null; pendingTarget.current = ""; pendingCandidates.current = [];
    const state = useCallStore.getState(); state.localStream?.getTracks().forEach((track) => track.stop()); state.remoteStream?.getTracks().forEach((track) => track.stop()); state.reset();
  }, []);

  const createPeer = useCallback(async () => {
    const config = await getWebRTCConfig().catch(() => ({ ice_servers: [] as RTCIceServer[] }));
    const connection = new RTCPeerConnection({ iceServers: config.ice_servers });
    peer.current = connection;
    connection.onicecandidate = (event) => {
      if (!event.candidate) return;
      const state = useCallStore.getState();
      if (state.callId && state.peerEmail) send({ type: "ice.candidate", call_id: state.callId, to: state.peerEmail, payload: event.candidate.toJSON() });
      else pendingCandidates.current.push(event.candidate.toJSON());
    };
    connection.ontrack = (event) => useCallStore.getState().patch({ remoteStream: event.streams[0] ?? new MediaStream([event.track]) });
    connection.onconnectionstatechange = () => {
      if (connection.connectionState === "connected") useCallStore.getState().patch({ status: "connected" });
      if (connection.connectionState === "disconnected") useCallStore.getState().patch({ status: "reconnecting" });
      if (connection.connectionState === "failed") useCallStore.getState().patch({ status: "failed", error: "媒体连接失败" });
    };
    return connection;
  }, [send]);

  const localMedia = useCallback(async (video = false, deviceId?: string) => {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: deviceId ? { deviceId: { exact: deviceId } } : true, video });
    useCallStore.getState().patch({ localStream: stream, cameraEnabled: video, muted: false });
    return stream;
  }, []);

  const handleSignal = useCallback(async (message: SignalMessage) => {
    const state = useCallStore.getState();
    try {
      if (message.type === "call.invite") {
        offer.current = message.payload as RTCSessionDescriptionInit;
        state.patch({ status: "incoming", callId: message.call_id ?? "", peerEmail: message.from ?? "" });
      } else if (message.type === "call.invite.ack") {
        state.patch({ status: "connecting", callId: message.call_id ?? "", peerEmail: pendingTarget.current });
        pendingCandidates.current.splice(0).forEach((candidate) => send({ type: "ice.candidate", call_id: message.call_id, to: pendingTarget.current, payload: candidate }));
      } else if (message.type === "call.accept" && peer.current) {
        await peer.current.setRemoteDescription(message.payload as RTCSessionDescriptionInit);
        state.patch({ status: "connected" });
      } else if (message.type === "ice.candidate") {
        const candidate = message.payload as RTCIceCandidateInit;
        if (peer.current?.remoteDescription) await peer.current.addIceCandidate(candidate);
        else pendingCandidates.current.push(candidate);
      } else if (message.type === "call.reject" || message.type === "call.end") {
        cleanup();
      } else if (message.type === "call.error") {
        state.patch({ status: "failed", error: String((message.payload as Record<string, unknown> | undefined)?.reason ?? "通话失败") });
      }
    } catch (error) {
      state.patch({ status: "failed", error: error instanceof Error ? error.message : "通话状态异常" });
    }
  }, [cleanup, send]);

  useEffect(() => {
    if (authStatus !== "authenticated") return;
    const client = new TicketSocket<SignalMessage>("signaling", {}, (message) => void handleSignal(message), () => undefined);
    socket.current = client; client.connect();
    return () => { client.disconnect(); socket.current = null; cleanup(); };
  }, [authStatus, cleanup, handleSignal]);

  const start = useCallback(async (email: string) => {
    if (useCallStore.getState().status !== "idle") return;
    try {
      const stream = await localMedia(false); const connection = await createPeer(); stream.getTracks().forEach((track) => connection.addTrack(track, stream));
      const description = await connection.createOffer({ offerToReceiveAudio: true, offerToReceiveVideo: true }); await connection.setLocalDescription(description);
      pendingTarget.current = email; useCallStore.getState().patch({ status: "outgoing", peerEmail: email }); send({ type: "call.invite", to: email, payload: { type: description.type, sdp: description.sdp } });
    } catch (error) { useCallStore.getState().patch({ status: "failed", error: error instanceof Error ? error.message : "无法访问麦克风" }); }
  }, [createPeer, localMedia, send]);

  const accept = useCallback(async () => {
    const state = useCallStore.getState(); if (!offer.current || state.status !== "incoming") return;
    try {
      const stream = await localMedia(false); const connection = await createPeer(); stream.getTracks().forEach((track) => connection.addTrack(track, stream)); await connection.setRemoteDescription(offer.current);
      for (const candidate of pendingCandidates.current.splice(0)) await connection.addIceCandidate(candidate);
      const answer = await connection.createAnswer(); await connection.setLocalDescription(answer); send({ type: "call.accept", call_id: state.callId, to: state.peerEmail, payload: { type: answer.type, sdp: answer.sdp } }); state.patch({ status: "connecting" });
    } catch (error) { state.patch({ status: "failed", error: error instanceof Error ? error.message : "无法接听" }); }
  }, [createPeer, localMedia, send]);

  const end = useCallback(() => { const state = useCallStore.getState(); if (state.peerEmail) send({ type: "call.end", call_id: state.callId, to: state.peerEmail }); cleanup(); }, [cleanup, send]);
  const reject = useCallback(() => { const state = useCallStore.getState(); send({ type: "call.reject", call_id: state.callId, to: state.peerEmail }); cleanup(); }, [cleanup, send]);
  const toggleMute = useCallback(() => { const state = useCallStore.getState(); const enabled = state.muted; state.localStream?.getAudioTracks().forEach((track) => { track.enabled = enabled; }); state.patch({ muted: !state.muted }); }, []);
  const toggleCamera = useCallback(async () => {
    const state = useCallStore.getState(); const connection = peer.current; if (!connection || !state.localStream) return;
    const sender = connection.getSenders().find((item) => item.track?.kind === "video");
    if (state.cameraEnabled) { await sender?.replaceTrack(null); state.localStream.getVideoTracks().forEach((track) => track.stop()); state.patch({ cameraEnabled: false }); return; }
    const video = await navigator.mediaDevices.getUserMedia({ video: true }); const track = video.getVideoTracks()[0];
    if (sender) await sender.replaceTrack(track); else connection.addTrack(track, state.localStream); state.localStream.addTrack(track); state.patch({ cameraEnabled: true });
  }, []);
  const switchInput = useCallback(async (deviceId: string) => { const state = useCallStore.getState(); const connection = peer.current; if (!connection || !state.localStream) return; const replacement = (await navigator.mediaDevices.getUserMedia({ audio: { deviceId: { exact: deviceId } } })).getAudioTracks()[0]; const sender = connection.getSenders().find((item) => item.track?.kind === "audio"); await sender?.replaceTrack(replacement); state.localStream.getAudioTracks().forEach((track) => { state.localStream?.removeTrack(track); track.stop(); }); state.localStream.addTrack(replacement); state.patch({ localStream: state.localStream }); }, []);

  const value = useMemo(() => ({ start, accept, reject, end, toggleMute, toggleCamera, switchInput }), [start, accept, reject, end, toggleMute, toggleCamera, switchInput]);
  return <CallContext.Provider value={value}>{children}</CallContext.Provider>;
}
