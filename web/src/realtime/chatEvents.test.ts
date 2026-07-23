import { describe, expect, it } from "vitest";
import { initialChatCursor, reduceChatCursor, type ChatEvent } from "@/realtime/chatEvents";

const event = (id: number): ChatEvent => ({ event_id: id, sequence: id, event: "message.created", organization_id: 7, payload: {}, created_at: "2026-01-01T00:00:00Z" });

describe("reduceChatCursor", () => {
  it("deduplicates replayed events and advances the cursor", () => {
    const next = reduceChatCursor(initialChatCursor, event(4));
    expect(reduceChatCursor(next, event(4))).toBe(next);
    expect(reduceChatCursor(next, event(5)).cursor).toBe(5);
  });

  it("ignores events whose id is at or below the current cursor", () => {
    const state = reduceChatCursor(initialChatCursor, event(10));
    const replay = reduceChatCursor(state, event(10));
    expect(replay).toBe(state);
    // An older event that already fell out of the recent window is still ignored.
    expect(reduceChatCursor(state, event(3))).toBe(state);
    // A newer event still advances.
    expect(reduceChatCursor(state, event(11)).cursor).toBe(11);
  });

  it("deduplicates an event id still held in the recent window", () => {
    let state = initialChatCursor;
    // Feed 250 distinct events so the 200-id sliding window is fully exercised.
    for (let i = 1; i <= 250; i++) state = reduceChatCursor(state, event(i));
    expect(state.cursor).toBe(250);
    expect(state.recentIds.length).toBe(200);
    // Replaying id 51 (which has scrolled out of the 200-id window) must still be
    // ignored because it is below the cursor, while id 200 (in window) is ignored too.
    expect(reduceChatCursor(state, event(51))).toBe(state);
    expect(reduceChatCursor(state, event(200))).toBe(state);
    expect(reduceChatCursor(state, event(251)).cursor).toBe(251);
  });

  it("preserves the current cursor when an older id is replayed mid-stream", () => {
    let state = initialChatCursor;
    state = reduceChatCursor(state, event(5));
    state = reduceChatCursor(state, event(2)); // below cursor, ignored
    expect(state.cursor).toBe(5);
    expect(state.recentIds).toEqual([5]);
  });
});

