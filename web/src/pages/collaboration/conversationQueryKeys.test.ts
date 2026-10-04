import { describe, expect, it } from "vitest";

import { conversationKeys } from "./conversationQueryKeys";

describe("conversationKeys", () => {
  it("creates nested keys without string assembly", () => {
    expect(conversationKeys.messages(7, 5)).toEqual([
      "organizations",
      7,
      "conversations",
      5,
      "messages",
    ]);
  });

  it("keeps detail and child resources under the same conversation root", () => {
    const detail = conversationKeys.detail(7, 5);

    expect(conversationKeys.pins(7, 5).slice(0, detail.length)).toEqual(detail);
    expect(conversationKeys.notes(7, 5).slice(0, detail.length)).toEqual(detail);
  });
});
