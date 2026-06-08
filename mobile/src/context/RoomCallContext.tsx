import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Platform } from "react-native";
import {
  MediaStream,
  RTCPeerConnection,
  RTCSessionDescription,
  mediaDevices as webrtcMediaDevices,
} from "../platform/rtc";

import {
  addRoomIceCandidate,
  fetchRoomState,
  joinRoom,
  leaveRoom,
  sendRoomOffer,
  startRoomRecording,
  stopRoomRecording,
  updateRoomMediaState,
  type MeetingControlState,
  type MeetingDeviceState,
  type MeetingJoinOptions,
  type RecordingRecord,
  type RoomRecord,
} from "../api/collaboration";
import { fetchWebRTCConfig } from "../api/webrtc";
import permissionsAdapter from "../platform/permissionsAdapter";
import AnalyticsService from "../services/AnalyticsService";
import AudioService from "../services/AudioServiceExpo";
import VideoService, { type CameraFacing } from "../services/VideoService";
import {
  buildRemoteStreamKey,
  parseParticipantIdFromMediaIds,
} from "../services/roomMediaMapping";
import {
  RoomRemoteStreamRegistry,
  type RemoteStreamRecord,
} from "../services/roomRemoteStreamRegistry";
import { applyRoomRealtimePatch } from "../services/roomRealtimeReducer";
import { DEFAULT_ICE_SERVERS } from "./signalingConstants";
import { preferRestrictedIceServers } from "./signalingHelpers";
import { RESTRICTED_NETWORK_MODE } from "../config";
import { useAuthContext } from "./AuthContext";

type MeetingRemoteStreamRecord = RemoteStreamRecord<MediaStream>;

type RoomRealtimeEvent = {
  event: string;
  organization_id: number;
  payload: unknown;
};

interface RoomCallContextValue {
  room: RoomRecord | null;
  localStream: MediaStream | null;
  remoteStreams: MeetingRemoteStreamRecord[];
  recording: RecordingRecord | null;
  deviceState: MeetingDeviceState;
  controlState: MeetingControlState;
  preparePreview: (options: MeetingJoinOptions) => Promise<void>;
  joinMeeting: (roomId: number, options: MeetingJoinOptions) => Promise<void>;
  leaveMeeting: () => Promise<void>;
  toggleAudio: () => void;
  toggleVideo: () => Promise<void>;
  switchCamera: () => Promise<void>;
  toggleSpeaker: () => Promise<void>;
  refreshRoom: (roomId?: number) => Promise<void>;
  startRecording: () => Promise<void>;
  stopRecording: () => Promise<void>;
  applyRoomEvent: (event: RoomRealtimeEvent) => void;
}

const RoomCallContext = createContext<RoomCallContextValue | undefined>(undefined);

const RoomCallProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { token } = useAuthContext();
  const [room, setRoom] = useState<RoomRecord | null>(null);
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStreams, setRemoteStreams] = useState<MeetingRemoteStreamRecord[]>([]);
  const [recording, setRecording] = useState<RecordingRecord | null>(null);
  const [controlState, setControlState] = useState<MeetingControlState>({
    joined: false,
    joining: false,
    connectionState: "idle",
  });
  const [deviceState, setDeviceState] = useState<MeetingDeviceState>({
    audioEnabled: true,
    videoEnabled: true,
    speakerOn: false,
    cameraFacing: "front",
  });

  const peerRef = useRef<RTCPeerConnection | null>(null);
  const roomRef = useRef<RoomRecord | null>(null);
  const localStreamRef = useRef<MediaStream | null>(null);
  const remoteStreamRegistryRef = useRef(new RoomRemoteStreamRegistry<MediaStream, MediaStreamTrack>());
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);

  useEffect(() => {
    roomRef.current = room;
  }, [room]);

  useEffect(() => {
    localStreamRef.current = localStream;
  }, [localStream]);

  const ensureMediaPermissions = useCallback(async () => {
    const result = await permissionsAdapter.requestMeetingPermissions();
    return result.allGranted;
  }, []);

  const loadIceServers = useCallback(async () => {
    if (!token) {
      return DEFAULT_ICE_SERVERS;
    }
    try {
      const config = await fetchWebRTCConfig(token);
      const servers = Array.isArray(config.ice_servers) ? config.ice_servers : [];
      if (servers.length === 0) {
        return DEFAULT_ICE_SERVERS;
      }
      return preferRestrictedIceServers(servers as never, RESTRICTED_NETWORK_MODE);
    } catch {
      return DEFAULT_ICE_SERVERS;
    }
  }, [token]);

  const clearRemoteStreams = useCallback(() => {
    remoteStreamRegistryRef.current.clear();
    setRemoteStreams([]);
  }, []);

  const stopLocalPreview = useCallback(() => {
    if (!localStreamRef.current) {
      return;
    }
    localStreamRef.current.getTracks().forEach((track) => track.stop());
    localStreamRef.current = null;
    setLocalStream(null);
  }, []);

  const closePeer = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
    reconnectAttemptRef.current = 0;
    if (peerRef.current) {
      try {
        peerRef.current.close();
      } catch {
        // Ignore close failures.
      }
      peerRef.current = null;
    }
  }, []);

  const refreshRoom = useCallback(async (roomId?: number) => {
    if (!token) {
      return;
    }
    const targetRoomId = roomId ?? roomRef.current?.room.id;
    if (!targetRoomId) {
      return;
    }
    const nextRoom = await fetchRoomState(token, targetRoomId);
    setRoom(nextRoom);
  }, [token]);

  const applyRoomEvent = useCallback((event: RoomRealtimeEvent) => {
    setRoom((current) => {
      return applyRoomRealtimePatch(current, event);
    });
  }, []);

  const syncRoomMediaState = useCallback(async (overrides?: {
    audioEnabled?: boolean;
    videoEnabled?: boolean;
    connectionState?: string;
  }) => {
    if (!token) {
      return;
    }
    const roomId = roomRef.current?.room.id;
    if (!roomId) {
      return;
    }
    try {
      await updateRoomMediaState(token, roomId, {
        audio_enabled: overrides?.audioEnabled ?? deviceState.audioEnabled,
        video_enabled: overrides?.videoEnabled ?? deviceState.videoEnabled,
        connection_state: overrides?.connectionState ?? controlState.connectionState,
      });
    } catch (error) {
      console.error("[RoomCallContext] Failed to sync room media state:", error);
    }
  }, [controlState.connectionState, deviceState.audioEnabled, deviceState.videoEnabled, token]);

  const preparePreview = useCallback(async (options: MeetingJoinOptions) => {
    const permitted = await ensureMediaPermissions();
    if (!permitted) {
      throw new Error("media permissions denied");
    }

    await VideoService.initialize();
    let previewStream: MediaStream | null = null;
    try {
      previewStream = await VideoService.getLocalStream(true, true, options.cameraFacing, "medium");
    } catch {
      previewStream = await VideoService.getLocalStream(true, false, options.cameraFacing, "medium");
    }
    if (!previewStream) {
      throw new Error("failed to create local preview");
    }

    previewStream.getAudioTracks().forEach((track) => {
      track.enabled = options.audioEnabled;
    });
    previewStream.getVideoTracks().forEach((track) => {
      track.enabled = options.videoEnabled;
    });

    await AudioService.setSpeakerphone(options.speakerOn);
    setDeviceState({
      audioEnabled: options.audioEnabled,
      videoEnabled: options.videoEnabled && previewStream.getVideoTracks().length > 0,
      speakerOn: options.speakerOn,
      cameraFacing: options.cameraFacing,
    });
    setLocalStream(previewStream);
  }, [ensureMediaPermissions]);

  const syncRemoteStreams = useCallback(() => {
    setRemoteStreams(remoteStreamRegistryRef.current.snapshot());
  }, []);

  const renegotiate = useCallback(async (roomId: number) => {
    if (!token || !peerRef.current) {
      return;
    }
    const offer = await peerRef.current.createOffer({
      offerToReceiveAudio: true,
      offerToReceiveVideo: true,
    });
    await peerRef.current.setLocalDescription(offer);
    const result = await sendRoomOffer(token, roomId, offer.sdp ?? "");
    await peerRef.current.setRemoteDescription(
      new RTCSessionDescription({ type: result.answer.type as RTCSdpType, sdp: result.answer.sdp })
    );
    setRoom(result.room);
  }, [token]);

  const scheduleReconnect = useCallback((roomId: number, reason = "ice_disconnected") => {
    if (reconnectTimeoutRef.current || !peerRef.current) {
      return;
    }
    reconnectTimeoutRef.current = setTimeout(() => {
      reconnectTimeoutRef.current = null;
      const attempt = reconnectAttemptRef.current + 1;
      reconnectAttemptRef.current = attempt;
      AnalyticsService.track("meeting_reconnect_started", { room_id: roomId, attempt, reason });
      void (async () => {
        try {
          setControlState((current) => ({
            ...current,
            joining: false,
            connectionState: "reconnecting",
          }));
          await refreshRoom(roomId);
          await renegotiate(roomId);
          setControlState((current) => ({
            ...current,
            joined: true,
            joining: false,
            connectionState: "connected",
          }));
          await syncRoomMediaState({ connectionState: "connected" });
          AnalyticsService.track("meeting_reconnect_succeeded", { room_id: roomId, attempt });
        } catch (error) {
          console.error("[RoomCallContext] Failed to renegotiate room connection:", error);
          if (attempt < 2) {
            scheduleReconnect(roomId, "renegotiation_failed");
            return;
          }
          setControlState((current) => ({
            ...current,
            joining: false,
            connectionState: "failed",
          }));
          void syncRoomMediaState({ connectionState: "failed" });
          AnalyticsService.track("meeting_reconnect_failed", { room_id: roomId, attempt, reason: "renegotiation_failed" });
        }
      })();
    }, 1200);
  }, [refreshRoom, renegotiate, syncRoomMediaState]);

  const joinMeeting = useCallback(async (roomId: number, options: MeetingJoinOptions) => {
    if (!token) {
      return;
    }
    setControlState({ joined: false, joining: true, connectionState: "connecting" });
    try {
      if (!localStreamRef.current) {
        await preparePreview(options);
      }

      await joinRoom(token, roomId);
      const iceServers = await loadIceServers();
      const pc = new RTCPeerConnection({ iceServers: iceServers as RTCIceServer[] } as never);
      peerRef.current = pc;

      const stream = localStreamRef.current;
      stream?.getTracks().forEach((track) => {
        pc.addTrack(track, stream);
      });

      (pc as any).onicecandidate = (event: any) => {
        if (!event.candidate) {
          return;
        }
        void addRoomIceCandidate(token, roomId, {
          candidate: event.candidate.candidate,
          sdpMid: event.candidate.sdpMid,
          sdpMLineIndex: event.candidate.sdpMLineIndex,
        });
      };

      (pc as any).ontrack = (event: any) => {
        const stream = event.streams[0] ?? new MediaStream([event.track]);
        const key = buildRemoteStreamKey(stream.id, event.track?.kind, event.track?.id);
        const participantId = parseParticipantIdFromMediaIds(stream.id, event.track?.id);
        remoteStreamRegistryRef.current.upsert({ key, stream, track: event.track, participantId });
        event.track.onended = () => {
          AnalyticsService.track("meeting_remote_stream_lost", {
            room_id: roomId,
            participant_id: participantId,
            track_kind: event.track.kind,
          });
          remoteStreamRegistryRef.current.removeTrack(key, event.track);
          syncRemoteStreams();
        };
        syncRemoteStreams();
      };

      (pc as any).onconnectionstatechange = () => {
        const nextState = pc.connectionState;
        const nextConnectionState =
          nextState === "connected"
            ? "connected"
            : nextState === "failed"
              ? "failed"
              : nextState === "disconnected"
                ? "reconnecting"
                : "connecting";
        if (nextState === "connected") {
          reconnectAttemptRef.current = 0;
          if (reconnectTimeoutRef.current) {
            clearTimeout(reconnectTimeoutRef.current);
            reconnectTimeoutRef.current = null;
          }
        }
        setControlState((current) => ({
          joined: current.joined || nextState === "connected",
          joining: nextState === "connecting",
          connectionState: nextConnectionState,
        }));
        void syncRoomMediaState({ connectionState: nextConnectionState });
      };

      (pc as any).oniceconnectionstatechange = () => {
        const nextIceState = (pc as any).iceConnectionState as string;
        if (nextIceState === "connected" || nextIceState === "completed") {
          reconnectAttemptRef.current = 0;
          if (reconnectTimeoutRef.current) {
            clearTimeout(reconnectTimeoutRef.current);
            reconnectTimeoutRef.current = null;
          }
          void refreshRoom(roomId);
          return;
        }
        if (nextIceState === "disconnected" || nextIceState === "failed") {
          setControlState((current) => ({
            ...current,
            connectionState: "reconnecting",
          }));
          void syncRoomMediaState({ connectionState: "reconnecting" });
          scheduleReconnect(roomId, nextIceState);
        }
      };

      const offer = await pc.createOffer({
        offerToReceiveAudio: true,
        offerToReceiveVideo: true,
      });
      await pc.setLocalDescription(offer);
      const result = await sendRoomOffer(token, roomId, offer.sdp ?? "");
      await pc.setRemoteDescription(
        new RTCSessionDescription({ type: result.answer.type as RTCSdpType, sdp: result.answer.sdp })
      );
      setRoom(result.room);
      setControlState({ joined: true, joining: false, connectionState: "connected" });
      await syncRoomMediaState({
        audioEnabled: options.audioEnabled,
        videoEnabled: options.videoEnabled,
        connectionState: "connected",
      });
    } catch (error) {
      AnalyticsService.track("meeting_join_failed", { room_id: roomId });
      closePeer();
      clearRemoteStreams();
      setControlState({ joined: false, joining: false, connectionState: "failed" });
      throw error;
    }
  }, [
    clearRemoteStreams,
    closePeer,
    loadIceServers,
    preparePreview,
    refreshRoom,
    scheduleReconnect,
    syncRemoteStreams,
    syncRoomMediaState,
    token,
  ]);

  const leaveMeetingInternal = useCallback(async (notifyBackend: boolean) => {
    const currentRoomId = roomRef.current?.room.id;
    closePeer();
    clearRemoteStreams();
    stopLocalPreview();
    setRecording(null);
    setControlState({ joined: false, joining: false, connectionState: "idle" });
    if (notifyBackend && token && currentRoomId) {
      try {
        await updateRoomMediaState(token, currentRoomId, {
          audio_enabled: false,
          video_enabled: false,
          connection_state: "left",
        });
        const nextRoom = await leaveRoom(token, currentRoomId);
        setRoom(nextRoom);
      } catch {
        setRoom(null);
      }
    } else {
      setRoom(null);
    }
  }, [clearRemoteStreams, closePeer, stopLocalPreview, token]);

  useEffect(() => () => {
    void leaveMeetingInternal(false);
  }, [leaveMeetingInternal]);

  const toggleAudio = useCallback(() => {
    const next = !deviceState.audioEnabled;
    localStreamRef.current?.getAudioTracks().forEach((track) => {
      track.enabled = next;
    });
    setDeviceState((current) => ({ ...current, audioEnabled: next }));
    void syncRoomMediaState({ audioEnabled: next });
  }, [deviceState.audioEnabled, syncRoomMediaState]);

  const toggleVideo = useCallback(async () => {
    const current = localStreamRef.current;
    const roomId = roomRef.current?.room.id;
    if (!current) {
      return;
    }
    const next = !deviceState.videoEnabled;
    const existingTrack = current.getVideoTracks()[0];
    if (existingTrack) {
      existingTrack.enabled = next;
      setDeviceState((value) => ({ ...value, videoEnabled: next }));
      void syncRoomMediaState({ videoEnabled: next });
      return;
    }
    if (!next) {
      setDeviceState((value) => ({ ...value, videoEnabled: false }));
      void syncRoomMediaState({ videoEnabled: false });
      return;
    }
    const videoStream = await webrtcMediaDevices.getUserMedia({
      audio: false,
      video: { facingMode: deviceState.cameraFacing === "front" ? "user" : "environment" },
    });
    const track = videoStream.getVideoTracks()[0];
    if (!track) {
      return;
    }
    current.addTrack(track);
    if (peerRef.current) {
      peerRef.current.addTrack(track, current);
      if (roomId) {
        await renegotiate(roomId);
      }
    }
    setLocalStream(new MediaStream(current.getTracks()));
    setDeviceState((value) => ({ ...value, videoEnabled: true }));
    await syncRoomMediaState({ videoEnabled: true });
  }, [deviceState.cameraFacing, deviceState.videoEnabled, renegotiate, syncRoomMediaState]);

  const switchCamera = useCallback(async () => {
    const current = localStreamRef.current;
    if (!current || !deviceState.videoEnabled) {
      return;
    }
    if (Platform.OS === "web") {
      return;
    }
    const nextFacing: CameraFacing = deviceState.cameraFacing === "front" ? "back" : "front";
    const videoStream = await webrtcMediaDevices.getUserMedia({
      audio: false,
      video: { facingMode: nextFacing === "front" ? "user" : "environment" },
    });
    const nextTrack = videoStream.getVideoTracks()[0];
    const currentTrack = current.getVideoTracks()[0];
    if (!nextTrack || !currentTrack) {
      return;
    }
    const sender = peerRef.current?.getSenders().find((item) => item.track?.kind === "video");
    if (sender) {
      await sender.replaceTrack(nextTrack);
    }
    current.removeTrack(currentTrack);
    currentTrack.stop();
    current.addTrack(nextTrack);
    setLocalStream(new MediaStream(current.getTracks()));
    setDeviceState((value) => ({ ...value, cameraFacing: nextFacing }));
    await syncRoomMediaState({ videoEnabled: true });
  }, [deviceState.cameraFacing, deviceState.videoEnabled, syncRoomMediaState]);

  const toggleSpeaker = useCallback(async () => {
    const next = !deviceState.speakerOn;
    await AudioService.setSpeakerphone(next);
    setDeviceState((current) => ({ ...current, speakerOn: next }));
  }, [deviceState.speakerOn]);

  const startRecordingForRoom = useCallback(async () => {
    if (!token || !roomRef.current) {
      return;
    }
    const next = await startRoomRecording(token, roomRef.current.room.id);
    setRecording(next);
    await refreshRoom(roomRef.current.room.id);
  }, [refreshRoom, token]);

  const stopRecordingForRoom = useCallback(async () => {
    if (!token || !roomRef.current) {
      return;
    }
    const next = await stopRoomRecording(token, roomRef.current.room.id);
    setRecording(next);
    await refreshRoom(roomRef.current.room.id);
  }, [refreshRoom, token]);

  const value = useMemo<RoomCallContextValue>(() => ({
    room,
    localStream,
    remoteStreams,
    recording,
    deviceState,
    controlState,
    preparePreview,
    joinMeeting,
    leaveMeeting: () => leaveMeetingInternal(true),
    toggleAudio,
    toggleVideo,
    switchCamera,
    toggleSpeaker,
    refreshRoom,
    startRecording: startRecordingForRoom,
    stopRecording: stopRecordingForRoom,
    applyRoomEvent,
  }), [
    room,
    localStream,
    remoteStreams,
    recording,
    deviceState,
    controlState,
    preparePreview,
    joinMeeting,
    leaveMeetingInternal,
    toggleAudio,
    toggleVideo,
    switchCamera,
    toggleSpeaker,
    refreshRoom,
    startRecordingForRoom,
    stopRecordingForRoom,
    applyRoomEvent,
  ]);

  return <RoomCallContext.Provider value={value}>{children}</RoomCallContext.Provider>;
};

export const useRoomCall = () => {
  const context = useContext(RoomCallContext);
  if (!context) {
    throw new Error("useRoomCall must be used within RoomCallProvider");
  }
  return context;
};

export default RoomCallProvider;
