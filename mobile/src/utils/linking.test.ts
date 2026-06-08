import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  buildConversationShareLinksWithOrigin,
  buildRoomShareLinksWithOrigin,
  parseConversationIdFromURL,
  parseInvitationCodeFromURL,
  parseRoomIdFromURL,
} from "./linking";

describe("linking helpers", () => {
  it("parses invitation codes from web and app URLs", () => {
    assert.equal(parseInvitationCodeFromURL("https://app.example.com/invite/abc123?utm=test"), "abc123");
    assert.equal(parseInvitationCodeFromURL("allcallall://invite/xyz#open"), "xyz");
    assert.equal(parseInvitationCodeFromURL("https://app.example.com/rooms/12"), null);
  });

  it("parses room and conversation ids from direct routes", () => {
    assert.equal(parseRoomIdFromURL("https://app.example.com/rooms/42?join=1"), 42);
    assert.equal(parseRoomIdFromURL("allcallall://rooms/7#prejoin"), 7);
    assert.equal(parseConversationIdFromURL("https://app.example.com/conversations/99"), 99);
    assert.equal(parseConversationIdFromURL("allcallall://conversations/15?open=1"), 15);
  });

  it("rejects invalid or non-positive ids", () => {
    assert.equal(parseRoomIdFromURL("https://app.example.com/rooms/not-a-number"), null);
    assert.equal(parseRoomIdFromURL("allcallall://rooms/0"), null);
    assert.equal(parseConversationIdFromURL("https://app.example.com/conversations/-1"), null);
  });

  it("builds web and app share links with a normalized origin", () => {
    assert.deepEqual(buildRoomShareLinksWithOrigin(12, "https://app.example.com/"), {
      appURL: "allcallall://rooms/12",
      webURL: "https://app.example.com/rooms/12",
    });
    assert.deepEqual(buildConversationShareLinksWithOrigin(34, "https://app.example.com///"), {
      appURL: "allcallall://conversations/34",
      webURL: "https://app.example.com/conversations/34",
    });
  });
});
