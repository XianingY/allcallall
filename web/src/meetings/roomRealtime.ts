import type { RealtimeTicket } from "@/api/realtime";

export interface RoomRealtimeEvent {
  event_id: number;
  sequence: number;
  event: string;
  organization_id: number;
  payload: Record<string, unknown>;
  created_at: string;
}

export type RoomRealtimeAction =
  | { kind: "renegotiate"; sdp: string }
  | { kind: "ice"; candidate: RTCIceCandidateInit }
  | { kind: "ignore" };

interface RenegotiatePayload {
  room_id?: number | string;
  sdp?: string;
}

interface IceCandidatePayload {
  room_id?: number | string;
  candidate?: string;
  sdpMid?: string;
  sdpMLineIndex?: number;
  usernameFragment?: string;
}

// interpretRoomRealtimeEvent turns a server-delivered room realtime event into
// a concrete action the meeting engine can act on. Events addressed to a
// different room are ignored so a single socket (one per organization) never
// mis-applies a renegotiation offer or ICE candidate meant for another call.
export function interpretRoomRealtimeEvent(
  event: RoomRealtimeEvent,
  roomId: number,
): RoomRealtimeAction {
  if (event.event === "room.renegotiate") {
    const payload = event.payload as RenegotiatePayload;
    if (Number(payload.room_id) !== roomId) return { kind: "ignore" };
    if (!payload.sdp) return { kind: "ignore" };
    return { kind: "renegotiate", sdp: payload.sdp };
  }
  if (event.event === "room.ice.candidate") {
    const payload = event.payload as IceCandidatePayload;
    if (Number(payload.room_id) !== roomId) return { kind: "ignore" };
    return {
      kind: "ice",
      candidate: {
        candidate: payload.candidate ?? "",
        sdpMid: payload.sdpMid ?? null,
        sdpMLineIndex: payload.sdpMLineIndex ?? null,
        usernameFragment: payload.usernameFragment ?? null,
      },
    };
  }
  return { kind: "ignore" };
}

// upsertRemoteStream stores or replaces a remote MediaStream in the
// per-participant map, keyed by the stable stream id. Returning a fresh Map
// keeps React state updates referentially distinct so the UI re-renders.
export function upsertRemoteStream(
  streams: Map<string, MediaStream>,
  stream: MediaStream,
): Map<string, MediaStream> {
  const next = new Map(streams);
  next.set(stream.id, stream);
  return next;
}

export type RoomChannel = RealtimeTicket["channel"];
