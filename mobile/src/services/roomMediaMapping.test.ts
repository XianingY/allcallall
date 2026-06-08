import assert from "node:assert/strict";
import test from "node:test";

import {
  buildRemoteStreamKey,
  parseParticipantIdFromMediaIds,
} from "./roomMediaMapping";

test("parseParticipantIdFromMediaIds extracts stable participant ids", () => {
  assert.equal(parseParticipantIdFromMediaIds("room-1-participant-42-video"), 42);
  assert.equal(parseParticipantIdFromMediaIds("stream-a", "participant-7-audio-track"), 7);
  assert.equal(parseParticipantIdFromMediaIds("participant-0"), undefined);
  assert.equal(parseParticipantIdFromMediaIds("stream-without-id"), undefined);
});

test("buildRemoteStreamKey prefers stream id and falls back to track identity", () => {
  assert.equal(buildRemoteStreamKey(" stream-1 ", "audio", "track-1"), "stream-1");
  assert.equal(buildRemoteStreamKey("", "video", "track-2"), "video-track-2");
  assert.equal(buildRemoteStreamKey(null, "", ""), "track-unknown");
});
