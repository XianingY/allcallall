import { describe, expect, it } from "vitest";

import {
  interpretRoomRealtimeEvent,
  upsertRemoteStream,
  type RoomRealtimeEvent,
} from "./roomRealtime";

const baseEvent: RoomRealtimeEvent = {
  event_id: 1,
  sequence: 1,
  event: "room.renegotiate",
  organization_id: 42,
  payload: { room_id: 7, sdp: "v=0..." },
  created_at: "2026-08-05T00:00:00Z",
};

describe("interpretRoomRealtimeEvent", () => {
  it("extracts a renegotiation offer for the matching room", () => {
    const action = interpretRoomRealtimeEvent(baseEvent, 7);
    expect(action).toEqual({ kind: "renegotiate", sdp: "v=0..." });
  });

  it("ignores a renegotiation offer addressed to another room", () => {
    const action = interpretRoomRealtimeEvent(baseEvent, 8);
    expect(action).toEqual({ kind: "ignore" });
  });

  it("ignores a renegotiation offer without an SDP", () => {
    const action = interpretRoomRealtimeEvent(
      { ...baseEvent, payload: { room_id: 7 } },
      7,
    );
    expect(action).toEqual({ kind: "ignore" });
  });

  it("extracts a server ICE candidate for the matching room", () => {
    const event: RoomRealtimeEvent = {
      ...baseEvent,
      event: "room.ice.candidate",
      payload: {
        room_id: 7,
        candidate: "candidate:1 1 udp 2122260223 1.2.3.4 5000 typ host",
        sdpMid: "0",
        sdpMLineIndex: 0,
        usernameFragment: "abc123",
      },
    };
    const action = interpretRoomRealtimeEvent(event, 7);
    expect(action).toEqual({
      kind: "ice",
      candidate: {
        candidate: "candidate:1 1 udp 2122260223 1.2.3.4 5000 typ host",
        sdpMid: "0",
        sdpMLineIndex: 0,
        usernameFragment: "abc123",
      },
    });
  });

  it("passes through an empty candidate as end-of-candidates", () => {
    const event: RoomRealtimeEvent = {
      ...baseEvent,
      event: "room.ice.candidate",
      payload: { room_id: 7, candidate: "", sdpMid: null, sdpMLineIndex: null },
    };
    const action = interpretRoomRealtimeEvent(event, 7);
    expect(action).toEqual({
      kind: "ice",
      candidate: { candidate: "", sdpMid: null, sdpMLineIndex: null, usernameFragment: null },
    });
  });

  it("ignores a server ICE candidate addressed to another room", () => {
    const event: RoomRealtimeEvent = {
      ...baseEvent,
      event: "room.ice.candidate",
      payload: { room_id: 9, candidate: "candidate:x" },
    };
    expect(interpretRoomRealtimeEvent(event, 7)).toEqual({ kind: "ignore" });
  });

  it("ignores unrelated events", () => {
    const event: RoomRealtimeEvent = {
      ...baseEvent,
      event: "room.something.else",
      payload: {},
    };
    expect(interpretRoomRealtimeEvent(event, 7)).toEqual({ kind: "ignore" });
  });

  it("marks the room as ended for a room.ended event", () => {
    const event: RoomRealtimeEvent = {
      ...baseEvent,
      event: "room.ended",
      payload: {},
    };
    expect(interpretRoomRealtimeEvent(event, 7)).toEqual({
      kind: "room_ended",
    });
  });
});

describe("upsertRemoteStream", () => {
  it("inserts a new stream and replaces an existing one by id", () => {
    const a = { id: "s-a" } as unknown as MediaStream;
    const b = { id: "s-b" } as unknown as MediaStream;
    const replacedA = { id: "s-a" } as unknown as MediaStream;

    let streams = upsertRemoteStream(new Map(), a);
    streams = upsertRemoteStream(streams, b);
    expect(streams.size).toBe(2);

    streams = upsertRemoteStream(streams, replacedA);
    expect(streams.size).toBe(2);
    expect(streams.get("s-a")).toBe(replacedA);
  });

  it("returns a new Map instance without mutating the input", () => {
    const original = new Map<string, MediaStream>();
    const next = upsertRemoteStream(original, { id: "x" } as unknown as MediaStream);
    expect(original.size).toBe(0);
    expect(next).not.toBe(original);
    expect(next.size).toBe(1);
  });
});
