import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { ChatRealtimeCursor } from "./chatRealtimeCursor";

describe("ChatRealtimeCursor", () => {
  it("advances since id for event-id based realtime events", () => {
    const cursor = new ChatRealtimeCursor(5_000);
    const event = {
      event_id: 12,
      event: "message.created",
      organization_id: 3,
      payload: { id: 9 },
    };

    assert.equal(cursor.getSinceId(3), 0);
    assert.equal(cursor.shouldSkip(event, 1_000), false);
    assert.equal(cursor.getSinceId(3), 12);
  });

  it("deduplicates replayed events without dropping newer events", () => {
    const cursor = new ChatRealtimeCursor(5_000);

    assert.equal(cursor.shouldSkip({
      event_id: 4,
      event: "room.member.updated",
      organization_id: 1,
      payload: { room_id: 7 },
    }, 1_000), false);
    assert.equal(cursor.shouldSkip({
      event_id: 4,
      event: "room.member.updated",
      organization_id: 1,
      payload: { room_id: 7 },
    }, 1_100), true);
    assert.equal(cursor.shouldSkip({
      event_id: 5,
      event: "room.state.updated",
      organization_id: 1,
      payload: { room_id: 7 },
    }, 1_200), false);
    assert.equal(cursor.getSinceId(1), 5);
  });

  it("keeps legacy payload-signature dedupe for events without ids", () => {
    const cursor = new ChatRealtimeCursor(5_000);
    const legacy = {
      event: "conversation.updated",
      organization_id: 2,
      payload: { conversation_id: 10, changes: { status: "pending" } },
    };

    assert.equal(cursor.shouldSkip(legacy, 1_000), false);
    assert.equal(cursor.shouldSkip(legacy, 1_100), true);
    assert.equal(cursor.shouldSkip(legacy, 8_000), false);
    assert.equal(cursor.getSinceId(2), 0);
  });
});
