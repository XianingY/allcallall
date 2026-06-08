import assert from "node:assert/strict";
import test from "node:test";

import {
  RoomRemoteStreamRegistry,
  type RemoteStreamLike,
  type RemoteTrackLike,
} from "./roomRemoteStreamRegistry";

interface FakeTrack extends RemoteTrackLike {
  id: string;
  kind: string;
}

class FakeStream implements RemoteStreamLike<FakeTrack> {
  id: string;
  private tracks: FakeTrack[];

  constructor(id: string, tracks: FakeTrack[] = []) {
    this.id = id;
    this.tracks = [...tracks];
  }

  getTracks(): FakeTrack[] {
    return [...this.tracks];
  }

  addTrack(track: FakeTrack): void {
    this.tracks.push(track);
  }

  removeTrack(track: FakeTrack): void {
    this.tracks = this.tracks.filter((item) => item.id !== track.id);
  }
}

const track = (kind: "audio" | "video", id: string): FakeTrack => ({ kind, id });

test("RoomRemoteStreamRegistry snapshots participant-bound streams", () => {
  const registry = new RoomRemoteStreamRegistry<FakeStream, FakeTrack>();
  const audio = track("audio", "audio-1");
  const stream = new FakeStream("participant-7-stream", [audio]);

  registry.upsert({
    key: stream.id,
    stream,
    track: audio,
    participantId: 7,
  });

  assert.deepEqual(registry.snapshot(), [{
    id: "participant-7-stream",
    stream,
    participantId: 7,
  }]);
});

test("RoomRemoteStreamRegistry adds missing tracks without duplicating existing tracks", () => {
  const registry = new RoomRemoteStreamRegistry<FakeStream, FakeTrack>();
  const audio = track("audio", "audio-1");
  const video = track("video", "video-1");
  const stream = new FakeStream("participant-7-stream", [audio]);

  registry.upsert({ key: stream.id, stream, track: audio, participantId: 7 });
  registry.upsert({ key: stream.id, stream, track: audio, participantId: 7 });
  registry.upsert({ key: stream.id, stream, track: video, participantId: 7 });

  assert.deepEqual(stream.getTracks().map((item) => item.id), ["audio-1", "video-1"]);
});

test("RoomRemoteStreamRegistry removes empty streams after track ended", () => {
  const registry = new RoomRemoteStreamRegistry<FakeStream, FakeTrack>();
  const audio = track("audio", "audio-1");
  const stream = new FakeStream("participant-7-stream", [audio]);

  registry.upsert({ key: stream.id, stream, track: audio, participantId: 7 });
  registry.removeTrack(stream.id, audio);

  assert.deepEqual(registry.snapshot(), []);
});

test("RoomRemoteStreamRegistry clear removes all streams", () => {
  const registry = new RoomRemoteStreamRegistry<FakeStream, FakeTrack>();
  const audio = track("audio", "audio-1");
  const stream = new FakeStream("participant-7-stream", [audio]);

  registry.upsert({ key: stream.id, stream, track: audio, participantId: 7 });
  registry.clear();

  assert.deepEqual(registry.snapshot(), []);
});
