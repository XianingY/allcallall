import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { resolveDeepLink } from "../linking";

describe("resolveDeepLink", () => {
  it("maps supported app URLs to deferred navigation intents", () => {
    assert.deepEqual(resolveDeepLink("allcallall://rooms/42"), { kind: "room", roomId: 42 });
    assert.deepEqual(resolveDeepLink("allcallall://conversations/7"), { kind: "conversation", conversationId: 7 });
    assert.deepEqual(resolveDeepLink("allcallall://invite/abc"), { kind: "invitation", code: "abc" });
  });

  it("ignores unsupported and empty URLs", () => {
    assert.equal(resolveDeepLink("https://example.com"), null);
    assert.equal(resolveDeepLink(null), null);
    assert.equal(resolveDeepLink(undefined), null);
  });
});
