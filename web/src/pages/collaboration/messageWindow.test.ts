import { describe, expect, it } from "vitest";

import type { Message } from "@/api/collaboration";
import { windowMessages } from "@/pages/collaboration/messageWindow";

const message = (id: number) => ({
  id,
  organization_id: 1,
  conversation_id: 1,
  sender_id: 1,
  sender_email: "demo@example.com",
  sender_display_name: "Demo",
  type: "text",
  body: `message ${id}`,
  pinned: false,
  created_at: "2026-06-30T00:00:00Z",
}) as Message;

describe("windowMessages", () => {
  it("keeps the most recent messages inside the render window", () => {
    const result = windowMessages(Array.from({ length: 10 }, (_, index) => message(index + 1)), 4);

    expect(result.hiddenCount).toBe(6);
    expect(result.visible.map((item) => item.id)).toEqual([7, 8, 9, 10]);
  });

  it("does not hide messages when the list is below the limit", () => {
    const result = windowMessages([message(1), message(2)], 4);

    expect(result.hiddenCount).toBe(0);
    expect(result.visible.map((item) => item.id)).toEqual([1, 2]);
  });
});
