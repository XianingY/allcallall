import { useCallback, useEffect, useRef, useState } from "react";

import { getWebRTCConfig } from "@/api/realtime";
import {
  joinRoom,
  leaveRoom,
  sendRoomICE,
  sendRoomOffer,
  sendRoomRenegotiationAnswer,
  updateRoomMedia,
} from "@/api/meetings";
import { useOrganization } from "@/organizations/OrganizationContext";
import { TicketSocket } from "@/realtime/TicketSocket";
import {
  interpretRoomRealtimeEvent,
  type RoomRealtimeEvent,
} from "@/meetings/roomRealtime";

interface MeetingOptions {
  audio: boolean;
  video: boolean;
  audioDeviceId?: string;
  videoDeviceId?: string;
}

export function useMeetingEngine(roomId: number, options: MeetingOptions) {
  const peer = useRef<RTCPeerConnection | null>(null);
  const localRef = useRef<MediaStream | null>(null);
  const { activeOrganization } = useOrganization();
  const organizationId = activeOrganization?.id;
  const [blockedByOtherTab] = useState(() =>
    Boolean(localStorage.getItem(`allcallall.meeting.${roomId}`)),
  );
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStreams, setRemoteStreams] = useState<
    Map<string, MediaStream>
  >(new Map());
  // peerReady flips to true only after the initial offer/answer exchange has
  // assigned `peer.current`. The room realtime socket is gated on it so a
  // server-initiated renegotiation offer can never arrive before the peer
  // connection exists (which would silently drop the offer, since backfill is
  // disabled via a max since_id).
  const [peerReady, setPeerReady] = useState(false);
  const [state, setState] = useState<
    RTCPeerConnectionState | "permission_denied" | "joining"
  >(blockedByOtherTab ? "failed" : "joining");
  const [audio, setAudio] = useState(options.audio);
  const [video, setVideo] = useState(options.video);
  const [error, setError] = useState(
    blockedByOtherTab ? "该会议已在另一个标签页中打开" : "",
  );

  useEffect(() => {
    let active = true;
    const lockKey = `allcallall.meeting.${roomId}`;
    const owner = crypto.randomUUID();
    if (blockedByOtherTab) return;
    localStorage.setItem(lockKey, owner);
    const connect = async () => {
      try {
        await joinRoom(roomId);
        const media = await navigator.mediaDevices.getUserMedia({
          audio: options.audio
            ? options.audioDeviceId
              ? { deviceId: { exact: options.audioDeviceId } }
              : true
            : false,
          video: options.video
            ? options.videoDeviceId
              ? { deviceId: { exact: options.videoDeviceId } }
              : true
            : false,
        });
        if (!active) {
          media.getTracks().forEach((track) => track.stop());
          return;
        }
        localRef.current = media;
        setLocalStream(media);
        const config = await getWebRTCConfig();
        const connection = new RTCPeerConnection({
          iceServers: config.ice_servers,
        });
        peer.current = connection;
        media.getTracks().forEach((track) => connection.addTrack(track, media));
        if (!options.audio)
          connection.addTransceiver("audio", { direction: "recvonly" });
        if (!options.video)
          connection.addTransceiver("video", { direction: "recvonly" });
        connection.ontrack = (event) => {
          const stream = event.streams[0] ?? new MediaStream([event.track]);
          // Drop the stream when any of its tracks ends (e.g. the remote
          // participant left and the SFU removed their transceiver). Without
          // this the last frame would stay frozen on screen after they leave.
          stream.getTracks().forEach((track) => {
            track.onended = () => {
              setRemoteStreams((prev) => {
                if (!prev.has(stream.id)) return prev;
                const next = new Map(prev);
                next.delete(stream.id);
                return next;
              });
            };
          });
          setRemoteStreams((prev) => {
            const next = new Map(prev);
            next.set(stream.id, stream);
            return next;
          });
        };
        connection.onicecandidate = (event) => {
          if (event.candidate) void sendRoomICE(roomId, event.candidate.toJSON());
        };
        connection.onconnectionstatechange = () => {
          setState(connection.connectionState);
          void updateRoomMedia(roomId, {
            connection_state: connection.connectionState,
          });
        };
        const offer = await connection.createOffer();
        await connection.setLocalDescription(offer);
        const response = await sendRoomOffer(roomId, offer.sdp ?? "");
        await connection.setRemoteDescription(response.answer);
        // The peer connection now exists with a remote description, so it is
        // safe to start consuming server renegotiation/ICE events.
        setPeerReady(true);
        await updateRoomMedia(roomId, {
          audio_enabled: options.audio,
          video_enabled: options.video,
          connection_state: "connecting",
        });
      } catch (caught) {
        if (
          caught instanceof DOMException &&
          (caught.name === "NotAllowedError" ||
            caught.name === "NotFoundError")
        )
          setState("permission_denied");
        else setState("failed");
        setError(caught instanceof Error ? caught.message : "无法加入会议");
      }
    };
    void connect();
    const leave = () => {
      peer.current?.close();
      peer.current = null;
      localRef.current?.getTracks().forEach((track) => track.stop());
      localRef.current = null;
      if (localStorage.getItem(lockKey) === owner)
        localStorage.removeItem(lockKey);
      void leaveRoom(roomId).catch((err) =>
        console.error("[useMeetingEngine] leaveRoom failed", err),
      );
    };
    window.addEventListener("beforeunload", leave);
    return () => {
      active = false;
      window.removeEventListener("beforeunload", leave);
      leave();
      setPeerReady(false);
    };
  }, [
    roomId,
    options.audio,
    options.video,
    options.audioDeviceId,
    options.videoDeviceId,
    blockedByOtherTab,
  ]);

  // Room realtime channel: receives server-initiated renegotiation offers and
  // (when ROOM_TRICKLE_ICE is enabled) server side ICE candidates. since_id is
  // set to the maximum so the server does NOT replay old signaling events -
  // a stale offer or candidate would corrupt the live peer connection.
  // The socket is only opened once `peerReady` is true, so a renegotiation
  // offer can never be processed before the peer connection exists.
  useEffect(() => {
    if (blockedByOtherTab || !organizationId || !peerReady) return;
    const socket = new TicketSocket<RoomRealtimeEvent>(
      "room",
      { organization_id: organizationId, since_id: Number.MAX_SAFE_INTEGER },
      (event) => {
        const action = interpretRoomRealtimeEvent(event, roomId);
        const connection = peer.current;
        if (action.kind === "renegotiate") {
          if (!connection) return;
          void (async () => {
            try {
              await connection.setRemoteDescription({
                type: "offer",
                sdp: action.sdp,
              });
              const answer = await connection.createAnswer();
              await connection.setLocalDescription(answer);
              await sendRoomRenegotiationAnswer(roomId, answer.sdp ?? "");
            } catch (err) {
              console.error("[useMeetingEngine] renegotiation failed", err);
            }
          })();
        } else if (action.kind === "ice") {
          if (!connection || !connection.remoteDescription) return;
          void connection.addIceCandidate(action.candidate).catch((err) =>
            console.error(
              "[useMeetingEngine] failed to add server ICE candidate",
              err,
            ),
          );
        } else if (action.kind === "room_ended") {
          setRemoteStreams(new Map());
        }
      },
      () => {},
    );
    socket.connect();
    return () => socket.disconnect();
  }, [roomId, organizationId, blockedByOtherTab, peerReady]);

  const toggleAudio = useCallback(() => {
    const next = !audio;
    localRef.current?.getAudioTracks().forEach((track) => {
      track.enabled = next;
    });
    setAudio(next);
    void updateRoomMedia(roomId, { audio_enabled: next });
  }, [audio, roomId]);

  const toggleVideo = useCallback(async () => {
    const next = !video;
    const connection = peer.current;
    const stream = localRef.current;
    if (!connection || !stream) return;
    if (!next) {
      const sender = connection
        .getSenders()
        .find((item) => item.track?.kind === "video");
      await sender?.replaceTrack(null);
      stream.getVideoTracks().forEach((track) => {
        stream.removeTrack(track);
        track.stop();
      });
      setLocalStream(new MediaStream(stream.getTracks()));
      setVideo(false);
      void updateRoomMedia(roomId, { video_enabled: false });
      return;
    }
    try {
      const camera = await navigator.mediaDevices.getUserMedia({
        video: true,
      });
      const track = camera.getVideoTracks()[0];
      const sender = connection
        .getSenders()
        .find((item) => item.track?.kind === "video");
      if (sender) await sender.replaceTrack(track);
      else connection.addTrack(track, stream);
      stream.addTrack(track);
      setLocalStream(new MediaStream(stream.getTracks()));
      setVideo(true);
      void updateRoomMedia(roomId, { video_enabled: true });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "无法开启摄像头");
    }
  }, [roomId, video]);

  return {
    localStream,
    remoteStreams,
    state,
    error,
    audio,
    video,
    toggleAudio,
    toggleVideo,
  };
}
