import { useCallback, useEffect, useRef, useState } from "react";

import { getWebRTCConfig } from "@/api/realtime";
import { joinRoom, leaveRoom, sendRoomICE, sendRoomOffer, updateRoomMedia } from "@/api/meetings";

interface MeetingOptions { audio: boolean; video: boolean; audioDeviceId?: string; videoDeviceId?: string }

export function useMeetingEngine(roomId: number, options: MeetingOptions) {
  const peer = useRef<RTCPeerConnection | null>(null); const localRef = useRef<MediaStream | null>(null);
  const [blockedByOtherTab] = useState(() => Boolean(localStorage.getItem(`allcallall.meeting.${roomId}`)));
  const [localStream, setLocalStream] = useState<MediaStream | null>(null); const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null); const [state, setState] = useState<RTCPeerConnectionState | "permission_denied" | "joining">(blockedByOtherTab ? "failed" : "joining"); const [audio, setAudio] = useState(options.audio); const [video, setVideo] = useState(options.video); const [error, setError] = useState(blockedByOtherTab ? "该会议已在另一个标签页中打开" : "");

  useEffect(() => {
    let active = true;
    const lockKey = `allcallall.meeting.${roomId}`;
    const owner = crypto.randomUUID();
    if (blockedByOtherTab) return;
    localStorage.setItem(lockKey, owner);
    const connect = async () => {
      try {
        await joinRoom(roomId);
        const media = await navigator.mediaDevices.getUserMedia({ audio: options.audio ? (options.audioDeviceId ? { deviceId: { exact: options.audioDeviceId } } : true) : false, video: options.video ? (options.videoDeviceId ? { deviceId: { exact: options.videoDeviceId } } : true) : false });
        if (!active) { media.getTracks().forEach((track) => track.stop()); return; }
        localRef.current = media; setLocalStream(media);
        const config = await getWebRTCConfig(); const connection = new RTCPeerConnection({ iceServers: config.ice_servers }); peer.current = connection;
        media.getTracks().forEach((track) => connection.addTrack(track, media));
        if (!options.audio) connection.addTransceiver("audio", { direction: "recvonly" }); if (!options.video) connection.addTransceiver("video", { direction: "recvonly" });
        connection.ontrack = (event) => setRemoteStream(event.streams[0] ?? new MediaStream([event.track]));
        connection.onicecandidate = (event) => { if (event.candidate) void sendRoomICE(roomId, event.candidate.toJSON()); };
        connection.onconnectionstatechange = () => { setState(connection.connectionState); void updateRoomMedia(roomId, { connection_state: connection.connectionState }); };
        const offer = await connection.createOffer(); await connection.setLocalDescription(offer); const response = await sendRoomOffer(roomId, offer.sdp ?? ""); await connection.setRemoteDescription(response.answer);
        await updateRoomMedia(roomId, { audio_enabled: options.audio, video_enabled: options.video, connection_state: "connecting" });
      } catch (caught) {
        if (caught instanceof DOMException && (caught.name === "NotAllowedError" || caught.name === "NotFoundError")) setState("permission_denied"); else setState("failed");
        setError(caught instanceof Error ? caught.message : "无法加入会议");
      }
    };
    void connect();
    const leave = () => { peer.current?.close(); peer.current = null; localRef.current?.getTracks().forEach((track) => track.stop()); localRef.current = null; if (localStorage.getItem(lockKey) === owner) localStorage.removeItem(lockKey); void leaveRoom(roomId).catch(() => undefined); };
    window.addEventListener("beforeunload", leave);
    return () => { active = false; window.removeEventListener("beforeunload", leave); leave(); };
  }, [roomId, options.audio, options.video, options.audioDeviceId, options.videoDeviceId, blockedByOtherTab]);

  const toggleAudio = useCallback(() => { const next = !audio; localRef.current?.getAudioTracks().forEach((track) => { track.enabled = next; }); setAudio(next); void updateRoomMedia(roomId, { audio_enabled: next }); }, [audio, roomId]);
  const toggleVideo = useCallback(async () => {
    const next = !video; const connection = peer.current; const stream = localRef.current; if (!connection || !stream) return;
    if (!next) { const sender = connection.getSenders().find((item) => item.track?.kind === "video"); await sender?.replaceTrack(null); stream.getVideoTracks().forEach((track) => { stream.removeTrack(track); track.stop(); }); setLocalStream(new MediaStream(stream.getTracks())); setVideo(false); void updateRoomMedia(roomId, { video_enabled: false }); return; }
    try { const camera = await navigator.mediaDevices.getUserMedia({ video: true }); const track = camera.getVideoTracks()[0]; const sender = connection.getSenders().find((item) => item.track?.kind === "video"); if (sender) await sender.replaceTrack(track); else connection.addTrack(track, stream); stream.addTrack(track); setLocalStream(new MediaStream(stream.getTracks())); setVideo(true); void updateRoomMedia(roomId, { video_enabled: true }); } catch (caught) { setError(caught instanceof Error ? caught.message : "无法开启摄像头"); }
  }, [roomId, video]);
  return { localStream, remoteStream, state, error, audio, video, toggleAudio, toggleVideo };
}
