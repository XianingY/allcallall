import { describe, expect, it } from "vitest";
import { initialChatCursor, reduceChatCursor, type ChatEvent } from "@/realtime/chatEvents";

const event = (id: number): ChatEvent => ({ event_id: id, sequence: id, event: "message.created", organization_id: 7, payload: {}, created_at: "2026-01-01T00:00:00Z" });

describe("reduceChatCursor", () => {
  it("deduplicates replayed events and advances the cursor", () => {
    const next = reduceChatCursor(initialChatCursor, event(4));
    expect(reduceChatCursor(next, event(4))).toBe(next);
    expect(reduceChatCursor(next, event(5)).cursor).toBe(5);
  });
});

