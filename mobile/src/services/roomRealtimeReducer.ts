import type { RoomMemberRecord, RoomRecord } from "../api/collaboration";

export interface RoomRealtimePatchEvent {
  event: string;
  payload?: unknown;
}

interface RoomPatchPayload {
  room_id?: number;
  member?: RoomMemberRecord;
  participant_count?: number;
  is_active?: boolean;
  latest_recording_id?: number | null;
  has_recording?: boolean;
  status?: string;
}

export const applyRoomRealtimePatch = (
  room: RoomRecord | null,
  event: RoomRealtimePatchEvent
): RoomRecord | null => {
  if (!room) {
    return room;
  }

  const payload = (event.payload ?? {}) as RoomPatchPayload;
  if (payload.room_id !== room.room.id) {
    return room;
  }

  if (event.event === "room.member.updated" && payload.member?.user_id) {
    const member = payload.member;
    const hasMember = room.members.some((item) => item.user_id === member.user_id);
    const nextMembers = hasMember
      ? room.members.map((item) => item.user_id === member.user_id ? { ...item, ...member } : item)
      : [...room.members, member];
    return {
      ...room,
      members: nextMembers,
    };
  }

  if (["room.state.updated", "room.recording.updated", "room.ended"].includes(event.event)) {
    return {
      ...room,
      room: {
        ...room.room,
        status: payload.status ?? room.room.status,
      },
      participant_count: payload.participant_count ?? room.participant_count,
      is_active: payload.is_active ?? room.is_active,
      has_recording: payload.has_recording ?? room.has_recording,
      latest_recording_id: payload.latest_recording_id ?? room.latest_recording_id,
    };
  }

  return room;
};
