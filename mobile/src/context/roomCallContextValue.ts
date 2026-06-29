import { createContext, useContext } from "react";

import type { MediaStream } from "../platform/rtc";
import type {
  MeetingControlState,
  MeetingDeviceState,
  MeetingJoinOptions,
  RecordingRecord,
  RoomRecord,
} from "../api/collaboration";
import type { RemoteStreamRecord } from "../services/roomRemoteStreamRegistry";

export type MeetingRemoteStreamRecord = RemoteStreamRecord<MediaStream>;

export type RoomRealtimeEvent = {
  event: string;
  organization_id: number;
  payload: unknown;
};

export interface RoomCallContextValue {
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

export const RoomCallContext = createContext<RoomCallContextValue | undefined>(undefined);

export const useRoomCall = () => {
  const context = useContext(RoomCallContext);
  if (!context) {
    throw new Error("useRoomCall must be used within RoomCallProvider");
  }
  return context;
};
