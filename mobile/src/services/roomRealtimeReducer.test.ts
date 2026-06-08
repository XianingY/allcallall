import assert from "node:assert/strict";
import test from "node:test";

import type { RoomMemberRecord, RoomRecord } from "../api/collaboration";
import { applyRoomRealtimePatch } from "./roomRealtimeReducer";

const member = (overrides: Partial<RoomMemberRecord> = {}): RoomMemberRecord => ({
  id: 1,
  room_id: 100,
  user_id: 7,
  role: "participant",
  user_email: "alice@example.com",
  user_display_name: "Alice",
  joined: true,
  left: false,
  audio_enabled: true,
  video_enabled: true,
  connection_state: "connected",
  is_host: false,
  ...overrides,
});

const roomRecord = (overrides: Partial<RoomRecord> = {}): RoomRecord => ({
  room: {
    id: 100,
    organization_id: 10,
    title: "Support meeting",
    status: "active",
    created_by: 7,
    created_at: "2026-06-08T00:00:00Z",
    updated_at: "2026-06-08T00:00:00Z",
  },
  members: [member()],
  events: [],
  participant_count: 1,
  is_active: true,
  has_recording: false,
  ...overrides,
});

test("applyRoomRealtimePatch updates an existing member by user id", () => {
  const previous = roomRecord();
  const next = applyRoomRealtimePatch(previous, {
    event: "room.member.updated",
    payload: {
      room_id: 100,
      member: member({ user_id: 7, audio_enabled: false, connection_state: "reconnecting" }),
    },
  });

  assert.notEqual(next, previous);
  assert.equal(next?.members.length, 1);
  assert.equal(next?.members[0].audio_enabled, false);
  assert.equal(next?.members[0].connection_state, "reconnecting");
});

test("applyRoomRealtimePatch appends a new member", () => {
  const previous = roomRecord();
  const next = applyRoomRealtimePatch(previous, {
    event: "room.member.updated",
    payload: {
      room_id: 100,
      member: member({ id: 2, user_id: 8, user_email: "bob@example.com" }),
    },
  });

  assert.equal(next?.members.length, 2);
  assert.equal(next?.members[1].user_id, 8);
});

test("applyRoomRealtimePatch applies state and recording patches", () => {
  const previous = roomRecord();
  const next = applyRoomRealtimePatch(previous, {
    event: "room.recording.updated",
    payload: {
      room_id: 100,
      status: "ended",
      participant_count: 3,
      is_active: false,
      has_recording: true,
      latest_recording_id: 55,
    },
  });

  assert.equal(next?.room.status, "ended");
  assert.equal(next?.participant_count, 3);
  assert.equal(next?.is_active, false);
  assert.equal(next?.has_recording, true);
  assert.equal(next?.latest_recording_id, 55);
});

test("applyRoomRealtimePatch ignores other rooms and unknown events", () => {
  const previous = roomRecord();

  assert.equal(
    applyRoomRealtimePatch(previous, {
      event: "room.state.updated",
      payload: { room_id: 999, status: "ended" },
    }),
    previous
  );
  assert.equal(
    applyRoomRealtimePatch(previous, {
      event: "message.created",
      payload: { room_id: 100 },
    }),
    previous
  );
});
